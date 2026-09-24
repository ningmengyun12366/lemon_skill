package report

import (
	"fmt"
	"sort"
	"strings"
)

const (
	vatContinuousBurdenRule = "VAT-CONTINUOUS-LOW-BURDEN-001"
	vatBurdenHistoryMinimum = 3
	vatBurdenHistoryMaximum = 6
	vatBurdenDrop           = 0.30
	vatBurdenLongWindow     = 6
)

type vatLowWindow struct {
	Facts, Baseline     []vatFact
	BaseRate, BaseSales float64
	CreditChange        float64
	HasCreditChange     bool
}

func comparableVATMonth(f vatFact) bool {
	return len(deliveryMonths(f.TaxPeriod.Start, f.TaxPeriod.End)) == 1 && f.HasSales && f.Sales > 0 && f.HasSupplementTax && f.SupplementTax >= 0
}

func medianValues(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]float64(nil), values...)
	sort.Float64s(values)
	i := len(values) / 2
	if len(values)%2 == 0 {
		return (values[i-1] + values[i]) / 2
	}
	return values[i]
}

// continuousVATWindows freezes a preceding 3-6 month baseline before each
// candidate run. Missing months and quarter/month mixing break continuity.
// Sales must not fall below the same baseline median; lower sales is not
// silently treated as unchanged. Six-month evidence never independently grades high.
func continuousVATWindows(ctx *reportContext) []vatLowWindow {
	facts := append([]vatFact(nil), ctx.vat...)
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].TaxPeriod.Start < facts[j].TaxPeriod.Start })
	var result []vatLowWindow
	for start := vatBurdenHistoryMinimum; start < len(facts); start++ {
		first := start
		for first > 0 && start-first < vatBurdenHistoryMaximum && comparableVATMonth(facts[first-1]) && adjacentAnalysisPeriods(facts[first-1].Period, facts[first].Period) {
			first--
		}
		if start-first < vatBurdenHistoryMinimum {
			continue
		}
		var rates, sales []float64
		for _, f := range facts[first:start] {
			rates = append(rates, f.SupplementTax/f.Sales)
			sales = append(sales, f.Sales)
		}
		baseRate, baseSales := medianValues(rates), medianValues(sales)
		if baseRate <= 0 {
			continue
		}
		end := start
		for end < len(facts) && comparableVATMonth(facts[end]) && (end == start || adjacentAnalysisPeriods(facts[end-1].Period, facts[end].Period)) && facts[end].SupplementTax/facts[end].Sales <= baseRate*(1-vatBurdenDrop)+1e-12 && facts[end].Sales >= baseSales {
			end++
		}
		if end-start < vatBurdenHistoryMinimum {
			continue
		}
		w := vatLowWindow{Facts: facts[start:end], Baseline: facts[first:start], BaseRate: baseRate, BaseSales: baseSales}
		if facts[end-1].HasEndCredit && facts[start-1].HasEndCredit {
			w.CreditChange = facts[end-1].EndCredit - facts[start-1].EndCredit
			w.HasCreditChange = true
		}
		// Keep the earliest frozen baseline for each continuous low run.
		if len(result) > 0 && result[len(result)-1].Facts[len(result[len(result)-1].Facts)-1].TaxPeriod.End >= facts[start].TaxPeriod.Start {
			continue
		}
		result = append(result, w)
	}
	return result
}

func evaluateContinuousVATBurden(ctx *reportContext) []RuleEvaluation {
	var result []RuleEvaluation
	for _, w := range continuousVATWindows(ctx) {
		periods := make([]string, len(w.Facts))
		for i, f := range w.Facts {
			periods[i] = f.Period
		}
		evidence := fmt.Sprintf("%s连续%d个月税负较此前%d个可比月中位数%s下降至少30%%，各月销售额均未低于同基期销售中位数%s。", strings.Join(periods, "、"), len(periods), len(w.Baseline), percentText(w.BaseRate), moneyText(w.BaseSales))
		if len(periods) >= vatBurdenLongWindow && w.HasCreditChange && w.CreditChange > 0 {
			evidence += fmt.Sprintf("该连续期间末留抵较开始前增加%s。", moneyText(w.CreditChange))
		}
		finding := Finding{ID: vatContinuousBurdenRule + "-" + periods[0], RiskThemeID: vatContinuousBurdenRule, Title: "增值税税负连续低于企业可比历史", Level: RiskMedium, ModuleCode: "vat_returns", Period: strings.Join(periods, "、"), Periods: periods, Evidence: evidence, Recommendation: "按连续偏离月份核对集中采购、固定资产投入、进项转出和留抵结转，对照可得毛利与简易、免税业务解释差异；以同属期调节结果确认原因，不按绝对税负率机械调整申报。", Confidence: "中", VerificationRequired: true}
		result = append(result, RuleEvaluation{RuleID: vatContinuousBurdenRule, Outcome: OutcomeRisk, Finding: &finding, Evidence: evidence})
	}
	return result
}

func analysisVATContinuity(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "vat.burden-trend"}
	for _, w := range continuousVATWindows(ctx) {
		first, last := w.Facts[0], w.Facts[len(w.Facts)-1]
		period := first.Period + "至" + last.Period
		baseline := w.Baseline[0].Period + "至" + w.Baseline[len(w.Baseline)-1].Period
		key := "vat-continuous." + analysisToken(period)
		p := d.text(key+".period", "连续偏离范围", "period", period)
		b := d.text(key+".baseline", "企业历史基期", "period", baseline)
		count := d.number(key+".count", "连续偏低月份数", "streak", "期", period, baseline, "每月税负较固定前期中位数下降至少30%且每月销售未低于前期中位数；缺月中断", float64(len(w.Facts)), "vat_returns")
		rate := d.number(key+".baseline-rate", "基期税负中位数", "ratio", "%", period, baseline, "连续区间开始前3至6个可比月应补退税额/当期销售额的中位数", w.BaseRate*100, "vat_returns")
		sales := d.number(key+".baseline-sales", "基期销售额中位数", "amount", "元", period, baseline, "与税负使用相同基期的当期销售额中位数", w.BaseSales, "vat_returns")
		refs := []string{p, b, count, rate, sales}
		if w.HasCreditChange {
			refs = append(refs, d.number(key+".credit-change", "连续区间留抵增加额", "change", "元", period, baseline, "连续区间期末留抵−开始前一期期末留抵", w.CreditChange, "vat_returns"))
		}
		if len(d.Focus) < 2 {
			d.focus(key, "说明连续偏离的具体范围、月份数、历史基期和税负基准，同时解释销售与留抵联动；有同属期毛利和进项证据时补业务联系，未取得原因证据不编造低税负原因。", refs...)
		}
	}
	for _, f := range ctx.vat {
		source := "vat_returns/" + f.RecordID
		prefix := "vat-link." + analysisToken(f.Period)
		for _, v := range []struct {
			name  string
			value float64
			known bool
		}{{"申报销售额", f.Sales, f.HasSales}, {"应补退税额", f.SupplementTax, f.HasSupplementTax}, {"申报进项税额", f.InputTax, f.HasInputTax}, {"期末留抵税额", f.EndCredit, f.HasEndCredit}} {
			if v.known {
				d.number(prefix+"."+analysisToken(v.name), v.name, "amount", "元", f.Period, "", "同属期申报已确认字段", v.value, source)
			}
		}
	}
	for _, f := range financialTrendFacts(ctx.financial) {
		if f.HasRevenue && f.Revenue > 0 && f.HasCost {
			d.number("vat-margin."+analysisToken(f.Period), "财报毛利率", "ratio", "%", f.Period, "", "(同一报表期间收入−成本)/收入×100；不得分摊至月", (f.Revenue-f.Cost)/f.Revenue*100, "income_statements/"+f.Period)
		}
	}
	d.Constraints = []string{"连续3个月规则只用于完整、连续且同口径月报；季度申报按实际季度展示，不将3个季度偷换为3个月。", "同一偏离区间使用开始前3至6个可比月固定中位数；本实现仅在销售未低于同基期中位数时确认未同步下降，其他情况保留趋势解释。", "连续6个月并叠加留抵增长时补充经营关注，缺少毛利异常标准或原因调节证据不升级为高风险；绝对低税负不直接判高。", "财报毛利按实际报表期间与增值税联动，季报不分摊为月报；未取得可靠行业基准只比较企业自身历史。"}
	return d
}
