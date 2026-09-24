package fetchrisk

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	createAttachmentTaskURL = "/attachment-download/create-task"
	attachmentTaskStatusURL = "/attachment-download/task-status"
)

func FetchAttachments(taskFile string) error {
	return FetchAttachmentsWithOptions(taskFile, FetchCommandOptions{})
}

/**
 * FetchAttachmentsWithOptions 使用任务企业快照创建、轮询并下载附件任务。
 *
 * @param taskFile 包含 enterprise_context 的取票任务文件。
 * @param options 可注入的请求客户端、轮询时间和锁参数。
 * @returns 全流程成功时返回 nil，否则返回本地校验或远端业务错误。
 */
func FetchAttachmentsWithOptions(taskFile string, options FetchCommandOptions) (retErr error) {
	common.AppendMessageLog("开始执行附件下载任务")
	executionOptions := resolveFetchRuntime(options)
	lock, taskPath, err := acquireFetchTaskCommandLock(taskFile, executionOptions)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, lock.Release()) }()
	state, err := LoadTaskState(taskPath)
	if err != nil {
		return err
	}
	if _, err := bindFetchTaskEnterprise(state, &executionOptions); err != nil {
		return err
	}
	params, err := IntentParams(state.Data)
	if err != nil {
		return err
	}
	uscc, err := RequiredString(params, "uscc")
	if err != nil {
		return err
	}
	start, err := RequiredString(params, "invoiceStartTime")
	if err != nil {
		return err
	}
	end, err := RequiredString(params, "invoiceEndTime")
	if err != nil {
		return err
	}

	execution := common.ObjectValue(state.Data, "execution")
	fetchInvoices, _ := execution["fetch_invoices"].(map[string]any)
	syncTaskID := intFrom(fetchInvoices["sync_task_id"])
	if syncTaskID == 0 {
		return fmt.Errorf("任务文件缺少 execution.fetch_invoices.sync_task_id，无法查询附件下载链接")
	}
	outputs := common.ObjectValue(state.Data, "outputs")
	artifacts := common.ObjectValue(state.Data, "artifacts")
	storage, _ := state.Data["storage"].(map[string]any)
	attachmentDir := strings.TrimSpace(common.StringValue(firstNonEmpty(storage["attachments_dir"], artifacts["attachments_dir"])))
	if attachmentDir == "" {
		attachmentDir = filepath.Join(common.StringValue(state.Data["workspace_dir"]), RunsDirName, "发票附件")
	}

	spec := attachmentSubmissionSpec()
	payload, err := spec.BuildPayload(params, executionOptions.now())
	if err != nil {
		return err
	}
	if err := executionOptions.client.ValidateJSONRequest(payload); err != nil {
		return err
	}
	submissionLock, fingerprint, err := acquireFetchSubmissionCommandLock(spec, payload, executionOptions)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, submissionLock.Release()) }()
	taskID, err := ensureFetchRemoteTask(state, spec, payload, fingerprint, executionOptions)
	if err != nil {
		return err
	}
	fetchAttachments := common.ObjectValue(execution, fetchAttachmentsStep)

	common.AppendMessageLog(fmt.Sprintf("使用附件下载任务ID: %d；开始查询原任务状态", taskID))
	_, pollErr := common.PollTaskStatus(executionOptions.client, attachmentTaskStatusURL, taskID, uscc, "附件下载任务失败", "附件下载任务", executionOptions.pollInterval, executionOptions.pollTimeout)
	if err := markFetchPollingResult(state, fetchAttachmentsStep, pollErr, executionOptions.now()); err != nil {
		return err
	}
	summariesPayload := map[string]any{"taskId": syncTaskID, "uscc": uscc, "startDate": start, "endDate": end, "packageAttachments": true}
	resp, err := executionOptions.client.PostSuccessJSON(getSummariesURL, summariesPayload, "查询附件下载地址", 120*time.Second)
	if err != nil {
		return err
	}
	attachmentURLs := ExtractAttachmentURLs(resp)
	volumeNotice := ""
	if len(attachmentURLs) > 500 {
		volumeNotice = "；当前下载数量较多，请耐心等待"
	}
	common.AppendMessageLog(fmt.Sprintf("查询到 %d 个附件下载链接，开始下载附件%s", len(attachmentURLs), volumeNotice))
	downloadResult := DownloadAttachments(attachmentURLs, attachmentDir)
	if downloadResult["status"] == "failed" {
		err := fmt.Errorf("附件下载失败: %v", downloadResult)
		fetchAttachments["status"] = "failed"
		fetchAttachments["last_error"] = err.Error()
		markPlannedStepStatus(state.Data, fetchAttachmentsStep, "failed", err.Error())
		return errors.Join(err, state.Save())
	}
	status := "success"
	if downloadResult["status"] == "partial_success" {
		status = "partial_success"
	}
	fetchAttachments["attachment_task_id"] = taskID
	fetchAttachments["attachment_count"] = len(attachmentURLs)
	fetchAttachments["downloaded_attachment_count"] = downloadResult["success_count"]
	fetchAttachments["failed_attachment_count"] = downloadResult["failed_count"]
	fetchAttachments["status"] = status
	delete(fetchAttachments, "last_error")
	outputs["invoice_attachments"] = attachmentDir
	markPlannedStepStatus(state.Data, fetchAttachmentsStep, status, "")
	if err := state.Save(); err != nil {
		return err
	}
	common.AppendMessageLog("附件下载完成")
	return nil
}

func ExtractAttachmentURLs(payload map[string]any) []string {
	data, _ := payload["data"].(map[string]any)
	items, _ := data["attachments"].([]any)
	urls := []string{}
	for _, item := range items {
		if raw, ok := item.(string); ok {
			text := strings.TrimSpace(raw)
			if text == "" {
				continue
			}
			urls = append(urls, text)
			continue
		}
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"url", "fileUrl", "attachmentUrl", "downloadUrl"} {
			if value := strings.TrimSpace(common.StringValue(obj[key])); value != "" {
				urls = append(urls, value)
				break
			}
		}
	}
	return urls
}

func DownloadAttachments(urls []string, destinationDir string) map[string]any {
	jobs := make([]common.DownloadJob, 0, len(urls))
	for idx, rawURL := range urls {
		fallback := fmt.Sprintf("attachment_%d", idx+1)
		filename := common.FilenameFromURL(rawURL, fallback, func(name string) string {
			return common.SanitizeFilename(name, fallback)
		})
		jobs = append(jobs, common.DownloadJob{URL: rawURL, FallbackName: fallback, TargetFilename: filename})
	}
	return common.DownloadMany(jobs, destinationDir, "attachment", func(name string) string {
		return common.SanitizeFilename(name, "attachment")
	}, 120*time.Second, "url")
}
