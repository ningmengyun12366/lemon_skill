package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	taxanalysis "invoice-assistant-skill/go-script/lib/tax-analysis"
)

var anyPlaceholderPattern = regexp.MustCompile(`\{\{[^{}]+\}\}`)

type PrepareOptions struct {
	PreparationMilliseconds int64
	WorkspaceDir            string
	PollInterval            time.Duration
	SupplementalRetries     int
	Now                     time.Time
	// EnterpriseContext is the whitelisted enterprise snapshot returned by the
	// same OpenTax login-preparation flow. It may supply identity display fields,
	// but its potentially stale nsrzg value never determines taxpayer status.
	EnterpriseContext map[string]any
}

type PrepareResult struct {
	Status             string           `json:"status"`
	WorkspacePath      string           `json:"workspacePath"`
	TaskID             string           `json:"taskId"`
	DataStatus         string           `json:"dataStatus"`
	RuleVersion        string           `json:"ruleVersion"`
	AIContractVersion  string           `json:"aiContractVersion"`
	IncludedSections   []string         `json:"includedSections"`
	AnalysisSectionIDs []string         `json:"analysisSectionIds"`
	Warnings           []string         `json:"warnings"`
	BatchCount         int              `json:"batchCount"`
	Timings            map[string]int64 `json:"timingsMilliseconds"`
}

type WorkspaceManifest struct {
	ContractVersion    string    `json:"contractVersion"`
	RuleVersion        string    `json:"ruleVersion"`
	TaskID             string    `json:"taskId"`
	USCC               string    `json:"uscc"`
	CompanyName        string    `json:"companyName"`
	StartTime          string    `json:"startTime"`
	EndTime            string    `json:"endTime"`
	GeneratedAt        time.Time `json:"generatedAt"`
	DataStatus         string    `json:"dataStatus"`
	SourceDigest       string    `json:"sourceDigest"`
	ModelDigest        string    `json:"modelDigest"`
	AnalysisDataDigest string    `json:"analysisDataDigest"`
	ReadingDigest      string    `json:"readingDigest"`
	SectionIDs         []string  `json:"sectionIds"`
	AnalysisSectionIDs []string  `json:"analysisSectionIds"`
	FindingIDs         []string  `json:"findingIds"`
}

type TrustedReference struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	SectionID string `json:"sectionId,omitempty"`
}

type AnalysisSectionOutput struct {
	SectionID string          `json:"sectionId"`
	Items     []NarrativeItem `json:"items"`
}

// NarrativeItem keeps plain report prose separate from its local source IDs.
// References are checked for validity, not used to verify the prose semantics.
type NarrativeItem struct {
	Text         string   `json:"text"`
	EvidenceRefs []string `json:"evidenceRefs"`
	ObjectiveIDs []string `json:"objectiveIds,omitempty"`
}

// ConclusionOutput contains three to five evidence-bound V4 priorities.
// Every item is rendered as one numbered paragraph.
type ConclusionOutput struct {
	Items []NarrativeItem `json:"items"`
}

type AnalysisOutput struct {
	ContractVersion    string                  `json:"contractVersion"`
	TaskID             string                  `json:"taskId"`
	SourceDigest       string                  `json:"sourceDigest"`
	ModelDigest        string                  `json:"modelDigest"`
	AnalysisDataDigest string                  `json:"analysisDataDigest"`
	SectionOrder       []string                `json:"sectionOrder"`
	Sections           []AnalysisSectionOutput `json:"sections"`
	Conclusion         ConclusionOutput        `json:"conclusion"`
}

type ValidationResult struct {
	ElapsedMilliseconds int64            `json:"elapsedMilliseconds"`
	Status              string           `json:"status"`
	Valid               bool             `json:"valid"`
	TaskID              string           `json:"taskId,omitempty"`
	AnalysisDigest      string           `json:"analysisDigest,omitempty"`
	SourceDigest        string           `json:"sourceDigest,omitempty"`
	ModelDigest         string           `json:"modelDigest,omitempty"`
	AnalysisDataDigest  string           `json:"analysisDataDigest,omitempty"`
	DeterministicValid  bool             `json:"deterministicValid"`
	MissingPaths        []string         `json:"missingPaths"`
	InvalidReferences   []string         `json:"invalidReferences"`
	ContentErrors       []string         `json:"contentErrors"`
	Issues              []NarrativeIssue `json:"issues,omitempty"`
}

// PrepareWorkspace obtains a reportable task, freezes its source and common
// report model, and creates the strict Agent analysis template.
//
// Parameters: ctx controls the remote task, runner executes the OpenTax task,
// request identifies the bound enterprise and month range, and options control
// the local workspace. Returns the prepared workspace summary or an error.
func PrepareWorkspace(ctx context.Context, runner TaskRunner, request taxanalysis.CreateTaskRequest, options PrepareOptions) (PrepareResult, error) {
	started := time.Now()
	timings := map[string]int64{"enterpriseAndLoginPreparation": options.PreparationMilliseconds}
	workspace, err := prepareWorkspaceDirectory(options.WorkspaceDir)
	if err != nil {
		return PrepareResult{}, err
	}
	collectionStarted := time.Now()
	task, err := obtainReportTask(ctx, runner, request, options.PollInterval, options.SupplementalRetries)
	if err != nil {
		return PrepareResult{}, err
	}
	timings["collection"] = time.Since(collectionStarted).Milliseconds()
	modelStarted := time.Now()
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	model, err := BuildModelWithEnterpriseContext(task, now, options.EnterpriseContext)
	if err != nil {
		return PrepareResult{}, err
	}
	model = applyAnalysisAttention(concretizeAnalysisActions(prepareModelForAI(model)))
	sourceBytes, err := marshalWorkspaceJSON(task)
	if err != nil {
		return PrepareResult{}, err
	}
	modelBytes, err := marshalWorkspaceJSON(model)
	if err != nil {
		return PrepareResult{}, err
	}
	analysisSections := analysisRequiredSectionIDs(model)
	references := trustedReferences(model)
	analysisData := buildAnalysisData(model)
	attachAnalysisReferences(&analysisData, references)
	analysisDataBytes, err := marshalWorkspaceJSON(analysisData)
	if err != nil {
		return PrepareResult{}, err
	}
	timings["model"] = time.Since(modelStarted).Milliseconds()
	writeStarted := time.Now()
	readingIndex, readingFiles := buildReadingBatches(model, analysisData, digestBytes(analysisDataBytes))
	readingBytes, err := marshalWorkspaceJSON(readingIndex)
	if err != nil {
		return PrepareResult{}, err
	}
	manifest := WorkspaceManifest{
		ContractVersion: AIContractVersion, RuleVersion: RuleVersion, TaskID: task.TaskID,
		USCC: model.USCC, CompanyName: model.CompanyName, StartTime: model.StartTime, EndTime: model.EndTime,
		GeneratedAt: model.GeneratedAt, DataStatus: string(task.Status), SourceDigest: digestBytes(sourceBytes), ModelDigest: digestBytes(modelBytes),
		SectionIDs: modelSemanticIDs(model), AnalysisSectionIDs: analysisSections, FindingIDs: findingIDs(model.Findings),
		AnalysisDataDigest: digestBytes(analysisDataBytes), ReadingDigest: digestBytes(readingBytes),
	}
	template := AnalysisOutput{
		ContractVersion: AIContractVersion, TaskID: task.TaskID, SourceDigest: manifest.SourceDigest,
		ModelDigest: manifest.ModelDigest, SectionOrder: append([]string(nil), manifest.SectionIDs...),
		AnalysisDataDigest: manifest.AnalysisDataDigest,
		Sections:           make([]AnalysisSectionOutput, 0, len(analysisSections)),
	}
	for _, sectionID := range analysisSections {
		template.Sections = append(template.Sections, AnalysisSectionOutput{SectionID: sectionID, Items: []NarrativeItem{}})
	}
	manifestBytes, err := marshalWorkspaceJSON(manifest)
	if err != nil {
		return PrepareResult{}, err
	}
	templateBytes, err := marshalWorkspaceJSON(template)
	if err != nil {
		return PrepareResult{}, err
	}
	taskMarkdown := analysisTaskMarkdown(manifest, model)
	files := map[string][]byte{
		workspaceManifestFilename: manifestBytes, workspaceSourceFilename: sourceBytes,
		workspaceModelFilename: modelBytes, workspaceAnalysisTaskFilename: []byte(taskMarkdown),
		workspaceAnalysisOutputFilename: templateBytes,
		workspaceAnalysisDataFilename:   analysisDataBytes,
		workspaceReadingIndexFilename:   readingBytes,
	}
	for filename, data := range readingFiles {
		files[filename] = data
	}
	if err := os.MkdirAll(filepath.Join(workspace, workspaceReadingDirectory), 0700); err != nil {
		return PrepareResult{}, err
	}
	for filename, data := range files {
		if err := writeNewWorkspaceFile(filepath.Join(workspace, filename), data); err != nil {
			return PrepareResult{}, err
		}
	}
	if err := writeAnalysisSectionFiles(workspace, analysisData); err != nil {
		return PrepareResult{}, err
	}
	timings["writeWorkspace"] = time.Since(writeStarted).Milliseconds()
	timings["total"] = time.Since(started).Milliseconds() + options.PreparationMilliseconds
	result := PrepareResult{
		BatchCount: len(readingIndex.Batches), Timings: timings,
		Status: "success", WorkspacePath: workspace, TaskID: task.TaskID, DataStatus: string(task.Status),
		RuleVersion: RuleVersion, AIContractVersion: AIContractVersion, IncludedSections: model.SectionIDs(),
		AnalysisSectionIDs: analysisSections, Warnings: append([]string(nil), model.Warnings...),
	}
	if err := writeWorkspaceJSON(filepath.Join(workspace, "prepare-result.json"), result); err != nil {
		return PrepareResult{}, err
	}
	return result, nil
}

// ValidateWorkspace validates the Agent-authored file. Parameter recordResult
// controls whether the latest validation evidence is persisted for the public
// report-validate command. Validation attempts are intentionally unlimited.
func ValidateWorkspace(workspaceDir string, recordResult bool) (ValidationResult, error) {
	started := time.Now()
	workspace, err := existingWorkspaceDirectory(workspaceDir)
	if err != nil {
		return ValidationResult{}, err
	}
	result := ValidationResult{Status: "invalid"}
	manifest, _, err := loadWorkspaceJSON[WorkspaceManifest](filepath.Join(workspace, workspaceManifestFilename))
	if err != nil {
		return result, err
	}
	model, modelBytes, err := loadWorkspaceJSON[Model](filepath.Join(workspace, workspaceModelFilename))
	if err != nil {
		return result, err
	}
	source, sourceBytes, err := loadWorkspaceJSON[taxanalysis.TaskResponse](filepath.Join(workspace, workspaceSourceFilename))
	if err != nil {
		return result, err
	}
	analysis, analysisBytes, err := loadAnalysisOutput(filepath.Join(workspace, workspaceAnalysisOutputFilename))
	if err != nil {
		return result, err
	}
	result.TaskID, result.SourceDigest, result.ModelDigest = manifest.TaskID, digestBytes(sourceBytes), digestBytes(modelBytes)
	result.AnalysisDigest = digestBytes(analysisBytes)
	data, dataBytes, err := loadWorkspaceJSON[AnalysisData](filepath.Join(workspace, workspaceAnalysisDataFilename))
	if err != nil {
		return result, err
	}
	result.AnalysisDataDigest = digestBytes(dataBytes)
	if result.AnalysisDataDigest != manifest.AnalysisDataDigest || analysis.AnalysisDataDigest != manifest.AnalysisDataDigest {
		result.ContentErrors = append(result.ContentErrors, "analysis-data.json或analysis-output.json的指标摘要与manifest不一致")
	}
	expectedData := buildAnalysisData(model)
	attachAnalysisReferences(&expectedData, trustedReferences(model))
	expectedBytes, err := marshalWorkspaceJSON(expectedData)
	if err != nil {
		return result, err
	}
	if !bytes.Equal(dataBytes, expectedBytes) {
		result.ContentErrors = append(result.ContentErrors, "analysis-data.json与冻结模型重新计算结果不一致")
	}
	if err := validateAnalysisSectionFiles(workspace, data); err != nil {
		result.ContentErrors = append(result.ContentErrors, err.Error())
	}
	if err := validateReadingBatches(workspace, manifest, model, expectedData); err != nil {
		result.ContentErrors = append(result.ContentErrors, err.Error())
	}
	validateWorkspaceIdentity(&result, manifest, source, model, analysis)
	validateAnalysisShape(&result, manifest, model, analysis)
	validateAnalysisReferences(&result, model, expectedData, analysis)
	if len(result.MissingPaths) == 0 && len(result.InvalidReferences) == 0 && len(result.ContentErrors) == 0 {
		finalModel, applyErr := applyValidatedAnalysis(model, analysis)
		if applyErr != nil {
			result.ContentErrors = append(result.ContentErrors, applyErr.Error())
		} else {
			result.ContentErrors = append(result.ContentErrors, validateDeliveryContent(finalModel)...)
		}
	}
	result.MissingPaths = uniqueSortedStrings(result.MissingPaths)
	result.InvalidReferences = uniqueSortedStrings(result.InvalidReferences)
	result.ContentErrors = uniqueSortedStrings(result.ContentErrors)
	result.DeterministicValid = len(result.MissingPaths) == 0 && len(result.InvalidReferences) == 0 && len(result.ContentErrors) == 0
	result.Valid = result.DeterministicValid
	if result.Valid {
		result.Status = "success"
	}
	result.ElapsedMilliseconds = time.Since(started).Milliseconds()
	if recordResult {
		if err := writeValidationResult(workspace, result); err != nil {
			return result, err
		}
	}
	return result, nil
}

// RenderWorkspace revalidates the frozen inputs and validated Agent analysis,
// then writes DOCX and PDF from one final model.
func RenderWorkspace(ctx context.Context, workspaceDir, outputDir string, options GenerateOptions) (Result, error) {
	started := time.Now()
	workspace, err := existingWorkspaceDirectory(workspaceDir)
	if err != nil {
		return Result{}, err
	}
	release, err := lockAnalysisWorkspace(workspace)
	if err != nil {
		return Result{}, err
	}
	defer release()
	validationStarted := time.Now()
	validation, err := ValidateWorkspace(workspace, false)
	if err != nil {
		return Result{}, err
	}
	if !validation.Valid {
		return Result{}, fmt.Errorf("AI分析校验未通过：missingPaths=%v invalidReferences=%v contentErrors=%v", validation.MissingPaths, validation.InvalidReferences, validation.ContentErrors)
	}
	savedValidation, _, err := loadWorkspaceJSON[ValidationResult](filepath.Join(workspace, workspaceValidationFilename))
	if err != nil {
		return Result{}, fmt.Errorf("请先执行 report-validate 并通过校验：%w", err)
	}
	if !savedValidation.Valid || savedValidation.AnalysisDigest != validation.AnalysisDigest ||
		savedValidation.SourceDigest != validation.SourceDigest || savedValidation.ModelDigest != validation.ModelDigest ||
		savedValidation.AnalysisDataDigest != validation.AnalysisDataDigest {
		return Result{}, errors.New("AI分析或可信输入在校验后已发生变化，请重新执行 report-validate")
	}
	manifest, _, err := loadWorkspaceJSON[WorkspaceManifest](filepath.Join(workspace, workspaceManifestFilename))
	if err != nil {
		return Result{}, err
	}
	model, _, err := loadWorkspaceJSON[Model](filepath.Join(workspace, workspaceModelFilename))
	if err != nil {
		return Result{}, err
	}
	if err := validateProductionModel(model); err != nil {
		return Result{}, err
	}
	analysis, _, err := loadAnalysisOutput(filepath.Join(workspace, workspaceAnalysisOutputFilename))
	if err != nil {
		return Result{}, err
	}
	model, err = applyValidatedAnalysis(model, analysis)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(outputDir) == "" {
		return Result{}, errors.New("output directory is required")
	}
	validationMillis := time.Since(validationStarted).Milliseconds()
	options.OutputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return Result{}, err
	}
	fingerprint := ""
	receiptPath := filepath.Join(workspace, workspaceRenderResultFilename)
	if options.chartRenderer == nil {
		fingerprint, err = renderFingerprint(validation, options)
		if err != nil {
			return Result{}, err
		}
		if previous, ok := reusableRenderReceipt(receiptPath, fingerprint, options.OutputDir); ok {
			previous.Timings = map[string]int64{"validation": validationMillis, "total": time.Since(started).Milliseconds()}
			return previous, nil
		}
	}
	result, err := renderModelPair(ctx, model, manifest.DataStatus, options)
	if err != nil {
		return Result{}, err
	}
	result.TaskID = manifest.TaskID
	result.ResultRecordPath = receiptPath
	result.Timings["validation"] = validationMillis
	result.Timings["total"] = time.Since(started).Milliseconds()
	if err := saveRenderReceipt(receiptPath, fingerprint, result); err != nil {
		return Result{}, err
	}
	return result, nil
}

// validateProductionModel prevents synthetic visual fixtures from entering the
// public report-render path. Parameter model is the frozen workspace model.
// Return value is nil only for production report data.
func validateProductionModel(model Model) error {
	if model.FixtureOnly {
		return errors.New("全图视觉覆盖夹具只能用于本地视觉回归，禁止通过正式report-render生成企业报告")
	}
	return nil
}

// prepareWorkspaceDirectory resolves and reserves an empty analysis workspace.
// Parameter value is the requested path. Return value is its absolute path or
// an error when workflow files already exist.
func prepareWorkspaceDirectory(value string) (string, error) {
	workspace := strings.TrimSpace(value)
	if workspace == "" {
		return "", errors.New("workspace directory is required")
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return "", fmt.Errorf("create report workspace: %w", err)
	}
	for _, filename := range []string{workspaceManifestFilename, workspaceSourceFilename, workspaceModelFilename, workspaceAnalysisDataFilename, workspaceReadingIndexFilename, workspaceReadingDirectory, workspaceAnalysisTaskFilename, workspaceAnalysisOutputFilename, workspaceValidationFilename} {
		if exists, err := pathExists(filepath.Join(absolute, filename)); err != nil {
			return "", err
		} else if exists {
			return "", fmt.Errorf("report workspace already contains %s; use a new directory", filename)
		}
	}
	return absolute, nil
}

// existingWorkspaceDirectory validates a prepared workspace path. Parameter
// value is the requested directory. Return value is its absolute path.
func existingWorkspaceDirectory(value string) (string, error) {
	workspace := strings.TrimSpace(value)
	if workspace == "" {
		return "", errors.New("workspace directory is required")
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("open report workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("report workspace must be a directory")
	}
	return absolute, nil
}

// marshalWorkspaceJSON encodes stable indented UTF-8 JSON. Parameter value is
// any workflow object. Returns newline-terminated bytes or an encoding error.
func marshalWorkspaceJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// writeNewWorkspaceFile creates one workflow file without overwriting. Its
// parameters are the target path and bytes. Return value is a write error.
func writeNewWorkspaceFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create workspace file %s: %w", filepath.Base(path), err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// writeWorkspaceJSON replaces mutable validation metadata using strict JSON.
// Parameters are the target path and value. Return value is a write error.
func writeWorkspaceJSON(path string, value any) error {
	data, err := marshalWorkspaceJSON(value)
	if err != nil {
		return err
	}
	if err := atomicWorkspaceWrite(path, data); err != nil {
		return fmt.Errorf("write workspace file %s: %w", filepath.Base(path), err)
	}
	return nil
}

// loadWorkspaceJSON reads one typed workflow file. Parameter path identifies
// the JSON file. Returns the decoded value, exact bytes and any read error.
func loadWorkspaceJSON[T any](path string) (T, []byte, error) {
	var result T
	data, err := os.ReadFile(path)
	if err != nil {
		return result, nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return result, nil, fmt.Errorf("%s必须仅包含一个JSON对象", filepath.Base(path))
	}
	return result, data, nil
}

// loadAnalysisOutput accepts the current plain-prose contract. It returns the
// decoded output and exact bytes, or asks for a new workspace on other versions.
func loadAnalysisOutput(path string) (AnalysisOutput, []byte, error) {
	var identity struct {
		ContractVersion string `json:"contractVersion"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AnalysisOutput{}, nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return AnalysisOutput{}, nil, fmt.Errorf("decode %s identity: %w", filepath.Base(path), err)
	}
	if identity.ContractVersion != AIContractVersion {
		return AnalysisOutput{}, nil, fmt.Errorf("%s使用旧版AI契约%s，请重新执行report-prepare生成%s工作区", filepath.Base(path), textOr(identity.ContractVersion, "未标识"), AIContractVersion)
	}
	analysis, _, err := loadWorkspaceJSON[AnalysisOutput](path)
	return analysis, data, err
}

// digestBytes computes a lowercase SHA-256. Parameter data is the exact file
// payload. Return value is the hexadecimal digest.
func digestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

// modelSemanticIDs returns model section keys in render order. Parameter model
// is the frozen deterministic model. Return value is a new slice.
func modelSemanticIDs(model Model) []string {
	result := make([]string, 0, len(model.Sections))
	for _, section := range model.Sections {
		result = append(result, section.Key)
	}
	return result
}

// findingIDs returns sorted stable finding IDs. Parameter findings contains
// deterministic results. Return value is unique and sorted.
func findingIDs(findings []Finding) []string {
	result := make([]string, 0, len(findings))
	for _, finding := range findings {
		result = append(result, finding.ID)
	}
	sort.Strings(result)
	return result
}

// prepareModelForAI removes script-authored final narrative while preserving
// deterministic facts. Parameter model is copied by value. Return value is the
// frozen model the Agent may analyze.
func prepareModelForAI(model Model) Model {
	for sectionIndex := range model.Sections {
		section := &model.Sections[sectionIndex]
		if section.Key == "chapter.conclusion" {
			section.Blocks = nil
			continue
		}
		structured := false
		for _, block := range section.Blocks {
			if block.Kind == BlockTable || block.Kind == BlockChart || block.Kind == BlockRiskCard {
				structured = true
				break
			}
		}
		if !structured || section.Level == 1 {
			continue
		}
		filtered := make([]Block, 0, len(section.Blocks))
		for _, block := range section.Blocks {
			if block.Kind != BlockParagraph || preservedFactParagraphTitles[strings.TrimSpace(block.Title)] {
				filtered = append(filtered, block)
			}
		}
		section.Blocks = filtered
	}
	return model
}

// sectionHasStructuredFacts reports whether a section requires Agent analysis.
// Parameter section is one final tree node. Return value excludes fixed-only
// narrative sections.
func sectionHasStructuredFacts(section Section) bool {
	for _, block := range section.Blocks {
		switch block.Kind {
		case BlockTable:
			if tableHasDeliveryFacts(block) {
				return true
			}
		case BlockChart:
			if block.Chart != nil && chartContainsStructuredFacts(*block.Chart) {
				return true
			}
		case BlockRiskCard:
			return true
		}
	}
	return false
}

// chartContainsStructuredFacts distinguishes real source values from a visual
// empty state. Parameter chart is one rendered component. Return value is true
// when at least one series value or matrix point is explicitly available.
func chartContainsStructuredFacts(chart Chart) bool {
	for _, series := range chart.Series {
		for index := range series.Values {
			if len(series.Available) == 0 || index >= len(series.Available) || series.Available[index] {
				return true
			}
		}
	}
	for _, point := range chart.Points {
		if point.Available {
			return true
		}
	}
	return false
}

// analysisRequiredSectionIDs returns all level-two sections that contain
// structured facts. Parameter model is the frozen tree. Return order is stable.
func analysisRequiredSectionIDs(model Model) []string {
	result := make([]string, 0)
	for _, section := range model.Sections {
		if section.Level == 2 && section.Narrative.MaxItems > 0 {
			result = append(result, section.Key)
		}
	}
	return result
}

// trustedReferences flattens report facts, charts and findings into immutable
// source references. Section IDs help the Agent locate the corresponding data.
func trustedReferences(model Model) []TrustedReference {
	return append(baseTrustedReferences(model), analysisDataReferences(buildAnalysisData(model))...)
}

func baseTrustedReferences(model Model) []TrustedReference {
	overallLevel := deliveryOverallRisk(model)
	result := []TrustedReference{
		{ID: "fact:report.companyName", Kind: "fact", Label: "企业名称", Value: model.CompanyName},
		{ID: "fact:report.uscc", Kind: "fact", Label: "统一社会信用代码", Value: model.USCC},
		{ID: "fact:report.startDate", Kind: "fact", Label: "报告开始日期", Value: model.StartTime},
		{ID: "fact:report.endDate", Kind: "fact", Label: "报告结束日期", Value: model.EndTime},
		{ID: "fact:report.generatedDate", Kind: "fact", Label: "报告生成日期", Value: model.GeneratedAt.Format("2006-01-02")},
		{ID: "fact:report.overallRiskLevel", Kind: "fact", Label: "整体风险水平", Value: overallLevel},
		{ID: "fact:report.riskCount", Kind: "fact", Label: "已确认风险事项数量", Value: fmt.Sprint(len(model.Findings))},
		{ID: "fact:report.normalAreas", Kind: "fact", Label: "未见明显异常的主要方面", Value: reportNormalAreas(model)},
	}
	for index, warning := range model.Warnings {
		result = append(result, TrustedReference{ID: fmt.Sprintf("fact:report.dataNotice.%d", index+1), Kind: "fact", Label: "数据提示", Value: warning})
	}
	for _, finding := range model.Findings {
		prefix := "finding:" + finding.ID
		result = append(result,
			TrustedReference{ID: prefix + ".title", Kind: "finding", Label: "风险事项", Value: finding.Title},
			TrustedReference{ID: prefix + ".level", Kind: "finding", Label: "风险等级", Value: string(finding.Level)},
			TrustedReference{ID: prefix + ".period", Kind: "finding", Label: "风险期间", Value: finding.Period},
			TrustedReference{ID: prefix + ".evidence", Kind: "finding", Label: "确定性证据", Value: finding.Evidence},
			TrustedReference{ID: prefix + ".recommendation", Kind: "finding", Label: "具体处理动作", Value: finding.Recommendation},
		)
		if finding.Amount != 0 {
			result = append(result, TrustedReference{ID: prefix + ".amount", Kind: "finding", Label: "风险金额", Value: moneyText(finding.Amount)})
		}
	}
	for _, section := range model.Sections {
		if section.DataNotice != "" {
			result = append(result, TrustedReference{ID: "fact:" + section.Key + ".dataNotice", Kind: "fact", Label: "数据限制", Value: section.DataNotice, SectionID: section.Key})
		}
		for blockIndex, block := range section.Blocks {
			for rowIndex, row := range block.Rows {
				for columnIndex, value := range row {
					label := fmt.Sprintf("%s 第%d行第%d列", block.Title, rowIndex+1, columnIndex+1)
					if columnIndex < len(block.Headers) {
						label = block.Title + " " + block.Headers[columnIndex]
					}
					result = append(result, TrustedReference{
						ID:   fmt.Sprintf("fact:%s.b%d.r%d.c%d", section.Key, blockIndex+1, rowIndex+1, columnIndex+1),
						Kind: "fact", Label: label, Value: value, SectionID: section.Key,
					})
				}
			}
			if block.Chart != nil {
				for pointIndex, label := range block.Chart.Labels {
					result = append(result, TrustedReference{
						ID:   fmt.Sprintf("fact:%s.b%d.chart.label.p%d", section.Key, blockIndex+1, pointIndex+1),
						Kind: "fact", Label: block.Chart.Title + " 期间/类别", Value: label, SectionID: section.Key,
					})
				}
				for seriesIndex, series := range block.Chart.Series {
					for pointIndex, value := range series.Values {
						if len(series.Available) > pointIndex && !series.Available[pointIndex] {
							continue
						}
						label := fmt.Sprintf("%s %s", block.Chart.Title, series.Name)
						if pointIndex < len(block.Chart.Labels) {
							label += " " + block.Chart.Labels[pointIndex]
						}
						result = append(result, TrustedReference{
							ID:   fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", section.Key, blockIndex+1, seriesIndex+1, pointIndex+1),
							Kind: "fact", Label: label, Value: deliveryChartValue(*block.Chart, series, value), SectionID: section.Key,
						})
					}
					for pointIndex, point := range block.Chart.Points {
						if !point.Available {
							continue
						}
						result = append(result,
							TrustedReference{ID: fmt.Sprintf("fact:%s.b%d.chart.point%d.x", section.Key, blockIndex+1, pointIndex+1), Kind: "fact", Label: block.Chart.Title + " " + point.Label + " 横轴值", Value: fmt.Sprintf("%.2f", point.X), SectionID: section.Key},
							TrustedReference{ID: fmt.Sprintf("fact:%s.b%d.chart.point%d.y", section.Key, blockIndex+1, pointIndex+1), Kind: "fact", Label: block.Chart.Title + " " + point.Label + " 纵轴值", Value: fmt.Sprintf("%.2f", point.Y), SectionID: section.Key},
						)
					}
				}
			}
		}
	}
	return result
}

// reportNormalAreas returns a deterministic, user-facing summary of clear
// evaluation domains. Parameter model is the frozen report. The return value
// never treats unavailable or inapplicable data as normal.
func reportNormalAreas(model Model) string {
	if value := strings.TrimSpace(clearEvaluationDomains(model.Evaluations)); value != "" && value != "—" {
		return value
	}
	return "当前没有可单独归纳的未见明显异常方面"
}

// analysisTaskMarkdown gives the Agent writing goals and the plain-prose contract.
func analysisTaskMarkdown(manifest WorkspaceManifest, model Model) string {
	var builder strings.Builder
	builder.WriteString("# 财税风险分析填写任务\n\n先用 report-read --next 读取批次，正文JSON通过 report-write --input 合并到 analysis-output.json，保持冻结文件不变。每点 text 是直接交付的自然语言正文，evidenceRefs 独立记录本点使用的来源ID；正文不写引用ID或占位符，不照抄全部证据原文。\n\n")
	fmt.Fprintf(&builder, "## 固定身份\n\n- contractVersion: %s\n- taskId: %s\n- sourceDigest: %s\n- modelDigest: %s\n- analysisDataDigest: %s\n\n", manifest.ContractVersion, manifest.TaskID, manifest.SourceDigest, manifest.ModelDigest, manifest.AnalysisDataDigest)
	builder.WriteString("## 通用分析指引\n\n")
	builder.WriteString(strings.TrimSpace(commonAnalysisGuidance))
	builder.WriteString("\n\n")
	builder.WriteString("## 可填写章节\n\n")
	sections := stringSet(manifest.AnalysisSectionIDs)
	for _, section := range model.Sections {
		if !sections[section.Key] {
			continue
		}
		fmt.Fprintf(&builder, "- %s：%s；类型%s；填写%d—%d条。目标、指标、期间和引用随report-read批次提供。\n", section.Key, section.BaseTitle, section.Narrative.Mode, section.Narrative.MinItems, section.Narrative.MaxItems)
	}
	builder.WriteString("\n## 综合结论\n\n章节完成后由 report-read --next 一次提供结论写作要求、全部已确认风险及动作、数据限制和关键指标。完整来源仍可通过 report-refs 查询。\n")
	builder.WriteString("\n## 完成后\n\n章节写入完成后，report-read --next会提供综合结论所需材料；结论同样用report-write提交。无需编写Python抽取或填充脚本。执行 report-validate。脚本只检查输出结构、引用ID、冻结摘要及文档结构，不核实自然语言分析的正确性。按错误字段及条目位置修正；valid=true后执行report-render，同时生成DOCX/PDF。无需单独填写语义审读文件。正文或引用修改后重新校验。引用清单和分析底稿只保存在工作区，不进入成品。\n")
	return builder.String()
}

// validateWorkspaceIdentity compares task, version and digest identities.
// Parameters are the accumulating result and three workflow objects. It mutates
// result and returns no value.
func validateWorkspaceIdentity(result *ValidationResult, manifest WorkspaceManifest, source taxanalysis.TaskResponse, model Model, analysis AnalysisOutput) {
	if manifest.ContractVersion != AIContractVersion {
		result.ContentErrors = append(result.ContentErrors, "manifest.json contractVersion 与当前AI契约不一致")
	}
	if manifest.RuleVersion != RuleVersion || model.RuleVersion != RuleVersion {
		result.ContentErrors = append(result.ContentErrors, "规则版本与当前执行器不一致")
	}
	if manifest.SourceDigest != result.SourceDigest {
		result.ContentErrors = append(result.ContentErrors, "source-snapshot.json 摘要与manifest不一致")
	}
	if manifest.ModelDigest != result.ModelDigest {
		result.ContentErrors = append(result.ContentErrors, "report-model.json 摘要与manifest不一致")
	}
	if analysis.ContractVersion != AIContractVersion {
		result.ContentErrors = append(result.ContentErrors, "analysis-output.json contractVersion错误")
	}
	if analysis.TaskID != manifest.TaskID || model.TaskID != manifest.TaskID {
		result.ContentErrors = append(result.ContentErrors, "taskId与准备阶段任务不一致")
	}
	if analysis.SourceDigest != manifest.SourceDigest {
		result.ContentErrors = append(result.ContentErrors, "analysis-output.json sourceDigest错误")
	}
	if analysis.ModelDigest != manifest.ModelDigest {
		result.ContentErrors = append(result.ContentErrors, "analysis-output.json modelDigest错误")
	}
	if source.TaskID != manifest.TaskID || source.USCC != manifest.USCC || source.StartTime != manifest.StartTime || source.EndTime != manifest.EndTime {
		result.ContentErrors = append(result.ContentErrors, "源任务与manifest中的任务、企业或期间不一致")
	}
	if manifest.DataStatus != string(source.Status) {
		result.ContentErrors = append(result.ContentErrors, "manifest中的dataStatus与源任务不一致")
	}
	if model.USCC != manifest.USCC || model.CompanyName != manifest.CompanyName || model.StartTime != manifest.StartTime || model.EndTime != manifest.EndTime || !model.GeneratedAt.Equal(manifest.GeneratedAt) {
		result.ContentErrors = append(result.ContentErrors, "企业或分析期间与manifest不一致")
	}
	if !equalStringSlices(manifest.SectionIDs, modelSemanticIDs(model)) {
		result.ContentErrors = append(result.ContentErrors, "manifest中的sectionIds与冻结模型不一致")
	}
	if !equalStringSlices(manifest.AnalysisSectionIDs, analysisRequiredSectionIDs(model)) {
		result.ContentErrors = append(result.ContentErrors, "manifest中的analysisSectionIds与冻结模型不一致")
	}
	if !equalStringSlices(manifest.FindingIDs, findingIDs(model.Findings)) {
		result.ContentErrors = append(result.ContentErrors, "manifest中的findingIds与冻结模型不一致")
	}
}

// validateAnalysisShape checks completeness, uniqueness and hierarchy.
// Parameters are the result, manifest, model and Agent output. It mutates the
// validation result and returns no value.
func validateAnalysisShape(result *ValidationResult, manifest WorkspaceManifest, model Model, analysis AnalysisOutput) {
	if len(analysis.Conclusion.Items) < 3 || len(analysis.Conclusion.Items) > 5 {
		result.ContentErrors = append(result.ContentErrors, "conclusion.items必须为3—5个有证据的重点")
	}
	for i, item := range analysis.Conclusion.Items {
		validateRequiredNarrativeItem(result, fmt.Sprintf("conclusion.items[%d]", i), item)
	}
	expectedOrder := modelSemanticIDs(model)
	if !sameUniqueStringSet(expectedOrder, analysis.SectionOrder) {
		result.ContentErrors = append(result.ContentErrors, "sectionOrder必须包含全部可用semanticId且各出现一次")
	} else if err := validateHierarchicalSectionOrder(model, analysis.SectionOrder); err != nil {
		result.ContentErrors = append(result.ContentErrors, err.Error())
	}
	expectedSections := stringSet(manifest.AnalysisSectionIDs)
	sectionPolicies := map[string]NarrativePolicy{}
	for _, section := range model.Sections {
		sectionPolicies[section.Key] = section.Narrative
	}
	seen := map[string]bool{}
	for index, section := range analysis.Sections {
		path := fmt.Sprintf("sections[%d;sectionId=%s]", index, section.SectionID)
		if !expectedSections[section.SectionID] {
			result.ContentErrors = append(result.ContentErrors, path+"包含未知或无需AI分析的sectionId")
			continue
		}
		if seen[section.SectionID] {
			result.ContentErrors = append(result.ContentErrors, path+"重复填写sectionId "+section.SectionID)
		}
		seen[section.SectionID] = true
		policy := sectionPolicies[section.SectionID]
		if len(section.Items) < policy.MinItems || len(section.Items) > policy.MaxItems {
			result.ContentErrors = append(result.ContentErrors, fmt.Sprintf("%s.items数量必须为%d—%d条", path, policy.MinItems, policy.MaxItems))
		}
		for itemIndex, item := range section.Items {
			validateRequiredNarrativeItem(result, fmt.Sprintf("%s.items[%d]", path, itemIndex), item)
		}
	}
	for sectionID := range expectedSections {
		if !seen[sectionID] {
			result.MissingPaths = append(result.MissingPaths, "sections[sectionId="+sectionID+"]")
		}
	}
}

// validateRequiredNarrativeItem checks one required Agent item. Parameters are
// the accumulating result, JSON path, and item. It appends missing paths only.
func validateRequiredNarrativeItem(result *ValidationResult, path string, item NarrativeItem) {
	if strings.TrimSpace(item.Text) == "" {
		result.MissingPaths = append(result.MissingPaths, path+".text")
	}
	if len(item.EvidenceRefs) == 0 {
		result.MissingPaths = append(result.MissingPaths, path+".evidenceRefs")
	}
}

// validateHierarchicalSectionOrder preserves chapter membership.
func validateHierarchicalSectionOrder(model Model, order []string) error {
	sections := map[string]Section{}
	parent := map[string]string{}
	currentChapter := ""
	for _, section := range model.Sections {
		sections[section.Key] = section
		if section.Level == 1 {
			currentChapter = section.Key
		} else {
			parent[section.Key] = currentChapter
		}
	}
	activeChapter := ""
	closed := map[string]bool{}
	for _, key := range order {
		section := sections[key]
		if section.Level == 1 {
			if activeChapter != "" {
				closed[activeChapter] = true
			}
			if closed[key] {
				return errors.New("sectionOrder中的章节被拆分")
			}
			activeChapter = key
			continue
		}
		if activeChapter == "" || parent[key] != activeChapter {
			return fmt.Errorf("sectionOrder中的子节%s未紧随所属章节", key)
		}
	}
	return nil
}

// applyValidatedAnalysis appends plain Agent text without expanding references.
// Parameters are the frozen model and validated output. Returns the single
// final model used by both renderers.
func applyValidatedAnalysis(model Model, analysis AnalysisOutput) (Model, error) {
	analysisBySection := map[string]AnalysisSectionOutput{}
	for _, section := range analysis.Sections {
		analysisBySection[section.SectionID] = section
	}
	ordered := make([]Section, 0, len(model.Sections))
	sectionByID := map[string]Section{}
	for _, section := range model.Sections {
		sectionByID[section.Key] = section
	}
	chapterNumber, childIndex := "", 0
	for _, sectionID := range analysis.SectionOrder {
		section := sectionByID[sectionID]
		if section.Level == 1 {
			chapterNumber = section.ID
			childIndex = 0
			section.DisplayNumber = chapterNumber
			section.Title = chineseChapterTitle(section.DisplayNumber, section.BaseTitle)
		} else {
			childIndex++
			section.DisplayNumber = fmt.Sprintf("%s.%d", chapterNumber, childIndex)
			section.Title = section.DisplayNumber + " " + section.BaseTitle
		}
		if section.Key == "chapter.conclusion" {
			conclusionItems := analysis.Conclusion.Items
			paragraphs := make([]string, 0, len(conclusionItems))
			for itemIndex, conclusion := range conclusionItems {
				value := conclusion.Text
				paragraphs = append(paragraphs, fmt.Sprintf("%s %s", chineseCircledNumber(itemIndex+1), value))
			}
			section.Blocks = []Block{{Kind: BlockParagraph, Title: "综合结论", Paragraphs: paragraphs}}
		}
		if output, ok := analysisBySection[section.Key]; ok && len(output.Items) > 0 {
			paragraphs := make([]string, 0, len(output.Items))
			for itemIndex, item := range output.Items {
				value := item.Text
				if len(output.Items) > 1 || section.Narrative.Mode == NarrativeRisk || section.Narrative.Mode == NarrativeAttention {
					value = chineseCircledNumber(itemIndex+1) + " " + value
				}
				paragraphs = append(paragraphs, value)
			}
			if title := sectionNarrativeTitle(section.Narrative.Mode); title != "" {
				section.Blocks = append(section.Blocks, Block{Kind: BlockParagraph, Title: title, Paragraphs: paragraphs})
			}
		}
		if section.Narrative.Mode == NarrativeRisk && !strings.HasPrefix(section.Key, "risk.") && !strings.HasPrefix(section.Key, "remediation.") {
			if advice := sectionRiskAdvice(model, section); len(advice) > 0 {
				section.Blocks = append(section.Blocks, Block{Kind: BlockParagraph, Title: "整改建议", Paragraphs: advice})
			}
		}
		ordered = append(ordered, section)
	}
	model.Sections = ordered
	return model, nil
}

// sectionRiskAdvice returns de-duplicated script-owned advice for confirmed
// findings mapped to one section's rule IDs. Parameters are the final model and
// section. Return value is empty for non-risk sections or missing advice.
func sectionRiskAdvice(model Model, section Section) []string {
	values := make([]string, 0)
	for _, finding := range sectionConfirmedFindings(section, model.Evaluations) {
		if value := strings.TrimSpace(finding.Recommendation); value != "" {
			values = append(values, finding.Title+"："+value)
		}
	}
	if len(values) > 3 {
		values = values[:3]
	}
	for i := range values {
		values[i] = chineseCircledNumber(i+1) + " " + values[i]
	}
	return values
}

// chineseCircledNumber returns the fixed marker used for short multi-point
// analysis. Parameter value is one-based. Return value falls back to a Chinese
// parenthesized number outside the supported product range.
func chineseCircledNumber(value int) string {
	markers := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}
	if value >= 1 && value <= len(markers) {
		return markers[value-1]
	}
	return fmt.Sprintf("（%d）", value)
}

// writeValidationResult stores the latest public validation evidence.
// Parameters are the workspace and result. Return value is a write error.
func writeValidationResult(workspace string, result ValidationResult) error {
	return writeWorkspaceJSON(filepath.Join(workspace, workspaceValidationFilename), result)
}

// stringSet converts strings into membership keys. Parameter values may contain
// duplicates. Return value contains each distinct string once.
func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

// sameUniqueStringSet compares two slices as unique sets. Parameters are the
// expected and actual values. Return value is false for duplicates.
func sameUniqueStringSet(expected, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	seen := map[string]bool{}
	expectedSet := stringSet(expected)
	for _, value := range actual {
		if seen[value] || !expectedSet[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

// equalStringSlices compares two ordered string lists. Parameters are the two
// lists to compare. Return value is true only when lengths and every item match.
func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// uniqueSortedStrings deduplicates diagnostics for stable stdout. Parameter
// values is not modified. Return value is sorted.
func uniqueSortedStrings(values []string) []string {
	set := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			set[value] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
