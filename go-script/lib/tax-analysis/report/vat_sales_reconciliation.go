package report

import (
	"fmt"
	"math"
	"strings"
)

// invoiceSalesForVAT requires all invoice months in the declared period.
// A successful empty chunk is an explicit zero, whereas a missing month or
// an unavailable group amount cannot establish a complete comparison.
func invoiceSalesForVAT(ctx *reportContext, period TaxPeriod) (float64, bool) {
	if (ctx.task.StartTime != "" && period.Start < ctx.task.StartTime) ||
		(ctx.task.EndTime != "" && period.End > ctx.task.EndTime) {
		return 0, false
	}
	amounts := aggregateInvoiceAmounts(ctx.issued, true)
	unknown := map[string]bool{}
	for _, group := range ctx.issued {
		if !group.HasAmount {
			unknown[group.Month] = true
		}
	}
	for _, chunk := range readInvoiceSummary(ctx.modules["issued_invoices"]).Chunks {
		if chunk.Total == 0 && len(chunk.Groups) == 0 {
			if _, exists := amounts[chunk.Month]; !exists {
				amounts[chunk.Month] = 0
			}
		}
	}
	months := deliveryMonths(period.Start, period.End)
	total := 0.0
	for _, month := range months {
		amount, exists := amounts[month]
		if !exists || unknown[month] {
			return 0, false
		}
		total += amount
	}
	return total, len(months) > 0
}

// unbilledSalesForVAT reads the same annex business rows as section 5.6,
// restricted to one effective declaration and its actual tax period.
// Missing/conflicting cells remain unknown; signed amounts are not clamped.
func unbilledSalesForVAT(fields []TaxFieldFact, vat vatFact) (float64, bool, []string) {
	total, known, found := 0.0, true, false
	seen := map[string]TaxFieldFact{}
	var refs []string
	for _, f := range fields {
		if !isVATUnbilledBusinessField(f) || f.RecordID != vat.RecordID ||
			f.Period.Start != vat.TaxPeriod.Start || f.Period.End != vat.TaxPeriod.End {
			continue
		}
		key := fmt.Sprintf("%s|%d|%d|%s|%s", f.FormCode, f.Table, f.Row, f.Line, f.RowGroup)
		if previous, exists := seen[key]; exists {
			known = known && previous.Value == f.Value && previous.Status == f.Status
			continue
		}
		seen[key] = f
		found = true
		known = known && f.Status == "present"
		total += f.Value
		refs = append(refs, "report-model.json/taxFields[id="+f.ID+"]")
	}
	return total, found && known, refs
}

// analysisVATSalesReconciliation supplies sales (not tax) adjustments to 5.4.
// Each comparison uses one declared period; a quarter is never compared with
// just its final invoice month, and partially requested quarters stay unknown.
func analysisVATSalesReconciliation(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "vat.invoice-declaration-difference", RelatedSections: []string{"vat.special-items"}}
	var refs, periods []string
	var zeroPeriods, nonzeroPeriods, missingPeriods []string
	total, complete := 0.0, 0
	for _, vat := range ctx.vat {
		prefix := "sales-bridge." + analysisToken(vat.RecordID+"|"+vat.Period)
		invoice, invoiceKnown := invoiceSalesForVAT(ctx, vat.TaxPeriod)
		p := d.text(prefix+".period", "票申销售额调节所属期", "period", vat.Period, "vat_returns/"+vat.RecordID)
		unbilled, unbilledKnown, sources := unbilledSalesForVAT(ctx.taxFields, vat)
		var adjustment string
		if unbilledKnown {
			adjustment = d.number(prefix+".unbilled", "未开票销售额", "adjustment", "元", vat.Period, "", "同一有效申报附列一未开具发票销售额，全部业务行有符号汇总", unbilled, sources...)
		}
		if !vat.HasSales || !invoiceKnown || !unbilledKnown {
			missingPeriods = append(missingPeriods, vat.Period)
			missing := d.text(prefix+".state", "销售额调节状态", "state", "申报销售额、同期发票金额或未开票销售额尚不完整，剩余差额未知；不得将未取得的金额填零", p)
			refs = append(refs, p, missing)
			if adjustment != "" {
				refs = append(refs, adjustment)
			}
			continue
		}
		declared := d.number(prefix+".declared", "申报销售额", "amount", "元", vat.Period, "", "有效增值税申报的当期销售额", vat.Sales, "vat_returns/"+vat.RecordID)
		issued := d.number(prefix+".invoiced", "同期销项发票不含税金额", "amount", "元", vat.Period, "", "按税款所属期汇总发票净额，保留红字符号", invoice, "issued_invoices/"+vat.Period)
		gap := d.number(prefix+".original", "票申销售额原差额", "difference", "元", vat.Period, "", "申报销售额−销项发票不含税金额", vat.Sales-invoice, declared, issued)
		remaining := math.Round((vat.Sales-invoice-unbilled)*100) / 100
		residual := d.number(prefix+".remaining", "未开票销售额调节后剩余差额", "difference", "元", vat.Period, "", "申报销售额−销项发票不含税金额−未开票销售额", remaining, gap, adjustment)
		state := "未开票销售额已参与调节，仍有剩余差额待核对；业务及凭证尚未核验"
		if math.Abs(remaining) < invoiceAggregationTolerance {
			zeroPeriods = append(zeroPeriods, vat.Period)
			state = "票申销售额差异已由本期未开票销售额在金额上对应，剩余差额为零；业务及凭证尚未核验"
		} else {
			nonzeroPeriods = append(nonzeroPeriods, vat.Period)
		}
		status := d.text(prefix+".state", "销售额调节状态", "state", state, residual)
		refs = append(refs, p, gap, adjustment, residual, status)
		periods = append(periods, vat.Period)
		complete++
		total += remaining
	}
	if len(refs) > 0 {
		var summaryRefs []string
		for _, coverage := range []struct {
			key, name string
			periods   []string
		}{
			{"zero", "调节后剩余差额为零", zeroPeriods},
			{"nonzero", "调节后仍有剩余差额", nonzeroPeriods},
			{"missing", "调节资料未完整取得", missingPeriods},
		} {
			id := d.number("sales-bridge."+coverage.key+"-count", coverage.name+"期数", "coverage", "期", "", "", "逐个实际所属期判定；缺失不计零，正负抵消不合并期间", float64(len(coverage.periods)), refs...)
			d.Metrics[len(d.Metrics)-1].Scope = append([]string(nil), coverage.periods...)
			summaryRefs = append(summaryRefs, id, d.text("sales-bridge."+coverage.key+"-periods", coverage.name+"所属期", "period", textOr(strings.Join(coverage.periods, "、"), "无"), id))
		}
		summaryRefs = append(summaryRefs, d.number("sales-bridge.complete", "销售额调节资料完整期数", "coverage", "期", "", "", "三项金额均明确且所属期完整的比较期数", float64(complete), refs...))
		if len(periods) > 0 && disjointAnalysisPeriods(periods) {
			summaryRefs = append(summaryRefs, d.number("sales-bridge.total.remaining", "已核对期间销售额调节后剩余差额合计", "difference", "元", strings.Join(periods, "、"), "", "仅汇总不重叠且调节资料完整的所属期，不以正负抵消代替逐期核查", math.Round(total*100)/100, refs...))
		}
		d.focus("sales-bridge", "按已计算期数和期间区分剩余为零、仍有差额、资料缺失。非零期数不为零时不能说所有期间均已对应，即使全年合计抵消为零；解释主要调节与逐期剩余，不从两个月极值推断全年普遍关系。", append(summaryRefs, refs...)...)
	}
	d.Constraints = []string{"销售额与税额分别调节，不能用未开票销售额抵扣销项或进项税额。月报按月、季报按同季发票汇总；缺失字段不补零，不将年合计相等推定为逐月相等。"}
	return d
}
