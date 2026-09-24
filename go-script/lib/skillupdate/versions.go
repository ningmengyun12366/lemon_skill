package skillupdate

const (
	BatchInvoiceVerifyVersion            = "0.1.4"
	InvoiceFetchAndRiskAnalysisVersion   = "0.1.6"
	InvoiceIssueVersion                  = "0.1.6"
	InvoiceReimbursementAssistantVersion = "0.1.4"
	InvoiceVerifyAnalysisVersion         = "0.1.4"
	TaxAnalysisVersion                   = "0.1.1"
	TaxKnowledgeVersion                  = "0.1.4"
)

var versions = map[string]string{
	"batch-invoice-verify":            BatchInvoiceVerifyVersion,
	"invoice-fetch-and-risk-analysis": InvoiceFetchAndRiskAnalysisVersion,
	"invoice-issue":                   InvoiceIssueVersion,
	"invoice-reimbursement-assistant": InvoiceReimbursementAssistantVersion,
	"invoice-verify-analysis":         InvoiceVerifyAnalysisVersion,
	"tax-analysis":                    TaxAnalysisVersion,
	"tax-knowledge":                   TaxKnowledgeVersion,
}

// Version returns the compiled version for a published skill.
func Version(skillName string) (string, bool) {
	version, ok := versions[skillName]
	return version, ok
}
