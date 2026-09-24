package report

import "strings"

// taxRecognition is sourced only from the tax authority's identification list.
// Its dates do not filter out declarations that actually occurred in the year.
type taxRecognition struct{ TaxName, Item, Cycle, Start, End string }

func taxRecognitions(ctx *reportContext) []taxRecognition {
	root, ok := ctx.modules["taxpayer_profile"].Raw.(map[string]any)
	if !ok {
		return nil
	}
	info, ok := root["identifyInfo"].(map[string]any)
	if !ok {
		return nil
	}
	var result []taxRecognition
	for _, object := range arrayAt(info, "taxIdentifyList") {
		row, ok := object.(map[string]any)
		if !ok {
			continue
		}
		start, end := mapText(row, "rdyxqQDate"), mapText(row, "rdyxqZDate")
		if (start != "" && start > ctx.task.EndTime) || (end != "" && end < ctx.task.StartTime) {
			continue
		}
		name, known := taxCodeName(row)
		if !known {
			name = textOr(mapText(row, "zsxmMc"), "未识别税费种")
		}
		result = append(result, taxRecognition{TaxName: name, Item: mapText(row, "zspmMc"), Cycle: mapText(row, "nsqx", "sbqx"), Start: start, End: end})
	}
	return result
}

func appendRecognitionCycleRows(ctx *reportContext, rows [][]string) [][]string {
	for _, recognition := range taxRecognitions(ctx) {
		found := false
		for index := range rows {
			if rows[index][0] != recognition.TaxName {
				continue
			}
			found = true
			note := "认定周期：" + textOr(recognition.Cycle, "未取得")
			rows[index][1] = appendUniqueText(rows[index][1], note)
			break
		}
		if !found {
			rows = append(rows, []string{recognition.TaxName, "认定周期：" + textOr(recognition.Cycle, "未取得"), "已取得税费种认定；本期申报状态以应申报清册与实际申报分别核实"})
		}
	}
	return rows
}

func appendUniqueText(original, value string) string {
	for _, part := range strings.Split(original, "；") {
		if part == value {
			return original
		}
	}
	if original == "" {
		return value
	}
	return original + "；" + value
}
