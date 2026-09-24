package report

import (
	"sort"
	"strings"
	"time"
)

// financialVATPeriodValues compares normalized current-period financial revenue
// with VAT sales over those exact dates. YTD reference records never become rows.
func financialVATPeriodValues(ctx *reportContext, f financialFact) (float64, bool, float64, bool) {
	p := f.TaxPeriod
	if p.Start == "" || p.End == "" {
		p.Start, p.End = periodBounds(f.Period)
	}
	v, ok := vatSalesWithinPeriod(ctx.vat, p)
	return f.Revenue, f.HasRevenue, v, ok
}

// vatSalesWithinPeriod sums only complete, adjacent declarations. It never
// prorates a quarterly declaration to a month or substitutes a YTD amount.
func vatSalesWithinPeriod(facts []vatFact, period TaxPeriod) (float64, bool) {
	if period.Start == "" || period.End < period.Start {
		return 0, false
	}
	ordered := append([]vatFact(nil), facts...)
	for i := range ordered {
		if ordered[i].TaxPeriod.Start == "" || ordered[i].TaxPeriod.End == "" {
			ordered[i].TaxPeriod.Start, ordered[i].TaxPeriod.End = periodBounds(ordered[i].Period)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].TaxPeriod.Start < ordered[j].TaxPeriod.Start })
	next, total := period.Start, 0.0
	for _, fact := range ordered {
		p := fact.TaxPeriod
		if p.Start == "" || p.End == "" {
			p.Start, p.End = periodBounds(fact.Period)
		}
		if p.End < period.Start || p.Start > period.End {
			continue
		}
		if p.Start != next || p.End > period.End || !fact.HasSales {
			return 0, false
		}
		end, err := time.Parse("2006-01-02", p.End)
		if err != nil {
			return 0, false
		}
		total += fact.Sales
		next = end.AddDate(0, 0, 1).Format("2006-01-02")
	}
	end, err := time.Parse("2006-01-02", period.End)
	return total, err == nil && next == end.AddDate(0, 0, 1).Format("2006-01-02")
}

func financialVATAnnual(f financialFact) bool {
	return f.TaxPeriod.Cycle == taxCycleAnnual || strings.Contains(f.Period, "年度")
}
