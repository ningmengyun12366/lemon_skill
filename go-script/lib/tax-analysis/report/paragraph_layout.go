package report

import (
	"regexp"
	"strings"
)

// A number and its unit form one wrap opportunity. The original characters
// are retained so PDF and DOCX do not acquire invisible separator characters.
var paragraphNumberPattern = regexp.MustCompile(`[+\-−]?(?:[0-9]{1,3}(?:[,，][0-9]{3})+|[0-9]+)(?:\.[0-9]+)?(?:[ \x{00A0}]*(?:个百分点|亿元|万元|元|%|％))?`)

func paragraphWrapTokens(value string) []string {
	var tokens []string
	appendToken := func(token string) {
		if len(tokens) > 0 && tokens[len(tokens)-1] != "\n" && strings.Contains("。；！？…，、：）》】」』", token) {
			tokens[len(tokens)-1] += token
		} else {
			tokens = append(tokens, token)
		}
	}
	position := 0
	for _, span := range paragraphNumberPattern.FindAllStringIndex(value, -1) {
		for _, r := range value[position:span[0]] {
			appendToken(string(r))
		}
		appendToken(value[span[0]:span[1]])
		position = span[1]
	}
	for _, r := range value[position:] {
		appendToken(string(r))
	}
	return tokens
}

// splitNumericParagraph wraps measured text without breaking normal-sized
// numbers. An exceptionally long token may split to prevent page overflow.
// Measurement and drawing consume the same lines; font errors are propagated.
func splitNumericParagraph(value string, width float64, measure func(string) (float64, error)) ([]string, error) {
	var lines []string
	line := ""
	appendToken := func(token string) error {
		w, err := measure(line + token)
		if err != nil {
			return err
		}
		if line != "" && w > width {
			lines = append(lines, line)
			line = ""
		}
		line += token
		return nil
	}
	for _, token := range paragraphWrapTokens(value) {
		if token == "\n" {
			lines = append(lines, line)
			line = ""
			continue
		}
		w, err := measure(token)
		if err != nil {
			return nil, err
		}
		if w > width {
			for _, r := range token {
				if err := appendToken(string(r)); err != nil {
					return nil, err
				}
			}
		} else if err := appendToken(token); err != nil {
			return nil, err
		}
	}
	if line != "" || len(lines) == 0 || strings.HasSuffix(value, "\n") {
		lines = append(lines, line)
	}
	return lines, nil
}
