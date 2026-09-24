package issue

import (
	"fmt"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	issueAuthRequiredCode             = 2015
	issueAuthRecoveryKey              = "auth_recovery"
	issueAuthRecoveryUncertainMessage = "开票权限认证恢复提交结果不确定，为防止重复开票，请前往税局或后台确认"
)

func shouldAttemptIssueAuthRecovery(taskState map[string]any, activeTaskID int) bool {
	issueResult, _ := taskState["issue_result"].(map[string]any)
	if issueResult == nil ||
		legacyPythonTruthy(issueResult["issue_completed"]) ||
		legacyPythonTruthy(issueResult["partial_success"]) {
		return false
	}
	statusResp, _ := issueResult["issue_status_response"].(map[string]any)
	if !isIssueAuthRequiredResponse(statusResp) ||
		!issueAuthPassed(taskState) ||
		hasIssuedInvoiceArtifacts(issueResult, statusResp) {
		return false
	}
	recovery, _ := issueResult[issueAuthRecoveryKey].(map[string]any)
	if recovery == nil || !legacyPythonTruthy(recovery["resubmit_started"]) {
		return true
	}
	newTaskID, found, _ := lookupIssueTaskID(map[string]any{"taskID": recovery["new_issue_task_id"]})
	return !found || newTaskID != activeTaskID
}

func issueAuthRecoverySubmissionUncertain(taskState map[string]any, activeTaskID int) bool {
	issueResult, _ := taskState["issue_result"].(map[string]any)
	recovery, _ := issueResult[issueAuthRecoveryKey].(map[string]any)
	if recovery == nil || !legacyPythonTruthy(recovery["resubmit_started"]) {
		return false
	}
	newTaskID, found, _ := lookupIssueTaskID(map[string]any{"taskID": recovery["new_issue_task_id"]})
	if found {
		return newTaskID != activeTaskID
	}
	return issueAuthRecoveryHasHistoricalTaskID(recovery, activeTaskID)
}

func issueAuthPassed(taskState map[string]any) bool {
	authResult, _ := taskState["issue_auth_result"].(map[string]any)
	status := strings.TrimSpace(common.StringValue(firstNonNil(authResult["verify_status"], authResult["status"])))
	return status == "auth_passed"
}

func isIssueAuthRequiredResponse(resp map[string]any) bool {
	return resp != nil && responseCode(resp) == issueAuthRequiredCode && issueDataStatusEquals(resp, -1)
}

func markIssueAuthRecoveryStarted(taskState map[string]any, previousTaskID int) error {
	issueResult := common.ObjectValue(taskState, "issue_result")
	recovery := common.ObjectValue(issueResult, issueAuthRecoveryKey)
	if legacyPythonTruthy(recovery["resubmit_started"]) {
		return fmt.Errorf("%s", issueAuthRecoveryUncertainMessage)
	}
	history := []any{}
	if existing, ok := asList(recovery["previous_issue_task_ids"]); ok {
		history = append(history, existing...)
	}
	if !issueAuthRecoveryHistoryContains(history, previousTaskID) {
		history = append(history, previousTaskID)
	}
	recovery["previous_issue_task_ids"] = history
	recovery["source_status_code"] = issueAuthRequiredCode
	recovery["resubmit_started"] = true
	recovery["attempt_count"] = 1
	recovery["started_at"] = common.NowISO()
	delete(recovery, "new_issue_task_id")
	return nil
}

func recordIssueAuthRecoveryTaskID(taskState map[string]any, newTaskID int) {
	issueResult := common.ObjectValue(taskState, "issue_result")
	recovery := common.ObjectValue(issueResult, issueAuthRecoveryKey)
	recovery["new_issue_task_id"] = newTaskID
	recovery["resubmitted_at"] = common.NowISO()
}

func issueAuthRecoveryHasHistoricalTaskID(recovery map[string]any, taskID int) bool {
	history, ok := asList(recovery["previous_issue_task_ids"])
	return ok && issueAuthRecoveryHistoryContains(history, taskID)
}

func issueAuthRecoveryHistoryContains(history []any, taskID int) bool {
	for _, item := range history {
		candidate, found, _ := lookupIssueTaskID(map[string]any{"taskID": item})
		if found && candidate == taskID {
			return true
		}
	}
	return false
}

func hasIssuedInvoiceArtifacts(issueResult, statusResp map[string]any) bool {
	if downloads, ok := asList(issueResult["downloaded_files"]); ok && len(downloads) > 0 {
		return true
	}
	if statusResult, ok := issueResult["issue_status_result"].(map[string]any); ok {
		if downloads, ok := asList(statusResult["local_file_downloads"]); ok && len(downloads) > 0 {
			return true
		}
	}
	for _, item := range extractStatusItems(statusResp) {
		if isIssueStatusItemSuccess(item) || issueStatusItemHasArtifact(item) {
			return true
		}
	}
	return false
}

func issueStatusItemHasArtifact(item map[string]any) bool {
	for _, key := range []string{"file", "local_file"} {
		container, _ := item[key].(map[string]any)
		for _, kind := range []string{"pdf", "xml", "ofd"} {
			if strings.TrimSpace(common.StringValue(container[kind])) != "" {
				return true
			}
		}
	}
	data, _ := item["data"].(map[string]any)
	for _, key := range []string{"sdphm", "invoiceNumber", "invoice_number"} {
		if strings.TrimSpace(common.StringValue(firstNonNil(item[key], data[key]))) != "" {
			return true
		}
	}
	return false
}

func issueStatusResultFromConfirmedResponse(resp map[string]any) map[string]any {
	attempt := map[string]any{
		"attempt":       1,
		"delay_seconds": 0,
		"status":        GetPath(resp, "data.status"),
		"response":      resp,
	}
	result := map[string]any{
		"completed":      true,
		"success":        isIssueStatusSuccess(resp) || isBatchIssueStatusCompleted(resp),
		"attempt_count":  1,
		"attempts":       []any{attempt},
		"final_response": sanitizeFinalResponseForStorage(resp),
	}
	if isIssueStatusTerminalFailure(resp) {
		result["failure_message"] = extractIssueFailureMessage(resp)
	}
	return result
}

func saveIssueAuthRecoveryFailure(state *TaskState, cause error, pretty bool) error {
	issueResult := common.ObjectValue(state.Data, "issue_result")
	issueResult["issue_completed"] = false
	issueResult["issue_failed"] = true
	issueResult["error_message"] = strings.TrimSpace(cause.Error())
	state.Data["task_status"] = "issue_failed"
	if err := state.Save(pretty); err != nil {
		return err
	}
	return cause
}
