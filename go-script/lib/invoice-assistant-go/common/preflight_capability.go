package common

import "strings"

const capabilityUnauthorizedText = "无权调用该能力"

const capabilityUnauthorizedSuggestion = "当前 API Key 未绑定本 skill 所需能力权限或企业。请按本 skill 同级 README.md 中的 API Key 获取方式，为当前 API Key 绑定对应能力权限和企业后重试；不要引导用户登录或重新登录。"

func responseHasCapabilityUnauthorized(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	return textHasCapabilityUnauthorized(payload["message"]) ||
		textHasCapabilityUnauthorized(payload["error"]) ||
		textHasCapabilityUnauthorized(payload["detail"])
}

func textHasCapabilityUnauthorized(value any) bool {
	return strings.Contains(strings.TrimSpace(StringValue(value)), capabilityUnauthorizedText)
}

func capabilityUnauthorizedIssue(action string, detailValue any) checkIssue {
	detail := strings.TrimSpace(StringValue(detailValue))
	if detail == "" {
		detail = capabilityUnauthorizedText
	}
	if action != "" {
		detail = action + "返回：" + detail
	}
	return checkIssue{
		Code:       "apikey_capability_unauthorized",
		Title:      "API Key 未绑定当前能力权限",
		Detail:     detail,
		Suggestion: capabilityUnauthorizedSuggestion,
		Status:     "failed",
	}
}
