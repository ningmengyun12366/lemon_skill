package report

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	readingBatchTargetBytes       = 80 * 1024
	readingBusinessTopCount       = 10
	workspaceReadingIndexFilename = "analysis-batches.json"
	workspaceReadingDirectory     = "analysis-batches"
)

// ReadingBatch describes one complete-section packet. Bytes is a soft budget:
// a large section stays intact instead of silently dropping periods or facts.
type ReadingBatch struct {
	ID         string   `json:"id"`
	SectionIDs []string `json:"sectionIds"`
	Path       string   `json:"path"`
	SHA256     string   `json:"sha256"`
	Bytes      int      `json:"bytes"`
}

type ReadingIndex struct {
	AnalysisDataDigest string         `json:"analysisDataDigest"`
	Batches            []ReadingBatch `json:"batches"`
}

// businessReadingSelection summarizes object-heavy business/service chapters.
// Other chapters retain full coverage; source objects stay in the archive.
func businessReadingSelection(s SectionAnalysisData) map[string]bool {
	if s.SectionID == "invoice.supplier-credit" {
		return supplierCreditReadingSelection(s)
	}
	if s.SectionID == "invoice.sensitive-services" {
		return sensitiveReadingSelection(s)
	}
	if (s.SectionID != "invoice.category-match" && s.SectionID != "invoice.business-match") || len(s.FindingIDs) > 0 {
		return nil
	}
	selected := map[string]bool{}
	for _, f := range s.Focus {
		for _, ref := range f.EvidenceRefs {
			selected[readingGroupID(ref)] = true
		}
	}
	for _, direction := range []string{"购进", "销项"} {
		var amounts []AnalysisMetric
		for _, m := range s.Metrics {
			if strings.HasPrefix(m.ID, metricID(s.SectionID, "business.")) && m.Name == direction+"该品名金额" {
				amounts = append(amounts, m)
			}
		}
		selectReadingAmountGroups(amounts, selected)
	}
	return selected
}

// Service category totals stay complete. Only repetitive supplier/category
// groups are condensed; recorded credit events, partial queries, focus objects,
// negative amounts and unavailable values are never hidden by the top count.
func sensitiveReadingSelection(s SectionAnalysisData) map[string]bool {
	selected, recorded := map[string]bool{}, map[string]bool{}
	for _, p := range s.Coverage {
		for _, q := range p.Queries {
			if q.Outcome == "partial" || q.RecordCount != nil && *q.RecordCount > 0 || q.TotalCount != nil && *q.TotalCount > 0 {
				recorded[p.Name] = true
			}
		}
	}
	for _, f := range s.Focus {
		for _, ref := range f.EvidenceRefs {
			selected[readingGroupID(ref)] = true
		}
	}
	var amounts []AnalysisMetric
	for _, m := range s.Metrics {
		if !strings.HasPrefix(m.ID, metricID(s.SectionID, "sensitive-purchase.")) {
			continue
		}
		if strings.HasSuffix(m.ID, ".supplier") && recorded[m.Display] || m.Status != "present" {
			selected[readingGroupID(m.ID)] = true
		}
		if strings.HasSuffix(m.ID, ".amount") {
			amounts = append(amounts, m)
		}
	}
	selectReadingAmountGroups(amounts, selected)
	return selected
}

func selectReadingAmountGroups(amounts []AnalysisMetric, selected map[string]bool) {
	sort.SliceStable(amounts, func(i, j int) bool { return math.Abs(amounts[i].Value) > math.Abs(amounts[j].Value) })
	for i, m := range amounts {
		if i < readingBusinessTopCount || m.Value < 0 || m.Status != "present" {
			selected[readingGroupID(m.ID)] = true
		}
	}
}

type readingAmountSummary struct {
	count, shownCount                int
	total, shown, totalAbs, shownAbs float64
	complete                         bool
	period                           string
	refs                             []string
}

// Summaries use frozen product amounts, not a second invoice calculation.
func summarizeReadingAmounts(amounts []AnalysisMetric, selected map[string]bool) readingAmountSummary {
	result := readingAmountSummary{complete: true}
	for _, m := range amounts {
		if result.count == 0 {
			result.period = m.Period
		}
		result.count++
		result.refs = append(result.refs, m.ID)
		shown := selected[readingGroupID(m.ID)]
		if shown {
			result.shownCount++
		}
		if m.Status != "present" {
			result.complete = false
			continue
		}
		result.total += m.Value
		result.totalAbs += math.Abs(m.Value)
		if shown {
			result.shown += m.Value
			result.shownAbs += math.Abs(m.Value)
		}
	}
	return result
}

func readingGroupID(id string) string {
	if i := strings.LastIndex(id, "."); i >= 0 {
		return id[:i]
	}
	return id
}

// addReadingCoverage stores unrounded selection totals in the frozen archive,
// so coverage values are genuine references rather than untraceable prose.
func addReadingCoverage(data *AnalysisData) {
	for i := range data.Sections {
		s := &data.Sections[i]
		selected := businessReadingSelection(*s)
		if selected == nil {
			continue
		}
		if s.SectionID == "invoice.supplier-credit" {
			addSupplierCreditReadingCoverage(s, selected)
			continue
		}
		if s.SectionID == "invoice.sensitive-services" {
			var total, shown int
			var refs []string
			for _, m := range s.Metrics {
				if strings.HasPrefix(m.ID, metricID(s.SectionID, "sensitive-purchase.")) && strings.HasSuffix(m.ID, ".amount") {
					total++
					refs = append(refs, m.ID)
					if selected[readingGroupID(m.ID)] {
						shown++
					}
				}
			}
			s.number("reading-sensitive.total-count", "全部敏感服务供应商与类别组合数", "count", "组", "", "", "完整购进底稿的供应商与类别组合计数", float64(total), refs...)
			s.number("reading-sensitive.shown-count", "本批重点展示的供应商与类别组合数", "count", "组", "", "", "绝对金额前十、重点、负数、缺失以及有信用记录或部分查询结果主体的并集", float64(shown), refs...)
			continue
		}
		for _, direction := range []string{"购进", "销项"} {
			var amounts []AnalysisMetric
			for _, m := range s.Metrics {
				if m.Name == direction+"该品名金额" {
					amounts = append(amounts, m)
				}
			}
			summary := summarizeReadingAmounts(amounts, selected)
			if summary.count == 0 {
				continue
			}
			count, shownCount, refs, period := summary.count, summary.shownCount, summary.refs, summary.period
			key := "reading-coverage." + analysisToken(direction)
			s.number(key+".total-count", direction+"全部品名编码组合数", "count", "组", period, "", "完整本地品名编码组合计数", float64(count), refs...)
			s.number(key+".shown-count", direction+"重点阅读组合数", "count", "组", period, "", "绝对金额前十、focus对象、负数及缺失对象的并集", float64(shownCount), refs...)
			if !summary.complete {
				s.Metrics = append(s.Metrics, AnalysisMetric{ID: metricID(s.SectionID, key+".coverage"), Name: direction + "重点阅读绝对金额覆盖率", Kind: "share", Unit: "%", Period: period, Status: "missing", Algorithm: "存在金额缺失的组合，不能计算完整净额及覆盖率", SourceRefs: refs})
				continue
			}
			s.number(key+".total-amount", direction+"全部品名净额", "amount", "元", period, "", "完整同方向有符号品名金额合计", summary.total, refs...)
			s.number(key+".shown-amount", direction+"重点阅读品名净额", "amount", "元", period, "", "重点阅读并集的有符号金额合计", summary.shown, refs...)
			if summary.totalAbs > 0 {
				s.number(key+".coverage", direction+"重点阅读绝对金额覆盖率", "share", "%", period, "", "选中组合绝对金额之和/全部组合绝对金额之和×100；不代表业务核验比例", summary.shownAbs/summary.totalAbs*100, refs...)
			}
		}
	}
}

// readingSectionMarkdown shares algorithms once and places copyable IDs next
// to values. It never includes raw inputs or unusable diagnostic numbers.
func readingSectionMarkdown(s SectionAnalysisData, policy NarrativePolicy, global []TrustedReference) string {
	c := analysisSectionCard(s)
	selected := businessReadingSelection(s)
	metrics := readingMetricSelection(s)
	focusRefs := map[string]bool{}
	for _, f := range c.Focus {
		for _, ref := range f.EvidenceRefs {
			focusRefs[ref] = true
		}
	}
	objectGroups := map[string]bool{}
	for _, m := range s.Metrics {
		if m.Name == "购进该品名金额" || m.Name == "销项该品名金额" || strings.HasPrefix(m.ID, metricID(s.SectionID, "sensitive-purchase.")) || supplierCreditProductParty(s.SectionID, m.ID) != "" {
			objectGroups[readingGroupID(m.ID)] = true
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s %s\n模式：%s；正文%d—%d条。\n目标：%s\n", s.SectionID, s.Title, s.Mode, policy.MinItems, policy.MaxItems, s.Objective.Goal)
	b.WriteString("\n### 本节写作要求\n")
	if c.Guidance != "" {
		fmt.Fprintf(&b, "%s\n", c.Guidance)
	}
	for _, f := range c.Focus {
		fmt.Fprintf(&b, "重点 [%s]：%s\n", f.ID, f.Goal)
	}
	constraints := map[string]bool{}
	for _, x := range c.Constraints {
		if !constraints[x] {
			fmt.Fprintf(&b, "限制：%s\n", x)
			constraints[x] = true
		}
	}
	b.WriteString("\n### 企业事实与引用\n")
	comparisons, comparisonRefs := readingComparisons(s)
	b.WriteString(comparisons)
	brief, briefRefs := readingKeyFacts(s)
	b.WriteString(brief)
	for id := range comparisonRefs {
		briefRefs[id] = true
	}
	b.WriteString(readingCoverageMarkdown(s))
	if metrics != nil {
		b.WriteString("阅读范围：优先展示重点、比较两端、极值和限制；完整逐期指标可用 report-refs 补查。\n")
	}
	for _, x := range c.Limitations {
		if briefRefs[x.ID] {
			continue
		}
		fmt.Fprintf(&b, "不可用 [%s] %s，期间%s，基期%s，范围%s：%s (%s)\n", x.ID, x.Name, x.Period, x.BasePeriod, strings.Join(x.Scope, "、"), x.Reason, x.Status)
	}
	if selected != nil {
		if s.SectionID == "invoice.sensitive-services" {
			b.WriteString("阅读范围：分类金额与购进总额覆盖完整查询期间；供应商与类别组合展示绝对金额前十、重点、负数、缺失以及有信用记录或部分查询结果的主体，组合覆盖数见下。全部主体核验范围仍保留；其余金额明细可用report-refs按主体查询，未展示不等于金额为零或已核验。\n")
		} else if s.SectionID == "invoice.supplier-credit" {
			b.WriteString("采购背景阅读范围：各信用主体按已取得品名金额的绝对值展示前十项，并保留重点、负数及缺失项；所展示组数、净额及绝对金额覆盖率见各主体指标。覆盖率仅以该主体已取得的品名明细为分母，不代表完整业务或履约已核验。其余品名及金额保存在完整底稿，可用report-refs --section invoice.supplier-credit --subject 主体名称补查。\n")
		} else {
			b.WriteString("阅读范围：品名按方向展示绝对金额前十、重点及全部负数对象；覆盖指标见下。其他品名保存在本地，可用report-refs按名称或主体查询；不能据重点对象概括全部业务已核验。\n")
		}
	}
	algorithms := map[string]int{}
	var algorithmList []string
	for _, g := range c.MetricGroups {
		if selected != nil && objectGroups[g.ID] && !selected[g.ID] {
			continue
		}
		shown := false
		for _, m := range g.Metrics {
			if briefRefs[m.ID] {
				continue
			}
			if metrics != nil && !metrics[m.ID] {
				continue
			}
			if metrics == nil && len(s.FindingIDs) == 0 && (strings.HasSuffix(m.ID, ".streak-change") || strings.HasSuffix(m.ID, ".streak-observations")) && !focusRefs[m.ID] {
				continue
			}
			if !shown {
				b.WriteString("\n")
				shown = true
			}
			n, ok := algorithms[m.Algorithm]
			if !ok {
				n = len(algorithmList) + 1
				algorithms[m.Algorithm] = n
				algorithmList = append(algorithmList, m.Algorithm)
			}
			fmt.Fprintf(&b, "[%s] %s = %s %s", m.ID, m.Name, m.Display, m.Unit)
			if m.Period != "" {
				fmt.Fprintf(&b, "；期间=%s", m.Period)
			}
			if m.BasePeriod != "" {
				fmt.Fprintf(&b, "；基期=%s", m.BasePeriod)
			}
			if len(m.Scope) > 0 {
				fmt.Fprintf(&b, "；范围=%s", strings.Join(m.Scope, "、"))
			}
			fmt.Fprintf(&b, "；算法A%d\n", n)
		}
	}
	for i, a := range algorithmList {
		fmt.Fprintf(&b, "A%d：%s\n", i+1, a)
	}
	// Table references still matter where there is no equivalent metric (for
	// example unqueried company identity). Chart references duplicate the metrics.
	for _, r := range c.References {
		if !strings.Contains(r.ID, ".chart.") && (metrics == nil || metrics[r.ID]) {
			fmt.Fprintf(&b, "[%s] %s = %s\n", r.ID, r.Label, r.Value)
		}
	}
	for _, r := range global {
		for _, id := range s.FindingIDs {
			if strings.HasPrefix(r.ID, "finding:"+id+".") {
				fmt.Fprintf(&b, "[%s] %s = %s\n", r.ID, r.Label, r.Value)
			}
		}
	}
	return b.String()
}

// buildReadingBatches keeps whole sections together and uses byte size rather
// than a fixed chapter count. It returns deterministic files and their index.
func buildReadingBatches(model Model, data AnalysisData, dataDigest string) (ReadingIndex, map[string][]byte) {
	index := ReadingIndex{AnalysisDataDigest: dataDigest, Batches: []ReadingBatch{}}
	files := map[string][]byte{}
	required := stringSet(analysisRequiredSectionIDs(model))
	policies := map[string]NarrativePolicy{}
	for _, s := range model.Sections {
		policies[s.Key] = s.Narrative
	}
	global := baseTrustedReferences(model)
	var buf strings.Builder
	var ids []string
	flush := func() {
		if len(ids) == 0 {
			return
		}
		id := fmt.Sprintf("batch-%02d", len(index.Batches)+1)
		path := filepath.ToSlash(filepath.Join(workspaceReadingDirectory, id+".md"))
		payload := []byte(buf.String())
		files[path] = payload
		index.Batches = append(index.Batches, ReadingBatch{id, append([]string(nil), ids...), path, digestBytes(payload), len(payload)})
		buf.Reset()
		ids = nil
	}
	for _, s := range data.Sections {
		if !required[s.SectionID] {
			continue
		}
		text := readingSectionMarkdown(s, policies[s.SectionID], global)
		if buf.Len()+len(text) > readingBatchTargetBytes {
			flush()
		}
		buf.WriteString(text)
		buf.WriteString("\n")
		ids = append(ids, s.SectionID)
	}
	flush()
	return index, files
}

func validateReadingBatches(workspace string, manifest WorkspaceManifest, model Model, data AnalysisData) error {
	index, files := buildReadingBatches(model, data, manifest.AnalysisDataDigest)
	expected, err := marshalWorkspaceJSON(index)
	if err != nil {
		return err
	}
	actual, err := os.ReadFile(filepath.Join(workspace, workspaceReadingIndexFilename))
	if err != nil {
		return err
	}
	if digestBytes(expected) != manifest.ReadingDigest || digestBytes(actual) != manifest.ReadingDigest {
		return fmt.Errorf("阅读索引与冻结模型不一致")
	}
	for path, payload := range files {
		actual, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		if digestBytes(actual) != digestBytes(payload) {
			return fmt.Errorf("%s与冻结模型不一致", path)
		}
	}
	return nil
}
