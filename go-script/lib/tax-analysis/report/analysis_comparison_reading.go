package report

import (
	"fmt"
	"strings"
)

// readingComparisons keeps each existing pair and its linked reconciliation
// together. Links come from SourceRefs, not similar names or matching amounts.
func readingComparisons(s SectionAnalysisData) (string, map[string]bool) {
	byID := map[string]AnalysisMetric{}
	groups := map[string][]AnalysisMetric{}
	var order []string
	for _, m := range s.Metrics {
		byID[m.ID] = m
		if strings.HasPrefix(m.ID, metricID(s.SectionID, "financial-comparison.")) {
			key := readingGroupID(m.ID)
			if _, exists := groups[key]; !exists {
				order = append(order, key)
			}
			groups[key] = append(groups[key], m)
		}
	}
	var b strings.Builder
	shown := map[string]bool{}
	for _, key := range order {
		b.WriteString("\n### 同口径比较\n")
		for _, m := range groups[key] {
			writeReadingMetric(&b, m)
			shown[m.ID] = true
			if m.Kind == "difference" || m.Kind == "difference-rate" {
				fmt.Fprintf(&b, "口径：%s\n", m.Algorithm)
			}
		}
		// Reconciliation links are added only when source identity and period
		// have already passed the existing financial comparison checks.
		var visit func(string)
		visit = func(id string) {
			m, ok := byID[id]
			if !ok || shown[id] || !isReadingAdjustment(s.SectionID, id) {
				return
			}
			shown[id] = true
			writeReadingMetric(&b, m)
			if m.Algorithm != "" {
				fmt.Fprintf(&b, "口径：%s\n", m.Algorithm)
			}
			for _, ref := range m.SourceRefs {
				visit(ref)
			}
		}
		for _, m := range groups[key] {
			if strings.HasSuffix(m.ID, ".reconciliation") {
				for _, ref := range m.SourceRefs {
					visit(ref)
				}
			}
		}
	}
	return b.String(), shown
}

func isReadingAdjustment(section, id string) bool {
	for _, family := range []string{"cit-quarter-bridge.", "cit-annual-financial-bridge.", "cit-bridge."} {
		if strings.HasPrefix(id, metricID(section, family)) {
			return true
		}
	}
	return false
}
