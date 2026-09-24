package verifyapi

import (
	"strings"
	"time"
)

func NormalizeIssueDate(value string) (string, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", false
	}
	for _, layout := range []string{
		"2006-01-02",
		"20060102",
		"2006/01/02",
		"2006.01.02",
		"2006年01月02日",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006.01.02 15:04:05",
		time.RFC3339,
	} {
		parsed, err := time.ParseInLocation(layout, text, time.Local)
		if err == nil {
			return parsed.Format("2006-01-02"), true
		}
	}
	return "", false
}

func NormalizeIssueDateOrTrim(value string) string {
	normalized, ok := NormalizeIssueDate(value)
	if ok {
		return normalized
	}
	return strings.TrimSpace(value)
}
