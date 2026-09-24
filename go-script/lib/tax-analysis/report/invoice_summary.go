package report

import "encoding/json"

// invoiceGroup 是服务端按业务维度计算的统计组；Count 是张数，Amount 是组内金额合计。
type invoiceGroup struct {
	EvidenceKey                                                   string
	EvidenceAmount                                                float64
	ID, Month, Counterparty, CounterpartyID                       string
	Status, SignalType                                            string
	Red, DirectAbnormal, Deducted, HasAmount, HasTax, HasCategory bool
	Amount, Tax, MaxAbsAmount                                     float64
	Count                                                         int
	Lines                                                         []invoiceLineSummary
	Allocations                                                   []struct {
		Name   string
		Amount float64
	}
}
type invoiceLineSummary struct {
	Item, Category, TaxCode string
	Amount, Tax             float64
	HasAmount, HasTax       bool
	Count                   int
}
type invoiceSummary struct {
	Format              string
	Chunks              []invoiceSummaryChunk
	YearEndObservations []yearEndReversalObservation
}
type invoiceSummaryChunk struct {
	Month          string
	Total          int
	Groups         []invoiceGroup
	Checks         invoiceChecks
	SpecificChecks []invoiceSpecificCheck
}
type invoiceChecks struct {
	Records, IdentityKnown, MissingItem, MissingCode, MissingRate, DirectAbnormal int
	OwnIdentities                                                                 []json.RawMessage
	Codes, Rates                                                                  []json.RawMessage
}
type invoiceSpecificCheck struct {
	Name           string
	Count, Missing int
	Labels         []string
}

func readInvoiceSummary(module ModuleSnapshot) invoiceSummary {
	var result invoiceSummary
	data, err := json.Marshal(module.Raw)
	if err == nil {
		_ = json.Unmarshal(data, &result)
	}
	return result
}

func invoiceGroups(module ModuleSnapshot, issued bool) []invoiceGroup {
	var result []invoiceGroup
	for _, chunk := range readInvoiceSummary(module).Chunks {
		result = append(result, chunk.Groups...)
	}
	return result
}

func invoiceCount(groups []invoiceGroup) int {
	count := 0
	for _, group := range groups {
		count += group.Count
	}
	return count
}

func invoiceCategoryAllocations(groups []invoiceGroup) []invoiceCategoryAllocation {
	var result []invoiceCategoryAllocation
	keys := invoicePartyKeys(groups)
	for index, group := range groups {
		for _, allocation := range group.Allocations {
			result = append(result, invoiceCategoryAllocation{Name: allocation.Name, Amount: allocation.Amount, Counterparty: group.Counterparty, CounterpartyKey: keys[index], Month: group.Month})
		}
	}
	return result
}

func invoiceFactLines(group invoiceGroup) []invoiceLineSummary { return group.Lines }

func invoiceHasCategoryData(group invoiceGroup) bool { return group.HasCategory }

func invoiceLineName(line invoiceLineSummary) string { return textOr(line.Category, line.Item) }

func invoiceLineSearchText(line invoiceLineSummary) string {
	return joinNonEmpty(line.Category, line.Item)
}

func mergedInvoiceChecks(module ModuleSnapshot, uscc, ownName string) (invoiceChecks, int, int, int, int) {
	var result invoiceChecks
	mismatch, nameMismatch := 0, 0
	codes, rates := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, chunk := range readInvoiceSummary(module).Chunks {
		c := chunk.Checks
		result.Records += c.Records
		result.IdentityKnown += c.IdentityKnown
		result.MissingItem += c.MissingItem
		result.MissingCode += c.MissingCode
		result.MissingRate += c.MissingRate
		result.DirectAbnormal += c.DirectAbnormal
		for _, raw := range c.OwnIdentities {
			var tuple []json.RawMessage
			_ = json.Unmarshal(raw, &tuple)
			if len(tuple) != 3 {
				continue
			}
			var id, name string
			var count int
			_ = json.Unmarshal(tuple[0], &id)
			_ = json.Unmarshal(tuple[1], &name)
			_ = json.Unmarshal(tuple[2], &count)
			if id != "" && uscc != "" && id != qixinIdentityText(uscc) {
				mismatch += count
			}
			if name != "" && ownName != "" && name != qixinIdentityText(ownName) {
				nameMismatch += count
			}
		}
		mergeInvoiceCheckSets(codes, c.Codes)
		mergeInvoiceCheckSets(rates, c.Rates)
	}
	multipleCodes, multipleRates := 0, 0
	for _, values := range codes {
		if len(values) > 1 {
			multipleCodes++
		}
	}
	for _, values := range rates {
		if len(values) > 1 {
			multipleRates++
		}
	}
	return result, mismatch, nameMismatch, multipleCodes, multipleRates
}

func mergeInvoiceCheckSets(result map[string]map[string]bool, rows []json.RawMessage) {
	for _, raw := range rows {
		var tuple []json.RawMessage
		_ = json.Unmarshal(raw, &tuple)
		if len(tuple) != 2 {
			continue
		}
		var key string
		var values []string
		_ = json.Unmarshal(tuple[0], &key)
		_ = json.Unmarshal(tuple[1], &values)
		if result[key] == nil {
			result[key] = map[string]bool{}
		}
		for _, value := range values {
			result[key][value] = true
		}
	}
}
