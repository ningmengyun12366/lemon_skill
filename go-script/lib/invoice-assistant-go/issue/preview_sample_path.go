package issue

import "invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"

func formatPreviewSampleLink(pathValue string) string {
	return common.FormatLocalPathText(pathValue)
}
