package app

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/fetchrisk"
)

var convertMarkdownToPDF = common.ToPDF

func runMarkdownToPDF(args []string) error {
	fs := flag.NewFlagSet("markdown-to-pdf", flag.ContinueOnError)
	input := fs.String("input", "", "markdown input file")
	output := fs.String("output", "", "pdf output file")
	taskFile := fs.String("task-file", "", "task JSON file for risk artifact cleanup")
	title := fs.String("title", "", "pdf title")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *input == "" && len(rest) > 0 {
		*input = rest[0]
	}
	if *output == "" && len(rest) > 1 {
		*output = rest[1]
	}
	if *input == "" || *output == "" {
		return fmt.Errorf("missing required --input or --output")
	}
	_ = title
	reportBody, err := os.ReadFile(*input)
	if err != nil {
		return fmt.Errorf("read markdown input %s: %w", *input, err)
	}
	if err := convertMarkdownToPDF(*input, *output); err != nil {
		return err
	}
	analysisExcelPathText := ""
	if *taskFile != "" {
		state, err := fetchrisk.LoadTaskState(*taskFile)
		if err != nil {
			return fmt.Errorf("load task state for final delivery: %w", err)
		}
		outputs := common.ObjectValue(state.Data, "outputs")
		analysisExcelPathText = common.FormatLocalPathText(common.StringValue(outputs["analysis_excel_path"]))
		if err := fetchrisk.FinalizeRiskArtifacts(*taskFile, *input, *output); err != nil {
			return err
		}
	}
	reportPDFPath, err := filepath.Abs(*output)
	if err != nil {
		return fmt.Errorf("resolve PDF output %s: %w", *output, err)
	}
	reportPDFPath = filepath.Clean(reportPDFPath)
	reportPDFInfo, err := os.Stat(reportPDFPath)
	if err != nil {
		return fmt.Errorf("stat PDF output %s: %w", reportPDFPath, err)
	}
	reportPDFPathText := common.FormatLocalPathText(reportPDFPath)
	artifactRootText := common.FormatLocalPathText(filepath.Dir(reportPDFPath))
	result := map[string]any{
		"status":            "success",
		"report_pdf_path":   reportPDFPathText,
		"artifact_root":     artifactRootText,
		"report_body":       string(reportBody),
		"report_size_bytes": reportPDFInfo.Size(),
	}
	if analysisExcelPathText != "" {
		result["analysis_excel_path"] = analysisExcelPathText
	}
	return printJSON(result)
}
