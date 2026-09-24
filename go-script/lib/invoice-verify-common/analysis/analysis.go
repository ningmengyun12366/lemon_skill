package analysis

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"

	"github.com/xuri/excelize/v2"
)

const EmptyMessage = "当前没有符合条件的发票数据，结果为空"

type Options struct {
	InputPath string
	Output    string
	Company   string
	Now       func() time.Time
}

type record map[string]string

type aggregate struct {
	Count  int
	Amount *big.Rat
}

type amountItem struct {
	Name   string
	Amount *big.Rat
}

type quantityItem struct {
	Name     string
	Quantity *big.Rat
}

func Run(options Options) (contract.AnalysisSuccess, error) {
	now := resolveNow(options.Now)
	company := strings.TrimSpace(options.Company)
	if company == "" {
		return contract.AnalysisSuccess{}, errors.New("missing required company name")
	}
	inputPath, err := pathx.Normalize(options.InputPath)
	if err != nil {
		return contract.AnalysisSuccess{}, err
	}
	defaultName := pathx.BuildDatedFilename("企业发票查验结果清单", ".xlsx", now())
	outputPath, err := pathx.ResolveOutput(options.Output, filepath.Dir(inputPath), defaultName, ".xlsx")
	if err != nil {
		return contract.AnalysisSuccess{}, err
	}
	if samePath(inputPath, outputPath) {
		return contract.AnalysisSuccess{}, errors.New("分析输出路径不能覆盖查验结果文件")
	}

	records, headers, err := readVerifyRecords(inputPath)
	if err != nil {
		return contract.AnalysisSuccess{}, err
	}
	if missing := missingHeaders(headers, excelx.AnalysisInputHeaders); len(missing) > 0 {
		return contract.AnalysisSuccess{}, fmt.Errorf("输入文件缺少必需列: %s", strings.Join(missing, ", "))
	}

	outputRecords := filterRecordsForFinalOutput(records)
	for _, item := range outputRecords {
		item["进项销项"] = DetermineDirection(company, item["购买方名称"], item["销售方名称"])
	}
	checkDuplicateInBatch(outputRecords)
	stats := analyze(outputRecords)
	actualOutput, err := writeAnalysisWorkbook(outputPath, outputRecords, now())
	if err != nil {
		return contract.AnalysisSuccess{}, err
	}

	result := contract.AnalysisSuccess{
		InputFile:   inputPath,
		OutputFile:  actualOutput,
		CompanyName: company,
		RecordCount: len(records),
		Analysis:    stats,
	}
	if len(outputRecords) == 0 {
		result.Message = EmptyMessage
	}
	return result, nil
}

func DetermineDirection(companyName, buyerName, sellerName string) string {
	company := strings.TrimSpace(companyName)
	buyer := strings.TrimSpace(buyerName)
	seller := strings.TrimSpace(sellerName)
	if company == buyer {
		return "进项"
	}
	if company == seller {
		return "销项"
	}
	return "未知"
}

func analyze(records []record) contract.AnalysisData {
	verifySuccess := 0
	verifyFailed := 0
	verifySkipped := 0
	validInvoiceCount := 0
	verifyFailedDetails := []map[string]string{}
	verifyFailedReasonCounts := map[string]int{}
	verifyFailedReasonOrder := []string{}

	inputCount := 0
	outputCount := 0
	unknownCount := 0
	inputAmount := ratZero()
	outputAmount := ratZero()
	unknownNetAmount := ratZero()
	unknownTaxAmount := ratZero()
	unknownAmount := ratZero()
	inputNetAmount := ratZero()
	inputTaxAmount := ratZero()
	outputNetAmount := ratZero()
	outputTaxAmount := ratZero()
	totalNetAmount := ratZero()
	totalTaxAmount := ratZero()
	totalAmount := ratZero()

	typeDistributionRaw := map[string]*aggregate{}
	monthlyCountRaw := map[string]int{}
	monthlyAmountRaw := map[string]*big.Rat{}
	monthlyInputCount := map[string]int{}
	monthlyOutputCount := map[string]int{}
	monthlyInputAmount := map[string]*big.Rat{}
	monthlyOutputAmount := map[string]*big.Rat{}

	purchaseQuantities := map[string]*big.Rat{}
	salesQuantities := map[string]*big.Rat{}
	purchaseCategories := map[string]string{}
	salesCategories := map[string]string{}
	purchaseTotalQuantity := ratZero()
	salesTotalQuantity := ratZero()

	customerAmounts := map[string]*big.Rat{}
	supplierAmounts := map[string]*big.Rat{}
	customerCounts := map[string]int{}
	supplierCounts := map[string]int{}

	duplicateCount := 0
	abnormalStatusCount := 0
	unknownDirectionCount := 0
	normalCount := 0
	revokedCount := 0
	cancelledCount := 0

	for _, item := range records {
		invoiceNumber := strings.TrimSpace(item["发票号码"])
		if invoiceNumber != "" {
			validInvoiceCount++
		}

		verifyStatus := strings.TrimSpace(item[excelx.TaxBureauVerifyStatusHeader])
		if verifyStatus == "" || strings.Contains(verifyStatus, "跳过") {
			verifySkipped++
			continue
		}
		if verifyStatus == "查验成功" {
			verifySuccess++
		} else {
			verifyFailed++
			fileName := strings.TrimSpace(item["文件名"])
			if _, ok := verifyFailedReasonCounts[verifyStatus]; !ok {
				verifyFailedReasonOrder = append(verifyFailedReasonOrder, verifyStatus)
			}
			verifyFailedReasonCounts[verifyStatus]++
			verifyFailedDetails = append(verifyFailedDetails, map[string]string{"file_name": fileName, "reason": verifyStatus})
			continue
		}

		duplicateRisk := strings.TrimSpace(item["是否重复风险"])
		if duplicateRisk != "" && duplicateRisk != "无重复风险" {
			duplicateCount++
		}

		invoiceStatus := strings.TrimSpace(item["发票状态"])
		isRevoked := strings.Contains(invoiceStatus, "红冲")
		if isRevoked {
			revokedCount++
			abnormalStatusCount++
		}
		if strings.Contains(invoiceStatus, "作废") {
			cancelledCount++
			abnormalStatusCount++
			continue
		}
		if !isRevoked {
			normalCount++
		}

		direction := strings.TrimSpace(item["进项销项"])
		if direction == "" {
			direction = "未知"
		}
		totalAmountValue := parseDecimal(item["价税合计"])
		netAmount := parseDecimal(item["不含税金额"])
		taxAmount := parseDecimal(item["税额"])

		switch direction {
		case "进项":
			inputCount++
			addInPlace(inputAmount, totalAmountValue)
			addInPlace(inputNetAmount, netAmount)
			addInPlace(inputTaxAmount, taxAmount)
			supplierName := strings.TrimSpace(item["销售方名称"])
			if supplierName != "" {
				addToRatMap(supplierAmounts, supplierName, netAmount)
				supplierCounts[supplierName]++
			}
		case "销项":
			outputCount++
			addInPlace(outputAmount, totalAmountValue)
			addInPlace(outputNetAmount, netAmount)
			addInPlace(outputTaxAmount, taxAmount)
			customerName := strings.TrimSpace(item["购买方名称"])
			if customerName != "" {
				addToRatMap(customerAmounts, customerName, netAmount)
				customerCounts[customerName]++
			}
		default:
			unknownCount++
			unknownDirectionCount++
			addInPlace(unknownNetAmount, netAmount)
			addInPlace(unknownTaxAmount, taxAmount)
			addInPlace(unknownAmount, totalAmountValue)
			continue
		}

		addInPlace(totalNetAmount, netAmount)
		addInPlace(totalTaxAmount, taxAmount)
		addInPlace(totalAmount, totalAmountValue)
		invoiceType := strings.TrimSpace(item["发票类型"])
		if invoiceType == "" {
			invoiceType = "未知类型"
		}
		agg := typeDistributionRaw[invoiceType]
		if agg == nil {
			agg = &aggregate{Amount: ratZero()}
			typeDistributionRaw[invoiceType] = agg
		}
		agg.Count++
		addInPlace(agg.Amount, netAmount)

		if issueDate, ok := parseDate(item["开票日期"]); ok {
			monthKey := issueDate.Format("2006-01")
			monthlyCountRaw[monthKey]++
			addToRatMap(monthlyAmountRaw, monthKey, netAmount)
			if direction == "进项" {
				monthlyInputCount[monthKey]++
				addToRatMap(monthlyInputAmount, monthKey, netAmount)
			} else if direction == "销项" {
				monthlyOutputCount[monthKey]++
				addToRatMap(monthlyOutputAmount, monthKey, netAmount)
			}
		}

		for _, product := range parseProductDetails(item["商品明细"]) {
			if direction == "进项" {
				addToRatMap(purchaseQuantities, product.Name, product.Quantity)
				purchaseCategories[product.Name] = product.Category
				addInPlace(purchaseTotalQuantity, product.Quantity)
			} else if direction == "销项" {
				addToRatMap(salesQuantities, product.Name, product.Quantity)
				salesCategories[product.Name] = product.Category
				addInPlace(salesTotalQuantity, product.Quantity)
			}
		}
	}

	verifyFailedReasonSummary := make([]map[string]any, 0, len(verifyFailedReasonOrder))
	for _, reason := range verifyFailedReasonOrder {
		verifyFailedReasonSummary = append(verifyFailedReasonSummary, map[string]any{"reason": reason, "count": verifyFailedReasonCounts[reason]})
	}

	monthlyCount, monthlyAmount := buildMonthlyStats(monthlyCountRaw, monthlyAmountRaw, monthlyInputCount, monthlyOutputCount, monthlyInputAmount, monthlyOutputAmount)
	typeDistribution := buildTypeDistribution(typeDistributionRaw)
	top5Purchase := buildTopQuantity(purchaseQuantities, purchaseCategories, purchaseTotalQuantity)
	top5Sales := buildTopQuantity(salesQuantities, salesCategories, salesTotalQuantity)
	top10Customers, top10CustomerTotalAmount, top10CustomerTotalCount := buildTopAmounts(customerAmounts, customerCounts, outputNetAmount)
	top10Suppliers, top10SupplierTotalAmount, top10SupplierTotalCount := buildTopAmounts(supplierAmounts, supplierCounts, inputNetAmount)

	top10CustomerTotalRatio := ratioFloat(top10CustomerTotalAmount, outputNetAmount)
	top10SupplierTotalRatio := ratioFloat(top10SupplierTotalAmount, inputNetAmount)
	customerConcentrationTop5 := ratioFloat(sumTopAmounts(customerAmounts, 5), outputNetAmount)
	supplierConcentrationTop5 := ratioFloat(sumTopAmounts(supplierAmounts, 5), inputNetAmount)
	customerConcentrationTop10 := ratioFloat(sumTopAmounts(customerAmounts, 10), outputNetAmount)
	supplierConcentrationTop10 := ratioFloat(sumTopAmounts(supplierAmounts, 10), inputNetAmount)

	riskDetails := []string{}
	if duplicateCount > 0 {
		riskDetails = append(riskDetails, fmt.Sprintf("发现 %d 张重复风险发票", duplicateCount))
	}
	if abnormalStatusCount > 0 {
		riskDetails = append(riskDetails, fmt.Sprintf("发现 %d 张状态异常发票（红冲/作废）", abnormalStatusCount))
	}
	if unknownDirectionCount > 0 {
		riskDetails = append(riskDetails, fmt.Sprintf("发现 %d 张无法判断进项/销项方向的发票", unknownDirectionCount))
	}

	return contract.AnalysisData{
		VerifyStats: map[string]any{
			"total":                        len(records),
			"valid_invoice_count":          validInvoiceCount,
			"verify_success":               verifySuccess,
			"verify_failed":                verifyFailed,
			"verify_failed_details":        verifyFailedDetails,
			"verify_failed_reason_summary": verifyFailedReasonSummary,
			"verify_skipped":               verifySkipped,
		},
		StatusDistribution: map[string]any{
			"normal_count":    normalCount,
			"revoked_count":   revokedCount,
			"cancelled_count": cancelledCount,
		},
		Overview: map[string]any{
			"invoice_count":      inputCount + outputCount,
			"total_net_amount":   formatMoney(totalNetAmount),
			"total_tax_amount":   formatMoney(totalTaxAmount),
			"total_amount":       formatMoney(totalAmount),
			"input_count":        inputCount,
			"input_net_amount":   formatMoney(inputNetAmount),
			"input_tax_amount":   formatMoney(inputTaxAmount),
			"input_amount":       formatMoney(inputAmount),
			"output_count":       outputCount,
			"output_net_amount":  formatMoney(outputNetAmount),
			"output_tax_amount":  formatMoney(outputTaxAmount),
			"output_amount":      formatMoney(outputAmount),
			"unknown_count":      unknownCount,
			"unknown_net_amount": formatMoney(unknownNetAmount),
			"unknown_tax_amount": formatMoney(unknownTaxAmount),
			"unknown_amount":     formatMoney(unknownAmount),
			"type_distribution":  typeDistribution,
		},
		TimeStats: map[string]any{
			"monthly_count":  monthlyCount,
			"monthly_amount": monthlyAmount,
		},
		ProductStats: map[string]any{
			"total_quantity": ratToFloat(ratAddCopy(purchaseTotalQuantity, salesTotalQuantity)),
			"top5_purchase":  top5Purchase,
			"top5_sales":     top5Sales,
		},
		CounterpartyStats: map[string]any{
			"top10_customers": top10Customers,
			"top10_suppliers": top10Suppliers,
			"top10_customer_total": map[string]any{
				"amount": formatMoney(top10CustomerTotalAmount),
				"count":  top10CustomerTotalCount,
				"ratio":  top10CustomerTotalRatio,
			},
			"top10_supplier_total": map[string]any{
				"amount": formatMoney(top10SupplierTotalAmount),
				"count":  top10SupplierTotalCount,
				"ratio":  top10SupplierTotalRatio,
			},
			"customer_concentration_top5":  customerConcentrationTop5,
			"customer_concentration_top10": customerConcentrationTop10,
			"supplier_concentration_top5":  supplierConcentrationTop5,
			"supplier_concentration_top10": supplierConcentrationTop10,
			"customer_evaluation":          evaluateConcentration(customerConcentrationTop5),
			"supplier_evaluation":          evaluateConcentration(supplierConcentrationTop5),
		},
		RiskAlerts: map[string]any{
			"duplicate_count":         duplicateCount,
			"abnormal_status_count":   abnormalStatusCount,
			"unknown_direction_count": unknownDirectionCount,
			"details":                 riskDetails,
		},
	}
}

func readVerifyRecords(path string) ([]record, []string, error) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("读取Excel失败: %w", err)
	}
	defer book.Close()
	sheet := activeSheetName(book)
	rows, err := book.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, nil, fmt.Errorf("读取Excel失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	headers := make([]string, len(rows[0]))
	for idx, value := range rows[0] {
		headers[idx] = strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
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
				return nil, nil, err
			}
			formula, _ := book.GetCellFormula(sheet, cell)
			if header == "文件名" && isHyperlinkFormula(formula) {
				target, display := parseHyperlinkFormula(formula)
				if display == "" {
					display = formula
				}
				item[header] = display
				if target != "" {
					item[excelx.SourcePathKey] = localPathFromHyperlinkTarget(target)
				}
				continue
			}
			value, err := book.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
			if err != nil {
				return nil, nil, err
			}
			if header == "发票代码" {
				value = normalizeInvoiceCode(value)
			}
			item[header] = value
		}
		result = append(result, item)
	}
	return result, headers, nil
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

func filterRecordsForFinalOutput(records []record) []record {
	filtered := make([]record, 0, len(records))
	for _, item := range records {
		if excelx.IsUnsupportedVerifyStatus(item[excelx.TaxBureauVerifyStatusHeader]) {
			continue
		}
		copyItem := record{}
		for key, value := range item {
			copyItem[key] = value
		}
		filtered = append(filtered, copyItem)
	}
	return filtered
}

func checkDuplicateInBatch(records []record) {
	counts := map[string]int{}
	for _, item := range records {
		if strings.TrimSpace(item[excelx.TaxBureauVerifyStatusHeader]) != "查验成功" {
			continue
		}
		invoiceNumber := strings.TrimSpace(item["发票号码"])
		if invoiceNumber == "" {
			continue
		}
		counts[buildKey(item["发票代码"], invoiceNumber)]++
	}
	for _, item := range records {
		if strings.TrimSpace(item[excelx.TaxBureauVerifyStatusHeader]) != "查验成功" || strings.TrimSpace(item["发票号码"]) == "" {
			item["是否重复风险"] = ""
			continue
		}
		key := buildKey(item["发票代码"], item["发票号码"])
		if counts[key] > 1 {
			item["是否重复风险"] = "重复风险（本次存在相同发票）"
		} else {
			item["是否重复风险"] = "无重复风险"
		}
	}
}

func writeAnalysisWorkbook(path string, records []record, now time.Time) (string, error) {
	rows := make([]map[string]any, 0, len(records))
	for _, item := range records {
		row := make(map[string]any, len(excelx.AnalysisFinalHeaders)+1)
		for _, header := range excelx.AnalysisFinalHeaders {
			value := item[header]
			if header == excelx.TaxBureauVerifyStatusHeader {
				value = localizeFinalVerifyMessage(value)
			}
			row[header] = value
		}
		if sourcePath := strings.TrimSpace(item[excelx.SourcePathKey]); sourcePath != "" {
			row[excelx.SourcePathKey] = sourcePath
		}
		rows = append(rows, row)
	}
	return excelx.WriteWorkbook(path, excelx.WorkbookSpec{
		SheetName:     "发票分析结果",
		Headers:       excelx.AnalysisFinalHeaders,
		Rows:          rows,
		AmountHeaders: excelx.FinalAmountHeaders,
		Widths:        excelx.AnalysisWidths,
		Hyperlinks:    true,
	}, now)
}

type productDetail struct {
	Name     string
	Category string
	Quantity *big.Rat
}

func parseProductDetails(value string) []productDetail {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(text)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil
	}
	items, ok := decoded.([]any)
	if !ok {
		return nil
	}
	var result []productDetail
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(productFieldValue(item, "Xmmc"))
		if name == "" {
			continue
		}
		category := strings.TrimSpace(productFieldValue(item, "Spfwjc"))
		quantity := productQuantity(item)
		result = append(result, productDetail{Name: name, Category: category, Quantity: quantity})
	}
	return result
}

func productFieldValue(item map[string]any, keys ...string) string {
	for _, key := range keys {
		for field, raw := range item {
			if !strings.EqualFold(field, key) {
				continue
			}
			value := strings.TrimSpace(anyToString(raw))
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func productQuantity(item map[string]any) *big.Rat {
	value := productFieldValue(item, "Spsl")
	if value == "" {
		return ratFromInt(1)
	}
	return parseDecimal(value)
}

func buildMonthlyStats(monthlyCountRaw map[string]int, monthlyAmountRaw map[string]*big.Rat, monthlyInputCount, monthlyOutputCount map[string]int, monthlyInputAmount, monthlyOutputAmount map[string]*big.Rat) (map[string]any, map[string]any) {
	months := make([]string, 0, len(monthlyCountRaw))
	for month := range monthlyCountRaw {
		months = append(months, month)
	}
	sort.Strings(months)
	monthlyCount := map[string]any{}
	monthlyAmount := map[string]any{}
	var prevCount *big.Rat
	var prevAmount *big.Rat
	for _, month := range months {
		countValue := monthlyCountRaw[month]
		amountValue := ratValue(monthlyAmountRaw, month)
		countRat := ratFromInt(countValue)
		countRatio := "-"
		if prevCount != nil {
			countRatio = formatChangeRatio(countRat, prevCount)
		}
		amountRatio := "-"
		if prevAmount != nil {
			amountRatio = formatChangeRatio(amountValue, prevAmount)
		}
		monthlyCount[month] = map[string]any{
			"input_count":  monthlyInputCount[month],
			"output_count": monthlyOutputCount[month],
			"count":        countValue,
			"change_ratio": countRatio,
		}
		monthlyAmount[month] = map[string]any{
			"input_amount":  formatMoney(ratValue(monthlyInputAmount, month)),
			"output_amount": formatMoney(ratValue(monthlyOutputAmount, month)),
			"amount":        formatMoney(amountValue),
			"change_ratio":  amountRatio,
		}
		prevCount = countRat
		prevAmount = amountValue
	}
	return monthlyCount, monthlyAmount
}

func buildTypeDistribution(raw map[string]*aggregate) map[string]any {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := map[string]any{}
	for _, key := range keys {
		data := raw[key]
		result[key] = map[string]any{"count": data.Count, "amount": formatMoney(data.Amount)}
	}
	return result
}

func buildTopQuantity(values map[string]*big.Rat, categories map[string]string, total *big.Rat) []map[string]any {
	items := topQuantityItems(values, 5)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ratio := "0.0%"
		if total.Sign() > 0 {
			ratio = formatPercent(item.Quantity, total)
		}
		result = append(result, map[string]any{
			"name":     item.Name,
			"category": categories[item.Name],
			"quantity": ratToFloat(item.Quantity),
			"ratio":    ratio,
		})
	}
	return result
}

func buildTopAmounts(values map[string]*big.Rat, counts map[string]int, total *big.Rat) ([]map[string]any, *big.Rat, int) {
	items := topAmountItems(values, 10)
	result := make([]map[string]any, 0, len(items))
	totalAmount := ratZero()
	totalCount := 0
	for _, item := range items {
		addInPlace(totalAmount, item.Amount)
		totalCount += counts[item.Name]
		result = append(result, map[string]any{
			"name":   item.Name,
			"amount": formatMoney(item.Amount),
			"count":  counts[item.Name],
			"ratio":  ratioFloat(item.Amount, total),
		})
	}
	return result, totalAmount, totalCount
}

func topQuantityItems(values map[string]*big.Rat, limit int) []quantityItem {
	items := make([]quantityItem, 0, len(values))
	for name, quantity := range values {
		items = append(items, quantityItem{Name: name, Quantity: ratCopy(quantity)})
	}
	sort.Slice(items, func(i, j int) bool {
		cmp := items[i].Quantity.Cmp(items[j].Quantity)
		if cmp == 0 {
			return items[i].Name < items[j].Name
		}
		return cmp > 0
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func topAmountItems(values map[string]*big.Rat, limit int) []amountItem {
	items := make([]amountItem, 0, len(values))
	for name, amount := range values {
		items = append(items, amountItem{Name: name, Amount: ratCopy(amount)})
	}
	sort.Slice(items, func(i, j int) bool {
		cmp := items[i].Amount.Cmp(items[j].Amount)
		if cmp == 0 {
			return items[i].Name < items[j].Name
		}
		return cmp > 0
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func sumTopAmounts(values map[string]*big.Rat, limit int) *big.Rat {
	sum := ratZero()
	for _, item := range topAmountItems(values, limit) {
		addInPlace(sum, item.Amount)
	}
	return sum
}

func parseDate(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006.01.02", "2006-01-02 15:04:05", "2006/01/02 15:04:05", "20060102"} {
		parsed, err := time.ParseInLocation(layout, text, time.Local)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func evaluateConcentration(ratio float64) string {
	switch {
	case ratio < 0.10:
		return "极低"
	case ratio < 0.30:
		return "低"
	case ratio < 0.50:
		return "中"
	case ratio < 0.70:
		return "高"
	default:
		return "极高"
	}
}

func formatChangeRatio(current, previous *big.Rat) string {
	if previous.Sign() == 0 {
		return "-"
	}
	delta := new(big.Rat).Sub(current, previous)
	delta.Quo(delta, previous)
	delta.Mul(delta, ratFromInt(100))
	return formatFixed(delta, 1, false) + "%"
}

func formatPercent(part, total *big.Rat) string {
	if total.Sign() == 0 {
		return "0.0%"
	}
	ratio := new(big.Rat).Quo(part, total)
	ratio.Mul(ratio, ratFromInt(100))
	return formatFixed(ratio, 1, false) + "%"
}

func ratioFloat(part, total *big.Rat) float64 {
	if total.Sign() <= 0 {
		return 0
	}
	ratio := new(big.Rat).Quo(part, total)
	return roundedRatToFloat(ratio, 4)
}

func formatMoney(value *big.Rat) string {
	return formatFixed(value, 2, true)
}

func formatFixed(value *big.Rat, places int, thousands bool) string {
	if value == nil {
		value = ratZero()
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil)
	scaled := roundScaled(value, scale)
	if scaled.Sign() == 0 {
		return zeroFixed(places)
	}
	negative := scaled.Sign() < 0
	if negative {
		scaled.Abs(scaled)
	}
	digits := scaled.String()
	if places > 0 {
		for len(digits) <= places {
			digits = "0" + digits
		}
		integerPart := digits[:len(digits)-places]
		fractionPart := digits[len(digits)-places:]
		if thousands {
			integerPart = addThousands(integerPart)
		}
		if negative {
			return "-" + integerPart + "." + fractionPart
		}
		return integerPart + "." + fractionPart
	}
	if thousands {
		digits = addThousands(digits)
	}
	if negative {
		return "-" + digits
	}
	return digits
}

func zeroFixed(places int) string {
	if places <= 0 {
		return "0"
	}
	return "0." + strings.Repeat("0", places)
}

func roundScaled(value *big.Rat, scale *big.Int) *big.Int {
	if value == nil || value.Sign() == 0 {
		return big.NewInt(0)
	}
	num := new(big.Int).Set(value.Num())
	den := new(big.Int).Set(value.Denom())
	negative := num.Sign() < 0
	if negative {
		num.Abs(num)
	}
	num.Mul(num, scale)
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	doubledRemainder := new(big.Int).Mul(remainder, big.NewInt(2))
	if doubledRemainder.Cmp(den) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if negative {
		quotient.Neg(quotient)
	}
	return quotient
}

func roundedRatToFloat(value *big.Rat, places int) float64 {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil)
	integer := roundScaled(value, scale)
	rat := new(big.Rat).SetFrac(integer, scale)
	result, _ := rat.Float64()
	return result
}

func addThousands(value string) string {
	if len(value) <= 3 {
		return value
	}
	first := len(value) % 3
	if first == 0 {
		first = 3
	}
	var builder strings.Builder
	builder.WriteString(value[:first])
	for i := first; i < len(value); i += 3 {
		builder.WriteByte(',')
		builder.WriteString(value[i : i+3])
	}
	return builder.String()
}

func parseDecimal(value string) *big.Rat {
	text := strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if text == "" {
		return ratZero()
	}
	parsed, ok := new(big.Rat).SetString(text)
	if !ok {
		return ratZero()
	}
	return parsed
}

func addToRatMap(values map[string]*big.Rat, key string, amount *big.Rat) {
	if values[key] == nil {
		values[key] = ratZero()
	}
	addInPlace(values[key], amount)
}

func ratValue(values map[string]*big.Rat, key string) *big.Rat {
	if value := values[key]; value != nil {
		return ratCopy(value)
	}
	return ratZero()
}

func ratZero() *big.Rat {
	return new(big.Rat)
}

func ratFromInt(value int) *big.Rat {
	return new(big.Rat).SetInt64(int64(value))
}

func ratCopy(value *big.Rat) *big.Rat {
	if value == nil {
		return ratZero()
	}
	return new(big.Rat).Set(value)
}

func ratAddCopy(left, right *big.Rat) *big.Rat {
	return new(big.Rat).Add(ratCopy(left), ratCopy(right))
}

func addInPlace(target, value *big.Rat) {
	target.Add(target, value)
}

func ratToFloat(value *big.Rat) float64 {
	result, _ := value.Float64()
	return result
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
	if text == "" || text == "--" || strings.EqualFold(text, "nan") || strings.EqualFold(text, "none") {
		return ""
	}
	return text
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
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

func isHyperlinkFormula(formula string) bool {
	inner := strings.TrimSpace(strings.TrimPrefix(formula, "="))
	return len(inner) >= len("HYPERLINK(") && strings.EqualFold(inner[:len("HYPERLINK(")], "HYPERLINK(")
}

func parseHyperlinkFormula(formula string) (string, string) {
	inner := strings.TrimSpace(strings.TrimPrefix(formula, "="))
	if isHyperlinkFormula(inner) {
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
	return "", ""
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

func localPathFromHyperlinkTarget(target string) string {
	trimmed := strings.TrimSpace(target)
	parsed, err := url.Parse(trimmed)
	if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
		return trimmed
	}
	pathValue, err := url.PathUnescape(parsed.Path)
	if err != nil {
		pathValue = parsed.Path
	}
	if parsed.Host != "" {
		return `\\` + parsed.Host + filepath.FromSlash(pathValue)
	}
	if len(pathValue) >= 3 && pathValue[0] == '/' && pathValue[2] == ':' {
		pathValue = pathValue[1:]
	}
	return filepath.FromSlash(pathValue)
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

func resolveNow(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return time.Now
}
