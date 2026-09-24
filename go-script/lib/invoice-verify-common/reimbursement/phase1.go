package reimbursement

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/config"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoicecore"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/mailimap"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
	"invoice-assistant-skill/go-script/lib/skillupdate"
)

const phase1TempDirPattern = ".phase1-*"

var loadRuntimeConfig = config.LoadRuntime

var fetcherOverrideFactory func() (mailimap.Fetcher, error)

var verifierOverrideFactory func() invoicecore.Verifier

type Phase1Options struct {
	Email        string
	Password     string
	StartDate    time.Time
	EndDate      time.Time
	InvoiceDir   string
	APIKey       string
	Keyword      string
	MailProvider string
	IMAPHost     string
	Port         int
	Limit        int
	Fetcher      mailimap.Fetcher
	Verifier     invoicecore.Verifier
	Now          func() time.Time
}

func Phase1(ctx context.Context, options Phase1Options) (contract.ReimbursementPhase1Success, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.EndDate.Before(options.StartDate) {
		return contract.ReimbursementPhase1Success{}, fmt.Errorf("--end-date must not be earlier than --start-date")
	}
	verifier, err := resolveVerifier(options.Verifier, options.APIKey)
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	invoiceDir, err := pathx.Normalize(options.InvoiceDir)
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	if err := os.MkdirAll(invoiceDir, 0o700); err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	now := resolveNow(options.Now)
	stagingDir, err := os.MkdirTemp(invoiceDir, phase1TempDirPattern)
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	defer os.RemoveAll(stagingDir)
	candidateDir := filepath.Join(stagingDir, "invoice_candidates")
	travelDir := filepath.Join(stagingDir, "travel_forms")
	if err := os.MkdirAll(candidateDir, 0o700); err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	if err := os.MkdirAll(travelDir, 0o700); err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}

	server, err := mailimap.ResolveServer(options.MailProvider, options.IMAPHost, options.Port)
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	fetcher, err := resolveFetcher(options.Fetcher)
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	scan, err := fetcher.Fetch(ctx, mailimap.ScanOptions{
		Email:         options.Email,
		Password:      options.Password,
		Host:          server.Host,
		Port:          server.Port,
		Provider:      server.Provider,
		Mailbox:       mailimap.DefaultMailbox,
		Limit:         options.Limit,
		StartDate:     options.StartDate,
		EndDate:       options.EndDate,
		Keyword:       options.Keyword,
		AttachmentDir: candidateDir,
		TravelDir:     travelDir,
	})
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}

	verifyExcelPath, err := pathx.ResolveOutput("", invoiceDir, invoiceinput.IntermediateResultFilename, ".xlsx")
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	coreResult, err := invoicecore.Process(ctx, candidateDir, verifyExcelPath, verifier, invoicecore.Options{Now: now})
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	hitInvoiceRelatedCount := hitInvoiceRelatedEmailCount(coreResult.Records, scan)
	if err := retainPhase1Files(invoiceDir, &coreResult, scan); err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	actualExcelPath, err := writeIntermediateWorkbook(coreResult, verifyExcelPath, now())
	if err != nil {
		return contract.ReimbursementPhase1Success{}, err
	}
	lineItemsSummary := extractLineItemsSummary(coreResult.Records)
	if lineItemsSummary == nil {
		lineItemsSummary = []string{}
	}

	return contract.ReimbursementPhase1Success{
		Status:  contract.StatusSuccess,
		Message: "phase1 完成",
		Data: contract.ReimbursementPhase1Data{
			MailScanEmail:                 options.Email,
			MailScanStartDate:             options.StartDate.Format("2006-01-02"),
			MailScanEndDate:               options.EndDate.Format("2006-01-02"),
			CompanyName:                   inferCompanyName(coreResult.Records),
			HitInvoiceRelatedEmailCount:   hitInvoiceRelatedCount,
			DownloadedAttachmentFileCount: scan.SavedAttachmentCount(),
			TravelFormAttachmentFileCount: scan.TravelFormAttachmentCount(),
			QRSuccessFileCount:            coreResult.Counts.QRSuccessFileCount,
			VerifySuccessCount:            coreResult.Counts.VerifySuccessCount,
			VerifyFailedCount:             coreResult.Counts.VerifyFailedCount,
			VerifyExcelPath:               actualExcelPath,
			InvoiceOriginalDir:            invoiceDir,
			LineItemsSummary:              lineItemsSummary,
			MailParseFailedMessages:       mailParseFailures(scan.ParseFailures),
		},
	}, nil
}

func retainPhase1Files(invoiceDir string, result *invoicecore.Result, scan mailimap.ScanResult) error {
	movedByPath := map[string]string{}
	for _, message := range scan.Messages {
		for _, attachment := range message.Attachments {
			if !attachment.IsTravelForm || strings.TrimSpace(attachment.SavedPath) == "" {
				continue
			}
			newPath, err := moveUniqueFile(invoiceDir, attachment.SavedPath)
			if err != nil {
				return err
			}
			movedByPath[lookupPath(attachment.SavedPath)] = newPath
		}
	}

	for index := range result.Records {
		record := &result.Records[index]
		if !recognizedInvoiceRecord(*record) || strings.TrimSpace(record.FilePath) == "" {
			continue
		}
		key := lookupPath(record.FilePath)
		newPath := movedByPath[key]
		if newPath == "" {
			var err error
			newPath, err = moveUniqueFile(invoiceDir, record.FilePath)
			if err != nil {
				return err
			}
			movedByPath[key] = newPath
		}
		record.FilePath = newPath
		record.Filename = filepath.Base(newPath)
	}
	return nil
}

func recognizedInvoiceRecord(record invoicecore.IntermediateRecord) bool {
	return strings.TrimSpace(record.InvoiceNumber) != ""
}

func moveUniqueFile(targetDir, sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", fmt.Errorf("source path is required")
	}
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return "", err
	}
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for attempt := 0; attempt < 10000; attempt++ {
		name := base
		if attempt > 0 {
			name = fmt.Sprintf("%s (%d)%s", stem, attempt, ext)
		}
		candidate := filepath.Join(targetDir, name)
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.Rename(sourcePath, candidate); err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", err
		}
		return candidate, nil
	}
	return "", fmt.Errorf("too many duplicate filenames for %s", base)
}

func mailParseFailures(failures []mailimap.MessageParseFailure) []contract.MailParseFailure {
	if len(failures) == 0 {
		return nil
	}
	converted := make([]contract.MailParseFailure, 0, len(failures))
	for _, failure := range failures {
		item := contract.MailParseFailure{
			UID:     failure.UID,
			Subject: failure.Subject,
			From:    failure.From,
			Error:   failure.Error,
		}
		if !failure.Date.IsZero() {
			item.Date = failure.Date.Format("2006-01-02 15:04:05")
		}
		converted = append(converted, item)
	}
	return converted
}

func writeIntermediateWorkbook(result invoicecore.Result, outputPath string, now time.Time) (string, error) {
	rows := make([]map[string]any, 0, len(result.Records))
	for _, record := range result.Records {
		row := make(map[string]any, len(excelx.IntermediateVerifyHeaders)+1)
		for key, value := range record.RowMap() {
			row[key] = value
		}
		row[excelx.SourcePathKey] = record.FilePath
		rows = append(rows, row)
	}
	return excelx.WriteWorkbook(outputPath, excelx.WorkbookSpec{
		SheetName:     excelx.DefaultSheetName,
		Headers:       excelx.IntermediateVerifyHeaders,
		Rows:          rows,
		AmountHeaders: excelx.IntermediateAmountHeaders,
		Widths:        excelx.IntermediateWidths,
		Hyperlinks:    true,
	}, now)
}

func resolveFetcher(fetcher mailimap.Fetcher) (mailimap.Fetcher, error) {
	if fetcher != nil {
		return fetcher, nil
	}
	if fetcherOverrideFactory != nil {
		if override, err := fetcherOverrideFactory(); err != nil || override != nil {
			return override, err
		}
	}
	cfg, err := loadRuntimeConfig()
	if err != nil {
		return nil, err
	}
	return mailimap.IMAPFetcher{VersionChecker: skillupdate.Shared(cfg.SkillName, cfg.InstallDocURL)}, nil
}

func resolveVerifier(verifier invoicecore.Verifier, apiKey string) (invoicecore.Verifier, error) {
	if verifier != nil {
		return verifier, nil
	}
	if verifierOverrideFactory != nil {
		if override := verifierOverrideFactory(); override != nil {
			return override, nil
		}
	}
	cfg, err := loadRuntimeConfig()
	if err != nil {
		return nil, err
	}
	return verifyapi.NewClientFromConfig(apiKey, cfg)
}

func inferCompanyName(records []invoicecore.IntermediateRecord) string {
	seen := map[string]bool{}
	var names []string
	for _, record := range records {
		if strings.TrimSpace(record.VerifyStatus) != "查验成功" {
			continue
		}
		name := strings.TrimSpace(record.BuyerName)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 1 {
		return names[0]
	}
	return ""
}

func hitInvoiceRelatedEmailCount(records []invoicecore.IntermediateRecord, mailScan mailimap.ScanResult) int {
	messageByPath := map[string]string{}
	for savedPath, messageID := range mailScan.MessageIDBySavedPath() {
		messageByPath[lookupPath(savedPath)] = messageID
	}
	seenMessages := map[string]bool{}
	for _, record := range records {
		if strings.TrimSpace(record.InvoiceNumber) == "" {
			continue
		}
		if messageID := strings.TrimSpace(messageByPath[lookupPath(record.FilePath)]); messageID != "" {
			seenMessages[messageID] = true
		}
	}
	return len(seenMessages)
}

func lookupPath(path string) string {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return strings.ToLower(filepath.Clean(path))
}

func extractLineItemsSummary(records []invoicecore.IntermediateRecord) []string {
	seen := map[string]bool{}
	var summary []string
	for _, record := range records {
		if strings.TrimSpace(record.VerifyStatus) != "查验成功" || strings.TrimSpace(record.LineItemsJSON) == "" {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(record.LineItemsJSON), &decoded); err != nil {
			continue
		}
		for _, item := range lineItemNames(decoded) {
			item = strings.TrimSpace(item)
			if item != "" && !seen[item] {
				seen[item] = true
				summary = append(summary, item)
			}
		}
	}
	return summary
}

func lineItemNames(value any) []string {
	switch typed := value.(type) {
	case []any:
		var names []string
		for _, item := range typed {
			names = append(names, lineItemNames(item)...)
		}
		return names
	case []map[string]string:
		var names []string
		for _, item := range typed {
			names = append(names, lineItemNames(item)...)
		}
		return names
	case map[string]any:
		for _, key := range []string{"Hwhyslwfwmc", "xmmc", "spmc", "name", "商品名称", "项目名称"} {
			if text := lineItemFieldValue(typed, key); text != "" {
				return []string{text}
			}
		}
		var names []string
		for _, value := range typed {
			names = append(names, lineItemNames(value)...)
		}
		return names
	case map[string]string:
		for _, key := range []string{"Hwhyslwfwmc", "xmmc", "spmc", "name", "商品名称", "项目名称"} {
			if text := lineItemStringFieldValue(typed, key); text != "" {
				return []string{text}
			}
		}
		var names []string
		for _, value := range typed {
			if text := strings.TrimSpace(value); text != "" {
				names = append(names, text)
			}
		}
		return names
	default:
		return nil
	}
}

func lineItemFieldValue(item map[string]any, key string) string {
	for field, value := range item {
		if strings.EqualFold(field, key) {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func lineItemStringFieldValue(item map[string]string, key string) string {
	for field, value := range item {
		if strings.EqualFold(field, key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveNow(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return time.Now
}
