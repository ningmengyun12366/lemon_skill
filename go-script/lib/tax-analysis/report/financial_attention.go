package report

import (
	"fmt"
	"sort"
)

// analysisFinancialAttention computes same-cycle beginning/end changes and cash
// direction facts for Word 8.3-8.6. Annual cash flow is not a quarterly substitute.
func analysisFinancialAttention(ctx *reportContext) []SectionAnalysisData {
	sections := map[string]*SectionAnalysisData{}
	for _, key := range []string{"financial.debt-asset", "financial.working-capital", "financial.cash-flow", "financial.summary"} {
		sections[key] = &SectionAnalysisData{SectionID: key}
	}
	groups := map[string][]financialFact{}
	for _, f := range ctx.financial {
		key := f.TaxPeriod.Cycle
		if len(f.TaxPeriod.End) >= 4 {
			key += "|" + f.TaxPeriod.End[:4]
		}
		groups[key] = append(groups[key], f)
	}
	for _, cycle := range sortedMapKeys(groups) {
		facts := groups[cycle]
		sort.SliceStable(facts, func(i, j int) bool { return facts[i].TaxPeriod.End < facts[j].TaxPeriod.End })
		type metric struct {
			key, name, section, unit string
			value                    func(financialFact) (float64, bool)
		}
		metrics := []metric{
			{"leverage", "资产负债率", "financial.debt-asset", "%", func(f financialFact) (float64, bool) {
				if !f.HasAssets || !f.HasLiabilities || f.Assets == 0 {
					return 0, false
				}
				return f.Liabilities / f.Assets * 100, true
			}},
			{"receivable", "应收账款", "financial.working-capital", "元", func(f financialFact) (float64, bool) { return f.AccountsReceivable, f.HasAccountsReceivable }},
			{"inventory", "存货", "financial.working-capital", "元", func(f financialFact) (float64, bool) { return f.Inventory, f.HasInventory }},
			{"contract", "合同负债", "financial.working-capital", "元", func(f financialFact) (float64, bool) { return f.ContractLiabilities, f.HasContractLiabilities }},
			{"cash", "经营活动现金流量净额", "financial.cash-flow", "元", func(f financialFact) (float64, bool) { return f.OperatingCashFlow, f.HasOperatingCashFlow }},
		}
		for _, m := range metrics {
			for _, key := range []string{m.section, "financial.summary"} {
				d := sections[key]
				var refs, periods, missing []string
				var values []float64
				positive, negative, zero := 0, 0, 0
				for _, f := range facts {
					value, ok := m.value(f)
					if !ok {
						missing = appendUnique(missing, f.Period)
						continue
					}
					source := "report-model.json/taxFields[period=" + f.Period + "]"
					id := d.number("financial."+m.key+"."+analysisToken(cycle+f.Period), m.name, "amount", m.unit, f.Period, "", "同一所属期财报金额；企业准则现金流累计还原当期，余额不差分；年报与季报/月报分开", value, source)
					refs = append(refs, id)
					periods = append(periods, f.Period)
					values = append(values, value)
					if value > 0 {
						positive++
					} else if value < 0 {
						negative++
					} else {
						zero++
					}
				}
				if len(values) == 0 {
					continue
				}
				prefix := "financial." + m.key + "." + analysisToken(cycle)
				if len(values) > 1 {
					unit := "元"
					if m.unit == "%" {
						unit = "个百分点"
					}
					change := d.number(prefix+".change", m.name+"期末值区间变化", "difference", unit, periods[len(periods)-1], periods[0], "同一申报周期最后可得值减最早可得值", values[len(values)-1]-values[0], refs[0], refs[len(refs)-1])
					refs = append(refs, change)
				}
				if m.key == "cash" {
					refs = append(refs, d.text(prefix+".direction", "现金流正负期间数", "state", fmt.Sprintf("%s：正值%d期、负值%d期、零值%d期", cycle, positive, negative, zero), refs...))
				}
				if len(missing) > 0 {
					d.text(prefix+".missing", "未取得该指标的财报期间", "coverage", joinEvidenceFragments(missing...), "report-model.json/taxFields")
				}
				d.focus(prefix, "说明无冲突的起止期间期末值、变化金额或百分点和限制；最早可得期末值不是年初余额，不能将该区间变化称为全年较年初变化。解释资金占用及现金流结构，不把负债率超过100%单独判为税务高风险。", refs...)
			}
		}
	}
	for _, key := range sortedMapKeys(sections) {
		d := sections[key]
		d.Constraints = []string{"年报与季报/月报分别计算；年度经营现金流不能代替缺失第四季度数据，不将季度金额再当月度金额。", "负债率高于100%只说明账面结构；税务高风险须另有合规证据。应收、存货变化不直接证明无法收回或积压。", "关注状态只解释已显示的变化及限制，不追加整改；未有证据时不编造原因、行业基准、减负目标或未来经营影响。"}
	}
	addFinancialOpeningAnalysis(sections, ctx.taxFields)
	addFinancialNormalizationFacts(sections["financial.cash-flow"], ctx.financial, financialMetricOperatingCF)
	var result []SectionAnalysisData
	for _, key := range sortedMapKeys(sections) {
		result = append(result, *sections[key])
	}
	return result
}
