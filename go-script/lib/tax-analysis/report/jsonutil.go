package report

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var sensitiveKeys = map[string]bool{
	"apikey": true, "api_key": true, "authorization": true, "cookie": true,
	"password": true, "sessionid": true, "session_id": true, "accountid": true,
	"account_id": true, "token": true,
}

func decodeRaw(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func collectFacts(root any, limit int) []Fact {
	if limit <= 0 {
		return nil
	}
	facts := make([]Fact, 0, limit)
	var walk func(any, string, int)
	walk = func(value any, path string, depth int) {
		if len(facts) >= limit || depth > 5 {
			return
		}
		switch typed := value.(type) {
		case map[string]any:
			keys := sortedKeys(typed)
			for _, key := range keys {
				if len(facts) >= limit || sensitiveKeys[strings.ToLower(strings.TrimSpace(key))] {
					continue
				}
				next := key
				if path != "" {
					next = path + "." + key
				}
				walk(typed[key], next, depth+1)
			}
		case []any:
			if path != "" {
				label := fieldLabel(lastPath(path))
				if label != "" {
					facts = append(facts, Fact{Path: path, Label: label + "数量", Value: strconv.Itoa(len(typed))})
				}
			}
			for index, item := range typed {
				if index >= 2 || len(facts) >= limit {
					break
				}
				walk(item, fmt.Sprintf("%s[%d]", path, index), depth+1)
			}
		default:
			text := scalarText(typed)
			if path != "" && text != "" {
				key := lastPath(path)
				label := fieldLabel(key)
				if label != "" && !nonDisplayFactKeys[strings.ToLower(strings.TrimSpace(key))] {
					facts = append(facts, Fact{Path: path, Label: label, Value: formatReportFactValue(key, text)})
				}
			}
		}
	}
	walk(root, "", 0)
	if len(facts) > limit {
		facts = facts[:limit]
	}
	return facts
}

// formatReportFactValue converts machine date-time values to stable Chinese
// report display values. Parameters: key is the source field name and value is
// its scalar text. Return value: the user-visible value, with non-date values
// preserved verbatim.
func formatReportFactValue(key, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	dateOnly := isReportDateField(key)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, reportTimestampLayout, "2006-01-02 15:04:05.999"} {
		parsed, err := time.Parse(layout, trimmed)
		if err != nil {
			continue
		}
		if dateOnly {
			return parsed.Format(reportDateLayout)
		}
		return parsed.In(time.FixedZone("CST", reportTimezoneOffsetSeconds)).Format(reportTimestampLayout)
	}
	return trimmed
}

// isReportDateField identifies fields that represent a business date or tax
// period. Parameters: key is the source field name. Return value: true when the
// report must omit a zero time component.
func isReportDateField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "period", "skssqq", "skssqz", "sbrq", "kprq", "jkrq", "invoicedate", "issuedate", "taxperiod":
		return true
	default:
		return false
	}
}

func recordCount(root any) int {
	best := 0
	var walk func(any, int)
	walk = func(value any, depth int) {
		if depth > 6 {
			return
		}
		switch typed := value.(type) {
		case []any:
			if len(typed) > best {
				best = len(typed)
			}
			for _, item := range typed {
				walk(item, depth+1)
			}
		case map[string]any:
			for _, item := range typed {
				walk(item, depth+1)
			}
		}
	}
	walk(root, 0)
	return best
}

func findFirst(root any, aliases ...string) (any, string, bool) {
	aliasSet := map[string]bool{}
	for _, alias := range aliases {
		aliasSet[strings.ToLower(alias)] = true
	}
	var value any
	var path string
	var found bool
	var walk func(any, string, int)
	walk = func(current any, currentPath string, depth int) {
		if found || depth > 8 {
			return
		}
		switch typed := current.(type) {
		case map[string]any:
			for _, key := range sortedKeys(typed) {
				next := key
				if currentPath != "" {
					next = currentPath + "." + key
				}
				if aliasSet[strings.ToLower(key)] {
					value, path, found = typed[key], next, true
					return
				}
				walk(typed[key], next, depth+1)
			}
		case []any:
			for index, item := range typed {
				walk(item, fmt.Sprintf("%s[%d]", currentPath, index), depth+1)
				if found {
					return
				}
			}
		}
	}
	walk(root, "", 0)
	return value, path, found
}

func findAllMaps(root any, predicate func(map[string]any) bool) []map[string]any {
	var result []map[string]any
	var walk func(any, int)
	walk = func(current any, depth int) {
		if depth > 10 {
			return
		}
		switch typed := current.(type) {
		case map[string]any:
			candidate := expandBusinessObject(typed)
			if predicate(candidate) {
				result = append(result, candidate)
			}
			for _, item := range typed {
				walk(item, depth+1)
			}
		case []any:
			for _, item := range typed {
				walk(item, depth+1)
			}
		}
	}
	walk(root, 0)
	return result
}

// expandBusinessObject flattens the actual declaration/financial response
// shape. Period metadata lives on the record while indicator values are stored
// in details/previews entries as {code|name|label, data|value}. Keeping the
// original record fields and adding the indicators lets all typed adapters use
// one case-insensitive lookup path without manufacturing zero values.
func expandBusinessObject(object map[string]any) map[string]any {
	result := make(map[string]any, len(object)+12)
	for key, value := range object {
		result[key] = value
	}
	for _, collection := range []string{"details", "previews"} {
		value, ok := mapValue(object, collection)
		if !ok {
			continue
		}
		items, ok := value.([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			indicator, exists := mapValue(entry, "data", "value", "amount")
			if !exists || !nonEmptyValue(indicator) {
				continue
			}
			for _, alias := range []string{"code", "name", "label"} {
				key := strings.TrimSpace(mapText(entry, alias))
				if key != "" {
					result[key] = indicator
				}
			}
		}
	}
	return result
}

func arrayAt(root any, aliases ...string) []any {
	value, _, ok := findFirst(root, aliases...)
	if !ok {
		return nil
	}
	if values, ok := value.([]any); ok {
		return values
	}
	return nil
}

func nonEmptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func scalarText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		if typed {
			return "是"
		}
		return "否"
	default:
		return ""
	}
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		text := strings.TrimSpace(strings.ReplaceAll(typed, ",", ""))
		parsed, err := strconv.ParseFloat(text, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func mapValue(object map[string]any, aliases ...string) (any, bool) {
	for _, alias := range aliases {
		for key, value := range object {
			if strings.EqualFold(key, alias) {
				return value, true
			}
		}
	}
	return nil, false
}

func mapText(object map[string]any, aliases ...string) string {
	value, ok := mapValue(object, aliases...)
	if !ok {
		return ""
	}
	return scalarText(value)
}

func mapNumber(object map[string]any, aliases ...string) (float64, bool) {
	value, ok := mapValue(object, aliases...)
	if !ok {
		return 0, false
	}
	return numberValue(value)
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func lastPath(path string) string {
	last := path
	if index := strings.LastIndex(last, "."); index >= 0 {
		last = last[index+1:]
	}
	if index := strings.Index(last, "["); index >= 0 {
		last = last[:index]
	}
	return last
}

var fieldLabels = map[string]string{
	"nsrmc": "企业名称", "enterpriseName": "企业名称", "companyName": "企业名称",
	"nsrsbh": "统一社会信用代码", "uscc": "统一社会信用代码", "fddbr": "法定代表人",
	"djzclxmc": "登记注册类型", "hymlmc": "行业", "zgswjgmc": "主管税务机关", "ZgswjMc": "主管税务机关",
	"registrationStatus": "登记状态", "taxpayerTypeName": "纳税人类型",
	"skssqq": "所属期起", "skssqz": "所属期止", "sbrq": "申报日期", "period": "所属期",
	"status": "状态", "confirmedExceptions": "已确认申报异常记录",
	"zsxmMc": "税种", "sjje": "金额", "jkrq": "缴款日期", "skzlMc": "税款种类",
	"taxName": "税种", "amount": "金额", "arrearsInfo": "欠税记录", "refundInfo": "退税记录",
	"taxpayerCreditRating": "纳税信用等级", "tagName": "纳税人标签",
	"hjje": "不含税金额", "hjse": "税额", "jshj": "价税合计", "kprq": "开票日期",
	"records": "业务记录", "effectiveRecords": "有效记录", "deductibleTax": "可抵扣税额",
	"taxableIncome": "应纳税所得额", "taxAmount": "税额",
	"revenue": "营业收入", "cost": "营业成本", "profit": "利润总额",
	"salesExpense": "销售费用", "managementExpense": "管理费用", "researchExpense": "研发费用",
	"financeExpense": "财务费用", "rdIncludedInManagement": "管理费用是否已含研发费用",
	"assets": "资产总额", "liabilities": "负债总额",
}

func fieldLabel(key string) string {
	for candidate, label := range fieldLabels {
		if strings.EqualFold(candidate, strings.TrimSpace(key)) {
			return label
		}
	}
	return ""
}
