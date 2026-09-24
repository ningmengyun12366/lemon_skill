package common

import (
	"path/filepath"
	"strings"
)

func FormatLocalMarkdownLink(pathValue string) string {
	abs := FormatLocalPathText(pathValue)
	if abs == "" {
		return ""
	}
	return "[" + escapeMarkdownLinkLabel(abs) + "](" + escapeMarkdownLinkDestination(abs) + ")"
}

func FormatLocalPathText(pathValue string) string {
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" {
		return ""
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		abs = trimmed
	}
	abs = filepath.ToSlash(abs)
	return abs
}

func escapeMarkdownLinkLabel(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`[`, `\[`,
		`]`, `\]`,
	)
	return replacer.Replace(value)
}

func escapeMarkdownLinkDestination(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`(`, `\(`,
		`)`, `\)`,
	)
	return replacer.Replace(value)
}
