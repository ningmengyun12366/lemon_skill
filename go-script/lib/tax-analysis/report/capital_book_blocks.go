package report

import (
	"fmt"
	"strings"
)

// capitalBookSummaryRows keeps registered capital out of the taxable base.
// It compares actual financial opening/closing equity with obtained declared
// book bases, preferring business books and falling back to capital books only
// when no usable business-book basis exists. Explicit zero is a usable basis.
func capitalBookSummaryRows(ctx *reportContext) [][]string {
	var rows [][]string
	maximum, hasMaximum, year := 0.0, false, ""
	for _, item := range []string{"营业账簿", "资金账簿"} {
		for _, fact := range ctx.taxFields {
			if fact.Module == "stamp_tax" && fact.Status == "present" && strings.Contains(fact.RowLabel, item) &&
				!strings.Contains(fact.RowLabel, "其他营业账簿") && canonicalFormLabel(fact.ColumnLabel) == "计税依据" {
				if !hasMaximum || fact.Value > maximum {
					maximum, hasMaximum, year = fact.Value, true, fact.Period.Display
				}
			}
		}
		if hasMaximum {
			break
		}
	}
	complete := false
	rangeText := "历史查询范围未取得"
	if root, ok := ctx.modules["stamp_tax"].Raw.(map[string]any); ok {
		if history, ok := root["historicalCapitalBooks"].(map[string]any); ok {
			complete, _ = history["complete"].(bool)
			if coverage, ok := history["queryCoverage"].(map[string]any); ok {
				rangeText = mapText(coverage, "startTime") + "至" + mapText(coverage, "endTime")
			}
		}
	}
	historyLabel := "已取得申报中的最高依据；历史尚未完整覆盖"
	if complete {
		historyLabel = "已查询历史范围内最高申报依据"
	}
	basisText := optionalMoney(maximum, hasMaximum)
	if hasMaximum && strings.TrimSpace(year) != "" {
		basisText += " · " + year
	}
	if !hasMaximum {
		historyLabel = "未取得可用的账簿申报计税依据"
	}
	rows = append(rows, []string{"营业账簿历史基数", basisText, historyLabel + "；" + rangeText})
	for _, fact := range ctx.financial {
		capital, ok := fact.Balances["实收资本（或股本）"]
		if !ok {
			capital, ok = fact.Balances["实收资本"]
		}
		reserve, reserveOK := fact.Balances["资本公积"]
		if !ok && !reserveOK {
			continue
		}
		openingOK := capital.HasOpening && reserve.HasOpening
		closingOK := capital.HasClosing && reserve.HasClosing
		opening, closing := capital.Opening+reserve.Opening, capital.Closing+reserve.Closing
		rows = append(rows, []string{fact.Period + " · 实收资本及资本公积", fmt.Sprintf("期初%s；期末%s；净变动%s", optionalMoney(opening, openingOK), optionalMoney(closing, closingOK), optionalMoney(closing-opening, openingOK && closingOK)), "采用财报账面值；净变动不等于本期新增资本，不能单凭该值确定应补税额"})
		if closingOK && hasMaximum {
			rows = append(rows, []string{fact.Period + " · 与历史申报依据差额", moneyText(closing - maximum), "未作历史覆盖缺口、适用减免等调节，不直接判定应补印花税"})
		}
	}
	return rows
}

// stampContractCoverageRows uses net invoices as an estimate, not as proof that
// every invoice corresponds to a taxable contract. Missing coverage stays open.
func stampContractCoverageRows(ctx *reportContext) [][]string {
	basis, hasBasis := 0.0, false
	var inputs []CalculationInput
	for _, line := range ctx.taxLines {
		if line.TaxName != "印花税" || !containsAny(line.Item, "买卖合同", "购销合同") {
			continue
		}
		if value, ok := line.basis(); ok {
			basis += value
			hasBasis = true
			for _, field := range line.Fields {
				if canonicalFormLabel(field.ColumnLabel) == "计税依据" {
					inputs = append(inputs, taxCalculationInput(field))
				}
			}
		}
	}
	estimate, invoicesKnown := 0.0, ctx.modules["issued_invoices"].Complete && ctx.modules["received_invoices"].Complete
	for _, group := range []struct {
		module   string
		invoices []invoiceGroup
	}{{"issued_invoices", ctx.issued}, {"received_invoices", ctx.received}} {
		for _, invoice := range group.invoices {
			if !monthWithinAnalysisPeriod(invoice.Month, ctx.task.StartTime, ctx.task.EndTime) {
				continue
			}
			estimate += invoice.Amount
			invoicesKnown = invoicesKnown && invoice.HasAmount
			inputs = append(inputs, CalculationInput{Source: group.module + ".chunks.groups[id=" + invoice.ID + "]", Label: invoice.Month + "不含税金额", Value: invoice.Amount, Present: invoice.HasAmount})
		}
	}
	known := hasBasis && invoicesKnown && estimate > 0
	period := analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime)
	ratio := safeRatio(basis, estimate)
	ctx.recordCalculation("7.1", period, "买卖合同估算覆盖率", "买卖合同申报计税依据合计 / 同期进销发票有符号不含税净额", inputs, ratio, known)
	return [][]string{{"买卖合同估算覆盖", "申报依据" + optionalMoney(basis, hasBasis) + "；进销发票净额" + optionalMoney(estimate, invoicesKnown), optionalPercent(ratio, known) + "；发票估算不代表全部应税合同，需核合同范围、跨期及减免"}}
}
