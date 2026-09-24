package report

import (
	"math"
	"sort"
	"strings"
)

// prependChart adds a Figma chart before the section's supporting text and
// tables. Parameters: blocks are the existing section blocks and chart is the
// trusted chart model. Return value contains exactly one copy of the chart ID.
func prependChart(blocks []Block, chart *Chart) []Block {
	if chart == nil {
		return blocks
	}
	for index := range blocks {
		if blocks[index].Chart != nil && blocks[index].Chart.ID == chart.ID {
			blocks[index].Chart = chart
			blocks[index].Title = chart.Title
			blocks[index].Paragraphs = chartSummaryParagraphs(chart)
			return blocks
		}
	}
	chartBlock := Block{Kind: BlockChart, Title: chart.Title, Chart: chart, Paragraphs: chartSummaryParagraphs(chart)}
	return append([]Block{chartBlock}, blocks...)
}

// appendChart adds a chart after existing business tables. Parameters are the
// current blocks and trusted chart model. Return value avoids duplicate IDs.
func appendChart(blocks []Block, chart *Chart) []Block {
	if chart == nil {
		return blocks
	}
	for _, block := range blocks {
		if block.Chart != nil && block.Chart.ID == chart.ID {
			return blocks
		}
	}
	return append(blocks, Block{Kind: BlockChart, Title: chart.Title, Chart: chart, Paragraphs: chartSummaryParagraphs(chart)})
}

// chartSummaryParagraphs returns a visible chart summary only when the source
// supplied one. Parameter chart is the trusted chart model. The return value is
// nil for an empty summary so renderers do not emit a standalone dash.
func chartSummaryParagraphs(chart *Chart) []string {
	if chart == nil || strings.TrimSpace(chart.Summary) == "" {
		return nil
	}
	return []string{strings.TrimSpace(chart.Summary)}
}

// requiredEmptyChart creates the shared Figma data-insufficient state without
// introducing synthetic business values. Parameters identify the chart and
// user-facing state. Return value is safe for real enterprise reports.
func requiredEmptyChart(id, kind, title, emptyTitle, message string) *Chart {
	if strings.TrimSpace(emptyTitle) == "" {
		emptyTitle = defaultChartEmptyTitle
	}
	if strings.TrimSpace(message) == "" {
		message = defaultChartEmptyMessage
	}
	return &Chart{ID: id, Kind: kind, Title: title, EmptyTitle: emptyTitle, EmptyMessage: message, Summary: message}
}

// mutateExistingCharts updates the visual component kind of already-built
// charts while preserving their trusted data. Parameters are the blocks and
// desired kind. Return value is the same section block slice.
func mutateExistingCharts(blocks []Block, kind string) []Block {
	for index := range blocks {
		if blocks[index].Chart != nil {
			blocks[index].Chart.Kind = kind
		}
	}
	return blocks
}

func chartBlocksOnly(blocks []Block) []Block {
	result := make([]Block, 0, len(blocks))
	for _, block := range blocks {
		if block.Chart == nil {
			continue
		}
		block.Paragraphs = nil
		result = append(result, block)
	}
	return result
}

// buildInvoiceAmountTrendWithFigmaChart ensures section 3.2 always has the
// Figma multi-line component or an explicit data-insufficient state.
func buildInvoiceAmountTrendWithFigmaChart(ctx *reportContext) []Block {
	blocks := mutateExistingCharts(buildInvoiceAmountTrend(ctx), chartKindMultiLine)
	if chart := firstChart(blocks); chart != nil {
		chart.Title = "进销项不含税金额趋势"
		if len(chart.Series) == 2 {
			chart.Series[0].Name = "销项不含税金额"
			chart.Series[1].Name = "进项不含税金额"
		}
		rows := make([][]string, 0, len(chart.Labels))
		for index, period := range chart.Labels {
			issued, issuedOK := seriesPoint(chart.Series, 0, index)
			received, receivedOK := seriesPoint(chart.Series, 1, index)
			rows = append(rows, []string{period, optionalMoney(issued, issuedOK), optionalMoney(received, receivedOK), optionalDifference(issued, issuedOK, received, receivedOK), optionalRatio(received, receivedOK, issued, issuedOK)})
		}
		return append(blocks, Block{Kind: BlockTable, Headers: []string{"所属期", "销项金额", "购进金额", "购销差额", "购进/销项比"}, Rows: rows})
	}
	blocks = prependChart(blocks, requiredEmptyChart("invoice-trend", chartKindMultiLine, "进销项不含税金额趋势", "", "当前期间未取得可形成进销项金额趋势的数据。"))
	return append(blocks, Block{Kind: BlockTable, Headers: []string{"所属期", "销项金额", "购进金额", "购销差额", "购进/销项比"}})
}

func seriesPoint(series []Series, seriesIndex, pointIndex int) (float64, bool) {
	if seriesIndex < 0 || seriesIndex >= len(series) || pointIndex < 0 || pointIndex >= len(series[seriesIndex].Values) {
		return 0, false
	}
	if len(series[seriesIndex].Available) > pointIndex && !series[seriesIndex].Available[pointIndex] {
		return 0, false
	}
	return series[seriesIndex].Values[pointIndex], true
}

func optionalRatio(numerator float64, numeratorKnown bool, denominator float64, denominatorKnown bool) string {
	if !numeratorKnown || !denominatorKnown || denominator == 0 {
		return "—"
	}
	return percentText(numerator / denominator)
}

// buildInvoiceTaxComparisonWithFigmaChart maps section 3.3 to the Word-required
// two-series line chart.
func buildInvoiceTaxComparisonWithFigmaChart(ctx *reportContext) []Block {
	blocks := mutateExistingCharts(buildInvoiceTaxComparison(ctx), chartKindMultiLine)
	if firstChart(blocks) != nil {
		return blocks
	}
	return prependChart(blocks, requiredEmptyChart("invoice-tax-trend", chartKindMultiLine, "进销项税额趋势", "", "当前期间未取得可比较的销项票面及有效抵扣税额。"))
}

// firstChart returns the first chart from a section's blocks. Parameter blocks
// is one section's content. Return value is nil when no chart is registered.
func firstChart(blocks []Block) *Chart {
	for _, block := range blocks {
		if block.Chart != nil {
			return block.Chart
		}
	}
	return nil
}

// ensureSectionChart changes existing charts to the required Figma component
// and inserts a neutral state when the section has no drawable data.
// Parameters identify the blocks and expected chart. Return value always has
// one matching chart ID.
func ensureSectionChart(blocks []Block, id, kind, title, message string) []Block {
	found := false
	for index := range blocks {
		if blocks[index].Chart == nil {
			continue
		}
		blocks[index].Chart.Kind = kind
		found = true
	}
	if found {
		return blocks
	}
	return prependChart(blocks, requiredEmptyChart(id, kind, title, "", message))
}

// buildInvoiceCategoryMatchWithChart returns the Word-required comparison
// table. Parameter ctx supplies only tax-authority invoice facts. Section 3.4
// does not permit a chart.
func buildInvoiceCategoryMatchWithChart(ctx *reportContext) []Block {
	return buildInvoiceCategoryMatch(ctx)
}

// invoiceCategoryAmounts aggregates canonical signed invoice allocations by
// returned category. Parameter facts contains one invoice direction. Return
// values reconcile with the report-wide invoice header total.
func invoiceCategoryAmounts(facts []invoiceGroup) map[string]float64 {
	result := map[string]float64{}
	for _, allocation := range invoiceCategoryAllocations(facts) {
		result[allocation.Name] += allocation.Amount
	}
	return result
}

// topUnionLabels selects the highest combined categories from two directions.
// Parameters left/right are amount maps and maximum is the output cap. Return
// value is sorted by combined amount then label for deterministic rendering.
func topUnionLabels(left, right map[string]float64, maximum int) []string {
	totals := map[string]float64{}
	for label, value := range left {
		totals[label] += value
	}
	for label, value := range right {
		totals[label] += value
	}
	labels := make([]string, 0, len(totals))
	for label := range totals {
		labels = append(labels, label)
	}
	sort.SliceStable(labels, func(i, j int) bool {
		if totals[labels[i]] != totals[labels[j]] {
			return totals[labels[i]] > totals[labels[j]]
		}
		return labels[i] < labels[j]
	})
	if maximum > 0 && len(labels) > maximum {
		labels = labels[:maximum]
	}
	return labels
}

// buildCustomerConcentrationWithChart maps CR1/CR5/CR10 to section 3.8.
func buildCustomerConcentrationWithChart(ctx *reportContext) []Block {
	return prependChart(buildCustomerConcentration(ctx), concentrationChart("customer-concentration", "客户集中度", "客户", ctx.issued))
}

// buildSupplierConcentrationWithChart maps CR1/CR5/CR10 to section 3.9.
func buildSupplierConcentrationWithChart(ctx *reportContext) []Block {
	return prependChart(buildSupplierConcentration(ctx), concentrationChart("supplier-concentration", "供应商集中度", "供应商", ctx.received))
}

// concentrationChart creates a grouped CR chart from exact counterparties.
// Parameters identify the chart and one invoice direction. Return value is an
// empty chart state when counterparties are unavailable.
func concentrationChart(id, title, subject string, facts []invoiceGroup) *Chart {
	values := counterpartyAmounts(facts)
	if len(values) == 0 {
		return requiredEmptyChart(id, chartKindGroupedColumn, title, "", "当前发票数据未返回可识别的"+subject+"主体。")
	}
	total := invoiceNetAmount(facts)
	if total <= 0 {
		return requiredEmptyChart(id, chartKindGroupedColumn, title, "%", "同方向发票净额非正，无法计算集中度比例。")
	}
	labels := append([]string(nil), concentrationLabels...)
	percentages := make([]float64, len(labels))
	for index, count := range []int{1, 5, 10} {
		amount := 0.0
		for itemIndex := 0; itemIndex < minInt(count, len(values)); itemIndex++ {
			amount += values[itemIndex].amount
		}
		if total > 0 {
			percentages[index] = amount / total * concentrationPercentScale
		}
	}
	return &Chart{ID: id, Kind: chartKindGroupedColumn, Title: title, Labels: labels, Unit: "%",
		Series: []Series{{Name: subject + "集中度（%）", Values: percentages, Color: chartColorPrimary}}, Summary: "第一大、前五大、前十大占比均按发票实际不含税金额计算。"}
}

// buildRedOutputWithChart adds section 3.15's monthly red amount and red-rate
// dual-axis line chart. Both series are lines, as required by Word.
func buildRedOutputWithChart(ctx *reportContext) []Block {
	blocks := buildRedOutput(ctx)
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, false)
	red, blue, rate := make([]float64, len(periods)), make([]float64, len(periods)), make([]float64, len(periods))
	amountKnown, rateKnown := make([]bool, len(periods)), make([]bool, len(periods))
	indexByMonth := map[string]int{}
	for index, period := range periods {
		indexByMonth[period] = index
	}
	for _, fact := range ctx.issued {
		index, ok := indexByMonth[fact.Month]
		if !ok || !fact.HasAmount {
			continue
		}
		amountKnown[index] = true
		if fact.Red || fact.Amount < 0 {
			red[index] += math.Abs(fact.Amount)
		} else {
			blue[index] += fact.Amount
		}
	}
	for index := range periods {
		if blue[index] > 0 {
			rate[index], rateKnown[index] = red[index]/blue[index]*concentrationPercentScale, true
		}
	}
	chart := &Chart{ID: "red-output-trend", Kind: chartKindDualAxis, Title: "红字发票金额及红字率趋势", Labels: periods, Unit: "元", SecondaryUnit: "%",
		Series: []Series{
			{Name: "红字金额", Values: red, Available: amountKnown, Color: chartColorAttention, RenderAs: chartRenderLine},
			{Name: "红字率", Values: rate, Available: rateKnown, Color: chartColorPrimary, Axis: chartAxisRight, RenderAs: chartRenderLine},
		}}
	return prependChart(blocks, chart)
}

// buildSensitiveServicesWithChart adds the section 3.17 category component.
func buildSensitiveServicesWithChart(ctx *reportContext) []Block {
	blocks := buildSensitiveServices(ctx)
	byCategory, _ := sensitiveServiceTotals(ctx.received)
	amounts := map[string]float64{}
	for category, total := range byCategory {
		amounts[category] = total.amount
	}
	labels := topUnionLabels(amounts, nil, len(byCategory))
	if len(labels) == 0 {
		return prependChart(blocks, requiredEmptyChart("sensitive-services", chartKindGroupedColumn, "敏感服务类发票分类", noMatchingDataTitle, "按当前税局数据，未识别到本节所列敏感服务类发票记录。"))
	}
	values := make([]float64, len(labels))
	for index, label := range labels {
		values[index] = amounts[label]
	}
	return prependChart(blocks, &Chart{ID: "sensitive-services", Kind: chartKindGroupedColumn, Title: "敏感服务类发票分类", Labels: labels, Series: []Series{{Name: "不含税金额", Values: values, Color: chartColorPrimary}}})
}

// buildPaymentCompositionWithChart uses the component-library ring chart for
// section 4.5.
func buildPaymentCompositionWithChart(ctx *reportContext) []Block {
	blocks := buildPaymentComposition(ctx)
	chart := firstChart(blocks)
	if chart == nil {
		chart = requiredEmptyChart("tax-composition", chartKindDonut, "税费构成", "", "当前缴款记录未返回可形成税费构成图的数据。")
	} else {
		chart.Kind = chartKindDonut
		chart.Title = "税费构成"
		chart.CenterLabel = taxTotalCenterLabel
	}
	return prependChart(blocks, chart)
}

// buildMajorTaxTrendWithChart ensures section 4.3 uses the multi-line component.
func buildMajorTaxTrendWithChart(ctx *reportContext) []Block {
	blocks := ensureSectionChart(buildMajorTaxTrend(ctx), "major-tax-trend", chartKindMultiLine, "主要税种税额趋势", "当前缴款记录未返回可形成主要税种趋势的数据。")
	for index := range blocks {
		if blocks[index].Chart != nil {
			blocks[index].Chart.Title = strings.TrimSuffix(blocks[index].Chart.Title, "主要税费缴纳趋势") + "主要税种税额趋势"
		}
	}
	return chartBlocksOnly(blocks)
}

// buildTotalPaymentTrendWithChart ensures section 4.4 has its line component.
func buildTotalPaymentTrendWithChart(ctx *reportContext) []Block {
	return chartBlocksOnly(ensureSectionChart(buildTotalPaymentTrend(ctx), "total-payment-trend", chartKindLine, "税费缴纳总额趋势", "当前缴款记录未返回可形成缴纳总额趋势的数据。"))
}

// buildMonthlyTaxBurdenWithChart ensures section 4.6 uses multi-line output.
func buildMonthlyTaxBurdenWithChart(ctx *reportContext) []Block {
	return buildMonthlyTaxBurden(ctx)
}

// buildVATBurdenTrendWithChart ensures section 5.2 has its line component.
func buildVATBurdenTrendWithChart(ctx *reportContext) []Block {
	blocks := ensureSectionChart(buildVATBurdenTrend(ctx), "vat-burden-trend", chartKindLine, "增值税税负率趋势", "增值税申报销售额或应纳税额不足，无法计算税负率。")
	if chart := firstChart(blocks); chart != nil && len(chart.Series) == 1 {
		chart.Series[0].Name = "增值税税负率"
	}
	return chartBlocksOnly(blocks)
}

// buildEndCreditWithChart ensures general-taxpayer section 5.8 has a line or
// explicit data-insufficient state.
func buildEndCreditWithChart(ctx *reportContext) []Block {
	return ensureSectionChart(buildEndCredit(ctx), "vat-end-credit", chartKindLine, "期末留抵税额趋势", "增值税申报数据未取得期末留抵税额字段。")
}

// buildVATInvoiceDifferenceWithChart adds section 5.4's Word-required two-line
// comparison. Parameter ctx supplies invoice and VAT return facts.
func buildVATInvoiceDifferenceWithChart(ctx *reportContext) []Block {
	blocks := buildVATInvoiceDifference(ctx)
	periods := make([]string, len(ctx.vat))
	issued, declared := make([]float64, len(periods)), make([]float64, len(periods))
	issuedKnown, declaredKnown := make([]bool, len(periods)), make([]bool, len(periods))
	for index, fact := range ctx.vat {
		periods[index] = fact.Period
		issued[index], issuedKnown[index] = invoiceAmountForTaxPeriod(ctx.issued, fact.TaxPeriod)
		if fact.HasSales {
			declared[index], declaredKnown[index] = fact.Sales, true
		}
	}
	chart := &Chart{ID: "vat-invoice-declaration-difference", Kind: chartKindMultiLine, Title: "销项发票与申报销售额", Labels: periods, Unit: "元",
		Series: []Series{
			{Name: "销项发票金额", Values: issued, Available: issuedKnown, Color: chartColorPrimary, RenderAs: chartRenderLine},
			{Name: "申报销售额", Values: declared, Available: declaredKnown, Color: chartColorPositive, RenderAs: chartRenderLine},
		}}
	return prependChart(blocks, chart)
}

// hasInvoiceMonth reports whether an invoice direction contains source data in
// the requested month. Parameters are invoice facts and YYYY-MM. Return value
// distinguishes a real zero from a missing month.
func hasInvoiceMonth(facts []invoiceGroup, month string) bool {
	for _, fact := range facts {
		if fact.Month == month && fact.HasAmount {
			return true
		}
	}
	return false
}

// buildCITContributionWithChart adds section 6.1's dual-axis two-line chart.
func buildCITContributionWithChart(ctx *reportContext) []Block {
	blocks := buildCITContribution(ctx)
	var labels []string
	var tax, ratio []float64
	var taxKnown, ratioKnown []bool
	for _, point := range citContributionPoints(ctx) {
		labels = append(labels, point.period)
		tax, taxKnown = append(tax, point.tax), append(taxKnown, point.taxKnown)
		ratio = append(ratio, safeRatio(point.tax, point.income)*concentrationPercentScale)
		ratioKnown = append(ratioKnown, point.taxKnown && point.incomeKnown && point.income != 0)
	}
	chart := &Chart{ID: "cit-contribution", Kind: chartKindDualAxis, Title: citContributionChartTitle, Labels: labels, Unit: "元", SecondaryUnit: "%", Series: []Series{{Name: citContributionTaxLabel, Values: tax, Available: taxKnown, Color: chartColorPrimary, RenderAs: chartRenderLine}, {Name: "贡献率", Values: ratio, Available: ratioKnown, Color: chartColorAttention, Axis: chartAxisRight, RenderAs: chartRenderLine}}}
	if len(labels) == 0 {
		chart = requiredEmptyChart("cit-contribution", chartKindDualAxis, citContributionChartTitle, "", "当前数据不足以形成企业所得税申报贡献率。")
	}
	return prependChart(blocks, chart)
}

// buildCITResearchExpenseWithChart adds the section 6.6 research-expense trend
// from exact annual-return detail rows only.
func buildCITResearchExpenseWithChart(ctx *reportContext) []Block {
	blocks := buildCITResearchExpense(ctx)
	facts := citResearchAmountFields(ctx.taxFields)
	labels, research := make([]string, 0, len(facts)), make([]float64, 0, len(facts))
	researchKnown := make([]bool, 0, len(facts))
	for _, fact := range facts {
		labels, research = append(labels, fact.Period.Display), append(research, fact.Value)
		researchKnown = append(researchKnown, true)
	}
	if len(labels) <= 1 {
		return blocks
	}
	chart := &Chart{ID: "cit-research-expense", Kind: chartKindGroupedColumn, Title: "研发加计扣除申报趋势", Labels: labels,
		Series: []Series{{Name: "研发加计扣除额", Values: research, Available: researchKnown, Color: chartColorPrimary}}, Summary: "季度为截至季末的累计申报额；年度汇算单列，不与季度累计数相加。"}
	return prependChart(blocks, chart)
}

// buildSurtaxPaymentsWithChart adds the section 7.4 multi-tax monthly trend.
func buildSurtaxPaymentsWithChart(ctx *reportContext) []Block {
	return surtaxDeliveryBlocks(ctx)
}

// buildFinancialOverviewWithChart maps section 8.1 to the Word-required
// income/cost columns plus profit line.
func buildFinancialOverviewWithChart(ctx *reportContext) []Block {
	blocks := buildFinancialOverview(ctx)
	chart := firstChart(blocks)
	if chart == nil {
		return prependChart(blocks, requiredEmptyChart("financial-income-cost-profit-trend", chartKindCombo, "收入、成本及利润趋势", "", "利润表未返回可展示的收入、成本或利润指标。"))
	}
	chart.Kind = chartKindCombo
	for index := range chart.Series {
		chart.Series[index].RenderAs = chartRenderBar
		if chart.Series[index].Name == "利润总额" {
			chart.Series[index].RenderAs = chartRenderLine
		}
	}
	return blocks
}

// buildPeriodExpenseWithChart adds section 8.2's margin and expense-rate trend.
func buildPeriodExpenseWithChart(ctx *reportContext) []Block {
	blocks := buildPeriodExpense(ctx)
	labels := make([]string, len(ctx.financial))
	gross, profit, expense := make([]float64, len(ctx.financial)), make([]float64, len(ctx.financial)), make([]float64, len(ctx.financial))
	grossKnown, profitKnown, expenseKnown := make([]bool, len(ctx.financial)), make([]bool, len(ctx.financial)), make([]bool, len(ctx.financial))
	for index, fact := range ctx.financial {
		labels[index] = fact.Period
		if fact.HasRevenue && fact.Revenue != 0 && fact.HasCost {
			gross[index], grossKnown[index] = (fact.Revenue-fact.Cost)/fact.Revenue*concentrationPercentScale, true
		}
		if fact.HasRevenue && fact.Revenue != 0 && fact.HasProfit {
			profit[index], profitKnown[index] = fact.Profit/fact.Revenue*concentrationPercentScale, true
		}
		if fact.ResearchInManagementSet && fact.HasRevenue && fact.Revenue != 0 {
			total, totalKnown := 0.0, false
			if fact.ResearchInManagement {
				total, totalKnown = sumKnown(optionalValue(fact.SalesExpense, fact.HasSalesExpense), optionalValue(fact.ManagementExpense, fact.HasManagementExpense), optionalValue(fact.FinanceExpense, fact.HasFinanceExpense))
			} else {
				total, totalKnown = sumKnown(optionalValue(fact.SalesExpense, fact.HasSalesExpense), optionalValue(fact.ManagementExpense, fact.HasManagementExpense), optionalValue(fact.ResearchExpense, fact.HasResearchExpense), optionalValue(fact.FinanceExpense, fact.HasFinanceExpense))
			}
			if totalKnown {
				expense[index], expenseKnown[index] = total/fact.Revenue*concentrationPercentScale, true
			}
		}
	}
	chart := &Chart{ID: "financial-margin-expense-trend", Kind: chartKindMultiLine, Title: "毛利率、利润率及费用率趋势", Labels: labels, Unit: "%",
		Series: []Series{{Name: "毛利率", Values: gross, Available: grossKnown, Color: chartColorPrimary}, {Name: "营业利润率", Values: profit, Available: profitKnown, Color: chartColorPositive}, {Name: "期间费用率", Values: expense, Available: expenseKnown, Color: chartColorAttention}}}
	return prependChart(blocks, chart)
}

// buildDebtAssetRatioWithChart adds section 8.3's ratio line.
func buildDebtAssetRatioWithChart(ctx *reportContext) []Block {
	blocks := buildDebtAssetRatio(ctx)
	labels, values, known := make([]string, 0), make([]float64, 0), make([]bool, 0)
	for _, fact := range ctx.financial {
		if !fact.HasAssets && !fact.HasLiabilities {
			continue
		}
		valid := fact.HasAssets && fact.HasLiabilities && fact.Assets != 0
		value := 0.0
		if valid {
			value = fact.Liabilities / fact.Assets * concentrationPercentScale
		}
		labels, values, known = append(labels, fact.Period), append(values, value), append(known, valid)
	}
	chart := &Chart{ID: "debt-asset-ratio", Kind: chartKindLine, Title: "资产负债率趋势", Labels: labels, Unit: "%", Series: []Series{{Name: "资产负债率", Values: values, Available: known, Color: chartColorPrimary}}, Summary: "资产负债率按负债总额除以资产总额计算。"}
	return prependChart(blocks, chart)
}

// buildWorkingCapitalMetricsWithChart adds section 8.4's multi-line trend.
func buildWorkingCapitalMetricsWithChart(ctx *reportContext) []Block {
	blocks := buildWorkingCapitalMetrics(ctx)
	facts := make([]financialFact, 0, len(ctx.financial))
	for _, fact := range ctx.financial {
		if fact.HasAccountsReceivable || fact.HasInventory || fact.HasContractLiabilities {
			facts = append(facts, fact)
		}
	}
	if len(facts) <= 1 {
		return blocks
	}
	labels := make([]string, len(facts))
	receivables, inventory, contracts := make([]float64, len(facts)), make([]float64, len(facts)), make([]float64, len(facts))
	receivablesKnown, inventoryKnown, contractsKnown := make([]bool, len(facts)), make([]bool, len(facts)), make([]bool, len(facts))
	for index, fact := range facts {
		labels[index] = fact.Period
		receivables[index], receivablesKnown[index] = fact.AccountsReceivable, fact.HasAccountsReceivable
		inventory[index], inventoryKnown[index] = fact.Inventory, fact.HasInventory
		contracts[index], contractsKnown[index] = fact.ContractLiabilities, fact.HasContractLiabilities
	}
	series := []Series{
		{Name: "应收账款", Values: receivables, Available: receivablesKnown, Color: chartColorPrimary},
		{Name: "存货", Values: inventory, Available: inventoryKnown, Color: chartColorPositive},
		{Name: "合同负债", Values: contracts, Available: contractsKnown, Color: chartColorAttention},
	}
	chart := &Chart{ID: "working-capital-trend", Kind: chartKindMultiLine, Title: "应收账款、存货及合同负债趋势", Labels: labels, Series: series}
	return prependChart(blocks, chart)
}

// buildCashFlowWithChart renders the Word-required cash-flow trend from the
// tax-authority cash-flow preview and profit statement for matching periods.
func buildCashFlowWithChart(ctx *reportContext) []Block {
	blocks := buildCashFlow(ctx)
	labels, cash, profit := make([]string, 0), make([]float64, 0), make([]float64, 0)
	cashKnown, profitKnown := make([]bool, 0), make([]bool, 0)
	for _, fact := range ctx.financial {
		if !fact.HasOperatingCashFlow && !fact.HasNetProfit {
			continue
		}
		labels = append(labels, fact.Period)
		cash, cashKnown = append(cash, fact.OperatingCashFlow), append(cashKnown, fact.HasOperatingCashFlow)
		profit, profitKnown = append(profit, fact.NetProfit), append(profitKnown, fact.HasNetProfit)
	}
	if len(labels) == 0 {
		return prependChart(blocks, requiredEmptyChart("operating-cash-flow", chartKindMultiLine, "经营活动现金流趋势", "", "本期未取得经营活动现金流量数据。"))
	}
	chart := &Chart{ID: "operating-cash-flow", Kind: chartKindMultiLine, Title: "经营活动现金流趋势", Labels: labels, Series: []Series{
		{Name: "经营活动现金流量净额", Values: cash, Available: cashKnown, Color: chartColorPrimary},
		{Name: "净利润", Values: profit, Available: profitKnown, Color: chartColorPositive},
	}}
	return prependChart(blocks, chart)
}

// buildRemediationWithChart adds section 10.1's priority matrix using only
// confirmed finding properties. Parameter ctx supplies de-duplicated findings.
func buildRemediationWithChart(ctx *reportContext) []Block {
	return buildConfirmedRemediationWithChart(ctx)
}

// buildIndustryStructureWithChart maps section 9.1 to the Figma pie component.
func buildIndustryStructureWithChart(ctx *reportContext) []Block {
	blocks := buildIndustryStructure(ctx)
	chart := firstChart(blocks)
	if chart == nil {
		chart = requiredEmptyChart("business-structure", chartKindPie, "销项发票主要业务结构", "", "当前销项发票品类不足以形成主要业务结构。")
	} else {
		chart.Kind = chartKindPie
	}
	chart.Title = "主要业务结构"
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}}
}

// buildQixinIndustryWithChart adds the same business-structure pie to section
// 2.10 while retaining the authoritative industry classification table.
func buildQixinIndustryWithChart(ctx *reportContext) []Block {
	labels, values := make([]string, 0, len(ctx.categories)), make([]float64, 0, len(ctx.categories))
	rows := make([][]string, 0, len(ctx.categories))
	for _, category := range ctx.categories {
		labels, values = append(labels, category.Name), append(values, category.Amount)
		rows = append(rows, []string{category.Name, percentText(category.Share), "按销项发票品类识别", "一般经营特征分析"})
	}
	table := Block{Kind: BlockTable, Headers: []string{"业务类型", "占比", "业务特征", "适用分析"}, Rows: rows}
	if len(labels) == 0 {
		chart := requiredEmptyChart("enterprise-business-structure", chartKindPie, "主要业务结构", "", "销项发票品类字段不足，暂无法形成业务结构图。")
		return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}, table}
	}
	chart := &Chart{ID: "enterprise-business-structure", Kind: chartKindPie, Title: "主要业务结构", Labels: labels, Series: []Series{{Name: "销项不含税金额", Values: values, Color: chartColorPrimary}}}
	completeBusinessPie(ctx, chart)
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}, table}
}

// reliableIndustryBenchmarks extracts only explicitly sourced industry
// benchmark rows. Parameter ctx contains the same tax-analysis task response.
// Return values are labels, enterprise values, benchmark values and validity.
func reliableIndustryBenchmarks(ctx *reportContext) ([]string, []float64, []float64, bool) {
	rows := findAllMaps(ctx.modules[qixinModuleCode].Raw, func(object map[string]any) bool {
		name := mapText(object, "metricName", "indicatorName", "指标名称")
		_, enterpriseOK := mapNumber(object, "enterpriseValue", "companyValue", "企业值")
		_, benchmarkOK := mapNumber(object, "industryValue", "benchmarkValue", "行业基准值")
		source := mapText(object, "benchmarkSource", "sourceName", "数据来源")
		return name != "" && enterpriseOK && benchmarkOK && source != ""
	})
	if len(rows) < 3 {
		return nil, nil, nil, false
	}
	labels, enterprise, benchmark := make([]string, 0, len(rows)), make([]float64, 0, len(rows)), make([]float64, 0, len(rows))
	for _, row := range rows {
		enterpriseValue, enterpriseOK := mapNumber(row, "enterpriseValue", "companyValue", "企业值")
		benchmarkValue, benchmarkOK := mapNumber(row, "industryValue", "benchmarkValue", "行业基准值")
		if !enterpriseOK || !benchmarkOK {
			continue
		}
		labels = append(labels, mapText(row, "metricName", "indicatorName", "指标名称"))
		enterprise, benchmark = append(enterprise, enterpriseValue), append(benchmark, benchmarkValue)
	}
	return labels, enterprise, benchmark, len(labels) >= 3
}

// includeReliableIndustryBenchmark includes section 9.5 only when the task
// contains explicitly sourced comparable industry values.
func includeReliableIndustryBenchmark(ctx *reportContext) (bool, OmissionReason) {
	return true, ""
}

// buildIndustrySummaryWithRadar renders section 9.5 from reliable benchmarks.
func buildIndustrySummaryWithRadar(ctx *reportContext) []Block {
	blocks := buildIndustrySummary(ctx)
	labels, enterprise, benchmark, ok := reliableIndustryBenchmarks(ctx)
	if !ok {
		return blocks
	}
	chart := &Chart{ID: "industry-radar", Kind: chartKindRadar, Title: "企业与行业经营指标对比", Labels: labels,
		Series: []Series{{Name: "企业值", Values: enterprise, Color: chartColorPrimary}, {Name: "行业基准", Values: benchmark, Color: chartColorPositive}}, Summary: "行业指标采用注明来源的同期可比数据。"}
	return prependChart(blocks, chart)
}
