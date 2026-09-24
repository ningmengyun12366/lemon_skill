package report

import (
	"fmt"
	"sort"
	"strings"
)

const citContributionTitle = "所得税申报税额及贡献率"
const citContributionChartTitle = "企业所得税申报应补退税额与贡献率"
const citContributionTaxLabel = "申报应补（退）所得税额"

var citContributionHeaders = []string{"期间", citContributionTaxLabel, "贡献率", "说明"}

type citContributionPoint struct {
	sortKey               string
	period, note          string
	tax, income           float64
	taxKnown, incomeKnown bool
	taxSource             TaxFieldFact
	incomeInputs          []CalculationInput
}

// citBusinessLabel removes numbering and add/subtract prefixes while retaining
// business parentheses such as 应补（退）; formula text remains distinguishable.
func citBusinessLabel(label string) string {
	label = canonicalFormLabel(label)
	for _, prefix := range financialLabelPrefixes {
		label = strings.TrimPrefix(label, prefix)
	}
	return label
}

func selectCITAmount(fields []TaxFieldFact, labels []string, annual bool) (TaxFieldFact, bool) {
	for _, label := range labels {
		for _, field := range fields {
			main := field.FormCode == citMainFormCode || strings.Contains(field.FormName, citMainFormNameFragment)
			if annual {
				main = isAnnualCITMainForm(field)
			}
			if !main || !declarationLabelMatches(citBusinessLabel(field.RowLabel), []string{label}) ||
				!declarationLabelMatches(field.ColumnLabel, []string{"金额", "本年累计", "本年累计金额"}) {
				continue
			}
			// A present final row with a blank amount is missing, not permission
			// to substitute the preceding pre-relief or apportioned amount.
			return field, true
		}
	}
	return TaxFieldFact{}, false
}

// citContributionPoints is shared by the table, chart and calculation evidence.
// Quarterly current payable is not a cumulative metric. Annual settlement is
// shown separately, never added to quarter payments or copied into Q4.
func citContributionPoints(ctx *reportContext) []citContributionPoint {
	allIncome := append(append([]citFact(nil), ctx.cit...), citFacts(yearToDateReferenceModule(ctx.modules["cit_quarterly_returns"]))...)
	incomeByQuarter := map[string]citFact{}
	incomePaths := map[string]string{}
	for _, fact := range ctx.cit {
		incomePaths[fact.Period] = "cit_quarterly_returns.effectiveRecords"
	}
	for _, fact := range allIncome {
		incomeByQuarter[fact.Period] = fact
		if incomePaths[fact.Period] == "" {
			incomePaths[fact.Period] = "cit_quarterly_returns.yearToDateReference.effectiveRecords"
		}
	}
	incomeInput := func(quarter string, fact citFact) CalculationInput {
		return CalculationInput{Source: incomePaths[quarter] + "[period=" + quarter + "]", Label: "营业收入 · 本年累计", Value: fact.CumulativeIncome, Present: fact.HasIncome}
	}
	var result []citContributionPoint
	for _, moduleCode := range []string{"cit_quarterly_returns", "cit_annual_returns"} {
		records, _ := namedCollectionMapsWithPresence(ctx.modules[moduleCode].Raw, "effectiveRecords")
		for index, record := range records {
			period, ok := taxPeriodFromRecord(record, false)
			if !ok {
				continue
			}
			if ctx.task.StartTime != "" && (period.Start < ctx.task.StartTime || period.End > ctx.task.EndTime) {
				continue
			}
			annual := moduleCode == "cit_annual_returns"
			fields := citRecordFields(ctx, moduleCode, record, index)
			labels := []string{"实际应补（退）所得税额", "本期应补（退）所得税额"}
			if annual {
				labels = []string{"本年实际应补（退）所得税额"}
			}
			taxField, _ := selectCITAmount(fields, labels, annual)
			point := citContributionPoint{sortKey: period.End + "0", period: period.Display, tax: taxField.Value, taxKnown: taxField.Status == "present", taxSource: taxField, note: "季报；累计收入还原单季"}
			if annual {
				income, _ := selectCITAmount(fields, []string{"营业收入"}, true)
				point.income, point.incomeKnown = income.Value, income.Status == "present"
				point.incomeInputs = []CalculationInput{taxCalculationInput(income)}
				point.note = "年度汇算；单独列示"
				point.sortKey = period.End + "1"
			} else {
				quarter := normalizeQuarter(period.End)
				current := incomeByQuarter[quarter]
				point.incomeInputs = []CalculationInput{incomeInput(quarter, current)}
				if current.HasIncome && len(quarter) >= 4 {
					q := quarterNumber(quarter)
					previousQuarter := fmt.Sprintf("%sQ%d", quarter[:4], q-1)
					previous := incomeByQuarter[previousQuarter]
					if q > 1 {
						point.incomeInputs = append(point.incomeInputs, incomeInput(previousQuarter, previous))
					}
					if q == 1 {
						point.income, point.incomeKnown = current.CumulativeIncome, true
					} else if previous.HasIncome {
						point.income, point.incomeKnown = current.CumulativeIncome-previous.CumulativeIncome, true
					}
				}
			}
			result = append(result, point)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].sortKey < result[j].sortKey })
	return result
}

// Reuse field evidence already parsed for the model. Only a standalone caller
// without that model needs to parse its record again.
func citRecordFields(ctx *reportContext, module string, record map[string]any, index int) []TaxFieldFact {
	id := mapText(record, "sbuuid")
	if id != "" {
		var fields []TaxFieldFact
		for _, field := range ctx.taxFields {
			if field.Module == module && field.RecordID == id && !strings.Contains(field.Path, "referenceAnnualRecords") {
				fields = append(fields, field)
			}
		}
		if len(fields) > 0 {
			return fields
		}
	}
	return recordTaxFieldFacts(module, fmt.Sprintf("%s.effectiveRecords[%d]", module, index), record)
}
