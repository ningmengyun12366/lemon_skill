package report

import "strings"

// readingMetricSelection narrows repetitive numeric chapters, never the frozen
// facts. Focus inputs, unavailable facts, negative observations and extrema stay
// visible; the complete tables and series remain available through report-refs.
func readingMetricSelection(s SectionAnalysisData) map[string]bool {
	if len(s.FindingIDs) > 0 || !(strings.HasPrefix(s.SectionID, "financial.") || s.SectionID == "vat.period-overview" || s.SectionID == "cit.deduction-evidence") {
		return nil
	}
	selected := map[string]bool{}
	latest := map[string]string{}
	for _, m := range s.Metrics {
		if strings.HasSuffix(m.ID, ".value") && m.Kind == "value" && m.Status == "present" {
			key := strings.TrimSuffix(readingGroupID(m.ID), "."+analysisToken(m.Period))
			if m.Period > latest[key] {
				latest[key] = m.Period
			}
		}
	}
	for _, f := range s.Focus {
		for _, ref := range f.EvidenceRefs {
			selected[ref] = true
		}
	}
	for _, m := range s.Metrics {
		// Annual reconciliations are bounded summaries. Keep both matching and
		// differing values so the reader sees which fields were checked.
		if strings.HasPrefix(m.ID, metricID(s.SectionID, "annual-flow.")) || strings.HasPrefix(m.ID, metricID(s.SectionID, "cash-profit.")) {
			selected[m.ID] = true
		}
		if m.Status != "present" || m.Kind == "extremum" || m.Kind == "sum" || m.Unit == "" && m.Kind != "period" ||
			(m.Kind == "value" || m.Kind == "amount") && m.Value < 0 || strings.HasPrefix(m.ID, metricID(s.SectionID, "deduction-gap.")) {
			selected[m.ID] = true
		}
		if m.BasePeriod != "" || m.Kind == "value" {
			group := readingGroupID(m.ID)
			key := strings.TrimSuffix(group, "."+analysisToken(m.Period))
			if latest[key] == m.Period && (strings.HasSuffix(m.ID, ".value") || strings.HasSuffix(m.ID, ".change") || strings.Contains(m.ID, ".streak")) {
				selected[m.ID] = true
			}
		}
	}
	// Include numeric operands of selected comparisons, but never expand the
	// complete source series merely because an extremum cites all of its inputs.
	for changed := true; changed; {
		changed = false
		for _, m := range s.Metrics {
			if !selected[m.ID] || m.Kind == "extremum" || m.Kind == "sum" {
				continue
			}
			for _, ref := range m.SourceRefs {
				if strings.Contains(ref, ".metric.") && !selected[ref] {
					selected[ref], changed = true, true
				}
			}
		}
	}
	return selected
}
