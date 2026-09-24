package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// TaxPeriod is the normalized filing or payment period carried from the tax
// authority response. Start and End are inclusive full dates. BucketMonth is
// always the month containing End and is used only for the report's monthly
// matrices and charts.
type TaxPeriod struct {
	Start           string `json:"start"`
	End             string `json:"end"`
	BucketMonth     string `json:"bucketMonth"`
	Cycle           string `json:"cycle"`
	Display         string `json:"display"`
	DeclarationDate string `json:"declarationDate,omitempty"`
	PaymentDate     string `json:"paymentDate,omitempty"`
	Basis           string `json:"basis"`
}

// filingFact is one effective filing or declaration normalized for report use.
type filingFact struct {
	RecordID      string
	Period        TaxPeriod
	TaxName       string
	Status        string
	Declared      float64
	HasDeclared   bool
	SourceModule  string
	EffectiveOnly bool
}

// taxPeriodFromRecord normalizes one tax-return or payment record.
//
// Parameters: object is one expanded response record and paymentFallback
// controls whether a missing tax period may fall back to the payment date.
// Returns: the normalized period and true when a usable period was found.
func taxPeriodFromRecord(object map[string]any, paymentFallback bool) (TaxPeriod, bool) {
	start := normalizeReportDate(mapText(object,
		"skssqq", "taxPeriodStart", "periodStart", "ssqq", "startPeriod", "startDate"), false)
	end := normalizeReportDate(mapText(object,
		"skssqz", "taxPeriodEnd", "periodEnd", "ssqz", "endPeriod", "endDate"), true)
	periodText := recordPeriod(object)
	if start == "" && end == "" && periodText != "" {
		start, end = periodBounds(periodText)
	}
	declarationDate := normalizeReportDate(mapText(object,
		"nssbrq", "sbrq", "declarationDate", "filingDate", "declareDate", "submittedAt"), false)
	paymentDate := normalizeReportDate(mapText(object,
		"jkrq", "paymentDate", "paidDate", "paymentTime", "paidAt"), false)
	basis := periodBasisTaxPeriod
	if start == "" && end == "" && paymentFallback && paymentDate != "" {
		start, end = paymentDate, paymentDate
		basis = periodBasisPaymentDate
	}
	if start == "" && end != "" {
		start = end
	}
	if end == "" && start != "" {
		end = start
	}
	if start == "" || end == "" {
		return TaxPeriod{}, false
	}
	if start > end {
		start, end = end, start
	}
	cycle := filingCycle(start, end, periodText, mapText(object,
		"filingCycle", "declarationCycle", "sbyf", "sblxmc", "sbzqmc", "periodType", "cycle"))
	bucket := extractMonth(end)
	if bucket == "" {
		return TaxPeriod{}, false
	}
	return TaxPeriod{
		Start: start, End: end, BucketMonth: bucket, Cycle: cycle,
		Display: taxPeriodDisplay(start, end, cycle), DeclarationDate: declarationDate,
		PaymentDate: paymentDate, Basis: basis,
	}, true
}

// periodBounds converts a compact month, quarter, year, or date value to an
// inclusive full-date range.
//
// Parameter value is the source period text. Returns are empty when the value
// cannot be recognized.
func periodBounds(value string) (string, string) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", ""
	}
	for _, separator := range []string{"—", "~", "～", "–"} {
		if from, to, ok := strings.Cut(text, separator); ok {
			a, _ := periodBounds(from)
			_, b := periodBounds(to)
			if a != "" && b != "" && a <= b {
				return a, b
			}
			return "", ""
		}
	}
	if from, to, ok := strings.Cut(text, "至"); ok {
		start := normalizeReportDate(from, false)
		end := normalizeReportDate(to, true)
		if start != "" && end != "" && start <= end {
			return start, end
		}
		return "", ""
	}
	if start, end := quarterPeriodBounds(text); start != "" && end != "" {
		return start, end
	}
	yearText := strings.TrimSpace(strings.NewReplacer("年度", "", "年", "", " ", "").Replace(text))
	if len(yearText) == 4 {
		if _, err := time.Parse("2006", yearText); err == nil {
			return yearText + "-01-01", yearText + "-12-31"
		}
	}
	upper := strings.ToUpper(strings.NewReplacer("年", "-", "月", "", "季度", "Q", "季", "Q", " ", "").Replace(text))
	if month := extractMonth(upper); month != "" && (len(upper) == 6 || len(upper) == 7) {
		start := month + "-01"
		parsed, err := time.Parse("2006-01-02", start)
		if err == nil {
			return start, parsed.AddDate(0, 1, -1).Format("2006-01-02")
		}
	}
	if date := normalizeReportDate(text, false); len(date) == 10 {
		return date, date
	}
	return "", ""
}

// quarterPeriodBounds converts common Chinese and compact quarter labels to
// inclusive full dates. Parameter value may contain forms such as 2025Q1,
// 2025年第1季度, or 2025-1季. Returns are empty for an invalid label.
func quarterPeriodBounds(value string) (string, string) {
	compact := strings.ToUpper(strings.NewReplacer(
		"年", "", "季度", "Q", "季", "Q", "第", "", "一", "1", "二", "2", "三", "3", "四", "4", "-", "", "/", "", " ", "",
	).Replace(strings.TrimSpace(value)))
	if len(compact) < 6 || !strings.Contains(compact, "Q") {
		return "", ""
	}
	year := compact[:4]
	if _, err := time.Parse("2006", year); err != nil {
		return "", ""
	}
	quarter := 0
	for _, character := range compact[4:] {
		if character >= '1' && character <= '4' {
			quarter = int(character - '0')
			break
		}
	}
	if quarter < quarterMinimum || quarter > quarterMaximum {
		return "", ""
	}
	startMonth := (quarter-quarterMinimum)*monthsPerQuarter + firstMonthOfYear
	start := fmt.Sprintf("%s-%02d-01", year, startMonth)
	startTime, err := time.Parse("2006-01-02", start)
	if err != nil {
		return "", ""
	}
	return start, startTime.AddDate(0, monthsPerQuarter, -1).Format("2006-01-02")
}

// normalizeReportDate returns a stable YYYY-MM-DD value. When endOfMonth is
// true, a source month is normalized to its last natural day. Complete source
// dates retain their day, including tax-authority timestamps without a zone.
func normalizeReportDate(value string, endOfMonth bool) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	text = strings.NewReplacer("年", "-", "月", "-", "日", "", "/", "-").Replace(text)
	text = strings.TrimSuffix(text, "-")
	for _, layout := range []string{time.DateOnly, "20060102", time.DateTime, "2006-01-02T15:04:05", time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	month := extractMonth(text)
	if month == "" {
		return ""
	}
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		return ""
	}
	if endOfMonth {
		return parsed.AddDate(0, 1, -1).Format("2006-01-02")
	}
	return parsed.Format("2006-01-02")
}

// filingCycle resolves the real filing cycle from explicit tax data, then from
// the inclusive period length. Parameters are normalized bounds plus source
// period/cycle text. Return value is one stable Chinese cycle label.
func filingCycle(start, end, periodText, explicit string) string {
	value := strings.ToLower(strings.TrimSpace(explicit))
	switch {
	case containsAny(value, "不定期", "按次", "irregular"):
		return taxCycleIrregular
	case containsAny(value, "按日", "日申报", "daily"):
		return taxCycleDaily
	case containsAny(value, "年", "annual", "year"):
		return taxCycleAnnual
	case containsAny(value, "季", "quarter") || strings.Contains(strings.ToUpper(periodText), "Q"):
		return taxCycleQuarterly
	case containsAny(value, "月", "monthly"):
		return taxCycleMonthly
	}
	startDate, startErr := time.Parse("2006-01-02", start)
	endDate, endErr := time.Parse("2006-01-02", end)
	if startErr != nil || endErr != nil {
		return taxCycleIrregular
	}
	days := int(endDate.Sub(startDate).Hours()/24) + 1
	switch {
	case days <= 1:
		return taxCycleDaily
	case days >= 360:
		return taxCycleAnnual
	case days >= 80 && days <= 95:
		return taxCycleQuarterly
	case days >= 28 && days <= 31:
		return taxCycleMonthly
	default:
		return taxCycleIrregular
	}
}

// taxPeriodDisplay formats a normalized inclusive period. Parameters are full
// dates and the resolved cycle. Return value preserves complete non-monthly
// bounds while shortening ordinary monthly or annual periods.
func taxPeriodDisplay(start, end, cycle string) string {
	if start == end {
		return start
	}
	if cycle == taxCycleMonthly && extractMonth(start) == extractMonth(end) {
		return extractMonth(end)
	}
	if cycle == taxCycleAnnual && len(start) >= 4 && start[:4] == end[:4] {
		return start[:4] + "年度"
	}
	if cycle == taxCycleQuarterly {
		startDate, startErr := time.Parse("2006-01-02", start)
		endDate, endErr := time.Parse("2006-01-02", end)
		if startErr == nil && endErr == nil && startDate.Year() == endDate.Year() &&
			startDate.Day() == 1 && startDate.Month() == time.Month((int(endDate.Month())-1)/monthsPerQuarter*monthsPerQuarter+1) &&
			endDate.Equal(startDate.AddDate(0, monthsPerQuarter, -1)) {
			quarter := (int(startDate.Month())-1)/monthsPerQuarter + quarterMinimum
			return fmt.Sprintf("%d年第%d季度", startDate.Year(), quarter)
		}
	}
	return start + " 至 " + end
}

// filingFacts extracts actual filed records. Parameters: module is the filing
// snapshot. Return value prefers effectiveRecords, then the actual OpenTax
// declaredRecords collection; undeclared and todo collections are excluded.
func filingFacts(module ModuleSnapshot) []filingFact {
	records, effectiveOnly := namedCollectionMapsWithPresence(module.Raw, "effectiveRecords")
	if !effectiveOnly {
		var declaredPresent bool
		records, declaredPresent = namedCollectionMapsWithPresence(module.Raw, "declaredRecords")
		effectiveOnly = declaredPresent
	}
	result := make([]filingFact, 0, len(records))
	seen := map[string]bool{}
	for _, object := range records {
		period, ok := taxPeriodFromRecord(object, false)
		if !ok {
			continue
		}
		name := taxNameFromRecord(object, "")
		if name == "" {
			continue
		}
		amount, amountOK := declaredAmount(object)
		status := strings.TrimSpace(mapText(object, "status", "declarationStatus", "sbztMc", "sbbzMc", "申报状态"))
		key := strings.Join([]string{name, period.Start, period.End, period.Cycle, status, optionalMoney(amount, amountOK)}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, filingFact{RecordID: mapText(object, "sbuuid"), Period: period, TaxName: name, Status: status, Declared: amount, HasDeclared: amountOK, SourceModule: module.Code, EffectiveOnly: effectiveOnly})
	}
	sortFilingFacts(result)
	return result
}

// declarationFacts combines effective filing records with the actual return
// modules so declared amounts can be shown without guessing from obligations.
func declarationFacts(modules map[string]ModuleSnapshot, startTime, endTime string) []filingFact {
	result := filingFacts(modules["filing_obligations"])
	moduleNames := map[string]string{
		"vat_returns": "增值税", "cit_quarterly_returns": "企业所得税",
		"cit_annual_returns": "企业所得税", "stamp_tax": "印花税",
		"property_tax": "房产税", "urban_land_use_tax": "城镇土地使用税",
	}
	for moduleCode, fallbackTaxName := range moduleNames {
		module := modules[moduleCode]
		records, effectiveRecordsPresent := namedCollectionMapsWithPresence(module.Raw, "effectiveRecords")
		for _, object := range records {
			period, ok := taxPeriodFromRecord(object, false)
			if !ok || !monthWithinAnalysisPeriod(period.BucketMonth, startTime, endTime) {
				continue
			}
			amount, amountOK := declaredAmount(object)
			if !amountOK && !effectiveRecordsPresent {
				continue
			}
			result = append(result, filingFact{
				RecordID: mapText(object, "sbuuid"),
				Period:   period, TaxName: taxNameFromRecord(object, fallbackTaxName),
				Status:   strings.TrimSpace(mapText(object, "status", "declarationStatus", "sbztMc")),
				Declared: amount, HasDeclared: amountOK, SourceModule: moduleCode, EffectiveOnly: effectiveRecordsPresent,
			})
		}
	}
	result = dedupeFilingFacts(result, startTime, endTime)
	sortFilingFacts(result)
	return result
}

// declaredAmount reads an explicitly returned declaration amount. Parameter
// object is one expanded tax record. Returns are the value and presence flag;
// missing never becomes zero.
func declaredAmount(object map[string]any) (float64, bool) {
	if len(taxProjectCodes(object)) > 1 || containsAny(mapText(object, "yzpzzlMc"), "增值税及附加") {
		return 0, false
	}
	return mapNumber(object,
		"declaredTaxAmount", "taxPayable", "payableTax", "ynse", "ybtse", "应纳税额",
		"actualTaxAmount", "taxAmount", "sbse", "declarationAmount")
}

// taxNameFromRecord resolves the tax name from a record. Parameters are the
// expanded source record and a module-level fallback. Return value is the
// canonical Chinese tax name.
func taxNameFromRecord(object map[string]any, fallback string) string {
	if name, known := taxCodeName(object); known {
		return name
	}
	return normalizeTaxName(textOr(mapText(object,
		"zsxmMc", "taxName", "taxTypeName", "taxItemName", "szMc", "征收项目"), fallback))
}

// normalizeTaxName maps tax-bureau label variants to stable report names.
// Parameter value is the returned label. Return value preserves unknown names.
func normalizeTaxName(value string) string {
	name := strings.TrimSpace(value)
	switch {
	case containsAny(name, "企业所得税"):
		return "企业所得税"
	case containsAny(name, "增值税"):
		return "增值税"
	case containsAny(name, "城镇土地使用税"):
		return "城镇土地使用税"
	case containsAny(name, "房产税"):
		return "房产税"
	case containsAny(name, "印花税"):
		return "印花税"
	case containsAny(name, "城市维护建设税"):
		return "城市维护建设税"
	case containsAny(name, "地方教育附加"):
		return "地方教育附加"
	case containsAny(name, "教育费附加"):
		return "教育费附加"
	default:
		return name
	}
}

// namedCollectionMapsWithPresence reads one explicit top-level JSON collection.
// Parameters are the decoded root and collection name. Returns distinguish an
// explicitly empty collection from a missing collection. It intentionally does
// not recurse into nested objects, so similarly named detail collections cannot
// be mistaken for the module-level record contract.
func namedCollectionMapsWithPresence(root any, name string) ([]map[string]any, bool) {
	object, ok := root.(map[string]any)
	if !ok {
		return nil, false
	}
	value, found := mapValue(object, name)
	if !found {
		return nil, false
	}
	values, ok := value.([]any)
	if !ok {
		return nil, true
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			result = append(result, expandBusinessObject(object))
		}
	}
	return result, true
}

// dedupeFilingFacts merges duplicate module facts without losing a known
// amount or status. Parameters are candidate facts and analysis bounds. Return
// value contains only in-range unique effective facts.
func dedupeFilingFacts(values []filingFact, startTime, endTime string) []filingFact {
	result := make([]filingFact, 0, len(values))
	seen := map[string]int{}
	for _, value := range values {
		if !monthWithinAnalysisPeriod(value.Period.BucketMonth, startTime, endTime) {
			continue
		}
		key := strings.Join([]string{value.TaxName, value.Period.Start, value.Period.End, value.Period.Cycle}, "\x00")
		if existing, ok := seen[key]; ok {
			current := &result[existing]
			if !current.HasDeclared && value.HasDeclared {
				current.Declared, current.HasDeclared = value.Declared, true
				current.SourceModule = value.SourceModule
			}
			if current.Status == "" && value.Status != "" {
				current.Status = value.Status
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, value)
	}
	return result
}

// sortFilingFacts orders facts deterministically by period, tax and module.
// Parameter values is modified in place. The function has no return value.
func sortFilingFacts(values []filingFact) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Period.End != values[j].Period.End {
			return values[i].Period.End < values[j].Period.End
		}
		if values[i].TaxName != values[j].TaxName {
			return values[i].TaxName < values[j].TaxName
		}
		return values[i].SourceModule < values[j].SourceModule
	})
}
