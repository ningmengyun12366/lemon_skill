package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	maxCategoryComparisonRows = 5
	concentrationPercentScale = 100
)

func includeInvoiceCategoryMatch(ctx *reportContext) (bool, OmissionReason) {
	if invoiceCount(ctx.issued)+invoiceCount(ctx.received) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func buildInvoiceCategoryMatch(ctx *reportContext) []Block {
	inputs, outputs := businessEvidenceGroups(ctx, "received_invoices"), businessEvidenceGroups(ctx, "issued_invoices")
	var rows [][]string
	for i, input := range inputs {
		if i >= maxCategoryComparisonRows {
			break
		}
		outputName, match := "—", "暂无可建立的直接对应；需结合库存、加工、外包及用途解释"
		for _, output := range outputs {
			if candidate := candidateBusinessLink(input.Name, output.Name); candidate != "" {
				outputName, match = output.Name, candidate
				break
			}
		}
		rows = append(rows, []string{input.Name, outputName, match, "购进" + moneyText(input.Amount)})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前未取得可用于购销品类匹配的完整品名资料。"}}}
	}
	return []Block{{Kind: BlockTable, Title: "主要进销品类结构", Headers: []string{"购进主要内容", "销项主要内容", "匹配判断", "关注"}, Rows: rows}}
}

func includeInvoiceBusinessMatch(ctx *reportContext) (bool, OmissionReason) {
	if len(ctx.categories) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func buildInvoiceBusinessMatch(ctx *reportContext) []Block {
	detail := qixinSubjectData(qixinOwnSubject(ctx), "detail")
	scope := firstText(detail, "Scope")
	rows := make([][]string, 0, len(ctx.categories))
	for _, category := range ctx.categories {
		result := "需结合品名、编码及实际业务作语义核对；文字差异不代表超范围"
		if scope == "" {
			result = "工商经营范围未取得，不作匹配判断"
		} else if businessTextOverlap(category.Name, scope) {
			result = "经营范围包含相关文字；实际业务仍需结合品名和编码核对"
		}
		code := invoiceCategoryTaxCodes(ctx.issued, category.Name)
		rows = append(rows, []string{category.Name, "税收分类编码" + code + "；经营范围：" + relatedScopeClause(category.Name, scope), result})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前销项发票数据未返回可与工商经营范围进行客观核对的主要开票品类。"}}}
	}
	return []Block{{Kind: BlockTable, Title: "开票内容与经营业务核对", Headers: []string{"检测维度", "结果", "结论"}, Rows: rows}}
}

// invoiceCategoryTaxCodes 汇总一个主要开票品类实际返回的税收分类编码。
//
// 参数 facts 为销项发票事实，category 为已聚合品类；返回去重编码，未取得时返回破折号。
func invoiceCategoryTaxCodes(facts []invoiceGroup, category string) string {
	codes := make([]string, 0)
	for _, fact := range facts {
		for _, line := range invoiceFactLines(fact) {
			if invoiceLineName(line) == category && strings.TrimSpace(line.TaxCode) != "" {
				codes = append(codes, strings.TrimSpace(line.TaxCode))
			}
		}
	}
	return textOr(strings.Join(uniqueSorted(codes), "、"), "—")
}

func includeConcentration(facts []invoiceGroup) (bool, OmissionReason) {
	if len(counterpartyAmounts(facts)) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func buildCustomerConcentration(ctx *reportContext) []Block {
	return buildConcentrationBlocks("客户", ctx.issued, ctx.task.StartTime, ctx.task.EndTime, ctx.invoiceCoverage["issued_invoices"].Complete)
}

func buildSupplierConcentration(ctx *reportContext) []Block {
	return buildConcentrationBlocks("供应商", ctx.received, ctx.task.StartTime, ctx.task.EndTime, ctx.invoiceCoverage["received_invoices"].Complete)
}

func buildConcentrationBlocks(subject string, facts []invoiceGroup, start, end string, complete bool) []Block {
	values := counterpartyAmounts(facts)
	if len(values) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前发票数据未返回可识别的" + subject + "主体，无法计算集中度。"}}}
	}
	total := invoiceNetAmount(facts)
	rows := make([][]string, 0, 3)
	shares, _, _ := concentrationShares(facts)
	level := "样本范围，暂不判级"
	if complete && total > 0 {
		level = textOr(string(concentrationLevel(shares[1], shares[2])), "一般")
	}
	for labelIndex, count := range []int{1, 5, 10} {
		covered := minInt(count, len(values))
		amount := 0.0
		for index := 0; index < covered; index++ {
			amount += values[index].amount
		}
		ratio := 0.0
		if total > 0 {
			ratio = amount / total
		}
		rows = append(rows, []string{concentrationLabels[labelIndex], optionalPercent(ratio, total > 0), level})
	}
	return []Block{{Kind: BlockTable, Title: subject + "集中度", Headers: []string{"指标", analysisPeriodText(start, end), "风险提示"}, Rows: rows}}
}

func includeQixinCounterparties(ctx *reportContext, role string) (bool, OmissionReason) {
	for _, subject := range qixinSubjects(ctx) {
		if mapText(subject, "role") == role {
			return true, ""
		}
	}
	return false, OmittedDataInsufficient
}

func buildQixinCustomerCredit(ctx *reportContext) []Block {
	return buildQixinCounterpartyCredit(ctx, qixinImportantCustomerRole)
}

func buildQixinSupplierCredit(ctx *reportContext) []Block {
	return buildQixinCounterpartyCredit(ctx, qixinImportantSupplierRole)
}

func buildQixinCounterpartyCredit(ctx *reportContext, role string) []Block {
	rows := make([][]string, 0)
	for _, subject := range qixinSubjects(ctx) {
		if mapText(subject, "role") != role {
			continue
		}
		executionCount := len(qixinItemMaps(qixinSubjectData(subject, "execution"), "CaseNumber", "Court", "Amount"))
		dishonestCount := len(qixinItemMaps(qixinSubjectData(subject, "dishonest"), "CaseNumber", "ExecutionStatus", "FinalDuty"))
		penaltyCount := len(qixinItemMaps(qixinSubjectData(subject, "adminPunishment"), "Number", "Department", "Content"))
		share, shareOK := invoiceCounterpartyShare(ctx, role, subject)
		credit := qixinListingSummary(subject, qixinDetailField, "经营异常") + "；" + qixinListingSummary(subject, qixinSeriousIllegalField, "严重违法") + fmt.Sprintf("；历史被执行%d项；历史失信被执行%d项；行政处罚%d项", executionCount, dishonestCount, penaltyCount)
		name := mapText(subject, "name")
		if link := qixinInvoiceNameLink(ctx, subject); link != "" {
			name += "（" + link + "）"
		}
		shareText := optionalPercent(share, shareOK)
		evaluation := counterpartyCreditEvaluation(ctx, subject)
		action := "—"
		if evaluation.Finding != nil {
			action = evaluation.Finding.Recommendation
		}
		rows = append(rows, []string{name, shareText, credit + "；" + counterpartyExecutionSummary(subject), action})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"—", "—", "当前报告未取得可精确核验的公开信用结果", "—"})
	}
	first := "客户"
	if role == qixinImportantSupplierRole {
		first = "供应商"
	}
	return []Block{{Kind: BlockTable, Headers: []string{first, "交易占比", "公开风险", "建议"}, Rows: rows}}
}

func includeInvoiceFieldQuality(ctx *reportContext) (bool, OmissionReason) {
	if invoiceCount(ctx.issued)+invoiceCount(ctx.received) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func buildInvoiceFieldQuality(ctx *reportContext) []Block {
	return []Block{
		{Kind: BlockTable, Title: "发票字段逐项核验", Headers: []string{"检测事项", "检测结果", "风险等级"}, Rows: invoiceFieldCheckRows(ctx)},
		{Kind: BlockParagraph, Title: "口径说明", Paragraphs: []string{
			"抬头及税号：比对本企业身份，销项为销售方、进项为购买方；名称差异需核对历史名称及开票时资料，不扩大为整体合规结论。",
			"品名与分类编码：同品名多编码属于待核实线索；未取得适用期分类依据或字段缺失时，不作正确性认定。",
			"税率及征收率：未完整取得交易适用条件和对手方纳税人资格，多税率差异不直接认定为税率错误。",
			"异常主体及凭证：票面异常状态属于待核实线索；未取得对手方开票时点的完整主体状态，不以当前工商状态推断历史开票异常。",
		}},
	}
}

func includeSpecificInvoiceFields(ctx *reportContext) (bool, OmissionReason) {
	if len(specificInvoiceFieldRows(ctx)) == 0 {
		return false, OmittedNotApplicable
	}
	return true, ""
}

func buildSpecificInvoiceFields(ctx *reportContext) []Block {
	rows := specificInvoiceFieldRows(ctx)
	var missing []string
	for _, row := range rows {
		if row[2] != "0" {
			missing = append(missing, row[0]+"的"+row[3])
		}
	}
	if len(missing) > 0 {
		return []Block{{Kind: BlockParagraph, Title: "数据说明", Paragraphs: []string{"本期取得的发票资料未完整包含" + strings.Join(missing, "、") + "，暂无法核验对应业务的必填信息；资料缺失不代表原始发票缺项。"}}}
	}
	for index, row := range rows {
		rows[index] = []string{row[0], row[3], "已取得" + row[1] + "条业务记录的对应字段", "未核验原始票面"}
	}
	return []Block{{Kind: BlockTable, Title: "特定业务发票字段覆盖", Headers: []string{"业务类型", "必填字段", "资料覆盖", "核验范围"}, Rows: rows}}
}

// specificInvoiceFieldRows 合并服务端逐票核验计数，不读取票面字段。
func specificInvoiceFieldRows(ctx *reportContext) [][]string {
	var order []string
	results := map[string]*invoiceSpecificCheck{}
	for _, module := range []string{"issued_invoices", "received_invoices"} {
		for _, chunk := range readInvoiceSummary(ctx.modules[module]).Chunks {
			for _, check := range chunk.SpecificChecks {
				value := results[check.Name]
				if value == nil {
					value = &invoiceSpecificCheck{Name: check.Name}
					results[check.Name] = value
					order = append(order, check.Name)
				}
				value.Count += check.Count
				value.Missing += check.Missing
				for _, label := range check.Labels {
					value.Labels = appendUnique(value.Labels, label)
				}
			}
		}
	}
	var rows [][]string
	for _, name := range order {
		value := results[name]
		if value.Missing > 0 {
			rows = append(rows, []string{name, fmt.Sprint(value.Count), fmt.Sprint(value.Missing), strings.Join(value.Labels, "、")})
		}
	}
	return rows
}

func includePaymentFacts(ctx *reportContext) (bool, OmissionReason) {
	if len(ctx.payments) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func buildMajorTaxTrend(ctx *reportContext) []Block {
	if len(ctx.payments) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前缴款记录未返回可形成主要税种税额趋势的数据。"}}}
	}
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, false)
	values, known := monthlyPaymentByTax(ctx.payments)
	names := majorTaxNames(ctx.payments, 3)
	blocks := make([]Block, 0)
	for _, group := range splitMonthlyPeriodsByNaturalYear(periods, len(periods) > 18) {
		suffix, yearTitle := chartYearSuffix(group)
		title := "主要税费缴纳趋势"
		if yearTitle != "" && len(periods) > 18 {
			title = yearTitle + title
		}
		chart := &Chart{ID: "major-tax-trend" + suffix, Kind: chartKindLine, Title: title, Labels: group, Series: monthlyChartSeries(group, names, values, known), Summary: "展示分析期间各月主要税费的实缴金额变化。"}
		blocks = append(blocks, Block{Kind: BlockChart, Title: chart.Title, Chart: chart, Paragraphs: []string{chart.Summary}})
	}
	return blocks
}

func buildTotalPaymentTrend(ctx *reportContext) []Block {
	if len(ctx.payments) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前缴款记录未返回可形成税费缴纳总额趋势的数据。"}}}
	}
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, false)
	values, known := totalMonthlyPayments(ctx.payments)
	blocks := make([]Block, 0)
	for _, group := range splitMonthlyPeriodsByNaturalYear(periods, len(periods) > 18) {
		suffix, yearTitle := chartYearSuffix(group)
		chartValues, available := make([]float64, len(group)), make([]bool, len(group))
		for index, period := range group {
			chartValues[index], available[index] = values[period], known[period]
		}
		title := "税费缴纳总额趋势"
		if yearTitle != "" && len(periods) > 18 {
			title = yearTitle + title
		}
		chart := &Chart{ID: "total-payment-trend" + suffix, Kind: chartKindLine, Title: title, Labels: group, Series: []Series{{Name: "税费缴纳总额", Values: chartValues, Available: available, Color: chartColorPrimary}}, Summary: "按税款所属期汇总同月全部实缴税费；只有缴款日期的记录按缴款日期归集并在明细表中标记。"}
		blocks = append(blocks, Block{Kind: BlockChart, Title: chart.Title, Chart: chart, Paragraphs: []string{chart.Summary}})
	}
	return append(blocks, Block{Kind: BlockTable, Title: "分月税费缴纳总额", Headers: []string{"月份", "实缴税费总额"}, Rows: formatMonthlyTaxRows(periods, values, known)})
}

func includeVATFacts(ctx *reportContext) (bool, OmissionReason) {
	if len(ctx.vat) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func buildVATBurdenTrend(ctx *reportContext) []Block {
	if len(ctx.vat) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前增值税申报数据未返回可计算税负率的所属期记录。"}}}
	}
	labels := make([]string, len(ctx.vat))
	values := make([]float64, len(ctx.vat))
	available := make([]bool, len(ctx.vat))
	rows := make([][]string, 0, len(ctx.vat))
	for index, fact := range ctx.vat {
		labels[index] = fact.Period
		valid := fact.HasSales && fact.Sales != 0 && fact.HasSupplementTax
		ratio := 0.0
		if valid {
			ratio = fact.SupplementTax / fact.Sales
			values[index], available[index] = ratio*concentrationPercentScale, true
		}
		rows = append(rows, []string{fact.Period, optionalMoney(fact.SupplementTax, fact.HasSupplementTax), optionalMoney(fact.Sales, fact.HasSales), optionalPercent(ratio, valid)})
	}
	chart := &Chart{ID: "vat-burden-trend", Kind: chartKindLine, Title: "增值税税负率趋势", Labels: labels, Unit: "%", Series: []Series{{Name: "增值税税负率（%）", Values: values, Available: available, Color: chartColorPrimary}}, Summary: "税负率按本期应补退增值税除以当期申报销售额计算；分母为0或缺失时显示空缺。"}
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart, Paragraphs: []string{chart.Summary}}, {Kind: BlockTable, Title: "增值税税负率", Headers: []string{"所属期", "应纳增值税", "申报销售额", "税负率"}, Rows: rows}}
}

func includeFinancialVATComparison(ctx *reportContext) (bool, OmissionReason) {
	if len(ctx.financial) == 0 || len(ctx.vat) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func buildFinancialVATComparison(ctx *reportContext) []Block {
	if len(ctx.financial) == 0 || len(ctx.vat) == 0 {
		return []Block{{Kind: BlockTable, Title: "财务报表收入与增值税销售额", Headers: []string{"所属期", "税务报送财务报表营业收入", "增值税申报销售额", "差异", "关注"}}, {Kind: BlockParagraph, Paragraphs: []string{"当前财务报表收入或增值税申报销售额数据不足，无法形成同口径对比。"}}}
	}
	rows := make([][]string, 0, len(ctx.financial))
	annualRows := make([][]string, 0)
	for _, financial := range ctx.financial {
		revenue, revenueOK, declared, declaredOK := financialVATPeriodValues(ctx, financial)
		valid := revenueOK && declaredOK
		difference := 0.0
		if valid {
			difference = revenue - declared
		}
		row := []string{financial.Period, optionalMoney(revenue, revenueOK), optionalMoney(declared, declaredOK), optionalMoney(difference, valid), "—"}
		if financialVATAnnual(financial) {
			annualRows = append(annualRows, row)
		} else {
			rows = append(rows, row)
		}
	}
	var blocks []Block
	if len(rows) > 0 {
		blocks = append(blocks, Block{Kind: BlockTable, Title: "财务报表收入与增值税销售额（当期）", Headers: []string{"所属期（单月/单季）", "税务报送财务报表营业收入", "增值税申报销售额", "差异", "关注"}, Rows: rows})
	}
	if len(annualRows) > 0 {
		blocks = append(blocks, Block{Kind: BlockTable, Title: "年度财务报表收入与增值税销售额", Headers: []string{"年度", "税务报送财务报表营业收入", "增值税申报销售额", "差异", "关注"}, Rows: annualRows})
	}
	return blocks
}

func buildVATSpecialItems(ctx *reportContext) []Block {
	return []Block{{Kind: BlockTable, Title: "增值税特殊销售项目", Headers: []string{"项目", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "金额", "分析"}, Rows: vatSpecialSummaryRows(ctx)}}
}

func buildVATRatesAndReliefs(ctx *reportContext) []Block {
	return []Block{{Kind: BlockTable, Title: "免税、简易计税及差额征税适用情况", Headers: []string{"项目", "申报情况", "结论"}, Rows: vatApplicabilityRows(ctx)}}
}

func includeCITFinancialComparison(ctx *reportContext) (bool, OmissionReason) {
	if (len(ctx.cit) == 0 && len(annualCITMainFields(ctx.taxFields, []string{"营业收入", "利润总额"})) == 0) || len(ctx.financial) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func buildQuarterlyCITRevenueComparison(ctx *reportContext) []Block {
	quarterIncome := quarterlyCITMetricForContext(ctx, func(fact citFact) (float64, bool) {
		return fact.CumulativeIncome, fact.HasIncome
	})
	if len(quarterIncome) == 0 || len(ctx.financial) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"季度", "财务报表营业收入", "企税申报营业收入", "差异额", "风险"}}}
	}
	rows := make([][]string, 0, len(quarterIncome))
	for _, item := range quarterIncome {
		financialRevenue, financialOK := financialValueForQuarter(ctx.financial, item.period, func(fact financialFact) (float64, bool) { return fact.Revenue, fact.HasRevenue })
		valid := item.known && financialOK
		rows = append(rows, []string{item.period, optionalMoney(financialRevenue, financialOK), optionalMoney(item.value, item.known), optionalMoney(financialRevenue-item.value, valid), "—"})
	}
	return []Block{{Kind: BlockTable, Title: "企税申报收入与财务报表收入", Headers: []string{"季度", "财务报表营业收入", "企税申报营业收入", "差异额", "风险"}, Rows: rows}}
}

func buildFinancialCostPurchaseComparison(ctx *reportContext) []Block {
	var rows [][]string
	for _, quarter := range financialQuarters(ctx.financial) {
		cost, costOK := financialValueForQuarter(ctx.financial, quarter, financialCostWithExpenses)
		purchase, purchaseOK := invoiceAmountForQuarter(ctx.received, quarter)
		rows = append(rows, []string{quarter, optionalMoney(cost, costOK), optionalMoney(purchase, purchaseOK), "—", "工资申报收入待补充，暂不计算扣除工资后的差异"})
	}
	return []Block{{Kind: BlockTable, Title: "财务成本费用与进项发票", Headers: financialCostComparisonHeaders, Rows: rows}}
}

var quarterlyCITProfitHeaders = []string{"季度", "财报单季利润", "企税单季利润", "实际利润单季变动", "财报−企税利润", "实际变动−企税利润"}

const quarterlyCITProfitBasis = "企税单季利润及实际利润单季变动，分别由申报累计利润总额及累计实际利润额减同年相邻上季累计得到，首季直接取累计；实际利润单季变动不代表本季申报的应纳税所得额。"

func buildQuarterlyCITProfitComparison(ctx *reportContext) []Block {
	actual := quarterlyCITMetricForContext(ctx, func(f citFact) (float64, bool) { return f.CumulativeActualProfit, f.HasActualProfit })
	declared := quarterlyCITMetricForContext(ctx, func(f citFact) (float64, bool) { return f.CumulativeProfit, f.HasProfit })
	rows := [][]string{}
	for _, item := range actual {
		financial, financialOK := financialValueForQuarter(ctx.financial, item.period, func(f financialFact) (float64, bool) { return f.Profit, f.HasProfit })
		profit, profitOK := 0.0, false
		for _, candidate := range declared {
			if candidate.period == item.period {
				profit, profitOK = candidate.value, candidate.known
				break
			}
		}
		rows = append(rows, []string{item.period, optionalMoney(financial, financialOK), optionalMoney(profit, profitOK), optionalMoney(item.value, item.known), optionalMoney(financial-profit, financialOK && profitOK), optionalMoney(item.value-profit, item.known && profitOK)})
	}
	return []Block{{Kind: BlockTable, Title: "财报与企税利润单季比较", Headers: quarterlyCITProfitHeaders, Rows: rows}, {Kind: BlockParagraph, Title: "口径说明", Paragraphs: []string{quarterlyCITProfitBasis}}}
}

func buildCITResearchExpense(ctx *reportContext) []Block {
	return buildCITResearchAmounts(ctx)
}

func buildCITResearchAmounts(ctx *reportContext) []Block {
	if rows := citResearchRows(ctx); len(rows) > 0 {
		return []Block{{Kind: BlockTable, Title: "研发加计扣除申报数据", Headers: []string{"季度/年度", "研发加计扣除额", "占营业收入比例", "趋势"}, Rows: rows}}
	}
	return []Block{{Kind: BlockTable, Headers: []string{"季度/年度", "研发加计扣除额", "占营业收入比例", "趋势"}}}
}

func buildCITPreferences(ctx *reportContext) []Block {
	if rows := mainCITPreferenceRows(ctx); len(rows) > 0 {
		return []Block{{Kind: BlockTable, Title: "企业所得税优惠申报", Headers: []string{"优惠项目", "申报情况", "关注"}, Rows: rows}}
	}
	rows := textMetricRows(ctx.modules["cit_annual_returns"].Raw, []textMetricDefinition{{"优惠项目", []string{"preferenceName", "taxPreferenceName", "优惠项目"}}, {"优惠金额", []string{"preferenceAmount", "taxPreferenceAmount", "优惠金额"}}})
	if len(rows) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"优惠项目", "申报情况", "关注"}, Rows: [][]string{{"企业所得税优惠", "当前年度申报数据未返回可展示项目", "数据不足"}}}}
	}
	contractRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		contractRows = append(contractRows, []string{row[1], row[0] + "申报" + row[2], "—"})
	}
	return []Block{{Kind: BlockTable, Title: "企业所得税优惠申报", Headers: []string{"优惠项目", "申报情况", "关注"}, Rows: contractRows}}
}

func buildCITDeductionEvidence(ctx *reportContext) []Block {
	return []Block{{Kind: BlockTable, Headers: []string{"事项", "发票/申报线索", "关注"}, Rows: deductionEvidenceRows(ctx)}}
}

func buildSurtaxPayments(ctx *reportContext) []Block {
	return declarationLineBlocks(ctx, "附加税费", func(name string) bool {
		return containsAny(name, "城市维护建设税", "教育费附加", "地方教育附加")
	})
}

func buildUnionFundPayments(ctx *reportContext) []Block {
	return unionDeliveryBlocks(ctx)
}

func includeOtherTaxPayments(ctx *reportContext) (bool, OmissionReason) {
	for _, fact := range ctx.payments {
		if isOtherTaxName(fact.TaxName) {
			return true, ""
		}
	}
	return false, OmittedNotApplicable
}

func buildOtherTaxPayments(ctx *reportContext) []Block {
	rows := aggregatePaymentsByTaxName(ctx.payments, isOtherTaxName)
	contractRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		contractRows = append(contractRows, []string{row[0], analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "实缴" + row[1], "适用"})
	}
	for _, fact := range ctx.declarations {
		if !isOtherTaxName(fact.TaxName) {
			continue
		}
		text := fact.Period.Display + "已申报；申报金额" + optionalMoney(fact.Declared, fact.HasDeclared)
		found := false
		for index := range contractRows {
			if contractRows[index][0] == fact.TaxName {
				contractRows[index][1] += "；" + text
				found = true
				break
			}
		}
		if !found {
			contractRows = append(contractRows, []string{fact.TaxName, text, "依据实际申报纳入"})
		}
	}
	for _, recognition := range taxRecognitions(ctx) {
		if !isOtherTaxName(recognition.TaxName) {
			continue
		}
		found := false
		for index := range contractRows {
			if contractRows[index][0] == recognition.TaxName {
				found = true
				break
			}
		}
		if !found {
			contractRows = append(contractRows, []string{recognition.TaxName, "税费认定品目：" + recognition.Item + "；认定周期：" + recognition.Cycle, "有税费种认定；缺少本期申报记录不能直接认定漏报"})
		}
	}
	for _, line := range ctx.taxLines {
		if !isOtherTaxName(line.TaxName) {
			continue
		}
		contractRows = append(contractRows, []string{line.TaxName + " · " + line.Item, line.Period.Display + "；计税依据" + line.value("计税依据") + "；应补退" + line.value("应补（退）税额", "本期应补（退）税（费）额"), "按已取得明细原值展示"})
	}
	if len(contractRows) == 0 {
		contractRows = append(contractRows, []string{"其他税种", "当前报告未取得可展示记录", "尚无足够数据判断适用情况"})
	}
	return []Block{{Kind: BlockTable, Headers: []string{"税种", "企业适用情况", "结论"}, Rows: contractRows}}
}

// aggregatePaymentsByTaxName collapses repeated payment records into the
// tax-name rows required by the Word tables. Parameters are normalized payment
// facts and a tax-name selector. Return value is sorted and contains name plus
// analysis-period total, so repeated payment batches do not create duplicate
// visible rows.
func aggregatePaymentsByTaxName(facts []paymentFact, match func(string) bool) [][]string {
	totals := map[string]float64{}
	for _, fact := range facts {
		name := textOr(strings.TrimSpace(fact.TaxName), "其他税种")
		if !match(name) {
			continue
		}
		totals[name] += fact.Amount
	}
	names := sortedMapKeys(totals)
	rows := make([][]string, 0, len(names))
	for _, name := range names {
		rows = append(rows, []string{name, moneyText(totals[name])})
	}
	return rows
}

func includeBalanceMetrics(ctx *reportContext) (bool, OmissionReason) {
	for _, fact := range ctx.financial {
		if fact.HasAssets || fact.HasLiabilities {
			return true, ""
		}
	}
	return false, OmittedDataInsufficient
}

// previousFinancialPeriodKey identifies the adjacent complete period of the same cycle.
// Annual statements are compared only with the preceding year, never with Q4.
func previousFinancialPeriodKey(period string) string {
	from, to := periodBounds(period)
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return ""
	}
	months := 0
	switch filingCycle(from, to, "", "") {
	case taxCycleMonthly:
		months = 1
	case taxCycleQuarterly:
		months = monthsPerQuarter
	case taxCycleAnnual:
		months = 12
	}
	if months == 0 || start.AddDate(0, months, -1).Format("2006-01-02") != to {
		return ""
	}
	return start.AddDate(0, -months, 0).Format("2006-01-02") + "|" + start.AddDate(0, 0, -1).Format("2006-01-02")
}

func buildDebtAssetRatio(ctx *reportContext) []Block {
	rows := [][]string{}
	known := map[string]float64{}
	for _, fact := range ctx.financial {
		if fact.HasAssets && fact.HasLiabilities && fact.Assets != 0 {
			from, to := periodBounds(fact.Period)
			known[from+"|"+to] = fact.Liabilities / fact.Assets
		}
	}
	for _, fact := range ctx.financial {
		if !fact.HasAssets && !fact.HasLiabilities {
			continue
		}
		valid := fact.HasAssets && fact.HasLiabilities && fact.Assets != 0
		ratio := 0.0
		if valid {
			ratio = fact.Liabilities / fact.Assets
		}
		change := "—"
		if previous, ok := known[previousFinancialPeriodKey(fact.Period)]; valid && ok {
			change = fmt.Sprintf("%+.2f个百分点", (ratio-previous)*100)
		}
		rows = append(rows, []string{fact.Period, optionalPercent(ratio, valid), change, "—"})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"期间", "资产负债率", "变化", "风险"}}}
	}
	return []Block{{Kind: BlockTable, Title: "资产负债率", Headers: []string{"期间", "资产负债率", "变化", "风险"}, Rows: rows}}
}

func buildWorkingCapitalMetrics(ctx *reportContext) []Block {
	rows := make([][]string, 0, len(ctx.financial))
	for _, fact := range ctx.financial {
		if !fact.HasAccountsReceivable && !fact.HasInventory && !fact.HasContractLiabilities {
			continue
		}
		rows = append(rows, []string{
			fact.Period,
			optionalMoney(fact.AccountsReceivable, fact.HasAccountsReceivable),
			optionalMoney(fact.Inventory, fact.HasInventory),
			optionalMoney(fact.ContractLiabilities, fact.HasContractLiabilities),
			"—",
		})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"期间", "应收账款", "存货", "合同负债", "关注"}}}
	}
	return []Block{{Kind: BlockTable, Title: "应收账款、存货及合同负债", Headers: []string{"期间", "应收账款", "存货", "合同负债", "关注"}, Rows: rows}}
}

func includeWorkingCapitalMetrics(ctx *reportContext) (bool, OmissionReason) {
	for _, fact := range ctx.financial {
		if fact.HasAccountsReceivable || fact.HasInventory || fact.HasContractLiabilities {
			return true, ""
		}
	}
	return false, OmittedDataInsufficient
}

func buildFinancialSummary(ctx *reportContext) []Block {
	return financialSummaryBlocks(ctx)
}

func buildIndustrySummary(ctx *reportContext) []Block {
	if len(ctx.categories) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前销项发票品类不足以形成主要业务综合分析。"}}}
	}
	// Facts are supplied by analysisIndustrySummary; the Agent writes the final
	// synthesis. Do not prepend a second script-authored reading of the table.
	return nil
}

type categoryAmount struct {
	name   string
	amount float64
}

func aggregateInvoiceCategories(facts []invoiceGroup) []categoryAmount {
	byName := map[string]float64{}
	for _, allocation := range invoiceCategoryAllocations(facts) {
		byName[allocation.Name] += allocation.Amount
	}
	result := make([]categoryAmount, 0, len(byName))
	for name, amount := range byName {
		result = append(result, categoryAmount{name: name, amount: amount})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].amount > result[j].amount })
	return result
}

func businessTextOverlap(category, scope string) bool {
	category = strings.TrimSpace(category)
	if len([]rune(category)) < 2 {
		return false
	}
	if strings.Contains(scope, category) {
		return true
	}
	for _, token := range strings.FieldsFunc(category, func(r rune) bool { return r == '、' || r == '/' || r == '-' || r == ' ' }) {
		if len([]rune(token)) >= 2 && strings.Contains(scope, token) {
			return true
		}
	}
	return false
}

func counterpartyAmounts(facts []invoiceGroup) []categoryAmount {
	items := invoiceCounterpartyItems(facts)
	result := make([]categoryAmount, 0, len(items))
	for _, item := range items {
		result = append(result, categoryAmount{name: item.Name, amount: item.Amount})
	}
	return result
}

func vatSalesForPeriod(facts []vatFact, period string) (float64, bool) {
	for _, fact := range facts {
		if fact.Period == period {
			return fact.Sales, fact.HasSales
		}
	}
	quarter := normalizeQuarter(period)
	quarterStart, _ := quarterPeriodBounds(period)
	isQuarter := quarterStart != ""
	expected := 3
	if strings.Contains(period, "年度") {
		expected = 12
	}
	byMonth := map[string]float64{}
	for _, fact := range facts {
		if !fact.HasSales || len(fact.Period) != 7 {
			continue
		}
		if isQuarter && normalizeQuarter(fact.Period) == quarter || expected == 12 && strings.HasPrefix(fact.Period, period[:4]) {
			byMonth[fact.Period] = fact.Sales
		}
	}
	if len(byMonth) != expected {
		return 0, false
	}
	total := 0.0
	for _, value := range byMonth {
		total += value
	}
	return total, true
}

type metricDefinition struct {
	name    string
	aliases []string
	money   bool
}

type textMetricDefinition struct {
	name    string
	aliases []string
}

type periodValue struct {
	period string
	value  float64
	known  bool
}

func quarterlyCITIncome(facts []citFact) []periodValue {
	return quarterlyCITMetric(facts, func(fact citFact) (float64, bool) {
		return fact.CumulativeIncome, fact.HasIncome
	})
}

// quarterlyCITMetric converts one cumulative enterprise-income-tax main-form
// series into single-quarter values. Parameters are ordered facts and a metric
// selector. Return value keeps a quarter unavailable when the preceding
// cumulative quarter needed for subtraction is absent.
func quarterlyCITMetric(facts []citFact, selector func(citFact) (float64, bool)) []periodValue {
	result := make([]periodValue, 0, len(facts))
	previousValue := map[string]float64{}
	previousQuarter := map[string]int{}
	for _, fact := range facts {
		year := ""
		if len(fact.Period) >= 4 {
			year = fact.Period[:4]
		}
		quarter := quarterNumber(fact.Period)
		item := periodValue{period: fact.Period}
		value, known := selector(fact)
		if known {
			if quarter == 1 {
				item.value, item.known = value, true
			} else if previousQuarter[year] == quarter-1 {
				item.value, item.known = value-previousValue[year], true
			}
			previousValue[year], previousQuarter[year] = value, quarter
		}
		result = append(result, item)
	}
	return result
}

func financialValueForQuarter(facts []financialFact, quarter string, selector func(financialFact) (float64, bool)) (float64, bool) {
	for _, fact := range facts {
		if quarterStart, _ := quarterPeriodBounds(fact.Period); quarterStart != "" && normalizeQuarter(fact.Period) == quarter {
			return selector(fact)
		}
	}
	total, count := 0.0, 0
	for _, fact := range facts {
		if len(fact.Period) != 7 || normalizeQuarter(fact.Period) != quarter {
			continue
		}
		value, ok := selector(fact)
		if ok {
			total += value
			count++
		}
	}
	return total, count == 3
}

func financialQuarters(facts []financialFact) []string {
	set := map[string]bool{}
	for _, fact := range facts {
		if strings.Contains(fact.Period, "年度") {
			continue
		}
		if quarter := normalizeQuarter(fact.Period); quarter != "" {
			set[quarter] = true
		}
	}
	return sortedMapKeys(set)
}

func invoiceAmountForQuarter(facts []invoiceGroup, quarter string) (float64, bool) {
	total, known := 0.0, false
	for _, fact := range facts {
		if normalizeQuarter(fact.Month) == quarter && fact.HasAmount {
			total += fact.Amount
			known = true
		}
	}
	return total, known
}

func paymentCategoryBlocks(facts []paymentFact, title string, match func(string) bool) []Block {
	rows := [][]string{}
	for _, fact := range facts {
		if match(fact.TaxName) {
			rows = append(rows, []string{fact.Period, fact.TaxName, moneyText(fact.Amount)})
		}
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前报告未取得可展示的" + title + "缴款记录。"}}}
	}
	return []Block{{Kind: BlockTable, Title: title, Headers: []string{"所属期", "项目", "实缴金额"}, Rows: rows}}
}

func isOtherTaxName(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	return !containsAny(name, "增值税", "企业所得税", "印花税", "房产税", "城镇土地使用税", "城市维护建设税", "教育费附加", "地方教育附加", "工会经费", "工会筹备金")
}

func priorityFindings(findings []Finding) []Finding {
	sorted := impactSortedFindings(findings)
	selected := make([]Finding, 0)
	for _, finding := range sorted {
		if finding.Level == RiskHigh {
			selected = append(selected, finding)
		}
	}
	if len(selected) == 0 {
		for _, finding := range sorted {
			if finding.Level == RiskMedium {
				selected = append(selected, finding)
				break
			}
		}
	}
	return selected
}

func impactSortedFindings(findings []Finding) []Finding {
	result := append([]Finding(nil), findings...)
	priority := map[RiskLevel]int{RiskHigh: 0, RiskMedium: 1, RiskLow: 2}
	sort.SliceStable(result, func(i, j int) bool {
		if priority[result[i].Level] != priority[result[j].Level] {
			return priority[result[i].Level] < priority[result[j].Level]
		}
		if result[i].Amount != result[j].Amount {
			return result[i].Amount > result[j].Amount
		}
		if len(result[i].Periods) != len(result[j].Periods) {
			return len(result[i].Periods) > len(result[j].Periods)
		}
		if confidencePriority(result[i].Confidence) != confidencePriority(result[j].Confidence) {
			return confidencePriority(result[i].Confidence) < confidencePriority(result[j].Confidence)
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func remediationDeadline(level RiskLevel) string {
	switch level {
	case RiskHigh:
		return "优先安排"
	case RiskMedium:
		return "本期跟进"
	default:
		return "持续优化"
	}
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

// relatedScopeClause displays relevant original clauses, while analysis inputs
// retain the complete scope. No semantic match is asserted by this excerpt.
func relatedScopeClause(category, scope string) string {
	if scope == "" {
		return "未取得"
	}
	var matched []string
	for _, clause := range strings.FieldsFunc(scope, func(c rune) bool { return c == '；' || c == ';' || c == '。' }) {
		if businessTextOverlap(category, clause) {
			matched = appendUnique(matched, strings.TrimSpace(clause))
		}
	}
	if len(matched) == 0 {
		return "完整经营范围见2.1；本项需结合实际业务核对"
	}
	return strings.Join(matched, "；")
}
