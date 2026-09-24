package invoicecore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

type Verifier interface {
	Verify(context.Context, verifyapi.Request) (verifyapi.Result, error)
}

type Options struct {
	Now func() time.Time
}

type Counts struct {
	TotalFileCount          int
	QRSuccessFileCount      int
	UnsupportedFileCount    int
	VerifySuccessCount      int
	VerifyFailedCount       int
	RateLimitedSkippedCount int
	CircuitBrokenCount      int
}

type IntermediateRecord struct {
	ImportTime       string
	Filename         string
	FilePath         string
	InvoiceType      string
	InvoiceCode      string
	InvoiceNumber    string
	CheckCode        string
	InvoiceStatus    string
	InvoiceDate      string
	SellerName       string
	SellerTaxNo      string
	BuyerName        string
	BuyerTaxNo       string
	AmountWithoutTax string
	TaxAmount        string
	LineItemsJSON    string
	TotalAmount      string
	VerifyStatus     string
	VerifyTime       string
	ExtractionStatus string
	ExtractionReason string
}

type Result struct {
	Summary contract.BatchVerifySuccess
	Counts  Counts
	Records []IntermediateRecord
	Scan    invoiceinput.ScanResult
}

func Process(ctx context.Context, inputPath, outputExcelPath string, verifier Verifier, options Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if verifier == nil {
		return Result{}, fmt.Errorf("verifier is required")
	}
	scan, err := invoiceinput.Scan(inputPath, invoiceinput.Options{Now: options.Now})
	if err != nil {
		return Result{}, err
	}
	actualOutput, err := resolveOutputPath(scan, outputExcelPath)
	if err != nil {
		return Result{}, err
	}

	counts := Counts{TotalFileCount: len(scan.Entries), UnsupportedFileCount: scan.UnsupportedCount}
	records := make([]IntermediateRecord, len(scan.Entries))
	jobs := make([]initialVerifyJob, 0, len(scan.Entries))
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}

	preparedEntries, err := prepareEntries(ctx, scan.Entries, prepareEntriesOptions{Mode: scan.Mode})
	if err != nil {
		return Result{}, err
	}
	for index, entry := range preparedEntries {
		if entry.QRSuccess {
			counts.QRSuccessFileCount++
		}
		if !entry.IsSupported {
			counts.VerifyFailedCount++
			records[index] = buildRecord(entry, nil, verifyStatusForUnverifiedEntry(entry), "")
			continue
		}
		if !entry.QRSuccess {
			counts.VerifyFailedCount++
			records[index] = buildRecord(entry, nil, verifyStatusForUnverifiedEntry(entry), "")
			continue
		}
		jobs = append(jobs, initialVerifyJob{Index: index, Entry: entry})
	}

	outcomes, err := runInitialVerifyJobs(ctx, jobs, verifier, now)
	if err != nil {
		return Result{}, err
	}
	for _, outcome := range outcomes {
		records[outcome.Index] = outcome.Record
		switch outcome.Status {
		case initialVerifySuccess:
			counts.VerifySuccessCount++
		case initialVerifyRateLimited:
			counts.RateLimitedSkippedCount++
		case initialVerifyFailed:
			counts.VerifyFailedCount++
		}
	}

	summary := contract.BatchVerifySuccess{
		Status:  contract.StatusSuccess,
		Message: "处理完成",
		Data: contract.BatchVerifyData{
			Mode:                  scan.Mode,
			ExcelFilePath:         actualOutput,
			IntermediateExcelPath: actualOutput,
			TotalFileCount:        counts.TotalFileCount,
			QRSuccessFileCount:    counts.QRSuccessFileCount,
			UnsupportedFileCount:  counts.UnsupportedFileCount,
			VerifySuccessCount:    counts.VerifySuccessCount,
			VerifyFailedCount:     counts.VerifyFailedCount,
		},
	}
	return Result{Summary: summary, Counts: counts, Records: records, Scan: scan}, nil
}

func resolveOutputPath(scan invoiceinput.ScanResult, rawOutput string) (string, error) {
	if strings.TrimSpace(rawOutput) != "" {
		return pathx.Normalize(rawOutput)
	}
	defaultDir := filepath.Dir(scan.InputPath)
	if scan.Mode == invoiceinput.ModeFolder {
		defaultDir = scan.InputPath
	}
	return pathx.ResolveOutput("", defaultDir, invoiceinput.IntermediateResultFilename, ".xlsx")
}

func deriveVerifyRequest(data invoiceinput.QRData) verifyapi.Request {
	fplx := invoiceTypeFromCodeAndNumber(data.InvoiceCode, data.InvoiceNumber)
	jshj, bhsje := amountFields(fplx, data.TotalAmount)
	return verifyapi.Request{
		FPDM:   data.InvoiceCode,
		FPHM:   data.InvoiceNumber,
		JYM:    data.CheckCode,
		JSHJ:   jshj,
		BHSJE:  bhsje,
		KPRQ:   verifyapi.NormalizeIssueDateOrTrim(data.InvoiceDate),
		BSWJLX: []string{},
	}
}

func invoiceTypeFromCodeAndNumber(fpdm, fphm string) string {
	result := "99"
	if len(fpdm) == 12 {
		if specialType10Codes[fpdm] {
			result = "10"
		} else if fpdm[0] == '0' {
			suffix := fpdm[len(fpdm)-2:]
			switch suffix {
			case "11":
				result = "10"
			case "04", "05":
				result = "04"
			case "06", "07":
				result = "11"
			case "12":
				result = "14"
			}
		}
		if result == "99" && fpdm[0] == '0' && fpdm[len(fpdm)-2:] == "17" {
			result = "15"
		}
		if result == "99" && fpdm[7] == '2' && fpdm[0] != '0' {
			result = "03"
		}
		if result == "99" && fpdm[0] == '0' && fpdm[len(fpdm)-2:] == "13" {
			result = "08"
		}
	} else if len(fpdm) == 10 {
		switch fpdm[7] {
		case '1', '5':
			result = "01"
		case '6', '3':
			result = "04"
		}
	} else if fpdm == "" && len(fphm) == 20 {
		switch {
		case fphm[4] == '9':
			result = "83"
		case fphm[4] == '8':
			result = "61"
		case fphm[4:6] == "79":
			result = "72"
		default:
			result = "09"
		}
	}
	return result
}

func amountFields(fplx, amount string) (string, string) {
	if categoryEInvoiceTypes[fplx] {
		return "", ""
	}
	if nonEUseTotalAmountTypes[fplx] {
		return amount, ""
	}
	return "", amount
}

var categoryEInvoiceTypes = map[string]bool{"04": true, "10": true, "11": true, "14": true}

var nonEUseTotalAmountTypes = map[string]bool{"02": true, "15": true, "61": true, "09": true, "72": true, "83": true}

var specialType10Codes = map[string]bool{
	"144031539110": true,
	"131001570151": true,
	"133011501118": true,
	"111001571071": true,
}
