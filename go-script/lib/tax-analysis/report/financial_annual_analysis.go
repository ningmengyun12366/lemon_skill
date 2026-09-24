package report

import (
	"math"
	"sort"
	"strings"
	"time"
)

// analysisFinancialAnnualFlows compares complete, normalized current periods
// with the independently filed annual statement. It never replaces either one.
func analysisFinancialAnnualFlows(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "financial.statements"}
	for _, annual := range ctx.financial {
		if annual.TaxPeriod.Cycle != taxCycleAnnual {
			continue
		}
		for _, cycle := range []string{taxCycleMonthly, taxCycleQuarterly} {
			var parts []financialFact
			for _, f := range ctx.financial {
				if f.TaxPeriod.Cycle == cycle && f.TaxPeriod.Start >= annual.TaxPeriod.Start && f.TaxPeriod.End <= annual.TaxPeriod.End {
					parts = append(parts, f)
				}
			}
			if len(parts) == 0 {
				continue
			}
			sort.SliceStable(parts, func(i, j int) bool { return parts[i].TaxPeriod.Start < parts[j].TaxPeriod.Start })
			complete := completeFinancialYear(parts, annual)
			key := "annual-flow." + analysisToken(annual.TaxPeriod.Start+"|"+annual.SourceRecordID+"|"+cycle)
			var scope []string
			for _, f := range parts {
				scope = append(scope, f.Period)
			}
			coverage := "全年同周期期间完整"
			if !complete {
				coverage = "未覆盖全年连续同周期期间，不计算全年分期合计与年报差额"
			}
			d.text(key+".coverage", "分期与年度勾稽覆盖范围", "coverage", coverage+"；已取得："+strings.Join(scope, "、"))
			for _, metric := range []struct {
				key, name string
				value     func(financialFact) (float64, bool)
			}{
				{"revenue", "营业收入", func(f financialFact) (float64, bool) { return f.Revenue, f.HasRevenue }},
				{"cost", "营业成本", func(f financialFact) (float64, bool) { return f.Cost, f.HasCost }},
				{"profit", "利润总额", func(f financialFact) (float64, bool) { return f.Profit, f.HasProfit }},
				{"net-profit", "净利润", func(f financialFact) (float64, bool) { return f.NetProfit, f.HasNetProfit }},
				{"expenses", "期间费用", financialExpenseTotal},
				{"operating-cashflow", "经营活动现金流量净额", func(f financialFact) (float64, bool) { return f.OperatingCashFlow, f.HasOperatingCashFlow }},
			} {
				annualValue, known := metric.value(annual)
				known = known && complete
				total := 0.0
				var inputs []CalculationInput
				for _, f := range parts {
					amount, ok := metric.value(f)
					known = known && ok
					total += amount
					inputs = append(inputs, financialFlowInput(f, metric.name, amount, ok))
				}
				prefix := key + "." + metric.key
				if !known {
					d.text(prefix+".coverage", metric.name+"分期年度勾稽状态", "coverage", "期间或该指标不完整，不计算分期全年合计及年度差额", metricID(d.SectionID, key+".coverage"))
					continue
				}
				var refs []string
				for _, input := range inputs {
					refs = append(refs, input.Source)
				}
				sumID := d.number(prefix+".sum", metric.name+"全年分期合计", "total", "元", annual.Period, "", "完整全年同周期当期金额相加；企业准则已还原当期，月季分别勾稽，不加入年报", total, refs...)
				d.Metrics[len(d.Metrics)-1].Inputs = inputs
				d.Metrics[len(d.Metrics)-1].Scope = scope
				annualInput := financialFlowInput(annual, metric.name, annualValue, true)
				annualID := d.number(prefix+".annual", metric.name+"独立年度申报金额", "amount", "元", annual.Period, "", "年度财报同名流量指标，保留原申报值，不替换分期合计", annualValue, annualInput.Source)
				d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{annualInput}
				difference := math.Round((total-annualValue)*100) / 100
				diffID := d.number(prefix+".difference", metric.name+"分期合计减年报差额", "difference", "元", annual.Period, "", "完整全年分期合计−独立年度申报金额；差额不自动构成税务风险", difference, sumID, annualID)
				if difference != 0 {
					d.focus(prefix, "说明全年分期合计与独立年报存在的金额差异，分别保留两种来源，不自行用年报覆盖季报/月报，也不把差额当作漏报或少缴。未取得更正或调节依据时原因待核实。", sumID, annualID, diffID)
				}
			}
		}
	}
	return d
}

func financialFlowInput(f financialFact, name string, value float64, present bool) CalculationInput {
	return CalculationInput{Source: "income_statements/records[id=" + f.SourceRecordID + "]/period=" + f.Period + "/" + name, Label: f.Period + " " + name, Value: value, Present: present}
}

func completeFinancialYear(parts []financialFact, annual financialFact) bool {
	start, err := time.Parse("2006-01-02", annual.TaxPeriod.Start)
	end, endErr := time.Parse("2006-01-02", annual.TaxPeriod.End)
	if err != nil || endErr != nil || start.Month() != time.January || start.Day() != 1 || !start.AddDate(1, 0, -1).Equal(end) {
		return false
	}
	span := 1
	if len(parts) > 0 && parts[0].TaxPeriod.Cycle == taxCycleQuarterly {
		span = 3
	}
	if len(parts) != 12/span {
		return false
	}
	for _, f := range parts {
		if f.TaxPeriod.Start != start.Format("2006-01-02") || f.TaxPeriod.End != start.AddDate(0, span, -1).Format("2006-01-02") {
			return false
		}
		start = start.AddDate(0, span, 0)
	}
	return true
}
