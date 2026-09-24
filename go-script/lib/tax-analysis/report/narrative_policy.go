package report

import "strings"

const (
	defaultNarrativeMinItems = 1
	defaultNarrativeMaxItems = 3
	descriptionMaxItems      = 2
)

// assignNarrativePolicies also supports isolated fixtures. Production checks
// data coverage and applicability first through applyDeliveryPolicies.
func assignNarrativePolicies(sections []Section, evaluations []RuleEvaluation) []Section {
	risks := map[string]bool{}
	for _, e := range evaluations {
		if e.Outcome == OutcomeRisk {
			risks[e.RuleID] = true
		}
	}
	result := append([]Section(nil), sections...)
	for i := range result {
		result[i].Narrative = narrativePolicyForSection(result[i], risks)
		result[i].FindingIDs = nil
		for _, finding := range sectionConfirmedFindings(result[i], evaluations) {
			result[i].FindingIDs = append(result[i].FindingIDs, finding.ID)
		}
		if result[i].Narrative.Mode == NarrativeRisk && result[i].Narrative.MaxItems > 0 {
			result[i].Narrative.MinItems = min(3, len(result[i].FindingIDs))
		}
	}
	return result
}

// sectionConfirmedFindings binds a section to the final merged risk identities,
// including the winning ID when several periods belong to one risk theme.
func sectionConfirmedFindings(section Section, evaluations []RuleEvaluation) []Finding {
	themes := map[string]bool{}
	rules := stringSet(section.RuleIDs)
	for _, e := range evaluations {
		if e.Outcome == OutcomeRisk && e.Finding != nil && rules[e.RuleID] {
			themes[textOr(e.Finding.RiskThemeID, e.Finding.ID)] = true
		}
	}
	var result []Finding
	for _, f := range findingsFromEvaluations(evaluations) {
		if themes[textOr(f.RiskThemeID, f.ID)] {
			result = append(result, f)
		}
	}
	return result
}

func narrativePolicyForSection(section Section, riskRules map[string]bool) NarrativePolicy {
	if section.Level == 1 {
		return NarrativePolicy{Mode: NarrativeNone}
	}
	if strings.HasPrefix(section.Key, "risk.") {
		return NarrativePolicy{Mode: NarrativeFact}
	}
	for _, id := range section.RuleIDs {
		if riskRules[id] {
			if strings.HasPrefix(section.Key, "remediation.") {
				return NarrativePolicy{Mode: NarrativeRisk}
			}
			return NarrativePolicy{Mode: NarrativeRisk, MinItems: 1, MaxItems: 3}
		}
	}
	if sectionHasStructuredFacts(section) {
		return factNarrativePolicyForSection(section)
	}
	return NarrativePolicy{Mode: NarrativeDataGap}
}

// factNarrativePolicyForSection prevents prose from repeating the designated
// fact tables. Risk analysis and script-owned data notices keep their policies.
func factNarrativePolicyForSection(section Section) NarrativePolicy {
	switch section.Key {
	case "enterprise.profile", "enterprise.shareholders", "enterprise.equity-relations", "enterprise.changes", "invoice.overall":
		return NarrativePolicy{Mode: NarrativeFact}
	default:
		return NarrativePolicy{Mode: NarrativeFact, MaxItems: descriptionMaxItems}
	}
}

func sectionNarrativeTitle(mode NarrativeMode) string {
	switch mode {
	case NarrativeRisk:
		return "风险分析"
	case NarrativeFact, NarrativeNotApplicable, NarrativeNoIssue:
		return "说明"
	case NarrativeAttention:
		return "关注事项"
	case NarrativeDataGap:
		return "数据说明"
	default:
		return ""
	}
}

// addSectionRiskBands marks each analytical subsection using only confirmed
// findings bound to that section. Overall cards already own their risk bands.
func addSectionRiskBands(sections []Section, evaluations []RuleEvaluation) []Section {
	for i := range sections {
		s := &sections[i]
		if s.Level != 2 || s.SampleOnly || s.DataNotice != "" || strings.HasPrefix(s.Key, "risk.") || strings.HasPrefix(s.Key, "remediation.") || s.Key == "conclusion" {
			continue
		}
		if band, ok := buildRiskBand(sectionConfirmedFindings(*s, evaluations)); ok {
			s.Blocks = append([]Block{band}, s.Blocks...)
		}
	}
	return sections
}
