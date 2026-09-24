package report

import (
	"sort"
	"strings"
)

// AnalysisReadingMetric presents one authoritative display value. Raw precision
// and calculation inputs stay in the audit file, not duplicated in prose input.
type AnalysisReadingMetric struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Display    string   `json:"display"`
	Unit       string   `json:"unit,omitempty"`
	Period     string   `json:"period,omitempty"`
	BasePeriod string   `json:"basePeriod,omitempty"`
	Scope      []string `json:"scope,omitempty"`
	Algorithm  string   `json:"algorithm"`
}

// AnalysisMetricGroup keeps a comparison's values, period and baseline together.
// IDs remain the same trusted references used by the full calculation archive.
type AnalysisMetricGroup struct {
	ID      string                  `json:"id"`
	Metrics []AnalysisReadingMetric `json:"metrics"`
}

// AnalysisLimitation intentionally carries no numeric value or calculation inputs.
// The complete diagnostic metric remains in analysis-data.json for traceability.
type AnalysisLimitation struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Period     string   `json:"period,omitempty"`
	BasePeriod string   `json:"basePeriod,omitempty"`
	Scope      []string `json:"scope,omitempty"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason"`
}

type AnalysisSectionCard struct {
	SectionID       string                  `json:"sectionId"`
	Title           string                  `json:"title"`
	Objective       AnalysisObjective       `json:"objective"`
	Mode            NarrativeMode           `json:"mode"`
	SampleOnly      bool                    `json:"sampleOnly,omitempty"`
	Focus           []AnalysisFocus         `json:"focus"`
	MetricGroups    []AnalysisMetricGroup   `json:"metricGroups"`
	Limitations     []AnalysisLimitation    `json:"limitations"`
	Coverage        []AnalysisPartyCoverage `json:"coverage,omitempty"`
	Constraints     []string                `json:"constraints"`
	Guidance        string                  `json:"guidance,omitempty"`
	RelatedSections []string                `json:"relatedSections,omitempty"`
	FindingIDs      []string                `json:"findingIds,omitempty"`
	References      []TrustedReference      `json:"references"`
}

// analysisSectionCard projects frozen calculations into the Agent's reading
// surface, without exposing unavailable derived numbers as candidate facts.
func analysisSectionCard(s SectionAnalysisData) AnalysisSectionCard {
	c := AnalysisSectionCard{SectionID: s.SectionID, Title: s.Title, Objective: s.Objective,
		Mode: s.Mode, SampleOnly: s.SampleOnly, Constraints: s.Constraints, Guidance: s.Guidance,
		RelatedSections: s.RelatedSections, FindingIDs: s.FindingIDs,
		Coverage: s.Coverage, MetricGroups: []AnalysisMetricGroup{}, Limitations: []AnalysisLimitation{}}
	groups, unavailable := map[string]int{}, map[string]bool{}
	for _, ref := range s.References {
		// Metric IDs and values already appear in groups or coverage; do not
		// duplicate the entire numeric catalog as a second reading surface.
		if !strings.HasPrefix(ref.ID, "fact:"+s.SectionID+".metric.") {
			c.References = append(c.References, ref)
		}
	}
	for _, metric := range s.Metrics {
		if metric.Status != "present" {
			unavailable[metric.ID] = true
			reason := "本指标资料不足，不计算或补零；仅限制所列期间。"
			if metric.Status == "conflicting" {
				reason = "来源口径冲突，不能用于趋势或增长判断；原版本数值可分别陈述，不选择其中一版覆盖。"
			}
			c.Limitations = append(c.Limitations, AnalysisLimitation{metric.ID, metric.Name, metric.Period, metric.BasePeriod, metric.Scope, metric.Status, reason})
			continue
		}
		if strings.HasPrefix(metric.ID, metricID(s.SectionID, "coverage.")) {
			continue // the same status and reference ID are in the coverage object
		}
		key := metric.ID
		if dot := strings.LastIndex(key, "."); dot >= 0 {
			key = key[:dot]
		}
		index, ok := groups[key]
		if !ok {
			index = len(c.MetricGroups)
			groups[key] = index
			c.MetricGroups = append(c.MetricGroups, AnalysisMetricGroup{ID: key})
		}
		c.MetricGroups[index].Metrics = append(c.MetricGroups[index].Metrics, AnalysisReadingMetric{
			metric.ID, metric.Name, metric.Display, metric.Unit, metric.Period, metric.BasePeriod, metric.Scope, metric.Algorithm,
		})
	}
	for _, focus := range s.Focus {
		refs := make([]string, 0, len(focus.EvidenceRefs))
		for _, ref := range focus.EvidenceRefs {
			if !unavailable[ref] {
				refs = append(refs, ref)
			}
		}
		focus.EvidenceRefs = refs
		c.Focus = append(c.Focus, focus)
	}
	priority := map[string]bool{}
	for _, f := range c.Focus {
		for _, ref := range f.EvidenceRefs {
			if dot := strings.LastIndex(ref, "."); dot >= 0 {
				priority[ref[:dot]] = true
			}
		}
	}
	sort.SliceStable(c.MetricGroups, func(i, j int) bool {
		return priority[c.MetricGroups[i].ID] && !priority[c.MetricGroups[j].ID]
	})
	return c
}
