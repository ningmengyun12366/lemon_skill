package issue

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

type issuePayloadValidationError struct {
	validationErrors []string
}

func (err *issuePayloadValidationError) Error() string {
	return strings.Join(err.validationErrors, "\n")
}

/**
 * prepareIssuePayload validates the persisted payload before any remote issue work.
 *
 * @param taskState persisted invoice task state.
 * @param confirmPreview whether this call explicitly confirms the generated preview.
 * @returns the same normalized payload stored by the check stage.
 */
func prepareIssuePayload(taskState map[string]any, confirmPreview bool) (map[string]any, error) {
	check, _ := taskState["check_result"].(map[string]any)
	if check == nil {
		return nil, fmt.Errorf("任务状态 JSON 中缺少有效的 check_result")
	}
	if !legacyPythonTruthy(check["validation_passed"]) {
		return nil, fmt.Errorf("当前任务未通过校验，不能正式开票")
	}
	if !legacyPythonTruthy(check["invoice_preview_generated"]) {
		return nil, fmt.Errorf("当前任务未生成发票预览，不能正式开票")
	}
	payload, _ := check["invoice_payload_json"].(map[string]any)
	if payload == nil {
		return nil, fmt.Errorf("任务状态中未找到 invoice_payload_json")
	}
	infoItems, ok := asList(payload["info"])
	if !ok || len(infoItems) == 0 {
		return nil, fmt.Errorf("invoice_payload_json.info 至少需要 1 条开票数据")
	}
	if validationErrors := normalizeSubmitLinePriceQuantityPairs(payload); len(validationErrors) > 0 {
		check["validation_errors"] = appendUniqueValidationErrors(copyStringSlice(check["validation_errors"]), validationErrors...)
		check["validation_passed"] = false
		check["invoice_preview_confirmed"] = false
		taskState["task_status"] = "check_failed"
		return nil, &issuePayloadValidationError{validationErrors: validationErrors}
	}
	taskState["invoice_payload_json"] = payload
	if confirmPreview && !legacyPythonTruthy(check["invoice_preview_confirmed"]) {
		check["invoice_preview_confirmed"] = true
		taskState["task_status"] = "preview_confirmed"
	}
	if !legacyPythonTruthy(check["invoice_preview_confirmed"]) {
		return nil, fmt.Errorf("发票预览尚未确认，请先确认开票内容后再执行正式开票")
	}
	return payload, nil
}

/**
 * normalizeSubmitLinePriceQuantityPairs repairs safe legacy omissions and rejects quantity-only lines.
 *
 * @param payload checked invoice payload immediately before formal submission.
 * @returns all line-level price/quantity pairing errors with one-based indexes.
 */
func normalizeSubmitLinePriceQuantityPairs(payload map[string]any) []string {
	errors := []string{}
	infoItems, _ := asList(payload["info"])
	for invoiceIndex, infoAny := range infoItems {
		info, ok := infoAny.(map[string]any)
		if !ok {
			continue
		}
		lineItems, _ := asList(GetPath(info, "invoiceDetail.data"))
		for itemIndex, itemAny := range lineItems {
			item, ok := itemAny.(map[string]any)
			if !ok {
				continue
			}
			p, pOK := ToDecimal(item["price"])
			pt, ptOK := ToDecimal(item["taxInclusivePrice"])
			r, rOK := parseTaxRateDecimal(item["taxRate"])
			if pOK {
				p = quantizeUnitPrice(p)
				item["price"] = decimalToString(p)
			}
			if ptOK {
				pt = quantizeUnitPrice(pt)
				item["taxInclusivePrice"] = decimalToString(pt)
			}
			onePlusRate := decimal.NewFromInt(1).Add(r)
			if rOK && pOK && !ptOK {
				item["taxInclusivePrice"] = decimalToString(quantizeUnitPrice(p.Mul(onePlusRate)))
				ptOK = true
			}
			if rOK && ptOK && !pOK {
				item["price"] = decimalToString(quantizeUnitPrice(pt.Div(onePlusRate)))
				pOK = true
			}
			priceMode := HasMeaningfulValue(item["price"]) || HasMeaningfulValue(item["taxInclusivePrice"])
			if priceMode && !HasMeaningfulValue(item["quantity"]) {
				item["quantity"] = "1"
			}
			if HasMeaningfulValue(item["quantity"]) && !priceMode {
				quantity, quantityOK := ToDecimal(item["quantity"])
				errors = append(errors, buildPriceQuantityPairError(invoiceIndex+1, itemIndex+1, item["quantity"], quantity, quantityOK))
			}
		}
	}
	return errors
}

func appendUniqueValidationErrors(existing []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(additions))
	out := make([]string, 0, len(existing)+len(additions))
	for _, message := range append(existing, additions...) {
		if _, ok := seen[message]; ok {
			continue
		}
		seen[message] = struct{}{}
		out = append(out, message)
	}
	return out
}

func submitInvoiceIssue(client *common.APIClient, payload map[string]any) (map[string]any, error) {
	resp, err := client.PostJSON(issueRequestURL, payload, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("提交开票请求失败：%w", err)
	}
	return resp, nil
}

func legacyPythonTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return HasMeaningfulValue(value)
	}
}
