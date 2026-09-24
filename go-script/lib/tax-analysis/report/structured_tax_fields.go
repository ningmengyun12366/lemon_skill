package report

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

var structuredFinancialColumns = map[string]string{
	"byje": "本期金额", "bqje": "本期金额", "bnljje": "本年累计金额", "bnje": "本年金额", "snje": "上年金额",
	"qmyeZc": "期末余额", "ncyeZc": "年初余额", "qmyeQy": "期末余额", "ncyeQy": "年初余额",
	"qmye": "期末余额", "ncye": "年初余额",
}

// structuredTaxFieldFacts retains exact object paths and native field names
// for label-bearing JSON rows; the paired balance-sheet sides stay separate.
func structuredTaxFieldFacts(base TaxFieldFact, value any, path string) []TaxFieldFact {
	var result []TaxFieldFact
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
			var decoded any
			if json.Unmarshal([]byte(text), &decoded) == nil {
				return structuredTaxFieldFacts(base, decoded, path)
			}
		}
	case []any:
		for index, child := range typed {
			result = append(result, structuredTaxFieldFacts(base, child, fmt.Sprintf("%s[%d]", path, index))...)
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, labelKey := range financialRowLabelKeys {
			label := mapText(typed, labelKey)
			if label == "" {
				continue
			}
			for _, key := range keys {
				column, known := structuredFinancialColumns[key]
				if !known {
					continue
				}
				if labelKey == "zcxmmc" && strings.HasSuffix(key, "Qy") || labelKey == "qyxmmc" && strings.HasSuffix(key, "Zc") {
					continue
				}
				fact := base
				fact.RowLabel = label
				fact.ColumnLabel = column
				fact.Raw = scalarText(typed[key])
				fact.Path = path + "." + key
				fact.Line = mapText(typed, "hmcDm", "hc", "xh")
				fact.Value, known = parseFormNumber(fact.Raw)
				if known {
					fact.Status = "present"
				} else {
					fact.Status = "missing"
				}
				fact.ID = fmt.Sprintf("tax-field-%x", sha256.Sum256([]byte(fact.RecordID+"|"+fact.Path+"|"+labelKey)))
				result = append(result, fact)
			}
		}
		for _, key := range keys {
			result = append(result, structuredTaxFieldFacts(base, typed[key], path+"."+key)...)
		}
	}
	return result
}
