package fetchrisk

import (
	"math"
	"sort"
	"strings"
)

func addMoneyAgg(agg *moneyAgg, amount, tax, total float64) {
	agg.Count++
	agg.Amount += amount
	agg.Tax += tax
	agg.Total += total
}

func addTypeAgg(stats map[string]*moneyAgg, invoiceType string, amount, tax, total float64) {
	key := strings.TrimSpace(invoiceType)
	if key == "" {
		key = "未知票种"
	}
	if stats[key] == nil {
		stats[key] = &moneyAgg{}
	}
	addMoneyAgg(stats[key], amount, tax, total)
}

func addMonthlyAgg(monthly map[string]map[string]*moneyAgg, month, direction string, amount, tax, total float64) {
	if monthly[month] == nil {
		monthly[month] = map[string]*moneyAgg{"input": {}, "output": {}, "unknown": {}}
	}
	addMoneyAgg(monthly[month][direction], amount, tax, total)
}

func buildMonthlyDirectionStats(monthly map[string]map[string]*moneyAgg) []any {
	months := make([]string, 0, len(monthly))
	for month := range monthly {
		months = append(months, month)
	}
	sort.Strings(months)
	out := make([]any, 0, len(months))
	for _, month := range months {
		stats := monthly[month]
		out = append(out, map[string]any{
			"month":   month,
			"input":   moneyAggMap(stats["input"]),
			"output":  moneyAggMap(stats["output"]),
			"unknown": moneyAggMap(stats["unknown"]),
		})
	}
	return out
}

func buildRiskTypeStats(stats map[string]map[string]*moneyAgg) map[string]any {
	out := map[string]any{}
	for _, direction := range []string{"input", "output", "unknown"} {
		items := make([]any, 0, len(stats[direction]))
		for invoiceType, agg := range stats[direction] {
			row := moneyAggMap(agg)
			row["invoice_type"] = invoiceType
			items = append(items, row)
		}
		sort.Slice(items, func(i, j int) bool {
			left := items[i].(map[string]any)
			right := items[j].(map[string]any)
			if floatAny(left["amount"]) == floatAny(right["amount"]) {
				return textAny(left["invoice_type"]) < textAny(right["invoice_type"])
			}
			return floatAny(left["amount"]) > floatAny(right["amount"])
		})
		out[direction] = items
	}
	return out
}

func topRiskExamples(rows []riskContextRow, limit int) []any {
	sort.Slice(rows, func(i, j int) bool {
		return math.Abs(rows[i].Total) > math.Abs(rows[j].Total)
	})
	out := []any{}
	for _, row := range rows {
		out = appendRiskExample(out, row, "大额发票样本")
		if len(out) >= limit {
			break
		}
	}
	return out
}

func appendRiskExample(items []any, row riskContextRow, reason string) []any {
	if len(items) >= 10 {
		return items
	}
	return append(items, map[string]any{
		"reason":       reason,
		"file_id":      row.FileID,
		"row_index":    row.RowIndex,
		"direction":    normalizedRiskDirection(row.Direction),
		"invoice_no":   row.InvoiceNo,
		"counterparty": riskCounterparty(row),
		"amount":       round2(row.Amount),
		"tax":          round2(row.Tax),
		"total":        round2(row.Total),
	})
}

func riskCounterparty(row riskContextRow) string {
	if normalizedRiskDirection(row.Direction) == "output" {
		return row.BuyerName
	}
	return row.SellerName
}
