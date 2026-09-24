package report

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Guidance is embedded so installed Skills need no external books or local
// design files. It supplies writing directions, never facts or risk rules.
//
//go:embed analysis_guidance_common.md
var commonAnalysisGuidance string

//go:embed analysis_guidance_sections.json
var sectionAnalysisGuidanceJSON []byte

var sectionAnalysisGuidance = loadSectionAnalysisGuidance()

// loadSectionAnalysisGuidance expands shared cards by stable semantic ID.
// Invalid built-in content is a development error, rather than a reason to
// silently send an unrelated or empty card to the Agent.
func loadSectionAnalysisGuidance() map[string]string {
	var cards []struct {
		SectionIDs []string `json:"sectionIds"`
		Guidance   string   `json:"guidance"`
	}
	if err := json.Unmarshal(sectionAnalysisGuidanceJSON, &cards); err != nil {
		panic(fmt.Errorf("invalid embedded analysis guidance: %w", err))
	}
	result := make(map[string]string)
	for _, card := range cards {
		if len(card.SectionIDs) == 0 || strings.TrimSpace(card.Guidance) == "" {
			panic("embedded analysis guidance has an empty card")
		}
		for _, id := range card.SectionIDs {
			if strings.TrimSpace(id) == "" || result[id] != "" {
				panic(fmt.Sprintf("invalid or duplicate analysis guidance section: %q", id))
			}
			result[id] = card.Guidance
		}
	}
	return result
}

// analysisGuidanceForSection respects the model's existing writing gate.
// Fixed notices and table-only sections do not become new analysis tasks.
func analysisGuidanceForSection(section Section) string {
	if section.Narrative.MaxItems == 0 {
		return ""
	}
	return sectionAnalysisGuidance[section.Key]
}
