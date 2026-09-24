package fetchrisk

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	createDeductionTaskURL = "/deduction/create-task"
	deductionTaskStatusURL = "/deduction/task-status"
	getDeductionDataURL    = "/deduction/get-data"
	defaultPageSize        = 500
)

func FetchDeduction(taskFile string) error {
	_, err := ExecuteFetchDeductionWithOptions(taskFile, FetchCommandOptions{})
	return err
}

func ExecuteFetchDeduction(taskFile string) (map[string]any, error) {
	return ExecuteFetchDeductionWithOptions(taskFile, FetchCommandOptions{})
}

/**
 * ExecuteFetchDeductionWithOptions 使用任务企业快照创建、轮询并读取勾选数据任务。
 *
 * @param taskFile 包含 enterprise_context 的取票任务文件。
 * @param options 可注入的请求客户端、轮询时间和锁参数。
 * @returns 勾选结果；失败时同时返回本地校验或远端业务错误。
 */
func ExecuteFetchDeductionWithOptions(taskFile string, options FetchCommandOptions) (result map[string]any, retErr error) {
	executionOptions := resolveFetchRuntime(options)
	lock, taskPath, err := acquireFetchTaskCommandLock(taskFile, executionOptions)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, lock.Release()) }()
	state, err := LoadTaskState(taskPath)
	if err != nil {
		return nil, err
	}
	if _, err := bindFetchTaskEnterprise(state, &executionOptions); err != nil {
		return nil, err
	}
	params, err := IntentParams(state.Data)
	if err != nil {
		return nil, err
	}
	execution := common.ObjectValue(state.Data, "execution")
	artifacts := common.ObjectValue(state.Data, "artifacts")
	outputs := common.ObjectValue(state.Data, "outputs")
	storage, _ := state.Data["storage"].(map[string]any)
	uscc, err := RequiredString(params, "uscc")
	if err != nil {
		return nil, err
	}
	areaCode, err := RequiredInt(params, "areaCode")
	if err != nil {
		return nil, err
	}
	personalAccount, err := RequiredString(params, "personalAccount")
	if err != nil {
		return nil, err
	}
	profile, err := QueryEnterpriseProfile(executionOptions.client, uscc, areaCode, personalAccount)
	if err != nil {
		return nil, err
	}
	taxpayerType := NormalizeTaxpayerType(profile["taxpayer_type"])
	businessScope := strings.TrimSpace(common.StringValue(profile["business_scope"]))
	params["taxpayer_type"] = taxpayerType
	artifacts["enterprise_profile"] = map[string]any{"taxpayer_type": taxpayerType, "business_scope": businessScope}
	if businessScope != "" && strings.TrimSpace(common.StringValue(params["business_scope"])) == "" {
		params["business_scope"] = businessScope
	}
	if taxpayerType != "一般纳税人" {
		execution["fetch_deduction"] = map[string]any{
			"status":             "skipped",
			"skip_reason":        fmt.Sprintf("企业纳税人类型为%s，无需获取勾选已认证数据", emptyDefault(taxpayerType, "未知")),
			"taxpayer_type":      taxpayerType,
			"enterprise_profile": profile,
		}
		delete(artifacts, "deduction_data_file")
		delete(artifacts, "deduction_data_list")
		delete(outputs, "deduction_data_file")
		outputs["deduction_markdown"] = fmt.Sprintf("### 获取勾选已认证数据\n\n- 纳税人类型: %s\n- 结果: 已跳过", emptyDefault(taxpayerType, "未知"))
		invalidateAfterDeduction(state.Data, "勾选步骤已重新执行，必须重新生成发票清单和下游分析")
		markPlannedStepStatus(state.Data, fetchDeductionStep, "skipped", common.StringValue(execution["fetch_deduction"].(map[string]any)["skip_reason"]))
		if err := state.Save(); err != nil {
			return nil, err
		}
		return map[string]any{"status": "skipped", "task_file": state.Path, "taxpayer_type": taxpayerType}, nil
	}
	spec := deductionSubmissionSpec()
	createPayload, err := spec.BuildPayload(params, executionOptions.now())
	if err != nil {
		return nil, err
	}
	if err := executionOptions.client.ValidateJSONRequest(createPayload); err != nil {
		return nil, err
	}
	skssqStart := common.StringValue(createPayload["skssqStart"])
	skssqEnd := common.StringValue(createPayload["skssqEnd"])
	months, err := IterateMonths(skssqStart, skssqEnd)
	if err != nil {
		return nil, err
	}
	submissionLock, fingerprint, err := acquireFetchSubmissionCommandLock(spec, createPayload, executionOptions)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, submissionLock.Release()) }()
	taskID, err := ensureFetchRemoteTask(state, spec, createPayload, fingerprint, executionOptions)
	if err != nil {
		return nil, err
	}
	fetchDeduction := common.ObjectValue(execution, fetchDeductionStep)
	_, pollErr := common.PollTaskStatus(executionOptions.client, deductionTaskStatusURL, taskID, uscc, "勾选数据任务失败", "勾选数据任务", executionOptions.pollInterval, executionOptions.pollTimeout)
	if err := markFetchPollingResult(state, fetchDeductionStep, pollErr, executionOptions.now()); err != nil {
		return nil, err
	}
	combined := []any{}
	monthlyCounts := []any{}
	for _, month := range months {
		items, err := FetchMonthDeductionData(executionOptions.client, uscc, taskID, month, defaultPageSize)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			combined = append(combined, item)
		}
		monthlyCounts = append(monthlyCounts, map[string]any{"skssq": month, "count": len(items)})
	}
	outputPath := ResolveDeductionOutputPath(state.Data, artifacts, storage)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, err
	}
	outputPayload := map[string]any{
		"taskID": taskID, "uscc": uscc, "skssqStart": skssqStart, "skssqEnd": skssqEnd,
		"months": months, "monthlyCounts": monthlyCounts, "totalCount": len(combined), "list": combined,
	}
	raw, _ := json.MarshalIndent(outputPayload, "", "  ")
	if err := os.WriteFile(outputPath, append(raw, '\n'), 0o644); err != nil {
		return nil, err
	}
	outputs["deduction_data_file"] = outputPath
	delete(artifacts, "deduction_data_list")
	delete(outputs, "deduction_markdown")
	fetchDeduction["deduction_task_id"] = taskID
	fetchDeduction["status"] = "success"
	fetchDeduction["skssq_start"] = skssqStart
	fetchDeduction["skssq_end"] = skssqEnd
	fetchDeduction["month_count"] = len(months)
	fetchDeduction["deduction_count"] = len(combined)
	delete(fetchDeduction, "last_error")
	invalidateAfterDeduction(state.Data, "勾选认证数据已更新，必须重新生成发票清单和下游分析")
	markPlannedStepStatus(state.Data, fetchDeductionStep, "success", "")
	if err := state.Save(); err != nil {
		return nil, err
	}
	return map[string]any{
		"status": "success", "task_file": state.Path, "taxpayer_type": taxpayerType,
		"deduction_task_id": taskID, "skssq_start": skssqStart, "skssq_end": skssqEnd,
		"month_count": len(months), "deduction_count": len(combined), "data_file_path": outputPath,
	}, nil
}

func NormalizeTaxpayerType(value any) string {
	normalized := strings.TrimSpace(common.StringValue(value))
	switch normalized {
	case "一般纳税人", "一般纳税人企业", "一般":
		return "一般纳税人"
	case "小规模纳税人", "小规模":
		return "小规模纳税人"
	default:
		return normalized
	}
}

func NormalizeMonthValue(value any, fieldName string) (string, error) {
	re := regexp.MustCompile(`(20\d{2})[-/]?(0[1-9]|1[0-2])`)
	match := re.FindStringSubmatch(common.StringValue(value))
	if len(match) != 3 {
		return "", fmt.Errorf("%s 无法解析所属月份: %v", fieldName, value)
	}
	return match[1] + match[2], nil
}

func IterateMonths(startMonth, endMonth string) ([]string, error) {
	start, err := time.Parse("200601", startMonth)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("200601", endMonth)
	if err != nil {
		return nil, err
	}
	if start.After(end) {
		return nil, fmt.Errorf("所属期范围非法: %s > %s", startMonth, endMonth)
	}
	months := []string{}
	for current := start; !current.After(end); current = current.AddDate(0, 1, 0) {
		months = append(months, current.Format("200601"))
	}
	return months, nil
}

func QueryEnterpriseProfile(client *common.APIClient, uscc string, areaCode int, personalAccount string) (map[string]any, error) {
	payload := map[string]any{"uscc": uscc, "areaCode": areaCode, "personalAccount": personalAccount}
	resp, err := client.PostSuccessJSON("/issue-info/user-info", payload, "按税号查询企业信息", 15*time.Second)
	if err != nil {
		return nil, err
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		return map[string]any{}, nil
	}
	taxpayerType := "待人工核验"
	switch strings.TrimSpace(common.StringValue(data["nsrlx"])) {
	case "1":
		taxpayerType = "一般纳税人"
	case "2":
		taxpayerType = "小规模纳税人"
	}
	return map[string]any{
		"status":         "success",
		"taxpayer_type":  taxpayerType,
		"business_scope": strings.TrimSpace(common.StringValue(data["jyfw"])),
	}, nil
}

func FetchMonthDeductionData(client *common.APIClient, uscc string, taskID int, skssq string, pageSize int) ([]any, error) {
	items := []any{}
	itemID := 0
	for {
		payload := map[string]any{
			"uscc": uscc, "skssq": intFrom(skssq),
			"page":   map[string]any{"itemId": itemID, "pageSize": pageSize},
			"taskID": taskID, "needInvoiceItemVO": false,
		}
		resp, err := client.PostSuccessJSON(getDeductionDataURL, payload, "查询勾选已认证数据", 30*time.Second)
		if err != nil {
			return nil, err
		}
		data, _ := resp["data"].(map[string]any)
		current, _ := data["list"].([]any)
		items = append(items, current...)
		if len(current) < pageSize {
			return items, nil
		}
		last, _ := current[len(current)-1].(map[string]any)
		nextID := intFrom(last["itemId"])
		if nextID == 0 {
			return items, nil
		}
		itemID = nextID
	}
}

func ResolveDeductionOutputPath(state map[string]any, artifacts map[string]any, storage map[string]any) string {
	for _, value := range []any{storage["deduction_data_file"], artifacts["deduction_data_file"]} {
		if path := strings.TrimSpace(common.StringValue(value)); path != "" {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}
	if root := strings.TrimSpace(common.StringValue(storage["root_dir"])); root != "" {
		abs, _ := filepath.Abs(filepath.Join(root, "勾选已认证数据.json"))
		return abs
	}
	abs, _ := filepath.Abs(filepath.Join(common.StringValue(state["workspace_dir"]), RunsDirName, "勾选已认证数据.json"))
	return abs
}

func emptyDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
