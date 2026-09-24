package reimbursement

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"

	"github.com/xuri/excelize/v2"
)

const emptyReimbursementMessage = "当前没有符合条件的发票数据，结果为空"

type Phase2Options struct {
	VerifyExcel  string
	KeywordsFile string
	OutputExcel  string
	Company      string
	Now          func() time.Time
}

type Phase2Result struct {
	Success      contract.ReimbursementPhase2Success
	MultiCompany *contract.MultiCompanyFailure
}

type reimbursementRecord map[string]string

func Phase2(options Phase2Options) (Phase2Result, error) {
	now := resolveNow(options.Now)
	verifyPath, err := pathx.Normalize(options.VerifyExcel)
	if err != nil {
		return Phase2Result{}, err
	}
	var keywords keywordMapping
	hasKeywords := strings.TrimSpace(options.KeywordsFile) != ""
	if hasKeywords {
		keywords, err = loadKeywordsFile(options.KeywordsFile)
		if err != nil {
			return Phase2Result{}, err
		}
	}
	classifyStats, err := enrichVerifyExcelWithExpenseFields(verifyPath, keywords, hasKeywords, now())
	if err != nil {
		return Phase2Result{}, err
	}
	verifyPath = classifyStats.VerifyPath
	defaultName := pathx.BuildDatedFilename("发票报销清单", ".xlsx", now())
	outputPath, err := pathx.ResolveOutput(options.OutputExcel, filepath.Dir(verifyPath), defaultName, ".xlsx")
	if err != nil {
		return Phase2Result{}, err
	}
	if samePath(verifyPath, outputPath) {
		return Phase2Result{}, errors.New("最终报销清单输出路径不能覆盖中间结果文件")
	}

	records, err := readReimbursementRecords(verifyPath)
	if err != nil {
		return Phase2Result{}, err
	}
	company, companyNames := resolveReimbursementCompany(records, options.Company)
	if company == "" && len(companyNames) > 1 {
		return Phase2Result{MultiCompany: &contract.MultiCompanyFailure{
			Status:       contract.StatusFailed,
			Message:      "检测到多个发票抬头，请确认公司名称后使用 --company 重试",
			CompanyNames: companyNames,
		}}, nil
	}
	rows := normalizeReimbursementRows(records, company, now())
	outputRows, headerMismatchCount := filterReimbursementRows(rows, company)
	rowStats := applyReimbursementStatistics(outputRows)
	actualPath, err := writeReimbursementWorkbook(outputPath, outputRows, now())
	if err != nil {
		return Phase2Result{}, err
	}
	message := "phase2 完成"
	if len(outputRows) == 0 {
		message = emptyReimbursementMessage
	}
	return Phase2Result{Success: contract.ReimbursementPhase2Success{
		Status:  contract.StatusSuccess,
		Message: message,
		Data: contract.ReimbursementPhase2Data{
			CompanyName:                 company,
			VerifySuccessCount:          rowStats.successCount,
			VerifyFailedCount:           rowStats.failedCount + headerMismatchCount,
			HeaderMismatchCount:         headerMismatchCount,
			DuplicateInvoiceCount:       len(rowStats.duplicates),
			DuplicateInvoices:           rowStats.duplicates,
			ReimbursementExcelPath:      actualPath,
			ReimbursementExcelName:      filepath.Base(actualPath),
			VerifyExcelPath:             verifyPath,
			ClassificationTotalCount:    classifyStats.TotalCount,
			ClassificationSuccessCount:  classifyStats.SuccessCount,
			ClassificationFallbackCount: classifyStats.FallbackCount,
		},
	}}, nil
}

func readReimbursementRecords(path string) ([]reimbursementRecord, error) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取Excel失败: %w", err)
	}
	defer book.Close()
	sheet := activeSheetName(book)
	rows, err := book.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("读取Excel失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	headers := rows[0]
	var records []reimbursementRecord
	for rowIndex := 2; rowIndex <= len(rows); rowIndex++ {
		item := reimbursementRecord{}
		nonEmpty := false
		for colIndex, header := range headers {
			header = strings.TrimSpace(header)
			if header == "" {
				continue
			}
			col := colIndex + 1
			cell, err := excelize.CoordinatesToCellName(col, rowIndex)
			if err != nil {
				return nil, err
			}
			formula, _ := book.GetCellFormula(sheet, cell)
			if header == "文件名" && strings.HasPrefix(strings.ToUpper(formula), "HYPERLINK(") {
				target, display := parseHyperlinkFormula(formula)
				item[header] = display
				item[excelx.SourcePathKey] = target
				nonEmpty = nonEmpty || strings.TrimSpace(display) != ""
				continue
			}
			value, err := book.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
			if err != nil {
				return nil, err
			}
			if header == "发票代码" {
				value = normalizeInvoiceCode(value)
			}
			item[header] = value
			if strings.TrimSpace(value) != "" {
				nonEmpty = true
			}
		}
		if nonEmpty {
			records = append(records, item)
		}
	}
	return records, nil
}

func resolveReimbursementCompany(records []reimbursementRecord, explicit string) (string, []string) {
	explicit = strings.TrimSpace(explicit)
	names := collectReimbursementCompanyNames(records)
	if explicit != "" {
		return explicit, names
	}
	if len(names) == 1 {
		return names[0], names
	}
	return "", names
}

func collectReimbursementCompanyNames(records []reimbursementRecord) []string {
	seen := map[string]bool{}
	var names []string
	for _, item := range records {
		name := strings.TrimSpace(item["购买方名称"])
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func normalizeReimbursementRows(records []reimbursementRecord, company string, now time.Time) []map[string]any {
	batchNo := now.Format("20060102150405")
	reimburseDate := now.Format("2006-01-02")
	rows := make([]map[string]any, 0, len(records))
	for idx, record := range records {
		row := map[string]any{
			"序号":                               idx + 1,
			"报销批次号":                            batchNo,
			"报销日期":                             reimburseDate,
			"报销人":                              "",
			"部门":                               "",
			"费用分类":                             record["费用分类"],
			"报销事由":                             record["报销事由"],
			"发票类型":                             record["发票类型"],
			"开票日期":                             record["开票日期"],
			"销售方名称":                            record["销售方名称"],
			"发票代码":                             normalizeInvoiceCode(record["发票代码"]),
			"发票号码":                             record["发票号码"],
			"发票状态":                             record["发票状态"],
			"发票商品名称":                           extractFirstProductName(record["商品明细"]),
			"发票金额":                             record["不含税金额"],
			"发票税额":                             record["税额"],
			"价税合计":                             record["价税合计"],
			excelx.TaxBureauVerifyStatusHeader: record[excelx.TaxBureauVerifyStatusHeader],
			excelx.VerifyTimeHeader:            record[excelx.VerifyTimeHeader],
			"是否重复风险":                           "无重复风险",
			"文件名":                              record["文件名"],
			"备注":                               "",
			"__buyer_name__":                   strings.TrimSpace(record["购买方名称"]),
		}
		if record[excelx.SourcePathKey] != "" {
			row[excelx.SourcePathKey] = record[excelx.SourcePathKey]
		}
		_ = company
		rows = append(rows, row)
	}
	return rows
}

func filterReimbursementRows(rows []map[string]any, company string) ([]map[string]any, int) {
	var filtered []map[string]any
	headerMismatchCount := 0
	for _, row := range rows {
		if excelx.IsUnsupportedVerifyStatus(fmt.Sprint(row[excelx.TaxBureauVerifyStatusHeader])) {
			continue
		}
		buyer := strings.TrimSpace(fmt.Sprint(row["__buyer_name__"]))
		if strings.TrimSpace(company) != "" && buyer != company {
			headerMismatchCount++
			continue
		}
		copyRow := map[string]any{}
		for key, value := range row {
			copyRow[key] = value
		}
		filtered = append(filtered, copyRow)
	}
	for idx, row := range filtered {
		row["序号"] = idx + 1
	}
	return filtered, headerMismatchCount
}

type reimbursementRowStats struct {
	successCount int
	failedCount  int
	duplicates   []map[string]any
}

func applyReimbursementStatistics(rows []map[string]any) reimbursementRowStats {
	keyIndexes := map[string][]int{}
	for idx, row := range rows {
		key := buildInvoiceKey(fmt.Sprint(row["发票代码"]), fmt.Sprint(row["发票号码"]))
		if key != "" {
			keyIndexes[key] = append(keyIndexes[key], idx)
		}
	}
	duplicateIndexes := map[int]bool{}
	for _, indexes := range keyIndexes {
		if len(indexes) > 1 {
			for _, idx := range indexes {
				duplicateIndexes[idx] = true
			}
		}
	}
	stats := reimbursementRowStats{duplicates: []map[string]any{}}
	for idx, row := range rows {
		if strings.TrimSpace(fmt.Sprint(row[excelx.TaxBureauVerifyStatusHeader])) == "查验成功" {
			stats.successCount++
		} else {
			stats.failedCount++
		}
		if duplicateIndexes[idx] {
			row["是否重复风险"] = "重复发票"
			stats.duplicates = append(stats.duplicates, map[string]any{
				"invoice_code":   normalizeInvoiceCode(fmt.Sprint(row["发票代码"])),
				"invoice_number": fmt.Sprint(row["发票号码"]),
				"file_name":      fmt.Sprint(row["文件名"]),
			})
		} else {
			row["是否重复风险"] = "无重复风险"
		}
	}
	return stats
}

func writeReimbursementWorkbook(path string, rows []map[string]any, now time.Time) (string, error) {
	cleanRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		clean := map[string]any{}
		for _, header := range excelx.ReimbursementFinalHeaders {
			value := row[header]
			if header == excelx.TaxBureauVerifyStatusHeader {
				value = localizeFinalVerifyMessage(fmt.Sprint(value))
			}
			clean[header] = value
		}
		if row[excelx.SourcePathKey] != nil {
			clean[excelx.SourcePathKey] = row[excelx.SourcePathKey]
		}
		cleanRows = append(cleanRows, clean)
	}
	return excelx.WriteWorkbook(path, excelx.WorkbookSpec{
		SheetName:     "发票报销清单",
		Headers:       excelx.ReimbursementFinalHeaders,
		Rows:          cleanRows,
		AmountHeaders: excelx.ReimbursementAmountHeaders,
		Widths:        excelx.ReimbursementWidths,
		Hyperlinks:    true,
	}, now)
}

func missingHeaders(headers, required []string) []string {
	seen := map[string]bool{}
	for _, header := range headers {
		seen[strings.TrimSpace(header)] = true
	}
	var missing []string
	for _, header := range required {
		if !seen[header] {
			missing = append(missing, header)
		}
	}
	return missing
}

func activeSheetName(book *excelize.File) string {
	sheet := book.GetSheetName(book.GetActiveSheetIndex())
	if strings.TrimSpace(sheet) != "" {
		return sheet
	}
	sheets := book.GetSheetList()
	if len(sheets) > 0 {
		return sheets[0]
	}
	return excelx.DefaultSheetName
}

func headerIndex(headers []string) map[string]int {
	result := map[string]int{}
	for idx, header := range headers {
		header = strings.TrimSpace(header)
		if header != "" {
			result[header] = idx + 1
		}
	}
	return result
}

func cellValue(row []string, oneBasedCol int) string {
	if oneBasedCol <= 0 || oneBasedCol > len(row) {
		return ""
	}
	return row[oneBasedCol-1]
}

func setCell(book *excelize.File, sheet string, col, row int, value any) error {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return err
	}
	return book.SetCellValue(sheet, cell, value)
}

func normalizeInvoiceCode(value string) string {
	text := strings.TrimSpace(value)
	if text == "--" || text == "" || strings.EqualFold(text, "nan") || strings.EqualFold(text, "none") {
		return ""
	}
	return text
}

func buildInvoiceKey(code, number string) string {
	code = normalizeInvoiceCode(code)
	number = strings.TrimSpace(number)
	if code != "" {
		return code + "_" + number
	}
	return number
}

func parseHyperlinkFormula(formula string) (string, string) {
	inner := strings.TrimSpace(formula)
	inner = strings.TrimPrefix(inner, "=")
	if len(inner) >= len("HYPERLINK(") && strings.EqualFold(inner[:len("HYPERLINK(")], "HYPERLINK(") {
		inner = inner[len("HYPERLINK("):]
	}
	inner = strings.TrimSuffix(inner, ")")
	parts := splitFormulaArgs(inner)
	if len(parts) >= 2 {
		return unquoteFormulaArg(parts[0]), unquoteFormulaArg(parts[1])
	}
	if len(parts) == 1 {
		value := unquoteFormulaArg(parts[0])
		return value, value
	}
	return "", formula
}

func splitFormulaArgs(input string) []string {
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

func unquoteFormulaArg(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	return strings.ReplaceAll(value, `""`, `"`)
}

func localizeFinalVerifyMessage(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "proxy") || strings.Contains(lower, "request failed") {
		return "网络请求失败"
	}
	if strings.Contains(lower, "timeout") && strings.Contains(lower, "exceeded") {
		return "接口请求超时"
	}
	return value
}

func extractFirstProductName(value string) string {
	items := parseLineItems(value)
	if len(items) == 0 {
		return ""
	}
	return firstItemString(items[0], "Hwhyslwfwmc", "hwhyslwfwmc")
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}
