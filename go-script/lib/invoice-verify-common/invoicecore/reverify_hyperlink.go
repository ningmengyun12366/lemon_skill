package invoicecore

import (
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
)

func applyReverifyHyperlink(row map[string]string) {
	formula := strings.TrimSpace(row[fileNameHeader])
	if !strings.HasPrefix(strings.ToUpper(strings.TrimPrefix(formula, "=")), "HYPERLINK(") {
		return
	}
	target, display := parseReverifyHyperlinkFormula(formula)
	if display != "" {
		row[fileNameHeader] = display
	}
	if target != "" {
		row[excelx.SourcePathKey] = target
	}
}

func parseReverifyHyperlinkFormula(formula string) (string, string) {
	inner := strings.TrimSpace(strings.TrimPrefix(formula, "="))
	if len(inner) >= len("HYPERLINK(") && strings.EqualFold(inner[:len("HYPERLINK(")], "HYPERLINK(") {
		inner = inner[len("HYPERLINK("):]
	}
	inner = strings.TrimSuffix(inner, ")")
	parts := splitReverifyFormulaArgs(inner)
	if len(parts) >= 2 {
		return unquoteReverifyFormulaArg(parts[0]), unquoteReverifyFormulaArg(parts[1])
	}
	if len(parts) == 1 {
		value := unquoteReverifyFormulaArg(parts[0])
		return value, value
	}
	return "", ""
}

func splitReverifyFormulaArgs(input string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if ch == '"' {
			inQuotes = !inQuotes
		}
		if ch == ',' && !inQuotes {
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteByte(ch)
	}
	parts = append(parts, strings.TrimSpace(current.String()))
	return parts
}

func unquoteReverifyFormulaArg(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	return strings.ReplaceAll(value, `""`, `"`)
}
