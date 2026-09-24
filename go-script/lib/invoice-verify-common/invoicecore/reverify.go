package invoicecore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

const (
	reverifyDefaultSuffix = "_重新查验_"
	reverifySuccessStatus = "查验成功"
	reverifyFailedStatus  = "查验失败"
	fileNameHeader        = "文件名"
)

var ErrReverifySameInputOutput = errors.New("reverify output path must not overwrite input workbook")

type ReverifyOptions struct {
	InputPath string
	Output    string
	Verifier  Verifier
	Now       func() time.Time
}

type ReverifyCounts struct {
	TotalFileCount        int
	RetryCandidateCount   int
	RetriedCount          int
	UnretryableCount      int
	UnsupportedCount      int
	PreservedSuccessCount int
	VerifySuccessCount    int
	VerifyFailedCount     int
	SkippedCount          int
}

type ReverifyResult struct {
	InputPath  string
	OutputPath string
	Counts     ReverifyCounts
	Rows       []map[string]string
}

func Reverify(ctx context.Context, options ReverifyOptions) (ReverifyResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.Verifier == nil {
		return ReverifyResult{}, errors.New("verifier is required")
	}
	inputPath, err := pathx.Normalize(options.InputPath)
	if err != nil {
		return ReverifyResult{}, err
	}
	now := reverifyNow(options.Now)
	outputPath, err := ResolveReverifyOutputPath(inputPath, options.Output, now())
	if err != nil {
		return ReverifyResult{}, err
	}
	rows, _, err := readReverifyRows(inputPath)
	if err != nil {
		return ReverifyResult{}, err
	}
	counts := ReverifyCounts{TotalFileCount: len(rows)}
	jobs := make([]reverifyJob, 0, len(rows))
	for idx := range rows {
		row := rows[idx]
		status := strings.TrimSpace(row[excelx.TaxBureauVerifyStatusHeader])
		if excelx.IsUnsupportedVerifyStatus(status) {
			counts.UnsupportedCount++
			continue
		}
		if status == reverifySuccessStatus {
			counts.PreservedSuccessCount++
			continue
		}
		counts.RetryCandidateCount++
		request, err := verifyRequestFromIntermediateRow(row)
		if err != nil {
			row[excelx.TaxBureauVerifyStatusHeader] = reverifySkipStatus(err)
			counts.UnretryableCount++
			continue
		}
		jobs = append(jobs, reverifyJob{index: idx, request: request})
	}
	counts.RetriedCount = len(jobs)
	runner := reverifyJobRunner{rows: rows, verifier: options.Verifier, now: now}
	if err := runReverifyJobs(ctx, jobs, runner); err != nil {
		return ReverifyResult{}, err
	}
	counts.finish(rows)
	actualPath, err := writeReverifyRows(outputPath, rows, now())
	if err != nil {
		return ReverifyResult{}, err
	}
	return ReverifyResult{InputPath: inputPath, OutputPath: actualPath, Counts: counts, Rows: rows}, nil
}

func ResolveReverifyOutputPath(inputPath, rawOutput string, now time.Time) (string, error) {
	input, err := pathx.Normalize(inputPath)
	if err != nil {
		return "", err
	}
	if now.IsZero() {
		now = time.Now()
	}
	defaultName := reverifyDefaultFilename(now)
	output, err := pathx.ResolveOutput(rawOutput, filepath.Dir(input), defaultName, ".xlsx")
	if err != nil {
		return "", err
	}
	if sameReverifyPath(input, output) {
		return "", ErrReverifySameInputOutput
	}
	return output, nil
}

func readReverifyRows(path string) ([]map[string]string, []string, error) {
	summary, err := excelx.ReadWorkbookSummary(path, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("read intermediate workbook: %w", err)
	}
	headers := make([]string, len(summary.Headers))
	for idx, header := range summary.Headers {
		headers[idx] = strings.TrimSpace(strings.TrimPrefix(header, "\ufeff"))
	}
	if missing := missingReverifyHeaders(headers); len(missing) > 0 {
		return nil, nil, fmt.Errorf("输入文件缺少必需列: %s", strings.Join(missing, ", "))
	}
	rows := make([]map[string]string, 0, len(summary.Rows))
	for _, source := range summary.Rows {
		row := make(map[string]string, len(source)+1)
		for idx, header := range headers {
			row[header] = source[summary.Headers[idx]]
		}
		applyReverifyHyperlink(row)
		rows = append(rows, row)
	}
	return rows, headers, nil
}

func writeReverifyRows(path string, rows []map[string]string, now time.Time) (string, error) {
	outputRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		output := make(map[string]any, len(excelx.IntermediateVerifyHeaders)+1)
		for _, header := range excelx.IntermediateVerifyHeaders {
			output[header] = row[header]
		}
		if sourcePath := strings.TrimSpace(row[excelx.SourcePathKey]); sourcePath != "" {
			output[excelx.SourcePathKey] = sourcePath
		}
		outputRows = append(outputRows, output)
	}
	return excelx.WriteWorkbook(path, excelx.WorkbookSpec{
		SheetName:     excelx.DefaultSheetName,
		Headers:       excelx.IntermediateVerifyHeaders,
		Rows:          outputRows,
		AmountHeaders: excelx.IntermediateAmountHeaders,
		Widths:        excelx.IntermediateWidths,
		Hyperlinks:    true,
	}, now)
}

func applyReverifyResult(row map[string]string, result verifyapi.Result) error {
	if !result.Success {
		row[excelx.TaxBureauVerifyStatusHeader] = verifyFailureStatus(result)
		return nil
	}
	data := result.Data
	row["发票类型"] = data.InvoiceType
	row["发票状态"] = data.InvoiceStatus
	row["销售方名称"] = data.SellerName
	row["销售方税号"] = data.SellerTaxNo
	row["购买方名称"] = data.BuyerName
	row["购买方税号"] = data.BuyerTaxNo
	row["不含税金额"] = data.AmountWithoutTax
	row["税额"] = data.TaxAmount
	row["价税合计"] = data.TotalAmount
	row[excelx.TaxBureauVerifyStatusHeader] = reverifySuccessStatus
	if data.LineItems == nil {
		return nil
	}
	payload, err := json.Marshal(data.LineItems)
	if err != nil {
		return fmt.Errorf("marshal line items: %w", err)
	}
	row["商品明细"] = string(payload)
	return nil
}

func (counts *ReverifyCounts) finish(rows []map[string]string) {
	for _, row := range rows {
		status := strings.TrimSpace(row[excelx.TaxBureauVerifyStatusHeader])
		if excelx.IsUnsupportedVerifyStatus(status) {
			continue
		}
		if status == reverifySuccessStatus {
			counts.VerifySuccessCount++
		} else {
			counts.VerifyFailedCount++
		}
	}
	counts.SkippedCount = counts.PreservedSuccessCount + counts.UnretryableCount + counts.UnsupportedCount
}

func missingReverifyHeaders(headers []string) []string {
	seen := make(map[string]bool, len(headers))
	for _, header := range headers {
		seen[strings.TrimSpace(header)] = true
	}
	missing := make([]string, 0)
	for _, header := range excelx.IntermediateVerifyHeaders {
		if !seen[header] {
			missing = append(missing, header)
		}
	}
	return missing
}

func reverifyNow(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return time.Now
}

func sameReverifyPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

func reverifyDefaultFilename(now time.Time) string {
	base := strings.TrimSuffix(invoiceinput.IntermediateResultFilename, filepath.Ext(invoiceinput.IntermediateResultFilename))
	return pathx.BuildDatedFilename(base+reverifyDefaultSuffix, ".xlsx", now)
}

func reverifySkipStatus(err error) string {
	message := strings.TrimSpace(err.Error())
	message = strings.TrimPrefix(message, ErrIntermediateRequest.Error()+": ")
	if message == "" {
		message = "无法重建查验请求"
	}
	return "跳过: " + message
}
