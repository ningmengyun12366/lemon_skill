package report

import (
	"encoding/json"
	"sort"
	"strings"
)

// financialFacts builds one typed financial fact per actual tax period.
// Parameters: incomeModule contains profit and cash-flow previews;
// balanceModule contains balance-sheet previews. Return value merges only facts
// with identical source period bounds and never searches unrelated modules.
func financialFacts(incomeModule, balanceModule ModuleSnapshot) []financialFact {
	byPeriod := make(map[string]*financialFact)
	order := make([]string, 0)
	currentPeriods := map[string]bool{}
	for moduleIndex, module := range []ModuleSnapshot{incomeModule, balanceModule} {
		records, _ := namedCollectionMapsWithPresence(module.Raw, "records")
		records = append([]map[string]any(nil), records...)
		for _, record := range records {
			if period, ok := taxPeriodFromRecord(record, false); ok {
				currentPeriods[period.Start+"\x00"+period.End] = true
			}
		}
		references, _ := namedCollectionMapsWithPresence(yearToDateReferenceModule(module).Raw, "records")
		records = append(records, references...)
		sort.SliceStable(records, func(i, j int) bool {
			return mapText(records[i], "lrrq", "nssbrq") > mapText(records[j], "lrrq", "nssbrq")
		})
		seenPeriod := map[string]bool{}
		for _, record := range records {
			period, ok := taxPeriodFromRecord(record, false)
			if !ok {
				continue
			}
			key := period.Start + "\x00" + period.End
			if seenPeriod[key] {
				continue
			}
			seenPeriod[key] = true
			fact := byPeriod[key]
			if fact == nil {
				fact = &financialFact{Period: period.Display, TaxPeriod: period, Balances: map[string]financialBalance{}}
				byPeriod[key] = fact
				order = append(order, key)
			}
			if moduleIndex == 0 {
				fact.AccountingStandard = financialAccountingStandard(record)
				fact.SourceRecordID = mapText(record, "zlbscjuuid", "recordId")
			}
			mergeStructuredFinancialRecord(fact, record)
			applyFinancialPreviews(fact, record)
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, right := byPeriod[order[i]].TaxPeriod, byPeriod[order[j]].TaxPeriod
		if left.End != right.End {
			return left.End < right.End
		}
		// At the same closing date, a quarter precedes the annual statement.
		return left.Start > right.Start
	})
	normalizeFinancialFlowAmounts(byPeriod)
	result := make([]financialFact, 0, len(order))
	for _, key := range order {
		if fact := byPeriod[key]; currentPeriods[key] && financialFactHasMetric(*fact) {
			result = append(result, *fact)
		}
	}
	return result
}

// mergeStructuredFinancialRecord fills explicitly named normalized fields from
// one top-level record. Parameters are the destination fact and source record;
// already known values are retained and the function has no return value.
func mergeStructuredFinancialRecord(fact *financialFact, record map[string]any) {
	setFinancialNumber(&fact.CumulativeRevenue, &fact.HasCumulativeRevenue, record, "cumulativeRevenue", "yearToDateRevenue", "营业收入本年累计")
	setFinancialNumber(&fact.Revenue, &fact.HasRevenue, record, "revenue", "operatingRevenue", "businessRevenue", "营业收入")
	setFinancialNumber(&fact.Cost, &fact.HasCost, record, "cost", "operatingCost", "businessCost", "营业成本")
	setFinancialNumber(&fact.Profit, &fact.HasProfit, record, "profit", "totalProfit", "profitTotal", "利润总额")
	setFinancialNumber(&fact.NetProfit, &fact.HasNetProfit, record, "netProfit", "净利润")
	setFinancialNumber(&fact.SalesExpense, &fact.HasSalesExpense, record, "salesExpense", "sellingExpense", "销售费用")
	setFinancialNumber(&fact.ManagementExpense, &fact.HasManagementExpense, record, "managementExpense", "administrativeExpense", "管理费用")
	setFinancialNumber(&fact.ResearchExpense, &fact.HasResearchExpense, record, "researchExpense", "rdExpense", "研发费用")
	setFinancialNumber(&fact.FinanceExpense, &fact.HasFinanceExpense, record, "financeExpense", "financialExpense", "财务费用")
	setFinancialNumber(&fact.Assets, &fact.HasAssets, record, "assets", "totalAssets", "资产总额", "资产总计")
	setFinancialNumber(&fact.Liabilities, &fact.HasLiabilities, record, "liabilities", "totalLiabilities", "负债总额", "负债合计")
	setFinancialNumber(&fact.AccountsReceivable, &fact.HasAccountsReceivable, record, "accountsReceivable", "receivables", "应收账款")
	setFinancialNumber(&fact.Inventory, &fact.HasInventory, record, "inventory", "inventories", "存货")
	setFinancialNumber(&fact.ContractLiabilities, &fact.HasContractLiabilities, record, "contractLiabilities", "contractLiability", "合同负债")
	setFinancialNumber(&fact.OperatingCashFlow, &fact.HasOperatingCashFlow, record, "operatingCashFlow", "netOperatingCashFlow", "经营活动产生的现金流量净额")
	if value, ok := mapValue(record, "rdIncludedInManagement", "researchIncludedInManagement"); ok {
		fact.ResearchInManagement, fact.ResearchInManagementSet = boolValue(value)
		return
	}
	presentation := mapText(record, "researchExpensePresentation", "rdPresentationStatus", "研发费用列报状态")
	switch {
	case containsAny(presentation, "管理费用已含", "包含在管理费用"):
		fact.ResearchInManagement, fact.ResearchInManagementSet = true, true
	case containsAny(presentation, "独立列报", "单独列报"):
		fact.ResearchInManagement, fact.ResearchInManagementSet = false, true
	}
}

// setFinancialNumber copies a direct normalized field only when the target is
// missing. Parameters include destination value/presence, source and aliases;
// the function has no return value and preserves explicit zero values.
func setFinancialNumber(target *float64, present *bool, source map[string]any, aliases ...string) {
	if *present {
		return
	}
	if value, ok := mapNumber(source, aliases...); ok {
		*target, *present = value, true
	}
}

// applyFinancialPreviews parses only the record's explicit preview list.
// Parameters are the period fact and record; the function uses each preview's
// own label to prevent balance, profit and cash-flow rows from being mixed.
func applyFinancialPreviews(fact *financialFact, record map[string]any) {
	value, ok := mapValue(record, financialPreviewField)
	if !ok {
		return
	}
	previews, ok := value.([]any)
	if !ok {
		return
	}
	for _, candidate := range previews {
		preview, ok := candidate.(map[string]any)
		if !ok {
			continue
		}
		label := strings.TrimSpace(mapText(preview, financialPreviewLabelField, "name"))
		data, dataOK := mapValue(preview, financialPreviewDataField)
		if !dataOK || data == nil {
			continue
		}
		switch {
		case strings.Contains(label, financialActivityLabel):
			applyFinancialDetailValue(fact, data, financialActivityLabel)
		case strings.Contains(label, financialIncomeLabel):
			applyFinancialDetailValue(fact, data, financialIncomeLabel)
		case strings.Contains(label, financialBalanceLabel):
			applyFinancialDetailValue(fact, data, financialBalanceLabel)
		case strings.Contains(label, financialCashFlowLabel):
			applyFinancialDetailValue(fact, data, financialCashFlowLabel)
		}
	}
}

// applyFinancialDetailValue extracts label-bound metrics from one known form.
// Parameters are the destination fact, returned form data and form kind. The
// function accepts structured JSON or rendered HTML and never resolves values
// by a recursive first-field-name match.
func applyFinancialDetailValue(fact *financialFact, data any, formKind string) {
	if isTaxTable(data) {
		applyFinancialTableRows(fact, data, formKind)
		return
	}
	rows := make([]financialJSONRow, 0)
	collectFinancialJSONRows(data, &rows)
	for _, row := range rows {
		applyFinancialJSONRow(fact, row, formKind)
	}
}

type financialJSONRow struct {
	labelKey string
	label    string
	values   map[string]any
}

// collectFinancialJSONRows walks decoded form containers and collects every
// row carrying an explicit financial label. Parameters are the current value
// and destination slice; it does not infer a metric from container field names.
func collectFinancialJSONRows(value any, rows *[]financialJSONRow) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectFinancialJSONRows(item, rows)
		}
	case map[string]any:
		for _, key := range financialRowLabelKeys {
			if label := strings.TrimSpace(mapText(typed, key)); label != "" {
				*rows = append(*rows, financialJSONRow{labelKey: key, label: label, values: typed})
			}
		}
		for key, item := range typed {
			if financialStringSliceContains(financialRowLabelKeys, key) {
				continue
			}
			if text, ok := item.(string); ok {
				trimmed := strings.TrimSpace(text)
				if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
					var decoded any
					if json.Unmarshal([]byte(trimmed), &decoded) == nil {
						collectFinancialJSONRows(decoded, rows)
					}
				}
				continue
			}
			collectFinancialJSONRows(item, rows)
		}
	}
}

// applyFinancialJSONRow maps one label-bearing row to the matching report
// metric. Parameters are the fact, source row and form kind; non-matching form
// rows are ignored and missing values never become zero.
func applyFinancialJSONRow(fact *financialFact, row financialJSONRow, formKind string) {
	if formKind == financialBalanceLabel {
		applyFinancialBalanceRow(fact, row)
	}
	metric := normalizeFinancialMetricLabel(row.label)
	if financialFlowMetric(metric) && financialMetricAllowedInForm(metric, formKind) {
		if value, ok := mapNumber(row.values, "bnljje", "bnje", "ljje"); ok {
			setFinancialCumulative(fact, metric, value)
			recordFinancialProfitSign(fact, row.label)
		}
	}
	if !financialStringSliceContains(financialMetricLabels, metric) || !financialMetricAllowedInForm(metric, formKind) {
		return
	}
	keys := financialProfitValueKeys
	if fact.TaxPeriod.Cycle == taxCycleAnnual || strings.Contains(fact.Period, "年度") {
		keys = financialAnnualValueKeys
	}
	if row.labelKey == "zcxmmc" {
		keys = financialAssetValueKeys
	} else if row.labelKey == "qyxmmc" {
		keys = financialLiabilityValueKeys
	}
	value, ok := mapNumber(row.values, keys...)
	if !ok {
		return
	}
	setFinancialMetric(fact, metric, value)
	recordFinancialProfitSign(fact, row.label)
	setResearchPresentation(fact, row.label)
}

// applyFinancialHTMLRows maps visible HTML table rows using exact normalized
// business labels. Parameters are the destination, source HTML and form kind;
// the function ignores layout and styling from the tax-authority page.
func applyFinancialTableRows(fact *financialFact, source any, formKind string) {
	rows, err := decodeTaxTableRows(source)
	if err != nil {
		return
	}
	if formKind == financialActivityLabel {
		// 民非按复合表头选“本年累计数→合计”，不能选同值的非限定性列。
		if value, found := financialActivityRevenue(rows); found {
			setFinancialValue(&fact.CumulativeRevenue, &fact.HasCumulativeRevenue, value)
			if fact.TaxPeriod.Cycle == taxCycleAnnual || strings.Contains(fact.Period, "年度") {
				setFinancialValue(&fact.Revenue, &fact.HasRevenue, value)
			}
		}
		return
	}
	for _, row := range rows {
		for _, cell := range row.Cells {
			if formKind == financialBalanceLabel {
				applyFinancialHTMLBalance(fact, rows, row, cell)
			}
			metric := normalizeFinancialMetricLabel(cell)
			if financialFlowMetric(metric) && financialMetricAllowedInForm(metric, formKind) && len(formColumnIndexes(rows, row, []string{"本年累计金额", "本年累计数", "本年累计", "本年金额"})) > 0 {
				if value, found := financialHTMLAmount(rows, row, cell, []string{"本年累计金额", "本年累计数", "本年累计", "本年金额"}); found {
					setFinancialCumulative(fact, metric, value)
					recordFinancialProfitSign(fact, cell)
				}
			}
			if !financialStringSliceContains(financialMetricLabels, metric) || !financialMetricAllowedInForm(metric, formKind) {
				continue
			}
			headers := []string{"本期金额", "本月金额", "本月数", "本期数"}
			if formKind == financialBalanceLabel {
				headers = []string{"期末余额", "期末数"}
			}
			// Balance forms place assets and liabilities side by side. Choose the
			// header immediately after this label, before the next business label.
			value, ok := financialHTMLAmount(rows, row, cell, headers)
			// 年度本期值覆盖全年；仅在本期字段不可用时读取同年度累计值。
			if !ok && formKind != financialBalanceLabel && fact.TaxPeriod.Cycle == taxCycleAnnual {
				value, ok = financialHTMLAmount(rows, row, cell, []string{"本年累计金额", "本年累计数", "本年累计", "本年金额"})
			}
			if ok {
				setFinancialMetric(fact, metric, value)
				recordFinancialProfitSign(fact, cell)
				setResearchPresentation(fact, cell)
			}
		}
	}
}

func setResearchPresentation(fact *financialFact, label string) {
	label = canonicalFormLabel(label)
	if label == "研究费用" || label == "其中：研究费用" {
		fact.ResearchInManagement = true
		fact.ResearchInManagementSet = true
	}
	if label == "研发费用" {
		fact.ResearchInManagement = false
		fact.ResearchInManagementSet = true
	}
}

func financialHTMLAmount(rows []formTableRow, row formTableRow, label string, headers []string) (float64, bool) {
	start := -1
	for index, cell := range row.Grid {
		if cell == label {
			start = index
			break
		}
	}
	columns := formColumnIndexes(rows, row, headers)
	for _, column := range columns {
		if column > start && column < len(row.Grid) {
			for _, between := range row.Grid[start+1 : column] {
				if canonicalFormLabel(between) == canonicalFormLabel(label) {
					continue
				}
				if financialStringSliceContains(financialMetricLabels, normalizeFinancialMetricLabel(between)) || financialStringSliceContains(financialSupplementaryBalanceLabels, canonicalFormLabel(between)) {
					return 0, false
				}
			}
			return parseFormNumber(row.Grid[column])
		}
	}
	if len(columns) > 0 {
		return 0, false
	}
	return singleFormRowAmount([]formTableRow{row}, []string{label})
}

// normalizeFinancialMetricLabel removes only known numbering, add/subtract
// prefixes and explanatory parentheses. Parameter value is a visible form row
// label. Return value remains exact enough to avoid matching nearby subitems.
func normalizeFinancialMetricLabel(value string) string {
	label := canonicalFormLabel(value)
	for _, prefix := range financialLabelPrefixes {
		label = strings.TrimPrefix(label, prefix)
	}
	if index := strings.IndexAny(label, "（("); index >= 0 {
		label = label[:index]
	}
	label = strings.TrimSpace(label)
	if canonical, ok := financialMetricLabelAliases[label]; ok {
		return canonical
	}
	return label
}

// financialMetricAllowedInForm validates that a metric belongs to the preview
// kind named by the source record. Parameters are metric and form kind. Return
// value prevents cross-form labels from populating unrelated facts.
func financialMetricAllowedInForm(metric, formKind string) bool {
	switch formKind {
	case financialIncomeLabel:
		return financialStringSliceContains([]string{financialMetricRevenue, financialMetricCost, financialMetricProfit, financialMetricNetProfit, financialMetricSalesExpense, financialMetricAdminExpense, financialMetricRDExpense, financialMetricFinanceCost}, metric) || financialProfitComponent(metric)
	case financialBalanceLabel:
		return financialStringSliceContains([]string{financialMetricAssets, financialMetricLiabilities, financialMetricReceivables, financialMetricInventory, financialMetricContractDebt}, metric)
	case financialCashFlowLabel:
		return metric == financialMetricOperatingCF
	default:
		return false
	}
}

// setFinancialMetric records one exact label-bound financial value. Parameters
// are the destination, canonical metric name and value; the function preserves
// the first known value from the selected source form.
func setFinancialMetric(fact *financialFact, metric string, value float64) {
	switch metric {
	case financialMetricRevenue:
		setFinancialValue(&fact.Revenue, &fact.HasRevenue, value)
	case financialMetricCost:
		setFinancialValue(&fact.Cost, &fact.HasCost, value)
	case financialMetricProfit:
		setFinancialValue(&fact.Profit, &fact.HasProfit, value)
	case financialMetricNetProfit:
		setFinancialValue(&fact.NetProfit, &fact.HasNetProfit, value)
	case financialMetricSalesExpense:
		setFinancialValue(&fact.SalesExpense, &fact.HasSalesExpense, value)
	case financialMetricAdminExpense:
		setFinancialValue(&fact.ManagementExpense, &fact.HasManagementExpense, value)
	case financialMetricRDExpense:
		setFinancialValue(&fact.ResearchExpense, &fact.HasResearchExpense, value)
	case financialMetricFinanceCost:
		setFinancialValue(&fact.FinanceExpense, &fact.HasFinanceExpense, value)
	case financialMetricAssets:
		setFinancialValue(&fact.Assets, &fact.HasAssets, value)
	case financialMetricLiabilities:
		setFinancialValue(&fact.Liabilities, &fact.HasLiabilities, value)
	case financialMetricReceivables:
		setFinancialValue(&fact.AccountsReceivable, &fact.HasAccountsReceivable, value)
	case financialMetricInventory:
		setFinancialValue(&fact.Inventory, &fact.HasInventory, value)
	case financialMetricContractDebt:
		setFinancialValue(&fact.ContractLiabilities, &fact.HasContractLiabilities, value)
	case financialMetricOperatingCF:
		setFinancialValue(&fact.OperatingCashFlow, &fact.HasOperatingCashFlow, value)
	default:
		if financialProfitComponent(metric) {
			if fact.ProfitComponents == nil {
				fact.ProfitComponents = map[string]*knownValue{}
			}
			if current := fact.ProfitComponents[metric]; current == nil || !current.known {
				fact.ProfitComponents[metric] = &knownValue{value, true}
			}
		}
	}
}

// setFinancialValue records a value only when the metric is still missing.
// Parameters are destination value/presence and source value; zero is valid.
func setFinancialValue(target *float64, present *bool, value float64) {
	if *present {
		return
	}
	*target, *present = value, true
}

// financialFactHasMetric reports whether a period contains any Word-required
// financial value. Parameter fact is a merged period. Return value keeps zero
// metrics when their presence flag is true.
func financialFactHasMetric(fact financialFact) bool {
	return fact.HasRevenue || fact.HasCumulativeRevenue || fact.HasCost || fact.HasProfit || fact.HasNetProfit ||
		fact.HasSalesExpense || fact.HasManagementExpense || fact.HasResearchExpense || fact.HasFinanceExpense ||
		fact.HasAssets || fact.HasLiabilities || fact.HasAccountsReceivable || fact.HasInventory ||
		fact.HasContractLiabilities || fact.HasOperatingCashFlow || len(fact.Balances) > 0
}

// financialStringSliceContains reports exact membership. Parameters are
// candidates and target; return value is true only for a byte-for-byte equal
// entry.
func financialStringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
