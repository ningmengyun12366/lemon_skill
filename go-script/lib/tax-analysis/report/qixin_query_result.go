package report

import "strings"

type qixinQueryResult struct {
	Rows   []map[string]any
	Known  bool
	Total  *int
	Reason string
}

// qixinQueryRecords distinguishes a successful empty payload from an absent,
// failed or unreadable operation. Counts use the same WCF records as the report.
func qixinQueryRecords(subject map[string]any, field string) qixinQueryResult {
	raw, present := mapValue(subject, field)
	if !present {
		return qixinQueryResult{Reason: "未取得查询结果"}
	}
	if failedQixinPayload(raw) {
		return qixinQueryResult{Reason: "查询返回错误，不能按零记录处理"}
	}
	if field == qixinDetailField && raw != nil {
		var found bool
		raw, _, found = findFirst(raw, "AbnormalItems")
		if !found {
			return qixinQueryResult{Reason: "已返回工商详情，但未提供经营异常查询结果"}
		}
	}
	aliases := []string{"InReason", "InDate", "OutDate", "InDepartment", "Department"}
	switch field {
	case qixinExecutionField, qixinDishonestField:
		aliases = []string{"CaseNumber", "Court", "Amount", "ExecutionStatus"}
	case qixinAdministrativePunishmentField:
		aliases = []string{"Number", "Content", "Reason", "InReason", "InDate", "Date", "PublishDate", "IllegalType", "Description"}
	}
	rows, known := qixinCollectionRecords(raw, aliases)
	r := qixinQueryResult{Rows: rows, Known: known}
	if object, ok := raw.(map[string]any); ok {
		if n, exists := mapNumber(object, "Total"); exists && n >= 0 && n == float64(int(n)) {
			total := int(n)
			r.Total = &total
			if total != len(rows) {
				r.Known = false
			}
		}
	}
	if !r.Known {
		r.Reason = "返回记录不完整或无法识别，不能概括为无记录"
	}
	return r
}

func failedQixinPayload(raw any) bool {
	o, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if v, exists := mapValue(o, "success", "Success"); exists {
		if success, known := boolValue(v); known && !success {
			return true
		}
	}
	for _, key := range []string{"errorCode", "ErrorCode", "code", "Code"} {
		if code, known := mapNumber(o, key); known && code != 0 && code != 200 {
			return true
		}
	}
	return mapText(o, "error", "Error", "faultstring", "Fault") != ""
}

func qixinCollectionRecords(raw any, aliases []string) ([]map[string]any, bool) {
	if raw == nil {
		return nil, true
	}
	switch value := raw.(type) {
	case []any:
		var rows []map[string]any
		known := true
		for _, item := range value {
			items, valid := qixinCollectionRecords(item, aliases)
			rows = append(rows, items...)
			known = known && valid
		}
		return rows, known
	case map[string]any:
		if failedQixinPayload(value) {
			return nil, false
		}
		if len(value) == 0 {
			return nil, true
		}
		for _, alias := range aliases {
			if mapText(value, alias) != "" {
				return []map[string]any{value}, true
			}
		}
		if items, present := mapValue(value, "Items"); present {
			return qixinCollectionRecords(items, aliases)
		}
		var rows []map[string]any
		found, known := false, true
		for _, key := range sortedMapKeys(value) {
			if strings.HasPrefix(key, "QiXin") {
				items, valid := qixinCollectionRecords(value[key], aliases)
				rows, found, known = append(rows, items...), true, known && valid
			}
		}
		if !found {
			if total, ok := mapNumber(value, "Total"); ok && total == 0 {
				return nil, true
			}
		}
		return rows, found && known
	}
	return nil, false
}
