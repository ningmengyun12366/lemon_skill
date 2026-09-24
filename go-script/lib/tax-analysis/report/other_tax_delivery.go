package report

import "strings"

// quarterlyOtherTaxPaymentRows sums period-aligned payments and compares only
// adjacent observed quarters. An absent quarter is not a zero payment.
func quarterlyOtherTaxPaymentRows(ctx *reportContext, match func(string) bool) [][]string {
	amounts := map[string]float64{}
	for _, payment := range ctx.payments {
		if !match(payment.TaxName) || payment.TaxPeriod.Cycle == taxCycleAnnual {
			continue
		}
		period := quarterlyPeriodLabel(payment.Period, ctx.task.StartTime, ctx.task.EndTime)
		amounts[period] += payment.Amount
	}
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, true)
	rows := [][]string{}
	for i, period := range periods {
		amount, known := amounts[period]
		if !known {
			continue
		}
		change := "—"
		if i > 0 {
			if prior, ok := amounts[periods[i-1]]; ok {
				change = moneyText(amount - prior)
			}
		}
		rows = append(rows, []string{period, moneyText(amount), change, "实缴金额按税款所属期归集"})
	}
	if len(rows) == 0 {
		return [][]string{{"—", "—", "—", "本期未取得可按季度归集的缴款记录，不能据此认定未申报或未缴税"}}
	}
	return rows
}

type surtaxPeriodSummary struct {
	Declared, Paid, Basis                               float64
	DeclaredKnown, PaidKnown, BasisKnown, BasisConflict bool
}

// surtaxPeriodSummaries counts a common main-tax base once across three
// surtaxes. Different returned bases are not silently added or averaged.
func surtaxPeriodSummaries(ctx *reportContext) map[string]*surtaxPeriodSummary {
	result := map[string]*surtaxPeriodSummary{}
	get := func(period string) *surtaxPeriodSummary {
		if result[period] == nil {
			result[period] = &surtaxPeriodSummary{}
		}
		return result[period]
	}
	for _, line := range ctx.taxLines {
		if !containsAny(line.TaxName, "城市维护建设税", "教育费附加", "地方教育附加") {
			continue
		}
		item := get(line.Period.BucketMonth)
		if amount, ok := line.amount("本期应补（退）税（费）额", "应补（退）税额"); ok {
			item.Declared += amount
			item.DeclaredKnown = true
		}
		if basis, ok := line.basis(); ok {
			if item.BasisKnown && abs(item.Basis-basis) >= invoiceAggregationTolerance {
				item.BasisConflict = true
			}
			item.Basis, item.BasisKnown = basis, true
		}
	}
	for _, payment := range ctx.payments {
		if !containsAny(payment.TaxName, "城市维护建设税", "教育费附加", "地方教育附加") {
			continue
		}
		item := get(payment.Period)
		item.Paid += payment.Amount
		item.PaidKnown = true
	}
	return result
}

func surtaxDeliveryBlocks(ctx *reportContext) []Block {
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, false)
	items := surtaxPeriodSummaries(ctx)
	rows := [][]string{}
	declared, ratios := make([]float64, len(periods)), make([]float64, len(periods))
	declaredKnown, ratioKnown := make([]bool, len(periods)), make([]bool, len(periods))
	for i, period := range periods {
		item := items[period]
		if item == nil {
			continue
		}
		basisKnown := item.BasisKnown && !item.BasisConflict
		known := basisKnown && item.Basis != 0 && item.DeclaredKnown
		ratio := safeRatio(item.Declared, item.Basis)
		declared[i], declaredKnown[i], ratios[i], ratioKnown[i] = item.Declared, item.DeclaredKnown, ratio*100, known
		rows = append(rows, []string{period, optionalMoney(item.Declared, item.DeclaredKnown), optionalMoney(item.Basis, basisKnown), optionalPercent(ratio, known), optionalMoney(item.Paid, item.PaidKnown)})
	}
	// Retain per-return recomputations in the evidence ledger only.
	_ = surtaxReconciliationRows(ctx)
	chart := &Chart{ID: "surtax-trend", Kind: chartKindDualAxis, Title: "城市维护建设税及附加趋势", Labels: periods, Unit: "元", SecondaryUnit: "%", Series: []Series{
		{Name: "附加税费申报额", Values: declared, Available: declaredKnown, Color: chartColorPrimary, RenderAs: chartRenderLine},
		{Name: "附加与主税计税基础比例", Values: ratios, Available: ratioKnown, Color: chartColorPositive, Axis: chartAxisRight, RenderAs: chartRenderLine},
	}}
	blocks := []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}, {Kind: BlockTable, Title: "附加税费与主税同期比较", Headers: []string{"所属期", "附加税费申报合计", "增值税/消费税计税基础", "附加与计税基础比例", "附加实缴合计"}, Rows: rows}}
	rates, reductions := []string{}, []string{}
	for _, line := range ctx.taxLines {
		if !containsAny(line.TaxName, "城市维护建设税", "教育费附加", "地方教育附加") {
			continue
		}
		if rate := line.rateText(); rate != "—" {
			rates = appendUnique(rates, line.TaxName+rate)
		}
		if reduction, ok := line.amount("减征比例", "减征比例（%）"); ok {
			reductions = appendUnique(reductions, percentText(reduction))
		}
	}
	if len(rates) > 0 {
		text := "各期申报使用的税率包括" + strings.Join(rates, "、") + "。"
		if len(reductions) > 0 {
			text += "申报减征比例包括" + strings.Join(reductions, "、") + "；附加与计税基础比例反映减免后的实际申报金额。"
		}
		blocks = append(blocks, Block{Kind: BlockParagraph, Title: "口径说明", Paragraphs: []string{text}})
	}
	return blocks
}

func unionDeliveryBlocks(ctx *reportContext) []Block {
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, true)
	declared, paid := map[string]float64{}, map[string]float64{}
	declaredKnown, paidKnown := map[string]bool{}, map[string]bool{}
	for _, fact := range ctx.declarations {
		if !containsAny(fact.TaxName, "工会经费", "工会筹备金") || !fact.HasDeclared || fact.Period.Cycle == taxCycleAnnual {
			continue
		}
		quarter := quarterlyPeriodLabel(fact.Period.BucketMonth, ctx.task.StartTime, ctx.task.EndTime)
		declared[quarter] += fact.Declared
		declaredKnown[quarter] = true
	}
	for _, fact := range ctx.payments {
		if !containsAny(fact.TaxName, "工会经费", "工会筹备金") {
			continue
		}
		quarter := quarterlyPeriodLabel(fact.Period, ctx.task.StartTime, ctx.task.EndTime)
		paid[quarter] += fact.Amount
		paidKnown[quarter] = true
	}
	rows := [][]string{}
	for i, period := range periods {
		change := "—"
		if i > 0 && declaredKnown[period] && declaredKnown[periods[i-1]] {
			change = moneyText(declared[period] - declared[periods[i-1]])
		}
		if declaredKnown[period] || paidKnown[period] {
			rows = append(rows, []string{period, optionalMoney(declared[period], declaredKnown[period]), optionalMoney(paid[period], paidKnown[period]), change})
		}
	}
	return []Block{{Kind: BlockTable, Title: "工会经费申报缴纳趋势", Headers: []string{"季度", "申报金额", "实缴金额", "申报额较上季变化"}, Rows: rows}, {Kind: BlockParagraph, Title: "数据说明", Paragraphs: []string{"本期未取得与工会经费同期间的工资薪金申报基数，暂不比较工资规模与计费金额。"}}}
}

// mainCITPreferenceRows uses one main-form line per benefit and return. An
// annual/quarterly total is never added to its annex breakdown.
func mainCITPreferenceRows(ctx *reportContext) [][]string {
	rows := [][]string{}
	for _, f := range citPreferenceAmountFields(ctx) {
		label := normalizeFinancialMetricLabel(f.RowLabel)
		rows = append(rows, []string{label, f.Period.Display + "：" + moneyText(f.Value), "申报金额不替代优惠资格及备查资料核验"})
	}
	for _, f := range citResearchAmountFields(ctx.taxFields) {
		rows = append(rows, []string{"研发费用加计扣除", f.Period.Display + "：" + moneyText(f.Value), "金额及占收入比例见研发加计扣除申报情况"})
	}
	return rows
}
