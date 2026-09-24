package issue

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func BuildIssueResultMarkdown(resp map[string]any) string {
	success, partial := summarizeFinalIssueOutcome(resp)
	lines := []string{"# 发票开具结果", ""}
	if success {
		lines = append(lines, "开票成功。", "")
	} else if partial {
		lines = append(lines, "部分发票开具成功，请检查失败明细。", "")
	} else {
		lines = append(lines, "开票未成功："+extractIssueFailureMessage(resp), "")
	}
	items := extractStatusItems(resp)
	if len(items) == 0 {
		if data, ok := resp["data"].(map[string]any); ok && len(data) > 0 {
			items = append(items, data)
		}
	}
	if len(items) > 0 {
		lines = append(lines, "| 序号 | 发票号码 | 状态 | 金额 | 税额 | 失败原因 |", "| --- | --- | --- | --- | --- | --- |")
		for idx, item := range items {
			lines = append(lines, fmt.Sprintf("| %d | %s | %s | %s | %s | %s |",
				idx+1,
				common.StringValue(firstNonNil(item["invoiceNo"], item["fphm"], item["invoiceNumber"])),
				common.StringValue(firstNonNil(item["status"], item["issueStatus"], item["kpzt"])),
				common.StringValue(extractItemIssueTotal(item)),
				common.StringValue(extractItemTaxAmount(item)),
				common.StringValue(firstNonNil(item["message"], item["failReason"], item["errorMessage"])),
			))
		}
	}
	return strings.Join(lines, "\n")
}

func BuildIssueResultMarkdownEquivalent(resp map[string]any) string {
	items := extractStatusItems(resp)
	if len(items) > 1 {
		return buildBatchIssueResultMarkdownEquivalent(resp, items)
	}
	item := extractSingleStatusItem(resp)
	if item == nil || isIssueStatusTerminalFailure(resp) {
		return "**开具失败：**\n\n1. 开票结果：发票开具失败，失败原因：" + extractIssueFailureMessage(resp)
	}
	nested, _ := item["data"].(map[string]any)
	fileInfo, _ := item["file"].(map[string]any)
	localFile, _ := item["local_file"].(map[string]any)
	invoiceNumber := strings.TrimSpace(common.StringValue(firstNonNil(nested["sdphm"], item["sdphm"])))
	purchaserName := strings.TrimSpace(common.StringValue(nested["gfmc"]))
	totalAmount := formatIssueCurrencyDisplay(firstNonNil(nested["jshj"], nested["je"], item["hjje"]))
	issueTime := strings.TrimSpace(common.StringValue(firstNonNil(nested["kprq"], item["kprq"])))
	if len(issueTime) >= 10 {
		issueTime = issueTime[:10]
	}
	lines := []string{
		"**开具成功：**",
		"",
		"1. 开票结果：发票开具成功",
		"2. 发票信息：",
		"   - 发票号码：" + invoiceNumber,
		"   - 购买方：" + purchaserName,
		"   - 开票金额：" + totalAmount + "（含税）",
		"   - 开票时间：" + issueTime,
		"3. 发票原件路径：",
	}
	lines = append(lines, buildOriginalFileLines(fileInfo, localFile)...)
	pdfURL := issuePDFDownloadLink(fileInfo)
	if pdfURL != "" {
		lines = append(lines, "4. 发票PDF下载链接："+pdfURL)
	} else {
		lines = append(lines, "4. 发票PDF下载链接：暂无")
	}
	return strings.Join(lines, "\n")
}

func BuildIssueProcessingMarkdown(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "开票状态仍在处理中，尚未得到明确成功或失败结果"
	}
	return strings.Join([]string{
		"**开具处理中：**",
		"",
		"1. 开票结果：仍在处理中",
		"2. 说明：" + message,
	}, "\n")
}

func buildBatchIssueResultMarkdownEquivalent(resp map[string]any, items []map[string]any) string {
	successful := []map[string]any{}
	failed := []map[string]any{}
	for _, item := range items {
		if isLegacyBatchIssueItemSuccessful(item) {
			successful = append(successful, item)
		} else {
			failed = append(failed, item)
		}
	}
	if len(successful) == 0 {
		reasons := []string{}
		for _, item := range failed {
			if reason := strings.TrimSpace(common.StringValue(item["reason"])); reason != "" {
				reasons = append(reasons, reason)
			}
		}
		msg := extractIssueFailureMessage(resp)
		if len(reasons) > 0 {
			msg = strings.Join(reasons, "；")
		}
		return "**全部开具失败：**\n\n1. 开具结果：发票开具失败，失败原因：" + msg
	}
	title := "**全部开具成功：**"
	summary := "1. 开票结果：批量开票任务全部开具成功"
	if len(failed) > 0 {
		title = "**部分开具成功：**"
		summary = "1. 开票结果：批量开票任务部分开具成功"
	}
	amountTotal := decimalZero()
	taxTotal := decimalZero()
	for _, item := range successful {
		amount, _ := ToDecimal(extractItemIssueTotal(item))
		tax, _ := ToDecimal(extractItemTaxAmount(item))
		amountTotal = amountTotal.Add(amount)
		taxTotal = taxTotal.Add(tax)
	}
	lines := []string{
		title,
		"",
		summary,
		"2. 关键信息：",
		fmt.Sprintf("   - 处理发票开具总数：%d张", len(items)),
		fmt.Sprintf("   - 开具成功数量：%d张", len(successful)),
		fmt.Sprintf("   - 开具失败数量：%d张", len(failed)),
		fmt.Sprintf("   - 合计开票金额：%s（含税），合计税额：%s", formatIssueCurrencyDisplay(amountTotal), formatIssueCurrencyDisplay(taxTotal)),
		"3. 发票原件路径：",
	}
	lines = append(lines, buildBatchOriginalPathLines(successful)...)
	lines = append(lines, "4. 发票PDF下载链接：")
	lines = append(lines, buildBatchPDFDownloadLines(successful)...)
	if len(failed) > 0 {
		lines = append(lines, "5. 异常说明：")
		for idx, item := range failed {
			identifier := strings.TrimSpace(common.StringValue(firstNonNil(item["fplsh"], item["id"], item["sdphm"], fmt.Sprintf("第%d条", idx+1))))
			reason := strings.TrimSpace(common.StringValue(item["reason"]))
			if reason == "" {
				reason = "未知失败原因"
			}
			lines = append(lines, fmt.Sprintf("   - 失败发票%d：任务号 %s，失败原因：%s", idx+1, identifier, reason))
		}
	}
	return strings.Join(lines, "\n")
}

func buildOriginalFileLines(fileInfo, localFile map[string]any) []string {
	lines := []string{}
	for _, kind := range []string{"pdf", "xml", "ofd"} {
		localPath := strings.TrimSpace(common.StringValue(localFile[kind]))
		remoteURL := issueFileURL(fileInfo, kind)
		if localPath != "" || remoteURL != "" {
			label := strings.ToUpper(kind) + "原件"
			value := remoteURL
			if localPath != "" {
				value = common.FormatLocalPathText(localPath)
			}
			lines = append(lines, "   - "+label+"："+value)
		}
	}
	if len(lines) == 0 {
		return []string{"   - 暂无原件路径"}
	}
	return lines
}

func buildBatchOriginalPathLines(items []map[string]any) []string {
	lines := []string{}
	for idx, item := range items {
		identifier := strings.TrimSpace(common.StringValue(firstNonNil(item["fplsh"], item["id"], fmt.Sprintf("第%d张", idx+1))))
		lines = append(lines, fmt.Sprintf("   - 发票%d（%s）", idx+1, identifier))
		fileInfo, _ := item["file"].(map[string]any)
		localFile, _ := item["local_file"].(map[string]any)
		lines = append(lines, buildBatchOriginalFileDetailLines(fileInfo, localFile)...)
	}
	if len(lines) == 0 {
		return []string{"   - 暂无原件路径"}
	}
	return lines
}

func buildBatchOriginalFileDetailLines(fileInfo, localFile map[string]any) []string {
	lines := []string{}
	for _, kind := range []string{"pdf", "xml", "ofd"} {
		localPath := strings.TrimSpace(common.StringValue(localFile[kind]))
		remoteURL := issueFileURL(fileInfo, kind)
		if localPath != "" || remoteURL != "" {
			label := strings.ToUpper(kind) + "原件"
			value := remoteURL
			if localPath != "" {
				value = common.FormatLocalPathText(localPath)
			}
			lines = append(lines, "     - "+label+"："+value)
		}
	}
	if len(lines) == 0 {
		return []string{"     - 暂无原件路径"}
	}
	return lines
}

func buildBatchPDFDownloadLines(items []map[string]any) []string {
	lines := []string{}
	for idx, item := range items {
		identifier := strings.TrimSpace(common.StringValue(firstNonNil(item["fplsh"], item["id"], fmt.Sprintf("第%d张", idx+1))))
		fileInfo, _ := item["file"].(map[string]any)
		pdfURL := issuePDFDownloadLink(fileInfo)
		if pdfURL != "" {
			lines = append(lines, fmt.Sprintf("   - 发票%d（%s）：%s", idx+1, identifier, pdfURL))
		}
	}
	if len(lines) == 0 {
		return []string{"   - 暂无 PDF 下载链接"}
	}
	return lines
}

func issueFileURL(fileInfo map[string]any, kind string) string {
	return strings.TrimSpace(common.StringValue(firstNonNil(fileInfo[kind], fileInfo[kind+"Url"], fileInfo[kind+"_url"])))
}

func formatIssueCurrencyDisplay(value any) string {
	d, ok := ToDecimal(value)
	if !ok {
		return "￥0.00"
	}
	return "￥" + quantizeCurrency(d).StringFixed(2)
}

func decimalZero() decimal.Decimal {
	return decimal.Zero
}
