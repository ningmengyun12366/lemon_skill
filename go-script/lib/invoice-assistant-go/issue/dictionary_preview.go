package issue

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
	"invoice-assistant-skill/go-script/lib/skillupdate"
)

var productEntriesCache []map[string]any
var productClassifyCodeDetailCache map[string]map[string]any
var commonDictCache map[string][]map[string]string
var fetchProductClassifyCodeDetail = fetchProductClassifyCodeDetailFromAPI

func getProductCodeByProjectName(projectName any) string {
	query := normalizeProductSearchText(projectName)
	if query == "" {
		return ""
	}
	bestScore := 0.0
	bestCode := ""
	for _, entry := range loadProductEntries() {
		code := common.StringValue(entry["spbm"])
		keywords, _ := entry["keywords"].([]string)
		for _, keyword := range keywords {
			score := scoreProductKeywordMatch(query, keyword)
			if score > bestScore {
				bestScore = score
				bestCode = code
			}
		}
	}
	if bestScore >= 0.55 {
		return bestCode
	}
	return ""
}

/**
 * getProductNameBySPBM 获取商品编码对应的商品名称。
 *
 * @param spbm 税收分类编码。
 * @returns 自动赋码时的原商品名；没有推荐缓存时返回商品详情中的货物劳务名称。
 */
func getProductNameBySPBM(spbm any) string {
	code := strings.TrimSpace(common.StringValue(spbm))
	if recommendation, ok := getCachedTaxCodingRecommendation(code); ok && recommendation.ProductName != "" {
		return recommendation.ProductName
	}
	return strings.TrimSpace(common.StringValue(getProductClassifyCodeDetail(code)["Hwhlwmc"]))
}

func getProductClassifyCodeDetail(spbm any) map[string]any {
	detail, _ := getProductClassifyCodeDetailChecked(spbm)
	return detail
}

// getProductClassifyCodeDetailChecked keeps ordinary lookup failures optional but never hides an outdated Skill.
func getProductClassifyCodeDetailChecked(spbm any) (map[string]any, error) {
	code := strings.TrimSpace(common.StringValue(spbm))
	if code == "" {
		return nil, nil
	}
	if productClassifyCodeDetailCache == nil {
		productClassifyCodeDetailCache = map[string]map[string]any{}
	}
	if detail, ok := productClassifyCodeDetailCache[code]; ok {
		return detail, nil
	}
	detail, err := fetchProductClassifyCodeDetail(code)
	if err != nil {
		if outdated, ok := skillupdate.FindOutdatedError(err); ok {
			return nil, outdated
		}
		return nil, nil
	}
	productClassifyCodeDetailCache[code] = detail
	return detail, nil
}

func fetchProductClassifyCodeDetailFromAPI(code string) (map[string]any, error) {
	resp, err := postIssueJSONWithRetry("/getProductClassifyCodeDetail", map[string]any{"sphfwssflhbbm": code}, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if responseCode(resp) != 0 {
		return nil, fmt.Errorf("get product classify code detail failed: %s", common.StringValue(resp["message"]))
	}
	return mapProductClassifyCodeDetailResponse(resp), nil
}

func mapProductClassifyCodeDetailResponse(resp map[string]any) map[string]any {
	return map[string]any{
		"Sllb":           GetPath(resp, "data.classifyCodeKeyword.sllb"),
		"SllbSmallScale": GetPath(resp, "data.classifyCodeKeyword.sllbSmallScale"),
		"Sphfwssflhbbm":  GetPath(resp, "data.classifyCode.sphfwssflhbbm"),
		"Hwhlwmc":        GetPath(resp, "data.classifyCode.hwhlwmc"),
		"Spfwjc":         GetPath(resp, "data.classifyCode.spfwjc"),
		"Gjz":            GetPath(resp, "data.classifyCodeKeyword.gjz"),
	}
}

func resolveCommonDictCode(dictionaryName string, rawValue any) (string, string) {
	text := strings.TrimSpace(common.StringValue(rawValue))
	if text == "" {
		return "", ""
	}
	options := loadCommonDictEntries()[dictionaryName]
	if len(options) == 0 {
		return text, ""
	}
	normalized := normalizeProductSearchText(text)
	for _, option := range options {
		if text == option["value"] || normalized == normalizeProductSearchText(option["label"]) {
			return option["value"], ""
		}
	}
	examples := []string{}
	for idx, option := range options {
		if idx >= 3 {
			break
		}
		examples = append(examples, option["label"]+"="+option["value"])
	}
	return "", fmt.Sprintf("%s 未匹配到字典编码：%s；示例：%s", dictionaryName, text, strings.Join(examples, "、"))
}

func BuildPreviewMarkdown(payload map[string]any, previewHTMLPath string) string {
	entries := extractPreviewMarkdownInvoiceEntries(payload)
	lines := []string{"**发票预览**"}
	previewLink := formatPreviewSampleLink(previewHTMLPath)

	if len(entries) > 0 {
		appendMarkdownBlock(&lines, fmt.Sprintf("本次将开具 %d 张发票。", len(entries)))
	}

	for _, invoice := range entries {
		basic, _ := invoice["basicInfo"].(map[string]any)
		seller, _ := invoice["sellerInfo"].(map[string]any)
		purchaser, _ := invoice["purchaserInfo"].(map[string]any)
		detail, _ := invoice["invoiceDetail"].(map[string]any)
		note, _ := invoice["noteInfo"].(map[string]any)
		previewRemarks := strings.Join(buildPreviewRemarkLines(invoice, note), "\n")
		previewTotals := computePreviewInvoiceTotals(invoice)

		fplsh := strings.TrimSpace(common.StringValue(invoice["fplsh"]))
		if fplsh == "" {
			fplsh = "未提供"
		}
		appendMarkdownBlock(&lines, fmt.Sprintf("**发票流水号：%s**", fplsh))
		appendMarkdownBlock(&lines, "【发票基础信息】")
		appendMarkdownBlock(&lines, "- 销售方名称："+strings.TrimSpace(common.StringValue(seller["name"])))
		appendMarkdownBlock(&lines, "- 购买方名称："+strings.TrimSpace(common.StringValue(purchaser["name"])))
		appendMarkdownBlock(&lines, "- 购买方税号："+strings.TrimSpace(common.StringValue(purchaser["uscc"])))
		appendMarkdownBlock(&lines, "- 发票种类："+strings.TrimSpace(common.StringValue(basic["invoiceType"])))
		appendMarkdownBlock(&lines, "- 价税合计："+formatCurrencyMarkdownText(previewTotals["total"]))
		appendMarkdownBlock(&lines, "- 备注："+previewRemarks)
		appendMarkdownBlock(&lines, "【商品明细】")

		items, _ := asList(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.data"))
		if len(items) == 0 {
			items, _ = asList(detail["data"])
		}
		detailIndex := 1
		for _, itemAny := range items {
			item, _ := itemAny.(map[string]any)
			if item == nil {
				continue
			}
			appendMarkdownDetailItem(&lines, detailIndex, buildMarkdownPreviewItemName(item, detailIndex), item, formatPreviewTaxAmount(item), formatLineItemTotalWithTax(item))
			detailIndex++

			discount, _ := item["discountInfo"].(map[string]any)
			if discountAmount, ok := ToDecimal(discount["discountAmount"]); ok && !discountAmount.IsZero() {
				discountTax, _ := ToDecimal(discount["discountTax"])
				appendMarkdownDetailItem(&lines, detailIndex, buildDiscountMarkdownPreviewItemName(item, detailIndex), item, formatNegativeCurrencyMarkdownText(discount["discountTax"]), formatNegativeCurrencyMarkdownText(discountAmount.Add(discountTax)))
				detailIndex++
			}
		}
	}

	appendMarkdownBlock(&lines, "**预览发票票样链接：**")
	if previewLink == "" {
		previewLink = "暂无"
	}
	appendMarkdownBlock(&lines, previewLink)

	if len(entries) > 0 {
		appendMarkdownBlock(&lines, "如确认发票内容无误，请回复：**确认开票**")
	}

	return strings.Join(lines, "\n")
}

func extractPreviewMarkdownInvoiceEntries(payload map[string]any) []map[string]any {
	info, ok := asList(payload["info"])
	if !ok {
		return nil
	}
	out := []map[string]any{}
	for _, item := range info {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func appendMarkdownBlock(lines *[]string, text string) {
	if len(*lines) > 0 {
		*lines = append(*lines, "")
	}
	*lines = append(*lines, text)
}

func appendMarkdownLine(lines *[]string, text string) {
	*lines = append(*lines, text)
}

func appendMarkdownDetailItem(lines *[]string, detailIndex int, itemName string, item map[string]any, taxAmountText, amountWithTaxText string) {
	appendMarkdownLine(lines, fmt.Sprintf("- 明细序号：%d", detailIndex))
	appendMarkdownLine(lines, "- 商品简称+项目名称："+itemName)
	appendMarkdownLine(lines, "- 税收分类编码："+strings.TrimSpace(common.StringValue(item["spbm"])))
	appendMarkdownLine(lines, "- 规格型号："+strings.TrimSpace(common.StringValue(item["size"])))
	appendMarkdownLine(lines, "- 单位："+strings.TrimSpace(common.StringValue(item["unit"])))
	appendMarkdownLine(lines, "- 数量："+strings.TrimSpace(common.StringValue(item["quantity"])))
	appendMarkdownLine(lines, "- 单价（含税）："+formatLineItemUnitPriceWithTax(item))
	appendMarkdownLine(lines, "- 税率："+getTaxPolicyDisplayName(item))
	appendMarkdownLine(lines, "- 税额："+taxAmountText)
	appendMarkdownLine(lines, "- 金额（含税）："+amountWithTaxText)
}

func WriteInvoicePreviewFile(payload map[string]any, outputPath string) (string, error) {
	if err := preloadPreviewProductDetails(payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(outputPath) == "" {
		workspace, err := ResolveWorkspaceDir()
		if err != nil {
			return "", err
		}
		outputPath = filepath.Join(workspace, issueRunsDirName, previewDirName, "invoice-preview-"+shortHash()+".html")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return "", err
	}
	body := buildTemplatePreviewHTML(payload)
	if err := os.WriteFile(outputPath, []byte(body), 0o644); err != nil {
		return "", err
	}
	return outputPath, nil
}

/**
 * preloadPreviewProductDetails 在写入预览文件前完成手工商品编码所需的详情查询。
 *
 * @param payload 已标准化的开票请求。
 * @returns 查询成功或可忽略失败时返回 nil；版本过低时返回错误。
 * @throws skillupdate.OutdatedError 当当前 Skill 版本过低时返回。
 */
func preloadPreviewProductDetails(payload map[string]any) error {
	seen := map[string]bool{}
	for _, invoice := range extractInvoiceEntries(payload) {
		items, _ := asList(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.data"))
		for _, itemAny := range items {
			item, _ := itemAny.(map[string]any)
			code := strings.TrimSpace(common.StringValue(item["spbm"]))
			if code == "" || seen[code] {
				continue
			}
			seen[code] = true
			// 新版推荐已提供预览所需的原商品名和税收分类简称，无需再次查询详情。
			if _, ok := getCachedTaxCodingRecommendation(code); ok {
				continue
			}
			if _, err := getProductClassifyCodeDetailChecked(code); err != nil {
				return err
			}
		}
	}
	return nil
}

func GenerateInvoicePreview(inputPath, outputPath string) (string, error) {
	raw, err := loadJSONInputFlexible(inputPath)
	if err != nil {
		return "", err
	}
	payload := extractPreviewPayload(raw)
	return WriteInvoicePreviewFile(payload, outputPath)
}

func extractPreviewPayload(raw map[string]any) map[string]any {
	if payload, ok := raw["invoice_payload_json"].(map[string]any); ok {
		return payload
	}
	if check, ok := raw["check_result"].(map[string]any); ok {
		if payload, ok := check["invoice_payload_json"].(map[string]any); ok {
			return payload
		}
	}
	return raw
}

func buildPreviewHTML(payload map[string]any) string {
	entries := extractInvoiceEntries(payload)
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>发票预览</title>")
	b.WriteString("<style>body{font-family:Arial,'Microsoft YaHei',sans-serif;margin:32px;color:#222}section{border:1px solid #ddd;padding:20px;margin-bottom:24px}table{border-collapse:collapse;width:100%;margin-top:12px}td,th{border:1px solid #ddd;padding:8px;text-align:left}h1,h2{margin:0 0 16px}</style></head><body><h1>发票预览</h1>")
	for idx, invoice := range entries {
		b.WriteString("<section>")
		if len(entries) > 1 {
			b.WriteString(fmt.Sprintf("<h2>第 %d 张</h2>", idx+1))
		}
		b.WriteString("<p><b>发票类型：</b>" + html.EscapeString(common.StringValue(GetPath(map[string]any{"x": invoice}, "x.basicInfo.invoiceType"))) + "</p>")
		b.WriteString("<p><b>销售方：</b>" + html.EscapeString(common.StringValue(GetPath(map[string]any{"x": invoice}, "x.sellerInfo.name"))) + "</p>")
		b.WriteString("<p><b>购买方：</b>" + html.EscapeString(common.StringValue(GetPath(map[string]any{"x": invoice}, "x.purchaserInfo.name"))) + "</p>")
		b.WriteString("<table><thead><tr><th>项目</th><th>编码</th><th>规格</th><th>单位</th><th>数量</th><th>单价</th><th>金额</th><th>税率</th><th>税额</th></tr></thead><tbody>")
		items, _ := asList(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.data"))
		for _, itemAny := range items {
			item, _ := itemAny.(map[string]any)
			name := common.StringValue(item["projectName"])
			if name == "" {
				name = getProductNameBySPBM(item["spbm"])
			}
			b.WriteString("<tr>")
			for _, value := range []string{
				name,
				common.StringValue(item["spbm"]),
				common.StringValue(item["size"]),
				common.StringValue(item["unit"]),
				common.StringValue(item["quantity"]),
				formatLineItemUnitPriceWithTax(item),
				formatLineItemTotalWithTax(item),
				common.StringValue(item["taxRate"]),
				common.StringValue(item["tax"]),
			} {
				b.WriteString("<td>" + html.EscapeString(value) + "</td>")
			}
			b.WriteString("</tr>")
		}
		b.WriteString("</tbody></table>")
		b.WriteString("<p><b>合计金额：</b>" + html.EscapeString(common.StringValue(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.amount"))) + " <b>合计税额：</b>" + html.EscapeString(common.StringValue(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.tax"))) + " <b>价税合计：</b>" + html.EscapeString(common.StringValue(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.total"))) + "</p>")
		b.WriteString("</section>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

func extractInvoiceEntries(payload map[string]any) []map[string]any {
	info, _ := asList(payload["info"])
	out := []map[string]any{}
	for _, item := range info {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	if len(out) == 0 {
		out = append(out, payload)
	}
	return out
}

func loadProductEntries() []map[string]any {
	if productEntriesCache != nil {
		return productEntriesCache
	}
	productEntriesCache = []map[string]any{}
	return productEntriesCache
}

func loadCommonDictEntries() map[string][]map[string]string {
	if commonDictCache != nil {
		return commonDictCache
	}
	commonDictCache = map[string][]map[string]string{}
	data, err := os.ReadFile(assetPath("common_dict.json"))
	if err != nil {
		return commonDictCache
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return commonDictCache
	}
	for _, item := range raw {
		name := strings.TrimSpace(common.StringValue(item["name"]))
		options, _ := asList(item["data"])
		for _, optionAny := range options {
			option, _ := optionAny.(map[string]any)
			label := strings.TrimSpace(common.StringValue(option["label"]))
			value := strings.TrimSpace(common.StringValue(option["value"]))
			if name != "" && label != "" && value != "" {
				commonDictCache[name] = append(commonDictCache[name], map[string]string{"label": label, "value": value})
			}
		}
	}
	return commonDictCache
}

func normalizeProductSearchText(value any) string {
	re := regexp.MustCompile(`[^0-9a-zA-Z\p{Han}]+`)
	return strings.ToLower(re.ReplaceAllString(strings.TrimSpace(common.StringValue(value)), ""))
}

func splitProductKeywords(value any) []string {
	text := strings.TrimSpace(common.StringValue(value))
	if text == "" {
		return nil
	}
	parts := regexp.MustCompile(`[、,，;\s]+`).Split(text, -1)
	out := []string{}
	for _, part := range parts {
		if keyword := normalizeProductSearchText(part); keyword != "" {
			out = append(out, keyword)
		}
	}
	return uniqueStrings(out)
}

func scoreProductKeywordMatch(query, candidate string) float64 {
	if query == "" || candidate == "" {
		return 0
	}
	if query == candidate {
		return 1
	}
	if strings.Contains(candidate, query) {
		return 0.95 + float64(minInt(len(query), len(candidate)))/float64(maxInt(len(query), len(candidate)))*0.03
	}
	if strings.Contains(query, candidate) {
		return 0.92 + float64(minInt(len(query), len(candidate)))/float64(maxInt(len(query), len(candidate)))*0.03
	}
	return similarityRatio(query, candidate)
}

func similarityRatio(a, b string) float64 {
	if a == "" && b == "" {
		return 1
	}
	matches := 0
	used := make([]bool, len([]rune(b)))
	br := []rune(b)
	for _, ar := range []rune(a) {
		for i, brv := range br {
			if !used[i] && ar == brv {
				used[i] = true
				matches++
				break
			}
		}
	}
	return float64(2*matches) / float64(len([]rune(a))+len([]rune(b)))
}

func uniqueStrings(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func escapeMarkdown(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func buildTemplatePreviewHTML(payload map[string]any) string {
	previewData, _ := json.MarshalIndent(buildPreviewInvoices(payload), "", "  ")
	templatePath := assetPath("invoice_preview.html")
	template, err := os.ReadFile(templatePath)
	if err == nil && strings.Contains(string(template), "__INVOICE_PREVIEW_DATA__") {
		return strings.ReplaceAll(string(template), "__INVOICE_PREVIEW_DATA__", string(previewData))
	}
	return buildPreviewHTML(payload)
}

func buildPreviewInvoices(payload map[string]any) []map[string]any {
	out := []map[string]any{}
	sellerTaxID := strings.TrimSpace(common.StringValue(payload["uscc"]))
	for _, invoice := range extractInvoiceEntries(payload) {
		basic, _ := invoice["basicInfo"].(map[string]any)
		purchaser, _ := invoice["purchaserInfo"].(map[string]any)
		seller, _ := invoice["sellerInfo"].(map[string]any)
		note, _ := invoice["noteInfo"].(map[string]any)
		totals := computePreviewInvoiceTotals(invoice)
		out = append(out, map[string]any{
			"titleType":   strings.TrimSpace(common.StringValue(basic["invoiceType"])),
			"buyerLines":  buildPartyPreviewLines(purchaser, ""),
			"sellerLines": buildPartyPreviewLines(seller, sellerTaxID),
			"items":       buildPreviewLineItems(invoice),
			"summary": map[string]any{
				"totalAmount":       formatCurrencyText(totals["amount"]),
				"totalTax":          formatCurrencyText(totals["tax"]),
				"grandTotalWords":   formatTotalUppercase(totals["total"]),
				"grandTotalFigures": formatCurrencyText(totals["total"]),
			},
			"remarks": strings.Join(buildPreviewRemarkLines(invoice, note), "\n"),
		})
	}
	return out
}

func buildPartyPreviewLines(info map[string]any, taxID string) []string {
	if info == nil {
		info = map[string]any{}
	}
	lines := []string{}
	appendLine := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			lines = append(lines, label+"："+value)
		}
	}
	appendLine("名称", common.StringValue(info["name"]))
	appendLine("统一社会信用代码/纳税人识别号", firstString(taxID, common.StringValue(info["uscc"])))
	appendLine("地址电话", joinNonEmpty(" / ", common.StringValue(info["address"]), common.StringValue(info["phone"])))
	appendLine("开户行及账号", joinNonEmpty(" / ", common.StringValue(info["bank"]), common.StringValue(info["bankAccount"])))
	return lines
}

func buildPreviewLineItems(invoice map[string]any) []map[string]string {
	items, _ := asList(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.data"))
	out := []map[string]string{}
	for idx, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		row := map[string]string{
			"name":      buildPreviewItemName(item, idx+1),
			"spec":      strings.TrimSpace(common.StringValue(item["size"])),
			"unit":      strings.TrimSpace(common.StringValue(item["unit"])),
			"quantity":  strings.TrimSpace(common.StringValue(item["quantity"])),
			"price":     formatLineItemUnitPriceWithTax(item),
			"amount":    formatLineItemTotalWithTax(item),
			"taxRate":   getTaxRateDisplayText(item),
			"taxAmount": formatCurrencyText(item["tax"]),
		}
		out = append(out, row)
		if discount, ok := item["discountInfo"].(map[string]any); ok && HasMeaningfulValue(discount["discountAmount"]) {
			discountTax, _ := ToDecimal(discount["discountTax"])
			discountAmount, _ := ToDecimal(discount["discountAmount"])
			out = append(out, map[string]string{
				"name":       "【折扣】" + buildPreviewItemName(item, idx+1),
				"spec":       row["spec"],
				"unit":       row["unit"],
				"quantity":   row["quantity"],
				"price":      row["price"],
				"amount":     formatCurrencyText(discountAmount.Neg()),
				"taxRate":    row["taxRate"],
				"taxAmount":  formatCurrencyText(discountTax.Neg()),
				"isDiscount": "true",
			})
		}
	}
	return out
}

func buildPreviewItemName(item map[string]any, index int) string {
	productName := strings.TrimSpace(common.StringValue(item["projectName"]))
	if productName == "" {
		productName = getProductNameBySPBM(item["spbm"])
	}
	if productName == "" {
		productName = strings.TrimSpace(common.StringValue(item["spbm"]))
	}
	if productName == "" {
		productName = fmt.Sprintf("明细%d", index)
	}
	category := getProductCategoryBySPBM(item["spbm"])
	if category != "" {
		return "*" + category + "*" + productName
	}
	return productName
}

func buildMarkdownPreviewItemName(item map[string]any, index int) string {
	return escapeMarkdownText(buildPreviewItemName(item, index))
}

func buildDiscountMarkdownPreviewItemName(item map[string]any, index int) string {
	return "【折扣】" + buildMarkdownPreviewItemName(item, index)
}

func escapeMarkdownText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "*", "\\*")
}

func computePreviewInvoiceTotals(invoice map[string]any) map[string]decimal.Decimal {
	items, _ := asList(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.data"))
	amountTotal := decimal.Zero
	taxTotal := decimal.Zero
	total := decimal.Zero
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		amount, _ := ToDecimal(item["amount"])
		tax, _ := ToDecimal(item["tax"])
		discountAmount := decimal.Zero
		discountTax := decimal.Zero
		if discount, ok := item["discountInfo"].(map[string]any); ok {
			discountAmount, _ = ToDecimal(discount["discountAmount"])
			discountTax, _ = ToDecimal(discount["discountTax"])
		}
		amountTotal = amountTotal.Add(amount).Sub(discountAmount)
		taxTotal = taxTotal.Add(tax).Sub(discountTax)
		total = total.Add(amount).Add(tax).Sub(discountAmount).Sub(discountTax)
	}
	return map[string]decimal.Decimal{"amount": quantizeCurrency(amountTotal), "tax": quantizeCurrency(taxTotal), "total": quantizeCurrency(total)}
}

func buildPreviewRemarkLines(invoice map[string]any, note map[string]any) []string {
	lines := []string{}
	items, _ := asList(GetPath(map[string]any{"x": invoice}, "x.invoiceDetail.data"))
	if note != nil {
		if value := strings.TrimSpace(common.StringValue(note["note"])); value != "" {
			lines = append(lines, value)
		}
		people := []string{}
		if value := strings.TrimSpace(common.StringValue(note["payee"])); value != "" {
			people = append(people, "收款人："+value)
		}
		if value := strings.TrimSpace(common.StringValue(note["reviewer"])); value != "" {
			people = append(people, "复核人："+value)
		}
		if len(people) > 0 {
			lines = append(lines, strings.Join(people, "  "))
		}
	}
	payments, _ := asList(invoice["paymentInfo"])
	for _, paymentAny := range payments {
		payment, _ := paymentAny.(map[string]any)
		parts := []string{}
		if value := getPaymentChannelDisplayName(payment["payChannelCode"]); value != "" {
			parts = append(parts, "支付渠道："+value)
		}
		if value := strings.TrimSpace(common.StringValue(payment["transactionOrderNo"])); value != "" {
			parts = append(parts, "交易单号："+value)
		}
		if len(parts) > 0 {
			lines = append(lines, strings.Join(parts, "  "))
		}
	}
	coalLabels := []string{}
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item == nil || !isCoalGoodsItem(item) {
			continue
		}
		label := normalizeCoalTypeLabel(getCommonDictLabel("煤炭种类", item["mtzlDm"]))
		if label != "" && !containsString(coalLabels, label) {
			coalLabels = append(coalLabels, label)
		}
	}
	if len(coalLabels) > 0 {
		lines = append(lines, "煤炭种类："+strings.Join(coalLabels, "、"))
	}
	return lines
}

func formatCurrencyText(value any) string {
	d, ok := ToDecimal(value)
	if !ok {
		return "0.00"
	}
	return quantizeCurrency(d).StringFixed(2)
}

func formatCurrencyMarkdownText(value any) string {
	d, ok := ToDecimal(value)
	if !ok {
		return "0.00"
	}
	return formatDecimalWithThousands(quantizeCurrency(d))
}

func formatUnitPriceMarkdownText(value any) string {
	d, ok := ToDecimal(value)
	if !ok {
		return ""
	}
	return formatDecimalWithThousandsVariable(quantizeUnitPrice(d))
}

func formatNegativeCurrencyMarkdownText(value any) string {
	d, ok := ToDecimal(value)
	if !ok {
		return "0.00"
	}
	d = quantizeCurrency(d)
	if d.IsZero() {
		return "0.00"
	}
	return formatCurrencyMarkdownText(d.Abs().Neg())
}

func formatLineItemTotalWithTax(item map[string]any) string {
	for _, field := range []string{"taxInclusiveAmount", "amountWithTax", "totalWithTax"} {
		if HasMeaningfulValue(item[field]) {
			return formatCurrencyMarkdownText(item[field])
		}
	}
	amount, _ := ToDecimal(item["amount"])
	tax, _ := ToDecimal(item["tax"])
	return formatCurrencyMarkdownText(amount.Add(tax))
}

func formatLineItemUnitPriceWithTax(item map[string]any) string {
	for _, field := range []string{"taxInclusivePrice", "priceWithTax", "unitPriceWithTax"} {
		if HasMeaningfulValue(item[field]) {
			return formatUnitPriceMarkdownText(item[field])
		}
	}
	quantity, qOK := ToDecimal(item["quantity"])
	if qOK && !quantity.IsZero() {
		amount, _ := ToDecimal(item["amount"])
		tax, _ := ToDecimal(item["tax"])
		return formatUnitPriceMarkdownText(amount.Add(tax).Div(quantity))
	}
	if HasMeaningfulValue(item["price"]) {
		price, priceOK := ToDecimal(item["price"])
		taxRateText := strings.TrimSpace(common.StringValue(item["taxRate"]))
		taxRate, taxRateOK := ToDecimal(strings.TrimSuffix(taxRateText, "%"))
		if priceOK && taxRateOK && strings.HasSuffix(taxRateText, "%") {
			return formatUnitPriceMarkdownText(price.Mul(decimal.NewFromInt(1).Add(taxRate.Div(decimal.NewFromInt(100)))))
		}
		return formatUnitPriceMarkdownText(item["price"])
	}
	return ""
}

func formatPreviewTaxAmount(item map[string]any) string {
	policyFlag := strings.ToUpper(strings.TrimSpace(common.StringValue(item["xsyhzcbz"])))
	specialPolicy := strings.TrimSpace(common.StringValue(item["zzstsgl"]))
	if policyFlag == "Y" && specialPolicy == "03" {
		return "***"
	}
	if policyFlag == "Y" && specialPolicy == "04" {
		return ""
	}
	return formatCurrencyMarkdownText(item["tax"])
}

func getTaxPolicyDisplayName(item map[string]any) string {
	policyFlag := strings.ToUpper(strings.TrimSpace(common.StringValue(item["xsyhzcbz"])))
	specialPolicy := strings.TrimSpace(common.StringValue(item["zzstsgl"]))
	if policyFlag == "Y" && specialPolicy == "03" {
		return "免税"
	}
	if policyFlag == "Y" && specialPolicy == "04" {
		return "不征税"
	}
	return strings.TrimSpace(common.StringValue(item["taxRate"]))
}

func formatDecimalWithThousands(value decimal.Decimal) string {
	text := value.StringFixed(2)
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign = "-"
		text = strings.TrimPrefix(text, "-")
	}
	parts := strings.SplitN(text, ".", 2)
	integer := parts[0]
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1]
	}
	chunks := []string{}
	for len(integer) > 3 {
		chunks = append([]string{integer[len(integer)-3:]}, chunks...)
		integer = integer[:len(integer)-3]
	}
	chunks = append([]string{integer}, chunks...)
	return sign + strings.Join(chunks, ",") + "." + fraction
}

func formatDecimalWithThousandsVariable(value decimal.Decimal) string {
	text := value.String()
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign = "-"
		text = strings.TrimPrefix(text, "-")
	}
	parts := strings.SplitN(text, ".", 2)
	integer := parts[0]
	chunks := []string{}
	for len(integer) > 3 {
		chunks = append([]string{integer[len(integer)-3:]}, chunks...)
		integer = integer[:len(integer)-3]
	}
	chunks = append([]string{integer}, chunks...)
	if len(parts) == 1 {
		return sign + strings.Join(chunks, ",")
	}
	return sign + strings.Join(chunks, ",") + "." + parts[1]
}

func formatLineItemAmountWithoutTax(item map[string]any) string {
	if HasMeaningfulValue(item["amount"]) {
		return formatCurrencyText(item["amount"])
	}
	quantity, qOK := ToDecimal(item["quantity"])
	price, pOK := ToDecimal(item["price"])
	if qOK && pOK && !quantity.IsZero() {
		return formatCurrencyText(quantity.Mul(price))
	}
	return "0.00"
}

func formatLineItemUnitPriceWithoutTax(item map[string]any) string {
	if HasMeaningfulValue(item["price"]) {
		return formatUnitPriceMarkdownText(item["price"])
	}
	quantity, qOK := ToDecimal(item["quantity"])
	amount, aOK := ToDecimal(item["amount"])
	if qOK && aOK && !quantity.IsZero() {
		return formatUnitPriceMarkdownText(amount.Div(quantity))
	}
	return ""
}

func getPaymentChannelDisplayName(value any) string {
	text := strings.TrimSpace(common.StringValue(value))
	if text == "" {
		return ""
	}
	candidates := []string{text}
	if isDigits(text) {
		candidates = append(candidates, leftPad(text, 3))
		if len(text) == 5 && strings.HasPrefix(text, "10") {
			candidates = append(candidates, text[len(text)-3:])
		}
	}
	for _, candidate := range candidates {
		for _, option := range loadCommonDictEntries()["支付渠道"] {
			if candidate == option["value"] || normalizeProductSearchText(candidate) == normalizeProductSearchText(option["label"]) {
				return option["label"]
			}
		}
	}
	if label := getCommonDictLabel("支付渠道", text); label != "" {
		return label
	}
	return text
}

func getCommonDictLabel(dictionaryName string, value any) string {
	text := strings.TrimSpace(common.StringValue(value))
	if text == "" {
		return ""
	}
	options := loadCommonDictEntries()[dictionaryName]
	for _, option := range options {
		if text == option["value"] {
			return option["label"]
		}
	}
	query := normalizeProductSearchText(text)
	for _, option := range options {
		if query == normalizeProductSearchText(option["value"]) || query == normalizeProductSearchText(option["label"]) {
			return option["label"]
		}
	}
	return text
}

func normalizeCoalTypeLabel(value string) string {
	label := strings.TrimSpace(value)
	if label == "" {
		return ""
	}
	if strings.HasPrefix(label, "长协煤") {
		return "长协煤"
	}
	if label == "政府保供煤" || label == "市场煤" {
		return label
	}
	return ""
}

func isCoalGoodsItem(item map[string]any) bool {
	candidates := []string{
		strings.TrimSpace(common.StringValue(item["projectName"])),
		getProductNameBySPBM(item["spbm"]),
		getProductCategoryBySPBM(item["spbm"]),
	}
	for _, candidate := range candidates {
		normalized := normalizeProductSearchText(candidate)
		if strings.Contains(normalized, "煤") || strings.Contains(normalized, "煤炭") {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func formatTotalUppercase(value any) string {
	d, ok := ToDecimal(value)
	if !ok {
		return "零元整"
	}
	d = quantizeCurrency(d.Abs())
	integer := int(d.IntPart())
	fenTotal := int(d.Mul(decimal.NewFromInt(100)).IntPart()) % 100
	jiao := fenTotal / 10
	fen := fenTotal % 10
	result := convertIntegerToChineseUpper(integer) + "元"
	digits := []string{"零", "壹", "贰", "叁", "肆", "伍", "陆", "柒", "捌", "玖"}
	if jiao == 0 && fen == 0 {
		return result + "整"
	}
	if jiao > 0 {
		result += digits[jiao] + "角"
	} else if fen > 0 {
		result += "零"
	}
	if fen > 0 {
		result += digits[fen] + "分"
	}
	return result
}

func convertIntegerToChineseUpper(value int) string {
	if value == 0 {
		return "零"
	}
	digits := []string{"零", "壹", "贰", "叁", "肆", "伍", "陆", "柒", "捌", "玖"}
	units := []string{"", "拾", "佰", "仟"}
	bigUnits := []string{"", "万", "亿", "兆"}
	sections := []string{}
	for value > 0 {
		section := value % 10000
		text := ""
		zero := false
		for i := 0; i < 4; i++ {
			digit := section % 10
			if digit == 0 {
				if text != "" {
					zero = true
				}
			} else {
				if zero {
					text = digits[0] + text
					zero = false
				}
				text = digits[digit] + units[i] + text
			}
			section /= 10
		}
		sections = append(sections, text)
		value /= 10000
	}
	out := ""
	for i := len(sections) - 1; i >= 0; i-- {
		if sections[i] != "" {
			out += sections[i] + bigUnits[i]
		}
	}
	return strings.TrimRight(out, "零")
}

/**
 * getProductCategoryBySPBM 获取商品编码对应的税收分类简称。
 *
 * @param spbm 税收分类编码。
 * @returns 新版智能赋码的 shortName；没有推荐缓存时返回商品详情中的 Spfwjc。
 */
func getProductCategoryBySPBM(spbm any) string {
	code := strings.TrimSpace(common.StringValue(spbm))
	if recommendation, ok := getCachedTaxCodingRecommendation(code); ok && recommendation.ShortName != "" {
		return recommendation.ShortName
	}
	return strings.TrimSpace(common.StringValue(getProductClassifyCodeDetail(code)["Spfwjc"]))
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func joinNonEmpty(sep string, values ...string) string {
	out := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return strings.Join(out, sep)
}

func leftPad(value string, length int) string {
	for len(value) < length {
		value = "0" + value
	}
	return value
}
