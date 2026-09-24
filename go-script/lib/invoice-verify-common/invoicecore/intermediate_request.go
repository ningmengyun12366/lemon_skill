package invoicecore

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

var ErrIntermediateRequest = errors.New("中间查验行无法重建查验请求")

func verifyRequestFromIntermediateRow(row map[string]string) (verifyapi.Request, error) {
	invoiceNumber := intermediateRowValue(row, invoiceinput.ListInvoiceNumberKeys)
	checkCode := normalizeOptionalIntermediateValue(intermediateRowValue(row, invoiceinput.ListCheckCodeKeys))
	invoiceDate := intermediateRowValue(row, invoiceinput.ListInvoiceDateKeys)
	totalAmount := intermediateRowValue(row, invoiceinput.ListTotalAmountKeys)

	var missing []string
	if invoiceNumber == "" {
		missing = append(missing, "发票号码")
	}
	if invoiceDate == "" {
		missing = append(missing, "开票日期")
	}
	if totalAmount == "" {
		missing = append(missing, "价税合计")
	}
	if len(missing) > 0 {
		return verifyapi.Request{}, fmt.Errorf("%w: 缺少必需字段: %s", ErrIntermediateRequest, strings.Join(missing, ", "))
	}

	normalizedDate, err := normalizeIntermediateDate(invoiceDate)
	if err != nil {
		return verifyapi.Request{}, err
	}
	normalizedAmount, err := normalizeIntermediateAmount(totalAmount)
	if err != nil {
		return verifyapi.Request{}, err
	}

	return deriveVerifyRequest(invoiceinput.QRData{
		InvoiceNumber: invoiceNumber,
		InvoiceCode:   normalizeOptionalIntermediateValue(intermediateRowValue(row, invoiceinput.ListInvoiceCodeKeys)),
		InvoiceDate:   normalizedDate,
		CheckCode:     checkCode,
		TotalAmount:   normalizedAmount,
	}), nil
}

func intermediateRowValue(row map[string]string, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(row[key]); value != "" {
			return value
		}
	}
	return ""
}

func normalizeIntermediateDate(value string) (string, error) {
	text := strings.TrimSpace(value)
	if normalized, ok := verifyapi.NormalizeIssueDate(text); ok {
		return normalized, nil
	}
	return "", fmt.Errorf("%w: 开票日期不可解析: %s", ErrIntermediateRequest, text)
}

func normalizeIntermediateAmount(value string) (string, error) {
	text := strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	amount, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return "", fmt.Errorf("%w: 价税合计不可解析: %s", ErrIntermediateRequest, strings.TrimSpace(value))
	}
	return text, nil
}

func normalizeOptionalIntermediateValue(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || text == "--" || strings.EqualFold(text, "nan") || strings.EqualFold(text, "none") {
		return ""
	}
	return text
}
