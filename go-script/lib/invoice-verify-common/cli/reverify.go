package cli

import (
	"context"
	"fmt"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/config"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoicecore"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

type ReverifyOptions struct {
	InputPath string
	Output    string
	APIKey    string
}

func RunReverify(ctx context.Context, options ReverifyOptions) (contract.ReverifySuccess, error) {
	inputPath, err := pathx.Normalize(options.InputPath)
	if err != nil {
		return contract.ReverifySuccess{}, err
	}
	if _, err := invoicecore.ResolveReverifyOutputPath(inputPath, options.Output, time.Now()); err != nil {
		return contract.ReverifySuccess{}, err
	}
	verifier, err := reverifyVerifierForAPIKey(options.APIKey)
	if err != nil {
		return contract.ReverifySuccess{}, err
	}
	result, err := invoicecore.Reverify(ctx, invoicecore.ReverifyOptions{
		InputPath: inputPath,
		Output:    options.Output,
		Verifier:  verifier,
	})
	if err != nil {
		return contract.ReverifySuccess{}, err
	}
	return ReverifySuccessFromResult(result), nil
}

func ReverifySuccessFromResult(result invoicecore.ReverifyResult) contract.ReverifySuccess {
	counts := result.Counts
	return contract.ReverifySuccess{
		Status:  contract.StatusSuccess,
		Message: "reverify completed",
		Data: contract.ReverifyData{
			Mode:                  "reverify",
			InputExcelPath:        result.InputPath,
			ExcelFilePath:         result.OutputPath,
			IntermediateExcelPath: result.OutputPath,
			OutputFile:            result.OutputPath,
			TotalFileCount:        counts.TotalFileCount,
			RetryCandidateCount:   counts.RetryCandidateCount,
			RetriedCount:          counts.RetriedCount,
			UnretryableCount:      counts.UnretryableCount,
			UnsupportedCount:      counts.UnsupportedCount,
			PreservedSuccessCount: counts.PreservedSuccessCount,
			VerifySuccessCount:    counts.VerifySuccessCount,
			VerifyFailedCount:     counts.VerifyFailedCount,
			SkippedCount:          counts.SkippedCount,
		},
	}
}

func reverifyVerifierForAPIKey(apiKey string) (invoicecore.Verifier, error) {
	cfg, err := config.LoadRuntime()
	if err != nil {
		return nil, fmt.Errorf("load verify API runtime config: %w", err)
	}
	return verifyapi.NewClientFromConfig(apiKey, cfg)
}
