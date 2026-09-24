package report

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

// tableColumnWidths reserves header space first at the narrower Word page
// width. Remaining space follows real body content, with long prose capped so
// it cannot squeeze dates or amounts. Both renderers scale these same ratios.
var tableAtomicValuePattern = regexp.MustCompile(`[+-]?[0-9]+(?:[-/.,][0-9]+)*%?`)

func tableColumnWidths(headers []string, rows [][]string, total float64, variant TableVariant) []float64 {
	const page, font, padding = 490.0, 8.0, 10.0
	widths, needs, desiredWidths := make([]float64, len(headers)), make([]float64, len(headers)), make([]float64, len(headers))
	if len(headers) == 0 {
		return widths
	}
	headerSum := 0.0
	for i, h := range headers {
		widths[i] = textWidthUnits(h)*font + padding
		headerSum += widths[i]
	}
	// Only wrap headers when their combined single-line width cannot fit.
	if headerSum > page {
		for lines := 2; ; lines++ {
			sum := 0.0
			for i, h := range headers {
				widths[i] = math.Max(font, math.Ceil(textWidthUnits(h)/float64(lines))*font) + padding
				sum += widths[i]
			}
			if sum <= page {
				break
			}
			if lines > 100 {
				for i := range widths {
					widths[i] = page / float64(len(widths))
				}
				break
			}
		}
	}
	// After headers, reserve complete numeric/date tokens before prose width.
	atomNeeds := make([]float64, len(headers))
	atomTotal, baseTotal := 0.0, 0.0
	for i := range headers {
		required := widths[i]
		for _, row := range rows {
			if i < len(row) {
				for _, token := range tableAtomicValuePattern.FindAllString(row[i], -1) {
					required = math.Max(required, textWidthUnits(token)*font+padding)
				}
			}
		}
		atomNeeds[i] = required - widths[i]
		atomTotal += atomNeeds[i]
		baseTotal += widths[i]
	}
	if atomTotal > 0 && baseTotal+atomTotal <= page {
		for i := range widths {
			widths[i] += atomNeeds[i]
		}
	}
	used, needSum, desiredSum := 0.0, 0.0, 0.0
	priors := tableColumnRatios(variant, len(headers))
	for i := range headers {
		desired := widths[i]
		for _, row := range rows {
			if i < len(row) {
				desired = math.Max(desired, math.Min(160, textWidthUnits(row[i])*font+padding))
			}
		}
		desiredWidths[i] = desired
		desiredSum += desired
		needs[i] = math.Max(0, desired-widths[i])
		if len(rows) == 0 && priors != nil {
			needs[i] = priors[i] * page
		}
		used += widths[i]
		needSum += needs[i]
	}
	available := math.Max(0, page-used)
	contentAllocation := math.Min(available, needSum)
	for i := range widths {
		if needSum > 0 {
			widths[i] += contentAllocation * needs[i] / needSum
		}
		// Once every cell already fits, spread spare space in proportion to
		// its desired width instead of stretching the only short unmet column.
		widths[i] += (available - contentAllocation) * desiredWidths[i] / desiredSum
		widths[i] *= total / page
	}
	return widths
}

// textWidthUnits conservatively measures CJK and Latin glyphs at table font size.
// Explicit line breaks contribute only their longest line.
func textWidthUnits(text string) float64 {
	longest := 0.0
	for _, line := range strings.Split(text, "\n") {
		width := 0.0
		for _, r := range line {
			if r < 128 {
				width += 0.65
			} else if unicode.IsControl(r) {
				continue
			} else {
				width++
			}
		}
		longest = math.Max(longest, width)
	}
	return longest
}

func containsHeader(header string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(header, candidate) {
			return true
		}
	}
	return false
}
