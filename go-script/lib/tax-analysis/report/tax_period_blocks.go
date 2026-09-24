package report

import (
	"sort"
	"strings"
)

// buildFilingCycleMatrix renders the actual effective tax type by filing-cycle
// matrix. It never turns a missing filing record into a zero value.
func buildFilingCycleMatrix(ctx *reportContext) []Block {
	byTax := map[string]map[string][]string{}
	for _, fact := range ctx.filings {
		if !monthWithinAnalysisPeriod(fact.Period.BucketMonth, ctx.task.StartTime, ctx.task.EndTime) {
			continue
		}
		cycle := fact.Period.Cycle
		if cycle == taxCycleDaily || cycle == taxCycleIrregular {
			cycle = taxCycleDaily + "/" + taxCycleIrregular
		}
		if byTax[fact.TaxName] == nil {
			byTax[fact.TaxName] = map[string][]string{}
		}
		byTax[fact.TaxName][cycle] = appendUnique(byTax[fact.TaxName][cycle], fact.Period.Display)
	}
	if len(byTax) == 0 && len(taxRecognitions(ctx)) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"本分析期间未取得可展示的有效申报记录；更正、作废历史不作为当前申报周期。"}}}
	}
	taxNames := make([]string, 0, len(byTax))
	for name := range byTax {
		taxNames = append(taxNames, name)
	}
	sort.Strings(taxNames)
	rows := make([][]string, 0, len(taxNames))
	for _, name := range taxNames {
		cycles := make([]string, 0, len(byTax[name]))
		periods := make([]string, 0)
		for _, cycle := range []string{taxCycleMonthly, taxCycleQuarterly, taxCycleAnnual, taxCycleDaily + "/" + taxCycleIrregular} {
			if len(byTax[name][cycle]) == 0 {
				continue
			}
			cycles = append(cycles, cycle)
			displays := make([]string, 0, len(byTax[name][cycle]))
			for _, display := range byTax[name][cycle] {
				displays = append(displays, filingCycleMatrixDisplay(display))
			}
			periods = append(periods, displays...)
		}
		rows = append(rows, []string{name, textOr(strings.Join(cycles, "、"), "—"), "申报期间、申报税额及缴款情况"})
	}
	rows = appendRecognitionCycleRows(ctx, rows)
	return []Block{{
		Kind: BlockTable, Title: "实际税费种与申报周期",
		Headers: []string{"税费种", "申报周期", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "分析内容"}, Rows: rows,
	}}
}

// filingCycleMatrixDisplay keeps complete dates on separate table lines.
// Parameter display is the normalized reporting-period label. Return value is
// safe for the narrow cycle-matrix columns and never asks a renderer to split a
// YYYY-MM-DD token in the middle.
func filingCycleMatrixDisplay(display string) string {
	parts := strings.Split(strings.TrimSpace(display), " 至 ")
	if len(parts) != 2 || len(parts[0]) != len("2006-01-02") || len(parts[1]) != len("2006-01-02") {
		return display
	}
	return parts[0] + "\n至\n" + parts[1]
}

// monthlyTaxCell collects known payment facts for one actual
// tax-authority period and one fixed Word tax column. Presence flags preserve
// missing-versus-zero semantics.
type monthlyTaxCell struct {
	paid      float64
	paidKnown bool
}

// buildTaxPaymentMatrix builds the Word tax-payment matrix by the actual tax
// authority reporting period. Monthly, quarterly, annual, daily and irregular
// records are placed at the period end; annual settlements remain separate.
// The complete month axis preserves unknown cells instead of inventing zeroes.
func buildTaxPaymentMatrix(ctx *reportContext) []Block {
	months := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, false)
	monthly, annual := map[string][]*monthlyTaxCell{}, map[string][]*monthlyTaxCell{}
	for _, month := range months {
		monthly[month] = make([]*monthlyTaxCell, 7)
	}
	cellFor := func(period TaxPeriod, column int) *monthlyTaxCell {
		target, key := monthly, period.BucketMonth
		if period.Cycle == taxCycleAnnual {
			target, key = annual, period.Display
		}
		if key == "" {
			return nil
		}
		if target[key] == nil {
			target[key] = make([]*monthlyTaxCell, 7)
		}
		if target[key][column] == nil {
			target[key][column] = &monthlyTaxCell{}
		}
		return target[key][column]
	}
	for _, fact := range ctx.payments {
		column := taxPaymentColumn(fact.TaxName)
		if column < 0 || !monthWithinAnalysisPeriod(fact.TaxPeriod.BucketMonth, ctx.task.StartTime, ctx.task.EndTime) {
			continue
		}
		cell := cellFor(fact.TaxPeriod, column)
		if cell != nil {
			cell.paid += fact.Amount
			cell.paidKnown = true
		}
	}
	headers := []string{"所属期", "增值税", "附加税费", "企业所得税", "印花税", "房产税", "土地使用税", "工会经费", "合计"}
	build := func(keys []string, cellsByPeriod map[string][]*monthlyTaxCell, title string, totalRow bool) Block {
		rows, totals := [][]string{}, make([]*monthlyTaxCell, 7)
		for _, key := range keys {
			row, sum := []string{key}, &monthlyTaxCell{}
			for column, cell := range cellsByPeriod[key] {
				row = append(row, textOr(formatTaxPaymentCell(cell), "—"))
				if cell == nil {
					continue
				}
				if totals[column] == nil {
					totals[column] = &monthlyTaxCell{}
				}
				for _, target := range []*monthlyTaxCell{sum, totals[column]} {
					target.paid += cell.paid
					target.paidKnown = target.paidKnown || cell.paidKnown
				}
			}
			row = append(row, textOr(formatTaxPaymentCell(sum), "—"))
			rows = append(rows, row)
		}
		if totalRow {
			row, sum := []string{"合计（已取得金额）"}, &monthlyTaxCell{}
			for _, cell := range totals {
				row = append(row, textOr(formatTaxPaymentCell(cell), "—"))
				if cell != nil {
					sum.paid += cell.paid
					sum.paidKnown = sum.paidKnown || cell.paidKnown
				}
			}
			row = append(row, textOr(formatTaxPaymentCell(sum), "—"))
			rows = append(rows, row)
		}
		return Block{Kind: BlockTable, Title: title + "（单位：元）", Headers: headers, Rows: rows}
	}
	blocks := []Block{build(months, monthly, "税费缴纳月度汇总", true)}
	if len(annual) > 0 {
		blocks = append(blocks, build(sortedMapKeys(annual), annual, "年度汇算单列（不计入月度合计）", false))
	}
	return append(blocks, Block{Kind: BlockParagraph, Title: "口径说明", Paragraphs: []string{"月报归入所属月份，季报归入季度末月份；年度汇算单列。金额均为实缴额，未取得缴款记录不等于未缴税。缺少税款所属期的缴款仅按缴款月份列示，不作为同属期缴纳一致的依据。"}})
}

// taxPaymentColumn maps one normalized tax name to the fixed Word table
// column. Parameter name is the tax-bureau tax name; the return value is the
// zero-based data-column index or -1 when Word has no dedicated column.
func taxPaymentColumn(name string) int {
	switch {
	case containsAny(name, "增值税"):
		return 0
	case containsAny(name, "城市维护建设税", "教育费附加", "地方教育附加", "附加税"):
		return 1
	case containsAny(name, "企业所得税"):
		return 2
	case containsAny(name, "印花税"):
		return 3
	case containsAny(name, "房产税"):
		return 4
	case containsAny(name, "城镇土地使用税", "土地使用税"):
		return 5
	case containsAny(name, "工会经费"):
		return 6
	default:
		return -1
	}
}

// formatTaxPaymentCell formats one tax-period cell without inventing zeroes.
// Parameter cell contains normalized payments. Return value is the paid amount
// in yuan with two decimals; an unknown payment returns an empty string.
func formatTaxPaymentCell(cell *monthlyTaxCell) string {
	if cell == nil || !cell.paidKnown {
		return ""
	}
	return taxMatrixMoneyText(cell.paid)
}

// taxMatrixMoneyText keeps the Word nine-column tax matrix readable while
// preserving its value. Parameter value is an amount in yuan. Return value
// uses yuan with two decimals throughout, preserving cent-level differences.
func taxMatrixMoneyText(value float64) string {
	return moneyText(value)
}

// optionalTaxMatrixMoney formats one known tax-matrix total. Parameters are
// the value and its presence flag. Return value keeps missing data as a dash.
func optionalTaxMatrixMoney(value float64, known bool) string {
	if !known {
		return "—"
	}
	return taxMatrixMoneyText(value)
}

// splitMonthlyPeriodsByNaturalYear groups a complete month axis by natural
// year. Parameters are ordered months and whether splitting is required.
// Return value always contains at least one group.
func splitMonthlyPeriodsByNaturalYear(periods []string, split bool) [][]string {
	if !split || len(periods) == 0 {
		return [][]string{periods}
	}
	groups := make([][]string, 0)
	currentYear := ""
	for _, period := range periods {
		year := period
		if len(period) >= 4 {
			year = period[:4]
		}
		if len(groups) == 0 || year != currentYear {
			groups = append(groups, []string{})
			currentYear = year
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], period)
	}
	return groups
}

// chartYearSuffix derives a stable chart ID suffix and visible title prefix.
// Parameter periods is one natural-year month group. Returns are empty when no
// four-digit year is available.
func chartYearSuffix(periods []string) (string, string) {
	if len(periods) == 0 || len(periods[0]) < 4 {
		return "", ""
	}
	year := periods[0][:4]
	return "-" + year, year + "年"
}

// monthlyPaymentByTax aggregates actual payments by tax and month. Parameter
// facts contains normalized payment records. Returns values and presence maps.
func monthlyPaymentByTax(facts []paymentFact) (map[string]map[string]float64, map[string]map[string]bool) {
	values := map[string]map[string]float64{}
	known := map[string]map[string]bool{}
	for _, fact := range facts {
		month, usable := paymentTrendMonth(fact)
		if !usable {
			continue
		}
		name := textOr(fact.TaxName, "其他税费")
		if values[name] == nil {
			values[name] = map[string]float64{}
			known[name] = map[string]bool{}
		}
		values[name][month] += fact.Amount
		known[name][month] = true
	}
	return values, known
}

// majorTaxNames ranks tax names by paid amount. Parameters are payment facts
// and the positive maximum count. Return value is deterministic on ties.
func majorTaxNames(facts []paymentFact, limit int) []string {
	totals := map[string]float64{}
	for _, fact := range facts {
		totals[textOr(fact.TaxName, "其他税费")] += fact.Amount
	}
	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		if totals[names[i]] != totals[names[j]] {
			return totals[names[i]] > totals[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > limit {
		names = names[:limit]
	}
	return names
}

// monthlyChartSeries builds chart series without converting unknown points to
// zero. Parameters provide months, tax names, values and presence. Return value
// uses the shared component-library palette.
func monthlyChartSeries(periods []string, names []string, values map[string]map[string]float64, known map[string]map[string]bool) []Series {
	colors := chartSeriesPalette
	series := make([]Series, 0, len(names))
	for index, name := range names {
		items := make([]float64, len(periods))
		available := make([]bool, len(periods))
		for periodIndex, period := range periods {
			items[periodIndex] = values[name][period]
			available[periodIndex] = known[name][period]
		}
		series = append(series, Series{Name: name, Values: items, Available: available, Color: colors[index%len(colors)]})
	}
	return series
}

// totalMonthlyPayments aggregates all actual payments by month. Parameter
// facts contains normalized records. Returns totals and presence flags.
func totalMonthlyPayments(facts []paymentFact) (map[string]float64, map[string]bool) {
	values, known := map[string]float64{}, map[string]bool{}
	for _, fact := range facts {
		month, usable := paymentTrendMonth(fact)
		if !usable {
			continue
		}
		values[month] += fact.Amount
		known[month] = true
	}
	return values, known
}

// paymentTrendMonth keeps annual settlements out of monthly and quarterly
// trends. Quarterly payments retain the approved quarter-end bucket. A payment
// date fallback remains visible in amounts but cannot establish a tax burden.
func paymentTrendMonth(fact paymentFact) (string, bool) {
	if fact.TaxPeriod.Cycle == taxCycleAnnual {
		return "", false
	}
	month := textOr(fact.TaxPeriod.BucketMonth, fact.Period)
	return month, extractMonth(month) == month && month != ""
}

// formatMonthlyTaxRows formats one monthly amount series. Parameters are the
// complete axis, values and presence flags. Return value displays missing as —.
func formatMonthlyTaxRows(periods []string, values map[string]float64, known map[string]bool) [][]string {
	rows := make([][]string, len(periods))
	for index, period := range periods {
		rows[index] = []string{period, optionalMoney(values[period], known[period])}
	}
	return rows
}

// paidForBurdenPeriod aligns a selected denominator with actual tax periods.
func paidForBurdenPeriod(facts []paymentFact, start, end string) (float64, float64, bool) {
	self, all, known := 0.0, 0.0, false
	for _, fact := range facts {
		if fact.TaxPeriod.Start >= start && fact.TaxPeriod.End <= end && fact.TaxPeriod.End != "" {
			all += fact.Amount
			known = true
			if fact.SelfPaid {
				self += fact.Amount
			}
		}
	}
	return self, all, known
}
