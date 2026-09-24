package runtimeio

import "regexp"

const redactedValue = "[REDACTED]"

var (
	secretFlagPattern  = regexp.MustCompile(`(?i)(--(?:api-key|password|authorization|token)(?:=|\s+))("[^"]*"|'[^']*'|\S+)`)
	secretValuePattern = regexp.MustCompile(`(?i)\b(api[_-]?key|password|authorization|token)\b(\s*[:=]\s*)([^\r\n,;]+)`)
)

func Redact(text string) string {
	redacted := secretFlagPattern.ReplaceAllString(text, `${1}`+redactedValue)
	redacted = secretValuePattern.ReplaceAllString(redacted, `${1}${2}`+redactedValue)
	return redacted
}
