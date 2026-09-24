package batch

import (
	"context"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/config"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoicecore"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

var loadRuntimeConfig = config.LoadRuntime

var verifierOverrideFactory func() invoicecore.Verifier

type VerifyOptions struct {
	InputPath string
	Output    string
	APIKey    string
	Now       func() time.Time
	Verifier  invoicecore.Verifier
}

func Verify(ctx context.Context, options VerifyOptions) (contract.BatchVerifySuccess, error) {
	now := resolveNow(options.Now)
	outputPath, err := resolveVerifyOutput(options.InputPath, options.Output)
	if err != nil {
		return contract.BatchVerifySuccess{}, err
	}
	verifier := options.Verifier
	if verifier == nil && verifierOverrideFactory != nil {
		verifier = verifierOverrideFactory()
	}
	if verifier == nil {
		verifier, err = verifierForAPIKey(options.APIKey)
		if err != nil {
			return contract.BatchVerifySuccess{}, err
		}
	}

	result, err := invoicecore.Process(ctx, options.InputPath, outputPath, verifier, invoicecore.Options{Now: now})
	if err != nil {
		return contract.BatchVerifySuccess{}, err
	}
	rows := make([]map[string]any, 0, len(result.Records))
	for _, record := range result.Records {
		row := make(map[string]any, len(excelx.IntermediateVerifyHeaders)+1)
		for key, value := range record.RowMap() {
			row[key] = value
		}
		row[excelx.SourcePathKey] = record.FilePath
		rows = append(rows, row)
	}
	actualPath, err := excelx.WriteWorkbook(result.Summary.Data.IntermediateExcelPath, excelx.WorkbookSpec{
		SheetName:     excelx.DefaultSheetName,
		Headers:       excelx.IntermediateVerifyHeaders,
		Rows:          rows,
		AmountHeaders: excelx.IntermediateAmountHeaders,
		Widths:        excelx.IntermediateWidths,
		Hyperlinks:    true,
	}, now())
	if err != nil {
		return contract.BatchVerifySuccess{}, err
	}
	result.Summary.Data.ExcelFilePath = actualPath
	result.Summary.Data.IntermediateExcelPath = actualPath
	return result.Summary, nil
}

func resolveVerifyOutput(inputPath, output string) (string, error) {
	scan, err := invoiceinput.Scan(inputPath, invoiceinput.Options{})
	if err != nil {
		return "", err
	}
	defaultDir := scan.InputPath
	if scan.Mode == invoiceinput.ModeList {
		defaultDir = pathDir(scan.InputPath)
	}
	return pathx.ResolveOutput(output, defaultDir, invoiceinput.IntermediateResultFilename, ".xlsx")
}

func verifierForAPIKey(apiKey string) (invoicecore.Verifier, error) {
	cfg, err := loadRuntimeConfig()
	if err != nil {
		return nil, err
	}
	return verifyapi.NewClientFromConfig(apiKey, cfg)
}

func resolveNow(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return time.Now
}
