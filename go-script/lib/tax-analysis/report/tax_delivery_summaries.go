package report

import (
	"fmt"
	"strings"
)

// vatCurrentSummary aggregates one current-period business field. Cumulative
// columns and repeated annex/main totals must never enter the same sum.
func vatCurrentSummary(ctx *reportContext, match func(TaxFieldFact) bool) (float64, bool) {
	total, seen, records := 0.0, map[string]bool{}, map[string]bool{}
	for _, f := range ctx.taxFields {
		if f.Module != "vat_returns" || f.Status != "present" || !match(f) {
			continue
		}
		key := f.RecordID + "|" + f.FormCode + "|" + fmt.Sprint(f.Table, "/", f.Row, "/", f.Column)
		if seen[key] {
			continue
		}
		seen[key] = true
		total += f.Value
		records[f.RecordID] = true
	}
	known := len(seen) > 0
	for _, f := range ctx.vat {
		known = known && records[f.RecordID]
	}
	return total, known
}

func vatMainCurrentField(f TaxFieldFact, labels ...string) bool {
	return (f.FormCode == vatMainFormCode || f.FormCode == vatNewMainFormCode) &&
		declarationLabelMatches(f.ColumnLabel, []string{"本月数", "本期数", "本期金额"}) &&
		(declarationLabelMatches(f.RowLabel, labels) || declarationLabelMatches(normalizeFinancialMetricLabel(f.RowLabel), labels))
}

func vatApplicabilityRows(ctx *reportContext) [][]string {
	rows := [][]string{}
	for _, definition := range []struct {
		name   string
		labels []string
	}{
		{"免税销售额", []string{"免税销售额", "（四）免税销售额"}},
		{"简易计税销售额", []string{"按简易办法计税销售额", "（二）按简易办法计税销售额", "简易计税销售额"}},
	} {
		amount, known := vatCurrentSummary(ctx, func(f TaxFieldFact) bool { return vatMainCurrentField(f, definition.labels...) })
		status := "本期未取得完整申报字段，暂不判断适用情况"
		if known {
			status = "本期申报金额为零"
			if amount != 0 {
				status = "本期有申报发生额"
			}
		}
		rows = append(rows, []string{definition.name, optionalMoney(amount, known), status})
	}
	difference, differenceKnown := vatCurrentSummary(ctx, func(f TaxFieldFact) bool {
		return containsAny(f.FormName, "扣除项目") && canonicalFormLabel(f.ColumnLabel) == "本期实际扣除金额" &&
			!declarationLabelMatches(f.RowLabel, []string{"合计", "总计"}) && !strings.Contains(f.RowLabel, "其中")
	})
	rows = append(rows, []string{"差额征税实际扣除额", optionalMoney(difference, differenceKnown), "扣除额按各期发生额汇总；未取得的期间不按零补齐"})
	rates := []string{}
	for _, row := range vatBusinessRateRows(ctx) {
		rates = appendUnique(rates, row[0])
	}
	rows = append(rows, []string{"有销售额业务的税率/征收率", textOr(strings.Join(rates, "、"), "未取得"), "仅反映本期有发生额的业务"})
	return append(rows, vatPolicyTableRows(ctx)...)
}

func vatSpecialSummaryRows(ctx *reportContext) [][]string {
	rows := [][]string{}
	if ctx.taxpayer == taxpayerSmall {
		// Small-taxpayer differences remain explicit differences, never unbilled sales.
		rows = vatSpecialRows(ctx)
	} else {
		amount, known := vatCurrentSummary(ctx, vatUnbilledBusinessField)
		rows = append(rows, []string{"未开票销售额", optionalMoney(amount, known), "本期各申报期间未开票销售额合计"})
		rows = append(rows, []string{"红字销项金额", moneyText(-ctx.redIssuedAmount), "不含税净额；红字保留负数"})
	}
	for _, name := range []string{"固定资产处置", "视同销售"} {
		amount, known := vatCurrentSummary(ctx, func(f TaxFieldFact) bool {
			return (f.FormCode == vatMainFormCode || f.FormCode == vatNewMainFormCode) && strings.Contains(f.RowLabel, name) && declarationLabelMatches(f.ColumnLabel, []string{"本月数", "本期数"})
		})
		note := "未取得单独列示金额，暂不能判断发生情况"
		if known {
			note = "申报单列金额"
		}
		rows = append(rows, []string{name, optionalMoney(amount, known), note})
	}
	return rows
}

// inputDeductionCheckRows joins invoice identities and inspects the dedicated
// abnormal-voucher collection. Selection alone does not prove a final deduction.
func inputDeductionCheckRows(ctx *reportContext) [][]string {
	module := ctx.modules["deduction_confirmations"]
	summary := readDeductionSummary(module.Raw)
	matched, missing, restricted := summary.Association.Matched, summary.Association.Missing, summary.Association.Restricted
	association, _, associationFound := findFirst(module.Raw, "association")
	associationMap, _ := association.(map[string]any)
	_, matchedKnown := mapNumber(associationMap, "matched")
	_, missingKnown := mapNumber(associationMap, "missing")
	_, restrictedKnown := mapNumber(associationMap, "restricted")
	associationKnown := associationFound && matchedKnown && missingKnown && restrictedKnown
	selectedAbnormal := summary.Association.SelectedAbnormal
	normalAbnormal, flagged, conflict, abnormalCount := 0, 0, 0, 0
	abnormalPresent := len(summary.Chunks) > 0
	for _, chunk := range summary.Chunks {
		if chunk.List != nil {
			normalAbnormal += chunk.List.NormalAbnormal
		}
		if chunk.AbnormalInvoiceList == nil {
			abnormalPresent = false
		} else {
			abnormal := chunk.AbnormalInvoiceList
			abnormalCount += abnormal.Count
			flagged += abnormal.Flagged
			conflict += abnormal.Conflict
			selectedAbnormal += abnormal.SelectedAbnormal
		}
	}
	abnormalText := fmt.Sprintf("取得%d条异常凭证查询记录，其中%d条标注异常，%d条异常记录显示已勾选/抵扣，%d条同时存在相反状态", abnormalCount, flagged, selectedAbnormal, conflict)
	if !abnormalPresent {
		abnormalText = fmt.Sprintf("勾选列表有%d条异常状态线索；未取得独立异常凭证查询资料，核验范围不完整", normalAbnormal)
	}
	transfer, transferKnown := vatCurrentSummary(ctx, func(f TaxFieldFact) bool { return vatMainCurrentField(f, "进项税额转出") })
	comparison, differing, greatest := 0, 0, 0.0
	for _, vat := range ctx.vat {
		tax, taxKnown := selectedTaxForPeriod(ctx, vat.TaxPeriod.Start, vat.TaxPeriod.End)
		known := taxKnown && vat.HasInputTax
		ctx.recordCalculation("5.7", vat.Period, "已勾选有效抵扣税额与申报进项差额", "已勾选有效抵扣税额 - 申报进项税额", []CalculationInput{{Source: "deduction_confirmations.chunks.list.selectedTax", Label: "同属期已勾选有效抵扣税额", Value: tax, Present: taxKnown}, {Source: "vat_returns", Label: "申报进项税额", Value: vat.InputTax, Present: vat.HasInputTax}}, tax-vat.InputTax, known)
		if known {
			comparison++
			delta := abs(vat.InputTax - tax)
			if delta >= invoiceAggregationTolerance {
				differing++
			}
			if delta > greatest {
				greatest = delta
			}
		}
	}
	comparisonText := "未取得可按同属期比较的完整勾选及申报进项数据，暂无法判断差额"
	if comparison > 0 {
		comparisonText = fmt.Sprintf("可比较%d期，%d期有差额，最大绝对差额%s", comparison, differing, moneyText(greatest))
	}
	abnormalLevel := "未见明显异常"
	if !module.Complete || !abnormalPresent {
		abnormalLevel = "数据不足"
	}
	if normalAbnormal > 0 || flagged > 0 || conflict > 0 || selectedAbnormal > 0 {
		abnormalLevel = "待核实"
	}
	restrictedLevel := "未见明显异常"
	if !module.Complete || !associationKnown || missing > 0 {
		restrictedLevel = "数据不足"
	}
	if restricted > 0 {
		restrictedLevel = "待核实"
	}
	transferLevel := "数据不足"
	if transferKnown {
		transferLevel = "待核实"
	}
	associationDetails := []string{"票号关联数量未取得", "餐饮/娱乐类已勾选线索数量未取得", "票面品名缺失数量未取得"}
	if matchedKnown {
		associationDetails[0] = fmt.Sprintf("按票号关联%d张", matched)
	}
	if restrictedKnown {
		associationDetails[1] = fmt.Sprintf("餐饮/娱乐类已勾选线索%d张", restricted)
	}
	if missingKnown {
		associationDetails[2] = fmt.Sprintf("缺少票面品名%d张", missing)
	}
	return [][]string{
		{"异常凭证", abnormalText, abnormalLevel},
		{"不得抵扣项目", strings.Join(associationDetails, "，"), restrictedLevel},
		{"进项税额转出", optionalMoney(transfer, transferKnown), transferLevel},
		{"勾选与申报差异", comparisonText, inputDeductionDifferenceLevel(ctx)},
	}
}

// inputDeductionDifferenceLevel reuses only the input reconciliation evaluations.
// Known risks take precedence; incomplete periods never become a clear result.
// This presentation does not create new findings or change rule thresholds.
func inputDeductionDifferenceLevel(ctx *reportContext) string {
	months := deliveryMonths(ctx.task.StartTime, ctx.task.EndTime)
	seen, complete := false, ctx.modules["vat_returns"].Complete && len(months) > 0
	covered := map[string]bool{}
	for _, vat := range ctx.vat {
		if !vat.HasInputTax {
			continue
		}
		for _, month := range deliveryMonths(vat.TaxPeriod.Start, vat.TaxPeriod.End) {
			covered[month] = true
		}
	}
	for _, month := range months {
		complete = complete && covered[month]
	}
	var level RiskLevel
	for _, evaluation := range evaluateInvoiceTaxComparisons(ctx) {
		if evaluation.RuleID != invoiceInputReconciliationRule {
			continue
		}
		seen = true
		switch evaluation.Outcome {
		case OutcomeNotApplicable:
			return "不适用"
		case OutcomeClear:
		case OutcomeRisk:
			if evaluation.Finding == nil {
				complete = false
				continue
			}
			next := evaluation.Finding.Level
			if level == "" || next == RiskHigh || (level == RiskLow && next == RiskMedium) {
				level = next
			}
		default:
			complete = false
		}
	}
	if level != "" {
		return string(level)
	}
	if seen && complete {
		return "未见明显异常"
	}
	return "数据不足"
}
