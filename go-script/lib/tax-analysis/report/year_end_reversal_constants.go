package report

const (
	yearEndReversalRuleID            = "INVOICE-YEAR-END-REVERSAL-001"
	yearEndReversalThemeID           = "year-end-invoice-reversal"
	yearEndReversalLowThreshold      = 0.03
	yearEndReversalMediumThreshold   = 0.05
	yearEndReversalHighThreshold     = 0.10
	yearEndReversalModuleCode        = "issued_invoices"
	yearEndReversalSectionSourceID   = "3.16"
	yearEndReversalSectionPriority   = 150
	sensitiveServicesSectionPriority = 160
	outputCluesSectionPriority       = 170
	inputCluesSectionPriority        = 180
)
