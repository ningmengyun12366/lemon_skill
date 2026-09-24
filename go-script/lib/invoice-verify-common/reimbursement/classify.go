package reimbursement

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"

	"github.com/xuri/excelize/v2"
)

type keywordMapping []keywordRule

type keywordRule struct {
	Keyword string
	Value   keywordValue
}

type keywordValue struct {
	Category string
	Reason   string
}

type classificationStats struct {
	TotalCount    int
	SuccessCount  int
	FallbackCount int
	VerifyPath    string
}

func loadKeywordsFile(path string) (keywordMapping, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '{' {
		return nil, fmt.Errorf("keywords 文件应为 JSON 对象")
	}
	var keywords keywordMapping
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("keywords 文件键必须是字符串")
		}
		var value []string
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if len(value) != 2 {
			return nil, fmt.Errorf("keywords[%q] 应为 [费用分类, 报销事由] 二元数组", key)
		}
		keywords = append(keywords, keywordRule{Keyword: strings.TrimSpace(key), Value: keywordValue{Category: value[0], Reason: value[1]}})
	}
	token, err = decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok = token.(json.Delim)
	if !ok || delimiter != '}' {
		return nil, fmt.Errorf("keywords 文件 JSON 对象未正确结束")
	}
	return keywords, nil
}

func enrichVerifyExcelWithExpenseFields(path string, keywords keywordMapping, hasKeywords bool, now time.Time) (classificationStats, error) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return classificationStats{}, fmt.Errorf("读取Excel失败: %w", err)
	}
	defer book.Close()
	sheet := activeSheetName(book)
	rows, err := book.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return classificationStats{}, fmt.Errorf("读取Excel失败: %w", err)
	}
	if len(rows) == 0 {
		return classificationStats{}, fmt.Errorf("输入 Excel 缺少表头")
	}
	headers := rows[0]
	headerMap := headerIndex(headers)
	if missing := missingHeaders(headers, excelx.IntermediateVerifyHeaders); len(missing) > 0 {
		return classificationStats{}, fmt.Errorf("输入文件缺少必需列: %s", strings.Join(missing, ", "))
	}
	lineItemsCol := headerMap["商品明细"]
	expenseCol, err := ensureHeader(book, sheet, headerMap, "费用分类")
	if err != nil {
		return classificationStats{}, err
	}
	reasonCol, err := ensureHeader(book, sheet, headerMap, "报销事由")
	if err != nil {
		return classificationStats{}, err
	}

	stats := classificationStats{}
	for rowIndex := 2; rowIndex <= len(rows); rowIndex++ {
		stats.TotalCount++
		if !hasKeywords {
			if err := setCell(book, sheet, expenseCol, rowIndex, ""); err != nil {
				return classificationStats{}, err
			}
			if err := setCell(book, sheet, reasonCol, rowIndex, ""); err != nil {
				return classificationStats{}, err
			}
			continue
		}
		lineItems := parseLineItems(cellValue(rows[rowIndex-1], lineItemsCol))
		category, reason := "未分类", "待补充"
		if len(lineItems) > 0 {
			category, reason = classifyByLineItems(lineItems, keywords)
		}
		if err := setCell(book, sheet, expenseCol, rowIndex, category); err != nil {
			return classificationStats{}, err
		}
		if err := setCell(book, sheet, reasonCol, rowIndex, reason); err != nil {
			return classificationStats{}, err
		}
		if category == "未分类" {
			stats.FallbackCount++
		} else {
			stats.SuccessCount++
		}
	}
	if err := reapplyIntermediateFormats(book, sheet, rows); err != nil {
		return classificationStats{}, err
	}
	actual, err := excelx.SaveWithCollisionFallback(book, path, now)
	if err != nil {
		return classificationStats{}, err
	}
	stats.VerifyPath = actual
	return stats, nil
}

func classifyByLineItems(items []map[string]any, keywords keywordMapping) (string, string) {
	var terms []string
	for _, item := range items {
		xmmc := firstItemString(item, "Xmmc", "xmmc")
		spfwjc := firstItemString(item, "Spfwjc", "spfwjc")
		hwdh := firstItemString(item, "Hwdh", "hwdh")
		if xmmc == "" {
			fallbackSpfwjc, fallbackXmmc := parseHwhyslwfwmc(firstItemString(item, "Hwhyslwfwmc", "hwhyslwfwmc"))
			xmmc = fallbackXmmc
			if spfwjc == "" {
				spfwjc = fallbackSpfwjc
			}
		}
		for _, term := range []string{xmmc, spfwjc, hwdh} {
			if term != "" {
				terms = append(terms, term)
			}
		}
	}
	combined := strings.ToLower(strings.Join(terms, " "))
	for _, rule := range keywords {
		if rule.Keyword != "" && strings.Contains(combined, strings.ToLower(rule.Keyword)) {
			return rule.Value.Category, truncateRunes(rule.Value.Reason, 40)
		}
	}
	return "未分类", "待补充"
}

func parseLineItems(value string) []map[string]any {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil
	}
	switch items := decoded.(type) {
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if mapped, ok := item.(map[string]any); ok {
				result = append(result, mapped)
			}
		}
		return result
	case map[string]any:
		return []map[string]any{items}
	default:
		return nil
	}
}

func parseHwhyslwfwmc(value string) (string, string) {
	parts := strings.Split(value, "*")
	var nonEmpty []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	spfwjc, xmmc := "", ""
	if len(nonEmpty) > 0 {
		spfwjc = nonEmpty[0]
	}
	if len(nonEmpty) > 1 {
		xmmc = nonEmpty[1]
	}
	return spfwjc, xmmc
}

func firstItemString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func ensureHeader(book *excelize.File, sheet string, headerMap map[string]int, header string) (int, error) {
	if col := headerMap[header]; col > 0 {
		return col, nil
	}
	col := len(headerMap) + 1
	for _, existingCol := range headerMap {
		if existingCol >= col {
			col = existingCol + 1
		}
	}
	if err := setCell(book, sheet, col, 1, header); err != nil {
		return 0, err
	}
	headerMap[header] = col
	return col, nil
}

func reapplyIntermediateFormats(book *excelize.File, sheet string, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	headerMap := headerIndex(rows[0])
	for rowIndex := 2; rowIndex <= len(rows); rowIndex++ {
		if col := headerMap["发票代码"]; col > 0 {
			if err := setCell(book, sheet, col, rowIndex, excelx.DisplayInvoiceCode(cellValue(rows[rowIndex-1], col))); err != nil {
				return err
			}
		}
		for _, header := range []string{"不含税金额", "税额", "价税合计"} {
			col := headerMap[header]
			if col == 0 {
				continue
			}
			if amount, ok := excelx.CoerceAmount(cellValue(rows[rowIndex-1], col)); ok {
				cell, err := excelize.CoordinatesToCellName(col, rowIndex)
				if err != nil {
					return err
				}
				if err := book.SetCellValue(sheet, cell, amount); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
