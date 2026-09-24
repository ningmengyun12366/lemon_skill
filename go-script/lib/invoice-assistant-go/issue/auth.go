package issue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	issueAuthQRCodeURL      = "/issue-auth/qrcode"
	issueAuthCheckStatusURL = "/issue-auth/check-qrcode-status"
	issueAuthStatusURL      = "/issue-auth/auth-status"
	issueAuthDirName        = "开票权限认证"
)

var issueAuthSuccessCodes = map[int]bool{0: true}

func IssueAuthWorkflow(action, taskFile, inputPath string, pretty bool) error {
	_, err := issueAuthWorkflowResult(action, taskFile, inputPath, pretty, nil)
	return err
}

func IssueAuthWorkflowWithOptions(action, taskFile, inputPath string, pretty bool, execution *IssueExecution) error {
	_, err := issueAuthWorkflowResult(action, taskFile, inputPath, pretty, execution)
	return err
}

/**
 * IssueAuthWorkflowOutput 执行认证动作并返回不含二维码原文和内部认证编号的 CLI 输出。
 * @param action 认证动作，仅支持 start 或 verify。
 * @param taskFile 开票任务文件路径。
 * @param inputPath start 动作允许使用的可选覆盖输入文件。
 * @param pretty 是否格式化保存任务 JSON。
 * @returns 可安全输出到 stdout 的认证摘要；失败时返回错误。
 */
func IssueAuthWorkflowOutput(action, taskFile, inputPath string, pretty bool) (map[string]any, error) {
	result, err := issueAuthWorkflowResult(action, taskFile, inputPath, pretty, nil)
	if err != nil {
		return nil, err
	}
	return buildIssueAuthCommandOutput(action, result), nil
}

/**
 * issueAuthWorkflowResult 执行指定认证动作并保留完整结果供内部持久化和安全摘要组装。
 * @param action 认证动作，仅支持 start 或 verify。
 * @param taskFile 开票任务文件路径。
 * @param inputPath start 动作允许使用的可选覆盖输入文件。
 * @param pretty 是否格式化保存任务 JSON。
 * @param execution 可选的请求执行依赖。
 * @returns 完整认证结果；参数、登录或远端调用失败时返回错误。
 */
func issueAuthWorkflowResult(action, taskFile, inputPath string, pretty bool, execution *IssueExecution) (map[string]any, error) {
	state, err := loadTaskStateFromAuthArgs(taskFile)
	if err != nil {
		return nil, err
	}
	var override map[string]any
	if strings.TrimSpace(inputPath) != "" {
		override, err = loadJSONInput(inputPath)
		if err != nil {
			return nil, err
		}
	}
	var result map[string]any
	switch action {
	case "start":
		result, err = ExecuteAuthStartWithExecution(state, override, pretty, execution)
	case "verify":
		result, err = ExecuteAuthVerifyWithExecution(state, pretty, execution)
	default:
		err = fmt.Errorf("unknown issue auth action: %s", action)
	}
	return result, err
}

func ExecuteAuthStart(state *TaskState, rawInputOverride map[string]any, pretty bool) (map[string]any, error) {
	return ExecuteAuthStartWithExecution(state, rawInputOverride, pretty, nil)
}

/**
 * ExecuteAuthStartWithExecution 使用任务企业快照发起开票二维码认证流程。
 *
 * @param state 已加载的开票任务。
 * @param rawInputOverride 本次允许覆盖的认证业务参数。
 * @param pretty 是否格式化保存任务 JSON。
 * @param execution 可选的请求执行依赖。
 * @returns 认证启动结果，失败时返回本地校验或远端接口错误。
 */
func ExecuteAuthStartWithExecution(state *TaskState, rawInputOverride map[string]any, pretty bool, execution *IssueExecution) (map[string]any, error) {
	if _, err := common.EnterpriseContextFromTask(state.Data); err != nil {
		return nil, err
	}
	context, err := buildAuthContext(state.Data, rawInputOverride)
	if err != nil {
		return nil, err
	}
	authExecution, err := resolveIssueAuthExecution(state.Data, context, execution)
	if err != nil {
		return nil, err
	}
	if err := authExecution.Client.ValidateJSONRequest(context); err != nil {
		return nil, err
	}
	if _, err := prepareIssueLoginReadiness(authExecution.LoginRequest); err != nil {
		clearIssueAuthQRCodeState(ensureIssueAuthResult(state.Data))
		return nil, saveAuthLoginRequired(state, context, nil, nil, err, pretty)
	}
	authResult := ensureIssueAuthResult(state.Data)
	clearIssueAuthQRCodeState(authResult)
	authResult["context"] = context
	authResult["request_context"] = context
	authResult["status"] = "qrcode_requesting"
	authResult["updated_at"] = common.NowISO()
	state.Data["task_status"] = "auth_qrcode_requesting"
	if err := state.Save(pretty); err != nil {
		return nil, err
	}
	resp, err := submitIssueAuthRequest(authExecution.Client, issueAuthQRCodeURL, context)
	if err != nil {
		return nil, err
	}
	if err := ensureExpectedCode(resp, issueAuthSuccessCodes, "获取开票权限认证二维码"); err != nil {
		return nil, err
	}
	entries := normalizeQRCodeEntries(resp)
	if len(entries) == 0 {
		return nil, fmt.Errorf("获取二维码成功，但未返回有效二维码")
	}
	markdownPath := buildAuthMarkdownPath(state, "issue-auth-qrcode")
	if err := persistLocalQRCodeImages(markdownPath, entries); err != nil {
		return nil, err
	}
	markdown := buildQRCodeMarkdown(context, entries)
	if err := writeTextFile(markdownPath, markdown); err != nil {
		return nil, fmt.Errorf("保存开票权限认证 Markdown 失败: %w", err)
	}
	now := common.NowISO()
	authResult["context"] = context
	authResult["request_context"] = context
	authResult["status"] = "awaiting_scan"
	authResult["updated_at"] = now
	authResult["qrcode_response"] = resp
	authResult["qrcode_list"] = entries
	authResult["qrcode_entries"] = entries
	authResult["qrcode_markdown"] = markdown
	authResult["qrcode_markdown_path"] = markdownPath
	authResult["started_at"] = now
	state.Data["task_status"] = "auth_qrcode_generated"
	if err := state.Save(pretty); err != nil {
		return nil, err
	}
	return authResult, nil
}

/**
 * buildIssueAuthCommandOutput 只保留 Agent 展示认证状态所需的路径和平台信息。
 * @param action 当前执行的认证动作。
 * @param authResult 完整认证结果。
 * @returns 不含 Base64、二维码原文、rzid 和下游原始响应的 CLI 安全摘要。
 */
func buildIssueAuthCommandOutput(action string, authResult map[string]any) map[string]any {
	summary := map[string]any{
		"status": common.StringValue(authResult["status"]),
	}
	for _, key := range []string{"qrcode_markdown_path", "verify_markdown_path"} {
		if value := strings.TrimSpace(common.StringValue(authResult[key])); value != "" {
			summary[key] = value
		}
	}
	sanitizedEntries := make([]map[string]string, 0)
	imagePaths := make([]string, 0)
	for _, entry := range qrcodeEntriesFromAuthResult(authResult) {
		imagePath := strings.TrimSpace(entry[qrcodeImagePathKey])
		if imagePath == "" {
			continue
		}
		sanitizedEntries = append(sanitizedEntries, map[string]string{
			"qrcodeType":       firstString(entry["qrcodeType"], entry["type"]),
			"qrcodeTypeLabel":  firstString(entry["qrcodeTypeLabel"], entry["label"]),
			qrcodeImagePathKey: imagePath,
		})
		imagePaths = append(imagePaths, imagePath)
	}
	if len(sanitizedEntries) > 0 {
		summary["qrcode_list"] = sanitizedEntries
		summary["qrcode_image_paths"] = imagePaths
	}
	return map[string]any{
		"action":            action,
		"issue_auth_result": summary,
	}
}

func ExecuteAuthVerify(state *TaskState, pretty bool) (map[string]any, error) {
	return ExecuteAuthVerifyWithExecution(state, pretty, nil)
}

/**
 * ExecuteAuthVerifyWithExecution 使用任务企业快照续查并确认开票认证状态。
 *
 * @param state 已加载的开票任务。
 * @param pretty 是否格式化保存任务 JSON。
 * @param execution 可选的请求执行依赖。
 * @returns 认证确认结果，失败时返回本地校验或远端接口错误。
 */
func ExecuteAuthVerifyWithExecution(state *TaskState, pretty bool, execution *IssueExecution) (map[string]any, error) {
	if _, err := common.EnterpriseContextFromTask(state.Data); err != nil {
		return nil, err
	}
	authResult := ensureIssueAuthResult(state.Data)
	contextOverride, _ := authResult["context"].(map[string]any)
	if contextOverride == nil {
		contextOverride, _ = authResult["request_context"].(map[string]any)
	}
	context, err := buildAuthContext(state.Data, contextOverride)
	if err != nil {
		return nil, err
	}
	checkPayload, err := buildCheckPayload(authResult, context)
	if err != nil {
		return nil, err
	}
	authExecution, err := resolveIssueAuthExecution(state.Data, context, execution)
	if err != nil {
		return nil, err
	}
	if err := authExecution.Client.ValidateJSONRequest(checkPayload); err != nil {
		return nil, err
	}
	statusEntries := storedSuccessfulScanStatusEntries(authResult)
	scanResp, _ := authResult["scan_status_response"].(map[string]any)
	if len(statusEntries) == 0 {
		scanResp, err = submitIssueAuthRequest(authExecution.Client, issueAuthCheckStatusURL, checkPayload)
		if err != nil {
			return nil, err
		}
		if err := ensureExpectedCode(scanResp, issueAuthSuccessCodes, "查询扫码状态"); err != nil {
			return nil, err
		}
		statusEntries = normalizeScanStatusEntries(scanResp)
		if len(statusEntries) == 0 {
			return nil, fmt.Errorf("查询扫码状态成功，但未返回有效状态")
		}
		if err := saveAuthScanStatus(state, authResult, context, checkPayload, scanResp, statusEntries, pretty); err != nil {
			return nil, err
		}
	}
	var statusResp map[string]any
	if hasSuccessfulScan(statusEntries) {
		if _, err := prepareIssueLoginReadiness(authExecution.LoginRequest); err != nil {
			return nil, saveAuthLoginRequired(state, context, statusEntries, nil, err, pretty)
		}
		statusResp, err = submitIssueAuthRequest(authExecution.Client, issueAuthStatusURL, context)
		if err != nil {
			return nil, err
		}
		if err := ensureExpectedCode(statusResp, issueAuthSuccessCodes, "查询认证状态"); err != nil {
			if responseCode(statusResp) == 2006 {
				readinessErr := refreshIssueAuthLoginReadiness(authExecution.LoginRequest)
				if readinessErr != nil {
					err = fmt.Errorf("%v\n%w", err, readinessErr)
				}
				return nil, saveAuthLoginRequired(state, context, statusEntries, statusResp, err, pretty)
			}
			return nil, err
		}
	}
	markdown := buildVerifyMarkdown(context, statusEntries, statusResp)
	markdownPath := buildAuthMarkdownPath(state, "issue-auth-verify")
	_ = writeTextFile(markdownPath, markdown)
	currentStatus := buildVerifyStatus(statusEntries, statusResp)
	now := common.NowISO()
	authResult["context"] = context
	authResult["request_context"] = context
	authResult["status"] = currentStatus
	authResult["updated_at"] = now
	authResult["scan_status_request_payload"] = checkPayload
	if scanResp != nil {
		authResult["scan_status_response"] = scanResp
	}
	authResult["scan_status_list"] = statusEntries
	authResult["scan_status_entries"] = statusEntries
	if statusResp != nil {
		authResult["auth_status_response"] = statusResp
	} else {
		delete(authResult, "auth_status_response")
	}
	authResult["verify_status"] = currentStatus
	authResult["verify_markdown"] = markdown
	authResult["verify_markdown_path"] = markdownPath
	authResult["verified_at"] = now
	delete(authResult, "error_message")
	delete(authResult, "login_readiness_error")
	state.Data["task_status"] = currentStatus
	if err := state.Save(pretty); err != nil {
		return nil, err
	}
	return authResult, nil
}

func clearIssueAuthQRCodeState(authResult map[string]any) {
	for _, key := range []string{
		"qrcode_response",
		"qrcode_list",
		"qrcode_entries",
		"qrcode_markdown",
		"qrcode_markdown_path",
		"started_at",
	} {
		delete(authResult, key)
	}
	clearIssueAuthVerificationState(authResult)
}

func clearIssueAuthVerificationState(authResult map[string]any) {
	for _, key := range []string{
		"scan_status_request_payload",
		"scan_status_response",
		"scan_status_list",
		"scan_status_entries",
		"scan_checked_at",
		"auth_status_response",
		"verify_markdown",
		"verify_markdown_path",
		"verified_at",
		"verify_status",
		"error_message",
		"login_readiness_error",
	} {
		delete(authResult, key)
	}
}

type issueAuthExecution struct {
	Client       *common.APIClient
	LoginRequest common.LoginReadinessRequest
}

func resolveIssueAuthExecution(taskState, context map[string]any, execution *IssueExecution) (issueAuthExecution, error) {
	config := common.LoadWorkspaceConfig()
	baseClient := newAPIClient()
	if execution != nil {
		if execution.WorkspaceConfig != nil {
			config = execution.WorkspaceConfig
		}
		if execution.BaseClient != nil {
			baseClient = execution.BaseClient
		}
	}
	taskContext, err := resolveIssueTaskContext(taskState, context)
	if err != nil {
		return issueAuthExecution{}, err
	}
	client := common.NewEnterpriseAPIClient(baseClient, config, taskContext.Request)
	loginRequest := common.LoginReadinessRequest{
		Enterprise: taskContext.Enterprise,
		Config:     config,
		Client:     client,
	}
	return issueAuthExecution{Client: client, LoginRequest: loginRequest}, nil
}

func refreshIssueAuthLoginReadiness(request common.LoginReadinessRequest) error {
	_, err := prepareIssueLoginReadiness(request)
	return err
}

func saveAuthScanStatus(
	state *TaskState,
	authResult, context, checkPayload, scanResp map[string]any,
	statusEntries []map[string]string,
	pretty bool,
) error {
	now := common.NowISO()
	currentStatus := buildVerifyStatus(statusEntries, nil)
	authResult["context"] = context
	authResult["request_context"] = context
	authResult["status"] = currentStatus
	authResult["verify_status"] = currentStatus
	authResult["updated_at"] = now
	authResult["scan_status_request_payload"] = checkPayload
	authResult["scan_status_response"] = scanResp
	authResult["scan_status_list"] = statusEntries
	authResult["scan_status_entries"] = statusEntries
	authResult["scan_checked_at"] = now
	delete(authResult, "error_message")
	delete(authResult, "login_readiness_error")
	state.Data["task_status"] = currentStatus
	return state.Save(pretty)
}

func saveAuthLoginRequired(
	state *TaskState,
	context map[string]any,
	statusEntries []map[string]string,
	statusResp map[string]any,
	cause error,
	pretty bool,
) error {
	authResult := ensureIssueAuthResult(state.Data)
	now := common.NowISO()
	message := strings.TrimSpace(cause.Error())
	authResult["context"] = context
	authResult["request_context"] = context
	authResult["status"] = "login_required"
	authResult["verify_status"] = "login_required"
	authResult["updated_at"] = now
	authResult["error_message"] = message
	authResult["login_readiness_error"] = message
	if len(statusEntries) > 0 {
		authResult["scan_status_list"] = statusEntries
		authResult["scan_status_entries"] = statusEntries
	}
	if statusResp != nil {
		authResult["auth_status_response"] = statusResp
	}
	state.Data["task_status"] = "login_required"
	if err := state.Save(pretty); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func ensureIssueAuthResult(taskState map[string]any) map[string]any {
	result := common.ObjectValue(taskState, "issue_auth_result")
	return result
}

func loadTaskStateFromAuthArgs(taskFile string) (*TaskState, error) {
	return LoadTaskState(taskFile)
}

func buildAuthContext(taskState map[string]any, rawInputOverride map[string]any) (map[string]any, error) {
	rawInput := rawInputOverride
	if rawInput == nil {
		rawInput, _ = taskState["raw_input_json"].(map[string]any)
	}
	if rawInput == nil {
		return nil, fmt.Errorf("任务状态缺少 raw_input_json，无法提取认证参数")
	}
	uscc := strings.TrimSpace(common.StringValue(rawInput["uscc"]))
	personalAccount := strings.TrimSpace(common.StringValue(rawInput["personalAccount"]))
	areaCode := rawInput["areaCode"]
	missingFields := []string{}
	if uscc == "" {
		missingFields = append(missingFields, "uscc")
	}
	if !HasMeaningfulValue(areaCode) {
		missingFields = append(missingFields, "areaCode")
	}
	if personalAccount == "" {
		missingFields = append(missingFields, "personalAccount")
	}
	if len(missingFields) > 0 {
		return nil, fmt.Errorf("缺少认证必填参数: %s", strings.Join(missingFields, ", "))
	}
	normalizedAreaCode, err := coerceAuthAreaCode(areaCode)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"uscc":            uscc,
		"areaCode":        normalizedAreaCode,
		"personalAccount": personalAccount,
	}, nil
}

func coerceAuthAreaCode(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	}
	text := strings.TrimSpace(common.StringValue(value))
	if text == "" {
		return 0, fmt.Errorf("缺少 areaCode，无法执行开票权限认证")
	}
	out, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("areaCode 不是合法整数: %v", value)
	}
	return out, nil
}

func submitIssueAuthRequest(client *common.APIClient, requestURL string, payload map[string]any) (map[string]any, error) {
	if client == nil {
		client = newAPIClient()
	}
	resp, err := client.PostJSON(requestURL, payload, 30*time.Second)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func ensureExpectedCode(result map[string]any, expected map[int]bool, stageName string) error {
	code := responseCode(result)
	if !expected[code] {
		return fmt.Errorf("%s失败：code=%v，message=%v", stageName, result["code"], result["message"])
	}
	return nil
}

func buildAuthMarkdownPath(state *TaskState, stageName string) string {
	if root := strings.TrimSpace(common.StringValue(GetPath(state.Data, "storage.root_dir"))); root != "" {
		return filepath.Join(root, issueAuthDirName, fileStem(state.Path)+"-"+stageName+".md")
	}
	root := strings.TrimSpace(common.StringValue(state.Data["workspace_dir"]))
	if root == "" {
		root, _ = ResolveWorkspaceDir()
	}
	return filepath.Join(root, "invoice_issue", issueAuthDirName, fileStem(state.Path)+"-"+stageName+".md")
}

func loadJSONFileLoose(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
