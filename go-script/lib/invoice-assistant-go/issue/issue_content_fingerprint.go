package issue

import (
	"strings"

	"github.com/shopspring/decimal"
)

func canonicalizeIssueFingerprintValue(key string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		typed = normalizeIssueFingerprintLineDefaults(typed)
		out := map[string]any{}
		for childKey, childValue := range typed {
			if volatileIssueFingerprintKey(childKey) {
				continue
			}
			canonical := canonicalizeIssueFingerprintValue(childKey, childValue)
			if emptyIssueFingerprintValue(canonical) {
				continue
			}
			out[childKey] = canonical
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			canonical := canonicalizeIssueFingerprintValue(key, item)
			if !emptyIssueFingerprintValue(canonical) {
				out = append(out, canonical)
			}
		}
		return out
	case string:
		return canonicalizeIssueString(key, typed)
	case float64, int, int64:
		if normalized, ok := normalizeDecimalForFingerprint(typed); ok {
			return normalized
		}
		return typed
	default:
		return typed
	}
}

// normalizeIssueFingerprintLineDefaults makes legacy implicit defaults equal to current normalized lines.
func normalizeIssueFingerprintLineDefaults(value map[string]any) map[string]any {
	p, pOK := ToDecimal(value["price"])
	pt, ptOK := ToDecimal(value["taxInclusivePrice"])
	if (!pOK && !ptOK) || !isIssueFingerprintLineItem(value) {
		return value
	}
	normalized := make(map[string]any, len(value)+2)
	for key, child := range value {
		normalized[key] = child
	}
	r, rOK := parseTaxRateDecimal(value["taxRate"])
	onePlusRate := decimal.NewFromInt(1).Add(r)
	if pOK {
		p = quantizeUnitPrice(p)
		normalized["price"] = decimalToString(p)
	}
	if ptOK {
		pt = quantizeUnitPrice(pt)
		normalized["taxInclusivePrice"] = decimalToString(pt)
	}
	if rOK && pOK && !ptOK {
		normalized["taxInclusivePrice"] = decimalToString(quantizeUnitPrice(p.Mul(onePlusRate)))
	}
	if rOK && ptOK && !pOK {
		normalized["price"] = decimalToString(quantizeUnitPrice(pt.Div(onePlusRate)))
	}
	if !HasMeaningfulValue(normalized["quantity"]) {
		normalized["quantity"] = "1"
	}
	return normalized
}

func isIssueFingerprintLineItem(value map[string]any) bool {
	for _, key := range []string{"taxRate", "amount", "taxInclusiveAmount", "tax", "projectName", "spbm"} {
		if _, exists := value[key]; exists {
			return true
		}
	}
	return false
}

func canonicalizeIssueString(key, value string) any {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if booleanIssueFingerprintKey(key) {
		switch strings.ToLower(text) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		}
	}
	if taxRateIssueFingerprintKey(key) {
		if rate, ok := parseTaxRateDecimal(text); ok {
			return rate.String()
		}
	}
	if numericIssueFingerprintKey(key) {
		if normalized, ok := normalizeDecimalForFingerprint(text); ok {
			return normalized
		}
	}
	return text
}

func volatileIssueFingerprintKey(key string) bool {
	normalized := normalizeIssueFingerprintKey(key)
	switch normalized {
	case "fplsh", "taskid", "taskstatus", "sceneid", "createdat", "updatedat", "startedat", "timestamp",
		"personalaccount", "areacode":
		return true
	}
	return pathKey(key) || strings.Contains(normalized, "preview")
}

func booleanIssueFingerprintKey(key string) bool {
	switch normalizeIssueFingerprintKey(key) {
	case "isnatureperson", "isincludetax":
		return true
	default:
		return false
	}
}

func taxRateIssueFingerprintKey(key string) bool {
	normalized := normalizeIssueFingerprintKey(key)
	return normalized == "taxrate" || normalized == "slv" || normalized == "shuilv"
}

func numericIssueFingerprintKey(key string) bool {
	switch normalizeIssueFingerprintKey(key) {
	case "amount", "tax", "total", "price", "quantity", "discount", "discountamount", "discounttax",
		"taxinclusiveamount", "taxinclusiveprice", "unitprice", "hjje", "jshj", "se", "je":
		return true
	default:
		return false
	}
}

func normalizeDecimalForFingerprint(value any) (string, bool) {
	decimalValue, ok := ToDecimal(value)
	if !ok {
		return "", false
	}
	return decimalValue.String(), true
}

func normalizeIssueFingerprintKey(key string) string {
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return strings.ToLower(replacer.Replace(strings.TrimSpace(key)))
}

func emptyIssueFingerprintValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}
