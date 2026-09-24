package report

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

//go:embed analysis_objectives.json
var analysisObjectivesJSON []byte

// AnalysisObjective retains the audited Word location, after applying V4 and
// the user's later scope decisions. It is not a new tax-risk rule.
type AnalysisObjective struct {
	SourceSectionID string `json:"sourceSectionId"`
	WordBlock       int    `json:"wordBlock"`
	Goal            string `json:"goal"`
}

// AnalysisMetric keeps unrounded inputs and a display-only value. Missing or
// conflicting values are never registered as usable narrative references.
type AnalysisMetric struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Kind       string             `json:"kind"`
	Value      float64            `json:"value"`
	Display    string             `json:"display"`
	Unit       string             `json:"unit,omitempty"`
	Period     string             `json:"period,omitempty"`
	BasePeriod string             `json:"basePeriod,omitempty"`
	Scope      []string           `json:"scope,omitempty"`
	Algorithm  string             `json:"algorithm"`
	Inputs     []CalculationInput `json:"inputs,omitempty"`
	SourceRefs []string           `json:"sourceRefs"`
	Status     string             `json:"status"`
}

// AnalysisFocus supplies an applicable writing goal and candidate evidence.
// The agent selects relevant references; validation only checks their identity.
type AnalysisFocus struct {
	ID           string   `json:"id"`
	Goal         string   `json:"goal"`
	Required     bool     `json:"required"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type SectionAnalysisData struct {
	SectionID   string            `json:"sectionId"`
	Title       string            `json:"title"`
	Objective   AnalysisObjective `json:"objective"`
	Mode        NarrativeMode     `json:"mode"`
	SampleOnly  bool              `json:"sampleOnly,omitempty"`
	Metrics     []AnalysisMetric  `json:"metrics"`
	Focus       []AnalysisFocus   `json:"focus"`
	Constraints []string          `json:"constraints"`
	// Guidance supplies optional analysis directions, separate from evidence.
	Guidance        string                  `json:"guidance,omitempty"`
	RelatedSections []string                `json:"relatedSections,omitempty"`
	FindingIDs      []string                `json:"findingIds,omitempty"`
	References      []TrustedReference      `json:"references,omitempty"`
	Coverage        []AnalysisPartyCoverage `json:"coverage,omitempty"`
}

type AnalysisData struct {
	RuleVersion string                `json:"ruleVersion"`
	Sections    []SectionAnalysisData `json:"sections"`
	Actions     []AnalysisAction      `json:"actions,omitempty"`
}

func analysisObjective(section Section) AnalysisObjective {
	var objectives []AnalysisObjective
	// The embedded catalog is validated in tests; malformed data must not be
	// silently converted into a generic task.
	if err := json.Unmarshal(analysisObjectivesJSON, &objectives); err != nil {
		panic(err)
	}
	for _, objective := range objectives {
		if objective.SourceSectionID == section.ID {
			return objective
		}
	}
	return AnalysisObjective{SourceSectionID: section.ID, Goal: "只综合已证实的事实与风险，不新增分析范围。"}
}

// buildAnalysisData reuses the exact chart values and TaxCalculations consumed
// by the report. Tables remain source evidence, not rounded ratio inputs.
func buildAnalysisData(model Model) AnalysisData {
	data := AnalysisData{RuleVersion: RuleVersion, Actions: analysisActions(model)}
	for _, section := range model.Sections {
		if section.Level != 2 {
			continue
		}
		d := SectionAnalysisData{SectionID: section.Key, Title: section.BaseTitle, Objective: analysisObjective(section), Mode: section.Narrative.Mode,
			Guidance:   analysisGuidanceForSection(section),
			SampleOnly: section.SampleOnly, FindingIDs: section.FindingIDs, Metrics: []AnalysisMetric{}, Focus: []AnalysisFocus{},
			Constraints: []string{}}
		if section.DataNotice != "" {
			d.Constraints = append(d.Constraints, section.DataNotice)
		}
		if section.Narrative.MaxItems == 0 {
			d.Constraints = append(d.Constraints, "本节不填写分析文字，保留既定表图或固定短文。")
		}
		addSectionChartAnalysis(&d, section)
		for _, c := range model.Calculations {
			if c.Section != section.ID {
				continue
			}
			unit, value, algorithm := "元", c.Value, c.Algorithm
			// These existing calculations store fractions, whereas chart series
			// already store percentages. Explicit names avoid guessing the scale
			// from a value being below one.
			if stringSet([]string{"所得税贡献率", "研发加计扣除占收入比例", "买卖合同估算覆盖率"})[c.Name] {
				unit, value, algorithm = "%", c.Value*100, "("+c.Algorithm+") × 100；原TaxCalculation保存比值"
			}
			d.Metrics = append(d.Metrics, AnalysisMetric{ID: metricID(section.Key, "calculation."+c.ID), Name: c.Name, Kind: "calculation", Value: c.Value,
				Display: analysisNumber(value, unit), Unit: unit, Period: c.Period, Algorithm: algorithm, Inputs: c.Inputs, SourceRefs: []string{"report-model.json/calculations/" + c.ID}, Status: c.Status})
			d.Metrics[len(d.Metrics)-1].Value = value
		}
		for _, source := range model.AnalysisSources {
			if source.SectionID == section.Key {
				d.Metrics = append(d.Metrics, source.Metrics...)
				d.Focus = append(d.Focus, source.Focus...)
				d.Constraints = append(d.Constraints, source.Constraints...)
				d.Coverage = append(d.Coverage, source.Coverage...)
				for _, related := range source.RelatedSections {
					d.RelatedSections = appendUnique(d.RelatedSections, related)
				}
			}
		}
		addCoverageSummary(&d)
		addSectionAnalysis(&d, model, section)
		if section.SampleOnly {
			for i := range d.Focus {
				d.Focus[i].Required = false
			}
		}
		data.Sections = append(data.Sections, d)
	}
	addReadingCoverage(&data)
	shareAnalysisMetrics(&data)
	hydrateAnalysisInputs(&data, model)
	return data
}

func metricID(section, key string) string { return "fact:" + section + ".metric." + key }
func analysisToken(value string) string   { return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))[:12] }
func analysisNumber(value float64, unit string) string {
	if math.Abs(value) < 0.005 {
		value = 0
	}
	if unit == "期" || unit == "笔" || unit == "家" || unit == "条" || unit == "月" || unit == "张" || unit == "行" || unit == "组" || unit == "次" || unit == "项" || unit == "名" {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.2f", value)
}
func (d *SectionAnalysisData) number(key, name, kind, unit, period, base, algorithm string, value float64, refs ...string) string {
	id := metricID(d.SectionID, key)
	d.Metrics = append(d.Metrics, AnalysisMetric{ID: id, Name: name, Kind: kind, Unit: unit, Period: period, BasePeriod: base, Algorithm: algorithm, Value: value, Display: analysisNumber(value, unit), Status: "present", SourceRefs: refs})
	return id
}
func (d *SectionAnalysisData) text(key, name, kind, value string, refs ...string) string {
	id := metricID(d.SectionID, key)
	d.Metrics = append(d.Metrics, AnalysisMetric{ID: id, Name: name, Kind: kind, Display: value, Status: "present", Algorithm: "保留已核实的对象、范围或关系", SourceRefs: refs})
	return id
}
func (d *SectionAnalysisData) focus(key, goal string, refs ...string) {
	d.Focus = append(d.Focus, AnalysisFocus{ID: d.SectionID + "." + key, Goal: goal, Required: true, EvidenceRefs: refs})
}

func chartSeriesUnit(c Chart, s Series) string {
	if s.Axis == "right" && c.SecondaryUnit != "" {
		return c.SecondaryUnit
	}
	if c.Unit != "" {
		return c.Unit
	}
	if containsAny(s.Name, "比例", "率", "（%）") {
		return "%"
	}
	if containsAny(s.Name, "数量", "笔数") {
		return "笔"
	}
	return "元"
}
func analysisPointAvailable(c Chart, s Series, i int) bool {
	return i >= 0 && i < len(c.Labels) && i < len(s.Values) && (len(s.Available) == 0 || i < len(s.Available) && s.Available[i]) && !math.IsNaN(s.Values[i]) && !math.IsInf(s.Values[i], 0)
}

// adjacentAnalysisPeriods disallows annual/quarter mixing and gaps. It never
// equates "previous available record" with "previous comparable period".
func adjacentAnalysisPeriods(previous, current string) bool {
	a, b := periodBounds(previous)
	c, e := periodBounds(current)
	if a == "" || b == "" || c == "" || e == "" {
		return false
	}
	start, _ := time.Parse("2006-01-02", a)
	end, _ := time.Parse("2006-01-02", b)
	next, _ := time.Parse("2006-01-02", c)
	nextEnd, _ := time.Parse("2006-01-02", e)
	span := func(x, y time.Time) int { return (y.Year()-x.Year())*12 + int(y.Month()-x.Month()) + 1 }
	return end.AddDate(0, 0, 1).Equal(next) && span(start, end) == span(next, nextEnd)
}

func addChartAnalysis(d *SectionAnalysisData, c Chart, bi int) {
	coverageUnit := "期"
	for _, label := range c.Labels {
		start, end := periodBounds(label)
		if start == "" || end == "" {
			coverageUnit = "项"
			break
		}
	}
	for si, s := range c.Series {
		unit := chartSeriesUnit(c, s)
		prefix := c.ID + "." + analysisToken(s.Name)
		count, maximum, minimum := 0, -1, -1
		var observed, missing []string
		for i, label := range c.Labels {
			if !analysisPointAvailable(c, s, i) {
				missing = append(missing, label)
				continue
			}
			observed = append(observed, label)
			count++
			if maximum < 0 || s.Values[i] > s.Values[maximum] {
				maximum = i
			}
			if minimum < 0 || s.Values[i] < s.Values[minimum] {
				minimum = i
			}
			key := prefix + "." + analysisToken(label)
			ref := fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", d.SectionID, bi+1, si+1, i+1)
			labelRef := fmt.Sprintf("fact:%s.b%d.chart.label.p%d", d.SectionID, bi+1, i+1)
			d.number(key+".value", s.Name, "value", unit, label, "", "共享图表的未舍入数值", s.Values[i], ref)
			d.text(key+".period", s.Name+"期间/对象", "period", label, labelRef)
			if !analysisPointAvailable(c, s, i-1) || !adjacentAnalysisPeriods(c.Labels[i-1], label) {
				continue
			}
			prevRef := fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", d.SectionID, bi+1, si+1, i)
			delta := s.Values[i] - s.Values[i-1]
			deltaUnit := unit
			if unit == "%" {
				deltaUnit = "百分点"
			}
			d.number(key+".change", s.Name+"较上期变化", "change", deltaUnit, label, c.Labels[i-1], "本期值−上期值", delta, ref, prevRef,
				metricID(d.SectionID, key+".value"), metricID(d.SectionID, prefix+"."+analysisToken(c.Labels[i-1])+".value"))
			if unit != "%" && s.Values[i-1] > invoiceAggregationTolerance {
				d.number(key+".growth", s.Name+"环比增幅", "growth", "%", label, c.Labels[i-1], "(本期值−上期值)/上期值×100；基期必须为正", delta/s.Values[i-1]*100, ref, prevRef)
			}
			addSeriesHistoryAnalysis(d, c, s, bi, si, i, key, unit)
		}
		if count == 0 {
			continue
		}
		for _, x := range []struct {
			name string
			i    int
		}{{"maximum", maximum}, {"minimum", minimum}} {
			id := d.number(prefix+"."+x.name, s.Name+map[string]string{"maximum": "最大值", "minimum": "最小值"}[x.name], "extremum", unit, c.Labels[x.i], "", "在本序列全部有效数据中比较，缺失期间不参与", s.Values[x.i])
			m := &d.Metrics[len(d.Metrics)-1]
			for j := range c.Labels {
				if analysisPointAvailable(c, s, j) {
					m.Scope = append(m.Scope, c.Labels[j])
					m.SourceRefs = append(m.SourceRefs, fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", d.SectionID, bi+1, si+1, j+1))
				}
			}
			d.text(prefix+"."+x.name+".period", s.Name+"极值期间/对象", "period", c.Labels[x.i], id)
		}
		d.number(prefix+".coverage", s.Name+"有效观察数", "coverage", coverageUnit, "", "", "仅统计明确存在的数据点；时间轴按期、分类轴按项，不将缺失填零", float64(count))
		scope := "有效比较范围：" + strings.Join(observed, "、")
		if len(missing) > 0 {
			scope += "；缺失：" + strings.Join(missing, "、") + "；极值仅限已取得的比较范围"
		}
		d.text(prefix+".coverage-scope", s.Name+"极值比较范围", "coverage", scope)
		d.text(prefix+".range", s.Name+"完整范围极值", "range",
			fmt.Sprintf("范围：%s；最低%s%s（%s），最高%s%s（%s）；仅适用于该完整集合，不能套用于排除部分期间后的其余集合。", strings.Join(observed, "、"), analysisNumber(s.Values[minimum], unit), unit, c.Labels[minimum], analysisNumber(s.Values[maximum], unit), unit, c.Labels[maximum]),
			metricID(d.SectionID, prefix+".minimum"), metricID(d.SectionID, prefix+".maximum"))
		d.Metrics[len(d.Metrics)-1].Scope = append([]string(nil), observed...)
	}
}

func findAnalysisSeries(c Chart, name string) (Series, bool) {
	for _, series := range c.Series {
		if series.Name == name {
			return series, true
		}
	}
	return Series{}, false
}

// hydrateAnalysisInputs records unrounded source values beside each formula.
// Source refs still lead to the shared report or its TaxFieldFact paths.
func hydrateAnalysisInputs(data *AnalysisData, model Model) {
	inputs := map[string]CalculationInput{}
	for _, r := range baseTrustedReferences(model) {
		if v, ok := parseFormNumber(r.Value); ok {
			inputs[r.ID] = CalculationInput{Source: r.ID, Label: r.Label, Value: v, Present: true}
		}
	}
	for _, s := range model.Sections {
		for bi, b := range s.Blocks {
			if b.Chart == nil {
				continue
			}
			for si, series := range b.Chart.Series {
				for i := range b.Chart.Labels {
					if analysisPointAvailable(*b.Chart, series, i) {
						id := fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", s.Key, bi+1, si+1, i+1)
						inputs[id] = CalculationInput{Source: id, Label: series.Name + " · " + b.Chart.Labels[i], Value: series.Values[i], Present: true}
					}
				}
			}
		}
	}
	for _, s := range data.Sections {
		for _, m := range s.Metrics {
			if m.Unit != "" {
				inputs[m.ID] = CalculationInput{Source: m.ID, Label: m.Name, Value: m.Value, Present: m.Status == "present"}
			}
		}
	}
	for si := range data.Sections {
		for mi := range data.Sections[si].Metrics {
			m := &data.Sections[si].Metrics[mi]
			if len(m.Inputs) > 0 {
				continue
			}
			for _, ref := range m.SourceRefs {
				if input, ok := inputs[ref]; ok {
					m.Inputs = append(m.Inputs, input)
				}
			}
		}
	}
}

func addSeriesHistoryAnalysis(d *SectionAnalysisData, c Chart, s Series, bi, si, i int, key, unit string) {
	// Cumulative R&D series is not a sequence of single-period expenses.
	if d.SectionID == "cit.research-expense" {
		return
	}
	streak := 0
	direction := 0.0
	for j := i; j > 0; j-- {
		if !analysisPointAvailable(c, s, j) || !analysisPointAvailable(c, s, j-1) || !adjacentAnalysisPeriods(c.Labels[j-1], c.Labels[j]) {
			break
		}
		delta := s.Values[j] - s.Values[j-1]
		if math.Abs(delta) < 0.0000001 {
			break
		}
		if direction == 0 {
			direction = delta
		}
		if delta*direction <= 0 {
			break
		}
		streak++
	}
	if streak > 0 {
		refs := []string{}
		for j := i - streak; j <= i; j++ {
			refs = append(refs, fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", d.SectionID, bi+1, si+1, j+1))
		}
		name := "连续上升比较期数"
		if direction < 0 {
			name = "连续下降比较期数"
		}
		d.number(key+".streak", s.Name+name, "streak", "期", c.Labels[i], c.Labels[i-streak], "连续同方向的相邻期比较次数；缺值、间隔及反向打断", float64(streak), refs...)
		deltaUnit := unit
		if unit == "%" {
			deltaUnit = "百分点"
		}
		d.number(key+".streak-change", s.Name+"连续同向区间净变化", "difference", deltaUnit, c.Labels[i], c.Labels[i-streak], "区间末值−区间起值；不累加已舍入的单期变化", s.Values[i]-s.Values[i-streak], refs...)
		d.number(key+".streak-observations", s.Name+"连续同向区间观察点数", "count", "期", c.Labels[i], c.Labels[i-streak], "相邻比较次数+1；起点计入观察点", float64(streak+1), refs...)
	}
	if i < analysisBaselinePeriods {
		return
	}
	var values []float64
	var refs []string
	for j := i - analysisBaselinePeriods; j < i; j++ {
		if !analysisPointAvailable(c, s, j) || !adjacentAnalysisPeriods(c.Labels[j], c.Labels[j+1]) {
			return
		}
		values = append(values, s.Values[j])
		refs = append(refs, fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", d.SectionID, bi+1, si+1, j+1))
	}
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	basePeriod := strings.Join(c.Labels[i-analysisBaselinePeriods:i], "、")
	base := d.number(key+".baseline-mean", s.Name+"前3个连续可比期均值", "baseline", unit, c.Labels[i], basePeriod, "前3个连续同周期可用数值算术平均", mean, refs...)
	d.number(key+".baseline-median", s.Name+"前3个连续可比期中位数", "baseline", unit, c.Labels[i], basePeriod, "前3个连续同周期可用数值中位数", medianAnalysis(values), refs...)
	current := fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", d.SectionID, bi+1, si+1, i+1)
	if unit == "%" {
		d.number(key+".baseline-deviation", s.Name+"较前3期均值偏离", "deviation", "百分点", c.Labels[i], basePeriod, "本期比率−前3期比率算术平均", s.Values[i]-mean, current, base)
	} else if mean > invoiceAggregationTolerance {
		d.number(key+".baseline-deviation", s.Name+"较前3期均值偏离", "deviation", "%", c.Labels[i], basePeriod, "(本期值−前3期均值)/前3期均值×100", (s.Values[i]-mean)/mean*100, current, base)
	}
}

func sectionByKey(model Model, key string) Section {
	for _, s := range model.Sections {
		if s.Key == key {
			return s
		}
	}
	return Section{}
}
func firstAnalysisChart(s Section) *Chart {
	var charts []*Chart
	for _, b := range s.Blocks {
		if b.Chart != nil {
			charts = append(charts, b.Chart)
		}
	}
	if len(charts) == 0 {
		return nil
	}
	if len(charts) == 1 {
		return charts[0]
	}
	for _, c := range charts {
		for _, p := range c.Labels {
			if start, _ := periodBounds(p); start == "" {
				return charts[0]
			}
		}
	}
	combined := *charts[0]
	combined.ID = "combined." + s.Key
	combined.Labels = nil
	combined.Series = nil
	labels, names := map[string]bool{}, map[string]bool{}
	var seriesOrder []string
	for _, c := range charts {
		if c.Unit != charts[0].Unit || c.SecondaryUnit != charts[0].SecondaryUnit {
			return charts[0]
		}
		for _, p := range c.Labels {
			labels[p] = true
		}
		for _, series := range c.Series {
			if !names[series.Name] {
				seriesOrder = append(seriesOrder, series.Name)
			}
			names[series.Name] = true
		}
	}
	combined.Labels = sortedMapKeys(labels)
	sort.SliceStable(combined.Labels, func(i, j int) bool {
		a, b := periodBounds(combined.Labels[i])
		c, e := periodBounds(combined.Labels[j])
		if a != c {
			return a < c
		}
		return b < e
	})
	for _, name := range seriesOrder {
		series := Series{Name: name, Values: make([]float64, len(combined.Labels)), Available: make([]bool, len(combined.Labels))}
		conflict := make([]bool, len(combined.Labels))
		for i, p := range combined.Labels {
			for _, c := range charts {
				for _, source := range c.Series {
					if source.Name != name {
						continue
					}
					series.Axis = source.Axis
					for j, q := range c.Labels {
						if p != q || !analysisPointAvailable(*c, source, j) {
							continue
						}
						if series.Available[i] && math.Abs(series.Values[i]-source.Values[j]) >= invoiceAggregationTolerance {
							conflict[i] = true
						}
						series.Values[i] = source.Values[j]
						series.Available[i] = true
					}
				}
			}
			if conflict[i] {
				series.Available[i] = false
			}
		}
		combined.Series = append(combined.Series, series)
	}
	return &combined
}

// Multi-year reports draw separate annual charts, but analytical comparisons
// keep one full range, including the prior December for January baselines.
func addSectionChartAnalysis(d *SectionAnalysisData, s Section) {
	if s.Key == "enterprise.changes" {
		return
	}
	c := firstAnalysisChart(s)
	if c != nil && strings.HasPrefix(c.ID, "combined.") {
		start := len(d.Metrics)
		addChartAnalysis(d, *c, 0)
		mapping := map[string]string{}
		for si, series := range c.Series {
			for i, p := range c.Labels {
				mapping[fmt.Sprintf("fact:%s.b1.chart.s%d.p%d", s.Key, si+1, i+1)] = analysisChartRef(s, *c, series, i)
				for bi, b := range s.Blocks {
					if b.Chart == nil {
						continue
					}
					for j, q := range b.Chart.Labels {
						if p == q {
							mapping[fmt.Sprintf("fact:%s.b1.chart.label.p%d", s.Key, i+1)] = fmt.Sprintf("fact:%s.b%d.chart.label.p%d", s.Key, bi+1, j+1)
						}
					}
				}
			}
		}
		for i := start; i < len(d.Metrics); i++ {
			for j, ref := range d.Metrics[i].SourceRefs {
				if actual, ok := mapping[ref]; ok {
					d.Metrics[i].SourceRefs[j] = actual
				}
			}
		}
		return
	}
	for bi, b := range s.Blocks {
		if b.Chart != nil && b.Chart.Kind != "timeline" {
			addChartAnalysis(d, *b.Chart, bi)
		}
	}
}
func medianAnalysis(values []float64) float64 {
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	if len(v)%2 == 1 {
		return v[len(v)/2]
	}
	return (v[len(v)/2-1] + v[len(v)/2]) / 2
}

// analysisDataReferences are semantic, immutable references in the existing
// source namespace. Metric meaning does not depend on table row indexes.
func analysisDataReferences(data AnalysisData) []TrustedReference {
	var refs []TrustedReference
	for _, s := range data.Sections {
		for _, m := range s.Metrics {
			if m.Status == "present" {
				refs = append(refs, TrustedReference{ID: m.ID, Kind: "metric", Label: m.Name + "（" + m.Unit + "）", Value: m.Display, SectionID: s.SectionID})
			}
		}
	}
	return refs
}

func applyAnalysisAttention(model Model) Model {
	data := buildAnalysisData(model)
	for i := range model.Sections {
		s := &model.Sections[i]
		if s.Narrative.Mode != NarrativeFact || s.Narrative.MaxItems == 0 || s.SampleOnly {
			continue
		}
		for _, d := range data.Sections {
			if d.SectionID == s.Key {
				for _, f := range d.Focus {
					if f.Required && !strings.HasSuffix(f.ID, ".comparison-scope") {
						s.Narrative = NarrativePolicy{Mode: NarrativeAttention, MinItems: 1, MaxItems: 3}
						break
					}
				}
			}
		}
	}
	return model
}
