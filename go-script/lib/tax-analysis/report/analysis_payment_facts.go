package report

import (
	"fmt"
	"math"
)

// Payment records establish a historical payment, not a current arrears balance
// or the processing state of its principal. Those boundaries travel with each
// record into chapter and conclusion reading materials.
func analysisLateFeePayments(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "tax-payment.exceptions"}
	for i, row := range lateFeePaymentRows(ctx.modules["payments"]) {
		key := fmt.Sprintf("late-fee-payment.%d", i+1)
		source := "source-snapshot.json/moduleResults[code=payments]/data/list"
		period, _ := taxPeriodFromRecord(row, false)
		d.text(key+".tax", "滞纳金对应税费种", "object", textOr(mapText(row, "zsxmMc", "taxType", "taxName"), "未取得"), source)
		d.text(key+".period", "滞纳金税款所属期", "period", textOr(period.Display, "未取得"), source)
		if amount, ok := mapNumber(row, "lateFeeAmount", "sjje", "amount"); ok {
			d.number(key+".amount", "滞纳金缴款记录金额", "amount", "元", period.Display, "", "原缴款记录滞纳金金额的绝对值，与风险证据同源；不是当前欠款余额", math.Abs(amount), source)
		}
		d.text(key+".paid-date", "滞纳金缴款日期", "period", textOr(normalizeReportDate(mapText(row, "jkrq", "paymentDate", "paidAt"), false), "未取得"), source)
		d.text(key+".state", "记录性质", "state", "历史滞纳金缴款记录，不等于当前欠税，不重复建议缴纳该笔已缴金额", source)
		d.text(key+".principal-state", "对应本金处理状态", "state", "未取得逐笔对应的本金处理结果，不能据此认定未缴或全部办结", source)
		d.text(key+".pending-state", "该事项当前待办状态", "state", "历史缴款记录不提供当前待办状态，尚需对应核实", source)
	}
	return d
}
