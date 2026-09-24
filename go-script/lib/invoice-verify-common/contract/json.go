package contract

const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

type GenericFailure struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

func NewGenericFailure(message string) GenericFailure {
	return GenericFailure{Status: StatusFailed, Error: message}
}

type MultiCompanyFailure struct {
	Status       string   `json:"status"`
	Message      string   `json:"message"`
	CompanyNames []string `json:"company_names"`
}

type BatchVerifySuccess struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    BatchVerifyData `json:"data"`
}

type BatchVerifyData struct {
	Mode                  string `json:"mode"`
	ExcelFilePath         string `json:"excel_file_path"`
	IntermediateExcelPath string `json:"intermediate_excel_path"`
	TotalFileCount        int    `json:"total_file_count"`
	QRSuccessFileCount    int    `json:"qr_success_file_count"`
	UnsupportedFileCount  int    `json:"unsupported_file_count"`
	VerifySuccessCount    int    `json:"verify_success_count"`
	VerifyFailedCount     int    `json:"verify_failed_count"`
}

type ReverifySuccess struct {
	Status  string       `json:"status"`
	Message string       `json:"message"`
	Data    ReverifyData `json:"data"`
}

type ReverifyData struct {
	Mode                  string `json:"mode"`
	InputExcelPath        string `json:"input_excel_path"`
	ExcelFilePath         string `json:"excel_file_path"`
	IntermediateExcelPath string `json:"intermediate_excel_path"`
	OutputFile            string `json:"output_file"`
	TotalFileCount        int    `json:"total_file_count"`
	RetryCandidateCount   int    `json:"retry_candidate_count"`
	RetriedCount          int    `json:"retried_count"`
	UnretryableCount      int    `json:"unretryable_count"`
	UnsupportedCount      int    `json:"unsupported_count"`
	PreservedSuccessCount int    `json:"preserved_success_count"`
	VerifySuccessCount    int    `json:"verify_success_count"`
	VerifyFailedCount     int    `json:"verify_failed_count"`
	SkippedCount          int    `json:"skipped_count"`
}

type BatchManageSuccess struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    BatchManageData `json:"data"`
}

type BatchManageData struct {
	ExcelFilePath         string              `json:"excel_file_path"`
	IntermediateExcelPath string              `json:"intermediate_excel_path"`
	CompanyName           string              `json:"company_name"`
	TotalFileCount        int                 `json:"total_file_count"`
	ValidInvoiceCount     int                 `json:"valid_invoice_count"`
	VerifySuccessCount    int                 `json:"verify_success_count"`
	VerifyFailedCount     int                 `json:"verify_failed_count"`
	DuplicateRiskCount    int                 `json:"duplicate_risk_count"`
	NewListCount          int                 `json:"new_list_count"`
	ErrorAlerts           map[string][]string `json:"error_alerts"`
}

type ReimbursementPhase1Success struct {
	Status  string                  `json:"status"`
	Message string                  `json:"message"`
	Data    ReimbursementPhase1Data `json:"data"`
}

type ReimbursementPhase1Data struct {
	MailScanEmail                 string             `json:"mail_scan_email"`
	MailScanStartDate             string             `json:"mail_scan_start_date"`
	MailScanEndDate               string             `json:"mail_scan_end_date"`
	CompanyName                   string             `json:"company_name"`
	HitInvoiceRelatedEmailCount   int                `json:"hit_invoice_related_email_count"`
	DownloadedAttachmentFileCount int                `json:"downloaded_attachment_file_count"`
	TravelFormAttachmentFileCount int                `json:"travel_form_attachment_file_count"`
	QRSuccessFileCount            int                `json:"qr_success_file_count"`
	VerifySuccessCount            int                `json:"verify_success_count"`
	VerifyFailedCount             int                `json:"verify_failed_count"`
	VerifyExcelPath               string             `json:"verify_excel_path"`
	InvoiceOriginalDir            string             `json:"invoice_original_dir"`
	LineItemsSummary              []string           `json:"line_items_summary"`
	MailParseFailedMessages       []MailParseFailure `json:"mail_parse_failed_messages,omitempty"`
}

type MailParseFailure struct {
	UID     string `json:"uid"`
	Subject string `json:"subject,omitempty"`
	From    string `json:"from,omitempty"`
	Date    string `json:"date,omitempty"`
	Error   string `json:"error"`
}

type ReimbursementPhase2Success struct {
	Status  string                  `json:"status"`
	Message string                  `json:"message"`
	Data    ReimbursementPhase2Data `json:"data"`
}

type ReimbursementPhase2Data struct {
	CompanyName                 string           `json:"company_name"`
	VerifySuccessCount          int              `json:"verify_success_count"`
	VerifyFailedCount           int              `json:"verify_failed_count"`
	HeaderMismatchCount         int              `json:"header_mismatch_count"`
	DuplicateInvoiceCount       int              `json:"duplicate_invoice_count"`
	DuplicateInvoices           []map[string]any `json:"duplicate_invoices"`
	ReimbursementExcelPath      string           `json:"reimbursement_excel_path"`
	ReimbursementExcelName      string           `json:"reimbursement_excel_name"`
	VerifyExcelPath             string           `json:"verify_excel_path"`
	ClassificationTotalCount    int              `json:"classification_total_count"`
	ClassificationSuccessCount  int              `json:"classification_success_count"`
	ClassificationFallbackCount int              `json:"classification_fallback_count"`
}

type AnalysisSuccess struct {
	Status      string       `json:"status,omitempty"`
	InputFile   string       `json:"input_file"`
	OutputFile  string       `json:"output_file"`
	CompanyName string       `json:"company_name"`
	RecordCount int          `json:"record_count"`
	Message     string       `json:"message,omitempty"`
	Analysis    AnalysisData `json:"analysis"`
}

type AnalysisData struct {
	VerifyStats        map[string]any `json:"verify_stats"`
	StatusDistribution map[string]any `json:"status_distribution"`
	Overview           map[string]any `json:"overview"`
	TimeStats          map[string]any `json:"time_stats"`
	ProductStats       map[string]any `json:"product_stats"`
	CounterpartyStats  map[string]any `json:"counterparty_stats"`
	RiskAlerts         map[string]any `json:"risk_alerts"`
}

type ReportPDFSuccess struct {
	Status          string `json:"status,omitempty"`
	PDFOutputFile   string `json:"pdf_output_file"`
	FinalReportText string `json:"final_report_text"`
}
