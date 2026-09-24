package report

import (
	"math"
	"strings"
)

// These audited Word boundaries select subjects for explanation, not findings.
// A raw difference has not yet passed reconciliation or materiality checks.
const (
	vatFinancialAttentionPercent = 3.0
	citRevenueAttentionPercent   = 1.0
	citProfitAttentionPercent    = 20.0
)

type financialComparison struct {
	key, period, basis string
	left, right        CalculationInput
	financialBase      bool
	reverseDifference  bool
	attention          bool
}

// analysisFinancialComparisons reuses the same unrounded period selectors as
// the delivery tables. Quarterly CIT is restored only from adjacent YTD data;
// VAT/accounting revenue uses the actual month/quarter; annual figures are separate.
func analysisFinancialComparisons(ctx *reportContext) []SectionAnalysisData {
	vat := SectionAnalysisData{SectionID: "vat.financial-comparison"}
	revenue := SectionAnalysisData{SectionID: "cit.financial-revenue"}
	profit := SectionAnalysisData{SectionID: "cit.profit-taxable"}
	pairs := map[string][]financialComparison{}
	input := func(module, period, label string, value float64, known bool) CalculationInput {
		return CalculationInput{Source: module + "/period=" + period + "/" + label, Label: label, Value: value, Present: known}
	}
	for _, f := range ctx.financial {
		r, rk, v, vk := financialVATPeriodValues(ctx, f)
		key, basis := "revenue-period", "同一实际所属期；财报单月或单季收入与该期间增值税销售额比较"
		if financialVATAnnual(f) {
			key, basis = "revenue-annual", "同年度单独比较，不与月季数据相加"
		}
		left, right := input("income_statements", f.Period, "财报当期营业收入", r, rk), input("vat_returns", f.Period, "同属期申报销售额", v, vk)
		pairs[vat.SectionID] = append(pairs[vat.SectionID], financialComparison{key: key, period: f.Period, basis: basis, left: left, right: right,
			attention: comparisonPercent(left, right, false) >= vatFinancialAttentionPercent})
	}
	for _, item := range quarterlyCITMetricForContext(ctx, func(f citFact) (float64, bool) { return f.CumulativeIncome, f.HasIncome }) {
		v, vk := financialValueForQuarter(ctx.financial, item.period, func(f financialFact) (float64, bool) { return f.Revenue, f.HasRevenue })
		left, right := input("income_statements", item.period, "财报单季营业收入", v, vk), input("cit_quarterly_returns", item.period, "预缴累计收入还原单季", item.value, item.known)
		pairs[revenue.SectionID] = append(pairs[revenue.SectionID], financialComparison{key: "revenue-quarter", period: item.period, basis: "单季；企税本季累计减同年相邻上季累计，首季直接取累计", left: left, right: right,
			attention: comparisonPercent(left, right, false) > citRevenueAttentionPercent})
	}
	declared := map[string]periodValue{}
	for _, item := range quarterlyCITMetricForContext(ctx, func(f citFact) (float64, bool) { return f.CumulativeProfit, f.HasProfit }) {
		declared[item.period] = item
	}
	for _, item := range quarterlyCITMetricForContext(ctx, func(f citFact) (float64, bool) { return f.CumulativeActualProfit, f.HasActualProfit }) {
		v, vk := financialValueForQuarter(ctx.financial, item.period, func(f financialFact) (float64, bool) { return f.Profit, f.HasProfit })
		p := declared[item.period]
		financial := input("income_statements", item.period, "财报单季利润总额", v, vk)
		filing := input("cit_quarterly_returns", item.period, "预缴利润总额还原单季", p.value, p.known)
		actual := input("cit_quarterly_returns", item.period, "预缴累计实际利润额的单季变动", item.value, item.known)
		for _, pair := range []financialComparison{
			{key: "financial-filing-profit", left: financial, right: filing},
			{key: "financial-actual-profit", left: financial, right: actual},
			{key: "filing-actual-profit", left: filing, right: actual, reverseDifference: true},
		} {
			pair.period, pair.basis, pair.financialBase = item.period, "单季；分别比较财报利润、预缴累计利润还原单季、预缴累计实际利润额的单季变动；后者不是本季申报的应纳税所得额", true
			pair.attention = comparisonPercent(pair.left, pair.right, true) > citProfitAttentionPercent
			pairs[profit.SectionID] = append(pairs[profit.SectionID], pair)
		}
	}
	for _, f := range annualCITMainFields(ctx.taxFields, []string{"营业收入", "利润总额", "应纳税所得额"}) {
		for _, financial := range ctx.financial {
			if financial.TaxPeriod.Start != f.Period.Start || financial.TaxPeriod.End != f.Period.End {
				continue
			}
			key, section, amount, known, financialBase := "revenue-annual", revenue.SectionID, financial.Revenue, financial.HasRevenue, false
			if normalizeFinancialMetricLabel(f.RowLabel) != financialMetricRevenue {
				key, section, amount, known, financialBase = "profit-annual-"+analysisToken(f.RowLabel), profit.SectionID, financial.Profit, financial.HasProfit, true
			}
			left, right := input("income_statements", financial.Period, "年度财报"+f.RowLabel, amount, known), taxCalculationInput(f)
			threshold := citRevenueAttentionPercent
			if financialBase {
				left.Label = "年度财报利润总额"
				threshold = citProfitAttentionPercent
			}
			pairs[section] = append(pairs[section], financialComparison{key: key, period: f.Period.Display, basis: "年度汇算；单列，不与季度预缴合计", left: left, right: right, financialBase: financialBase,
				attention: comparisonPercent(left, right, financialBase) > threshold})
		}
	}
	result := []SectionAnalysisData{vat, revenue, profit}
	for i := range result {
		d := &result[i]
		addFinancialComparisonMetrics(d, pairs[d.SectionID])
		d.Constraints = []string{financialPeriodBasis, "原始差额尚未调节，不能当作调节后剩余差额；不自动构成新增风险，不写成漏报或少缴。", "累计原值、还原当期金额分别保留。小企业及其他准则的本期原值与累计差分若不一致，差分仅作勾稽，不能为消除差异替换原值。", "服务先开票后按履约进度分期确认收入、合同负债结转等可作为待核实方向；没有合同、履约或递延收入证据，不能直接认定本企业存在该原因。", "只把同期间、同方向且金额可勾稽的调整计入解释；已知某项优惠金额不等于已解释全部差额。"}
		if d.SectionID == profit.SectionID {
			d.RelatedSections = []string{"cit.annual", "cit.research-expense", "cit.preferences"}
			addComparisonAdjustments(d, ctx.taxFields)
			bridges := addAnnualCITReconciliation(d, ctx.taxFields)
			addAnnualFinancialReconciliation(d, bridges)
			addQuarterlyCITReconciliation(d, ctx)
			addFinancialBasisChecks(d, ctx.taxFields, financialMetricProfit, ctx.financial...)
			addFinancialNormalizationFacts(d, ctx.financial, financialMetricProfit)
		} else if d.SectionID == revenue.SectionID {
			addFinancialBasisChecks(d, ctx.taxFields, financialMetricRevenue, ctx.financial...)
			addFinancialNormalizationFacts(d, ctx.financial, financialMetricRevenue)
		} else {
			addFinancialNormalizationFacts(d, ctx.financial, financialMetricRevenue)
		}
	}
	return result
}

// addFinancialBasisChecks reconciles the source current-period amount with
// adjacent YTD values from the same year. The source tax period determines the
// current amount's duration; the reconciliation never replaces that amount.
func addFinancialBasisChecks(d *SectionAnalysisData, fields []TaxFieldFact, metric string, facts ...financialFact) {
	type basisFields struct{ current, cumulative TaxFieldFact }
	byQuarter := map[string]basisFields{}
	enterprisePeriods := map[string]bool{}
	for _, f := range facts {
		if f.AccountingStandard == financialStandardEnterprise {
			enterprisePeriods[f.TaxPeriod.Start+"|"+f.TaxPeriod.End] = true
		}
	}
	for _, f := range fields {
		if enterprisePeriods[f.Period.Start+"|"+f.Period.End] || f.Module != "income_statements" || f.Status != "present" || normalizeFinancialMetricLabel(f.RowLabel) != metric || strings.Contains(f.Period.Display, "年度") {
			continue
		}
		if start, _ := quarterPeriodBounds(f.Period.Display); start == "" {
			continue
		}
		quarter := normalizeQuarter(f.Period.End)
		entry := byQuarter[quarter]
		column := canonicalFormLabel(f.ColumnLabel)
		if containsAny(column, "本年累计", "本年金额") && f.FiledAt >= entry.cumulative.FiledAt {
			entry.cumulative = f
		} else if containsAny(column, "本期金额", "本月金额", "本期数", "本月数") && f.FiledAt >= entry.current.FiledAt {
			entry.current = f
		}
		byQuarter[quarter] = entry
	}
	for _, quarter := range sortedMapKeys(byQuarter) {
		entry := byQuarter[quarter]
		q := quarterNumber(quarter)
		if q <= 1 || entry.current.ID == "" || entry.cumulative.ID == "" || entry.current.RecordID != entry.cumulative.RecordID || entry.current.FiledAt != entry.cumulative.FiledAt {
			continue
		}
		previous := byQuarter[quarter[:4]+"Q"+string(rune('0'+q-1))].cumulative
		if previous.ID == "" {
			continue
		}
		bridge := entry.cumulative.Value - previous.Value
		if math.Abs(bridge-entry.current.Value) < invoiceAggregationTolerance {
			continue
		}
		prefix := "financial-basis." + analysisToken(quarter+metric)
		var refs []string
		for i, f := range []TaxFieldFact{entry.current, entry.cumulative, previous} {
			id := d.number(prefix+".source-"+string(rune('0'+i)), f.RowLabel+" · "+f.ColumnLabel, "source-amount", "元", f.Period.Display, "", "财报原栏原值，保留所属期和报送版本", f.Value, "report-model.json/taxFields[id="+f.ID+"]")
			d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(f)}
			refs = append(refs, id)
		}
		id := d.number(prefix+".cumulative-bridge", "同年相邻季度累计金额之差", "basis-comparison", "元", quarter, previous.Period.Display, "本季末累计−同年前季末累计；仅作勾稽，不替换按所属期取得的本期金额", bridge, refs[1], refs[2])
		d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(entry.cumulative), taxCalculationInput(previous)}
		differenceID := d.number(prefix+".current-minus-bridge", "本期金额与相邻累计差分的勾稽差额", "basis-comparison", "元", quarter, previous.Period.Display, "本期金额−（本季末累计−同年前季末累计）；不等于税会比较的调节后差额", entry.current.Value-bridge, refs[0], id)
		refs = append(refs, id, differenceID)
		refs = append(refs, d.text(prefix+".status", "财报累计勾稽状态", "state", "本期金额按所属期取值，与同年相邻累计差分存在勾稽差额；差额形成原因尚未确认，不能据此认定企业收入/利润异常", refs...))
		d.focus(prefix, "结合收入或利润比较说明本期金额与相邻累计差分的勾稽差额。按所属期使用财报本期原值，不能仅凭本月列名把季度金额解释为单月，也不改用累计差分消除差异。填报、更正或账务调整仅作为有待证据支持的原因，不写成已确认结论。", refs...)
	}
}

func comparisonPercent(left, right CalculationInput, financialBase bool) float64 {
	base := math.Max(math.Abs(left.Value), 1)
	if !financialBase {
		base = math.Max(base, math.Abs(right.Value))
	}
	return math.Abs(left.Value-right.Value) / base * 100
}

func addFinancialComparisonMetrics(d *SectionAnalysisData, pairs []financialComparison) {
	best, bundles := map[string]float64{}, map[string][]string{}
	for _, pair := range pairs {
		if !pair.left.Present || !pair.right.Present {
			continue
		}
		prefix := "financial-comparison." + pair.key + "." + analysisToken(pair.period)
		sources := []string{pair.left.Source, pair.right.Source}
		period := d.text(prefix+".period", "比较期间及口径", "period", pair.period+"；"+pair.basis, sources...)
		left := d.number(prefix+".left", pair.left.Label, "amount", "元", pair.period, "", "沿用最终表格的未舍入期间底数", pair.left.Value, sources...)
		right := d.number(prefix+".right", pair.right.Label, "amount", "元", pair.period, "", "沿用最终表格的未舍入期间底数", pair.right.Value, sources...)
		difference := pair.left.Value - pair.right.Value
		algorithm := "左侧金额−右侧金额；未调节差额"
		if pair.reverseDifference {
			difference, algorithm = -difference, "右侧实际利润−左侧申报利润总额；沿用表格差额方向"
		}
		diff := d.number(prefix+".difference", pair.left.Label+"与"+pair.right.Label+"差额", "difference", "元", pair.period, "", algorithm, difference, left, right)
		formula := "|两侧差额|/MAX(|两侧金额|,1)×100"
		if pair.financialBase {
			formula = "|两侧差额|/MAX(|左侧比较利润|,1)×100；利润接近0时结合金额判断"
		}
		rate := d.number(prefix+".rate", "未调节差异率", "difference-rate", "%", pair.period, "", formula, comparisonPercent(pair.left, pair.right, pair.financialBase), left, right)
		for i := len(d.Metrics) - 4; i < len(d.Metrics); i++ {
			d.Metrics[i].Inputs = []CalculationInput{pair.left, pair.right}
		}
		state := "尚未建立本项差额与调整项目的勾稽；已取得调整项目另列，备查凭证未核验"
		if math.Abs(difference) < invoiceAggregationTolerance {
			state = "两侧金额相符，无待解释金额差额；不代表业务及凭证已核验"
		}
		status := d.text(prefix+".reconciliation", "差异解释状态", "state", state, diff)
		if pair.attention && math.Abs(difference) >= invoiceAggregationTolerance && math.Abs(difference) > best[pair.key] {
			best[pair.key], bundles[pair.key] = math.Abs(difference), []string{period, left, right, diff, rate, status}
		}
	}
	for _, key := range sortedMapKeys(bundles) {
		d.focus("financial-comparison-"+key, "说明此类比较差额最明显的期间、两侧金额、差额方向及差异率；结合其他非零期间解释变化。区分已取得调整项目与尚未证明的原因，不把未经调节的差额定为漏报；无更正/调节证据时明确待核实范围。", bundles[key]...)
	}
}

// Adjustment amounts remain separate source facts. Their mere presence cannot
// be subtracted from a comparison with a different period, sign or tax basis.
func addComparisonAdjustments(d *SectionAnalysisData, fields []TaxFieldFact) {
	for _, f := range fields {
		if f.Status != "present" || f.Value == 0 || !strings.HasPrefix(f.Module, "cit_") ||
			!containsAny(f.RowLabel, "弥补以前年度亏损", "研究开发费用加计扣除", "研发费用加计扣除", "纳税调整增加额", "纳税调整减少额") ||
			!declarationLabelMatches(f.ColumnLabel, []string{"金额", "本年累计", "本年累计金额"}) {
			continue
		}
		d.number("comparison-adjustment."+analysisToken(f.ID), f.RowLabel+"（原申报口径）", "adjustment-source", "元", f.Period.Display, "", "申报原金额；季度累计与年度分开，不自动冲抵单季差异", f.Value, "report-model.json/taxFields[id="+f.ID+"]")
		d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(f)}
	}
}
