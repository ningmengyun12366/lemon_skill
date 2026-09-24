package issue

import (
	"fmt"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func buildBatchItemResult(invoiceIndex int, result map[string]any) map[string]any {
	return map[string]any{
		"invoice_index":               invoiceIndex,
		"validation_passed":           legacyPythonTruthy(result["validation_passed"]),
		"missing_fields_by_module":    copyMapValue(result["missing_fields_by_module"]),
		"validation_errors":           copyStringSlice(result["validation_errors"]),
		"tax_rate_validation":         copyMapValue(result["tax_rate_validation"]),
		"purchaser_autofill":          copyMapValue(result["purchaser_autofill"]),
		"recognized_fields_by_module": copyMapValue(result["recognized_fields_by_module"]),
	}
}

func flattenBatchValidationErrors(itemResults []map[string]any) []string {
	out := []string{}
	for _, result := range itemResults {
		out = append(out, copyStringSlice(result["validation_errors"])...)
	}
	return out
}

func copyStringSlice(value any) []string {
	switch source := value.(type) {
	case []string:
		return append([]string{}, source...)
	case []any:
		out := make([]string, 0, len(source))
		for _, item := range source {
			if text := strings.TrimSpace(common.StringValue(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return []string{}
	}
}

func buildPreviewMarkdownWithWarnings(result, invoicePayload map[string]any, previewPath string) string {
	previewMarkdown := BuildPreviewMarkdown(invoicePayload, previewPath)
	if warning := taxRateWarningMarkdown(result); warning != "" {
		return warning + "\n\n" + previewMarkdown
	}
	return previewMarkdown
}

func taxRateWarningMarkdown(result map[string]any) string {
	if taxRateValidation, ok := result["tax_rate_validation"].(map[string]any); ok {
		if warning := strings.TrimSpace(common.StringValue(taxRateValidation["warning_markdown"])); warning != "" {
			return warning
		}
	}
	if itemResults, ok := result["item_results"].([]map[string]any); ok {
		return buildBatchTaxRateWarningMarkdown(itemResults)
	}
	if itemResults, ok := asList(result["item_results"]); ok {
		out := make([]map[string]any, 0, len(itemResults))
		for _, itemAny := range itemResults {
			item, ok := itemAny.(map[string]any)
			if ok {
				out = append(out, item)
			}
		}
		return buildBatchTaxRateWarningMarkdown(out)
	}
	return ""
}

func buildBatchTaxRateWarningMarkdown(itemResults []map[string]any) string {
	sections := []string{}
	for _, itemResult := range itemResults {
		taxRateValidation, _ := itemResult["tax_rate_validation"].(map[string]any)
		warnings, _ := asList(taxRateValidation["warnings"])
		if len(warnings) == 0 {
			continue
		}
		invoiceIndex := intValue(itemResult["invoice_index"])
		lines := []string{fmt.Sprintf("**第%d张发票税率参考提示**", invoiceIndex)}
		taxpayerType := strings.TrimSpace(common.StringValue(taxRateValidation["taxpayer_type"]))
		if taxpayerType != "" {
			lines = append(lines, "- 纳税人类型："+taxpayerType)
		}
		for _, warningAny := range warnings {
			warning, ok := warningAny.(map[string]any)
			if !ok {
				continue
			}
			message := strings.TrimSpace(common.StringValue(warning["message"]))
			if message != "" {
				lines = append(lines, "- "+message)
			}
		}
		lines = append(lines, "- 以上仅为参考提示，不强制拦截开票，请核对后继续。")
		sections = append(sections, strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func extractSharedTopLevelFields(rawData map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"uscc", "areaCode", "personalAccount", "taxpayerType"} {
		if HasMeaningfulValue(rawData[key]) {
			out[key] = rawData[key]
		}
	}
	return out
}

func extractBatchInvoiceSources(rawData map[string]any) []map[string]any {
	candidate := firstNonNil(rawData["info"], rawData["invoices"], rawData["items"], rawData["发票"])
	list, ok := asList(candidate)
	if !ok {
		if looksLikeNestedInvoiceItem(rawData) {
			return []map[string]any{rawData}
		}
		return []map[string]any{}
	}
	out := []map[string]any{}
	for _, item := range list {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func buildSingleInvoiceRawData(shared, source map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range shared {
		out[k] = v
	}
	for k, v := range source {
		out[k] = v
	}
	return out
}

func looksLikeNestedInvoiceItem(source map[string]any) bool {
	for _, key := range []string{"basicInfo", "sellerInfo", "purchaserInfo", "invoiceDetail", "noteInfo", "paymentInfo", "personInfo", "fplsh", "isIncludeTax"} {
		if _, ok := source[key]; ok {
			return true
		}
	}
	return false
}

func flattenBatchMissingFields(itemResults []map[string]any) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for _, result := range itemResults {
		invoiceIndex := intValue(result["invoice_index"])
		modules, _ := result["missing_fields_by_module"].(map[string]any)
		for module, rawEntries := range modules {
			entries, _ := asList(rawEntries)
			if len(entries) == 0 {
				continue
			}
			key := fmt.Sprintf("第%d张发票 / %s", invoiceIndex, module)
			for _, entryAny := range entries {
				entry, ok := entryAny.(map[string]any)
				if !ok {
					continue
				}
				out[key] = append(out[key], copyMapValue(entry))
			}
		}
	}
	return out
}

func copyMapValue(value any) map[string]any {
	out := map[string]any{}
	switch source := value.(type) {
	case map[string]any:
		for k, v := range source {
			out[k] = v
		}
	case map[string][]map[string]any:
		for k, entries := range source {
			copied := make([]any, 0, len(entries))
			for _, entry := range entries {
				copied = append(copied, copyMapValue(entry))
			}
			out[k] = copied
		}
	}
	return out
}
