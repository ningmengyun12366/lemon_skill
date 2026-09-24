package report

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

func writeAnalysisSectionFiles(workspace string, data AnalysisData) error {
	dir := filepath.Join(workspace, "analysis-sections")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, s := range data.Sections {
		value, err := marshalWorkspaceJSON(analysisSectionCard(s))
		if err != nil {
			return err
		}
		if err := writeNewWorkspaceFile(filepath.Join(dir, s.SectionID+".json"), value); err != nil {
			return err
		}
	}
	return nil
}
func validateAnalysisSectionFiles(workspace string, data AnalysisData) error {
	for _, s := range data.Sections {
		expected, err := marshalWorkspaceJSON(analysisSectionCard(s))
		if err != nil {
			return err
		}
		actual, err := os.ReadFile(filepath.Join(workspace, "analysis-sections", s.SectionID+".json"))
		if err != nil {
			return err
		}
		if !bytes.Equal(expected, actual) {
			return fmt.Errorf("analysis-sections/%s.json与冻结指标底稿不一致", s.SectionID)
		}
	}
	return nil
}

func attachAnalysisReferences(data *AnalysisData, refs []TrustedReference) {
	for i := range data.Sections {
		s := &data.Sections[i]
		for _, r := range refs {
			if r.SectionID == s.SectionID {
				s.References = append(s.References, r)
			}
		}
	}
}
