package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type invoiceRecord struct {
	Month        string
	Amount       float64
	Counterparty string
}

func invoiceTrendChart(modules map[string]ModuleSnapshot, start, end string) *Chart {
	issued, issuedKnown := aggregateInvoices(modules["issued_invoices"], true)
	received, receivedKnown := aggregateInvoices(modules["received_invoices"], false)
	if len(issuedKnown) == 0 && len(receivedKnown) == 0 {
		return nil
	}
	months := completeTrendPeriods(start, end, false)
	if len(months) == 0 {
		monthsSet := map[string]bool{}
		for month := range issuedKnown {
			monthsSet[month] = true
		}
		for month := range receivedKnown {
			monthsSet[month] = true
		}
		months = sortedMapKeys(monthsSet)
	}
	issuedValues := make([]float64, len(months))
	receivedValues := make([]float64, len(months))
	issuedAvailable := make([]bool, len(months))
	receivedAvailable := make([]bool, len(months))
	for index, month := range months {
		issuedValues[index], issuedAvailable[index] = issued[month], issuedKnown[month]
		receivedValues[index], receivedAvailable[index] = received[month], receivedKnown[month]
	}
	return &Chart{
		ID: "invoice-trend", Kind: chartKindLine, Title: "进销项不含税金额趋势",
		Labels:  months,
		Series:  []Series{{Name: "销项", Values: issuedValues, Available: issuedAvailable, Color: chartColorPrimary}, {Name: "进项", Values: receivedValues, Available: receivedAvailable, Color: chartColorPositive}},
		Summary: "展示分析期间各月进销项不含税金额，未取得数据的月份显示为空缺。",
	}
}

func aggregateInvoices(module ModuleSnapshot, issued bool) (map[string]float64, map[string]bool) {
	result := map[string]float64{}
	known := map[string]bool{}
	for _, record := range invoiceRecords(module, issued) {
		result[record.Month] += record.Amount
		known[record.Month] = true
	}
	return result, known
}

func invoiceRecords(module ModuleSnapshot, issued bool) []invoiceRecord {
	result := make([]invoiceRecord, 0)
	for _, fact := range invoiceGroups(module, issued) {
		if fact.Month == "" || !fact.HasAmount {
			continue
		}
		result = append(result, invoiceRecord{Month: fact.Month, Amount: fact.Amount, Counterparty: fact.Counterparty})
	}
	return result
}

func counterpartyChart(module ModuleSnapshot, issued bool) *Chart {
	analysis := analyzeCounterparties(module, issued)
	if analysis == nil {
		return nil
	}
	return analysis.Chart
}

// paymentCompositionChart builds the tax-payment composition chart from the
// finalized payment facts. Parameters: facts contains the already normalized
// report records. Return value: nil for no usable payments, or a
// chart whose totals are identical to the report tables.
func paymentCompositionChart(facts []paymentFact) *Chart {
	amounts := map[string]float64{}
	for _, fact := range facts {
		name := strings.TrimSpace(fact.TaxName)
		amount := fact.Amount
		if name != "" && amount != 0 {
			if amount < 0 {
				amount = -amount
			}
			amounts[name] += amount
		}
	}
	if len(amounts) == 0 {
		return nil
	}
	type pair struct {
		name  string
		value float64
	}
	pairs := make([]pair, 0, len(amounts))
	for name, value := range amounts {
		pairs = append(pairs, pair{name: name, value: value})
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].value > pairs[j].value })
	if len(pairs) > 8 {
		pairs = pairs[:8]
	}
	labels := make([]string, len(pairs))
	values := make([]float64, len(pairs))
	for index, item := range pairs {
		labels[index], values[index] = item.name, item.value
	}
	return &Chart{ID: "tax-composition", Kind: chartKindHorizontalBar, Title: "分析期间税费缴款构成", Labels: labels, Series: []Series{{Name: "缴款金额", Values: values, Color: chartColorPrimary}}, Summary: "按税局缴款记录的征收项目名称和实缴金额汇总。"}
}

func findingsByModuleChart(findings []Finding) *Chart {
	moduleNames := map[string]string{}
	for _, module := range requiredModules {
		moduleNames[module.Code] = module.Name
	}
	moduleNames[qixinModuleCode] = "工商司法公开信息"
	type counts struct{ high, medium, low float64 }
	byModule := map[string]counts{}
	for _, finding := range findings {
		value := byModule[finding.ModuleCode]
		switch finding.Level {
		case RiskHigh:
			value.high++
		case RiskMedium:
			value.medium++
		case RiskLow:
			value.low++
		}
		byModule[finding.ModuleCode] = value
	}
	codes := make([]string, 0, len(byModule))
	for code := range byModule {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	labels := make([]string, len(codes))
	high, medium, low := make([]float64, len(codes)), make([]float64, len(codes)), make([]float64, len(codes))
	for index, code := range codes {
		labels[index] = textOr(moduleNames[code], code)
		high[index], medium[index], low[index] = byModule[code].high, byModule[code].medium, byModule[code].low
	}
	return &Chart{ID: "module-risk", Kind: chartKindStackedBar, Title: "各模块风险分布", Labels: labels, Unit: "风险事项数量", Series: []Series{{Name: string(RiskHigh), Values: high, Color: chartColorModuleHighRisk}, {Name: string(RiskMedium), Values: medium, Color: chartColorModuleMediumRisk}, {Name: string(RiskLow), Values: low, Color: chartColorModuleLowRisk}}}
}

func extractMonth(value string) string {
	text := strings.TrimSpace(value)
	formats := []string{"2006-01-02", "2006/01/02", "2006-01", "2006/01", "20060102", "200601"}
	for _, format := range formats {
		if parsed, err := time.Parse(format, text); err == nil {
			return parsed.Format("2006-01")
		}
	}
	if len(text) >= 7 && text[4] == '-' {
		if parsed, err := time.Parse("2006-01", text[:7]); err == nil {
			return parsed.Format("2006-01")
		}
	}
	if len(text) >= 6 {
		digits := true
		for _, char := range text[:6] {
			if char < '0' || char > '9' {
				digits = false
				break
			}
		}
		if digits {
			candidate := fmt.Sprintf("%s-%s", text[:4], text[4:6])
			if parsed, err := time.Parse("2006-01", candidate); err == nil {
				return parsed.Format("2006-01")
			}
		}
	}
	return ""
}
