package report

import "fmt"

// NarrativeIssue locates an invalid field without inferring prose semantics.
// ItemIndex is zero-based; Message names the one-based item for the reader.
type NarrativeIssue struct {
	SectionID   string `json:"sectionId"`
	ItemIndex   int    `json:"itemIndex"`
	Field       string `json:"field"`
	Code        string `json:"code"`
	ReferenceID string `json:"referenceId,omitempty"`
	Message     string `json:"message"`
}

func addNarrativeIssue(result *ValidationResult, sectionID string, index int, field, code, referenceID, message string) {
	message = fmt.Sprintf("%s 第%d条的%s：%s", sectionID, index+1, field, message)
	result.Issues = append(result.Issues, NarrativeIssue{sectionID, index, field, code, referenceID, message})
	if code == "UNKNOWN_REFERENCE" {
		result.InvalidReferences = append(result.InvalidReferences, message)
	} else {
		result.ContentErrors = append(result.ContentErrors, message)
	}
}

// validateAnalysisReferences checks IDs against this frozen report, and checks
// plain-text syntax. It neither expands sources nor checks what the prose claims.
func validateAnalysisReferences(result *ValidationResult, model Model, data AnalysisData, analysis AnalysisOutput) {
	valid := map[string]bool{}
	for _, ref := range append(baseTrustedReferences(model), analysisDataReferences(data)...) {
		valid[ref.ID] = true
	}
	for _, section := range model.Sections {
		for _, id := range section.RuleIDs {
			valid["rule:"+id] = true
		}
	}
	objectives := map[string]map[string]bool{}
	for _, section := range data.Sections {
		objectives[section.SectionID] = map[string]bool{}
		for _, focus := range section.Focus {
			objectives[section.SectionID][focus.ID] = true
		}
	}
	check := func(sectionID string, index int, item NarrativeItem) {
		if anyPlaceholderPattern.MatchString(item.Text) {
			addNarrativeIssue(result, sectionID, index, "text", "PLACEHOLDER_IN_TEXT", "", "请直接填写正文，将引用ID放入evidenceRefs")
		}
		for _, r := range item.Text {
			if (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0xFFFE || r == 0xFFFF {
				addNarrativeIssue(result, sectionID, index, "text", "INVALID_TEXT_CHARACTER", "", "包含文档格式不支持的控制字符")
				break
			}
		}
		for i, ref := range item.EvidenceRefs {
			if !valid[ref] {
				addNarrativeIssue(result, sectionID, index, fmt.Sprintf("evidenceRefs[%d]", i), "UNKNOWN_REFERENCE", ref, "引用不在本报告可用底稿中："+ref)
			}
		}
		for i, id := range item.ObjectiveIDs {
			if !objectives[sectionID][id] {
				addNarrativeIssue(result, sectionID, index, fmt.Sprintf("objectiveIds[%d]", i), "UNKNOWN_OBJECTIVE", id, "目标ID不属于本节："+id)
			}
		}
	}
	for i, item := range analysis.Conclusion.Items {
		check("conclusion", i, item)
	}
	for _, section := range analysis.Sections {
		for i, item := range section.Items {
			check(section.SectionID, i, item)
		}
	}
}

// validateDeliveryContent checks document structure and unsupported template
// syntax. Natural-language quality and factual interpretation belong to the Agent.
func validateDeliveryContent(model Model) []string {
	errors := validateDeliveryShape(model)
	for _, s := range model.Sections {
		if s.ID == "10" && len(model.Findings) == 0 {
			errors = append(errors, "无确认风险时不得生成第十章")
		}
		for i, b := range s.Blocks {
			if b.Kind == BlockParagraph && b.Title == "整改建议" && s.Narrative.Mode != NarrativeRisk {
				errors = append(errors, fmt.Sprintf("%s.blocks[%d]只有风险状态允许整改建议", s.Key, i))
			}
			for j, value := range b.Paragraphs {
				if anyPlaceholderPattern.MatchString(value) {
					errors = append(errors, fmt.Sprintf("%s.blocks[%d].paragraphs[%d]含占位符，请填写纯正文", s.Key, i, j))
				}
			}
		}
	}
	return uniqueSortedStrings(errors)
}
