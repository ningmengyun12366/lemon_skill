package report

import "strings"

var annualComparisonHeaders = []string{"期间", "项目", "财报金额", "年度申报金额", "差异/说明"}

func buildCITFinancialRevenueComparison(ctx *reportContext) []Block {
	return append(buildQuarterlyCITRevenueComparison(ctx), annualCITComparisonBlocks(ctx, false)...)
}

func buildCITProfitTaxableComparison(ctx *reportContext) []Block {
	return append(buildQuarterlyCITProfitComparison(ctx), annualCITComparisonBlocks(ctx, true)...)
}

// annualCITMainFields selects the annual main form's amount column by its
// Chinese business row. Reference years remain explicitly identified in Path.
func annualCITMainFields(fields []TaxFieldFact, labels []string) []TaxFieldFact {
	var result []TaxFieldFact
	seen := map[string]bool{}
	for _, fact := range fields {
		if fact.Module != "cit_annual_returns" || fact.Status != "present" || canonicalFormLabel(fact.ColumnLabel) != "金额" {
			continue
		}
		if !isAnnualCITMainForm(fact) {
			continue
		}
		if !financialStringSliceContains(labels, normalizeFinancialMetricLabel(fact.RowLabel)) {
			continue
		}
		key := fact.RecordID + "|" + fact.RowLabel
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, fact)
	}
	return result
}

// annualCITComparisonBlocks appends annual results after the frozen quarterly
// table. It only compares the exact annual period, never Q4 against a full year.
func annualCITComparisonBlocks(ctx *reportContext, profit bool) []Block {
	labels := []string{"营业收入", "营业成本"}
	section := "6.2"
	if profit {
		labels = []string{"利润总额", "纳税调整增加额", "纳税调整减少额", "免税、减计收入及加计扣除", "抵扣应纳税所得额", "应纳税所得额", "弥补以前年度亏损"}
		section = "6.4"
	}
	var rows [][]string
	for _, fact := range annualCITMainFields(ctx.taxFields, labels) {
		label := normalizeFinancialMetricLabel(fact.RowLabel)
		financialValue, financialOK := 0.0, false
		for _, financial := range ctx.financial {
			if financial.TaxPeriod.Start != fact.Period.Start || financial.TaxPeriod.End != fact.Period.End {
				continue
			}
			switch label {
			case "营业收入":
				financialValue, financialOK = financial.Revenue, financial.HasRevenue
			case "营业成本":
				financialValue, financialOK = financial.Cost, financial.HasCost
			case "利润总额":
				financialValue, financialOK = financial.Profit, financial.HasProfit
			}
		}
		note := "年度纳税调节项目；不与财报同名项目直接比较"
		if financialOK {
			note = "同年度差额" + moneyText(financialValue-fact.Value)
		} else if financialStringSliceContains([]string{"营业收入", "营业成本", "利润总额"}, label) {
			note = "未取得同年度财报字段"
		}
		if strings.Contains(fact.Path, "referenceAnnualRecords") {
			note = "最近可取得参考年度；" + note
		}
		rows = append(rows, []string{fact.Period.Display, label, optionalMoney(financialValue, financialOK), moneyText(fact.Value), note})
		if financialStringSliceContains([]string{"营业收入", "营业成本", "利润总额"}, label) {
			ctx.recordCalculation(section, fact.Period.Display, label+"年度申报差额", "同所属期财报金额 - 年度申报金额", []CalculationInput{{Source: "income_statements.records[period=" + fact.Period.Start + ":" + fact.Period.End + "]", Label: label, Value: financialValue, Present: financialOK}, taxCalculationInput(fact)}, financialValue-fact.Value, financialOK)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return []Block{{Kind: BlockTable, Title: "年度汇算与财报同期间核对", Headers: annualComparisonHeaders, Rows: rows}}
}
