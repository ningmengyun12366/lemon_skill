package report

import (
	"sort"
	"strings"
	"time"
)

var vatCumulativeHeaders = []string{"所属期", "申报销售额本年累计", "累计覆盖说明"}

func vatCumulativeBlocks(ctx *reportContext) Block {
	var rows [][]string
	for _, fact := range ctx.vat {
		value, known := vatCumulativeSales(ctx, fact.TaxPeriod)
		note := "本年累计列或年初起连续申报汇总；与当期税负分开列示"
		if !known {
			note = "本年累计列缺失且年初起申报未齐，暂不计算累计"
		}
		rows = append(rows, []string{fact.Period, optionalMoney(value, known), note})
	}
	return Block{Kind: BlockTable, Title: "增值税累计销售额核对", Headers: vatCumulativeHeaders, Rows: rows}
}

// financialActivityRevenue selects 收入合计 at 本年累计数 -> 合计.
// Both headers must identify the same column; equal nonrestricted values are
// not evidence that a neighbouring column is interchangeable.
func financialActivityRevenue(rows []formTableRow) (float64, bool) {
	for _, row := range rows {
		if _, found := formValueCells(row, []string{"收入合计"}); !found {
			continue
		}
		cumulative := formColumnIndexes(rows, row, []string{"本年累计数", "本年累计"})
		totals := formColumnIndexes(rows, row, []string{"合计"})
		for _, column := range cumulative {
			for _, total := range totals {
				if column == total && column < len(row.Grid) {
					return parseFormNumber(row.Grid[column])
				}
			}
		}
	}
	return 0, false
}

func financialCumulativeRevenue(fact financialFact) (float64, bool) {
	if fact.HasCumulativeRevenue {
		return fact.CumulativeRevenue, true
	}
	// Annual and first-quarter amounts cover the year to date by definition.
	if fact.TaxPeriod.Start != "" && strings.HasSuffix(fact.TaxPeriod.Start, "-01-01") || fact.TaxPeriod.Cycle == taxCycleAnnual || strings.Contains(fact.Period, "年度") || fact.TaxPeriod.Cycle == taxCycleQuarterly && quarterNumber(fact.Period) == 1 {
		return fact.Revenue, fact.HasRevenue
	}
	return 0, false
}

// financialCumulativeRevenueForContext uses explicit YTD first, otherwise a
// complete sequence of same-cycle current amounts from January without overlap.
func financialCumulativeRevenueForContext(ctx *reportContext, target financialFact) (float64, bool) {
	if value, ok := financialCumulativeRevenue(target); ok {
		return value, true
	}
	if len(target.TaxPeriod.End) < 10 {
		return 0, false
	}
	facts := []financialFact{}
	seen := map[string]financialFact{}
	for _, f := range ctx.financial {
		if f.TaxPeriod.Cycle != target.TaxPeriod.Cycle || f.TaxPeriod.Start < target.TaxPeriod.End[:4]+"-01-01" || f.TaxPeriod.End > target.TaxPeriod.End {
			continue
		}
		key := f.TaxPeriod.Start + "|" + f.TaxPeriod.End
		if prior, exists := seen[key]; exists {
			if prior.HasRevenue != f.HasRevenue || prior.Revenue != f.Revenue {
				return 0, false
			}
			continue
		}
		seen[key] = f
		facts = append(facts, f)
	}
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].TaxPeriod.Start < facts[j].TaxPeriod.Start })
	next, sum := target.TaxPeriod.End[:4]+"-01-01", 0.0
	for _, fact := range facts {
		if fact.TaxPeriod.Cycle != target.TaxPeriod.Cycle || fact.TaxPeriod.End > target.TaxPeriod.End {
			continue
		}
		if fact.TaxPeriod.Start != next || !fact.HasRevenue {
			return 0, false
		}
		end, err := time.Parse("2006-01-02", fact.TaxPeriod.End)
		if err != nil {
			return 0, false
		}
		sum += fact.Revenue
		if fact.TaxPeriod.End == target.TaxPeriod.End {
			return sum, true
		}
		next = end.AddDate(0, 0, 1).Format("2006-01-02")
	}
	return 0, false
}

// yearToDateReferenceModule keeps reference records outside ordinary sections.
func yearToDateReferenceModule(module ModuleSnapshot) ModuleSnapshot {
	if root, ok := module.Raw.(map[string]any); ok {
		if reference, ok := root["yearToDateReference"].(map[string]any); ok {
			return ModuleSnapshot{Raw: reference}
		}
	}
	return ModuleSnapshot{}
}

// quarterlyCITMetricForContext uses earlier declarations only to restore the
// requested quarters. Reference quarters must not become report rows.
func quarterlyCITMetricForContext(ctx *reportContext, selector func(citFact) (float64, bool)) []periodValue {
	current := map[string]bool{}
	byPeriod := map[string]citFact{}
	for _, fact := range citFacts(yearToDateReferenceModule(ctx.modules["cit_quarterly_returns"])) {
		byPeriod[fact.Period] = fact
	}
	for _, fact := range ctx.cit {
		current[fact.Period], byPeriod[fact.Period] = true, fact
	}
	facts := make([]citFact, 0, len(byPeriod))
	for _, fact := range byPeriod {
		facts = append(facts, fact)
	}
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].Period < facts[j].Period })
	var result []periodValue
	for _, item := range quarterlyCITMetric(facts, selector) {
		if current[item.period] {
			result = append(result, item)
		}
	}
	return result
}

// vatCumulativeSales uses an explicit cumulative column when present. New VAT
// forms require a contiguous, nonoverlapping sequence from January 1; a missing
// declaration cannot silently become zero or a partial year total.
func vatCumulativeSales(ctx *reportContext, period TaxPeriod) (float64, bool) {
	if len(period.End) < 10 {
		return 0, false
	}
	facts := append(append([]vatFact(nil), ctx.vat...), vatFacts(yearToDateReferenceModule(ctx.modules["vat_returns"]))...)
	start := period.End[:4] + "-01-01"
	for _, fact := range facts {
		if fact.TaxPeriod.End == period.End && fact.HasCumulativeSales {
			return fact.CumulativeSales, true
		}
	}
	facts = dedupeVATFacts(facts)
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].TaxPeriod.Start < facts[j].TaxPeriod.Start })
	next, total := start, 0.0
	for _, fact := range facts {
		if fact.TaxPeriod.Start < start || fact.TaxPeriod.End > period.End {
			continue
		}
		if fact.TaxPeriod.Start != next || !fact.HasSales {
			return 0, false
		}
		end, err := time.Parse("2006-01-02", fact.TaxPeriod.End)
		if err != nil {
			return 0, false
		}
		total += fact.Sales
		next = end.AddDate(0, 0, 1).Format("2006-01-02")
		if fact.TaxPeriod.End == period.End {
			return total, true
		}
	}
	return 0, false
}
