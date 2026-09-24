package issue

import (
	"strconv"
	"strings"
)

func summarizeFinalIssueOutcome(resp map[string]any) (bool, bool) {
	items := extractStatusItems(resp)
	if len(items) == 0 {
		return isIssueStatusSuccess(resp), false
	}
	if len(items) > 1 {
		successCount := 0
		for _, item := range items {
			if isLegacyBatchIssueItemSuccessful(item) {
				successCount++
			}
		}
		return successCount == len(items), successCount > 0 && successCount < len(items)
	}
	successCount := 0
	for _, item := range items {
		if isIssueStatusItemSuccess(item) {
			successCount++
		}
	}
	return successCount == len(items), successCount > 0 && successCount < len(items)
}

func isLegacyBatchIssueItemSuccessful(item map[string]any) bool {
	result, ok := item["result"].(bool)
	return !ok || result
}

func sanitizeFinalResponseForStorage(resp map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range resp {
		out[k] = v
	}
	responseData, ok := out["data"].(map[string]any)
	if ok {
		normalizedData := map[string]any{}
		for k, v := range responseData {
			if k != "data" {
				normalizedData[k] = v
			}
		}
		out["data"] = normalizedData
	}
	return out
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		out, _ := strconv.Atoi(strings.TrimSpace(typed))
		return out
	default:
		return 0
	}
}
