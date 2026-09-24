package report

import "strings"

// analysisFinancialAmounts complements rate comparisons with their actual
// amounts, preserving the statement period and unknown other profit drivers.
func analysisFinancialAmounts(ctx *reportContext) []SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "financial.period-expense"}
	deduction := SectionAnalysisData{SectionID: "cit.deduction-evidence"}
	previous := map[string]financialFact{}
	latest := map[string]AnalysisFocus{}
	for _, f := range ctx.financial {
		prefix := "profit-amounts." + analysisToken(f.Period)
		base, haveBase := previous[f.TaxPeriod.Cycle]
		haveBase = haveBase && adjacentAnalysisPeriods(base.Period, f.Period)
		var changes []string
		for _, m := range []struct {
			key, name string
			value     func(financialFact) (float64, bool)
		}{
			{"revenue", "营业收入", func(x financialFact) (float64, bool) { return x.Revenue, x.HasRevenue }},
			{"cost", "营业成本", func(x financialFact) (float64, bool) { return x.Cost, x.HasCost }},
			{"gross-profit", "毛利金额", func(x financialFact) (float64, bool) { return x.Revenue - x.Cost, x.HasRevenue && x.HasCost }},
			{"expenses", "期间费用合计", financialExpenseTotal},
			{"profit", "利润总额", func(x financialFact) (float64, bool) { return x.Profit, x.HasProfit }},
		} {
			value, ok := m.value(f)
			if !ok {
				continue
			}
			id := d.number(prefix+"."+m.key+".value", m.name, "amount", "元", f.Period, "", "同一所属期金额，企业准则累计还原当期；毛利=收入−成本，期间费用按研发列报去重", value, "income_statements/"+f.Period)
			if prior, known := m.value(base); haveBase && known {
				priorID := metricID(d.SectionID, "profit-amounts."+analysisToken(base.Period)+"."+m.key+".value")
				changes = append(changes, d.number(prefix+"."+m.key+".change", m.name+"较上期变化", "change", "元", f.Period, base.Period, "本期金额−相邻同周期上期金额", value-prior, id, priorID))
			}
		}
		if len(changes) > 0 {
			latest[f.TaxPeriod.Cycle] = AnalysisFocus{ID: d.SectionID + "." + prefix, Goal: "结合最近可比期间的金额和比率提炼主要变化；利润的各项贡献及未分配金额使用profit-change，不把账面贡献解释成已确认的业务原因。", Required: true, EvidenceRefs: changes}
		}
		previous[f.TaxPeriod.Cycle] = f
		addDeductionAmountAnalysis(&deduction, ctx, f)
	}
	for _, cycle := range sortedMapKeys(latest) {
		d.Focus = append(d.Focus, latest[cycle])
	}
	addFinancialNormalizationFacts(&d, ctx.financial, financialMetricRevenue, financialMetricCost, financialMetricSalesExpense, financialMetricAdminExpense, financialMetricRDExpense, financialMetricFinanceCost, financialMetricProfit)
	d.Constraints = []string{"毛利率上升可以与毛利金额下降同时发生。费用率上升也可能受到收入分母下降影响，不能仅由三项比率认定利润金额变化由费用端主导。", "使用profit-change已取得项目的金额贡献和未分配差额解释损益变化；数据是否充分沿用该组状态，不预设全部缺失，不把金额关系当作已确认的经营原因。"}
	deduction.Constraints = []string{"成本费用与进项发票未调节差额=营业成本+期间费用−进项发票净额；不涉及营业收入，不能重命名为收入与成本费用差额，也不等于确定缺票金额。", "年度与季度分别比较，年度差额不与季度差额再相加；工资社保、折旧摊销、存货变动和跨期等只有同口径证据充分时才调节。"}
	addKnownDeductionFacts(&deduction, ctx)
	return []SectionAnalysisData{d, deduction}
}

// These tax-basis annual totals are reconciliation leads, not automatic
// deductions from accounting costs or substitutes for payroll declarations.
func addKnownDeductionFacts(d *SectionAnalysisData, ctx *reportContext) {
	seen := map[string]bool{}
	for _, f := range ctx.taxFields {
		if f.Module != "cit_annual_returns" || f.Status != "present" || canonicalFormLabel(f.RowLabel) != "合计" {
			continue
		}
		name := ""
		if containsAny(f.FormCode+f.FormName, "A105050", "职工薪酬") && canonicalFormLabel(f.ColumnLabel) == "税收金额" {
			name = "年度职工薪酬税收金额"
		}
		if containsAny(f.FormCode+f.FormName, "A105080", "折旧、摊销") && canonicalFormLabel(f.ColumnLabel) == "税收折旧、摊销额" {
			name = "年度税收折旧摊销金额"
		}
		key := f.RecordID + "|" + f.FormCode + "|" + name
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		id := d.number("deduction-known."+analysisToken(key), name, "reconciliation-lead", "元", f.Period.Display, "", "对应年度附表合计原值；尚未核对财报归集，不直接扣减未调节差额", f.Value, "report-model.json/taxFields[id="+f.ID+"]")
		d.focus("deduction-known."+analysisToken(key), "讨论成本费用与发票差额时同时说明已取得的薪酬、折旧等非发票成本线索；未核对账务归集，不直接用税收口径抵减财报差额，也不认定全部差额属于缺票。", id)
	}
}

type deductionAmountValues struct {
	Expenses, Purchases           float64
	ExpensesKnown, PurchasesKnown bool
}

// deductionAmounts is shared by the displayed table and analysis facts.
func deductionAmounts(ctx *reportContext, f financialFact) deductionAmountValues {
	purchases, purchasesOK := invoiceAmountForQuarter(ctx.received, normalizeQuarter(f.Period))
	if strings.Contains(f.Period, "年度") {
		purchases, purchasesOK = invoiceAmountForAnnualPeriod(ctx.received, f.TaxPeriod, ctx.modules["received_invoices"].Complete)
	}
	expenses, expensesOK := financialExpenseTotal(f)
	return deductionAmountValues{expenses, purchases, expensesOK, purchasesOK}
}

func addDeductionAmountAnalysis(d *SectionAnalysisData, ctx *reportContext, f financialFact) {
	if !f.HasCost {
		return
	}
	v := deductionAmounts(ctx, f)
	key := "deduction-gap." + analysisToken(f.Period)
	refs := []string{d.number(key+".cost", "营业成本", "amount", "元", f.Period, "", "同所属期财报营业成本", f.Cost, "income_statements/"+f.Period)}
	if v.ExpensesKnown {
		refs = append(refs, d.number(key+".expenses", "期间费用", "amount", "元", f.Period, "", "销售+管理+财务费用，研发独立列报时再计入研发费用", v.Expenses, "income_statements/"+f.Period))
	}
	if v.PurchasesKnown {
		refs = append(refs, d.number(key+".purchases", "进项发票净额", "amount", "元", f.Period, "", "同所属期有效进项发票有符号净额", v.Purchases, "received_invoices/"+f.Period))
	}
	if !v.ExpensesKnown || !v.PurchasesKnown {
		return
	}
	id := d.number(key+".difference", "成本费用与进项发票未调节差额", "difference", "元", f.Period, "", "营业成本+期间费用−进项发票净额；工资社保、折旧摊销、存货及跨期等尚未调节", f.Cost+v.Expenses-v.Purchases, refs...)
	d.text(key+".basis", "未调节差额比较对象", "basis", "财报营业成本及期间费用，与同期进项发票净额比较；不比较营业收入，不等于确定缺票", id)
}
