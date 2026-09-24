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
	createInvoiceTaskURL = "/invoice-sync/create-task"
	invoiceTaskStatusURL = "/invoice-sync/task-status"
	getSummariesURL      = "/invoice-sync/get-summaries"
)

func FetchInvoices(taskFile string) error {
	return FetchInvoicesWithOptions(taskFile, FetchCommandOptions{})
}

/**
 * FetchInvoicesWithOptions 使用任务企业快照创建、轮询并读取发票同步任务。
 *
 * @param taskFile 包含 enterprise_context 的取票任务文件。
 * @param options 可注入的请求客户端、轮询时间和锁参数。
 * @returns 全流程成功时返回 nil，否则返回本地校验或远端业务错误。
 */
func FetchInvoicesWithOptions(taskFile string, options FetchCommandOptions) (retErr error) {
	common.AppendMessageLog("开始执行取票任务")
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
	artifacts := common.ObjectValue(state.Data, "artifacts")
	outputs := common.ObjectValue(state.Data, "outputs")
	storage, _ := state.Data["storage"].(map[string]any)
	spec := invoiceSubmissionSpec()
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
	fetchExecution := common.ObjectValue(execution, fetchInvoicesStep)

	common.AppendMessageLog(fmt.Sprintf("使用取票任务ID:%d；开始查询原任务执行结果", taskID))
	_, pollErr := common.PollTaskStatus(executionOptions.client, invoiceTaskStatusURL, taskID, uscc, "发票同步任务失败", "发票同步任务", executionOptions.pollInterval, executionOptions.pollTimeout)
	if err := markFetchPollingResult(state, fetchInvoicesStep, pollErr, executionOptions.now()); err != nil {
		return err
	}
	summariesPayload := map[string]any{"taskId": taskID, "uscc": uscc, "startDate": start, "endDate": end}
	resp, err := executionOptions.client.PostSuccessJSON(getSummariesURL, summariesPayload, "查询发票汇总数据", 30*time.Second)
	if err != nil {
		return err
	}
	data, _ := resp["data"].(map[string]any)
	dataList, _ := data["list"].([]any)
	rawDir := common.StringValue(firstNonEmpty(storage["raw_invoice_dir"], artifacts["raw_invoice_dir"]))
	if strings.TrimSpace(rawDir) == "" {
		rawDir = filepath.Join(common.StringValue(state.Data["workspace_dir"]), RunsDirName, "税局原始发票数据")
	}
	downloadResult := DownloadExcelFiles(dataList, rawDir, params["rawInvoiceRenameMap"])
	if downloadResult["status"] == "failed" {
		err := fmt.Errorf("原始发票Excel下载失败: %v", downloadResult)
		fetchExecution["status"] = "failed"
		fetchExecution["last_error"] = err.Error()
		markPlannedStepStatus(state.Data, fetchInvoicesStep, "failed", err.Error())
		return errors.Join(err, state.Save())
	}
	localFiles := []any{}
	for _, item := range anySlice(downloadResult["successes"]) {
		if obj, ok := item.(map[string]any); ok {
			if saved := strings.TrimSpace(common.StringValue(obj["saved_path"])); saved != "" {
				localFiles = append(localFiles, saved)
			}
		}
	}
	artifacts["summaries_data_list"] = dataList
	artifacts["raw_invoice_local_files"] = localFiles
	artifacts["raw_invoice_dir"] = rawDir
	outputs["tax_invoice_excel_files"] = localFiles
	outputs["tax_invoice_excel_dir"] = rawDir
	status := "success"
	if downloadResult["status"] == "partial_success" {
		status = "partial_success"
	}
	fetchExecution["sync_task_id"] = taskID
	fetchExecution["status"] = status
	fetchExecution["downloaded_excel_count"] = downloadResult["success_count"]
	fetchExecution["failed_excel_count"] = downloadResult["failed_count"]
	fetchExecution["raw_invoice_dir"] = rawDir
	delete(fetchExecution, "last_error")
	markPlannedStepStatus(state.Data, fetchInvoicesStep, status, "")
	if err := state.Save(); err != nil {
		return err
	}
	common.AppendMessageLog("执行取票成功")
	return nil
}

func DownloadExcelFiles(dataList []any, destinationDir string, renameMapRaw any) map[string]any {
	renameMap, _ := renameMapRaw.(map[string]any)
	jobs := []common.DownloadJob{}
	for idx, item := range dataList {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rawURL := strings.TrimSpace(common.StringValue(obj["excelFileUrl"]))
		if rawURL == "" {
			continue
		}
		hint := fmt.Sprintf("invoice_%d.xlsx", idx+1)
		filename := ResolveItemRename(obj, idx+1, renameMap)
		if filename == "" {
			filename = common.FilenameFromURL(rawURL, hint, NormalizeExcelFilename)
		}
		jobs = append(jobs, common.DownloadJob{URL: rawURL, FallbackName: hint, TargetFilename: filename})
	}
	return common.DownloadMany(jobs, destinationDir, "invoice", NormalizeExcelFilename, 120*time.Second, "excelFileUrl")
}

func ResolveItemRename(item map[string]any, idx int, renameMap map[string]any) string {
	for _, key := range []string{"downloadFilename", "targetFilename", "outputFilename", "filename", "fileName"} {
		if value := strings.TrimSpace(common.StringValue(item[key])); value != "" {
			return NormalizeExcelFilename(value)
		}
	}
	candidates := []string{
		common.StringValue(item["excelFileUrl"]),
		fmt.Sprint(idx),
		common.StringValue(item["dateRange"]),
		common.StringValue(item["type"]),
	}
	if candidates[2] != "" && candidates[3] != "" {
		candidates = append(candidates, candidates[2]+"|"+candidates[3])
	}
	for _, candidate := range candidates {
		if value := strings.TrimSpace(common.StringValue(renameMap[candidate])); value != "" {
			return NormalizeExcelFilename(value)
		}
	}
	return ""
}
