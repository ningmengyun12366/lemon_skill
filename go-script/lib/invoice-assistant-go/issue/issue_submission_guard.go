package issue

import "invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"

func issueSubmissionStartedWithoutTaskID(taskState map[string]any) bool {
	guard := getSubmissionGuard(taskState)
	started := legacyPythonTruthy(guard["issue_api_called"])
	_, hasTaskID := getRecordedIssueTaskIDValue(taskState)
	return started && !hasTaskID
}

func mergeIssueResultWithGuard(taskState, result map[string]any) map[string]any {
	if existing, ok := taskState["issue_result"].(map[string]any); ok {
		for k, v := range existing {
			if (k == submissionGuardKey || k == duplicateGuardKey || k == issueAuthRecoveryKey) && result[k] == nil {
				result[k] = v
			}
		}
	}
	result[submissionGuardKey] = getSubmissionGuard(taskState)
	return result
}

func getSubmissionGuard(taskState map[string]any) map[string]any {
	issueResult := common.ObjectValue(taskState, "issue_result")
	return common.ObjectValue(issueResult, submissionGuardKey)
}

func markIssueSubmissionStarted(taskState map[string]any) {
	guard := getSubmissionGuard(taskState)
	guard["issue_api_called"] = true
	if _, ok := guard["issue_task_id"]; !ok {
		guard["issue_task_id"] = nil
	}
}

func recordIssueTaskID(taskState map[string]any, issueTaskID int) {
	guard := getSubmissionGuard(taskState)
	guard["issue_api_called"] = true
	guard["issue_task_id"] = issueTaskID
}

func getRecordedIssueTaskID(taskState map[string]any) int {
	taskID, _ := getRecordedIssueTaskIDValue(taskState)
	return taskID
}

func getRecordedIssueTaskIDValue(taskState map[string]any) (int, bool) {
	guard := getSubmissionGuard(taskState)
	if !legacyPythonTruthy(guard["issue_api_called"]) {
		return 0, false
	}
	taskID, found, err := lookupIssueTaskID(map[string]any{"taskID": guard["issue_task_id"]})
	if err != nil {
		return 0, false
	}
	return taskID, found
}

func buildIssueRequestStub(taskID int) map[string]any {
	return map[string]any{"code": 0, "data": map[string]any{"taskID": taskID}}
}
