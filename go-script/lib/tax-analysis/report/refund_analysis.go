package report

import (
	"fmt"
	"strings"
)

// Keep business records inside refunds. Transport SUCCESS is never a refund
// completion status and envelope metadata never becomes a business row.
func refundBusinessRecords(raw any) []map[string]any {
	refunds, _, ok := findFirst(raw, "refunds")
	if !ok {
		return nil
	}
	return findAllMaps(refunds, func(row map[string]any) bool {
		return mapText(row, "Zsxmmc", "zsxmmc", "taxName", "税种") != "" && (mapText(row, "Skssqq", "skssqq", "periodStart") != "" || refundAmountPresent(row))
	})
}

func refundAmountPresent(row map[string]any) bool {
	_, ok := mapNumber(row, "Syktdje", "Se", "refundAmount", "amount")
	return ok
}

func refundRecordDescription(row map[string]any) string {
	parts := []string{textOr(mapText(row, "Zsxmmc", "zsxmmc", "taxName", "税种"), "税种未取得")}
	if item := mapText(row, "Zspmmc", "zspmmc", "taxItem"); item != "" {
		parts = append(parts, item)
	}
	start, end := mapText(row, "Skssqq", "skssqq", "periodStart"), mapText(row, "Skssqz", "skssqz", "periodEnd")
	if start != "" {
		parts = append(parts, "税款所属期"+start+"至"+textOr(end, start))
	}
	for _, field := range []struct{ key, label string }{{"Se", "税额"}, {"Syktdje", "剩余可退抵金额"}, {"Ljytdje", "累计已退抵金额"}} {
		if value, ok := mapNumber(row, field.key); ok {
			parts = append(parts, field.label+moneyText(value)+"元")
		}
	}
	if _, ok := mapNumber(row, "Se", "Syktdje", "Ljytdje"); !ok {
		if value, present := mapNumber(row, "refundAmount", "amount"); present {
			parts = append(parts, "退抵记录金额"+moneyText(value)+"元")
		}
	}
	return strings.Join(parts, "；")
}

func analysisRefundRecords(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "tax-payment.exceptions"}
	for i, row := range refundBusinessRecords(ctx.modules[degradableModuleCode].Raw) {
		key := fmt.Sprintf("refund.%d", i+1)
		ref := d.text(key+".detail", "退抵事项及已取得金额", "refund-record", refundRecordDescription(row), "source-snapshot.json/moduleResults[code=arrears_and_refunds]/data/refunds")
		d.text(key+".state", "退抵记录含义", "state", "查询取得退抵记录不代表已申请或已完成退款；处理状态未明确返回时不推断", ref)
		d.focus(key, "有记录时说明具体税种、品目、所属期及已取得金额，不只写记录条数；可退抵金额与已退抵金额分别表述。", ref)
	}
	return d
}
