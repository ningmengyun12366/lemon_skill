package report

import (
	"fmt"
	"math"
)

const (
	invoiceOutputReconciliationRule = "INVOICE-OUTPUT-RECONCILIATION-001"
	invoiceInputReconciliationRule  = "INVOICE-INPUT-RECONCILIATION-001"
	invoiceDifferenceNormal         = 0.01
	invoiceDifferenceLow            = 0.03
	invoiceDifferenceMediumAmount   = 5000.0
	invoiceDifferenceHighAttention  = 0.10
)

type invoiceTaxComparison struct {
	VAT                 vatFact
	Output, Input       float64
	HasOutput, HasInput bool
}

// selectedTaxForPeriod reads effective selected tax by its actual deduction
// period. Missing query chunks or incomplete source amounts remain unknown.
func selectedTaxForPeriod(ctx *reportContext, start, end string) (float64, bool) {
	module := ctx.modules[deductionConfirmationModuleCode]
	summary := readDeductionSummary(module.Raw)
	months := deliveryMonths(start, end)
	covered := map[string]bool{}
	seen := map[string]bool{}
	amount, complete := 0.0, module.Complete && len(months) > 0
	for _, chunk := range summary.Chunks {
		if chunk.List == nil {
			continue
		}
		for _, value := range chunk.List.SelectedTax {
			if monthWithinAnalysisPeriod(value.Period, start, end) {
				if seen[value.Period] || !monthWithinAnalysisPeriod(value.Period, chunk.StartTime, chunk.EndTime) {
					complete = false
					continue
				}
				seen[value.Period] = true
				covered[value.Period] = value.Complete
				amount += value.Amount
				complete = complete && value.Complete
			}
		}
	}
	for _, month := range months {
		complete = complete && covered[month]
	}
	return amount, complete
}

func invoiceTaxForPeriod(ctx *reportContext, start, end string) (float64, bool) {
	module := ctx.modules["issued_invoices"]
	covered := map[string]bool{}
	for _, chunk := range readInvoiceSummary(module).Chunks {
		month := chunk.Month
		if len(month) == 6 {
			month = month[:4] + "-" + month[4:]
		}
		covered[month] = true
	}
	amount, complete := 0.0, module.Complete
	months := deliveryMonths(start, end)
	if len(months) == 0 {
		return 0, false
	}
	for _, month := range months {
		complete = complete && covered[month]
	}
	for _, group := range ctx.issued {
		if monthWithinAnalysisPeriod(group.Month, start, end) {
			amount += group.Tax
			complete = complete && group.HasTax
		}
	}
	return amount, complete
}

func invoiceTaxComparisons(ctx *reportContext) []invoiceTaxComparison {
	var result []invoiceTaxComparison
	for _, vat := range ctx.vat {
		output, hasOutput := invoiceTaxForPeriod(ctx, vat.TaxPeriod.Start, vat.TaxPeriod.End)
		input, hasInput := selectedTaxForPeriod(ctx, vat.TaxPeriod.Start, vat.TaxPeriod.End)
		result = append(result, invoiceTaxComparison{vat, output, input, hasOutput, hasInput && ctx.taxpayer == taxpayerGeneral})
	}
	return result
}

func invoiceTaxDifferenceRate(ticket, filing float64) float64 {
	return math.Abs(ticket-filing) / math.Max(1, math.Max(math.Abs(ticket), math.Abs(filing)))
}

// invoiceTaxDifferenceLevel applies only the Word ticket-to-filing comparison,
// never the difference between sales tax and input tax. Reconciliation evidence
// is not returned by this contract, so a large gap alone cannot become high risk.
func invoiceTaxDifferenceLevel(ticket, filing float64) RiskLevel {
	rate := invoiceTaxDifferenceRate(ticket, filing)
	if rate <= invoiceDifferenceNormal {
		return ""
	}
	if rate <= invoiceDifferenceLow {
		return RiskLow
	}
	if math.Abs(ticket-filing) > invoiceDifferenceMediumAmount {
		return RiskMedium
	}
	return ""
}

// outputUnbilledTax reads tax (not sales) from all-business general-tax rows
// of annex one. Source period and effective declaration must match the VAT fact.
func outputUnbilledTax(ctx *reportContext, vat vatFact) (float64, bool, []string) {
	total, found := 0.0, false
	var refs []string
	seen := map[string]bool{}
	for _, f := range ctx.taxFields {
		if f.Module != "vat_returns" || f.RecordID != vat.RecordID || f.Period.Start != vat.TaxPeriod.Start || f.Period.End != vat.TaxPeriod.End ||
			(f.FormCode != vatAnnexOneFormCode && f.FormCode != vatNewAnnexOneFormCode) || canonicalFormLabel(f.Group) != "未开具发票" ||
			!declarationLabelMatches(f.ColumnLabel, []string{"销项（应纳）税额", "销项税额"}) || containsAny(f.RowLabel+f.RowGroup, "其中", "即征即退", "简易", "免税") {
			continue
		}
		if !containsAny(canonicalFormLabel(f.RowGroup), "一般计税") || !containsAny(canonicalFormLabel(f.RowGroup), "全部征税项目") {
			continue
		}
		key := f.FormCode + f.RowLabel + f.Line
		if seen[key] {
			continue
		}
		seen[key] = true
		if f.Status != "present" {
			return 0, false, nil
		}
		total += f.Value
		found = true
		refs = append(refs, f.ID)
	}
	return total, found, refs
}

func evaluateInvoiceTaxComparisons(ctx *reportContext) []RuleEvaluation {
	var results []RuleEvaluation
	for _, c := range invoiceTaxComparisons(ctx) {
		for _, input := range []bool{false, true} {
			id, name, tax, known, filing, filingKnown := invoiceOutputReconciliationRule, "销项", c.Output, c.HasOutput, c.VAT.OutputTax, c.VAT.HasOutputTax
			if input {
				id, name, tax, known, filing, filingKnown = invoiceInputReconciliationRule, "进项", c.Input, c.HasInput, c.VAT.InputTax, c.VAT.HasInputTax
			}
			e := RuleEvaluation{RuleID: id, Outcome: OutcomeDataInsufficient, Evidence: c.VAT.Period + name + "票申税额缺少同属期可比数据"}
			if input && ctx.taxpayer == taxpayerSmall {
				e.Outcome = OutcomeNotApplicable
				results = append(results, e)
				continue
			}
			if known && filingKnown {
				e.Outcome = OutcomeClear
				e.Evidence = fmt.Sprintf("%s%s对应税额%s，申报税额%s，差额%s，差异率%s。", c.VAT.Period, name, moneyText(tax), moneyText(filing), moneyText(tax-filing), percentText(invoiceTaxDifferenceRate(tax, filing)))
				comparedFiling := filing
				if !input {
					if unbilled, ok, _ := outputUnbilledTax(ctx, c.VAT); ok {
						comparedFiling -= unbilled
						e.Evidence = fmt.Sprintf("%s销项申报税额%s，发票税额%s，原差额%s；未开票申报税额%s，调节后剩余差额%s。", c.VAT.Period, moneyText(filing), moneyText(tax), moneyText(filing-tax), moneyText(unbilled), moneyText(comparedFiling-tax))
					}
				}
				level := invoiceTaxDifferenceLevel(tax, comparedFiling)
				if level != "" {
					e.Outcome = OutcomeRisk
					recommendation := "按所属期核对销项开票、红冲及未开票申报税额，解释调节后剩余差额；确认申报错误后处理对应期间。"
					if input {
						recommendation = "按所属期核对有效勾选、进项转出和其他抵扣凭证，解释抵扣与申报差额；确认错误后处理对应期间。"
					}
					e.Finding = &Finding{ID: id + "-" + c.VAT.Period, RiskThemeID: id, Title: name + "票申税额存在差异待核实", Level: level, ModuleCode: "vat_returns", Period: c.VAT.Period, Periods: []string{c.VAT.Period}, Amount: math.Abs(tax - comparedFiling), Evidence: e.Evidence, Recommendation: recommendation, Confidence: "中", VerificationRequired: true}
				}
			}
			results = append(results, e)
		}
	}
	return results
}

func analysisInvoiceTaxComparison(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "invoice.tax-comparison"}
	statistics := []invoiceComparisonStatistics{{key: "output", label: "销项"}, {key: "input", label: "进项有效抵扣"}, {key: "all", label: "进销项合计"}}
	var focus []string
	largest := 0.0
	for _, c := range invoiceTaxComparisons(ctx) {
		for _, input := range []bool{false, true} {
			if input && ctx.taxpayer == taxpayerSmall {
				continue
			}
			statIndex := 0
			if input {
				statIndex = 1
			}
			name, tax, known, filing, filingKnown, source := "销项票面税额", c.Output, c.HasOutput, c.VAT.OutputTax, c.VAT.HasOutputTax, "issued_invoices/chunks/groups"
			if input {
				name, tax, known, filing, filingKnown, source = "已勾选有效抵扣税额", c.Input, c.HasInput, c.VAT.InputTax, c.VAT.HasInputTax, "deduction_confirmations/chunks/list/selectedTax"
			}
			prefix := "tax-match." + analysisToken(name+"|"+c.VAT.Period)
			if !known || !filingKnown {
				statistics[statIndex].missing++
				statistics[2].missing++
				statistics[statIndex].refs = append(statistics[statIndex].refs, "vat_returns/"+c.VAT.RecordID, source)
				statistics[2].refs = append(statistics[2].refs, "vat_returns/"+c.VAT.RecordID, source)
				if input && ctx.taxpayer == taxpayerGeneral {
					id := d.text(prefix+".coverage", "进项票申比对数据状态", "coverage", c.VAT.Period+"有效抵扣税额或申报进项税额未完整取得，不能计算差额和差异率；未取得不代表零抵扣", source)
					d.focus(prefix+".coverage", "指出无法比对的期间和缺失数据，不把未知金额补零，不由缺数据判定税务风险。", id)
				}
				continue
			}
			period := d.text(prefix+".period", "票申比较所属期", "period", c.VAT.Period, source)
			a := d.number(prefix+".ticket", name, "amount", "元", c.VAT.Period, "", "同属期有符号税额合计", tax, source)
			b := d.number(prefix+".filing", "对应申报税额", "amount", "元", c.VAT.Period, "", "同属期申报对应税额", filing, "vat_returns/"+c.VAT.RecordID)
			diff := d.number(prefix+".difference", name+"与对应申报差额", "difference", "元", c.VAT.Period, "", "对应税额−申报税额", tax-filing, a, b)
			comparedFiling := filing
			rate := invoiceTaxDifferenceRate(tax, filing)
			r := d.number(prefix+".rate", "票申差异率", "ratio", "%", c.VAT.Period, "", "绝对差额/MAX(两税额绝对值,1)×100", rate*100, a, b)
			if !input {
				if unbilled, ok, refs := outputUnbilledTax(ctx, c.VAT); ok {
					comparedFiling -= unbilled
					adjustment := d.number(prefix+".unbilled-tax", "未开票申报销项税额", "amount", "元", c.VAT.Period, "", "附表一一般计税全部征税项目未开票税额合计", unbilled, refs...)
					diff = d.number(prefix+".remaining", "已知调节后剩余差额（票面减调节后申报）", "difference", "元", c.VAT.Period, "", "发票销项税额−（申报销项税额−未开票申报税额）", tax-(filing-unbilled), a, b, adjustment)
					r = d.number(prefix+".remaining-rate", "已知调节后差异率", "ratio", "%", c.VAT.Period, "", "剩余绝对差额/MAX(发票税额与调节后申报税额绝对值,1)×100", invoiceTaxDifferenceRate(tax, filing-unbilled)*100, a, b, adjustment)
				}
			}
			statistics[statIndex].add(c.VAT.Period, tax-comparedFiling, tax, comparedFiling, diff, a, b)
			statistics[2].add(c.VAT.Period, tax-comparedFiling, tax, comparedFiling, diff)
			if focus == nil || math.Abs(tax-filing) > largest {
				largest = math.Abs(tax - filing)
				focus = []string{period, a, b, diff, r}
			}
		}
	}
	for _, stat := range statistics {
		if stat.key != "input" || ctx.taxpayer != taxpayerSmall {
			stat.appendFacts(&d)
		}
	}
	if len(focus) > 0 {
		d.focus("tax-reconciliation", "说明主要销项或进项差异的所属期、两个对应税额、差额和差异率，并说明已取得的合理调节证据与尚不能解释的部分。进销差额不得使用票申阈值。", focus...)
	}
	d.Constraints = []string{"进项趋势采用已勾选有效抵扣税额；不以进项票面税额冒充有效抵扣。缺用途确认不能判断抵扣合规。", "差异超过10%仍需完成调节或直接异常状态证据；本接口未提供完整调节结论，不自动判高。", "小规模不适用进项抵扣核对；销项票面与申报销项字段缺失时不以应纳税额代替。"}
	return d
}
