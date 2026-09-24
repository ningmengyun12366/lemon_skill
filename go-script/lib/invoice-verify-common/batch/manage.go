package batch

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"

	"github.com/xuri/excelize/v2"
)

const (
	headerMismatchStatus = "发票抬头与用户提供的企业抬头不一致"
	emptyFinalMessage    = "当前没有符合条件的发票数据，结果为空"
)

type ManageOptions struct {
	InputPath      string
	Output         string
	Company        string
	HistoryPath    string
	CheckDuplicate bool
	CheckReimburse bool
	CheckAccount   bool
	Now            func() time.Time
}

type ManageResult struct {
	Success      contract.BatchManageSuccess
	MultiCompany *contract.MultiCompanyFailure
}

type record map[string]string

func Manage(options ManageOptions) (ManageResult, error) {
	now := resolveNow(options.Now)
	inputPath, err := pathx.Normalize(options.InputPath)
	if err != nil {
		return ManageResult{}, err
	}
	defaultName := pathx.BuildDatedFilename("发票查验结果清单", ".xlsx", now())
	outputPath, err := pathx.ResolveOutput(options.Output, filepath.Dir(inputPath), defaultName, ".xlsx")
	if err != nil {
		return ManageResult{}, err
	}
	if samePath(inputPath, outputPath) {
		return ManageResult{}, errors.New("最终清单输出路径不能覆盖中间结果文件")
	}

	records, err := readRecords(inputPath)
	if err != nil {
		return ManageResult{}, err
	}
	company, companyNames := resolveCompany(records, options.Company)
	if company == "" && len(companyNames) > 1 {
		return ManageResult{MultiCompany: &contract.MultiCompanyFailure{
			Status:       contract.StatusFailed,
			Message:      "检测到多个发票抬头，请确认公司名称后使用 --company 重试",
			CompanyNames: companyNames,
		}}, nil
	}

	hasHistory := strings.TrimSpace(options.HistoryPath) != ""
	checkDuplicate := true
	checkReimburse := options.CheckReimburse
	checkAccount := options.CheckAccount
	if !hasHistory && (checkReimburse || checkAccount) {
		return ManageResult{}, errors.New("启用报销状态或入账状态核对时必须提供 --history 标准 CSV 历史清单")
	}
	historyIndex := map[string]record{}
	if hasHistory {
		historyRecords, err := readHistory(options.HistoryPath, checkReimburse, checkAccount)
		if err != nil {
			return ManageResult{}, err
		}
		for _, item := range historyRecords {
			historyIndex[buildKey(item["发票代码"], item["发票号码"])] = item
			if strings.TrimSpace(item["发票代码"]) != "" {
				historyIndex[buildKey("", item["发票号码"])] = item
			}
		}
	}

	applyHeaderCheck(records, company)
	batchKeys := buildBatchKeys(records)
	duplicateRiskCount := applyStatusColumns(records, batchKeys, historyIndex, checkDuplicate, checkReimburse, checkAccount)
	finalRecords := filterFinalRecords(records)
	validCount, successCount, failedCount, errorAlerts := summarize(finalRecords)

	message := "处理完成"
	if len(finalRecords) == 0 {
		message = emptyFinalMessage
	}
	rows := make([]map[string]any, 0, len(finalRecords))
	for _, item := range finalRecords {
		row := make(map[string]any, len(excelx.BatchManageFinalHeaders)+1)
		for _, header := range excelx.BatchManageFinalHeaders {
			value := item[header]
			if header == excelx.TaxBureauVerifyStatusHeader {
				value = localizeFinalVerifyMessage(value)
			}
			row[header] = value
		}
		if item[excelx.SourcePathKey] != "" {
			row[excelx.SourcePathKey] = item[excelx.SourcePathKey]
		}
		rows = append(rows, row)
	}
	actualPath, err := excelx.WriteWorkbook(outputPath, excelx.WorkbookSpec{
		SheetName:     excelx.DefaultSheetName,
		Headers:       excelx.BatchManageFinalHeaders,
		Rows:          rows,
		AmountHeaders: excelx.FinalAmountHeaders,
		Widths:        excelx.BatchManageWidths,
		Hyperlinks:    true,
	}, now())
	if err != nil {
		return ManageResult{}, err
	}

	return ManageResult{Success: contract.BatchManageSuccess{
		Status:  contract.StatusSuccess,
		Message: message,
		Data: contract.BatchManageData{
			ExcelFilePath:         actualPath,
			IntermediateExcelPath: inputPath,
			CompanyName:           company,
			TotalFileCount:        len(records),
			ValidInvoiceCount:     validCount,
			VerifySuccessCount:    successCount,
			VerifyFailedCount:     failedCount,
			DuplicateRiskCount:    duplicateRiskCount,
			NewListCount:          len(finalRecords),
			ErrorAlerts:           errorAlerts,
		},
	}}, nil
}

func readRecords(path string) ([]record, error) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取Excel失败: %w", err)
	}
	defer book.Close()
	sheet := book.GetSheetName(book.GetActiveSheetIndex())
	if strings.TrimSpace(sheet) == "" {
		sheets := book.GetSheetList()
		if len(sheets) == 0 {
			return nil, errors.New("读取Excel失败: 工作簿无工作表")
		}
		sheet = sheets[0]
	}
	rows, err := book.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("读取Excel失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	headers := make([]string, len(rows[0]))
	for idx, header := range rows[0] {
		headers[idx] = strings.TrimSpace(strings.TrimPrefix(header, "\ufeff"))
	}
	if missing := missingHeaders(headers, excelx.IntermediateVerifyHeaders); len(missing) > 0 {
		return nil, fmt.Errorf("输入文件缺少必需列: %s", strings.Join(missing, ", "))
	}
	result := make([]record, 0, len(rows)-1)
	for rowIndex := 2; rowIndex <= len(rows); rowIndex++ {
		item := record{}
		for colIndex, header := range headers {
			if strings.TrimSpace(header) == "" {
				continue
			}
			cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex)
			if err != nil {
				return nil, err
			}
			formula, _ := book.GetCellFormula(sheet, cell)
			if header == "文件名" && strings.HasPrefix(strings.ToUpper(formula), "HYPERLINK(") {
				target, display := parseHyperlinkFormula(formula)
				item[header] = display
				item[excelx.SourcePathKey] = target
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
		}
		result = append(result, item)
	}
	return result, nil
}

func readHistory(path string, checkReimburse, checkAccount bool) ([]record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("历史清单CSV文件不存在: %s", path)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("历史清单CSV缺少表头")
		}
		return nil, fmt.Errorf("读取历史清单CSV失败: %w", err)
	}
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}
	index := map[string]int{}
	for i, header := range headers {
		index[strings.TrimSpace(header)] = i
	}
	requiredHeaders := []string{"发票号码"}
	if checkReimburse || checkAccount {
		requiredHeaders = append(requiredHeaders, "发票代码")
	}
	if checkReimburse {
		requiredHeaders = append(requiredHeaders, "报销状态")
	}
	if checkAccount {
		requiredHeaders = append(requiredHeaders, "入账状态")
	}
	for _, required := range requiredHeaders {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("历史清单CSV缺少必需列: %s", required)
		}
	}
	var records []record
	for rowNumber := 2; ; rowNumber++ {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取历史清单CSV失败: %w", err)
		}
		item := record{}
		for header, col := range index {
			if col < len(row) {
				item[header] = strings.TrimSpace(row[col])
			}
		}
		if item["发票号码"] == "" {
			return nil, fmt.Errorf("历史清单CSV第%d行: 发票号码不能为空", rowNumber)
		}
		if checkReimburse && !validValue(item["报销状态"], "已报销", "未报销") {
			return nil, fmt.Errorf("历史清单CSV第%d行: 报销状态值无效，只能是 '已报销' 或 '未报销'", rowNumber)
		}
		if checkAccount && !validValue(item["入账状态"], "已入账", "未入账") {
			return nil, fmt.Errorf("历史清单CSV第%d行: 入账状态值无效，只能是 '已入账' 或 '未入账'", rowNumber)
		}
		records = append(records, item)
	}
	return records, nil
}

func resolveCompany(records []record, explicit string) (string, []string) {
	explicit = strings.TrimSpace(explicit)
	names := collectCompanyNames(records)
	if explicit != "" {
		return explicit, names
	}
	if len(names) == 1 {
		return names[0], names
	}
	return "", names
}

func collectCompanyNames(records []record) []string {
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

func applyHeaderCheck(records []record, company string) {
	if strings.TrimSpace(company) == "" {
		return
	}
	for _, item := range records {
		if strings.TrimSpace(item[excelx.TaxBureauVerifyStatusHeader]) == "查验成功" && strings.TrimSpace(item["购买方名称"]) != company {
			item[excelx.TaxBureauVerifyStatusHeader] = headerMismatchStatus
		}
	}
}

func buildBatchKeys(records []record) map[string]int {
	counts := map[string]int{}
	for _, item := range records {
		item["发票商品名称"] = extractFirstProductName(item["商品明细"])
		if strings.TrimSpace(item[excelx.TaxBureauVerifyStatusHeader]) == "查验成功" && strings.TrimSpace(item["发票号码"]) != "" {
			counts[buildKey(item["发票代码"], item["发票号码"])]++
		}
	}
	return counts
}

func applyStatusColumns(records []record, batchKeys map[string]int, history map[string]record, checkDuplicate, checkReimburse, checkAccount bool) int {
	duplicateRiskCount := 0
	for _, item := range records {
		verifyStatus := strings.TrimSpace(item[excelx.TaxBureauVerifyStatusHeader])
		invoiceNumber := strings.TrimSpace(item["发票号码"])
		if verifyStatus != "查验成功" || invoiceNumber == "" {
			item["是否重复风险"] = ""
			item["报销状态"] = ""
			item["入账状态"] = ""
			item["处理建议"] = "建议检查发票信息并重新上传查验"
			continue
		}
		key := buildKey(item["发票代码"], invoiceNumber)
		duplicateRisk := ""
		if checkDuplicate {
			duplicateRisk = checkDuplicateRisk(key, invoiceNumber, batchKeys, history)
		}
		item["是否重复风险"] = duplicateRisk
		if duplicateRisk != "" && duplicateRisk != "无重复风险" {
			duplicateRiskCount++
		}
		reimburse := "未报销"
		if checkReimburse {
			reimburse = checkReimburseStatus(key, batchKeys, history)
		}
		account := "未入账"
		if checkAccount {
			account = checkAccountStatus(key, batchKeys, history)
		}
		item["报销状态"] = reimburse
		item["入账状态"] = account
		item["处理建议"] = generateAdvice(verifyStatus, reimburse, account)
	}
	return duplicateRiskCount
}

func filterFinalRecords(records []record) []record {
	filtered := make([]record, 0, len(records))
	for _, item := range records {
		if !excelx.IsUnsupportedVerifyStatus(item[excelx.TaxBureauVerifyStatusHeader]) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func summarize(records []record) (int, int, int, map[string][]string) {
	validCount := 0
	successCount := 0
	failedCount := 0
	errorAlerts := map[string][]string{}
	for _, item := range records {
		if strings.TrimSpace(item["发票号码"]) != "" {
			validCount++
		}
		status := strings.TrimSpace(item[excelx.TaxBureauVerifyStatusHeader])
		if status == "查验成功" {
			successCount++
		} else {
			failedCount++
			if status != "" {
				filename := strings.TrimSpace(item["文件名"])
				if filename == "" {
					filename = strings.TrimSpace(item[excelx.SourcePathKey])
				}
				if filename == "" {
					filename = strings.TrimSpace(item["发票号码"])
				}
				errorAlerts[status] = append(errorAlerts[status], filename)
			}
		}
	}
	return validCount, successCount, failedCount, errorAlerts
}

func buildKey(code, number string) string {
	code = normalizeInvoiceCode(code)
	number = strings.TrimSpace(number)
	if code != "" {
		return code + "_" + number
	}
	return number
}

func normalizeInvoiceCode(value string) string {
	text := strings.TrimSpace(value)
	if text == "--" || text == "" || strings.EqualFold(text, "nan") || strings.EqualFold(text, "none") {
		return ""
	}
	return text
}

func checkDuplicateRisk(key, number string, batchKeys map[string]int, history map[string]record) string {
	if batchKeys[key] > 1 {
		return "重复风险（本次存在相同发票）"
	}
	if _, ok := history[key]; ok {
		return "历史已处理（本次发票在历史清单已存在）"
	}
	if _, ok := history[number]; ok {
		return "历史已处理（本次发票在历史清单已存在）"
	}
	return "无重复风险"
}

func checkReimburseStatus(key string, batchKeys map[string]int, history map[string]record) string {
	if batchKeys[key] > 1 {
		return "疑似重复报销"
	}
	if item, ok := history[key]; ok {
		switch strings.TrimSpace(item["报销状态"]) {
		case "已报销":
			return "已报销"
		case "未报销":
			return "疑似重复报销"
		}
	}
	return "未报销"
}

func checkAccountStatus(key string, batchKeys map[string]int, history map[string]record) string {
	if batchKeys[key] > 1 {
		return "疑似重复入账"
	}
	if item, ok := history[key]; ok {
		switch strings.TrimSpace(item["入账状态"]) {
		case "已入账":
			return "已入账"
		case "未入账":
			return "疑似重复入账"
		}
	}
	return "未入账"
}

func generateAdvice(verifyStatus, reimburseStatus, accountStatus string) string {
	if verifyStatus != "查验成功" {
		return "建议检查发票信息并重新上传查验"
	}
	if reimburseStatus == "已报销" || accountStatus == "已入账" {
		return "建议拦截，避免重复报销/入账"
	}
	if reimburseStatus == "疑似重复报销" {
		return "建议人工复核后再报销"
	}
	if reimburseStatus == "未报销" && accountStatus == "疑似重复入账" {
		return "建议人工复核后再入账"
	}
	return "可进入报销/入账流程"
}

func extractFirstProductName(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return ""
	}
	var first map[string]any
	switch items := decoded.(type) {
	case []any:
		if len(items) == 0 {
			return ""
		}
		first, _ = items[0].(map[string]any)
	case map[string]any:
		first = items
	}
	if first == nil {
		return ""
	}
	for _, key := range []string{"Hwhyslwfwmc", "hwhyslwfwmc"} {
		if value, ok := first[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
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

func validValue(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
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

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

func pathDir(path string) string {
	dir := filepath.Dir(path)
	if strings.TrimSpace(dir) == "" {
		return "."
	}
	return dir
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
