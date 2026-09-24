package invoicecore

import (
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

func verifyFailureStatus(result verifyapi.Result) string {
	if result.IsRateLimited {
		return "发票查验接口调用频繁，请稍后再试"
	}
	status := strings.TrimSpace(result.Message)
	if status == "" {
		return reverifyFailedStatus
	}
	if strings.HasPrefix(status, reverifyFailedStatus+":") {
		return status
	}
	return reverifyFailedStatus + ": " + status
}
