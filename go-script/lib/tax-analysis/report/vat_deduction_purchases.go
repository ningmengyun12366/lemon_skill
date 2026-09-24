package report

import (
	"encoding/json"
	"strings"
)

// certifiedPurchasesForPeriod reads the confirmation month, never issue date.
// The module list is already deduplicated upstream; chunks are used solely to
// prove month coverage, not summed again with the same invoices.
func certifiedPurchasesForPeriod(ctx *reportContext, period TaxPeriod) (float64, float64, bool, bool) {
	module := ctx.modules["deduction_confirmations"]
	summary := readDeductionSummary(module.Raw)
	var records []json.RawMessage
	present := len(summary.Chunks) > 0
	for _, chunk := range summary.Chunks {
		if chunk.List == nil {
			present = false
			continue
		}
		records = append(records, chunk.List.Purchases...)
	}
	if !present {
		return 0, 0, false, false
	}
	covered := module.Complete && period.Start >= ctx.task.StartTime && period.End <= ctx.task.EndTime
	if !covered {
		chunks := summary.Chunks
		months := map[string]bool{}
		for _, chunk := range chunks {
			months[extractMonth(textOr(chunk.StartTime, chunk.Month))] = true
		}
		covered = true
		for _, month := range completeTrendPeriods(period.Start, period.End, false) {
			covered = covered && months[month]
		}
		covered = covered && len(months) > 0
	}
	amount, tax, amountKnown, taxKnown := 0.0, 0.0, covered, covered
	seen := map[string]bool{}
	for _, record := range records {
		var tuple []json.RawMessage
		if json.Unmarshal(record, &tuple) != nil || len(tuple) != 4 {
			amountKnown, taxKnown = false, false
			continue
		}
		var month, id string
		var a, t *float64
		_ = json.Unmarshal(tuple[0], &month)
		_ = json.Unmarshal(tuple[1], &id)
		_ = json.Unmarshal(tuple[2], &a)
		_ = json.Unmarshal(tuple[3], &t)
		if month == "" {
			amountKnown, taxKnown = false, false
			continue
		}
		if !monthWithinAnalysisPeriod(month, period.Start, period.End) {
			continue
		}
		key := month + "|" + id
		if strings.Trim(id, ":0") != "" && seen[key] {
			continue
		}
		seen[key] = true
		if a != nil {
			amount += *a
		} else {
			amountKnown = false
		}
		if t != nil {
			tax += *t
		} else {
			taxKnown = false
		}
	}
	return amount, tax, amountKnown, taxKnown
}
