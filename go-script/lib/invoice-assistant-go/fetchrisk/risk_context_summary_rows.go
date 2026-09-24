package fetchrisk

import (
	"math"
	"strings"
	"time"
)

func parseRiskContextRows(records []any, directions map[string]string) []riskContextRow {
	rows := make([]riskContextRow, 0, len(records))
	for _, item := range records {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fileID := riskText(record, "file_id")
		invoiceAmount := riskFloat(record, amountHeaderKeys()...)
		invoiceTax := riskFloat(record, taxHeaderKeys()...)
		invoiceTotal := riskFloat(record, totalHeaderKeys()...)
		detailAmount, hasDetailAmount := riskFloatWithPresence(record, "明细金额")
		detailTax, hasDetailTax := riskFloatWithPresence(record, "明细税额")
		amount, tax, total := invoiceAmount, invoiceTax, invoiceTotal
		if hasDetailAmount || hasDetailTax {
			amount, tax = detailAmount, detailTax
			total = round2(amount + tax)
		} else if total == 0 && (amount != 0 || tax != 0) {
			total = round2(amount + tax)
		}
		issueDate, hasIssueDate := parseIssueDate(riskText(record, "开票日期", "日期"))
		positiveFlag := riskText(record, "是否正数发票")
		rows = append(rows, riskContextRow{
			FileID:             fileID,
			Direction:          directions[fileID],
			InvoiceNo:          riskText(record, "数电发票号码", "数电票号码", "发票号码"),
			IssueMonth:         riskIssueMonth(issueDate, hasIssueDate),
			InvoiceType:        riskText(record, "发票票种", "发票类型"),
			Status:             riskText(record, "发票状态", "状态"),
			PositiveFlag:       positiveFlag,
			HasPositiveFlag:    positiveFlag != "",
			BuyerName:          riskText(record, "购买方名称", "购方名称", "购方单位名称"),
			SellerName:         riskText(record, "销方名称", "销售方名称", "销方单位名称"),
			Amount:             amount,
			Tax:                tax,
			Total:              total,
			InvoiceAmount:      invoiceAmount,
			InvoiceTax:         invoiceTax,
			InvoiceTotal:       invoiceTotal,
			HasDetailAmount:    hasDetailAmount || hasDetailTax,
			RowIndex:           intAny(record["row_index"]),
			AmountBasisDiffers: (hasDetailAmount || hasDetailTax) && moneyDiffers(invoiceAmount, amount, invoiceTax, tax, invoiceTotal, total),
		})
	}
	return rows
}

func directionByFileID(fileMetas []any) map[string]string {
	directions := map[string]string{}
	for _, item := range fileMetas {
		meta, ok := item.(map[string]any)
		if !ok {
			continue
		}
		directions[textAny(meta["file_id"])] = normalizedRiskDirection(textAny(meta["direction_tag"]))
	}
	return directions
}

func normalizedRiskDirection(direction string) string {
	switch strings.TrimSpace(direction) {
	case "input", "output":
		return strings.TrimSpace(direction)
	default:
		return "unknown"
	}
}

func riskText(record map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := textAny(record[key]); text != "" {
			return text
		}
	}
	return ""
}

func riskFloat(record map[string]any, keys ...string) float64 {
	value, _ := riskFloatWithPresence(record, keys...)
	return value
}

func riskFloatWithPresence(record map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if text := textAny(record[key]); text != "" {
			return parseFloat(text), true
		}
	}
	return 0, false
}

func riskIssueMonth(issueDate time.Time, ok bool) string {
	if !ok {
		return ""
	}
	return issueDate.Format("2006-01")
}

func moneyDiffers(invoiceAmount, amount, invoiceTax, tax, invoiceTotal, total float64) bool {
	return math.Abs(invoiceAmount-amount) > 0.01 || math.Abs(invoiceTax-tax) > 0.01 || (invoiceTotal != 0 && math.Abs(invoiceTotal-total) > 0.01)
}
