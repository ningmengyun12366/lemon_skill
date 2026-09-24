package fetchrisk

import (
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func NormalizeInvoiceFilenameLabel(fileName string) string {
	return strings.ReplaceAll(strings.ReplaceAll(fileName, "取得发票", "进项发票"), "开具发票", "销项发票")
}

func NormalizeExcelFilename(filename string) string {
	normalized := commonSanitize(NormalizeInvoiceFilenameLabel(filename), "invoice.xlsx")
	if !strings.HasSuffix(strings.ToLower(normalized), ".xlsx") {
		normalized += ".xlsx"
	}
	return normalized
}

func commonSanitize(filename, fallback string) string {
	return common.SanitizeFilename(strings.TrimSpace(filename), fallback)
}
