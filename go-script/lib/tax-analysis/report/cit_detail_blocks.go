package report

import (
	"fmt"
	"strings"
)

func citAdjustmentRows(ctx *reportContext) [][]string {
	var rows [][]string
	seen := map[string]bool{}
	for _, fact := range ctx.taxFields {
		if fact.Module != "cit_annual_returns" || !(containsAny(fact.FormCode, "A105000") || strings.Contains(fact.FormName, "纳税调整项目明细表")) || fact.Status != "present" {
			continue
		}
		if !declarationLabelMatches(fact.ColumnLabel, []string{"账载金额", "税收金额", "调增金额", "调减金额"}) {
			continue
		}
		key := fact.RecordID + "|" + fact.RowLabel
		if seen[key] {
			continue
		}
		seen[key] = true
		var values []string
		nonzero := false
		for _, column := range []string{"账载金额", "税收金额", "调增金额", "调减金额"} {
			value := "—"
			for _, candidate := range ctx.taxFields {
				if candidate.RecordID == fact.RecordID && candidate.FormCode == fact.FormCode && candidate.Row == fact.Row && candidate.Table == fact.Table && canonicalFormLabel(candidate.ColumnLabel) == column && candidate.Status == "present" {
					value = moneyText(candidate.Value)
					nonzero = nonzero || (candidate.Value != 0 && (column == "调增金额" || column == "调减金额"))
					break
				}
			}
			values = append(values, column+"："+value)
		}
		if (nonzero && !containsAny(fact.RowLabel, "类调整项目")) || fact.RowLabel == "合计" {
			rows = append(rows, []string{fact.RowLabel, fact.Period.Display + "；" + strings.Join(values, "；"), citAdjustmentNote(fact.RowLabel)})
		}
	}
	return rows
}

// citResearchRows keeps financial research, quarterly research deductions and
// annual super-deduction in separate rows with their actual period and column.
func citResearchRows(ctx *reportContext) [][]string {
	var rows [][]string
	facts := citResearchAmountFields(ctx.taxFields)
	// A returned monetary breakdown is still useful when its total is absent.
	// Preserve its name and never invent a total by adding parent/child rows.
	covered := map[string]bool{}
	for _, fact := range facts {
		covered[fact.RecordID] = true
	}
	for _, fact := range citResearchDetailFields(ctx.taxFields) {
		if !covered[fact.RecordID] {
			facts = append(facts, fact)
		}
	}
	for _, fact := range facts {
		income, incomeOK, incomeSource := financialIncomeForResearch(ctx, fact)
		known := incomeOK && income != 0
		share := safeRatio(fact.Value, income)
		ctx.recordCalculation("6.6", fact.Period.Display, "研发加计扣除占收入比例", "研发加计扣除申报额 / 同期累计财报营业收入", []CalculationInput{taxCalculationInput(fact), incomeSource}, share, known)
		basis := "截至季度末累计"
		if fact.Module == "cit_annual_returns" {
			basis = "年度汇算"
		}
		if !covered[fact.RecordID] {
			basis += " · " + fact.RowLabel + "（总额未取得）"
		}
		rows = append(rows, []string{fact.Period.Display, moneyText(fact.Value), optionalPercent(share, known), basis})
	}
	return rows
}

func isResearchProjectCount(fact TaxFieldFact) bool {
	return isCITResearchForm(fact) && strings.Contains(canonicalFormLabel(fact.RowLabel), "项目数量")
}

// Project counts remain visible as quantities, outside monetary tables and ratios.
func citResearchQuantityBlocks(ctx *reportContext) []Block {
	var paragraphs []string
	seen := map[string]bool{}
	for _, fact := range ctx.taxFields {
		if fact.Module != "cit_annual_returns" || fact.Status != "present" || !isResearchProjectCount(fact) {
			continue
		}
		key := fact.RecordID + "|" + fact.FormCode + "|" + fact.RowLabel
		if seen[key] {
			continue
		}
		seen[key] = true
		paragraphs = append(paragraphs, fmt.Sprintf("%s · %s：%g项（数量，不参与金额及收入占比计算）。", fact.Period.Display, fact.RowLabel, fact.Value))
	}
	if len(paragraphs) == 0 {
		return nil
	}
	return []Block{{Kind: BlockParagraph, Paragraphs: paragraphs}}
}

func citPreferenceRows(ctx *reportContext) [][]string {
	var rows [][]string
	seen := map[string]bool{}
	for _, fact := range ctx.taxFields {
		if fact.Status != "present" || (fact.Module != "cit_quarterly_returns" && fact.Module != "cit_annual_returns") {
			continue
		}
		if isResearchProjectCount(fact) {
			continue
		}
		if !containsAny(fact.RowLabel, "小型微利企业减免", "高新技术企业", "加计扣除", "减免所得税", "减免税额", "抵免所得税额") {
			continue
		}
		if !declarationLabelMatches(fact.ColumnLabel, []string{"金额", "本年累计", "本年累计金额"}) {
			continue
		}
		key := fact.RecordID + "|" + fact.FormCode + "|" + fact.RowLabel + "|" + fact.ColumnLabel
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, []string{fact.RowLabel, fact.Period.Display + " · " + fact.ColumnLabel + "：" + moneyText(fact.Value), "实际申报金额；资格信息另行核实"})
	}
	return rows
}

func safeRatio(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

// deductionEvidenceRows exposes an estimate with all included inputs. Invoice
// purchases and accrued expense periods are not interchangeable, so the delta
// never becomes a definitive missing-invoice amount or a new confirmed risk.
func deductionEvidenceRows(ctx *reportContext) [][]string {
	var rows [][]string
	for _, fact := range ctx.financial {
		if !fact.HasCost {
			continue
		}
		values := deductionAmounts(ctx, fact)
		purchases, purchasesOK := values.Purchases, values.PurchasesKnown
		expenses, expensesOK := values.Expenses, values.ExpensesKnown
		known := purchasesOK && expensesOK && fact.ResearchInManagementSet
		base := fact.Cost + expenses
		rows = append(rows, []string{fact.Period + " · 成本费用与进项票据估算", fmt.Sprintf("营业成本%s；期间费用%s；进项发票净额%s；未调节差额%s", moneyText(fact.Cost), optionalMoney(expenses, expensesOK), optionalMoney(purchases, purchasesOK), optionalMoney(base-purchases, known)), "工资社保、折旧摊销、存货变动、资本性购进及跨期等尚需调节，不等于确定缺票金额"})
	}
	rows = append(rows, []string{"已取得的进项发票", fmt.Sprintf("纳入分析%d张，红字按原始负数参与净额", invoiceCount(ctx.received)), "票据与费用归集关系需进一步核对"})
	rows = append(rows, knownDeductionEvidenceRows(ctx)...)
	return rows
}

func invoiceAmountForAnnualPeriod(facts []invoiceGroup, period TaxPeriod, complete bool) (float64, bool) {
	value := 0.0
	for _, fact := range facts {
		if !monthWithinAnalysisPeriod(fact.Month, period.Start, period.End) {
			continue
		}
		complete = complete && fact.HasAmount
		value += fact.Amount
	}
	return value, complete
}

// knownDeductionEvidenceRows lists observed payroll/depreciation and invoice
// evidence without subtracting possibly overlapping costs from every quarter.
func knownDeductionEvidenceRows(ctx *reportContext) [][]string {
	var rows [][]string
	seen := map[string]bool{}
	for _, fact := range ctx.taxFields {
		if fact.Module != "cit_annual_returns" || fact.Status != "present" {
			continue
		}
		if !containsAny(fact.FormCode, "A105050", "A105080") && !containsAny(fact.FormName, "职工薪酬", "折旧、摊销") {
			continue
		}
		if canonicalFormLabel(fact.RowLabel) != "合计" {
			continue
		}
		if !(strings.Contains(fact.FormName, "职工薪酬") && canonicalFormLabel(fact.ColumnLabel) == "税收金额") && !(containsAny(fact.FormName, "折旧", "摊销") && canonicalFormLabel(fact.ColumnLabel) == "税收折旧、摊销额") {
			continue
		}
		key := fact.RecordID + "|" + fact.FormCode + "|" + fact.RowLabel + "|" + fact.ColumnLabel
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, []string{fact.Period.Display + " · " + fact.FormName, fact.ColumnLabel + "：" + moneyText(fact.Value), "年度税收口径合计，不直接抵减季度发票差额"})
	}
	_, sensitive := sensitiveServiceTotals(ctx.received)
	abnormal, abnormalCount := 0.0, 0
	for _, invoice := range ctx.received {
		if invoice.DirectAbnormal {
			abnormalCount += invoice.Count
			if invoice.HasAmount {
				abnormal += invoice.Amount
			}
		}
	}
	if sensitive.count > 0 {
		rows = append(rows, []string{"取得敏感服务发票", fmt.Sprintf("%d条明细；不含税净额%s", sensitive.count, moneyText(sensitive.amount)), "分类线索，不据名称认定不合规扣除"})
	}
	if abnormalCount > 0 {
		rows = append(rows, []string{"有直接异常状态的进项发票", fmt.Sprintf("%d张；已知不含税净额%s", abnormalCount, moneyText(abnormal)), "按原始异常状态核实扣除处理；缺失金额不参与已知金额合计"})
	}
	return rows
}
