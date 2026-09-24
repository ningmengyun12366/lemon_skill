package issue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	issueRequestURL             = "/invoice-issue/issue"
	issueStatusURL              = "/invoice-issue/issue-status"
	issueStatusMaxAttempts      = 10
	issueStatusInitialDelay     = 2 * time.Second
	issueResultRepeatGuardError = "获取发票开具结果异常，为防止重复开票，请前往税局确认"
	submissionGuardKey          = "submission_guard"
)

func IssueFromOutput(taskFile, outputMarkdownPath string, confirmPreview, pretty bool) error {
	return IssueFromOutputWithOptions(IssueFromOutputOptions{
		TaskFile:           taskFile,
		OutputMarkdownPath: outputMarkdownPath,
		ConfirmPreview:     confirmPreview,
		Pretty:             pretty,
	})
}

type IssueFromOutputOptions struct {
	TaskFile           string
	OutputMarkdownPath string
	ConfirmPreview     bool
	ConfirmDuplicate   bool
	Pretty             bool
	Execution          *IssueExecution
}

/**
 * IssueFromOutputWithOptions 仅使用开票任务快照完成登录准备、正式提交和状态查询。
 *
 * @param options 任务路径、预览确认、重复确认和可注入执行依赖。
 * @returns 开票完成时返回 nil，否则返回本地契约、登录或远端业务错误。
 */
func IssueFromOutputWithOptions(options IssueFromOutputOptions) (retErr error) {
	taskPath, err := resolveIssueTaskPath(options.TaskFile)
	if err != nil {
		return err
	}
	lockTimeout := 30 * time.Second
	if options.Execution != nil && options.Execution.LockTimeout > 0 {
		lockTimeout = options.Execution.LockTimeout
	}
	lockContext, cancelLock := context.WithTimeout(context.Background(), lockTimeout)
	defer cancelLock()
	lock, err := acquireTaskFileLock(lockContext, taskPath)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, lock.release())
	}()
	state, err := LoadTaskState(taskPath)
	if err != nil {
		return err
	}
	if _, err := common.EnterpriseContextFromTask(state.Data); err != nil {
		return err
	}
	outputMarkdownPath := options.OutputMarkdownPath
	payload, err := prepareIssuePayload(state.Data, options.ConfirmPreview)
	if err != nil {
		var validationErr *issuePayloadValidationError
		if errors.As(err, &validationErr) {
			state.Data["issue_result"] = map[string]any{}
			state.Data["task_status"] = "check_failed"
			if saveErr := state.Save(options.Pretty); saveErr != nil {
				return saveErr
			}
			return err
		}
		return saveIssueWorkflowFailure(state, err, options.Pretty)
	}
	config := common.LoadWorkspaceConfig()
	baseClient := newAPIClient()
	if options.Execution != nil {
		if options.Execution.WorkspaceConfig != nil {
			config = options.Execution.WorkspaceConfig
		}
		if options.Execution.BaseClient != nil {
			baseClient = options.Execution.BaseClient
		}
	}
	taskContext, err := resolveIssueTaskContext(state.Data, payload)
	if err != nil {
		return err
	}
	client := common.NewEnterpriseAPIClient(baseClient, config, taskContext.Request)
	if err := client.ValidateJSONRequest(payload); err != nil {
		return err
	}
	recordedID, hasRecordedID := getRecordedIssueTaskIDValue(state.Data)
	if !hasRecordedID {
		if issueSubmissionStartedWithoutTaskID(state.Data) {
			return saveIssueWorkflowFailure(state, fmt.Errorf(issueResultRepeatGuardError), options.Pretty)
		}
		if err := requireNoDuplicateIssueSubmission(state, payload, options.ConfirmDuplicate); err != nil {
			return saveIssueWorkflowFailure(state, err, options.Pretty)
		}
	}
	var issueResp map[string]any
	var confirmedStatusResp map[string]any
	if hasRecordedID && issueAuthRecoverySubmissionUncertain(state.Data, recordedID) {
		return saveIssueAuthRecoveryFailure(state, fmt.Errorf(issueAuthRecoveryUncertainMessage), options.Pretty)
	}
	if hasRecordedID && shouldAttemptIssueAuthRecovery(state.Data, recordedID) {
		confirmedStatusResp, err = submitIssueStatusRequest(client, buildIssueStatusPayloadFromTaskID(recordedID, payload))
		if err != nil {
			return saveIssueAuthRecoveryFailure(state, fmt.Errorf("确认权限认证失败任务状态：%w", err), options.Pretty)
		}
		if isIssueAuthRequiredResponse(confirmedStatusResp) {
			loginRequest := common.LoginReadinessRequest{Enterprise: taskContext.Enterprise, Config: config, Client: client}
			if _, err := prepareIssueLoginReadiness(loginRequest); err != nil {
				return saveIssueAuthRecoveryFailure(state, err, options.Pretty)
			}
			if err := markIssueAuthRecoveryStarted(state.Data, recordedID); err != nil {
				return saveIssueAuthRecoveryFailure(state, err, options.Pretty)
			}
			state.Data["task_status"] = "issue_auth_recovery_submitting"
			if err := state.Save(options.Pretty); err != nil {
				return err
			}
			issueResp, err = submitInvoiceIssueRequest(client, payload)
			if err != nil {
				return saveIssueWorkflowFailure(state, err, options.Pretty)
			}
			currentIssueResult := common.ObjectValue(state.Data, "issue_result")
			currentIssueResult["issue_response"] = issueResp
			currentIssueResult["request_success"] = responseCode(issueResp) == 0
			if responseCode(issueResp) != 0 {
				return saveIssueRequestFailure(state, issueResp, options.Pretty)
			}
			issueID, idErr := extractIssueTaskIDStrict(issueResp)
			if idErr != nil {
				return saveIssueWorkflowFailure(state, idErr, options.Pretty)
			}
			recordIssueTaskID(state.Data, issueID)
			recordIssueAuthRecoveryTaskID(state.Data, issueID)
			state.Data["task_status"] = "issue_processing"
			if err := state.Save(options.Pretty); err != nil {
				return err
			}
			confirmedStatusResp = nil
		}
	}
	if !hasRecordedID {
		loginRequest := common.LoginReadinessRequest{Enterprise: taskContext.Enterprise, Config: config, Client: client}
		if _, err := prepareIssueLoginReadiness(loginRequest); err != nil {
			return saveIssueWorkflowFailure(state, err, options.Pretty)
		}
		markIssueSubmissionStarted(state.Data)
		if err := state.Save(options.Pretty); err != nil {
			return err
		}
	}
	if issueResp != nil {
		// The authorization recovery branch has already submitted a new remote task.
	} else if hasRecordedID {
		issueResp = buildIssueRequestStub(recordedID)
	} else {
		state.Data["task_status"] = "issue_processing"
		issueResp, err = submitInvoiceIssueRequest(client, payload)
		if err != nil {
			return saveIssueWorkflowFailure(state, err, options.Pretty)
		}
		currentIssueResult := common.ObjectValue(state.Data, "issue_result")
		currentIssueResult["issue_response"] = issueResp
		currentIssueResult["request_success"] = responseCode(issueResp) == 0
		if responseCode(issueResp) == 0 {
			issueID, idErr := extractIssueTaskIDStrict(issueResp)
			if idErr != nil {
				return saveIssueWorkflowFailure(state, idErr, options.Pretty)
			}
			recordIssueTaskID(state.Data, issueID)
		}
		if err := state.Save(options.Pretty); err != nil {
			return err
		}
		if responseCode(issueResp) != 0 {
			return saveIssueRequestFailure(state, issueResp, options.Pretty)
		}
	}
	var statusResp map[string]any
	var issueStatusResult map[string]any
	var pollErr error
	if confirmedStatusResp != nil && isIssueStatusPollingTerminal(confirmedStatusResp) {
		statusResp = confirmedStatusResp
		issueStatusResult = issueStatusResultFromConfirmedResponse(confirmedStatusResp)
	} else {
		statusResp, issueStatusResult, pollErr = pollIssueStatusWithResult(client, issueResp, payload)
	}
	if pollErr != nil && statusResp == nil {
		return saveIssueWorkflowFailure(state, pollErr, options.Pretty)
	}
	processing := isIssueStatusStillProcessing(statusResp, pollErr)
	completed := legacyPythonTruthy(issueStatusResult["completed"])
	downloads := []any{}
	markdown := ""
	if completed {
		downloads = downloadIssueFiles(statusResp, state.Data)
		markdown = BuildIssueResultMarkdownEquivalent(statusResp)
		if outputMarkdownPath == "" {
			outputMarkdownPath = resolveIssueResultMarkdownOutputPath(state)
		}
		if outputMarkdownPath != "" {
			if err := writeTextFile(outputMarkdownPath, markdown); err == nil {
				statusResp["issue_result_markdown_path"] = outputMarkdownPath
			}
			common.AppendMessageLog("开票结果已保存到 ", outputMarkdownPath, "读取md文件内容，并原样渲染回对话中\n")
		}
		statusResp["downloaded_files"] = downloads
		issueStatusResult["local_file_downloads"] = downloads
		if markdown != "" {
			common.AppendMessageLog(markdown + "\n")
		}
	}
	success, partial := summarizeFinalIssueOutcome(statusResp)
	issueCompleted := success || partial
	issueFailed := !issueCompleted && !processing
	result := map[string]any{
		"request_success":                 true,
		"success":                         success,
		"partial_success":                 partial,
		"issue_completed":                 issueCompleted,
		"issue_failed":                    issueFailed,
		"issue_status":                    common.StringValue(firstNonNil(statusResp["status"], GetPath(statusResp, "data.status"))),
		"source_task_path":                state.Path,
		"issue_response":                  issueResp,
		"issue_status_result":             issueStatusResult,
		"issue_status_response":           sanitizeFinalResponseForStorage(statusResp),
		"issue_result_markdown_generated": markdown != "",
		"issue_result_markdown":           markdown,
		"issue_result_markdown_path":      common.StringValue(statusResp["issue_result_markdown_path"]),
		"downloaded_files":                downloads,
	}
	state.Data["issue_result"] = mergeIssueResultWithGuard(state.Data, result)
	if issueCompleted {
		state.Data["task_status"] = "issued"
	} else if processing {
		state.Data["task_status"] = "issue_processing"
	} else {
		state.Data["task_status"] = "issue_failed"
	}
	return state.Save(options.Pretty)
}

func saveIssueWorkflowFailure(state *TaskState, cause error, pretty bool) error {
	message := strings.TrimSpace(cause.Error())
	result := map[string]any{
		"request_success":                 false,
		"issue_completed":                 false,
		"issue_failed":                    true,
		"error_message":                   message,
		"source_task_path":                state.Path,
		"issue_status_result":             map[string]any{},
		"issue_result_markdown_generated": false,
		"issue_result_markdown":           "",
		"issue_result_markdown_path":      "",
	}
	state.Data["issue_result"] = mergeIssueResultWithGuard(state.Data, result)
	state.Data["task_status"] = "issue_failed"
	if err := state.Save(pretty); err != nil {
		return err
	}
	return cause
}

func isIssueStatusStillProcessing(statusResp map[string]any, pollErr error) bool {
	if pollErr == nil || statusResp == nil {
		return false
	}
	if isIssueStatusSuccess(statusResp) || isBatchIssueStatusCompleted(statusResp) || isIssueStatusTerminalFailure(statusResp) {
		return false
	}
	return true
}

func saveIssueRequestFailure(state *TaskState, issueResp map[string]any, pretty bool) error {
	message := strings.TrimSpace(common.StringValue(issueResp["message"]))
	if message == "" {
		message = "接口返回业务失败"
	}
	result := map[string]any{
		"request_success":                 false,
		"issue_completed":                 false,
		"issue_failed":                    true,
		"issue_status":                    common.StringValue(GetPath(issueResp, "data.status")),
		"source_task_path":                state.Path,
		"issue_response":                  issueResp,
		"issue_status_result":             map[string]any{},
		"issue_result_markdown_generated": false,
		"issue_result_markdown":           "",
		"issue_result_markdown_path":      "",
		"error_message":                   message,
	}
	state.Data["issue_result"] = mergeIssueResultWithGuard(state.Data, result)
	state.Data["task_status"] = "issue_failed"
	if err := state.Save(pretty); err != nil {
		return err
	}
	return fmt.Errorf("提交开票请求失败：%s", message)
}
