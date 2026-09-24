package issue

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func pollIssueStatus(issueResponse, payload map[string]any) (map[string]any, error) {
	resp, _, err := pollIssueStatusWithResult(newAPIClient(), issueResponse, payload)
	return resp, err
}

func pollIssueStatusWithResult(client *common.APIClient, issueResponse, payload map[string]any) (map[string]any, map[string]any, error) {
	statusPayload, err := buildIssueStatusPayload(issueResponse, payload)
	if err != nil {
		return nil, map[string]any{}, err
	}
	var last map[string]any
	attempts := []any{}
	for attempt := 1; attempt <= issueStatusMaxAttempts; attempt++ {
		delay := calculateBackoffDelay(attempt)
		sleepBeforeIssueStatusPoll(delay)
		resp, err := submitIssueStatusRequest(client, statusPayload)
		if err != nil {
			return nil, map[string]any{}, err
		}
		last = resp
		attempts = append(attempts, map[string]any{
			"attempt":       attempt,
			"delay_seconds": int(delay / time.Second),
			"status":        GetPath(resp, "data.status"),
			"response":      resp,
		})
		if isIssueStatusPollingTerminal(resp) {
			result := map[string]any{
				"completed":      true,
				"success":        isIssueStatusSuccess(resp) || isBatchIssueStatusCompleted(resp),
				"attempt_count":  attempt,
				"attempts":       attempts,
				"final_response": sanitizeFinalResponseForStorage(resp),
			}
			if isIssueStatusTerminalFailure(resp) {
				result["failure_message"] = extractIssueFailureMessage(resp)
			}
			return resp, result, nil
		}
	}
	if last == nil {
		last = map[string]any{"code": -1, "message": "未获取到开票状态"}
	}
	result := map[string]any{
		"completed":       false,
		"success":         false,
		"attempt_count":   len(attempts),
		"attempts":        attempts,
		"final_response":  sanitizeFinalResponseForStorage(last),
		"failure_message": "开票状态轮询超过最大次数，仍未得到明确结果",
	}
	return last, result, fmt.Errorf("开票状态轮询超时")
}

func submitIssueStatus(client *common.APIClient, payload map[string]any) (map[string]any, error) {
	resp, err := client.PostJSON(issueStatusURL, payload, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("查询开票状态失败：%w", err)
	}
	return resp, nil
}

func buildIssueStatusPayload(issueResponse, payload map[string]any) (map[string]any, error) {
	taskID, err := extractIssueTaskIDStrict(issueResponse)
	if err != nil {
		return nil, err
	}
	uscc := extractIssueUSCC(payload)
	if uscc == "" {
		return nil, fmt.Errorf("开票请求缺少 uscc，无法查询开票状态")
	}
	return buildIssueStatusPayloadFromTaskID(taskID, payload), nil
}

func buildIssueStatusPayloadFromTaskID(taskID int, payload map[string]any) map[string]any {
	return map[string]any{"taskID": taskID, "uscc": extractIssueUSCC(payload), "isAuth": true}
}

func extractIssueTaskID(issueResponse map[string]any) int {
	taskID, err := extractIssueTaskIDStrict(issueResponse)
	if err != nil {
		return 0
	}
	return taskID
}

func extractIssueTaskIDStrict(issueResponse map[string]any) (int, error) {
	data, hasData := issueResponse["data"].(map[string]any)
	if !hasData {
		return 0, fmt.Errorf("开票接口响应缺少 data，无法查询开票状态")
	}
	if value, exists := data["taskID"]; exists && value != nil {
		return parseIssueTaskID(value)
	}
	if value, exists := data["taskId"]; exists && value != nil {
		return parseIssueTaskID(value)
	}
	return 0, fmt.Errorf("开票接口响应缺少 taskID，无法查询开票状态")
}

func lookupIssueTaskID(container map[string]any) (int, bool, error) {
	for _, key := range []string{"taskID", "taskId", "task_id", "id"} {
		value, exists := container[key]
		if !exists || value == nil {
			continue
		}
		taskID, err := parseIssueTaskID(value)
		if err != nil {
			return 0, true, err
		}
		return taskID, true, nil
	}
	return 0, false, nil
}

func parseIssueTaskID(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	case string:
		text := strings.TrimSpace(typed)
		taskID, err := strconv.Atoi(text)
		if err != nil {
			return 0, fmt.Errorf("开票接口响应中的 taskID 非法：%v", value)
		}
		return taskID, nil
	default:
		return 0, fmt.Errorf("开票接口响应中的 taskID 非法：%v", value)
	}
}

func extractIssueUSCC(payload map[string]any) string {
	if uscc := strings.TrimSpace(common.StringValue(payload["uscc"])); uscc != "" {
		return uscc
	}
	return strings.TrimSpace(common.StringValue(GetPath(payload, "info.0.sellerInfo.uscc")))
}

func calculateBackoffDelay(attempt int) time.Duration {
	return time.Duration(1<<(attempt-1)) * issueStatusInitialDelay
}

func isIssueStatusSuccess(resp map[string]any) bool {
	if responseCode(resp) != 0 {
		return false
	}
	if !issueDataStatusEquals(resp, 1) {
		return false
	}
	item := extractSingleStatusItem(resp)
	if item == nil {
		return false
	}
	return isIssueStatusItemSuccess(item)
}

func isIssueStatusPollingTerminal(resp map[string]any) bool {
	return isIssueStatusSuccess(resp) || isBatchIssueStatusCompleted(resp) || isIssueStatusTerminalFailure(resp)
}

func isBatchIssueStatusCompleted(resp map[string]any) bool {
	if responseCode(resp) != 0 || !issueDataStatusEquals(resp, 1) {
		return false
	}
	return len(extractStatusItems(resp)) > 1
}

func isIssueStatusItemSuccess(item map[string]any) bool {
	if result, ok := item["result"].(bool); ok && !result {
		return false
	}
	rawResultCode := item["resultCode"]
	if !HasMeaningfulValue(rawResultCode) {
		return true
	}
	return intValue(rawResultCode) == 1
}

func isIssueStatusItemFailure(item map[string]any) bool {
	if result, ok := item["result"].(bool); ok && !result {
		return true
	}
	if intValue(firstNonNil(item["resultCode"], item["code"])) == -1 {
		return true
	}
	status := strings.TrimSpace(common.StringValue(firstNonNil(item["status"], item["issueStatus"], item["kpzt"])))
	return status == "failed" || status == "FAILED" || status == "-1"
}

func isIssueStatusTerminalFailure(resp map[string]any) bool {
	return issueDataStatusEquals(resp, -1)
}

func issueDataStatusEquals(resp map[string]any, want int) bool {
	switch status := GetPath(resp, "data.status").(type) {
	case int:
		return status == want
	case int64:
		return status == int64(want)
	case float64:
		return status == float64(want)
	default:
		return false
	}
}

func extractSingleStatusItem(resp map[string]any) map[string]any {
	items := extractStatusItems(resp)
	if len(items) == 1 {
		return items[0]
	}
	return nil
}

func extractStatusItems(resp map[string]any) []map[string]any {
	resultList := GetPath(resp, "data.list")
	list, ok := asList(resultList)
	if !ok {
		return []map[string]any{}
	}
	out := []map[string]any{}
	for _, item := range list {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func extractIssueFailureMessage(resp map[string]any) string {
	item := extractSingleStatusItem(resp)
	if item != nil {
		if msg := strings.TrimSpace(common.StringValue(item["reason"])); msg != "" {
			return msg
		}
		if nested, ok := item["data"].(map[string]any); ok {
			if msg := strings.TrimSpace(common.StringValue(firstNonNil(nested["message"], nested["msg"]))); msg != "" {
				return msg
			}
		}
	}
	data, _ := resp["data"].(map[string]any)
	if msg := strings.TrimSpace(common.StringValue(firstNonNil(data["message"], data["msg"]))); msg != "" {
		return msg
	}
	return strings.TrimSpace(common.StringValue(firstNonNil(resp["message"], "开票状态查询失败")))
}
