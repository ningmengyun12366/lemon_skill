package report

import (
	"fmt"
	"strings"
)

// conclusionReading starts with every confirmed finding and its original action.
// Previous AI prose is not evidence. Full chapter facts remain in report-refs;
// this projection neither recalculates numbers nor truncates by character count.
func conclusionReading(w analysisWorkspace) string {
	var b strings.Builder
	refs := baseTrustedReferences(w.Model)
	b.WriteString("# 综合结论材料\n\n## 已有风险与处理方向\n")
	for _, r := range refs {
		if r.ID == "fact:report.overallRiskLevel" || r.ID == "fact:report.riskCount" || r.ID == "fact:report.startDate" || r.ID == "fact:report.endDate" {
			writeReadingReference(&b, r)
		}
	}
	for _, f := range sortedFindings(w.Model.Findings) {
		b.WriteByte('\n')
		for _, r := range refs {
			if strings.HasPrefix(r.ID, "finding:"+f.ID+".") {
				writeReadingReference(&b, r)
			}
		}
	}
	if len(w.Model.Findings) == 0 {
		b.WriteString("本次没有已确认风险事项；结论范围仍受下列资料限制。\n")
	}

	b.WriteString("\n## 数据限制\n")
	notices := map[string]bool{}
	for _, r := range refs {
		if strings.Contains(r.ID, ".dataNotice") && !notices[r.Value] {
			writeReadingReference(&b, r)
			notices[r.Value] = true
		}
	}
	seen := map[string]bool{}
	for _, s := range w.Data.Sections {
		for _, m := range s.Metrics {
			if m.Status != "present" && !seen[conclusionMetricIdentity(m)] {
				writeReadingMetric(&b, m)
				seen[conclusionMetricIdentity(m)] = true
			}
		}
	}
	for _, s := range w.Data.Sections {
		if s.SectionID == "invoice.customer-credit" || s.SectionID == "invoice.supplier-credit" {
			b.WriteString(readingCoverageMarkdown(s))
		}
	}

	b.WriteString("\n## 支撑结论的关键事实\n")
	for _, r := range refs {
		if r.ID == "fact:report.normalAreas" {
			writeReadingReference(&b, r)
		}
	}
	for _, s := range w.Data.Sections {
		selected := conclusionMetricSelection(s)
		started := false
		algorithms := map[string]bool{}
		for _, m := range s.Metrics {
			identity := conclusionMetricIdentity(m)
			if !selected[m.ID] || seen[identity] {
				continue
			}
			if !started {
				fmt.Fprintf(&b, "\n### %s\n", textOr(s.Title, s.SectionID))
				started = true
			}
			writeReadingMetric(&b, m)
			seen[identity] = true
			if m.Algorithm != "" && !algorithms[m.Algorithm] {
				fmt.Fprintf(&b, "口径：%s\n", m.Algorithm)
				algorithms[m.Algorithm] = true
			}
		}
	}
	b.WriteString("\n## 写作要求\n")
	b.WriteString(sectionAnalysisGuidance["chapter.conclusion"])
	b.WriteString("\n完整章节、比较明细和引用可通过 report-refs 查询；未展示不等于缺失。\n")
	return b.String()
}

// Shared metrics retain the original reference as their first source. Collapse
// only that explicit alias, never merely equal values from different sources.
func conclusionMetricIdentity(m AnalysisMetric) string {
	id := m.ID
	_, suffix, ok := strings.Cut(m.ID, ".metric.")
	if ok && len(m.SourceRefs) > 0 {
		if _, original, found := strings.Cut(m.SourceRefs[0], ".metric."); found && original == suffix && m.SourceRefs[0] != m.ID {
			id = m.SourceRefs[0]
		}
	}
	// Even an explicit alias cannot hide a changed status, period or value.
	return strings.Join([]string{id, m.Name, m.Status, m.Display, m.Unit, m.Period, m.BasePeriod, strings.Join(m.Scope, ","), m.Algorithm}, "\x00")
}

func conclusionMetricSelection(s SectionAnalysisData) map[string]bool {
	selected := map[string]bool{}
	if s.SectionID == "financial.summary" || s.SectionID == "invoice.sensitive-services" || s.SectionID == "tax-payment.exceptions" {
		_, selected = readingKeyFacts(s)
	}
	selection := readingMetricSelection(s)
	riskRefs := map[string]bool{}
	if len(s.FindingIDs) > 0 {
		for _, focus := range s.Focus {
			for _, ref := range focus.EvidenceRefs {
				riskRefs[ref] = true
			}
		}
	}
	groups := map[string]bool{}
	for _, m := range s.Metrics {
		key := strings.TrimPrefix(m.ID, metricID(s.SectionID, ""))
		wanted := riskRefs[m.ID] ||
			s.SectionID == "vat.period-overview" && strings.HasPrefix(key, "vat-summary.") && (strings.HasSuffix(key, ".total") || strings.HasSuffix(key, ".coverage")) ||
			s.SectionID == "financial.period-expense" && strings.HasPrefix(key, "profit-amounts.") && strings.HasSuffix(key, ".change") && selection[m.ID] ||
			s.SectionID == "cit.deduction-evidence" && (strings.HasPrefix(key, "deduction-known.") || strings.HasPrefix(key, "deduction-gap.") && (strings.HasSuffix(key, ".difference") || strings.HasSuffix(key, ".basis")))
		if wanted || selected[m.ID] {
			groups[readingGroupID(m.ID)] = true
		}
	}
	for _, m := range s.Metrics {
		if groups[readingGroupID(m.ID)] {
			selected[m.ID] = true
		}
	}
	return selected
}

func writeReadingReference(b *strings.Builder, r TrustedReference) {
	fmt.Fprintf(b, "[%s] %s = %s\n", r.ID, r.Label, r.Value)
}

func writeReadingMetric(b *strings.Builder, m AnalysisMetric) {
	value := m.Display
	if m.Status != "present" {
		value = "未取得可用值（" + m.Status + "），不补零，不用于比较"
	} else if m.Unit != "" {
		value += " " + m.Unit
	}
	fmt.Fprintf(b, "[%s] %s = %s", m.ID, m.Name, value)
	if m.Period != "" {
		fmt.Fprintf(b, "；期间=%s", m.Period)
	}
	if m.BasePeriod != "" {
		fmt.Fprintf(b, "；基期=%s", m.BasePeriod)
	}
	if len(m.Scope) > 0 {
		fmt.Fprintf(b, "；范围=%s", strings.Join(m.Scope, "、"))
	}
	b.WriteByte('\n')
}
