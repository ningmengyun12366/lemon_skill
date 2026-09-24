package report

import "strings"

// joinEvidenceFragments joins script-owned evidence sentences without carrying
// their terminal punctuation into the separator. Source records stay unchanged.
func joinEvidenceFragments(fragments ...string) string {
	parts := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		if text := strings.TrimRight(strings.TrimSpace(fragment), "。；，;,. \t\r\n"); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "；")
}
