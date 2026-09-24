package report

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	taxanalysis "invoice-assistant-skill/go-script/lib/tax-analysis"
)

type vatFact struct {
	SupplementTax      float64
	HasSupplementTax   bool
	CumulativeSales    float64
	HasCumulativeSales bool
	RecordID           string
	Period             string
	TaxPeriod          TaxPeriod
	BucketMonth        string
	Cycle              string
	Sales              float64
	HasSales           bool
	TaxPayable         float64
	HasTaxPayable      bool
	OutputTax          float64
	HasOutputTax       bool
	InputTax           float64
	HasInputTax        bool
	EndCredit          float64
	HasEndCredit       bool
}

type paymentFact struct {
	SourceRef string
	Period    string
	TaxPeriod TaxPeriod
	TaxName   string
	Amount    float64
	SelfPaid  bool
}

type citFact struct {
	Period                 string
	CumulativeIncome       float64
	HasIncome              bool
	CumulativeCost         float64
	HasCost                bool
	CumulativeProfit       float64
	HasProfit              bool
	CumulativeActualProfit float64
	HasActualProfit        bool
	CumulativeTax          float64
	HasTax                 bool
	CumulativeResearch     float64
	HasResearch            bool
}

type financialFact struct {
	AccountingStandard      string
	SourceRecordID          string
	CumulativeFlows         map[string]knownValue
	SourceFlows             map[string]knownValue
	ProfitComponents        map[string]*knownValue
	ProfitComponentSigns    map[string]float64
	PreviousCumulativeFlows map[string]knownValue
	PreviousSourceRecordID  string
	CumulativeRevenue       float64
	HasCumulativeRevenue    bool
	TaxPeriod               TaxPeriod
	Balances                map[string]financialBalance
	Period                  string
	Revenue                 float64
	HasRevenue              bool
	Cost                    float64
	HasCost                 bool
	Profit                  float64
	HasProfit               bool
	SalesExpense            float64
	HasSalesExpense         bool
	ManagementExpense       float64
	HasManagementExpense    bool
	ResearchExpense         float64
	HasResearchExpense      bool
	FinanceExpense          float64
	HasFinanceExpense       bool
	NetProfit               float64
	HasNetProfit            bool
	Assets                  float64
	HasAssets               bool
	Liabilities             float64
	HasLiabilities          bool
	AccountsReceivable      float64
	HasAccountsReceivable   bool
	Inventory               float64
	HasInventory            bool
	ContractLiabilities     float64
	HasContractLiabilities  bool
	OperatingCashFlow       float64
	HasOperatingCashFlow    bool
	ResearchInManagement    bool
	ResearchInManagementSet bool
}

type creditGrade struct {
	Year  string
	Grade string
}

type counterpartyItem struct {
	Name   string
	TaxID  string
	Amount float64
	Share  float64
}

type counterpartyAnalysis struct {
	Title string
	Items []counterpartyItem
	Chart *Chart
}

type businessCategory struct {
	Name             string
	Amount           float64
	Share            float64
	Customers        int
	UnknownCustomers bool
	TopOneShare      float64
	Periods          int
}

type ruleSignal struct {
	EvidenceID             string
	CrossDirectionEvidence bool
	Type                   string
	Counterparty           string
	Period                 string
	Evidence               string
	Amount                 float64
	Direct                 bool
}

func taxpayerClassification(module ModuleSnapshot) taxpayerKind {
	kind, _ := taxpayerClassificationWithWarning(module)
	return kind
}

// taxpayerClassificationWithWarning resolves the tax-authority taxpayer type.
// The qualification field nsrzg/Nsrzg is authoritative and follows the actual
// interface contract: 0 means general taxpayer and 1 means small-scale
// taxpayer. Text fields are used only as a fallback or conflict diagnostic.
//
// Parameter module is the taxpayer_profile snapshot. Returns are the resolved
// type and an optional user-readable conflict warning.
func taxpayerClassificationWithWarning(module ModuleSnapshot) (taxpayerKind, string) {
	return taxpayerClassificationWithEnterpriseContext(module, nil)
}

// taxpayerClassificationWithEnterpriseContext resolves taxpayer type only from
// the current tax-authority profile. The enterprise context is intentionally
// ignored because the bound-enterprise list may contain stale manual metadata.
// Parameters: module is the taxpayer-profile snapshot and enterpriseContext is
// retained for call compatibility. Return values are the normalized kind and an
// optional user-readable conflict warning.
func taxpayerClassificationWithEnterpriseContext(module ModuleSnapshot, enterpriseContext map[string]any) (taxpayerKind, string) {
	_ = enterpriseContext
	qualificationKinds := taxpayerKindsForAliases(module.Raw, taxpayerQualificationFieldAliases, true)
	textKinds := taxpayerKindsForAliases(module.Raw, taxpayerTypeTextFieldAliases, false)
	qualification := uniqueTaxpayerKind(qualificationKinds)
	textual := uniqueTaxpayerKind(textKinds)
	if qualification != taxpayerUnknown {
		if textual != taxpayerUnknown && textual != qualification {
			return qualification, taxpayerQualificationConflictWarning
		}
		return qualification, ""
	}
	return textual, ""
}

// taxpayerClassificationForTask resolves taxpayer type from tax-authority
// evidence available in one completed task. Parameters are the task response and
// normalized taxpayer-profile module. Return values are the normalized kind and
// a user-readable provenance warning when the deduction endpoint is the fallback.
func taxpayerClassificationForTask(task taxanalysis.TaskResponse, module ModuleSnapshot) (taxpayerKind, string) {
	classification, warning := taxpayerClassificationWithEnterpriseContext(module, nil)
	if classification != taxpayerUnknown {
		return classification, warning
	}
	if taskConfirmsSmallTaxpayer(task) {
		return taxpayerSmall, smallTaxpayerAuthorityEvidenceWarning
	}
	return taxpayerUnknown, warning
}

// taskConfirmsSmallTaxpayer reports whether the tax-authority deduction endpoint
// explicitly identified this enterprise as outside the general-taxpayer scope.
// The successful applicability marker must retain the exact authority reason;
// an unrelated failure cannot be used as taxpayer qualification evidence.
func taskConfirmsSmallTaxpayer(task taxanalysis.TaskResponse) bool {
	module, ok := moduleResultsByCode(task.ModuleResults)[deductionConfirmationModuleCode]
	if !ok {
		return false
	}
	if module.Status == taxanalysis.ModuleStatusSuccess && len(module.FailedItems) == 0 {
		raw, err := decodeRaw(module.Data)
		if err != nil {
			return false
		}
		applicability := readDeductionSummary(raw).Applicability
		return applicability.Status == deductionApplicabilityNotApplicable && applicability.ReasonCode == deductionGeneralTaxpayerOnlyReason &&
			strings.TrimSpace(applicability.Message) == smallTaxpayerAuthorityMessage
	}
	if len(module.FailedItems) == 0 {
		return false
	}
	for _, failedItem := range module.FailedItems {
		if !strings.Contains(strings.TrimSpace(failedItem.Message), smallTaxpayerAuthorityMessageFragment) {
			return false
		}
	}
	return true
}

func taxpayerKindsForAliases(root any, aliases []string, qualification bool) []taxpayerKind {
	aliasSet := map[string]bool{}
	for _, alias := range aliases {
		aliasSet[strings.ToLower(alias)] = true
	}
	result := make([]taxpayerKind, 0)
	var walk func(any, int)
	walk = func(value any, depth int) {
		if depth > 8 {
			return
		}
		switch typed := value.(type) {
		case map[string]any:
			for key, item := range typed {
				if aliasSet[strings.ToLower(strings.TrimSpace(key))] {
					if kind := parseTaxpayerKind(item, qualification); kind != taxpayerUnknown {
						result = append(result, kind)
					}
				}
				walk(item, depth+1)
			}
		case []any:
			for _, item := range typed {
				walk(item, depth+1)
			}
		}
	}
	walk(root, 0)
	return result
}

func parseTaxpayerKind(value any, qualification bool) taxpayerKind {
	text := strings.ToLower(strings.TrimSpace(scalarText(value)))
	if qualification {
		switch text {
		case taxpayerGeneralQualificationValue, taxpayerGeneralQualificationCode:
			return taxpayerGeneral
		case taxpayerSmallQualificationValue:
			return taxpayerSmall
		}
	}
	switch {
	case strings.Contains(text, "小规模"), text == "small", text == "small_scale":
		return taxpayerSmall
	case strings.Contains(text, "一般纳税人"), text == "general", text == "general_taxpayer":
		return taxpayerGeneral
	default:
		return taxpayerUnknown
	}
}

func uniqueTaxpayerKind(values []taxpayerKind) taxpayerKind {
	result := taxpayerUnknown
	for _, value := range values {
		if result == taxpayerUnknown {
			result = value
			continue
		}
		if result != value {
			return taxpayerUnknown
		}
	}
	return result
}

type invoiceCategoryAllocation struct {
	Name            string
	Amount          float64
	Counterparty    string
	CounterpartyKey string
	Month           string
}

// vatFacts extracts current-period VAT metrics from effective declarations.
// Parameter module is the VAT module snapshot. Return value prefers structured
// fields and fills only missing metrics from the tax-authority main-form HTML.
func vatFacts(module ModuleSnapshot) []vatFact {
	records, effectivePresent := namedCollectionMapsWithPresence(module.Raw, "effectiveRecords")
	if !effectivePresent {
		return nil
	}
	result := make([]vatFact, 0, len(records))
	for _, object := range records {
		period, periodOK := taxPeriodFromRecord(object, false)
		if !periodOK {
			continue
		}
		fact := vatFact{RecordID: mapText(object, "sbuuid"), Period: period.Display, TaxPeriod: period, BucketMonth: period.BucketMonth}
		fact.Sales, fact.HasSales = mapNumber(object, "salesAmount", "declaredSales", "申报销售额", "yshxse")
		fact.TaxPayable, fact.HasTaxPayable = mapNumber(object, "taxPayable", "payableTax", "应纳税额")
		fact.SupplementTax, fact.HasSupplementTax = mapNumber(object, "supplementTax", "本期应补（退）税额")
		fact.OutputTax, fact.HasOutputTax = mapNumber(object, "outputTax", "declaredOutputTax", "销项税额", "xxse")
		fact.InputTax, fact.HasInputTax = mapNumber(object, "inputTax", "declaredInputTax", "进项税额", "jxse")
		fact.EndCredit, fact.HasEndCredit = mapNumber(object, "endCredit", "endingCredit", "期末留抵税额", "qmldse")
		if source, ok := mainFormData(object, vatMainFormCode, vatMainFormNameFragment); ok {
			mergeParsedVATForm(&fact, parseVATMainForm(source))
		}
		fact.Cycle = period.Cycle
		if vatFactHasMetric(fact) {
			result = append(result, fact)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Period < result[j].Period })
	return dedupeVATFacts(result)
}

// hasStructuredVATMetric checks one decoded record for explicit VAT values.
// Parameter object is a candidate record. Return value is true when at least
// one supported metric is present, including an explicit zero.
func hasStructuredVATMetric(object map[string]any) bool {
	for _, aliases := range [][]string{
		{"salesAmount", "declaredSales", "申报销售额", "yshxse"},
		{"taxPayable", "payableTax", "应纳税额"},
		{"outputTax", "declaredOutputTax", "销项税额"},
		{"inputTax", "declaredInputTax", "进项税额"},
		{"endCredit", "endingCredit", "期末留抵税额"},
	} {
		if _, ok := mapNumber(object, aliases...); ok {
			return true
		}
	}
	return false
}

// mergeParsedVATForm fills missing structured values from parsed main-form
// HTML. Parameters are the target fact and parsed source. Structured fields are
// never overwritten; the function has no return value.
func mergeParsedVATForm(target *vatFact, parsed parsedVATForm) {
	if !target.HasSupplementTax && parsed.HasSupplementTax {
		target.SupplementTax, target.HasSupplementTax = parsed.SupplementTax, true
	}
	if !target.HasCumulativeSales && parsed.HasCumulativeSales {
		target.CumulativeSales = parsed.CumulativeSales
		target.HasCumulativeSales = true
	}
	if !target.HasSales && parsed.HasSales {
		target.Sales, target.HasSales = parsed.Sales, true
	}
	if !target.HasTaxPayable && parsed.HasTaxPayable {
		target.TaxPayable, target.HasTaxPayable = parsed.TaxPayable, true
	}
	if !target.HasOutputTax && parsed.HasOutputTax {
		target.OutputTax, target.HasOutputTax = parsed.OutputTax, true
	}
	if !target.HasInputTax && parsed.HasInputTax {
		target.InputTax, target.HasInputTax = parsed.InputTax, true
	}
	if !target.HasEndCredit && parsed.HasEndCredit {
		target.EndCredit, target.HasEndCredit = parsed.EndCredit, true
	}
}

// vatFactHasMetric reports whether a normalized fact contains any explicit
// metric. Parameter fact is one period. Return value preserves zero presence.
func vatFactHasMetric(fact vatFact) bool {
	return fact.HasSales || fact.HasSupplementTax || fact.HasTaxPayable || fact.HasOutputTax || fact.HasInputTax || fact.HasEndCredit
}

// paymentFacts extracts, normalizes, and filters paid-tax records. Parameters:
// module is the payments snapshot; startTime and endTime are inclusive report
// dates. Return value: stable monthly facts inside the requested analysis
// period; every list row is retained.
func paymentFacts(module ModuleSnapshot, startTime, endTime string) []paymentFact {
	maps, present := namedCollectionMapsWithPresence(module.Raw, "list")
	if !present {
		return nil
	}
	result := make([]paymentFact, 0, len(maps))
	for index, object := range maps {
		amount, ok := mapNumber(object, "sjje", "amount", "paidAmount", "paymentAmount")
		if !ok {
			continue
		}
		period, periodOK := taxPeriodFromRecord(object, true)
		if !periodOK || !monthWithinAnalysisPeriod(period.BucketMonth, startTime, endTime) {
			continue
		}
		name := taxNameFromRecord(object, "")
		withholding := containsAny(name, "代扣", "代缴", "代收") || boolText(object, "withholding", "withheld", "isWithholding")
		fact := paymentFact{SourceRef: fmt.Sprintf("source-snapshot.json/moduleResults[code=payments]/data/list[%d]", index), Period: period.BucketMonth, TaxPeriod: period, TaxName: name, Amount: amount, SelfPaid: !withholding}
		result = append(result, fact)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Period != result[j].Period {
			return result[i].Period < result[j].Period
		}
		if result[i].TaxName != result[j].TaxName {
			return result[i].TaxName < result[j].TaxName
		}
		return result[i].Amount < result[j].Amount
	})
	return result
}

// monthWithinAnalysisPeriod checks whether a normalized YYYY-MM value belongs
// to the inclusive report interval. Parameters: month is the candidate month;
// startTime and endTime are full report dates. Return value: true only when the
// candidate falls inside the requested interval.
func monthWithinAnalysisPeriod(month, startTime, endTime string) bool {
	startMonth := extractMonth(startTime)
	endMonth := extractMonth(endTime)
	if len(month) != 7 || len(startMonth) != 7 || len(endMonth) != 7 {
		return false
	}
	return month >= startMonth && month <= endMonth
}

// citFacts extracts cumulative quarterly income and tax from effective CIT
// declarations. Parameter module is the quarterly CIT snapshot. Return value
// prefers structured fields and fills missing values from main-form HTML.
func citFacts(module ModuleSnapshot) []citFact {
	records, effectivePresent := namedCollectionMapsWithPresence(module.Raw, "effectiveRecords")
	if !effectivePresent {
		return nil
	}
	result := make([]citFact, 0, len(records))
	for _, object := range records {
		period, periodOK := taxPeriodFromRecord(object, false)
		if !periodOK {
			continue
		}
		fact := citFact{Period: normalizeQuarter(period.End)}
		fact.CumulativeIncome, fact.HasIncome = mapNumber(object, "cumulativeRevenue", "operatingRevenue", "businessRevenue", "revenue", "营业收入")
		fact.CumulativeCost, fact.HasCost = mapNumber(object, "cumulativeCost", "operatingCost", "businessCost", "cost", "营业成本")
		fact.CumulativeProfit, fact.HasProfit = mapNumber(object, "cumulativeProfit", "totalProfit", "profitTotal", "利润总额")
		fact.CumulativeActualProfit, fact.HasActualProfit = mapNumber(object, "cumulativeActualProfit", "actualProfit", "taxableIncomeAmount", "实际利润额")
		fact.CumulativeTax, fact.HasTax = mapNumber(object, "cumulativeTax", "taxPayable", "paidTax", "incomeTax", "应纳所得税额")
		fact.CumulativeResearch, fact.HasResearch = mapNumber(object, "cumulativeResearchExpense", "researchExpense", "rdExpense", "研发费用")
		if source, ok := mainFormData(object, citMainFormCode, citMainFormNameFragment); ok {
			parsed := parseCITMainForm(source)
			if !fact.HasIncome && parsed.HasIncome {
				fact.CumulativeIncome, fact.HasIncome = parsed.Income, true
			}
			if !fact.HasTax && parsed.HasTax {
				fact.CumulativeTax, fact.HasTax = parsed.Tax, true
			}
			if !fact.HasCost && parsed.HasCost {
				fact.CumulativeCost, fact.HasCost = parsed.Cost, true
			}
			if !fact.HasProfit && parsed.HasProfit {
				fact.CumulativeProfit, fact.HasProfit = parsed.Profit, true
			}
			if !fact.HasActualProfit && parsed.HasActualProfit {
				fact.CumulativeActualProfit, fact.HasActualProfit = parsed.ActualProfit, true
			}
			if !fact.HasResearch && parsed.HasResearch {
				fact.CumulativeResearch, fact.HasResearch = parsed.Research, true
			}
		}
		if fact.Period != "" && citFactHasMetric(fact) {
			result = append(result, fact)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Period < result[j].Period })
	return dedupeCITFacts(result)
}

// dedupeCITFacts merges duplicate period records and preserves known metrics.
// Parameter values is sorted source data. Return value has one row per quarter.
func dedupeCITFacts(values []citFact) []citFact {
	result := make([]citFact, 0, len(values))
	indexes := map[string]int{}
	for _, value := range values {
		if index, ok := indexes[value.Period]; ok {
			current := &result[index]
			if !current.HasIncome && value.HasIncome {
				current.CumulativeIncome, current.HasIncome = value.CumulativeIncome, true
			}
			if !current.HasTax && value.HasTax {
				current.CumulativeTax, current.HasTax = value.CumulativeTax, true
			}
			if !current.HasCost && value.HasCost {
				current.CumulativeCost, current.HasCost = value.CumulativeCost, true
			}
			if !current.HasProfit && value.HasProfit {
				current.CumulativeProfit, current.HasProfit = value.CumulativeProfit, true
			}
			if !current.HasActualProfit && value.HasActualProfit {
				current.CumulativeActualProfit, current.HasActualProfit = value.CumulativeActualProfit, true
			}
			if !current.HasResearch && value.HasResearch {
				current.CumulativeResearch, current.HasResearch = value.CumulativeResearch, true
			}
			continue
		}
		indexes[value.Period] = len(result)
		result = append(result, value)
	}
	return result
}

// citFactHasMetric reports whether a quarterly CIT fact contains at least one
// explicit main-form amount. Parameter fact is one normalized period. Return
// value preserves explicit zeroes through the presence flags.
func citFactHasMetric(fact citFact) bool {
	return fact.HasIncome || fact.HasCost || fact.HasProfit || fact.HasActualProfit || fact.HasTax || fact.HasResearch
}

func creditGrades(module ModuleSnapshot) []creditGrade {
	maps := findAllMaps(module.Raw, func(object map[string]any) bool {
		return mapText(object, "evaluationYear", "year", "nd", "评价年度") != "" &&
			mapText(object, "taxpayerCreditRating", "creditRating", "nsxydj", "grade") != ""
	})
	byYear := map[string]string{}
	for _, object := range maps {
		year := strings.TrimSpace(mapText(object, "evaluationYear", "year", "nd", "评价年度"))
		grade := strings.ToUpper(strings.TrimSpace(mapText(object, "taxpayerCreditRating", "creditRating", "nsxydj", "grade")))
		if year != "" && grade != "" {
			byYear[year] = grade
		}
	}
	years := make([]string, 0, len(byYear))
	for year := range byYear {
		years = append(years, year)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(years)))
	if len(years) > 3 {
		years = years[:3]
	}
	result := make([]creditGrade, 0, len(years))
	for index := len(years) - 1; index >= 0; index-- {
		result = append(result, creditGrade{Year: years[index], Grade: byYear[years[index]]})
	}
	return result
}

func analyzeCounterparties(module ModuleSnapshot, issued bool) *counterpartyAnalysis {
	groups := invoiceGroups(module, issued)
	items := invoiceCounterpartyItems(groups)
	if len(items) == 0 {
		return nil
	}
	total := invoiceNetAmount(groups)
	for index := range items {
		if total > 0 {
			items[index].Share = items[index].Amount / total
		}
	}
	topFiveShare := 0.0
	for index := 0; index < len(items) && index < counterpartyTopFiveCount; index++ {
		topFiveShare += items[index].Share
	}
	limit := counterpartyTopFiveCount
	titleNoun := "客户"
	if !issued {
		titleNoun = "供应商"
	}
	title := "前五大" + titleNoun + "分析"
	if len(items) < counterpartyTopFiveCount {
		limit = len(items)
		title = fmt.Sprintf("主要%s（%d家）", titleNoun, len(items))
	} else if len(items) > counterpartyTopTenCount && topFiveShare < counterpartyTopFiveShareThreshold {
		limit = counterpartyTopTenCount
		title = "前十大" + titleNoun + "分析"
	}
	items = items[:limit]
	labels, values := make([]string, len(items)), make([]float64, len(items))
	for index, item := range items {
		labels[index], values[index] = item.Name, item.Amount
	}
	id, seriesName := "top-customers", "销项金额"
	if !issued {
		id, seriesName = "top-suppliers", "进项金额"
	}
	return &counterpartyAnalysis{
		Title: title,
		Items: items,
		Chart: &Chart{ID: id, Kind: chartKindHorizontalBar, Title: "主要" + titleNoun + "排名", Labels: labels,
			Series:  []Series{{Name: seriesName, Values: values, Color: chartColorPrimary}},
			Summary: "本图仅根据税局发票中的交易对手方和不含税金额汇总；公开信用核验不改变金额和排序口径。"},
	}
}

func businessCategories(module ModuleSnapshot) []businessCategory {
	type accumulator struct {
		amount           float64
		customers        map[string]float64
		unknownCustomers bool
		periods          map[string]bool
	}
	byName := map[string]*accumulator{}
	total := 0.0
	for _, allocation := range invoiceCategoryAllocations(invoiceGroups(module, true)) {
		if allocation.Amount == 0 || isUnidentifiedInvoiceCategory(allocation.Name) {
			continue
		}
		item := byName[allocation.Name]
		if item == nil {
			item = &accumulator{customers: map[string]float64{}, periods: map[string]bool{}}
			byName[allocation.Name] = item
		}
		item.amount += allocation.Amount
		if allocation.CounterpartyKey != "" {
			item.customers[allocation.CounterpartyKey] += allocation.Amount
		} else {
			item.unknownCustomers = true
		}
		if allocation.Month != "" {
			item.periods[allocation.Month] = true
		}
	}
	for _, item := range byName {
		if item.amount > 0 {
			total += item.amount
		}
	}
	result := make([]businessCategory, 0, len(byName))
	for name, item := range byName {
		if item.amount <= 0 {
			continue
		}
		top := 0.0
		for _, amount := range item.customers {
			if amount > top {
				top = amount
			}
		}
		category := businessCategory{Name: name, Amount: item.amount, Customers: len(item.customers), UnknownCustomers: item.unknownCustomers, Periods: len(item.periods)}
		if total > 0 {
			category.Share = item.amount / total
		}
		if item.amount > 0 {
			category.TopOneShare = top / item.amount
		}
		result = append(result, category)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Amount != result[j].Amount {
			return result[i].Amount > result[j].Amount
		}
		return result[i].Name < result[j].Name
	})
	if len(result) > 3 {
		result = result[:3]
	}
	return result
}

// isUnidentifiedInvoiceCategory reports whether an allocation is only a
// reconciliation placeholder. Parameters contain one display label. Return
// value prevents missing category data from creating a false industry chapter.
func isUnidentifiedInvoiceCategory(name string) bool {
	name = strings.TrimSpace(name)
	return name == "" || name == "未识别品类" || name == "未识别/未分摊"
}

func collectSuspiciousSignals(modules map[string]ModuleSnapshot, issued bool) []ruleSignal {
	facts := invoiceGroups(modules[map[bool]string{true: "issued_invoices", false: "received_invoices"}[issued]], issued)
	other := invoiceGroups(modules[map[bool]string{true: "received_invoices", false: "issued_invoices"}[issued]], !issued)
	otherKeys := map[string]bool{}
	ownIDs := invoiceOwnPartyIDs(modules)
	for _, fact := range other {
		if ownIDs[invoicePartyTaxID(fact.CounterpartyID)] || containsAny(fact.Status, "已红冲-全额", "已全额红冲") {
			continue
		}
		counterparty := qixinCanonicalCounterparty(modules[qixinModuleCode], fact.Counterparty, fact.CounterpartyID)
		key := signalGroupKey(counterparty, fact.Month)
		if key != "|" {
			otherKeys[key] = true
		}
	}
	result := make([]ruleSignal, 0)
	validGroups := make(map[string]bool)
	for _, fact := range facts {
		counterparty := qixinCanonicalCounterparty(modules[qixinModuleCode], fact.Counterparty, fact.CounterpartyID)
		if counterparty == "" || fact.Month == "" {
			continue
		}
		validGroups[signalGroupKey(counterparty, fact.Month)] = true
		evidenceID, evidenceAmount := fact.ID, abs(fact.Amount)
		if fact.EvidenceKey != "" {
			evidenceID, evidenceAmount = fact.EvidenceKey, fact.EvidenceAmount
		}
		if evidenceID == "" {
			evidenceID = fmt.Sprintf("group-%d", len(result))
		}
		if fact.SignalType != "" {
			result = append(result, ruleSignal{Type: fact.SignalType, Counterparty: counterparty, Period: fact.Month, Evidence: "税局发票记录返回" + signalTypeDisplay(fact.SignalType) + "。", Amount: evidenceAmount, EvidenceID: evidenceID, CrossDirectionEvidence: fact.EvidenceKey != "", Direct: fact.DirectAbnormal})
		}
		if !ownIDs[invoicePartyTaxID(fact.CounterpartyID)] && !containsAny(fact.Status, "已红冲-全额", "已全额红冲") && otherKeys[signalGroupKey(counterparty, fact.Month)] {
			result = append(result, ruleSignal{Type: "reciprocal_trade", Counterparty: counterparty, Period: fact.Month, Evidence: "同一期间与该主体同时存在进销交易。", Amount: evidenceAmount, EvidenceID: evidenceID, CrossDirectionEvidence: fact.EvidenceKey != ""})
		}
		if fact.Red && monthNumber(fact.Month) >= 12 {
			result = append(result, ruleSignal{Type: "year_end_red", Counterparty: counterparty, Period: fact.Month, Evidence: "年末存在红字发票记录。", Amount: evidenceAmount, EvidenceID: evidenceID, CrossDirectionEvidence: fact.EvidenceKey != ""})
		}
		if containsAny(fact.Status, "税率异常", "编码异常", "品名异常") {
			result = append(result, ruleSignal{Type: "invoice_field_anomaly", Counterparty: counterparty, Period: fact.Month, Evidence: "税局发票状态包含税率、编码或品名异常。", Amount: evidenceAmount, EvidenceID: evidenceID, CrossDirectionEvidence: fact.EvidenceKey != ""})
		}
		if fact.DirectAbnormal {
			signalType := "tax_authority_invoice_abnormal"
			evidence := "税局直接返回发票合规异常状态。"
			if !issued && fact.Deducted {
				signalType = "abnormal_voucher_deducted"
				evidence = "税局记录表明异常凭证已抵扣。"
			}
			result = append(result, ruleSignal{Type: signalType, Counterparty: counterparty, Period: fact.Month, Evidence: evidence, Amount: evidenceAmount, EvidenceID: evidenceID, CrossDirectionEvidence: fact.EvidenceKey != "", Direct: true})
		}
	}
	if !issued {
		for _, signal := range deductionSignals(modules["deduction_confirmations"]) {
			signal.Counterparty = qixinCanonicalCounterparty(modules[qixinModuleCode], signal.Counterparty, signal.Counterparty)
			result = append(result, signal)
		}
	}
	role := qixinImportantCustomerRole
	if !issued {
		role = qixinImportantSupplierRole
	}
	result = append(result, qixinCounterpartySignals(modules[qixinModuleCode], role, validGroups)...)
	return dedupeSignals(result)
}

// qixinCanonicalCounterparty 将发票名称或税号映射为已精确核验主体的标准名称。
//
// 参数 module 为统一任务中的启信宝模块，name 和 taxID 为发票主体标识；返回标准名称，未匹配时返回原名称或税号。
func qixinCanonicalCounterparty(module ModuleSnapshot, name, taxID string) string {
	expectedTaxID := qixinIdentityText(taxID)
	expectedName := qixinIdentityText(name)
	for _, subject := range qixinSubjectsFromRaw(module.Raw) {
		if mapText(subject, qixinSubjectRoleField) == qixinOwnEnterpriseRole {
			continue
		}
		actualTaxID := qixinIdentityText(mapText(subject, "taxId"))
		actualName := qixinIdentityText(mapText(subject, qixinSubjectNameField))
		if (expectedTaxID != "" && actualTaxID == expectedTaxID) || (expectedTaxID == "" && expectedName != "" && actualName == expectedName) {
			return mapText(subject, qixinSubjectNameField)
		}
	}
	return textOr(strings.TrimSpace(name), strings.TrimSpace(taxID))
}

// qixinCounterpartySignals 提取同一分析方向、同一交易主体和同一发票月份可验证的公开信用弱信号。
//
// 参数 module 为启信宝模块，role 为客户或供应商角色，validGroups 为实际存在发票交易的主体月份；返回弱信号列表。
func qixinCounterpartySignals(module ModuleSnapshot, role string, validGroups map[string]bool) []ruleSignal {
	result := make([]ruleSignal, 0)
	for _, subject := range qixinSubjectsFromRaw(module.Raw) {
		if mapText(subject, qixinSubjectRoleField) != role {
			continue
		}
		counterparty := mapText(subject, qixinSubjectNameField)
		result = appendQixinSignals(result, subject, counterparty, validGroups, qixinExecutionField, "counterparty_execution", "主要交易主体存在公开被执行记录。", []string{"CaseDate"}, []string{"Amount"}, false)
		result = appendQixinSignals(result, subject, counterparty, validGroups, qixinDishonestField, "counterparty_dishonest_execution", "主要交易主体存在公开失信被执行记录。", []string{"Date", "PublishDate"}, []string{"Amount"}, false)
		result = appendQixinSignals(result, subject, counterparty, validGroups, qixinAdministrativePunishmentField, "counterparty_administrative_penalty", "主要交易主体存在公开行政处罚记录。", []string{"Date", "PublishDate"}, []string{"PunishAmnt"}, true)
		for _, row := range qixinCurrentAbnormalRows(subject) {
			result = appendQixinRowSignal(result, counterparty, validGroups, row, "counterparty_operating_abnormal", "主要交易主体存在尚未移出的经营异常记录。", []string{"InDate"}, nil, false)
		}
		for _, row := range qixinCurrentSeriousIllegalRows(subject) {
			result = appendQixinRowSignal(result, counterparty, validGroups, row, "counterparty_serious_illegal", "主要交易主体存在尚未移出的严重违法失信记录。", []string{"InDate"}, nil, false)
		}
		detail := qixinSubjectData(subject, qixinDetailField)
		status := firstText(detail, "NewStatus", "Status")
		if containsAny(status, qixinAbnormalBusinessStatusKeywords...) {
			row := map[string]any{"EndDate": firstText(detail, "EndDate")}
			result = appendQixinRowSignal(result, counterparty, validGroups, row, "counterparty_abnormal_status", "主要交易主体工商登记状态为“"+status+"”。", []string{"EndDate"}, nil, false)
		}
	}
	return result
}

// appendQixinSignals 将一个启信宝子查询中的事件记录转换为组合规则弱信号。
//
// 参数 result 为已有信号，subject 为主体，counterparty 为标准名称，validGroups 为可用主体月份，field 为子查询字段，signalType/evidence 为信号定义，dateAliases/amountAliases 为字段别名，amountInTenThousands 表示金额是否以万元返回；返回追加后的信号列表。
func appendQixinSignals(result []ruleSignal, subject map[string]any, counterparty string, validGroups map[string]bool, field, signalType, evidence string, dateAliases, amountAliases []string, amountInTenThousands bool) []ruleSignal {
	rows := qixinItemMaps(qixinSubjectData(subject, field), append(append([]string{}, dateAliases...), amountAliases...)...)
	for _, row := range rows {
		result = appendQixinRowSignal(result, counterparty, validGroups, row, signalType, evidence, dateAliases, amountAliases, amountInTenThousands)
	}
	return result
}

// appendQixinRowSignal 仅在事件月份存在同主体发票交易时追加弱信号。
//
// 参数 result 为已有信号，其余参数描述主体、事件字段和金额单位；返回追加后的信号列表。
func appendQixinRowSignal(result []ruleSignal, counterparty string, validGroups map[string]bool, row map[string]any, signalType, evidence string, dateAliases, amountAliases []string, amountInTenThousands bool) []ruleSignal {
	period := extractMonth(mapText(row, dateAliases...))
	if period == "" || !validGroups[signalGroupKey(counterparty, period)] {
		return result
	}
	amount := 0.0
	if len(amountAliases) > 0 {
		amount, _ = mapNumber(row, amountAliases...)
		amount = math.Abs(amount)
		if amountInTenThousands {
			amount *= qixinTenThousandYuanScale
		}
	}
	return append(result, ruleSignal{Type: signalType, Counterparty: counterparty, Period: period, Evidence: evidence, Amount: amount})
}

func dedupeSignals(signals []ruleSignal) []ruleSignal {
	byKey := map[string]ruleSignal{}
	for _, signal := range signals {
		key := signalGroupKey(signal.Counterparty, signal.Period) + "|" + signal.Type + "|" + signal.EvidenceID
		current, ok := byKey[key]
		if !ok || (signal.Direct && !current.Direct) || (signal.Direct == current.Direct && signal.Amount > current.Amount) {
			byKey[key] = signal
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]ruleSignal, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func boolText(object map[string]any, aliases ...string) bool {
	value, ok := mapValue(object, aliases...)
	if !ok {
		return false
	}
	result, known := boolValue(value)
	return known && result
}

func boolValue(value any) (bool, bool) {
	text := strings.ToLower(strings.TrimSpace(scalarText(value)))
	switch text {
	case "true", "1", "yes", "y", "是", "已抵扣", "已勾选":
		return true, true
	case "false", "0", "no", "n", "否", "未抵扣", "未勾选":
		return false, true
	default:
		return false, false
	}
}

func recordPeriod(object map[string]any) string {
	return strings.TrimSpace(mapText(object, "period", "taxPeriod", "skssq", "skssqq", "month", "quarter", "evaluationYear", "year", "nd"))
}

func firstText(root any, aliases ...string) string {
	value, _, found := findFirst(root, aliases...)
	if !found {
		return ""
	}
	return scalarText(value)
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(value), strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func normalizeQuarter(period string) string {
	if start, _ := quarterPeriodBounds(period); start != "" {
		month := extractMonth(start)
		if len(month) == 7 {
			return fmt.Sprintf("%sQ%d", month[:4], (monthNumber(month)-firstMonthOfYear)/monthsPerQuarter+quarterMinimum)
		}
	}
	month := extractMonth(period)
	if month != "" {
		m := monthNumber(month)
		return fmt.Sprintf("%sQ%d", month[:4], (m-firstMonthOfYear)/monthsPerQuarter+quarterMinimum)
	}
	return ""
}

func monthNumber(period string) int {
	month := extractMonth(period)
	if len(month) != 7 {
		return 0
	}
	value, _ := strconv.Atoi(month[5:7])
	return value
}

func signalGroupKey(counterparty, period string) string {
	return strings.TrimSpace(counterparty) + "|" + strings.TrimSpace(period)
}

func signalTypeDisplay(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "reciprocal_trade":
		return "同一主体同期进销交易信号"
	case "year_end_red":
		return "年末红字发票信号"
	case "invoice_field_anomaly":
		return "发票税率、编码或品名异常信号"
	case "abnormal_voucher_deducted":
		return "异常凭证已抵扣信号"
	case "tax_authority_invoice_abnormal":
		return "税局直接发票合规异常信号"
	case "counterparty_execution":
		return "主要交易主体被执行信号"
	case "counterparty_dishonest_execution":
		return "主要交易主体失信被执行信号"
	case "counterparty_administrative_penalty":
		return "主要交易主体行政处罚信号"
	case "counterparty_operating_abnormal":
		return "主要交易主体经营异常信号"
	case "counterparty_serious_illegal":
		return "主要交易主体严重违法失信信号"
	case "counterparty_abnormal_status":
		return "主要交易主体登记状态异常信号"
	}
	for _, character := range strings.TrimSpace(value) {
		if unicode.Is(unicode.Han, character) {
			return strings.TrimSpace(value)
		}
	}
	return "税局发票合规异常信号"
}

func dedupeVATFacts(values []vatFact) []vatFact {
	byPeriod := map[string]vatFact{}
	for _, value := range values {
		current, exists := byPeriod[value.Period]
		if !exists || value.TaxPeriod.DeclarationDate >= current.TaxPeriod.DeclarationDate {
			// Keep one effective declaration intact: cumulative values and source
			// identity travel together, and a newer blank is not filled from an old return.
			byPeriod[value.Period] = value
		}
	}
	periods := make([]string, 0, len(byPeriod))
	for period := range byPeriod {
		periods = append(periods, period)
	}
	sort.Strings(periods)
	result := make([]vatFact, 0, len(periods))
	for _, period := range periods {
		result = append(result, byPeriod[period])
	}
	return result
}

func analysisMonthCount(start, end string) int {
	startDate, startErr := time.Parse("2006-01-02", start)
	endDate, endErr := time.Parse("2006-01-02", end)
	if startErr != nil || endErr != nil || endDate.Before(startDate) {
		return 0
	}
	return (endDate.Year()-startDate.Year())*12 + int(endDate.Month()-startDate.Month()) + 1
}

func analysisPeriodText(start, end string) string {
	startDate, startErr := time.Parse("2006-01-02", start)
	endDate, endErr := time.Parse("2006-01-02", end)
	if startErr != nil || endErr != nil {
		return strings.TrimSpace(start + " 至 " + end)
	}
	return fmt.Sprintf("%d年%02d月—%d年%02d月", startDate.Year(), startDate.Month(), endDate.Year(), endDate.Month())
}

func moneyText(value float64) string {
	if abs(value) < 0.005 {
		value = 0
	}
	return fmt.Sprintf("%.2f", value)
}

func percentText(value float64) string {
	return fmt.Sprintf("%.2f%%", value*100)
}

func optionalMoney(value float64, ok bool) string {
	if !ok {
		return "—"
	}
	return moneyText(value)
}

func optionalPercent(value float64, ok bool) string {
	if !ok {
		return "—"
	}
	return percentText(value)
}

// invoiceOwnPartyIDs reads own identities once per evaluation, not once per invoice group.
func invoiceOwnPartyIDs(modules map[string]ModuleSnapshot) map[string]bool {
	ids := map[string]bool{}
	for _, subject := range qixinSubjectsFromRaw(modules[qixinModuleCode].Raw) {
		if mapText(subject, qixinSubjectRoleField) == qixinOwnEnterpriseRole {
			if id := invoicePartyTaxID(mapText(subject, "taxId")); id != "" {
				ids[id] = true
			}
		}
	}
	// Verified enterprise identity takes precedence over erroneous invoice fields.
	if len(ids) > 0 {
		return ids
	}
	for _, code := range []string{"issued_invoices", "received_invoices"} {
		for _, chunk := range readInvoiceSummary(modules[code]).Chunks {
			for _, raw := range chunk.Checks.OwnIdentities {
				var tuple []json.RawMessage
				if json.Unmarshal(raw, &tuple) != nil || len(tuple) < 1 {
					continue
				}
				var own string
				if json.Unmarshal(tuple[0], &own) == nil {
					if id := invoicePartyTaxID(own); id != "" {
						ids[id] = true
					}
				}
			}
		}
	}
	if len(ids) > 1 {
		return map[string]bool{}
	}
	return ids
}
