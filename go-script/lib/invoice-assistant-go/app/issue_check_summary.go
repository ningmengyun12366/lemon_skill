package app

import (
	"os"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func appendIssueCheckSummary(taskPath string) {
	common.AppendMessageLog("任务状态已保存到 ", taskPath, "\n")

	data, err := common.ReadJSONFile(taskPath)
	if err != nil {
		common.AppendMessageLog("无法读取任务状态 JSON；请先打开上述任务文件确认 check_result，不要直接开票或手动拼预览。\n")
		return
	}
	for _, message := range issueCheckSummaryMessages(data) {
		common.AppendMessageLog(message, "\n")
	}
}

func issueCheckSummaryMessages(task map[string]any) []string {
	check, _ := task["check_result"].(map[string]any)
	if check == nil {
		return []string{"任务状态缺少 check_result；请重新执行开票信息检查命令。"}
	}

	if taskFailed(task, check) {
		return []string{"发票参数校验未通过，不会生成预览 Markdown；请先根据 check_result.missing_fields_by_module、validation_errors 或 tax_rate_validation 补齐/确认信息后重新执行 check，勿改用 issue-invoice-preview-generator 试错。"}
	}

	previewMarkdownPath := strings.TrimSpace(common.StringValue(check["invoice_preview_markdown_path"]))
	if truthyBool(check["invoice_preview_markdown_generated"]) {
		if previewMarkdown := previewMarkdownFromCheck(check, previewMarkdownPath); previewMarkdown != "" {
			return appendPreviewSampleLinkMessage([]string{previewMarkdown}, check)
		}
		return appendPreviewSampleLinkMessage([]string{"发票预览已生成，但未能读取预览 Markdown 内容；请重新执行开票信息检查命令。"}, check)
	}

	previewError := strings.TrimSpace(common.StringValue(check["invoice_preview_error"]))
	if previewError != "" {
		return []string{"发票参数校验已通过，但预览生成失败：" + previewError}
	}
	return []string{"发票参数校验已通过，但未生成预览 Markdown；请检查 task JSON 中的 check_result，勿手动拼预览或直接开票。"}
}

func previewMarkdownFromCheck(check map[string]any, previewMarkdownPath string) string {
	if previewMarkdownPath != "" {
		if data, err := os.ReadFile(previewMarkdownPath); err == nil {
			if previewMarkdown := strings.TrimSpace(string(data)); previewMarkdown != "" {
				return previewMarkdown
			}
		}
	}
	return strings.TrimSpace(common.StringValue(check["invoice_preview_markdown"]))
}

func appendPreviewSampleLinkMessage(messages []string, check map[string]any) []string {
	if message := previewSampleLinkMessage(check); message != "" {
		return append(messages, message)
	}
	return messages
}

func previewSampleLinkMessage(check map[string]any) string {
	previewPath := strings.TrimSpace(common.StringValue(check["invoice_preview_path"]))
	if previewPath == "" {
		return ""
	}
	previewLink := common.FormatLocalPathText(previewPath)
	if previewLink == "" {
		return ""
	}
	return "预览发票票样链接（请原样展示）：\n" + previewLink
}

func taskFailed(task, check map[string]any) bool {
	status := strings.TrimSpace(common.StringValue(task["task_status"]))
	return status == "check_failed" || !truthyBool(check["validation_passed"])
}

func truthyBool(value any) bool {
	got, ok := value.(bool)
	return ok && got
}
