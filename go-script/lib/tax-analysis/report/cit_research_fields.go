package report

import (
	"sort"
	"strings"
)

// citResearchAmountFields resolves a monetary super-deduction source per
// declaration. Annual total outranks its main-form detail; neither the general
// deduction total (row 22) nor project quantities may masquerade as R&D.
func citResearchAmountFields(fields []TaxFieldFact) []TaxFieldFact {
	selected := map[string]TaxFieldFact{}
	priority := map[string]int{}
	for _, fact := range fields {
		if fact.Status != "present" || isResearchProjectCount(fact) ||
			!declarationLabelMatches(fact.ColumnLabel, []string{"本年累计", "本年累计金额", "金额"}) {
			continue
		}
		label := citBusinessLabel(fact.RowLabel)
		rank := 0
		if fact.Module == "cit_quarterly_returns" && (fact.FormCode == citMainFormCode || strings.Contains(fact.FormName, citMainFormNameFragment)) &&
			containsAny(label, "研究开发费用加计扣除", "研发费用加计扣除") {
			rank = 1
		}
		if fact.Module == "cit_annual_returns" {
			if isAnnualCITMainForm(fact) && containsAny(label, "研究开发费用加计扣除", "研发费用加计扣除") {
				rank = 1
			}
			if isCITResearchForm(fact) && containsAny(label, "加计扣除总额") {
				rank = 2
			}
		}
		key := fact.Module + "|" + fact.RecordID + "|" + fact.Period.Display
		if rank > priority[key] {
			selected[key], priority[key] = fact, rank
		}
	}
	result := make([]TaxFieldFact, 0, len(selected))
	for _, fact := range selected {
		result = append(result, fact)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Period.End != result[j].Period.End {
			return result[i].Period.End < result[j].Period.End
		}
		return result[i].Module == "cit_quarterly_returns" && result[j].Module != "cit_quarterly_returns"
	})
	return result
}

// Monetary annex breakdowns remain visible as detail; they are not additional
// points to add to the selected super-deduction total in the trend chart.
func citResearchDetailFields(fields []TaxFieldFact) []TaxFieldFact {
	result := citResearchAmountFields(fields)
	seen := map[string]bool{}
	for _, fact := range result {
		seen[fact.ID] = true
	}
	for _, fact := range fields {
		if fact.Module != "cit_annual_returns" || !isCITResearchForm(fact) || fact.Status != "present" || isResearchProjectCount(fact) ||
			!containsAny(fact.RowLabel, "费用化", "资本化") || !declarationLabelMatches(fact.ColumnLabel, []string{"金额"}) || (fact.ID != "" && seen[fact.ID]) {
			continue
		}
		result = append(result, fact)
		seen[fact.ID] = true
	}
	return result
}

// financialIncomeForResearch requires the same year/end date and cumulative
// basis. A quarter's revenue is never substituted for annual-to-date revenue.
func financialIncomeForResearch(ctx *reportContext, field TaxFieldFact) (float64, bool, CalculationInput) {
	for _, fact := range ctx.financial {
		if fact.TaxPeriod.End != field.Period.End || len(fact.TaxPeriod.Start) < 4 || len(field.Period.Start) < 4 || fact.TaxPeriod.Start[:4] != field.Period.Start[:4] {
			continue
		}
		if field.Module == "cit_annual_returns" && fact.TaxPeriod.Start != field.Period.Start {
			continue
		}
		income, known := financialCumulativeRevenueForContext(ctx, fact)
		return income, known, CalculationInput{Source: "income_statements.records[period=" + fact.TaxPeriod.Start + ":" + fact.TaxPeriod.End + "]", Label: "财报营业收入 · 本年累计", Value: income, Present: known}
	}
	return 0, false, CalculationInput{Label: "同期间财报营业收入 · 本年累计"}
}
