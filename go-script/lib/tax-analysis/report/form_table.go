package report

import (
	"strings"
)

// formColumnIndexes resolves Chinese headers within the same table, before the
// data row. The latest matching header wins; blank numeric cells never shift it.
func formColumnIndexes(rows []formTableRow, target formTableRow, headers []string) []int {
	result := formRawColumnIndexes(rows, target, headers)
	offset := formTaxRowOffset(rows, target)
	for index := range result {
		result[index] += offset
	}
	return result
}

func formRawColumnIndexes(rows []formTableRow, target formTableRow, headers []string) []int {
	var result []int
	for _, row := range rows {
		if row.Table != target.Table || row.Index >= target.Index {
			continue
		}
		var columns []int
		for column, cell := range row.Grid {
			if declarationLabelMatches(cell, headers) {
				columns = append(columns, column)
			}
		}
		if len(columns) > 0 {
			result = columns
		}
	}
	return result
}

// Some authority annexes omit the leading empty layout cell on business rows.
// Align against the explicit Chinese tax-name header, preserving its offset as
// a row-local transformation instead of compressing all empty cells.
func formTaxRowOffset(rows []formTableRow, target formTableRow) int {
	columns := formRawColumnIndexes(rows, target, []string{"税（费）种", "税种"})
	if len(columns) == 0 {
		return 0
	}
	for index, cell := range target.Grid {
		if taxProjectNames[canonicalFormLabel(cell)] {
			return index - columns[0]
		}
	}
	return 0
}

// formHeaderAmount reads the selected row/column intersection, optionally
// summing repeated column groups such as VAT ordinary and immediate refund.
// The third result distinguishes an absent header from an empty source cell.
func formHeaderAmount(rows []formTableRow, labels, headers []string, sum bool) (float64, bool, bool) {
	for _, label := range labels {
		for _, row := range rows {
			matched := false
			for _, cell := range row.Cells {
				if declarationLabelMatches(cell, []string{label}) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			columns := formColumnIndexes(rows, row, headers)
			if len(columns) == 0 {
				continue
			}
			if !sum && len(columns) != 1 {
				return 0, false, true
			}
			amount, numericCells := 0.0, 0
			for _, column := range columns {
				if column >= len(row.Grid) {
					return 0, false, true
				}
				value, ok := parseFormNumber(row.Grid[column])
				if !ok {
					// An explicit crossed-out VAT column is outside the sum. A
					// blank remains missing, and an all-inapplicable row has no value.
					if sum && formCellNotApplicable(row.Grid[column]) {
						continue
					}
					return 0, false, true
				}
				amount += value
				numericCells++
			}
			return amount, numericCells > 0, true
		}
	}
	return 0, false, false
}

func formCellNotApplicable(raw string) bool {
	value := canonicalFormLabel(raw)
	return value != "" && strings.Trim(value, "*—") == ""
}

// formValueCells keeps original positions after a row label and skips a known
// line-number/formula cell. It does not compress blanks into adjacent amounts.
func formValueCells(row formTableRow, labels []string) ([]string, bool) {
	for index, cell := range row.Cells {
		if !declarationLabelMatches(cell, labels) {
			continue
		}
		values := row.Cells[index+1:]
		if len(values) > 1 {
			line := canonicalFormLabel(values[0])
			if isDeclarationLineNumber(line) || strings.ContainsAny(line, "=＝") {
				values = values[1:]
			}
		}
		return values, true
	}
	return nil, false
}

// singleFormRowAmount only accepts one value after a labelled row. Multi-column
// forms must supply an explicit header selector instead of choosing an edge.
func singleFormRowAmount(rows []formTableRow, labels []string) (float64, bool) {
	for _, row := range rows {
		cells, matches := formValueCells(row, labels)
		if matches && len(cells) == 1 {
			return parseFormNumber(cells[0])
		}
	}
	return 0, false
}
