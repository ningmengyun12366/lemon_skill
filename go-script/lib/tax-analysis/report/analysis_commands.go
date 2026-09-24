package report

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	taxanalysis "invoice-assistant-skill/go-script/lib/tax-analysis"
)

type analysisWorkspace struct {
	Path     string
	Manifest WorkspaceManifest
	Model    Model
	Data     AnalysisData
	Analysis AnalysisOutput
	Index    ReadingIndex
}

// loadAnalysisWorkspace verifies frozen identities before any local read/write.
// It does not log in, create tasks, or rebuild the business calculations.
func loadAnalysisWorkspace(directory string) (analysisWorkspace, error) {
	var w analysisWorkspace
	var err error
	w.Path, err = existingWorkspaceDirectory(directory)
	if err != nil {
		return w, err
	}
	w.Manifest, _, err = loadWorkspaceJSON[WorkspaceManifest](filepath.Join(w.Path, workspaceManifestFilename))
	if err != nil {
		return w, err
	}
	if w.Manifest.ContractVersion != AIContractVersion || w.Manifest.RuleVersion != RuleVersion {
		return w, errors.New("工作区版本已过期，请重新执行report-prepare")
	}
	var modelBytes, dataBytes, sourceBytes []byte
	w.Model, modelBytes, err = loadWorkspaceJSON[Model](filepath.Join(w.Path, workspaceModelFilename))
	if err != nil {
		return w, err
	}
	w.Data, dataBytes, err = loadWorkspaceJSON[AnalysisData](filepath.Join(w.Path, workspaceAnalysisDataFilename))
	if err != nil {
		return w, err
	}
	source, sourceBytes, err := loadWorkspaceJSON[taxanalysis.TaskResponse](filepath.Join(w.Path, workspaceSourceFilename))
	if err != nil {
		return w, err
	}
	w.Analysis, _, err = loadAnalysisOutput(filepath.Join(w.Path, workspaceAnalysisOutputFilename))
	if err != nil {
		return w, err
	}
	v := ValidationResult{SourceDigest: digestBytes(sourceBytes), ModelDigest: digestBytes(modelBytes)}
	validateWorkspaceIdentity(&v, w.Manifest, source, w.Model, w.Analysis)
	if digestBytes(dataBytes) != w.Manifest.AnalysisDataDigest || w.Analysis.AnalysisDataDigest != w.Manifest.AnalysisDataDigest {
		v.ContentErrors = append(v.ContentErrors, "指标底稿摘要不一致")
	}
	if len(v.ContentErrors) > 0 {
		return w, errors.New(strings.Join(v.ContentErrors, "；"))
	}
	var outputSections []string
	for _, section := range w.Analysis.Sections {
		outputSections = append(outputSections, section.SectionID)
	}
	if !sameUniqueStringSet(w.Manifest.AnalysisSectionIDs, outputSections) || !sameUniqueStringSet(w.Manifest.SectionIDs, w.Analysis.SectionOrder) {
		return w, errors.New("analysis-output章节集合与工作区不一致")
	}
	var indexBytes []byte
	w.Index, indexBytes, err = loadWorkspaceJSON[ReadingIndex](filepath.Join(w.Path, workspaceReadingIndexFilename))
	if err != nil {
		return w, err
	}
	if digestBytes(indexBytes) != w.Manifest.ReadingDigest || w.Index.AnalysisDataDigest != w.Manifest.AnalysisDataDigest {
		return w, errors.New("阅读批次索引与冻结底稿不一致")
	}
	return w, nil
}

type AnalysisProgress struct {
	CompletedSectionIDs []string `json:"completedSectionIds"`
	PendingSectionIDs   []string `json:"pendingSectionIds"`
	ConclusionPending   bool     `json:"conclusionPending"`
	NextBatchID         string   `json:"nextBatchId,omitempty"`
}

func analysisProgress(w analysisWorkspace) AnalysisProgress {
	p := AnalysisProgress{CompletedSectionIDs: []string{}, PendingSectionIDs: []string{}}
	v := ValidationResult{}
	validateAnalysisReferences(&v, w.Model, w.Data, w.Analysis)
	bad := map[string]bool{}
	for _, issue := range v.Issues {
		bad[issue.SectionID] = true
	}
	policies := map[string]NarrativePolicy{}
	for _, s := range w.Model.Sections {
		policies[s.Key] = s.Narrative
	}
	sections := map[string]AnalysisSectionOutput{}
	for _, s := range w.Analysis.Sections {
		sections[s.SectionID] = s
	}
	complete := func(id string, items []NarrativeItem, minItems, maxItems int) bool {
		if bad[id] || len(items) < minItems || len(items) > maxItems {
			return false
		}
		for _, item := range items {
			if strings.TrimSpace(item.Text) == "" || len(item.EvidenceRefs) == 0 {
				return false
			}
		}
		return true
	}
	pending := map[string]bool{}
	for _, id := range w.Manifest.AnalysisSectionIDs {
		s, ok := sections[id]
		policy := policies[id]
		if ok && complete(id, s.Items, policy.MinItems, policy.MaxItems) {
			p.CompletedSectionIDs = append(p.CompletedSectionIDs, id)
		} else {
			p.PendingSectionIDs = append(p.PendingSectionIDs, id)
			pending[id] = true
		}
	}
	p.ConclusionPending = !complete("conclusion", w.Analysis.Conclusion.Items, 3, 5)
	for _, b := range w.Index.Batches {
		for _, id := range b.SectionIDs {
			if pending[id] {
				p.NextBatchID = b.ID
				return p
			}
		}
	}
	return p
}

type AnalysisReadResult struct {
	Status              string           `json:"status"`
	AnalysisDataDigest  string           `json:"analysisDataDigest"`
	Progress            AnalysisProgress `json:"progress"`
	BatchID             string           `json:"batchId,omitempty"`
	Content             string           `json:"content"`
	ElapsedMilliseconds int64            `json:"elapsedMilliseconds"`
}

// ReadAnalysis returns the next incomplete batch, a named batch, or one section.
// Empty batch/section means --next. Once sections finish it supplies conclusions.
func ReadAnalysis(directory, batchID, sectionID string) (AnalysisReadResult, error) {
	started := time.Now()
	w, err := loadAnalysisWorkspace(directory)
	if err != nil {
		return AnalysisReadResult{}, err
	}
	if batchID != "" && sectionID != "" {
		return AnalysisReadResult{}, errors.New("batch和section只能指定一个")
	}
	r := AnalysisReadResult{Status: "success", AnalysisDataDigest: w.Manifest.AnalysisDataDigest, Progress: analysisProgress(w)}
	if sectionID != "" {
		if !stringSet(w.Manifest.AnalysisSectionIDs)[sectionID] {
			return r, errors.New("该章节无需AI填写或不存在")
		}
		var policy NarrativePolicy
		for _, s := range w.Model.Sections {
			if s.Key == sectionID {
				policy = s.Narrative
			}
		}
		for _, s := range w.Data.Sections {
			if s.SectionID == sectionID {
				r.Content = readingSectionMarkdown(s, policy, baseTrustedReferences(w.Model))
			}
		}
	} else {
		if batchID == "" {
			batchID = r.Progress.NextBatchID
		}
		if batchID != "" {
			found := false
			for _, b := range w.Index.Batches {
				if b.ID != batchID {
					continue
				}
				found = true
				// Never use an untrusted index path as a filesystem traversal.
				if b.Path != filepath.ToSlash(filepath.Join(workspaceReadingDirectory, b.ID+".md")) || filepath.Base(b.ID) != b.ID {
					return r, errors.New("阅读批次路径无效")
				}
				payload, e := os.ReadFile(filepath.Join(w.Path, filepath.FromSlash(b.Path)))
				if e != nil {
					return r, e
				}
				if digestBytes(payload) != b.SHA256 {
					return r, errors.New("阅读批次内容被修改，请重新prepare")
				}
				r.BatchID = b.ID
				r.Content = string(payload)
			}
			if !found {
				return r, fmt.Errorf("未知批次 %s", batchID)
			}
		} else {
			r.Content = conclusionReading(w)
		}
	}
	r.ElapsedMilliseconds = time.Since(started).Milliseconds()
	return r, nil
}

type AnalysisPatch struct {
	AnalysisDataDigest string                  `json:"analysisDataDigest"`
	Sections           []AnalysisSectionOutput `json:"sections,omitempty"`
	Conclusion         *ConclusionOutput       `json:"conclusion,omitempty"`
}

type AnalysisWriteResult struct {
	Status              string           `json:"status"`
	Valid               bool             `json:"valid"`
	Validation          ValidationResult `json:"validation"`
	Progress            AnalysisProgress `json:"progress"`
	ElapsedMilliseconds int64            `json:"elapsedMilliseconds"`
}

// WriteAnalysis merges complete section replacements under a process lock.
// Invalid submissions leave the old bytes unchanged; no semantic prose rules
// are introduced. Digest identity is mandatory even for a conclusion-only patch.
func WriteAnalysis(directory, inputPath string) (AnalysisWriteResult, error) {
	started := time.Now()
	path, err := existingWorkspaceDirectory(directory)
	if err != nil {
		return AnalysisWriteResult{}, err
	}
	release, err := lockAnalysisWorkspace(path)
	if err != nil {
		return AnalysisWriteResult{}, err
	}
	defer release()
	w, err := loadAnalysisWorkspace(path)
	if err != nil {
		return AnalysisWriteResult{}, err
	}
	patch, _, err := loadWorkspaceJSON[AnalysisPatch](inputPath)
	if err != nil {
		return AnalysisWriteResult{}, err
	}
	r := AnalysisWriteResult{Status: "invalid"}
	if patch.AnalysisDataDigest != w.Manifest.AnalysisDataDigest {
		return r, errors.New("提交的analysisDataDigest与当前工作区不一致，请从report-read复制")
	}
	if len(patch.Sections) == 0 && patch.Conclusion == nil {
		return r, errors.New("提交未包含章节或综合结论")
	}
	expected := stringSet(w.Manifest.AnalysisSectionIDs)
	seen := map[string]bool{}
	policies := map[string]NarrativePolicy{}
	for _, s := range w.Model.Sections {
		policies[s.Key] = s.Narrative
	}
	for _, s := range patch.Sections {
		if !expected[s.SectionID] || seen[s.SectionID] {
			r.Validation.ContentErrors = append(r.Validation.ContentErrors, "未知、无需分析或重复章节："+s.SectionID)
			continue
		}
		seen[s.SectionID] = true
		p := policies[s.SectionID]
		if len(s.Items) < p.MinItems || len(s.Items) > p.MaxItems {
			r.Validation.ContentErrors = append(r.Validation.ContentErrors, fmt.Sprintf("%s.items必须%d—%d条", s.SectionID, p.MinItems, p.MaxItems))
		}
		for i, item := range s.Items {
			validateRequiredNarrativeItem(&r.Validation, fmt.Sprintf("%s.items[%d]", s.SectionID, i), item)
		}
	}
	partial := AnalysisOutput{Sections: patch.Sections}
	if patch.Conclusion != nil {
		partial.Conclusion = *patch.Conclusion
		if len(partial.Conclusion.Items) < 3 || len(partial.Conclusion.Items) > 5 {
			r.Validation.ContentErrors = append(r.Validation.ContentErrors, "conclusion.items必须3—5条")
		}
		for i, item := range partial.Conclusion.Items {
			validateRequiredNarrativeItem(&r.Validation, fmt.Sprintf("conclusion.items[%d]", i), item)
		}
	}
	validateAnalysisReferences(&r.Validation, w.Model, w.Data, partial)
	r.Valid = len(r.Validation.MissingPaths)+len(r.Validation.ContentErrors)+len(r.Validation.InvalidReferences) == 0
	r.Validation.Valid = r.Valid
	r.Validation.DeterministicValid = r.Valid
	if r.Valid {
		for i, s := range w.Analysis.Sections {
			for _, update := range patch.Sections {
				if s.SectionID == update.SectionID {
					w.Analysis.Sections[i] = update
				}
			}
		}
		if patch.Conclusion != nil {
			w.Analysis.Conclusion = *patch.Conclusion
		}
		if err := writeWorkspaceJSON(filepath.Join(w.Path, workspaceAnalysisOutputFilename), w.Analysis); err != nil {
			return r, err
		}
		r.Status = "success"
	}
	r.Validation.Status = r.Status
	r.Progress = analysisProgress(w)
	r.ElapsedMilliseconds = time.Since(started).Milliseconds()
	return r, nil
}

type ReferenceQuery struct {
	SectionID  string
	Name       string
	Subject    string
	Period     string
	BasePeriod string
	ID         string
	Offset     int
	Limit      int
}

type ReferenceMatch struct {
	ID         string   `json:"id"`
	SectionID  string   `json:"sectionId"`
	Name       string   `json:"name"`
	Display    string   `json:"display,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	Period     string   `json:"period,omitempty"`
	BasePeriod string   `json:"basePeriod,omitempty"`
	Scope      []string `json:"scope,omitempty"`
	Subject    string   `json:"subject,omitempty"`
	Algorithm  string   `json:"algorithm,omitempty"`
	Status     string   `json:"status"`
}

type ReferenceResult struct {
	Status             string           `json:"status"`
	AnalysisDataDigest string           `json:"analysisDataDigest"`
	Total              int              `json:"total"`
	NextOffset         *int             `json:"nextOffset,omitempty"`
	Matches            []ReferenceMatch `json:"matches"`
}

// QueryAnalysisReferences searches the complete local archive, not just the
// selected reading objects. Exact periods prevent January/October ambiguity.
func QueryAnalysisReferences(directory string, q ReferenceQuery) (ReferenceResult, error) {
	w, err := loadAnalysisWorkspace(directory)
	if err != nil {
		return ReferenceResult{}, err
	}
	if q.Offset < 0 || q.Limit < 1 || q.Limit > 200 {
		return ReferenceResult{}, errors.New("offset须>=0，limit须为1—200")
	}
	r := ReferenceResult{Status: "success", AnalysisDataDigest: w.Manifest.AnalysisDataDigest, Matches: []ReferenceMatch{}}
	var matches []ReferenceMatch
	seen := map[string]bool{}
	accept := func(m ReferenceMatch) {
		if seen[m.ID] || (q.SectionID != "" && q.SectionID != m.SectionID) || (q.ID != "" && q.ID != m.ID) ||
			(q.Name != "" && !strings.Contains(m.Name, q.Name)) || (q.Subject != "" && !strings.Contains(m.Subject+" "+m.Display+" "+strings.Join(m.Scope, " "), q.Subject)) ||
			(q.Period != "" && q.Period != m.Period) || (q.BasePeriod != "" && q.BasePeriod != m.BasePeriod) {
			return
		}
		seen[m.ID] = true
		matches = append(matches, m)
	}
	for _, s := range w.Data.Sections {
		if q.SectionID != "" && q.SectionID != s.SectionID {
			continue
		}
		subjects := map[string][]string{}
		for _, m := range s.Metrics {
			g := readingGroupID(m.ID)
			if m.Kind == "object" || m.Kind == "period" {
				subjects[g] = append(subjects[g], m.Display)
			}
		}
		for _, m := range s.Metrics {
			g := readingGroupID(m.ID)
			display := m.Display
			if m.Status != "present" {
				display = ""
			}
			accept(ReferenceMatch{m.ID, s.SectionID, m.Name, display, m.Unit, m.Period, m.BasePeriod, m.Scope, strings.Join(subjects[g], "、"), m.Algorithm, m.Status})
		}
	}
	for _, ref := range baseTrustedReferences(w.Model) {
		accept(ReferenceMatch{ID: ref.ID, SectionID: ref.SectionID, Name: ref.Label, Display: ref.Value, Status: "present"})
	}
	r.Total = len(matches)
	start := min(q.Offset, r.Total)
	end := min(start+q.Limit, r.Total)
	r.Matches = append(r.Matches, matches[start:end]...)
	if end < r.Total {
		r.NextOffset = &end
	}
	return r, nil
}
