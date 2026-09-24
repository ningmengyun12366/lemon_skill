package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

// declarationMetricFact is one exact, label-bound amount from an effective
// declaration record. Period is the authority tax period, not the query month.
type declarationMetricFact struct {
	Period string
	Name   string
	Value  float64
}

// declarationMetricFacts reads only module-level effective records and their
// explicit detail list. Parameters are the decoded module and supported metric
// definitions. Return value never searches unrelated nested maps.
func declarationMetricFacts(root any, definitions []metricDefinition) []declarationMetricFact {
	records, present := namedCollectionMapsWithPresence(root, "effectiveRecords")
	if !present {
		return nil
	}
	result := make([]declarationMetricFact, 0)
	for _, record := range records {
		period, ok := taxPeriodFromRecord(record, false)
		if !ok {
			continue
		}
		for _, definition := range definitions {
			value, found := mapNumber(record, definition.aliases...)
			if !found {
				value, found = declarationDetailNumber(record, definition.aliases)
			}
			if found {
				result = append(result, declarationMetricFact{Period: period.Display, Name: definition.name, Value: value})
			}
		}
	}
	return result
}

// declarationDetailNumber finds one numeric row by an explicit business label
// inside a declaration's detail list. Parameters are the record and accepted
// labels. Return values preserve explicit zero and ignore unrelated fields.
func declarationDetailNumber(record map[string]any, labels []string) (float64, bool) {
	for _, data := range declarationDetailValues(record) {
		if isTaxTable(data) {
			rows, err := decodeTaxTableRows(data)
			if err == nil {
				if value, found := singleFormRowAmount(rows, labels); found {
					return value, true
				}
			}
			continue
		}
		if value, found := explicitLabeledDetailNumber(data, labels); found {
			return value, true
		}
	}
	return 0, false
}

func declarationDetailText(record map[string]any, labels []string) (string, bool) {
	for _, data := range declarationDetailValues(record) {
		if isTaxTable(data) {
			if value, found := labeledTableText(data, labels); found {
				return value, true
			}
			continue
		}
		if value, found := explicitLabeledDetailText(data, labels); found {
			return value, true
		}
	}
	return "", false
}

// declarationDetailValues returns the data member of every explicit detail
// item. Parameter record is one effective declaration. Return value preserves
// source order and excludes failed or empty detail entries.
func declarationDetailValues(record map[string]any) []any {
	value, ok := mapValue(record, "details")
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		detail, ok := item.(map[string]any)
		if !ok || strings.EqualFold(strings.TrimSpace(mapText(detail, "status")), "FAILED") {
			continue
		}
		data, found := mapValue(detail, "data")
		if found && nonEmptyValue(data) {
			result = append(result, data)
		}
	}
	return result
}

// explicitLabeledDetailNumber walks a known declaration-detail payload and
// accepts a value only from an object carrying a matching row label. Parameters
// are the decoded payload and labels. Return values never infer from a repeated
// field name elsewhere in the response.
func explicitLabeledDetailNumber(value any, labels []string) (float64, bool) {
	var numbers []float64
	var collect func(any)
	collect = func(candidate any) {
		if object, ok := candidate.(map[string]any); ok && declarationObjectLabelMatches(object, labels) {
			for _, key := range declarationMetricValueKeys {
				if number, found := mapNumber(object, key); found {
					numbers = append(numbers, number)
				}
			}
		}
		for _, child := range declarationContainerValues(candidate) {
			collect(child)
		}
	}
	collect(value)
	if len(numbers) == 0 {
		return 0, false
	}
	for _, number := range numbers[1:] {
		if number != numbers[0] {
			return 0, false
		}
	}
	return numbers[0], true
}

// explicitLabeledDetailText is the text counterpart of the numeric helper.
// Parameters are a known detail payload and accepted labels. Return values only
// use scalar fields from a matching label-bearing row.
func explicitLabeledDetailText(value any, labels []string) (string, bool) {
	if object, ok := value.(map[string]any); ok && declarationObjectLabelMatches(object, labels) {
		if candidate, found := mapValue(object, declarationMetricValueKeys...); found {
			switch candidate.(type) {
			case map[string]any, []any:
			default:
				text := strings.TrimSpace(scalarText(candidate))
				if text != "" {
					return text, true
				}
			}
		}
	}
	for _, child := range declarationContainerValues(value) {
		if text, found := explicitLabeledDetailText(child, labels); found {
			return text, true
		}
	}
	return "", false
}

// declarationObjectLabelMatches reports whether a structured detail row has a
// supported explicit label. Parameters are the row and accepted labels.
func declarationObjectLabelMatches(object map[string]any, labels []string) bool {
	for _, key := range declarationMetricLabelKeys {
		label := strings.TrimSpace(mapText(object, key))
		if label != "" && declarationLabelMatches(label, labels) {
			return true
		}
	}
	return false
}

// declarationLabelMatches performs exact normalized label matching while
// allowing only a trailing formula in parentheses. Parameters are one source
// label and accepted business labels.
func declarationLabelMatches(label string, labels []string) bool {
	source := canonicalFormLabel(label)
	for _, label := range labels {
		candidate := canonicalFormLabel(label)
		if source == candidate || strings.HasPrefix(source, candidate+"（") || strings.HasPrefix(source, candidate+"(") {
			return true
		}
	}
	return false
}

// declarationContainerValues returns only child containers that may contain
// label-bearing detail rows. Parameter value is decoded JSON. Scalar fields are
// intentionally excluded from recursion.
func declarationContainerValues(value any) []any {
	result := make([]any, 0)
	switch typed := value.(type) {
	case []any:
		result = append(result, typed...)
	case map[string]any:
		for _, child := range typed {
			switch childValue := child.(type) {
			case map[string]any, []any:
				result = append(result, child)
			case string:
				text := strings.TrimSpace(childValue)
				if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
					var decoded any
					if json.Unmarshal([]byte(text), &decoded) == nil {
						result = append(result, decoded)
					}
				}
			}
		}
	}
	return result
}

// labeledHTMLText returns the final visible cell after an exact label. The
// numeric row number immediately following a label is skipped when present.
func labeledTableText(source any, labels []string) (string, bool) {
	rows, err := decodeTaxTableRows(source)
	if err != nil {
		return "", false
	}
	for _, row := range rows {
		for index, cell := range row.Cells {
			if !declarationLabelMatches(cell, labels) {
				continue
			}
			values := row.Cells[index+1:]
			if len(values) > 1 && isDeclarationLineNumber(canonicalFormLabel(values[0])) {
				values = values[1:]
			}
			if len(values) == 1 && strings.TrimSpace(values[0]) != "" {
				return strings.TrimSpace(values[0]), true
			}
		}
	}
	return "", false
}

// metricRows formats typed declaration amounts for report tables. Parameters
// are the module root and metric definitions. Return value keeps source period
// labels and does not add missing rows.
func metricRows(root any, definitions []metricDefinition) [][]string {
	facts := declarationMetricFacts(root, definitions)
	rows := make([][]string, 0, len(facts))
	definitionsByName := make(map[string]metricDefinition, len(definitions))
	for _, definition := range definitions {
		definitionsByName[definition.name] = definition
	}
	for _, fact := range facts {
		value := fmt.Sprint(fact.Value)
		if definitionsByName[fact.Name].money {
			value = moneyText(fact.Value)
		}
		rows = append(rows, []string{fact.Period, fact.Name, value})
	}
	return rows
}

// textMetricRows formats exact text metrics from effective declarations.
// Parameters are the module root and supported definitions. Return value uses
// only top-level normalized fields or explicitly labelled detail rows.
func textMetricRows(root any, definitions []textMetricDefinition) [][]string {
	records, present := namedCollectionMapsWithPresence(root, "effectiveRecords")
	if !present {
		return nil
	}
	rows := make([][]string, 0)
	for _, record := range records {
		period, ok := taxPeriodFromRecord(record, false)
		if !ok {
			continue
		}
		for _, definition := range definitions {
			value := strings.TrimSpace(mapText(record, definition.aliases...))
			if value == "" {
				value, _ = declarationDetailText(record, definition.aliases)
			}
			if value != "" {
				rows = append(rows, []string{period.Display, definition.name, value})
			}
		}
	}
	return rows
}
