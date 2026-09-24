package report

import (
	"fmt"
	"sort"
	"strings"
)

// The reading brief reuses frozen metrics and their original reference IDs.
// It does not derive new numbers or treat diagnostic values as usable facts.
var readingFactFamilies = []struct{ prefix, title string }{
	{"industry-invoice.", "销项发票分类及其分母"},
	{"industry-financial.", "财报收入：独立来源与期间"},
	{"late-fee-payment.", "历史滞纳金缴款及当前状态边界"},
	{"annual-flow.", "分期合计与独立年报"},
	{"financial-opening.", "同版报表年初与期末"},
	{"balance-conflict.", "不同报表版本的余额差异"},
	{"cash-profit.", "同期间现金净额、净利润及可比状态"},
	{"profit-change.", "相邻期间利润变化及各项目金额贡献"},
	{"balance-structure.", "资产、负债及构成"},
	{"trend-direction.", "完整观察范围与局部连续变化"},
	{"sensitive-total.", "本次查询期间的敏感服务购进总额"},
	{"sensitive-category.", "与图表同源的敏感服务购进分类"},
}

// readingKeyFacts keeps all members of each selected comparison together,
// including missing periods. Annual reconciliation and cash/profit pairs are
// never reduced to only the largest value or the last period.
func readingKeyFacts(s SectionAnalysisData) (string, map[string]bool) {
	selection := readingMetricSelection(s)
	shown := map[string]bool{}
	var b strings.Builder
	for _, family := range readingFactFamilies {
		groups := map[string][]AnalysisMetric{}
		var order []string
		for _, m := range s.Metrics {
			if !strings.HasPrefix(m.ID, metricID(s.SectionID, family.prefix)) {
				continue
			}
			key := readingGroupID(m.ID)
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], m)
		}
		started := false
		algorithms := map[string]bool{}
		var algorithmOrder []string
		for _, key := range order {
			group := groups[key]
			selected := selection == nil || family.prefix == "annual-flow." || family.prefix == "cash-profit." || family.prefix == "financial-opening." || family.prefix == "balance-conflict."
			for _, m := range group {
				selected = selected || selection[m.ID] || m.Status != "present"
			}
			if !selected {
				continue
			}
			if !started {
				fmt.Fprintf(&b, "\n### %s\n", family.title)
				started = true
			}
			b.WriteString("\n")
			for _, m := range group {
				shown[m.ID] = true
				if m.Algorithm != "" && !algorithms[m.Algorithm] {
					algorithms[m.Algorithm] = true
					algorithmOrder = append(algorithmOrder, m.Algorithm)
				}
				value := m.Display
				if m.Status != "present" {
					value = "未取得可用值（" + m.Status + "），不补零，不用于比较"
				}
				fmt.Fprintf(&b, "- [%s] %s：%s", m.ID, m.Name, value)
				if m.Status == "present" && m.Unit != "" {
					fmt.Fprintf(&b, " %s", m.Unit)
				}
				if m.Period != "" {
					fmt.Fprintf(&b, "；期间=%s", m.Period)
				}
				if m.BasePeriod != "" {
					fmt.Fprintf(&b, "；基期=%s", m.BasePeriod)
				}
				if len(m.Scope) > 0 {
					fmt.Fprintf(&b, "；范围=%s", strings.Join(m.Scope, "、"))
				}
				b.WriteByte('\n')
			}
		}
		for _, algorithm := range algorithmOrder {
			fmt.Fprintf(&b, "口径：%s\n", algorithm)
		}
	}
	return b.String(), shown
}

func readingTableCell(value string) string {
	return strings.NewReplacer("|", "\\|", "\r", "", "\n", " ").Replace(value)
}

// Credit coverage is intentionally complete for the section's scoped parties.
// Prioritizing recorded/incomplete queries never drops the remaining subjects
// or turns an unavailable count into zero.
func readingCoverageMarkdown(s SectionAnalysisData) string {
	if len(s.Coverage) == 0 {
		return ""
	}
	parties := append([]AnalysisPartyCoverage(nil), s.Coverage...)
	ranks := map[string]string{}
	metrics := map[string]AnalysisMetric{}
	for _, m := range s.Metrics {
		metrics[m.ID] = m
	}
	for _, m := range s.Metrics {
		if strings.HasPrefix(m.ID, metricID(s.SectionID, "credit-trade.")) && strings.HasSuffix(m.ID, ".name") && m.Status == "present" {
			key := readingGroupID(m.ID)
			rank, share := metrics[key+".rank"], metrics[key+".share"]
			if rank.Status == "present" && share.Status == "present" {
				ranks[m.Display] = fmt.Sprintf("第%s名 / %s%% [%s] [%s]", rank.Display, share.Display, rank.ID, share.ID)
			}
		}
	}
	priority := func(p AnalysisPartyCoverage) int {
		for _, q := range p.Queries {
			if q.RecordCount != nil && *q.RecordCount > 0 || q.TotalCount != nil && *q.TotalCount > 0 {
				return 0
			}
		}
		if p.IdentityStatus != "verified" {
			return 1
		}
		for _, q := range p.Queries {
			if q.Outcome != "empty" && q.Outcome != "records" {
				return 1
			}
		}
		return 2
	}
	sort.SliceStable(parties, func(i, j int) bool { return priority(parties[i]) < priority(parties[j]) })
	var b strings.Builder
	b.WriteString("\n### 主体核验范围（全部本节主体）\n记录条数与当前处理状态分开。正向记录和未取得结果均保留；下表不是当前未结案数量。\n\n| 主体 | 同方向交易排名/占比 | 身份核验 | 逐项查询 | 依据 |\n| --- | --- | --- | --- | --- |\n")
	for _, p := range parties {
		var queries []string
		for _, q := range p.Queries {
			if q.Outcome == "returned" {
				// Registry detail is an object, not a paginated event list.
				queries = append(queries, q.Name+"："+q.Status)
				continue
			}
			count := "返回明细未取得"
			if q.RecordCount != nil {
				count = fmt.Sprintf("返回%d条", *q.RecordCount)
			}
			if q.TotalCount != nil {
				count += fmt.Sprintf("/总数%d条", *q.TotalCount)
			}
			queries = append(queries, q.Name+"："+count+"（"+q.Status+"，"+q.Outcome+"）")
		}
		rank := ranks[p.Name]
		if rank == "" {
			rank = "未取得本节排名指标"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", readingTableCell(p.Name), readingTableCell(rank), readingTableCell(string(p.IdentityStatus)+"："+p.Identity), readingTableCell(strings.Join(queries, "；")), readingTableCell(strings.Join(p.EvidenceRefs, "、")))
	}
	return b.String()
}
