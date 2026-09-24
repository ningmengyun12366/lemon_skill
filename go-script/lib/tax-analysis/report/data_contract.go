package report

import (
	"encoding/json"
	"fmt"
)

func decodeTaxDocuments(raw map[string]json.RawMessage) (map[string]any, error) {
	result := map[string]any{}
	for id, data := range raw {
		value, err := decodeRaw(data)
		if err != nil {
			return nil, fmt.Errorf("公共税表 %s 无法解析：%w", id, err)
		}
		if _, err = decodeTaxTableRows(value); err != nil {
			return nil, fmt.Errorf("公共税表 %s 无效：%w", id, err)
		}
		result[id] = value
	}
	return result, nil
}

func resolveTaxDocumentRefs(value any, documents map[string]any) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		if id, ok := value["$taxDocument"].(string); ok {
			document, exists := documents[id]
			if !exists {
				return nil, fmt.Errorf("公共税表 %s 不存在", id)
			}
			return document, nil
		}
		for key, item := range value {
			resolved, err := resolveTaxDocumentRefs(item, documents)
			if err != nil {
				return nil, err
			}
			value[key] = resolved
		}
	case []any:
		for i, item := range value {
			resolved, err := resolveTaxDocumentRefs(item, documents)
			if err != nil {
				return nil, err
			}
			value[i] = resolved
		}
	}
	return value, nil
}

func validateModuleSummary(code string, raw any) error {
	expected := ""
	switch code {
	case "issued_invoices", "received_invoices":
		expected = "invoice-summary.v1"
	case "deduction_confirmations":
		expected = "deduction-summary.v1"
	}
	if expected == "" {
		return nil
	}
	object, ok := raw.(map[string]any)
	if !ok || object["format"] != expected {
		return fmt.Errorf("模块 %s 的汇总格式无效", code)
	}
	chunks, ok := object["chunks"].([]any)
	if !ok {
		return fmt.Errorf("模块 %s 缺少汇总分区", code)
	}
	data, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("模块 %s 汇总无法编码：%w", code, err)
	}
	if expected == "invoice-summary.v1" {
		var summary invoiceSummary
		if err = json.Unmarshal(data, &summary); err != nil {
			return fmt.Errorf("模块 %s 汇总字段无效：%w", code, err)
		}
		for _, chunk := range chunks {
			row, ok := chunk.(map[string]any)
			if !ok {
				return fmt.Errorf("模块 %s 汇总分区无效", code)
			}
			groups, ok := row["groups"].([]any)
			if !ok {
				return fmt.Errorf("模块 %s 缺少统计组", code)
			}
			for _, group := range groups {
				value, ok := group.(map[string]any)
				if !ok {
					return fmt.Errorf("模块 %s 统计组无效", code)
				}
				for _, key := range []string{"hasAmount", "hasTax"} {
					if _, ok := value[key].(bool); !ok {
						return fmt.Errorf("模块 %s 缺少金额可用性标记 %s", code, key)
					}
				}
			}
		}
		for _, chunk := range summary.Chunks {
			for _, group := range chunk.Groups {
				if group.ID == "" || group.Count <= 0 {
					return fmt.Errorf("模块 %s 统计组缺少标识或真实张数", code)
				}
			}
		}
	} else {
		var summary deductionSummary
		if err = json.Unmarshal(data, &summary); err != nil {
			return fmt.Errorf("模块 %s 汇总字段无效：%w", code, err)
		}
	}
	return nil
}
