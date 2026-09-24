package fetchrisk

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

func (w *analysisWorkbookWriter) writeTypeSheet(sheet string, row int, a map[string]any) (int, error) {
	typeStats := objectFromAny(a["type_stats"])
	if err := w.setTitle(sheet, row, "类型统计"); err != nil {
		return row, err
	}
	row++
	directions := []struct{ key, label string }{{"input", "进项发票"}, {"output", "销项发票"}}
	for index, dir := range directions {
		dirType := objectFromAny(typeStats[dir.key])
		special := objectFromAny(dirType["special"])
		normal := objectFromAny(dirType["normal"])
		displayCount := intAny(special["count"]) + intAny(normal["count"])
		displayAmount := floatAny(special["amount"]) + floatAny(normal["amount"])
		totalRatio := "0.0%"
		if displayCount > 0 {
			totalRatio = "100.0%"
		}
		next, err := w.appendTable(sheet, row, analysisWorkbookTable{
			Title:   dir.label,
			Headers: []any{"发票类型", "张数", "金额（元）", "占比"},
			Rows: [][]any{
				{"数电发票（增值税专用发票）", fmt.Sprintf("%d 张", intAny(special["count"])), formatMoneyAny(special["amount"]), formatPercent(countRatio(intAny(special["count"]), displayCount))},
				{"数电发票（普通发票）", fmt.Sprintf("%d 张", intAny(normal["count"])), formatMoneyAny(normal["amount"]), formatPercent(countRatio(intAny(normal["count"]), displayCount))},
				{"合计", fmt.Sprintf("%d 张", displayCount), formatMoney(displayAmount), totalRatio},
			},
		})
		if err != nil {
			return row, err
		}
		row = next
		if index < len(directions)-1 {
			row++
		}
	}
	return row, nil
}

func (w *analysisWorkbookWriter) writeStatusSheet(sheet string, row int, a map[string]any) (int, error) {
	overview := objectFromAny(a["overview"])
	statusStats := objectFromAny(a["status_stats"])
	statusByDir := objectFromAny(statusStats["by_direction"])
	inputStatus := intMapFromAny(statusByDir["input"])
	outputStatus := intMapFromAny(statusByDir["output"])
	inputCount := intAny(objectFromAny(overview["input"])["count"])
	outputCount := intAny(objectFromAny(overview["output"])["count"])
	rows := [][]any{}
	for _, item := range []struct{ key, label string }{{"normal", "正常"}, {"reversed", "已红冲"}, {"void", "作废"}} {
		ic, oc := inputStatus[item.key], outputStatus[item.key]
		rows = append(rows, []any{item.label, fmt.Sprintf("%d 张", ic), fmt.Sprintf("%d 张", oc), formatPercent(countRatio(ic, inputCount)), formatPercent(countRatio(oc, outputCount))})
	}
	rows = append(rows, []any{"合计", fmt.Sprintf("%d 张", inputCount), fmt.Sprintf("%d 张", outputCount), "100.0%", "100.0%"})
	return w.appendTable(sheet, row, analysisWorkbookTable{Title: "状态统计", Headers: []any{"发票状态", "进项情况（张数）", "销项情况（张数）", "进项占比", "销项占比"}, Rows: rows})
}

func (w *analysisWorkbookWriter) writeTimeSheet(sheet string, row int, a map[string]any) (int, error) {
	timeStats := objectFromAny(a["time_stats"])
	if err := w.setTitle(sheet, row, "时间统计"); err != nil {
		return row, err
	}
	row++
	var err error
	row, err = w.appendPeriodTables(sheet, row, sliceMapFromAny(timeStats["monthly_series"]), "月份", "month")
	if err != nil {
		return row, err
	}
	span := objectFromAny(timeStats["span"])
	if boolAny(span["include_quarterly"]) {
		row, err = w.appendPeriodTables(sheet, row+1, sliceMapFromAny(timeStats["quarterly_series"]), "季度", "quarter")
		if err != nil {
			return row, err
		}
	}
	if boolAny(span["include_yearly"]) {
		row, err = w.appendPeriodTables(sheet, row+1, sliceMapFromAny(timeStats["yearly_series"]), "年度", "year")
		return row, err
	}
	return row, nil
}

func (w *analysisWorkbookWriter) writeCounterpartySheet(sheet string, row int, a map[string]any) (int, error) {
	counterparty := objectFromAny(a["counterparty_stats"])
	if err := w.setTitle(sheet, row, "客户供应商统计"); err != nil {
		return row, err
	}
	row++
	row, err := w.appendCounterpartyTable(sheet, row, "前五大客户（销项）", sliceMapFromAny(counterparty["top5_customers"]), floatAny(counterparty["customer_concentration"]))
	if err != nil {
		return row, err
	}
	row, err = w.appendCounterpartyTable(sheet, row+1, "前五大供应商（进项）", sliceMapFromAny(counterparty["top5_suppliers"]), floatAny(counterparty["supplier_concentration"]))
	if err != nil {
		return row, err
	}
	rows := [][]any{
		{"客户集中度", formatPercent(floatAny(counterparty["customer_concentration"])), formatPercent(floatAny(counterparty["customer_concentration_top10"])), concentrationLevel(floatAny(counterparty["customer_concentration"]))},
		{"供应商集中度", formatPercent(floatAny(counterparty["supplier_concentration"])), formatPercent(floatAny(counterparty["supplier_concentration_top10"])), concentrationLevel(floatAny(counterparty["supplier_concentration"]))},
	}
	return w.appendTable(sheet, row+1, analysisWorkbookTable{Title: "客户集中度", Headers: []any{"统计维度", "前 5 大占比", "前 10 大占比", "集中度评价"}, Rows: rows})
}

func (w *analysisWorkbookWriter) appendPeriodTables(sheet string, row int, series []map[string]any, label, keyName string) (int, error) {
	if len(series) == 0 {
		next, err := w.appendTable(sheet, row, analysisWorkbookTable{Title: label + "暂无可统计数据", Headers: []any{"说明"}, Rows: [][]any{{"暂无可统计数据"}}})
		return next, err
	}
	countRows := make([][]any, 0, len(series))
	amountRows := make([][]any, 0, len(series))
	for _, item := range series {
		period := textAny(item[keyName])
		countRows = append(countRows, []any{period, fmt.Sprintf("%d 张", intAny(item["input_count"])), fmt.Sprintf("%d 张", intAny(item["output_count"])), fmt.Sprintf("%d 张", intAny(item["total_count"])), momText(item["count_mom"])})
		amountRows = append(amountRows, []any{period, formatMoneyAny(item["input_amount"]), formatMoneyAny(item["output_amount"]), formatMoneyAny(item["total_amount"]), momText(item["amount_mom"])})
	}
	next, err := w.appendTable(sheet, row, analysisWorkbookTable{Title: label + "发票数量变化", Headers: []any{label, "进项张数", "销项张数", "总张数", "环比变化（总张数）"}, Rows: countRows})
	if err != nil {
		return next, err
	}
	return w.appendTable(sheet, next+1, analysisWorkbookTable{Title: label + "金额变化（单位：元）", Headers: []any{label, "进项金额", "销项金额", "总金额", "环比变化（总金额）"}, Rows: amountRows})
}

func (w *analysisWorkbookWriter) appendCounterpartyTable(sheet string, row int, title string, items []map[string]any, concentration float64) (int, error) {
	rows := make([][]any, 0, len(items)+1)
	sumAmount := 0.0
	sumCount := 0
	for idx, item := range items {
		sumAmount += floatAny(item["amount"])
		sumCount += intAny(item["count"])
		rows = append(rows, []any{idx + 1, textAny(item["name"]), formatMoneyAny(item["amount"]), fmt.Sprintf("%d 张", intAny(item["count"])), formatPercent(floatAny(item["ratio"]))})
	}
	rows = append(rows, []any{"合计", "-", formatMoney(sumAmount), fmt.Sprintf("%d 张", sumCount), formatPercent(concentration)})
	return w.appendTable(sheet, row, analysisWorkbookTable{Title: title, Headers: []any{"排名", "名称", "金额（元）", "发票张数", "占比"}, Rows: rows})
}

func (w *analysisWorkbookWriter) appendTable(sheet string, row int, table analysisWorkbookTable) (int, error) {
	if err := w.setTitle(sheet, row, table.Title); err != nil {
		return row, err
	}
	if err := w.setRow(sheet, row+1, table.Headers); err != nil {
		return row, err
	}
	for col := range table.Headers {
		headerCell, err := excelize.CoordinatesToCellName(col+1, row+1)
		if err != nil {
			return row, err
		}
		if err := w.file.SetCellStyle(sheet, headerCell, headerCell, w.headerStyle); err != nil {
			return row, err
		}
		colName, err := excelize.ColumnNumberToName(col + 1)
		if err != nil {
			return row, err
		}
		if err := w.file.SetColWidth(sheet, colName, colName, 20); err != nil {
			return row, err
		}
	}
	for idx, values := range table.Rows {
		if err := w.setRow(sheet, row+2+idx, values); err != nil {
			return row, err
		}
	}
	return row + len(table.Rows) + 2, nil
}

func (w *analysisWorkbookWriter) setTitle(sheet string, row int, title string) error {
	if err := w.setRow(sheet, row, []any{title}); err != nil {
		return err
	}
	cell, err := excelize.CoordinatesToCellName(1, row)
	if err != nil {
		return err
	}
	return w.file.SetCellStyle(sheet, cell, cell, w.titleStyle)
}

func (w *analysisWorkbookWriter) setRow(sheet string, row int, values []any) error {
	for col, value := range values {
		cell, err := excelize.CoordinatesToCellName(col+1, row)
		if err != nil {
			return err
		}
		if err := w.file.SetCellValue(sheet, cell, value); err != nil {
			return err
		}
	}
	return nil
}
