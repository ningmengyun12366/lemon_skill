package common

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/skillupdate"
)

const (
	precheckDirname         = "preflight"
	preflightResultsDirname = "preflight-results"
	loginTypeKey            = "loginType"
	quickAutoLoginEnabled   = true
	enterpriseListURL       = "/share/enterprise-list"
	loginStatusURL          = "/login/login-status"
	runningTaskURL          = "/login/checkOtherTaxNumberRunningTask"
	quickAutoLoginCheckURL  = "/login/survivalStatusDetection"
	quickAutoLoginURL       = "/login/quick-auto-login"
)

var defaultBindingURL = PortalBaseURL + "/workspace"
var defaultSignupURL = PortalBaseURL + "/workspace"
var defaultLoginURL = PortalBaseURL + "/shareVerification"

var loginTypeToYWLX = map[string]string{"1": "DLYW", "2": "QYYW"}
var quickAutoLoginRetryDelays = []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second, 8 * time.Second, 10 * time.Second}
var temporarilySkippedCheckpoints = []string{}

type checkIssue struct {
	Code       string `json:"code"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion"`
	Status     string `json:"status"`
}

func TaskPreflightCheck(enterpriseName string) map[string]any {
	return TaskPreflightCheckWithOptions(enterpriseName, PreflightOptions{RequireAPIKeyConfirmation: true})
}

type PreflightOptions struct {
	RequireAPIKeyConfirmation bool
	UseConfiguredAPIKey       bool
	DeferLoginChecks          bool
}

func TaskPreflightCheckWithOptions(enterpriseName string, options PreflightOptions) map[string]any {
	result, err := runPreflightWithOptions(enterpriseName, NewAPIClient(), options)
	if err != nil {
		return map[string]any{
			"status":                "failed",
			"preflight_passed":      false,
			"enterprise_name":       strings.TrimSpace(enterpriseName),
			"config_path":           ConfigPath(),
			"default_workspace_dir": DefaultWorkspaceDir(),
			"issues": []map[string]any{{
				"code":       "preflight_runtime_error",
				"title":      "预检执行失败",
				"detail":     err.Error(),
				"suggestion": "请检查 workspace-config.json 是否可读写，并稍后重试。",
				"status":     "failed",
			}},
			"problem_count": 1,
			"generated_at":  NowISO(),
		}
	}
	return result
}

func runPreflight(enterpriseName string, client *APIClient) (map[string]any, error) {
	return runPreflightWithOptions(enterpriseName, client, PreflightOptions{})
}

func runPreflightWithOptions(enterpriseName string, client *APIClient, options PreflightOptions) (map[string]any, error) {
	config := LoadWorkspaceConfig()
	workspaceDir, workspacePresent, defaultCandidate := resolveWorkspaceDir(config)
	issues := []checkIssue{}
	bindingResult := preflightPending("binding_check")
	loginStatusResult := preflightPending("login_status")
	selectedEnterprise := strings.TrimSpace(enterpriseName)

	if err := ensureWritableDir(workspaceDir); err != nil {
		issues = append(issues, checkIssue{
			Code:       "workspace_dir_not_writable",
			Title:      "工作区目录不可写",
			Detail:     fmt.Sprintf("工作区目录不可创建或不可写：%s；错误信息：%v", workspaceDir, err),
			Suggestion: "请调整目录权限或改用其他可写的绝对路径作为工作区目录。",
			Status:     "failed",
		})
	}

	apikey := strings.TrimSpace(StringValue(config[APIKey]))
	if apikey == "" {
		issues = append(issues, checkIssue{
			Code:       "apikey_missing",
			Title:      "未配置 API Key",
			Detail:     "workspace-config.json 中未找到 apikey。",
			Suggestion: "请补充 apikey，或前往以下网址注册后再回填：" + defaultSignupURL,
			Status:     "failed",
		})
		return finishPreflight(enterpriseName, selectedEnterprise, workspaceDir, workspacePresent, defaultCandidate, apikey, bindingResult, loginStatusResult, issues, "apikey")
	}
	if options.RequireAPIKeyConfirmation && !options.UseConfiguredAPIKey {
		issues = append(issues, checkIssue{
			Code:       "apikey_confirmation_required",
			Title:      "需要确认 API Key",
			Detail:     "检测到 workspace-config.json 中已配置 API Key。继续执行前，请确认使用当前 API Key，或改用新的 API Key。",
			Suggestion: "若使用当前 API Key，请确认后重新执行预检并追加 --use-config-apikey；若使用新的 API Key，请先通过 workspace-config-writer --apikey <新 API Key> 更新后再执行预检。",
			Status:     "failed",
		})
		return finishPreflight(enterpriseName, selectedEnterprise, workspaceDir, workspacePresent, defaultCandidate, apikey, bindingResult, loginStatusResult, issues, "apikey_confirmation")
	}

	bindingOK, binding, bindingIssues := checkBoundEnterprises(enterpriseName, apikey, client)
	bindingResult = binding
	issues = append(issues, bindingIssues...)
	selectedEnterprise = strings.TrimSpace(pickFirstNonEmpty(bindingResult["selected_enterprise"], enterpriseName))
	if !bindingOK {
		return finishPreflight(enterpriseName, selectedEnterprise, workspaceDir, workspacePresent, defaultCandidate, apikey, bindingResult, loginStatusResult, issues, "binding_check")
	}
	if options.DeferLoginChecks {
		loginStatusResult = preflightDeferred("login_status")
		return finishPreflight(enterpriseName, selectedEnterprise, workspaceDir, workspacePresent, defaultCandidate, apikey, bindingResult, loginStatusResult, issues, "")
	}

	loginOK, loginResult, loginIssues := checkLoginStatus(selectedEnterprise, apikey, config, bindingResult, client, quickAutoLoginEnabled)
	loginStatusResult = loginResult
	issues = append(issues, loginIssues...)
	if !loginOK {
		return finishPreflight(enterpriseName, selectedEnterprise, workspaceDir, workspacePresent, defaultCandidate, apikey, bindingResult, loginStatusResult, issues, "login_status")
	}

	return finishPreflight(enterpriseName, selectedEnterprise, workspaceDir, workspacePresent, defaultCandidate, apikey, bindingResult, loginStatusResult, issues, "")
}

func finishPreflight(enterpriseName, selectedEnterprise, workspaceDir string, workspacePresent bool, defaultCandidate, apikey string, bindingResult, loginStatusResult map[string]any, issues []checkIssue, stoppedAt string) (map[string]any, error) {
	markdownPath, err := writePreflightProblemMarkdown(issues)
	if err != nil {
		return nil, err
	}
	passed := true
	issueMaps := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		if issue.Status == "failed" {
			passed = false
		}
		issueMaps = append(issueMaps, map[string]any{
			"code":       issue.Code,
			"title":      issue.Title,
			"detail":     issue.Detail,
			"suggestion": issue.Suggestion,
			"status":     issue.Status,
		})
	}
	result := map[string]any{
		"status":                          map[bool]string{true: "success", false: "failed"}[passed],
		"preflight_passed":                passed,
		"config_path":                     ConfigPath(),
		"workspace_dir":                   workspaceDir,
		"workspace_dir_present":           workspacePresent,
		"default_workspace_dir":           DefaultWorkspaceDir(),
		"default_workspace_candidate":     defaultCandidate,
		"apikey_present":                  strings.TrimSpace(apikey) != "",
		"enterprise_name":                 strings.TrimSpace(enterpriseName),
		"selected_enterprise":             selectedEnterprise,
		"login_status":                    loginStatusResult,
		"binding_check":                   bindingResult,
		"temporarily_skipped_checkpoints": temporarilySkippedCheckpoints,
		"issues":                          issueMaps,
		"problem_markdown_path":           markdownPath,
		"problem_count":                   len(issues),
		"generated_at":                    NowISO(),
	}
	if stoppedAt != "" {
		result["stopped_at"] = stoppedAt
	}
	jsonPath := strings.TrimSuffix(markdownPath, filepath.Ext(markdownPath)) + ".json"
	if err := WriteJSONFile(jsonPath, result); err != nil {
		return nil, err
	}
	result["result_json_path"] = jsonPath
	_ = WriteJSONFile(jsonPath, result)
	return result, nil
}

func preflightPending(step string) map[string]any {
	return map[string]any{"checked": false, "mode": "pending", "step": step}
}

func preflightDeferred(step string) map[string]any {
	return map[string]any{"checked": false, "mode": "deferred", "status": "deferred", "step": step, "reason": InvoiceIssueCapabilityCode}
}

func resolveWorkspaceDir(config map[string]any) (string, bool, string) {
	if workspace := strings.TrimSpace(StringValue(config[WorkspaceKey])); workspace != "" {
		return NormalizeWorkspaceDir(workspace), true, ""
	}
	defaultDir := DefaultWorkspaceDir()
	return defaultDir, false, defaultDir
}

/**
 * checkBoundEnterprises 查询 API Key 绑定企业并返回本次预检选择，不写入工作区企业状态。
 *
 * @param enterpriseName 用户本次明确选择的企业名称或 USCC；空值时仅允许现有预检展示逻辑处理。
 * @param apikey 当前工作区 API Key。
 * @param client 用于访问 Gateway 的 APIClient。
 * @returns 是否匹配成功、只供本次预检使用的企业结果以及问题列表。
 */
func checkBoundEnterprises(enterpriseName, apikey string, client *APIClient) (bool, map[string]any, []checkIssue) {
	normalizedName := strings.TrimSpace(enterpriseName)
	result := map[string]any{
		"checked":                true,
		"mode":                   "api",
		"apikey_present":         strings.TrimSpace(apikey) != "",
		"enterprise_name":        normalizedName,
		"matched":                false,
		"requires_confirmation":  false,
		"bound_enterprise_count": 0,
		"selected_enterprise":    "",
		"binding_url":            defaultBindingURL,
		"response_message":       "",
		"enterprise_names":       []string{},
		"enterprise_list":        []map[string]any{},
		"fuzzy_candidates":       []map[string]any{},
		"matched_enterprise":     nil,
	}
	// enterprise-list 按接口契约只在请求体传 API Key，不发送公共登录接口的鉴权请求头。
	enterpriseListClient := *client
	enterpriseListClient.HeaderProvider = func() map[string]string {
		return map[string]string{"Content-Type": "application/json"}
	}
	payload, err := enterpriseListClient.PostJSON(
		enterpriseListURL,
		map[string]any{"apiKey": apikey},
		15*time.Second,
	)
	if err != nil {
		if outdated, ok := skillupdate.FindOutdatedError(err); ok {
			return false, result, []checkIssue{{
				Code:       "skill_version_outdated",
				Title:      "Skill 版本过低",
				Detail:     outdated.Error(),
				Suggestion: "请按提示重新安装最新版 Skill 后重试。",
				Status:     "failed",
			}}
		}
		if textHasCapabilityUnauthorized(err.Error()) {
			return false, result, []checkIssue{capabilityUnauthorizedIssue("查询企业绑定信息", err.Error())}
		}
		return false, result, []checkIssue{{Code: "enterprise_binding_request_error", Title: "获取企业绑定信息失败", Detail: fmt.Sprintf("调用 /share/enterprise-list 失败：%v", err), Suggestion: "请前往以下网址补充信息，或稍后重试：" + defaultBindingURL, Status: "failed"}}
	}
	message := strings.TrimSpace(StringValue(payload["message"]))
	result["response_message"] = message
	if responseHasCapabilityUnauthorized(payload) {
		return false, result, []checkIssue{capabilityUnauthorizedIssue("查询企业绑定信息", message)}
	}
	data, ok := payload["data"].([]any)
	if !codeEquals(payload["code"], 200) || message != "查询成功" || !ok {
		return false, result, []checkIssue{{Code: "enterprise_binding_query_failed", Title: "获取企业绑定信息失败", Detail: pickFirstNonEmpty(message, "调用 /share/enterprise-list 未返回预期成功结果。"), Suggestion: "请前往以下网址补充信息，或稍后重试：" + defaultBindingURL, Status: "failed"}}
	}
	enterprises := []map[string]any{}
	names := []string{}
	for _, item := range data {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		record := normalizeEnterpriseRecord(obj)
		enterprises = append(enterprises, record)
		if name := strings.TrimSpace(StringValue(record["enterpriseName"])); name != "" {
			names = append(names, name)
		}
	}
	result["enterprise_list"] = enterprises
	result["enterprise_names"] = names
	result["bound_enterprise_count"] = len(enterprises)
	if len(enterprises) == 0 {
		return false, result, []checkIssue{{Code: "enterprise_binding_empty", Title: "未查询到绑定企业", Detail: "/share/enterprise-list 返回成功，但 data 为空，当前 apikey 未绑定可用企业。", Suggestion: "请前往以下网址补充信息，或稍后重试：" + defaultBindingURL, Status: "failed"}}
	}

	var matched map[string]any
	if normalizedName != "" {
		for _, item := range enterprises {
			if StringValue(item["enterpriseName"]) == normalizedName || StringValue(item["uscc"]) == normalizedName {
				matched = item
				break
			}
		}
		if matched == nil {
			candidates := buildFuzzyMatchCandidates(normalizedName, enterprises)
			result["fuzzy_candidates"] = candidates
			if len(candidates) > 0 {
				result["requires_confirmation"] = true
				detail := fmt.Sprintf("当前未精确命中企业名称或税号“%s”，但识别到 %d 个可能匹配的企业。\n候选企业如下：\n%s", normalizedName, len(candidates), formatEnterpriseChoiceList(candidates))
				return false, result, []checkIssue{{Code: "enterprise_binding_confirmation_required", Title: "识别到可能匹配的企业，需要确认", Detail: detail, Suggestion: "请从候选列表中选择后重试。", Status: "failed"}}
			}
			return false, result, []checkIssue{{Code: "enterprise_binding_not_matched", Title: "未查询到指定企业", Detail: fmt.Sprintf("当前 apikey 共绑定 %d 家企业，但未找到企业名称或税号为“%s”的企业。", len(enterprises), normalizedName), Suggestion: "请确认输入的企业名称或税号无误，或前往 " + defaultBindingURL + " 控制台修改绑定关系后重试。", Status: "failed"}}
		}
	} else if len(enterprises) == 1 {
		matched = enterprises[0]
	} else {
		return false, result, []checkIssue{{Code: "enterprise_binding_selection_required", Title: "存在多个绑定企业，需先明确企业", Detail: "当前用户 apikey 下绑定了以下企业：\n" + formatEnterpriseChoiceList(enterprises), Suggestion: "请选择一个企业后重试。", Status: "failed"}}
	}
	if matched == nil {
		return false, result, []checkIssue{{Code: "enterprise_binding_selection_required", Title: "未能确定绑定企业", Detail: "当前未能确定要使用的绑定企业。", Suggestion: "请指定企业名称或税号后重试。", Status: "failed"}}
	}
	result["matched"] = true
	result["matched_enterprise"] = matched
	result["selected_enterprise"] = strings.TrimSpace(StringValue(matched["enterpriseName"]))
	for k, v := range matched {
		result[k] = v
	}
	result["config_persisted"] = false
	return true, result, nil
}

func checkLoginStatus(selectedEnterprise, apikey string, config, bindingResult map[string]any, client *APIClient, quickEnabled bool) (bool, map[string]any, []checkIssue) {
	loginType, uscc, personalAccount := resolveLoginStatusContext(bindingResult)
	loginURL := buildLoginURL(apikey, uscc)
	ywlx := loginTypeToYWLX[loginType]
	result := defaultLoginStatusResult(selectedEnterprise, loginType, ywlx, uscc, personalAccount, quickEnabled)
	if loginType == "" || ywlx == "" {
		return false, result, []checkIssue{{Code: "login_status_missing_login_type", Title: "缺少登录类型", Detail: "前置接口未提供 loginType，当前无法确定应使用企业登录还是代理登录进行状态检测。", Suggestion: "请先完成前置登录主体识别，再执行登录状态检测。", Status: "failed"}}
	}
	if uscc == "" || personalAccount == "" {
		return false, result, []checkIssue{{Code: "login_status_missing_context", Title: "缺少登录状态检测参数", Detail: "缺少 uscc 或 personalAccount，当前无法调用 /login/login-status 检测登录状态。", Suggestion: "请先补齐销售方税号和登录账号，再执行登录状态检测。", Status: "failed"}}
	}
	requestContext, contextErr := EnterpriseRequestContextFromSnapshot(bindingResult, requestCapabilityCode(client))
	if contextErr != nil {
		return false, result, []checkIssue{{Code: "login_status_missing_context", Title: "登录企业上下文不完整", Detail: contextErr.Error(), Suggestion: "请重新执行企业绑定预检。", Status: "failed"}}
	}
	client = NewEnterpriseAPIClient(client, config, requestContext)
	requestPayload := map[string]any{"uscc": uscc, "personalAccount": personalAccount, "ywlx": ywlx}
	payload, err := client.PostJSON(loginStatusURL, requestPayload, 15*time.Second)
	if err != nil {
		if textHasCapabilityUnauthorized(err.Error()) {
			return false, result, []checkIssue{capabilityUnauthorizedIssue("登录状态检测", err.Error())}
		}
		code := "login_status_request_error"
		title := "登录状态查询失败"
		if isTimeoutFailure(err.Error()) {
			code = "login_status_query_timeout"
			title = "登录状态查询超时"
		}
		setLoginStatusAssessment(result, "unknown", code, true)
		setLoginStatusCauseDetail(result, "transport_error", err.Error())
		return false, result, []checkIssue{{
			Code:       code,
			Title:      title,
			Detail:     unknownLoginStatusDetail(err.Error()),
			Suggestion: appendManualLoginFallback("请稍后重新执行当前步骤；不要根据本次结果判断登录失效，Agent 不得自动重试。", loginURL),
			Status:     "failed",
		}}
	}
	if responseHasCapabilityUnauthorized(payload) {
		return false, result, []checkIssue{capabilityUnauthorizedIssue("登录状态检测", payload["message"])}
	}
	initialDetail := pickFirstNonEmpty(payload["message"], "未查询到账户信息")
	if codeEquals(payload["code"], 0) {
		ready := applyLoginStatusResponse(payload, result)
		if ready {
			return true, result, nil
		}
	}
	if ywlx == "QYYW" && initialDetail == "未查询到账户信息" {
		setNested(result, "quick_auto_login", "fallback_reason", "enterprise_account_not_found")
		setLoginStatusAssessment(result, "not_ready", "enterprise_account_not_found", false)
		return false, result, []checkIssue{{Code: "login_status_not_ready", Title: "未查询到登录状态", Detail: initialDetail, Suggestion: "请前往以下网址完成登录后重试：" + loginURL, Status: "failed"}}
	}
	if !quickEnabled {
		setNested(result, "quick_auto_login", "fallback_reason", "disabled")
		issue := buildQuickAutoLoginFailureIssue(result, initialDetail, "快速登录开关已关闭。", loginURL)
		return false, result, []checkIssue{issue}
	}
	quickOK, quickDetail := attemptQuickAutoLogin(bindingResult, loginType, uscc, personalAccount, result, client)
	if textHasCapabilityUnauthorized(quickDetail) {
		return false, result, []checkIssue{capabilityUnauthorizedIssue("快速登录", quickDetail)}
	}
	if quickOK {
		setNested(result, "quick_auto_login", "status_rechecked", true)
		retryDetail := "快速登录后仍未查询到有效登录状态。"
		statusKnown := false
		for i, delay := range quickAutoLoginRetryDelays {
			setNested(result, "quick_auto_login", "status_recheck_attempts", i+1)
			time.Sleep(delay)
			retryPayload, err := client.PostJSON(loginStatusURL, requestPayload, 15*time.Second)
			if err != nil {
				retryDetail = err.Error()
				continue
			}
			if responseHasCapabilityUnauthorized(retryPayload) {
				return false, result, []checkIssue{capabilityUnauthorizedIssue("登录状态复检", retryPayload["message"])}
			}
			if codeEquals(retryPayload["code"], 0) {
				ready := applyLoginStatusResponse(retryPayload, result)
				statusKnown = StringValue(result["status_assessment"]) != "unknown"
				if ready {
					return true, result, nil
				}
			}
			retryDetail = pickFirstNonEmpty(retryPayload["message"], retryDetail)
		}
		if !statusKnown {
			code := "login_status_recheck_failed"
			title := "快速登录后的状态复查失败"
			if isTimeoutFailure(retryDetail) {
				code = "login_status_recheck_timeout"
				title = "快速登录后的状态复查超时"
			}
			setLoginStatusAssessment(result, "unknown", code, true)
			setLoginStatusCauseDetail(result, "status_recheck", retryDetail)
			return false, result, []checkIssue{{
				Code:       code,
				Title:      title,
				Detail:     unknownLoginStatusDetail(retryDetail),
				Suggestion: appendManualLoginFallback("请稍后重新执行当前步骤；不要根据本次结果判断登录失效，Agent 不得自动重试。", loginURL),
				Status:     "failed",
			}}
		}
		setLoginStatusAssessment(result, "not_ready", "login_status_not_ready", false)
		return false, result, []checkIssue{{Code: "login_status_not_ready", Title: "未获取到可用登录状态", Detail: retryDetail, Suggestion: "请前往以下网址完成登录后重试：" + loginURL, Status: "failed"}}
	}
	issue := buildQuickAutoLoginFailureIssue(result, initialDetail, quickDetail, loginURL)
	return false, result, []checkIssue{issue}
}

/**
 * requestCapabilityCode 优先沿用调用方已经设置的 capabilityCode，避免公共登录流程覆盖具体 Skill 的权限契约。
 *
 * @param client 当前业务流程传入的 API 客户端。
 * @returns Authorization 中的 capabilityCode；未设置时返回当前运行时默认 capabilityCode。
 */
func requestCapabilityCode(client *APIClient) string {
	if client != nil {
		for _, part := range strings.Split(client.requestHeaders()["Authorization"], ";") {
			keyValue := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(keyValue) == 2 && strings.EqualFold(strings.TrimSpace(keyValue[0]), "capabilityCode") {
				if capabilityCode := strings.TrimSpace(keyValue[1]); capabilityCode != "" {
					return capabilityCode
				}
			}
		}
	}
	return CapabilityCode
}

/**
 * attemptQuickAutoLogin 使用本次企业绑定结果尝试快速登录，不读取工作区企业选择状态。
 *
 * @param bindingResult 本次 enterprise-list 返回并选中的企业记录。
 * @param loginType 企业或代理登录类型。
 * @param uscc 本次业务企业 USCC。
 * @param personalAccount 服务端企业记录中的登录账号。
 * @param statusResult 需要更新的登录状态结果对象。
 * @param client 已绑定本次企业请求上下文的 APIClient。
 * @returns 是否完成快速登录，以及失败时的可诊断原因。
 */
func attemptQuickAutoLogin(bindingResult map[string]any, loginType, uscc, personalAccount string, statusResult map[string]any, client *APIClient) (bool, string) {
	areaCode := pickFirstNonEmpty(bindingResult["areaCode"])
	agencyUSCC := pickFirstNonEmpty(bindingResult["agencyUSCC"])
	quick := map[string]any{"enabled": true, "attempted": false, "eligible": false, "area_code": areaCode, "agency_uscc": agencyUSCC, "other_tax_number_running_task_checked": false, "has_other_tax_number_running_task": nil, "other_tax_number_running_task_message": "", "precheck_type": nil, "precheck_message": "", "auto_login_type": "", "execution_success": nil, "execution_message": "", "status_rechecked": false, "status_recheck_attempts": 0, "status_recheck_delays": []int{2, 4, 6, 8, 10}, "fallback_reason": "", "error": ""}
	statusResult["quick_auto_login"] = quick
	if uscc == "" || personalAccount == "" || areaCode == "" {
		quick["fallback_reason"] = "missing_context"
		return false, "缺少 uscc、areaCode 或 personalAccount，无法尝试快速登录。"
	}
	quick["attempted"] = true
	runningPayload := map[string]any{"uscc": uscc, "areaCode": normalizeJSONScalar(areaCode), "personalAccount": personalAccount}
	precheck, err := client.PostJSON(quickAutoLoginCheckURL, runningPayload, 15*time.Second)
	if err != nil {
		quick["fallback_reason"] = "precheck_request_failed"
		quick["error"] = err.Error()
		return false, err.Error()
	}
	if responseHasCapabilityUnauthorized(precheck) {
		quick["fallback_reason"] = "apikey_capability_unauthorized"
		return false, pickFirstNonEmpty(precheck["message"], capabilityUnauthorizedText)
	}
	precheckData, _ := precheck["data"].(map[string]any)
	precheckType := firstValue(precheckData["type"], precheck["type"])
	precheckMessage := pickFirstNonEmpty(precheckData["message"], precheck["message"])
	autoLoginType := pickFirstNonEmpty(precheckData["autoLoginType"], precheck["autoLoginType"])
	quick["precheck_type"] = precheckType
	quick["precheck_message"] = precheckMessage
	quick["auto_login_type"] = autoLoginType
	eligible := codeEquals(precheck["code"], 0) && StringValue(precheckType) != "0" && autoLoginType != ""
	quick["eligible"] = eligible
	if !eligible {
		quick["fallback_reason"] = "not_eligible"
		return false, pickFirstNonEmpty(precheckMessage, "当前不满足快速登录条件。")
	}

	running, err := client.PostJSON(runningTaskURL, runningPayload, 15*time.Second)
	if err != nil {
		quick["fallback_reason"] = "other_tax_number_running_task_check_failed"
		quick["error"] = err.Error()
		return false, err.Error()
	}
	if responseHasCapabilityUnauthorized(running) {
		quick["fallback_reason"] = "apikey_capability_unauthorized"
		return false, pickFirstNonEmpty(running["message"], capabilityUnauthorizedText)
	}
	quick["other_tax_number_running_task_checked"] = true
	quick["other_tax_number_running_task_message"] = pickFirstNonEmpty(running["message"])
	runningData, _ := running["data"].(map[string]any)
	quick["has_other_tax_number_running_task"] = runningData["hasOtherTaxNumberRunningTask"]
	if !codeEquals(running["code"], 0) {
		quick["fallback_reason"] = "other_tax_number_running_task_check_failed"
		return false, pickFirstNonEmpty(quick["other_tax_number_running_task_message"], "检查其它税号运行中任务失败。")
	}
	if runningData["hasOtherTaxNumberRunningTask"] == true {
		quick["fallback_reason"] = "other_tax_number_running_task_exists"
		return false, "账号下有其它税号正在执行任务，请稍后重试。"
	}
	executePayload := map[string]any{"taskId": 0, "uscc": uscc, "personalAccount": personalAccount, "targetAreaCode": normalizeJSONScalar(areaCode), "autoLoginType": autoLoginType, "timeout": 120}
	if loginType == "1" {
		if agencyUSCC == "" {
			quick["fallback_reason"] = "missing_agency_uscc"
			return false, "当前企业为代理业务，但缺少 agencyUSCC，无法尝试快速登录。"
		}
		executePayload["accountingCompanyUSCC"] = agencyUSCC
	}
	execute, err := client.PostJSON(quickAutoLoginURL, executePayload, 135*time.Second)
	if err != nil {
		quick["fallback_reason"] = "execution_request_failed"
		quick["error"] = err.Error()
		return false, err.Error()
	}
	if responseHasCapabilityUnauthorized(execute) {
		quick["fallback_reason"] = "apikey_capability_unauthorized"
		return false, pickFirstNonEmpty(execute["message"], capabilityUnauthorizedText)
	}
	executeData, _ := execute["data"].(map[string]any)
	quick["execution_success"] = executeData["success"] == true
	quick["execution_message"] = pickFirstNonEmpty(executeData["message"], execute["message"])
	if quick["execution_success"] == true {
		return true, StringValue(quick["execution_message"])
	}
	if strings.Contains(StringValue(quick["execution_message"]), "切换地区过于频繁") {
		quick["fallback_reason"] = "region_switch_rate_limited"
		return false, StringValue(quick["execution_message"])
	}
	quick["fallback_reason"] = "execution_failed"
	return false, pickFirstNonEmpty(quick["execution_message"], "快速登录失败。")
}

func buildQuickAutoLoginFailureIssue(result map[string]any, initialDetail, quickDetail, loginURL string) checkIssue {
	quick, _ := result["quick_auto_login"].(map[string]any)
	fallbackReason := strings.TrimSpace(StringValue(quick["fallback_reason"]))
	detail := strings.TrimSpace(initialDetail)
	if quickDetail != "" {
		if detail != "" {
			detail += "；"
		}
		detail += "快速登录未成功：" + quickDetail
	}

	assessment := strings.TrimSpace(StringValue(result["status_assessment"]))
	if assessment != "not_ready" {
		assessment = "unknown"
	}
	causeCategory := fallbackReason
	retryable := true
	if assessment == "not_ready" {
		causeCategory = "login_status_not_ready"
		retryable = false
	}
	if causeCategory == "" {
		causeCategory = "unclassified_login_check_failure"
	}

	issue := checkIssue{
		Code:       "login_status_query_failed",
		Title:      "暂时无法确认登录状态",
		Detail:     unknownLoginStatusDetail(detail),
		Suggestion: "请稍后重新执行当前步骤；不要根据本次结果判断登录失效，Agent 不得自动重试。",
		Status:     "failed",
	}

	switch fallbackReason {
	case "precheck_request_failed":
		issue.Code = "quick_login_precheck_request_failed"
		issue.Title = "快速登录预检请求失败"
		causeCategory = issue.Code
		if isTimeoutFailure(quickDetail) {
			issue.Code = "quick_login_precheck_timeout"
			issue.Title = "快速登录预检超时"
			causeCategory = issue.Code
		}
		retryable = true
	case "other_tax_number_running_task_check_failed":
		issue.Code = "quick_login_task_check_failed"
		issue.Title = "快速登录前置检查失败"
		causeCategory = issue.Code
		if isTimeoutFailure(quickDetail) {
			issue.Code = "quick_login_task_check_timeout"
			issue.Title = "快速登录前置检查超时"
			causeCategory = issue.Code
		}
		retryable = true
	case "execution_request_failed":
		issue.Code = "quick_login_request_failed"
		issue.Title = "快速登录请求失败"
		causeCategory = issue.Code
		if isTimeoutFailure(quickDetail) {
			issue.Code = "quick_login_timeout"
			issue.Title = "快速登录请求超时"
			causeCategory = issue.Code
		}
		retryable = true
	case "not_eligible":
		issue.Code = "quick_login_not_eligible"
		issue.Title = "当前不满足快速登录条件"
		issue.Suggestion = "当前无法通过快速登录继续处理；请停止本次流程，等待用户决定是否稍后重试。"
		causeCategory = fallbackReason
		retryable = true
	case "region_switch_rate_limited":
		issue.Code = "region_switch_rate_limited"
		issue.Title = "切换地区过于频繁"
		issue.Suggestion = "请停止自动重试和跨地区切换，稍后再试；当前不能据此判断登录失效。"
		causeCategory = fallbackReason
		retryable = true
	case "other_tax_number_running_task_exists":
		issue.Code = "other_tax_number_running_task_exists"
		issue.Title = "其他税号任务正在执行"
		issue.Suggestion = "请等待其他税号任务结束后再试；Agent 不得自动重试。"
		causeCategory = fallbackReason
		retryable = true
	case "missing_context", "missing_agency_uscc":
		issue.Code = "quick_login_context_missing"
		issue.Title = "快速登录所需信息不完整"
		issue.Suggestion = "请先补齐快速登录所需的企业和账号信息；当前不能据此判断登录失效。"
		causeCategory = fallbackReason
		assessment = "unknown"
		retryable = false
	case "disabled":
		issue.Code = "quick_login_disabled"
		issue.Title = "快速登录当前不可用"
		if assessment == "not_ready" {
			issue.Detail = detail
		}
	case "execution_failed":
		issue.Code = "quick_login_execution_failed"
		issue.Title = "快速登录未成功"
		issue.Suggestion = "请原样展示快速登录返回的实际原因并停止；不得自行补充登录失效或自动重试结论。"
		causeCategory = issue.Code
		if assessment == "not_ready" {
			issue.Detail = detail
		}
	default:
		issue.Code = "unclassified_login_check_failure"
		issue.Title = "登录检查未完成（未分类异常）"
		issue.Suggestion = "请原样展示实际返回信息并停止；不得自行归因为登录失效或自动重试。"
		causeCategory = issue.Code
	}

	issue.Suggestion = appendManualLoginFallback(issue.Suggestion, loginURL)
	setLoginStatusAssessment(result, assessment, causeCategory, retryable)
	setLoginStatusCauseDetail(result, fallbackReason, detail)
	return issue
}

func appendManualLoginFallback(suggestion, loginURL string) string {
	suggestion = strings.TrimSpace(suggestion)
	loginURL = strings.TrimSpace(loginURL)
	if loginURL == "" {
		return suggestion
	}
	fallback := "您也可以手动尝试登录（该入口仅作为兜底，不表示已确认登录失效）：" + loginURL
	if suggestion == "" {
		return fallback
	}
	return suggestion + " " + fallback
}

func unknownLoginStatusDetail(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "当前登录状态未知，不能据此判断登录有效或失效。"
	}
	return "当前登录状态未知，不能据此判断登录有效或失效。原因：" + reason
}

func isTimeoutFailure(detail string) bool {
	normalized := strings.ToLower(strings.TrimSpace(detail))
	for _, marker := range []string{"context deadline exceeded", "client.timeout", "timed out", "timeout", "超时"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func setLoginStatusAssessment(result map[string]any, assessment, causeCategory string, retryable bool) {
	result["status_assessment"] = assessment
	result["cause_category"] = causeCategory
	result["retryable"] = retryable
	result["auto_retry_allowed"] = false
}

func setLoginStatusCauseDetail(result map[string]any, rawCause, detail string) {
	result["raw_cause"] = strings.TrimSpace(rawCause)
	result["cause_detail"] = strings.TrimSpace(detail)
}

func defaultLoginStatusResult(selectedEnterprise, loginType, ywlx, uscc, personalAccount string, enabled bool) map[string]any {
	return map[string]any{
		"checked": true, "mode": "api", "enterprise_name": selectedEnterprise, "login_status": nil,
		"status_assessment": "unknown", "cause_category": "not_checked", "retryable": false, "auto_retry_allowed": false,
		"raw_cause": "", "cause_detail": "",
		"login_type": loginType, "ywlx": ywlx, "uscc": uscc, "personal_account": personalAccount,
		"response_message": "", "login_level": "", "task_id": "", "sflx": "",
		"quick_auto_login": map[string]any{"enabled": enabled, "attempted": false, "eligible": false, "area_code": "", "agency_uscc": "", "other_tax_number_running_task_checked": false, "has_other_tax_number_running_task": nil, "other_tax_number_running_task_message": "", "precheck_type": nil, "precheck_message": "", "auto_login_type": "", "execution_success": nil, "execution_message": "", "status_rechecked": false, "status_recheck_attempts": 0, "status_recheck_delays": []int{2, 4, 6, 8, 10}, "fallback_reason": "", "error": ""},
	}
}

func normalizeEnterpriseRecord(record map[string]any) map[string]any {
	normalized := map[string]any{
		"enterpriseName":  pickFirstNonEmpty(record["enterpriseName"]),
		"uscc":            pickFirstNonEmpty(record["uscc"]),
		"areaCode":        pickFirstNonEmpty(record["areaCode"]),
		"agencyUSCC":      pickFirstNonEmpty(record["agencyUSCC"]),
		loginTypeKey:      pickFirstNonEmpty(record[loginTypeKey]),
		"identityType":    pickFirstNonEmpty(record["identityType"]),
		"personalAccount": pickFirstNonEmpty(record["personalAccount"]),
	}
	if nsrzg, ok := record["nsrzg"]; ok {
		normalized["nsrzg"] = nsrzg
	}
	return normalized
}

func applyLoginStatusResponse(payload, statusResult map[string]any) bool {
	statusResult["response_message"] = pickFirstNonEmpty(payload["message"])
	data, _ := payload["data"].(map[string]any)
	loginStatus := data["status"]
	statusResult["login_status"] = loginStatus
	nestedData, _ := data["data"].(map[string]any)
	if loginStatus == nil || strings.TrimSpace(StringValue(loginStatus)) == "" {
		setLoginStatusAssessment(statusResult, "unknown", "login_status_response_incomplete", true)
		return false
	}
	if !codeEquals(loginStatus, 1) {
		setLoginStatusAssessment(statusResult, "not_ready", "login_status_not_ready", false)
		return false
	}
	if nestedData == nil {
		setLoginStatusAssessment(statusResult, "unknown", "login_status_response_incomplete", true)
		return false
	}
	setLoginStatusAssessment(statusResult, "valid", "login_status_ready", false)
	statusResult["login_level"] = pickFirstNonEmpty(nestedData["loginLevel"])
	statusResult["task_id"] = pickFirstNonEmpty(nestedData["taskId"])
	statusResult["sflx"] = pickFirstNonEmpty(nestedData["sflx"])
	return true
}

/**
 * resolveLoginStatusContext 从本次企业绑定记录读取登录状态查询参数。
 *
 * @param binding 当前预检选中的服务端企业记录。
 * @returns loginType、uscc 和 personalAccount。
 */
func resolveLoginStatusContext(binding map[string]any) (string, string, string) {
	return pickFirstNonEmpty(binding[loginTypeKey]),
		pickFirstNonEmpty(binding["uscc"]),
		pickFirstNonEmpty(binding["personalAccount"])
}

/**
 * firstValue 返回第一个有意义的原始值，保留数字和布尔值类型。
 *
 * @param values 按优先级排列的候选值。
 * @returns 第一个非空值；全部为空时返回 nil。
 */
func firstValue(values ...any) any {
	for _, value := range values {
		if HasMeaningfulValue(value) {
			return value
		}
	}
	return nil
}

func buildLoginURL(apikey, uscc string) string {
	return defaultLoginURL + "?nmyk=" + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(apikey))) + "&uscc=" + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(uscc)))
}

func buildFuzzyMatchCandidates(input string, enterprises []map[string]any) []map[string]any {
	normalizedInput := normalizeTextForFuzzyMatch(input)
	if normalizedInput == "" {
		return nil
	}
	candidates := []map[string]any{}
	seen := map[string]bool{}
	for _, item := range enterprises {
		name := pickFirstNonEmpty(item["enterpriseName"])
		normalizedName := normalizeTextForFuzzyMatch(name)
		if normalizedName == "" {
			continue
		}
		matchType := ""
		score := 0.0
		if strings.Contains(normalizedName, normalizedInput) || strings.Contains(normalizedInput, normalizedName) {
			matchType = "contains"
			score = 1
		} else {
			score = similarityRatio(normalizedInput, normalizedName)
			if score >= 0.6 {
				matchType = "similarity"
			}
		}
		if matchType == "" {
			continue
		}
		key := name + "|" + pickFirstNonEmpty(item["uscc"])
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, map[string]any{
			"enterpriseName":  name,
			"uscc":            pickFirstNonEmpty(item["uscc"]),
			"areaCode":        pickFirstNonEmpty(item["areaCode"]),
			loginTypeKey:      pickFirstNonEmpty(item[loginTypeKey]),
			"identityType":    pickFirstNonEmpty(item["identityType"]),
			"personalAccount": pickFirstNonEmpty(item["personalAccount"]),
			"match_type":      matchType,
			"match_score":     math.Round(score*10000) / 10000,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := floatValue(candidates[i]["match_score"])
		right := floatValue(candidates[j]["match_score"])
		if left == right {
			return StringValue(candidates[i]["enterpriseName"]) < StringValue(candidates[j]["enterpriseName"])
		}
		return left > right
	})
	if len(candidates) > 5 {
		return candidates[:5]
	}
	return candidates
}

func normalizeTextForFuzzyMatch(value string) string {
	out := strings.TrimSpace(strings.ToLower(value))
	out = regexp.MustCompile(`\s+`).ReplaceAllString(out, "")
	out = strings.ReplaceAll(out, "（", "(")
	out = strings.ReplaceAll(out, "）", ")")
	return out
}

func similarityRatio(a, b string) float64 {
	if a == "" && b == "" {
		return 1
	}
	dist := levenshtein([]rune(a), []rune(b))
	return 1 - float64(dist)/float64(max(len([]rune(a)), len([]rune(b))))
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr := make([]int, len(b)+1)
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = curr
	}
	return prev[len(b)]
}

func formatEnterpriseChoiceList(records []map[string]any) string {
	lines := make([]string, 0, len(records))
	for _, item := range records {
		lines = append(lines, "- "+formatBoundEnterpriseLabel(item))
	}
	return strings.Join(lines, "\n")
}

func formatBoundEnterpriseLabel(record map[string]any) string {
	name := pickFirstNonEmpty(record["enterpriseName"])
	uscc := pickFirstNonEmpty(record["uscc"])
	if name != "" && uscc != "" {
		return fmt.Sprintf("%s（%s）", name, uscc)
	}
	return pickFirstNonEmpty(name, uscc, "未知企业")
}

func writePreflightProblemMarkdown(issues []checkIssue) (string, error) {
	now := time.Now()
	dir := filepath.Join(DefaultWorkspaceDir(), preflightResultsDirname, now.Format("2006-01-02"), fmt.Sprintf("%s-%d", precheckDirname, now.UnixMilli()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "preflight-problems.md")
	if err := os.WriteFile(path, []byte(buildProblemMarkdown(issues)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func buildProblemMarkdown(issues []checkIssue) string {
	if len(issues) == 0 {
		return "预检通过\n"
	}
	lines := []string{"## 发现的问题", ""}
	for i, issue := range issues {
		lines = append(lines, fmt.Sprintf("### %d. %s", i+1, issue.Title), "")
		if (issue.Code == "enterprise_binding_selection_required" || issue.Code == "enterprise_binding_confirmation_required") && issue.Detail != "" {
			lines = append(lines, issue.Detail, "")
		} else if strings.TrimSpace(issue.Detail) != "" {
			lines = append(lines, "- 详情："+issue.Detail, "")
		}
		lines = append(lines, "- 建议："+issue.Suggestion, "")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func ensureWritableDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	probe := filepath.Join(path, ".write_probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return err
	}
	return os.Remove(probe)
}

func pickFirstNonEmpty(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(StringValue(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func codeEquals(value any, expected int) bool {
	switch typed := value.(type) {
	case int:
		return typed == expected
	case int64:
		return int(typed) == expected
	case float64:
		return int(typed) == expected
	case string:
		return strings.TrimSpace(typed) == fmt.Sprint(expected)
	default:
		return false
	}
}

func normalizeJSONScalar(value string) any {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	var n int
	if _, err := fmt.Sscanf(text, "%d", &n); err == nil && fmt.Sprint(n) == text {
		return n
	}
	return text
}

func setNested(root map[string]any, key, nestedKey string, value any) {
	nested, _ := root[key].(map[string]any)
	if nested == nil {
		nested = map[string]any{}
		root[key] = nested
	}
	nested[nestedKey] = value
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func max(values ...int) int {
	out := values[0]
	for _, value := range values[1:] {
		if value > out {
			out = value
		}
	}
	return out
}

func min(values ...int) int {
	out := values[0]
	for _, value := range values[1:] {
		if value < out {
			out = value
		}
	}
	return out
}

func marshalPreflightForTest(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
