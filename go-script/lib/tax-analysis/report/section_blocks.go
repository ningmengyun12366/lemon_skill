package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func buildOverallEvaluation(ctx *reportContext) []Block {
	counts := riskCounts(ctx.findings)
	assessment := assessOverallRisk(ctx.findings, ctx.evaluations)
	overall := assessment.Level
	blocks := make([]Block, 0, 3)
	if overall != overallCoverageLimited {
		blocks = append(blocks, Block{Kind: BlockRiskBand, Title: overall, Paragraphs: []string{overall}})
	}
	blocks = append(blocks,
		Block{
			Kind: BlockRiskCard, Title: "综合风险评价",
			Paragraphs: []string{overall, assessment.summary()},
			Headers:    []string{string(RiskHigh), string(RiskMedium), string(RiskLow)},
			Rows:       [][]string{{fmt.Sprintf("%d项", counts[RiskHigh]), fmt.Sprintf("%d项", counts[RiskMedium]), fmt.Sprintf("%d项", counts[RiskLow])}},
		},
		Block{Kind: BlockTable, TableVariant: TableVariantRiskSummary, Title: "风险等级汇总", Headers: []string{"风险等级", "数量", "主要领域"}, Rows: [][]string{
			{string(RiskHigh), fmt.Sprintf("%d项", counts[RiskHigh]), findingDomains(ctx.findings, RiskHigh)},
			{string(RiskMedium), fmt.Sprintf("%d项", counts[RiskMedium]), findingDomains(ctx.findings, RiskMedium)},
			{string(RiskLow), fmt.Sprintf("%d项", counts[RiskLow]), findingDomains(ctx.findings, RiskLow)},
		}},
	)
	return blocks
}

// buildRiskBand creates the solid Figma warning strip for the highest
// confirmed level. Parameter findings contains deterministic rule results. The
// returned block is absent when no risk has been confirmed.
func buildRiskBand(findings []Finding) (Block, bool) {
	items := sortedFindings(findings)
	if len(items) == 0 {
		return Block{}, false
	}
	level := items[0].Level
	return Block{Kind: BlockRiskBand, Title: string(level), Paragraphs: []string{string(level)}}, true
}

// findingDomains lists the de-duplicated trusted finding titles for one level.
// Parameters findings and level are deterministic rule outputs. The returned
// text is suitable for the Figma risk-summary table and contains no new facts.
func findingDomains(findings []Finding, level RiskLevel) string {
	titles := make([]string, 0)
	for _, finding := range findings {
		if finding.Level == level {
			titles = append(titles, finding.Title)
		}
	}
	values := uniqueSorted(titles)
	if len(values) == 0 {
		return "—"
	}
	return strings.Join(values, "、")
}

// clearEvaluationDomains summarizes only clear rules into stable business
// areas. Parameter evaluations is the completed rule set. The return value is
// a concise, deterministic label and never treats missing data as normal.
func clearEvaluationDomains(evaluations []RuleEvaluation) string {
	names := map[string]string{
		"INVOICE-OUTPUT-001": "销项虚开组合线索", "INVOICE-INPUT-001": "进项虚假凭证组合线索",
		"TAX-FILING-001": "申报异常记录", "TAX-PAYMENT-001": "滞纳金记录", "TAX-ARREARS-001": "欠税记录", "TAX-CREDIT-001": "当前纳税信用等级",
	}
	clear, blocked := map[string]bool{}, map[string]bool{}
	for _, evaluation := range evaluations {
		if evaluation.Outcome == OutcomeClear {
			clear[evaluation.RuleID] = true
		} else {
			blocked[evaluation.RuleID] = true
		}
	}
	result := []string{}
	for ruleID := range clear {
		if !blocked[ruleID] && names[ruleID] != "" {
			result = append(result, names[ruleID])
		}
	}
	return textOr(strings.Join(uniqueSorted(result), "、"), "—")
}

func buildRiskDistribution(ctx *reportContext) []Block {
	assessment := assessOverallRisk(ctx.findings, ctx.evaluations)
	counts := map[RiskLevel]int{RiskHigh: assessment.High, RiskMedium: assessment.Medium, RiskLow: assessment.Low}
	clear := assessment.Clear
	chart := &Chart{
		ID: "risk-distribution", Kind: chartKindDonut, Title: "风险等级分布",
		Labels:      []string{string(RiskHigh), string(RiskMedium), string(RiskLow), "未见明显异常"},
		Series:      []Series{{Name: "已完成检测数量", Values: []float64{float64(counts[RiskHigh]), float64(counts[RiskMedium]), float64(counts[RiskLow]), float64(clear)}, Colors: []string{chartColorDonutHighRisk, chartColorDonutMediumRisk, chartColorDonutLowRisk, chartColorPositive}}},
		CenterLabel: riskTotalCenterLabel, Unit: "项",
		Summary: "占比以已完成检测为分母；数据不足和不适用不计入。",
	}
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}}
}

// completedClearRuleCount excludes incomplete outcomes and counts each check once.
func completedClearRuleCount(evaluations []RuleEvaluation) int {
	clear, excluded := map[string]bool{}, map[string]bool{}
	for _, evaluation := range evaluations {
		if evaluation.Outcome == OutcomeClear {
			clear[evaluation.RuleID] = true
		} else {
			excluded[evaluation.RuleID] = true
		}
	}
	count := 0
	for rule := range clear {
		if !excluded[rule] {
			count++
		}
	}
	return count
}

func buildModuleRiskDistribution(ctx *reportContext) []Block {
	chart := findingsByModuleChart(ctx.findings)
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}}
}

func buildRiskTables(ctx *reportContext) []Block {
	all := sortedFindings(ctx.findings)
	top := append([]Finding(nil), all...)
	sort.SliceStable(top, func(i, j int) bool {
		priority := map[RiskLevel]int{RiskHigh: 0, RiskMedium: 1, RiskLow: 2}
		if priority[top[i].Level] != priority[top[j].Level] {
			return priority[top[i].Level] < priority[top[j].Level]
		}
		if top[i].Amount != top[j].Amount {
			return top[i].Amount > top[j].Amount
		}
		if len(top[i].Periods) != len(top[j].Periods) {
			return len(top[i].Periods) > len(top[j].Periods)
		}
		if top[i].Confidence != top[j].Confidence {
			return confidencePriority(top[i].Confidence) < confidencePriority(top[j].Confidence)
		}
		return top[i].ID < top[j].ID
	})
	if len(top) > 10 {
		top = top[:10]
	}
	totalRows := make([][]string, 0, len(all))
	for index, finding := range all {
		row := findingRow(finding, index+1)
		totalRows = append(totalRows, []string{row[1], row[2], row[0], row[3], row[4]})
	}
	detailRows := make([][]string, 0, len(top))
	for _, finding := range top {
		detailRows = append(detailRows, []string{finding.Title, string(finding.Level), findingOverviewEvidence(finding), textOr(strings.TrimSpace(finding.Recommendation), "—")})
	}
	blocks := make([]Block, 0, 3)
	if band, ok := buildRiskBand(all); ok {
		blocks = append(blocks, band)
	}
	blocks = append(blocks,
		Block{Kind: BlockTable, TableVariant: TableVariantKeyRisk, Title: "风险事项总表", Headers: []string{"序号", "风险事项", "等级", "异常期间", "异常金额或情况"}, Rows: totalRows},
		Block{Kind: BlockTable, TableVariant: TableVariantKeyRisk, Title: "重点风险明细（前10项）", Headers: []string{"风险事项", "等级", "判断依据", "建议方向"}, Rows: detailRows},
	)
	return blocks
}

func confidencePriority(value string) int {
	switch strings.TrimSpace(value) {
	case "高":
		return 0
	case "中":
		return 1
	case "低":
		return 2
	default:
		return 3
	}
}

// findingOverviewEvidence keeps monthly detail in its source section and the
// full fact file. The overview reports period span, count and total once.
func findingOverviewEvidence(f Finding) string {
	if len(f.Periods) <= 2 {
		if f.VerificationRequired {
			return joinEvidenceFragments(f.Evidence, "需核实")
		}
		return textOr(f.Evidence, "—")
	}
	amount := ""
	if f.Amount != 0 {
		amount = "，涉及金额合计" + moneyText(f.Amount) + "元"
	}
	return fmt.Sprintf("%s，共%d个期间%s；%s", findingPeriodSpan(f), len(uniqueSorted(f.Periods)), amount, map[bool]string{true: "具体原因及处理结果待核实", false: "已取得相应记录，详见对应章节"}[f.VerificationRequired])
}
func findingPeriodSpan(f Finding) string {
	var starts, ends []string
	for _, p := range f.Periods {
		a, b := periodBounds(p)
		if a != "" && b != "" {
			starts = append(starts, a)
			ends = append(ends, b)
		}
	}
	if len(starts) == 0 {
		return f.Period
	}
	sort.Strings(starts)
	sort.Strings(ends)
	return starts[0] + "至" + ends[len(ends)-1]
}
func findingRow(finding Finding, sequence int) []string {
	return []string{compactRiskLevel(finding.Level), fmt.Sprint(sequence), finding.Title, findingPeriodSpan(finding), findingOverviewEvidence(finding)}
}

func buildRiskTrend(ctx *reportContext) []Block {
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, true)
	headers := append([]string{"风险事项"}, periods...)
	headers = append(headers, "趋势")
	rows := make([][]string, 0, len(ctx.findings))
	for _, finding := range ctx.findings {
		active := quarterlyRiskOccurrences(finding, ctx.task.StartTime, ctx.task.EndTime)
		row := []string{finding.Title}
		for _, period := range periods {
			if occurrence, ok := active[period]; ok {
				level := finding.Level
				if occurrence.Level != "" {
					level = occurrence.Level
				}
				value := fmt.Sprintf("%s\n%d次记录", level, occurrence.Count)
				if occurrence.HasAmount {
					value += "\n" + moneyText(occurrence.Amount) + "元"
				}
				row = append(row, value)
			} else {
				row = append(row, "—")
			}
		}
		trend := "单期"
		if len(active) > 1 {
			trend = "多季度有记录"
		} else if len(active) == 0 {
			trend = "年度或历史所属期，无法分配至本期季度"
		}
		row = append(row, trend)
		rows = append(rows, row)
	}
	return []Block{{Kind: BlockTable, Title: "风险变化趋势", Headers: headers, Rows: rows}}
}

// findingPointMonth returns a month only for a risk that belongs to one source
// period. Multi-month and explicitly ranged findings are kept out of the trend
// and displayed in the cross-period table.
func findingPointMonth(finding Finding) (string, bool) {
	months := make(map[string]bool)
	for _, period := range finding.Periods {
		if month := extractMonth(period); month != "" {
			months[month] = true
		}
	}
	if len(months) != 1 || strings.ContainsAny(finding.Period, "—~至") {
		return "", false
	}
	for month := range months {
		return month, true
	}
	return "", false
}

func completeTrendPeriods(start, end string, quarterly bool) []string {
	startDate, startErr := time.Parse("2006-01-02", start)
	endDate, endErr := time.Parse("2006-01-02", end)
	if startErr != nil || endErr != nil || endDate.Before(startDate) {
		return nil
	}
	cursor := time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	endMonth := time.Date(endDate.Year(), endDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	periods := make([]string, 0)
	for !cursor.After(endMonth) {
		key := cursor.Format("2006-01")
		if quarterly {
			key = quarterlyPeriodLabel(key, start, end)
		}
		if len(periods) == 0 || periods[len(periods)-1] != key {
			periods = append(periods, key)
		}
		cursor = cursor.AddDate(0, 1, 0)
	}
	return periods
}

// quarterlyPeriodLabel 将季度首尾裁剪到用户实际选择月份，避免把不完整季度标成完整Q。
//
// 参数 month 为 YYYY-MM 月份，start/end 为用户选择日期；返回完整季度标签或实际月份范围。
func quarterlyPeriodLabel(month, start, end string) string {
	monthDate, monthErr := time.Parse("2006-01", month)
	startDate, startErr := time.Parse("2006-01-02", start)
	endDate, endErr := time.Parse("2006-01-02", end)
	if monthErr != nil || startErr != nil || endErr != nil {
		return normalizeQuarter(month)
	}
	quarterStartMonth := time.Month((int(monthDate.Month())-1)/3*3 + 1)
	quarterStart := time.Date(monthDate.Year(), quarterStartMonth, 1, 0, 0, 0, 0, time.UTC)
	quarterEnd := quarterStart.AddDate(0, 2, 0)
	analysisStart := time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	analysisEnd := time.Date(endDate.Year(), endDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	clippedStart, clippedEnd := quarterStart, quarterEnd
	if analysisStart.After(clippedStart) {
		clippedStart = analysisStart
	}
	if analysisEnd.Before(clippedEnd) {
		clippedEnd = analysisEnd
	}
	if clippedStart.Equal(quarterStart) && clippedEnd.Equal(quarterEnd) {
		return normalizeQuarter(month)
	}
	if clippedStart.Equal(clippedEnd) {
		return fmt.Sprintf("%d年%d月", clippedStart.Year(), clippedStart.Month())
	}
	return fmt.Sprintf("%d年%d-%d月", clippedStart.Year(), clippedStart.Month(), clippedEnd.Month())
}

func buildCreditTrend(ctx *reportContext) []Block {
	labels, values := make([]string, len(ctx.credits)), make([]float64, len(ctx.credits))
	for index, credit := range ctx.credits {
		labels[index], values[index] = credit.Year, creditScore(credit.Grade)
	}
	chart := &Chart{ID: "credit-trend", Kind: chartKindLine, Title: "最近三个评价年度纳税信用等级", Labels: labels, Series: []Series{{Name: "信用等级序位", Values: values, Color: chartColorPrimary}}, Summary: "仅在取得至少两个不同评价年度时生成，最多展示最近三个年度。"}
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}}
}

func buildEnterpriseProfile(ctx *reportContext) []Block {
	blocks := buildQixinEnterpriseProfile(ctx)
	grade := strings.TrimSpace(firstText(ctx.modules["tax_credit"].Raw, "taxpayerCreditRating", "creditRating", "nsxydj", "grade"))
	if grade != "" {
		if len(blocks) == 0 || blocks[0].Kind != BlockTable {
			blocks = []Block{{Kind: BlockTable, Headers: []string{"项目", "企业信息"}}}
		}
		blocks[0].Rows = append(blocks[0].Rows, []string{"当前纳税信用等级", grade})
	}
	if len(blocks) == 0 {
		blocks = []Block{{Kind: BlockTable, Headers: []string{"项目", "企业信息"}, Rows: [][]string{{"企业名称", textOr(companyName(ctx.modules["taxpayer_profile"].Raw), ctx.task.USCC)}}}}
	}
	return blocks
}

func creditScore(grade string) float64 {
	switch strings.ToUpper(strings.TrimSpace(grade)) {
	case "A", "A级":
		return 5
	case "B", "B级", "M", "M级":
		return 4
	case "C", "C级":
		return 2
	case "D", "D级":
		return 1
	default:
		return 3
	}
}

func buildInvoiceOverall(ctx *reportContext) []Block {
	issuedAmount, receivedAmount, issuedTax, receivedTax := 0.0, 0.0, 0.0, 0.0
	issuedAmountKnown, receivedAmountKnown, issuedTaxKnown, receivedTaxKnown := false, false, false, false
	for _, fact := range ctx.issued {
		if fact.HasAmount {
			issuedAmount += fact.Amount
			issuedAmountKnown = true
		}
		if fact.HasTax {
			issuedTax += fact.Tax
			issuedTaxKnown = true
		}
	}
	for _, fact := range ctx.received {
		if fact.HasAmount {
			receivedAmount += fact.Amount
			receivedAmountKnown = true
		}
		if fact.HasTax {
			receivedTax += fact.Tax
			receivedTaxKnown = true
		}
	}
	periodHeader := analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "金额/数量"
	return []Block{{Kind: BlockTable, Title: "发票总体情况", Headers: []string{"项目", periodHeader, "说明"}, Rows: [][]string{
		{"销项发票份数", fmt.Sprint(invoiceCount(ctx.issued)), "有效记录"},
		{"销项不含税金额", optionalMoney(issuedAmount, issuedAmountKnown), "按统一发票口径汇总"},
		{"销项票面税额", optionalMoney(issuedTax, issuedTaxKnown), "已取得发票的票面税额"},
		{"进项发票份数", fmt.Sprint(invoiceCount(ctx.received)), "有效记录"},
		{"进项不含税金额", optionalMoney(receivedAmount, receivedAmountKnown), "按统一发票口径汇总"},
		{"进项票面税额", optionalMoney(receivedTax, receivedTaxKnown), "已取得发票的票面税额"},
		{"销项红字金额", moneyText(ctx.redIssuedAmount), "红字不含税金额绝对值"},
	}}}
}

func buildInvoiceAmountTrend(ctx *reportContext) []Block {
	chart := invoiceTrendChart(ctx.modules, ctx.task.StartTime, ctx.task.EndTime)
	if chart == nil {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前进销项发票模块调用成功，但本分析期间未返回可形成金额趋势的数据。"}}}
	}
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart, Paragraphs: []string{chart.Summary}}}
}

func buildInvoiceTaxComparison(ctx *reportContext) []Block {
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, false)
	values, inputValues := make([]float64, len(periods)), make([]float64, len(periods))
	known, inputKnown := make([]bool, len(periods)), make([]bool, len(periods))
	for i, p := range periods {
		start, end := periodBounds(p)
		values[i], known[i] = invoiceTaxForPeriod(ctx, start, end)
		inputValues[i], inputKnown[i] = selectedTaxForPeriod(ctx, start, end)
		inputKnown[i] = inputKnown[i] && ctx.taxpayer == taxpayerGeneral
	}
	series := []Series{{Name: "销项票面税额", Values: values, Available: known, Color: chartColorPrimary}}
	if ctx.taxpayer != taxpayerSmall {
		series = append(series, Series{Name: "已勾选有效抵扣税额", Values: inputValues, Available: inputKnown, Color: chartColorPositive})
	}
	chart := &Chart{ID: "invoice-tax-trend", Kind: chartKindMultiLine, Title: "进销项税额趋势", Labels: periods, Unit: "元", Series: series}
	blocks := []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}}
	var rows [][]string
	for _, c := range invoiceTaxComparisons(ctx) {
		for _, input := range []bool{false, true} {
			name, tax, has, filing, hasFiling := "销项", c.Output, c.HasOutput, c.VAT.OutputTax, c.VAT.HasOutputTax
			if input {
				if ctx.taxpayer == taxpayerSmall {
					continue
				}
				name, tax, has, filing, hasFiling = "进项（有效抵扣）", c.Input, c.HasInput, c.VAT.InputTax, c.VAT.HasInputTax
			}
			if !input {
				if unbilled, ok, _ := outputUnbilledTax(ctx, c.VAT); ok {
					filing -= unbilled
					name = "销项（已扣未开票申报税额）"
				}
			}
			rows = append(rows, []string{c.VAT.Period, name, optionalMoney(tax, has), optionalMoney(filing, hasFiling), optionalDifference(tax, has, filing, hasFiling), optionalPercent(invoiceTaxDifferenceRate(tax, filing), has && hasFiling)})
		}
	}
	if len(rows) > 0 {
		blocks = append(blocks, Block{Kind: BlockTable, Title: "进销项票申税额差异", Headers: []string{"所属期", "比较项目", "发票对应税额", "申报对应税额", "差额", "差异率"}, Rows: rows})
	}
	return blocks
}

func buildCustomers(ctx *reportContext) []Block {
	if ctx.customers == nil {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前销项发票数据未返回可识别的客户主体。"}}}
	}
	return counterpartyBlocks(ctx, ctx.customers, qixinImportantCustomerRole)
}
func buildSuppliers(ctx *reportContext) []Block {
	if ctx.suppliers == nil {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前进项发票数据未返回可识别的供应商主体。"}}}
	}
	return counterpartyBlocks(ctx, ctx.suppliers, qixinImportantSupplierRole)
}

// counterpartyBlocks 构建动态TopN交易主表，并合并后端已精确核验的公开信用和直接关系。
//
// 参数 ctx 为报告上下文，analysis 为发票汇总结果，role 为启信宝主体角色；返回图表和主表区块。
func counterpartyBlocks(ctx *reportContext, analysis *counterpartyAnalysis, role string) []Block {
	rows := make([][]string, 0, len(analysis.Items))
	groups := ctx.issued
	if role == qixinImportantSupplierRole {
		groups = ctx.received
	}
	positiveBase := invoiceNetAmount(groups) > 0
	for _, item := range analysis.Items {
		rows = append(rows, []string{item.Name, moneyText(item.Amount), optionalPercent(item.Share, positiveBase), qixinCounterpartyCreditSummary(ctx, item, role), qixinVerifiedRelationSummary(ctx, item)})
	}
	first := "客户名称"
	amount := "销项金额"
	if role == qixinImportantSupplierRole {
		first, amount = "供应商名称", "进项金额"
	}
	return []Block{{Kind: BlockChart, Title: analysis.Chart.Title, Chart: analysis.Chart}, {Kind: BlockTable, Title: analysis.Title, Headers: []string{first, amount, "占比", "公开信用", "关联线索"}, Rows: rows}}
}

func buildRedOutput(ctx *reportContext) []Block {
	return redInvoiceSummaryBlocks(ctx)
}

func buildSensitiveServices(ctx *reportContext) []Block {
	byCategory, _ := sensitiveServiceTotals(ctx.received)
	if len(byCategory) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"发票类别", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "金额", "关注事项"}, Rows: [][]string{{"敏感服务类发票", "—", "按当前已取得的税局数据，未识别到本节所列敏感服务类发票记录。"}}}}
	}
	keys := sortedMapKeys(byCategory)
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{key, moneyText(byCategory[key].amount), sensitiveServiceAttention(key)})
	}
	return []Block{{Kind: BlockTable, Title: "敏感服务类发票分类", Headers: []string{"发票类别", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "金额", "关注事项"}, Rows: rows}}
}

func buildSignalSection(ctx *reportContext, ruleID string) []Block {
	rows := make([][]string, 0)
	for _, evaluation := range ctx.evaluations {
		if evaluation.RuleID != ruleID || evaluation.Outcome != OutcomeRisk || evaluation.Finding == nil {
			continue
		}
		finding := *evaluation.Finding
		rows = append(rows, []string{strings.Join(finding.SignalTypes, "、"), finding.Evidence, string(finding.Level)})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"组合指标", "检测结果", "结论"}, Rows: [][]string{{"相关发票线索", "未识别到疑似发票异常的明显线索", "未见明显异常"}}}}
	}
	return []Block{{Kind: BlockTable, Title: "风险线索明细", Headers: []string{"组合指标", "检测结果", "结论"}, Rows: rows}}
}

func buildPaymentDetail(ctx *reportContext) []Block {
	return buildTaxPaymentMatrix(ctx)
}

func buildPaymentComposition(ctx *reportContext) []Block {
	chart := paymentCompositionChart(ctx.payments)
	if chart == nil {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前缴款记录未返回可形成税费构成图的数据。"}}}
	}
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart, Paragraphs: []string{chart.Summary}}}
}

func buildMonthlyTaxBurden(ctx *reportContext) []Block {
	return taxBurdenBlocks(ctx)
}

func buildVATOverview(ctx *reportContext) []Block {
	rows := make([][]string, 0, len(ctx.vat))
	for _, fact := range ctx.vat {
		issuedAmount, issuedKnown := invoiceAmountForTaxPeriod(ctx.issued, fact.TaxPeriod)
		receivedAmount, receivedTax, receivedKnown, taxKnown := certifiedPurchasesForPeriod(ctx, fact.TaxPeriod)
		burdenKnown := fact.HasSales && fact.Sales != 0 && fact.HasSupplementTax
		rows = append(rows, []string{fact.Period, optionalMoney(fact.Sales, fact.HasSales), optionalMoney(issuedAmount, issuedKnown), optionalMoney(receivedAmount, receivedKnown), optionalMoney(receivedTax, taxKnown), optionalMoney(fact.SupplementTax, fact.HasSupplementTax), optionalPercent(safeRatio(fact.SupplementTax, fact.Sales), burdenKnown)})
	}
	return []Block{{Kind: BlockTable, Title: "增值税申报指标", Headers: []string{"所属期", "申报销售额", "销项发票金额", "认证购进金额", "认证购进税额", "本期应补（退）增值税", "税负率"}, Rows: rows}}
}

func buildVATCycleComparison(ctx *reportContext) []Block {
	rows := make([][]string, 0, len(ctx.vat))
	for index, fact := range ctx.vat {
		salesChange, taxChange, direction := "—", "—", "—"
		if index > 0 && ctx.vat[index-1].HasSales && fact.HasSales && ctx.vat[index-1].Sales != 0 {
			salesDelta := (fact.Sales - ctx.vat[index-1].Sales) / abs(ctx.vat[index-1].Sales)
			salesChange = percentText(salesDelta)
			if ctx.vat[index-1].HasSupplementTax && fact.HasSupplementTax && ctx.vat[index-1].SupplementTax != 0 {
				taxDelta := (fact.SupplementTax - ctx.vat[index-1].SupplementTax) / abs(ctx.vat[index-1].SupplementTax)
				taxChange = percentText(taxDelta)
				if (salesDelta > 0 && taxDelta < 0) || (salesDelta < 0 && taxDelta > 0) {
					direction = "变动方向相反"
				} else {
					direction = "变动方向一致"
				}
			}
		}
		rows = append(rows, []string{fact.Period, salesChange, taxChange, direction})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"比较期间", "销售额变动", "应纳税额变动", "判断"}}}
	}
	return []Block{{Kind: BlockTable, Title: vatCycleTitle(ctx), Headers: []string{"比较期间", "销售额变动", "应纳税额变动", "判断"}, Rows: rows}}
}

func buildVATInvoiceDifference(ctx *reportContext) []Block {
	rows := make([][]string, 0, len(ctx.vat))
	for _, fact := range ctx.vat {
		issuedAmount, issuedKnown := invoiceSalesForVAT(ctx, fact.TaxPeriod)
		valid := fact.HasSales && fact.Sales > 0 && issuedKnown
		difference := 0.0
		ratio := 0.0
		if fact.HasSales && issuedKnown {
			difference = fact.Sales - issuedAmount
		}
		if valid {
			ratio = abs(difference) / fact.Sales
		}
		rows = append(rows, []string{fact.Period, optionalMoney(issuedAmount, issuedKnown), optionalMoney(fact.Sales, fact.HasSales), optionalMoney(difference, fact.HasSales && issuedKnown), optionalPercent(ratio, valid)})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"所属期", "销项发票金额", "申报销售额", "差异额", "差异率"}}}
	}
	return []Block{{Kind: BlockTable, Title: "票申销售额差异", Headers: []string{"所属期", "销项发票金额", "申报销售额", "差异额", "差异率"}, Rows: rows}}
}

// invoiceAmountForTaxPeriod aggregates invoice months covered by one actual
// tax-authority reporting period. Parameters are one invoice direction and the
// normalized tax period. Return values distinguish a real zero from no invoice
// data and work for monthly, quarterly, annual, daily, or irregular periods.
func invoiceAmountForTaxPeriod(facts []invoiceGroup, period TaxPeriod) (float64, bool) {
	startMonth := extractMonth(period.Start)
	endMonth := extractMonth(period.End)
	if startMonth == "" || endMonth == "" {
		startMonth, endMonth = period.BucketMonth, period.BucketMonth
	}
	amounts := aggregateInvoiceAmounts(facts, true)
	total, known := 0.0, false
	for month, amount := range amounts {
		if month < startMonth || month > endMonth {
			continue
		}
		total += amount
		known = true
	}
	return total, known
}

func buildEndCredit(ctx *reportContext) []Block {
	periods := completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, true)
	ending := map[string]vatFact{}
	for _, fact := range ctx.vat {
		month := fact.BucketMonth
		if len(month) != 7 || !containsAny(month[5:], "03", "06", "09", "12") {
			continue
		}
		ending[quarterlyPeriodLabel(month, ctx.task.StartTime, ctx.task.EndTime)] = fact
	}
	rows := [][]string{}
	values, available := make([]float64, len(periods)), make([]bool, len(periods))
	for i, period := range periods {
		fact, found := ending[period]
		values[i], available[i] = fact.EndCredit, found && fact.HasEndCredit
		change := "—"
		if i > 0 && available[i] && available[i-1] {
			change = moneyText(values[i] - values[i-1])
		}
		rows = append(rows, []string{period, optionalMoney(values[i], available[i]), change, "—"})
	}
	chart := &Chart{ID: "vat-end-credit", Kind: chartKindLine, Title: "期末留抵税额趋势", Labels: periods, Unit: "元", Series: []Series{{Name: "期末留抵税额", Values: values, Available: available, Color: chartColorPrimary}}}
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}, {Kind: BlockTable, Title: "期末留抵税额", Headers: []string{"季度末", "期末留抵税额", "变化", "关注"}, Rows: rows}}
}

func buildCITContribution(ctx *reportContext) []Block {
	var rows [][]string
	for _, point := range citContributionPoints(ctx) {
		known := point.taxKnown && point.incomeKnown && point.income != 0
		ratio := safeRatio(point.tax, point.income)
		rows = append(rows, []string{point.period, optionalMoney(point.tax, point.taxKnown), optionalPercent(ratio, known), point.note})
		ctx.recordCalculation("6.1", point.period, "所得税贡献率", "同所属期申报应补退所得税额 / 企税申报营业收入（季度累计还原单季，年度单列）", append([]CalculationInput{taxCalculationInput(point.taxSource)}, point.incomeInputs...), ratio, known)
	}
	return []Block{{Kind: BlockTable, Title: citContributionTitle, Headers: citContributionHeaders, Rows: rows}}
}

func buildFinancialOverview(ctx *reportContext) []Block {
	if len(ctx.financial) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前利润表数据未返回可展示的收入、成本或利润指标。"}}}
	}
	labels := make([]string, len(ctx.financial))
	revenueValues, costValues, profitValues := make([]float64, len(ctx.financial)), make([]float64, len(ctx.financial)), make([]float64, len(ctx.financial))
	revenueAvailable, costAvailable, profitAvailable := make([]bool, len(ctx.financial)), make([]bool, len(ctx.financial)), make([]bool, len(ctx.financial))
	rows := make([][]string, 0, len(ctx.financial))
	for index, fact := range ctx.financial {
		labels[index] = fact.Period
		revenueValues[index], revenueAvailable[index] = fact.Revenue, fact.HasRevenue
		costValues[index], costAvailable[index] = fact.Cost, fact.HasCost
		profitValues[index], profitAvailable[index] = fact.Profit, fact.HasProfit
		rows = append(rows, []string{fact.Period, optionalMoney(fact.Revenue, fact.HasRevenue), optionalMoney(fact.Cost, fact.HasCost), optionalMoney(fact.Profit, fact.HasProfit), optionalMoney(fact.NetProfit, fact.HasNetProfit)})
	}
	chart := &Chart{
		ID: "financial-income-cost-profit-trend", Kind: chartKindLine, Title: "收入、成本及利润趋势", Labels: labels,
		Series: []Series{
			{Name: "营业收入", Values: revenueValues, Available: revenueAvailable, Color: chartColorPrimary},
			{Name: "营业成本", Values: costValues, Available: costAvailable, Color: chartColorPositive},
			{Name: "利润总额", Values: profitValues, Available: profitAvailable, Color: chartColorWarning},
		},
		Summary: "按税务报送利润表实际期间展示；字段缺失显示为空缺，不补零。",
	}
	return []Block{
		{Kind: BlockChart, Title: chart.Title, Chart: chart},
		{Kind: BlockTable, Title: "收入、成本及利润", Headers: []string{"期间", "营业收入", "营业成本", "利润总额", "净利润"}, Rows: rows},
	}
}

func buildPeriodExpense(ctx *reportContext) []Block {
	rows := make([][]string, 0, len(ctx.financial))
	type ratios struct {
		values [3]float64
		known  [3]bool
	}
	byPeriod := map[string]ratios{}
	notices := map[string][]string{}
	for _, fact := range ctx.financial {
		status := "无法判断是否重复"
		total, totalOK := 0.0, false
		if fact.ResearchInManagementSet {
			if fact.ResearchInManagement {
				status = "管理费用已含研发费用"
				total, totalOK = sumKnown(
					optionalValue(fact.SalesExpense, fact.HasSalesExpense),
					optionalValue(fact.ManagementExpense, fact.HasManagementExpense),
					optionalValue(fact.FinanceExpense, fact.HasFinanceExpense),
				)
			} else {
				status = "研发费用独立列报"
				total, totalOK = sumKnown(
					optionalValue(fact.SalesExpense, fact.HasSalesExpense),
					optionalValue(fact.ManagementExpense, fact.HasManagementExpense),
					optionalValue(fact.ResearchExpense, fact.HasResearchExpense),
					optionalValue(fact.FinanceExpense, fact.HasFinanceExpense),
				)
			}
		}
		expenseRatioOK := totalOK && fact.HasRevenue && fact.Revenue != 0
		expenseRatio, grossMargin, profitMargin := 0.0, 0.0, 0.0
		grossOK := fact.HasRevenue && fact.HasCost && fact.Revenue != 0
		profitOK := fact.HasRevenue && fact.HasProfit && fact.Revenue != 0
		if expenseRatioOK {
			expenseRatio = total / fact.Revenue
		}
		if grossOK {
			grossMargin = (fact.Revenue - fact.Cost) / fact.Revenue
		}
		if profitOK {
			profitMargin = fact.Profit / fact.Revenue
		}
		from, to := periodBounds(fact.Period)
		byPeriod[from+"|"+to] = ratios{[3]float64{grossMargin, profitMargin, expenseRatio}, [3]bool{grossOK, profitOK, expenseRatioOK}}
		notices[status] = append(notices[status], fact.Period)
		rows = append(rows, []string{fact.Period, optionalPercent(grossMargin, grossOK), optionalPercent(profitMargin, profitOK), optionalPercent(expenseRatio, expenseRatioOK), "—"})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockTable, Headers: []string{"期间", "毛利率", "营业利润率", "期间费用率", "趋势"}}}
	}
	for _, row := range rows {
		from, to := periodBounds(row[0])
		current := byPeriod[from+"|"+to]
		previous, ok := byPeriod[previousFinancialPeriodKey(row[0])]
		if !ok {
			continue
		}
		var changes []string
		for i, name := range []string{"毛利率", "利润率", "费用率"} {
			if current.known[i] && previous.known[i] {
				changes = append(changes, fmt.Sprintf("%s%+.2f", name, (current.values[i]-previous.values[i])*100))
			}
		}
		if len(changes) > 0 {
			row[4] = strings.Join(changes, "；")
		}
	}
	paragraphs := []string{"趋势列为较相邻同周期的百分点变化；缺少可比期间时不计算。"}
	for _, status := range sortedMapKeys(notices) {
		paragraphs = append(paragraphs, strings.Join(notices[status], "、")+"："+status+"。")
	}
	return []Block{{Kind: BlockTable, Title: "毛利率、利润率及费用率", Headers: []string{"期间", "毛利率", "营业利润率", "期间费用率", "趋势"}, Rows: rows}, {Kind: BlockParagraph, Title: "口径说明", Paragraphs: paragraphs}}
}

func buildIndustryStructure(ctx *reportContext) []Block {
	if len(ctx.categories) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前销项发票数据未返回可形成业务结构的品类或项目名称。"}}}
	}
	labels, values := make([]string, len(ctx.categories)), make([]float64, len(ctx.categories))
	rows := make([][]string, 0, len(ctx.categories))
	for index, category := range ctx.categories {
		labels[index], values[index] = category.Name, category.Amount
		rows = append(rows, []string{fmt.Sprintf("第%d名", index+1), category.Name, moneyText(category.Amount), percentText(category.Share)})
	}
	chart := &Chart{ID: "business-structure", Kind: chartKindHorizontalBar, Title: "销项发票主要业务结构", Labels: labels, Series: []Series{{Name: "销项不含税金额", Values: values, Color: chartColorPrimary}}, Summary: "展示销项发票主要品类或项目名称及其不含税金额。"}
	completeBusinessPie(ctx, chart)
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart, Paragraphs: []string{chart.Summary}}, {Kind: BlockTable, Title: "主要业务", Headers: []string{"排名", "业务品类", "销项不含税金额", "占比"}, Rows: rows}}
}

func buildBusinessCategory(category businessCategory, rank int) []Block {
	customers, topShare := fmt.Sprint(category.Customers), percentText(category.TopOneShare)
	if category.UnknownCustomers {
		customers = fmt.Sprintf("已识别%d个，另有身份缺失记录", category.Customers)
		topShare = "—"
	}
	baseRows := [][]string{
		{"销项不含税金额", moneyText(category.Amount)},
		{"占全部销项金额比例", percentText(category.Share)},
		{"涉及客户数量", customers},
		{"最大客户占该品类比例", topShare},
		{"覆盖期间数", fmt.Sprint(category.Periods)},
	}
	return []Block{{Kind: BlockTable, Title: category.Name + "经营特征", Headers: []string{"指标", "本期数据"}, Rows: baseRows}}
}

func riskCounts(findings []Finding) map[RiskLevel]int {
	result := map[RiskLevel]int{RiskHigh: 0, RiskMedium: 0, RiskLow: 0}
	for _, finding := range findings {
		result[finding.Level]++
	}
	return result
}

func riskPeriods(findings []Finding) []string {
	seen := map[string]bool{}
	for _, finding := range findings {
		for _, period := range finding.Periods {
			if month := extractMonth(period); month != "" {
				seen[month] = true
			}
		}
	}
	return sortedMapKeys(seen)
}

func aggregateInvoiceTax(facts []invoiceGroup) (map[string]float64, map[string]bool) {
	result := map[string]float64{}
	known := map[string]bool{}
	for _, fact := range facts {
		if fact.Month != "" && fact.HasTax {
			result[fact.Month] += fact.Tax
			known[fact.Month] = true
		}
	}
	return result, known
}

func quarterNumber(period string) int {
	text := strings.ToUpper(strings.TrimSpace(period))
	for i, label := range []string{"1", "2", "3", "4"} {
		if strings.Contains(text, "第"+label+"季度") || strings.Contains(text, "第"+[]string{"一", "二", "三", "四"}[i]+"季度") {
			return i + 1
		}
	}
	if index := strings.LastIndex(text, "Q"); index >= 0 && index+1 < len(text) {
		value := int(text[index+1] - '0')
		if value >= 1 && value <= 4 {
			return value
		}
	}
	return 0
}

func aggregateInvoiceAmounts(facts []invoiceGroup, includeRed bool) map[string]float64 {
	result := map[string]float64{}
	for _, fact := range facts {
		if fact.Month == "" || !fact.HasAmount || (!includeRed && fact.Red) {
			continue
		}
		result[fact.Month] += fact.Amount
	}
	return result
}

func optionalDifference(left float64, leftOK bool, right float64, rightOK bool) string {
	if !leftOK || !rightOK {
		return "—"
	}
	return moneyText(left - right)
}

func boolLabel(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

type knownValue struct {
	value float64
	known bool
}

func optionalValue(value float64, known bool) knownValue {
	return knownValue{value: value, known: known}
}

func sumKnown(values ...knownValue) (float64, bool) {
	total := 0.0
	for _, value := range values {
		if !value.known {
			return 0, false
		}
		total += value.value
	}
	return total, len(values) > 0
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func moneyOrDash(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "—"
	}
	return moneyText(value)
}
