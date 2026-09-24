package fetchrisk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func FinalizeRiskArtifacts(taskFile, reportMD, reportPDF string) error {
	if strings.TrimSpace(taskFile) == "" {
		return fmt.Errorf("missing required --task-file")
	}
	if strings.TrimSpace(reportMD) == "" {
		return fmt.Errorf("missing required --report-md")
	}
	if strings.TrimSpace(reportPDF) == "" {
		return fmt.Errorf("missing required --report-pdf")
	}
	reportPDFAbs, err := filepath.Abs(strings.TrimSpace(reportPDF))
	if err != nil {
		return fmt.Errorf("resolve report PDF %s: %w", reportPDF, err)
	}
	reportPDFAbs = filepath.Clean(reportPDFAbs)
	pdfInfo, err := os.Stat(reportPDFAbs)
	if err != nil {
		return fmt.Errorf("report PDF %s is not available: %w", reportPDFAbs, err)
	}
	if pdfInfo.IsDir() {
		return fmt.Errorf("report PDF %s is not a file", reportPDFAbs)
	}

	state, err := LoadTaskState(taskFile)
	if err != nil {
		return fmt.Errorf("load task state: %w", err)
	}
	outputs := common.ObjectValue(state.Data, "outputs")
	artifacts := common.ObjectValue(state.Data, "artifacts")
	reportMarkdownPaths := uniqueNonEmptyPaths(
		reportMD,
		common.StringValue(outputs["risk_report_markdown_path"]),
		common.StringValue(outputs["risk_report_md_path"]),
		common.StringValue(outputs["report_markdown_path"]),
		common.StringValue(outputs["report_md_path"]),
		common.StringValue(artifacts["risk_report_markdown_path"]),
		common.StringValue(artifacts["risk_report_md_path"]),
		common.StringValue(artifacts["report_markdown_path"]),
		common.StringValue(artifacts["report_md_path"]),
	)
	contextJSONPaths := uniqueNonEmptyPaths(
		common.StringValue(outputs["invoice_context_data_path"]),
		common.StringValue(artifacts["invoice_context_data_path"]),
	)
	summaryMarkdownPaths := uniqueNonEmptyPaths(
		common.StringValue(outputs["summary_markdown_path"]),
		common.StringValue(artifacts["summary_markdown_path"]),
	)
	deductionJSONPaths := uniqueNonEmptyPaths(
		common.StringValue(outputs["deduction_data_file"]),
		common.StringValue(artifacts["deduction_data_file"]),
	)
	deletionPaths := append([]string{}, reportMarkdownPaths...)
	deletionPaths = append(deletionPaths, contextJSONPaths...)
	deletionPaths = append(deletionPaths, summaryMarkdownPaths...)
	deletionPaths = append(deletionPaths, deductionJSONPaths...)
	allowedRoots, err := finalizerDeletionRoots(state, reportMD, reportPDFAbs)
	if err != nil {
		return err
	}
	if err := validateFinalizerDeletionPaths(deletionPaths, allowedRoots); err != nil {
		return err
	}
	reportBody, err := os.ReadFile(reportMD)
	if err != nil {
		return fmt.Errorf("read risk report markdown %s before cleanup: %w", reportMD, err)
	}

	for _, path := range reportMarkdownPaths {
		if err := removeFileIfExists(path); err != nil {
			return fmt.Errorf("remove risk report markdown %s: %w", path, err)
		}
	}
	for _, path := range contextJSONPaths {
		if err := removeFileIfExists(path); err != nil {
			return fmt.Errorf("remove invoice context data %s: %w", path, err)
		}
	}
	for _, path := range summaryMarkdownPaths {
		if err := removeFileIfExists(path); err != nil {
			return fmt.Errorf("remove invoice summary markdown %s: %w", path, err)
		}
	}
	for _, path := range deductionJSONPaths {
		if err := removeFileIfExists(path); err != nil {
			return fmt.Errorf("remove deduction data %s: %w", path, err)
		}
	}

	deleteFinalizedIntermediateRefs(outputs)
	deleteFinalizedIntermediateRefs(artifacts)
	outputs["risk_report_pdf_path"] = reportPDFAbs
	outputs["risk_report_body"] = string(reportBody)
	markdownToPDF := common.ObjectValue(common.ObjectValue(state.Data, "execution"), "markdown_to_pdf")
	markdownToPDF["status"] = "success"
	markdownToPDF["report_pdf_path"] = reportPDFAbs
	markdownToPDF["artifact_root"] = filepath.Dir(reportPDFAbs)
	markdownToPDF["report_size_bytes"] = pdfInfo.Size()
	delete(markdownToPDF, "last_error")
	markPlannedStepStatus(state.Data, riskAnalysisStep, "success", "")
	if err := state.Save(); err != nil {
		return fmt.Errorf("save finalized task state: %w", err)
	}
	return nil
}

type finalizerDeletionRoot struct {
	path     string
	realPath string
}

func finalizerDeletionRoots(state *TaskState, reportMD, reportPDF string) ([]finalizerDeletionRoot, error) {
	storage := common.ObjectValue(state.Data, "storage")
	if root := strings.TrimSpace(common.StringValue(storage["root_dir"])); root != "" {
		return canonicalFinalizerDeletionRoots([]string{root})
	}
	return canonicalFinalizerDeletionRoots([]string{
		filepath.Dir(state.Path),
		filepath.Dir(reportPDF),
		filepath.Dir(reportMD),
	})
}

func canonicalFinalizerDeletionRoots(paths []string) ([]finalizerDeletionRoot, error) {
	roots := []finalizerDeletionRoot{}
	seen := map[string]struct{}{}
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" || trimmed == "." {
			continue
		}
		absPath, err := filepath.Abs(trimmed)
		if err != nil {
			return nil, fmt.Errorf("resolve allowed artifact root %s: %w", trimmed, err)
		}
		absPath = filepath.Clean(absPath)
		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}
		realPath := absPath
		if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
			realPath = filepath.Clean(resolved)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("resolve allowed artifact root %s: %w", absPath, err)
		}
		roots = append(roots, finalizerDeletionRoot{path: absPath, realPath: realPath})
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no allowed artifact roots available")
	}
	return roots, nil
}

func validateFinalizerDeletionPaths(paths []string, roots []finalizerDeletionRoot) error {
	for _, path := range paths {
		if err := validateFinalizerDeletionPath(path, roots); err != nil {
			return err
		}
	}
	return nil
}

func validateFinalizerDeletionPath(path string, roots []finalizerDeletionRoot) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return fmt.Errorf("resolve deletion candidate %s: %w", trimmed, err)
	}
	absPath = filepath.Clean(absPath)
	if !pathWithinAnyFinalizerRoot(absPath, roots, func(root finalizerDeletionRoot) string { return root.path }) {
		return fmt.Errorf("refusing to delete %s: outside allowed artifact roots", trimmed)
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("resolve deletion candidate %s: %w", trimmed, err)
	}
	if !pathWithinAnyFinalizerRoot(filepath.Clean(realPath), roots, func(root finalizerDeletionRoot) string { return root.realPath }) {
		return fmt.Errorf("refusing to delete %s: outside allowed artifact roots", trimmed)
	}
	return nil
}

func pathWithinAnyFinalizerRoot(path string, roots []finalizerDeletionRoot, rootPath func(finalizerDeletionRoot) string) bool {
	for _, root := range roots {
		if pathWithinFinalizerRoot(path, rootPath(root)) {
			return true
		}
	}
	return false
}

func pathWithinFinalizerRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}

func uniqueNonEmptyPaths(values ...string) []string {
	seen := map[string]struct{}{}
	paths := []string{}
	for _, value := range values {
		path := strings.TrimSpace(value)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func deleteFinalizedIntermediateRefs(taskMap map[string]any) {
	delete(taskMap, "invoice_context_data_path")
	delete(taskMap, "invoice_context_data")
	delete(taskMap, "risk_context_summary")
	delete(taskMap, "risk_report_markdown_path")
	delete(taskMap, "risk_report_md_path")
	delete(taskMap, "report_markdown_path")
	delete(taskMap, "report_md_path")
	delete(taskMap, "summary_markdown_path")
	delete(taskMap, "summary_markdown")
	delete(taskMap, "deduction_data_file")
	delete(taskMap, "deduction_data_list")
	delete(taskMap, "deduction_markdown")
}
