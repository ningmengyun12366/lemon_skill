package report

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	taxanalysis "invoice-assistant-skill/go-script/lib/tax-analysis"
)

const defaultSupplementalRetries = 2

type TaskRunner interface {
	Run(context.Context, taxanalysis.CreateTaskRequest, time.Duration) (taxanalysis.TaskResponse, error)
}

type GenerateOptions struct {
	OutputDir           string
	FontDir             string
	ChartRendererPath   string
	chartRenderer       func(Model, FontFiles) (map[string][]byte, error)
	PollInterval        time.Duration
	SupplementalRetries int
	Now                 time.Time
}

// obtainReportTask runs the existing create-and-poll flow and at most two
// supplemental queries against the same deterministic task. Parameters are the
// remote runner, bound-enterprise request, polling interval, and retry count.
// Return value is a reportable terminal task or a data-gate error.
func obtainReportTask(ctx context.Context, runner TaskRunner, request taxanalysis.CreateTaskRequest, pollInterval time.Duration, supplementalRetries int) (taxanalysis.TaskResponse, error) {
	if runner == nil {
		return taxanalysis.TaskResponse{}, errors.New("tax analysis task runner is required")
	}
	retries := supplementalRetries
	if retries <= 0 {
		retries = defaultSupplementalRetries
	}
	var task taxanalysis.TaskResponse
	var firstTaskID string
	for attempt := 0; attempt <= retries; attempt++ {
		attemptRequest := request
		if attempt > 0 {
			attemptRequest.ForceRefresh = false
		}
		var err error
		task, err = runner.Run(ctx, attemptRequest, pollInterval)
		if err != nil {
			return taxanalysis.TaskResponse{}, fmt.Errorf("obtain tax analysis task: %w", err)
		}
		if firstTaskID == "" {
			firstTaskID = task.TaskID
		} else if task.TaskID != firstTaskID {
			return taxanalysis.TaskResponse{}, errors.New("supplemental query returned a different taskId")
		}
		if task.Status == taxanalysis.StatusInterrupted {
			reason := "执行已中断，数据尚未采集完成"
			if task.ExecutionError != nil && task.ExecutionError.Message != "" {
				reason = task.ExecutionError.Message
			}
			return task, fmt.Errorf("税务分析任务 %s 已中断：%s；保留原任务及部分结果，恢复后再准备报告", task.TaskID, reason)
		}
		if reportDataComplete(task) {
			return task, nil
		}
		if attempt == retries {
			if reportDataReady(task) {
				return task, nil
			}
			return taxanalysis.TaskResponse{}, incompleteDataError(task)
		}
	}
	return taxanalysis.TaskResponse{}, errors.New("tax analysis task did not reach a reportable state")
}

// renderModelPair renders and atomically commits the final DOCX/PDF pair from
// one already validated model. Parameters: ctx controls the JS renderer, model
// is the finalized shared content, dataStatus is the source task status, and
// options configure output/fonts/runtime. Returns both output paths.
func renderModelPair(ctx context.Context, model Model, dataStatus string, options GenerateOptions) (Result, error) {
	timings := map[string]int64{}
	stage := time.Now()
	if problems := validateDeliveryContent(model); len(problems) > 0 {
		return Result{}, fmt.Errorf("报告交付内容校验失败：%s", strings.Join(problems, "；"))
	}
	outputDir := strings.TrimSpace(options.OutputDir)
	if outputDir == "" {
		return Result{}, errors.New("output directory is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}
	outputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return Result{}, err
	}
	fonts, err := LoadFonts(options.FontDir)
	if err != nil {
		return Result{}, err
	}
	var charts map[string][]byte
	if options.chartRenderer != nil {
		charts, err = options.chartRenderer(model, fonts)
	} else {
		rendererPath, resolveErr := ResolveChartRendererPath(fonts.Directory, options.ChartRendererPath)
		if resolveErr != nil {
			return Result{}, resolveErr
		}
		charts, err = RenderCharts(ctx, model, fonts, rendererPath)
	}
	if err != nil {
		return Result{}, fmt.Errorf("render report charts: %w", err)
	}
	timings["chartsAndAssets"] = time.Since(stage).Milliseconds()
	now := options.Now
	if now.IsZero() {
		now = model.GeneratedAt
	}
	if now.IsZero() {
		now = time.Now()
	}
	docxPath, pdfPath, err := availableOutputPair(outputDir, model.CompanyName, now)
	if err != nil {
		return Result{}, err
	}
	tempDOCX, err := reserveTemporaryPath(outputDir, ".tax-analysis-*.docx")
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(tempDOCX)
	tempPDF, err := reserveTemporaryPath(outputDir, ".tax-analysis-*.pdf")
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(tempPDF)
	stage = time.Now()
	sectionPages, err := layoutPDFSectionPages(model, charts, fonts)
	if err != nil {
		return Result{}, fmt.Errorf("layout report pages: %w", err)
	}
	timings["pageLayout"] = time.Since(stage).Milliseconds()
	stage = time.Now()
	if err := writeDOCXWithPageRefs(tempDOCX, model, charts, fonts, logicalSectionPages(sectionPages)); err != nil {
		return Result{}, fmt.Errorf("generate DOCX: %w", err)
	}
	timings["docx"] = time.Since(stage).Milliseconds()
	stage = time.Now()
	if err := writePDFWithSectionPages(tempPDF, model, charts, fonts, sectionPages); err != nil {
		return Result{}, fmt.Errorf("generate PDF: %w", err)
	}
	timings["pdf"] = time.Since(stage).Milliseconds()
	if err := commitOutputPair(tempDOCX, tempPDF, docxPath, pdfPath); err != nil {
		return Result{}, err
	}
	included := model.SectionIDs()
	return Result{
		Timings: timings,
		Status:  "success", TaskID: model.TaskID, DataStatus: dataStatus, RuleVersion: model.RuleVersion,
		DOCXPath: docxPath, PDFPath: pdfPath, IncludedSections: included,
		OmittedSections: omittedSectionIDs(included), Warnings: append([]string(nil), model.Warnings...),
	}, nil
}

// reportDataComplete reports whether all 17 modules and the task have no
// failures. Parameters: task is the terminal OpenTax response. Return value:
// true only when no supplemental or degraded handling is required.
func reportDataComplete(task taxanalysis.TaskResponse) bool {
	if task.Status != taxanalysis.StatusSuccess || len(task.FailedItems) > 0 {
		return false
	}
	byCode := moduleResultsByCode(task.ModuleResults)
	for _, required := range requiredModules {
		module, ok := byCode[required.Code]
		if !ok || module.Status != taxanalysis.ModuleStatusSuccess || len(module.FailedItems) > 0 {
			return false
		}
	}
	return true
}

// reportDataReady reports whether a terminal task is safe to render after all
// supplemental retries. Parameters: task is the terminal OpenTax response.
// Return value is true when only arrears/refunds, exact empty tax-source results,
// or a confirmed small taxpayer's deduction-feature rejection are degraded;
// every other module must be complete.
func reportDataReady(task taxanalysis.TaskResponse) bool {
	if task.Status != taxanalysis.StatusSuccess && task.Status != taxanalysis.StatusPartialSuccess {
		return false
	}
	taxpayer := taxpayerClassificationFromTask(task)
	for _, failedItem := range task.FailedItems {
		moduleCode := strings.TrimSpace(failedItem.ModuleCode)
		if moduleCode == degradableModuleCode {
			continue
		}
		if emptyTaxSourceModuleCodes[moduleCode] && strings.TrimSpace(failedItem.Message) == emptyTaxSourceMessage {
			continue
		}
		if moduleCode == deductionConfirmationModuleCode && taxpayer == taxpayerSmall && strings.Contains(strings.TrimSpace(failedItem.Message), smallTaxpayerAuthorityMessageFragment) {
			continue
		}
		return false
	}
	byCode := moduleResultsByCode(task.ModuleResults)
	for _, required := range requiredModules {
		module, ok := byCode[required.Code]
		if !ok {
			return false
		}
		if !moduleUsableForReportForTaxpayer(module, taxpayer) {
			return false
		}
	}
	return true
}

// moduleResultsByCode indexes module results by stable module code.
// Parameters: modules is the OpenTax module result list. Return value: a map
// whose keys are module codes; later duplicate entries replace earlier ones.
func moduleResultsByCode(modules []taxanalysis.ModuleResult) map[string]taxanalysis.ModuleResult {
	byCode := make(map[string]taxanalysis.ModuleResult, len(modules))
	for _, module := range modules {
		byCode[module.Code] = module
	}
	return byCode
}

func incompleteDataError(task taxanalysis.TaskResponse) error {
	problems := make([]string, 0)
	byCode := moduleResultsByCode(task.ModuleResults)
	for _, required := range requiredModules {
		module, ok := byCode[required.Code]
		if !ok {
			problems = append(problems, required.Code+":missing")
			continue
		}
		if module.Status != taxanalysis.ModuleStatusSuccess {
			problems = append(problems, required.Code+":"+string(module.Status))
		} else if len(module.FailedItems) > 0 {
			problems = append(problems, required.Code+":failed_items")
		}
	}
	if len(task.FailedItems) > 0 && len(problems) == 0 {
		problems = append(problems, "task:failed_items")
	}
	sort.Strings(problems)
	return fmt.Errorf("报告生成条件未满足，两次补充重查后仍有失败数据：%s", strings.Join(problems, ", "))
}

func availableOutputPair(outputDir, companyName string, now time.Time) (string, string, error) {
	company := sanitizeFilename(companyName)
	if company == "" {
		company = "企业"
	}
	base := fmt.Sprintf("%s_财税风险分析报告_%s", company, now.Format("20060102"))
	for suffix := 0; suffix < 10_000; suffix++ {
		name := base
		if suffix > 0 {
			name = fmt.Sprintf("%s(%d)", base, suffix+1)
		}
		docxPath := filepath.Join(outputDir, name+".docx")
		pdfPath := filepath.Join(outputDir, name+".pdf")
		docxExists, err := pathExists(docxPath)
		if err != nil {
			return "", "", err
		}
		pdfExists, err := pathExists(pdfPath)
		if err != nil {
			return "", "", err
		}
		if !docxExists && !pdfExists {
			return docxPath, pdfPath, nil
		}
	}
	return "", "", errors.New("cannot allocate a unique report filename")
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func reserveTemporaryPath(directory, pattern string) (string, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func commitOutputPair(tempDOCX, tempPDF, docxPath, pdfPath string) error {
	if err := renameNoReplace(tempDOCX, docxPath); err != nil {
		return fmt.Errorf("commit DOCX output: %w", err)
	}
	if err := renameNoReplace(tempPDF, pdfPath); err != nil {
		_ = os.Remove(docxPath)
		return fmt.Errorf("commit PDF output: %w", err)
	}
	return nil
}

func sanitizeFilename(value string) string {
	replacer := strings.NewReplacer("<", "_", ">", "_", ":", "_", "\"", "_", "/", "_", "\\", "_", "|", "_", "?", "_", "*", "_")
	value = strings.Trim(replacer.Replace(strings.TrimSpace(value)), " .")
	if value == "" {
		return ""
	}
	if utf8.RuneCountInString(value) > 80 {
		runes := []rune(value)
		value = string(runes[:80])
	}
	upper := strings.ToUpper(value)
	reserved := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true, "LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}
	if reserved[upper] {
		value = "_" + value
	}
	return value
}

func omittedSectionIDs(included []string) []string {
	seen := make(map[string]bool, len(included))
	for _, id := range included {
		seen[id] = true
	}
	omitted := make([]string, 0)
	for _, id := range potentialSectionIDs() {
		if !seen[id] {
			omitted = append(omitted, id)
		}
	}
	return omitted
}
