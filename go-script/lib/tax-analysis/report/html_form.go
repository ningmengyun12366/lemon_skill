package report

import (
	"strconv"
	"strings"
	"unicode"
)

// formTableRow contains the visible cell text from one source table row.
// Source styling is deliberately discarded so report fonts and layout remain
// controlled by the bundled OFL assets.
type formTableRow struct {
	Cells []string
	Grid  []string
	Table int
	Index int
}

// parsedVATForm contains metrics extracted from one VAT main declaration form.
// Presence flags distinguish an explicit zero from a missing source value.
type parsedVATForm struct {
	SupplementTax                                                    float64
	HasSupplementTax                                                 bool
	CumulativeSales                                                  float64
	HasCumulativeSales                                               bool
	Sales, TaxPayable, OutputTax, InputTax, EndCredit                float64
	HasSales, HasTaxPayable, HasOutputTax, HasInputTax, HasEndCredit bool
}

// parsedCITForm contains cumulative metrics extracted from one quarterly CIT
// main declaration form. Presence flags preserve missing source values.
type parsedCITForm struct {
	Income, Cost, Profit, ActualProfit, Tax, Research                   float64
	HasIncome, HasCost, HasProfit, HasActualProfit, HasTax, HasResearch bool
}

// normalizeHTMLCellText normalizes authority form whitespace without changing
// business labels. Parameter value is raw cell text. Return value is suitable
// for deterministic row matching and never contains source font information.
func normalizeHTMLCellText(value string) string {
	for _, candidate := range htmlSpaceRunes {
		value = strings.ReplaceAll(value, candidate, " ")
	}
	return strings.Join(strings.Fields(value), " ")
}

// canonicalFormLabel removes whitespace for stable Chinese label comparison.
// Parameter value is a visible cell label. Return value retains punctuation so
// similarly named declaration rows are not merged accidentally.
func canonicalFormLabel(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		switch r {
		case '(':
			return '（'
		case ')':
			return '）'
		}
		return r
	}, normalizeHTMLCellText(value))
}

// parseFormNumber parses one declaration-table numeric cell. Parameter value
// may contain comma separators, a percent sign or accounting parentheses.
// Return values are the numeric value and a presence flag.
func parseFormNumber(value string) (float64, bool) {
	text := canonicalFormLabel(value)
	if text == "" || text == "-" || text == "—" {
		return 0, false
	}
	percent := strings.HasSuffix(text, "%")
	negative := (strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")")) ||
		(strings.HasPrefix(text, "（") && strings.HasSuffix(text, "）"))
	if negative {
		runes := []rune(text)
		text = string(runes[1 : len(runes)-1])
	}
	text = strings.TrimSuffix(strings.ReplaceAll(text, ",", ""), "%")
	if text == "" {
		return 0, false
	}
	for index, r := range text {
		if unicode.IsDigit(r) || r == '.' || (index == 0 && (r == '-' || r == '+')) {
			continue
		}
		return 0, false
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	if negative {
		parsed = -parsed
	}
	if percent {
		parsed /= 100
	}
	return parsed, true
}

// rowValuesAfterLabel returns numeric cells after a matching row label.
// Parameters are the parsed row and accepted labels. Return value omits a
// leading line-number cell when the row also contains business amounts.
func rowValuesAfterLabel(row formTableRow, labels []string) ([]float64, bool) {
	for cellIndex, cell := range row.Cells {
		cellLabel := canonicalFormLabel(cell)
		matched := false
		for _, label := range labels {
			candidate := canonicalFormLabel(label)
			if cellLabel == candidate || strings.HasPrefix(cellLabel, candidate+"（") || strings.HasPrefix(cellLabel, candidate+"(") {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		values := make([]float64, 0, len(row.Cells)-cellIndex-1)
		valueTexts := make([]string, 0, len(row.Cells)-cellIndex-1)
		for _, valueCell := range row.Cells[cellIndex+1:] {
			if value, ok := parseFormNumber(valueCell); ok {
				values = append(values, value)
				valueTexts = append(valueTexts, canonicalFormLabel(valueCell))
			}
		}
		if len(values) > 1 && isDeclarationLineNumber(valueTexts[0]) {
			values = values[1:]
		}
		return values, len(values) > 0
	}
	return nil, false
}

// isDeclarationLineNumber reports whether a numeric token is a table row code
// rather than a monetary value. Parameter value is normalized cell text.
func isDeclarationLineNumber(value string) bool {
	if strings.Contains(value, ".") || strings.Contains(value, ",") || strings.HasPrefix(value, "-") {
		return false
	}
	parsed, err := strconv.Atoi(value)
	return err == nil && parsed >= 0 && parsed <= maximumDeclarationLineNumber
}

// vatCurrentPeriodAmount reads the current-period total from one VAT row.
// Parameter values excludes a row line number. Return values sum ordinary and
// immediate-refund current-period columns when both column groups are present.
func vatCurrentPeriodAmount(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	if len(values) >= 4 {
		return values[0] + values[2], true
	}
	return values[0], true
}

// parseVATMainForm extracts stable current-period metrics from one VAT main
// form. Parameter source is authority HTML. Return value never imports source
// formatting; missing metrics retain false presence flags.
func parseVATMainForm(source any) parsedVATForm {
	rows, err := decodeTaxTableRows(source)
	if err != nil {
		return parsedVATForm{}
	}
	result := parsedVATForm{}
	labels := vatSalesRowLabels
	for _, row := range rows {
		if _, ok := formValueCells(row, []string{"全部销售额"}); ok {
			labels = []string{"全部销售额"}
			break
		}
	}
	if taxTableHint(source, "smallVat") {
		labels = smallVATSalesRowLabels
	}
	complete := true
	cumulativeComplete := true
	for _, label := range labels {
		value, found := firstVATRowAmount(rows, []string{label})
		if found {
			result.Sales += value
			result.HasSales = true
		} else {
			complete = false
		}
		if cumulative, found, _ := formHeaderAmount(rows, []string{label}, []string{"本年累计"}, true); found {
			result.CumulativeSales += cumulative
			result.HasCumulativeSales = true
		} else {
			cumulativeComplete = false
		}
	}
	result.HasCumulativeSales = result.HasCumulativeSales && cumulativeComplete
	// A real multi-row form requires all sales categories, including explicit zero.
	if taxTableHint(source, "taxpayerForm") && !complete {
		result.HasSales = false
	}
	result.OutputTax, result.HasOutputTax = firstVATRowAmount(rows, vatOutputTaxRowLabels)
	result.InputTax, result.HasInputTax = firstVATRowAmount(rows, vatInputTaxRowLabels)
	result.TaxPayable, result.HasTaxPayable = firstVATRowAmount(rows, vatTaxPayableRowLabels)
	result.SupplementTax, result.HasSupplementTax = firstVATRowAmount(rows, vatSupplementTaxRowLabels)
	result.EndCredit, result.HasEndCredit = firstVATRowAmount(rows, vatEndCreditRowLabels)
	return result
}

// firstVATRowAmount resolves the first matching VAT row. Parameters are parsed
// rows and labels ordered by specificity. Return values preserve explicit zero.
func firstVATRowAmount(rows []formTableRow, labels []string) (float64, bool) {
	if value, found, header := formHeaderAmount(rows, labels, vatCurrentColumnLabels, true); header {
		return value, found
	}
	for _, label := range labels {
		for _, row := range rows {
			cells, matched := formValueCells(row, []string{label})
			if !matched {
				continue
			}
			if len(cells) == 1 {
				return parseFormNumber(cells[0])
			}
			// Legacy general-taxpayer fixtures without headers still have a fixed
			// four-cell schema. Blank current cells must never shift to cumulative.
			if len(cells) == 4 {
				first, a := parseFormNumber(cells[0])
				second, b := parseFormNumber(cells[2])
				return first + second, a && b
			}
		}
	}
	return 0, false
}

// parseCITMainForm extracts cumulative income and tax from one quarterly CIT
// main form. Parameter source is authority HTML. Return value uses the final
// numeric cell after an exact business label.
func parseCITMainForm(source any) parsedCITForm {
	rows, err := decodeTaxTableRows(source)
	if err != nil {
		return parsedCITForm{}
	}
	result := parsedCITForm{}
	result.Income, result.HasIncome = citCumulativeRowAmount(rows, citIncomeRowLabels)
	result.Cost, result.HasCost = citCumulativeRowAmount(rows, citCostRowLabels)
	result.Profit, result.HasProfit = citCumulativeRowAmount(rows, citProfitRowLabels)
	result.ActualProfit, result.HasActualProfit = citCumulativeRowAmount(rows, citActualProfitRowLabels)
	result.Tax, result.HasTax = citCumulativeRowAmount(rows, citTaxRowLabels)
	result.Research, result.HasResearch = citCumulativeRowAmount(rows, citResearchRowLabels)
	return result
}

func citCumulativeRowAmount(rows []formTableRow, labels []string) (float64, bool) {
	if value, found, header := formHeaderAmount(rows, labels, []string{"本年累计", "累计金额", "本年累计金额"}, false); header {
		return value, found
	}
	return singleFormRowAmount(rows, labels)
}

// lastFormRowAmount resolves the last numeric cell after a matching label.
// Parameters are parsed rows and candidate labels. Return values preserve zero.
func lastFormRowAmount(rows []formTableRow, labels []string) (float64, bool) {
	for _, label := range labels {
		for _, row := range rows {
			values, matched := rowValuesAfterLabel(row, []string{label})
			if matched && len(values) > 0 {
				return values[len(values)-1], true
			}
		}
	}
	return 0, false
}

// mainFormData returns the selected structured main form from one effective record.
// Parameters are the record, exact code and identifying name fragment. Return
// values are the HTML string and whether an eligible detail was present.
func mainFormData(record map[string]any, exactCode, nameFragment string) (any, bool) {
	details, ok := mapValue(record, "details")
	if !ok {
		return nil, false
	}
	values, ok := details.([]any)
	if !ok {
		return nil, false
	}
	for _, value := range values {
		detail, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(mapText(detail, "status"), "FAILED") {
			continue
		}
		code := strings.TrimSpace(mapText(detail, "code", "formCode"))
		name := strings.TrimSpace(mapText(detail, "name", "formName"))
		newVATMain := exactCode == vatMainFormCode && (code == vatNewMainFormCode || name == "增值税及附加税费申报表主表")
		if !newVATMain && code != exactCode && !containsAny(name, nameFragment) {
			continue
		}
		source, present := mapValue(detail, "data")
		if present && source != nil {
			return source, true
		}
	}
	return nil, false
}
