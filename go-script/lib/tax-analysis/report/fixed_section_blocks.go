package report

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// buildAssociationClues builds the fixed related-transaction clue section.
// Parameter ctx is the finalized report context. Return value contains only
// counterparties whose direct shareholder or branch relationship was verified
// by the QiXin result in the same tax-analysis task.
func buildAssociationClues(ctx *reportContext) []Block {
	rows := make([][]string, 0)
	coverageInsufficient := false
	appendRows := func(role string, analysis *counterpartyAnalysis) {
		if analysis == nil {
			return
		}
		for _, item := range analysis.Items {
			state := qixinDirectRelation(ctx, item)
			coverageInsufficient = coverageInsufficient || state == qixinRelationInsufficient
			if state != qixinRelationShareholder && state != qixinRelationBranch {
				continue
			}
			relation := state.summary()
			rows = append(rows, []string{role, item.Name, textOr(item.TaxID, "—"), relation, moneyText(item.Amount), percentText(item.Share), "需结合合同、物流、资金及实际业务进一步核实"})
		}
	}
	appendRows("客户", ctx.customers)
	appendRows("供应商", ctx.suppliers)
	contractRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		contractRows = append(contractRows, []string{row[3], row[1], "需结合合同及业务资料核实"})
	}
	if len(contractRows) == 0 {
		if coverageInsufficient {
			contractRows = append(contractRows, []string{"关系核验覆盖不足", "—", "未取得完整的直接股东或分支机构资料，暂无法判断关系线索"})
		} else {
			contractRows = append(contractRows, []string{"未识别到明确关联线索", "—", associationNoClueText})
		}
	}
	return []Block{{Kind: BlockTable, Title: "关联交易线索", Headers: []string{"识别线索", "涉及主体", "结论"}, Rows: contractRows}}
}

// associationRelationState summarizes only the reporting company's direct
// shareholder/branch name checks. It says nothing about the wider equity graph.
func associationRelationState(ctx *reportContext) qixinRelationState {
	state := qixinRelationNotFound
	for _, parties := range []*counterpartyAnalysis{ctx.customers, ctx.suppliers} {
		if parties == nil {
			continue
		}
		for _, item := range parties.Items {
			current := qixinDirectRelation(ctx, item)
			if current == qixinRelationShareholder || current == qixinRelationBranch {
				return current
			}
			if current == qixinRelationInsufficient {
				state = current
			}
		}
	}
	return state
}

// buildTaxPaymentExceptions builds the fixed filing-correction, overdue and
// arrears section. Parameter ctx is the finalized report context. Return value
// preserves degraded-module uncertainty and never turns missing data into a
// no-abnormality conclusion.
func buildTaxPaymentExceptions(ctx *reportContext) []Block {
	rows := make([][]string, 0)
	for _, fact := range ctx.declarations {
		if !containsAny(fact.Status, "更正", "逾期", "未申报", "催报", "作废") {
			continue
		}
		rows = append(rows, []string{"申报", fact.Period.Display, textOr(fact.TaxName, "未识别税费种"), "—", textOr(fact.Status, "未获取"), "以税局申报记录为准"})
	}
	module := ctx.modules[degradableModuleCode]
	for _, object := range arrearsBusinessRecords(module.Raw) {
		amountAliases := append(append([]string(nil), arrearsExplicitAmountAliases...), arrearsGenericAmountAliases...)
		amount, amountKnown := mapNumber(object, amountAliases...)
		rows = append(rows, []string{
			"欠税", textOr(recordPeriod(object), "未获取"), textOr(mapText(object, arrearsTaxNameAliases...), "未识别税费种"),
			optionalMoney(amount, amountKnown), textOr(mapText(object, append(append([]string(nil), arrearsExplicitStatusAliases...), "status", "state")...), "未获取"),
			textOr(mapText(object, "sourceName", "department", "authorityName", "来源"), "税局查询结果"),
		})
	}
	resultRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		resultRows = append(resultRows, []string{row[0], row[1] + "；" + row[2] + "；" + row[3] + "；" + row[4], "需关注"})
	}
	// Authority-confirmed filing/payment exceptions need the same evidence here
	// as in the risk summary; otherwise this section could claim no issue.
	for _, evaluation := range ctx.evaluations {
		if finding := evaluation.Finding; evaluation.Outcome == OutcomeRisk && finding != nil && (evaluation.RuleID == "TAX-FILING-001" || evaluation.RuleID == "TAX-PAYMENT-001") {
			resultRows = append(resultRows, []string{finding.Title, finding.Evidence, string(finding.Level)})
		}
	}
	if history, present := namedCollectionMapsWithPresence(ctx.modules["filing_obligations"].Raw, "recordHistory"); present {
		corrections := 0
		for _, record := range history {
			if containsAny(mapText(record, "gzlx", "status", "declarationStatus"), "更正") {
				corrections++
			}
		}
		resultRows = append(resultRows, []string{"申报更正记录", fmt.Sprintf("已取得申报历史中有%d条更正记录", corrections), "更正本身不认定为风险"})
	}
	if todo, present := namedCollectionMapsWithPresence(ctx.modules["filing_obligations"].Raw, "todoRecords"); present {
		resultRows = append(resultRows, []string{"申报待办", fmt.Sprintf("本次查询取得%d条待办记录", len(todo)), "与已确认申报异常分别列示"})
	}
	for _, evaluation := range ctx.evaluations {
		if evaluation.RuleID == "TAX-ARREARS-001" && evaluation.Outcome == OutcomeClear {
			resultRows = append(resultRows, []string{"欠税查询", evaluation.Evidence, "未查询到欠税记录"})
		}
	}
	if refunds, _, found := findFirst(module.Raw, "refunds"); found && module.Complete {
		count, known := refundRecordCount(refunds)
		if known && count == 0 {
			resultRows = append(resultRows, []string{"退税查询", "未查询到相关退税记录", "—"})
		} else if known {
			resultRows = append(resultRows, []string{"退税查询", fmt.Sprintf("已取得%d条相关退税记录", count), "以具体退税事项及处理状态为准"})
			for _, row := range refundBusinessRecords(module.Raw) {
				resultRows = append(resultRows, []string{"退抵事项", refundRecordDescription(row), "取得记录不代表已申请或已完成退款"})
			}
		} else {
			resultRows = append(resultRows, []string{"退税查询", "未取得可确认的退税记录", "待核实"})
		}
	}
	for _, event := range filingEvents(ctx.modules["filing_obligations"]) {
		if event.Kind == filingCorrected {
			continue
		}
		resultRows = append(resultRows, []string{"申报状态", filingEventText(event), map[string]string{filingNotDue: "未到期", filingUnknown: "待核实", filingOverdue: "逾期未申报", filingLateFiled: "已补报"}[event.Kind]})
	}
	if len(resultRows) == 0 {
		conclusion := "未见明显异常"
		text := taxExceptionNoFindingText
		if !module.Complete {
			conclusion, text = "数据不足", taxExceptionDegradedText
		}
		resultRows = append(resultRows, []string{"申报更正、逾期及欠税", text, conclusion})
	}
	blocks := []Block{{Kind: BlockTable, Title: "申报更正、逾期及欠税情况", Headers: []string{"事项", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "情况", "风险结论"}, Rows: resultRows}}
	if !module.Complete {
		blocks = append(blocks, Block{Kind: BlockParagraph, Title: "数据说明", Paragraphs: []string{taxExceptionDegradedText}})
	}
	return blocks
}

// refundRecordCount counts business data collections, never status envelopes.
func refundRecordCount(raw any) (int, bool) {
	count, known := 0, false
	for _, row := range findAllMaps(raw, func(row map[string]any) bool { _, ok := row["data"]; return ok }) {
		switch data := row["data"].(type) {
		case []any:
			count += len(data)
			known = true
		case map[string]any:
			if len(data) > 0 {
				count++
				known = true
			}
		case nil:
			if mapText(row, "status") == "EMPTY" {
				known = true
			}
		}
	}
	return count, known
}

// arrearsBusinessRecords extracts business rows only from the arrears branch.
// Parameter raw is the degradable aggregate payload. Return value excludes
// refund records and status-only wrapper metadata.
func arrearsBusinessRecords(raw any) []map[string]any {
	if arrearsValue, _, found := findFirst(raw, "arrearsInfo"); found {
		return findAllMaps(arrearsValue, func(object map[string]any) bool {
			amountAliases := append(append([]string(nil), arrearsExplicitAmountAliases...), arrearsGenericAmountAliases...)
			_, hasAmount := mapNumber(object, amountAliases...)
			hasBusinessField := mapText(object, arrearsTaxNameAliases...) != "" || recordPeriod(object) != "" || mapText(object, arrearsExplicitStatusAliases...) != ""
			return hasAmount || hasBusinessField
		})
	}
	return findAllMaps(raw, func(object map[string]any) bool {
		_, hasExplicitAmount := mapNumber(object, arrearsExplicitAmountAliases...)
		if hasExplicitAmount || mapText(object, arrearsExplicitStatusAliases...) != "" {
			return true
		}
		return containsAny(mapText(object, arrearsSemanticAliases...), "欠税", "欠缴", "催缴")
	})
}

// buildInputDeductionRisk builds the fixed general-taxpayer input-deduction
// section. Parameter ctx is the finalized report context. Return value contains
// only confirmation facts returned by the tax authority and does not infer a
// risk from a missing optional field.
func buildInputDeductionRisk(ctx *reportContext) []Block {
	return []Block{
		{Kind: BlockTable, Title: "进项抵扣检查汇总", Headers: []string{"检测事项", "结果", "风险等级"}, Rows: inputDeductionCheckRows(ctx)},
		{Kind: BlockParagraph, Title: "口径说明", Paragraphs: []string{
			"异常凭证：结论限于已取得的异常凭证查询范围；状态冲突或仅有勾选状态时，尚不能确定已申报抵扣。",
			"不得抵扣项目：凭品名仅能核验已明确业务；用途及其他不得抵扣情形资料未齐。",
			"进项税额转出：各期发生额合计；不据总额认定逐张异常凭证已处理。",
			"勾选与申报差异：沿用同所属期票申比对结果，差额不直接认定违规抵扣。",
		}},
	}
}

type deductionConfirmationFact struct {
	Count         int
	Period        string
	InvoiceNumber string
	Purpose       string
	Status        string
	Amount        float64
	AmountKnown   bool
	Attention     bool
}

// normalizeDeductionPeriod converts YYYYMM and ordinary date values to a
// stable month. Parameter value is the returned tax period. Return value is
// YYYY-MM when recognized, otherwise the trimmed source text.
func normalizeDeductionPeriod(value string) string {
	text := strings.TrimSpace(value)
	digits := strings.NewReplacer("-", "", "/", "", ".", "").Replace(text)
	if len(digits) == 6 {
		year, yearErr := strconv.Atoi(digits[:4])
		month, monthErr := strconv.Atoi(digits[4:])
		if yearErr == nil && monthErr == nil && year >= 1900 && month >= 1 && month <= 12 {
			return fmt.Sprintf("%04d-%02d", year, month)
		}
	}
	if month := extractMonth(text); month != "" {
		return month
	}
	return text
}

// usableDeductionInvoiceNumber prefers the all-digital invoice number returned
// by the confirmation interface and ignores placeholder zero codes. Parameter
// object is one confirmation row. Return value is empty when no usable number
// is present.
func usableDeductionInvoiceNumber(object map[string]any) string {
	for _, alias := range deductionInvoiceAliases {
		value := strings.TrimSpace(mapText(object, alias))
		if value == "" || strings.Trim(value, "0") == "" {
			continue
		}
		return value
	}
	return ""
}

// deductionSummaryRows aggregates confirmation records by period, purpose and
// status. Parameter facts contains direct module records. Return value is sorted
// and preserves whether any amount was actually returned.
func deductionSummaryRows(facts []deductionConfirmationFact) [][]string {
	type aggregate struct {
		Period, Purpose, Status string
		Count                   int
		Amount                  float64
		AmountKnown             bool
	}
	groups := map[string]*aggregate{}
	for _, fact := range facts {
		key := strings.Join([]string{fact.Period, fact.Purpose, fact.Status}, "\x00")
		group := groups[key]
		if group == nil {
			group = &aggregate{Period: fact.Period, Purpose: fact.Purpose, Status: fact.Status}
			groups[key] = group
		}
		group.Count += fact.Count
		if fact.AmountKnown {
			group.Amount += fact.Amount
			group.AmountKnown = true
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		rows = append(rows, []string{group.Period, group.Purpose, group.Status, fmt.Sprint(group.Count), optionalMoney(group.Amount, group.AmountKnown)})
	}
	return rows
}

// deductionAttentionRows returns only records whose authority status or purpose
// requires review. Parameter facts contains normalized records. Return value is
// stable and suitable for the bounded detail table.
func deductionAttentionRows(facts []deductionConfirmationFact) [][]string {
	rows := make([][]string, 0)
	for _, fact := range facts {
		if !fact.Attention {
			continue
		}
		rows = append(rows, []string{fact.Period, textOr(fact.InvoiceNumber, "—"), fact.Purpose, fact.Status, optionalMoney(fact.Amount, fact.AmountKnown), "需核实凭证状态及抵扣处理"})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i][0] != rows[j][0] {
			return rows[i][0] < rows[j][0]
		}
		return rows[i][1] < rows[j][1]
	})
	return rows
}

// buildCITAdjustments builds the fixed annual corporate-income-tax adjustment
// section. Parameter ctx is the finalized report context. Return value contains
// configured adjustment items only, with a neutral fallback when the annual
// return does not expose those fields.
func buildCITAdjustments(ctx *reportContext) []Block {
	if rows := citAdjustmentRows(ctx); len(rows) > 0 {
		return []Block{{Kind: BlockTable, Title: "主要纳税调整项目", Headers: []string{"调整项目", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "申报金额", "关注"}, Rows: rows}}
	}
	rows := metricRows(ctx.modules["cit_annual_returns"].Raw, annualAdjustmentMetrics)
	contractRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		contractRows = append(contractRows, []string{row[1], row[0] + "申报金额" + row[2], "需关注实际申报口径"})
	}
	if len(contractRows) == 0 {
		contractRows = append(contractRows, []string{"纳税调整项目", annualAdjustmentNoDataText, "—"})
	}
	return []Block{{Kind: BlockTable, Title: "主要纳税调整项目", Headers: []string{"调整项目", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "申报金额", "关注"}, Rows: contractRows}}
}

// buildStampTaxAnalysis builds the fixed stamp-tax section. Parameter ctx is
// the finalized report context. Return value combines tax-source facts and
// matching paid-tax records without inferring omitted declarations.
func buildStampTaxAnalysis(ctx *reportContext) []Block {
	return declarationLineBlocks(ctx, "印花税", func(name string) bool { return name == "印花税" })
}

// buildPropertyTaxAnalysis builds the fixed property-tax section. Parameter
// ctx is the finalized report context. Return value combines returned property
// tax-source facts and matching paid-tax records.
func buildPropertyTaxAnalysis(ctx *reportContext) []Block {
	return declarationLineBlocks(ctx, "房产税", func(name string) bool { return name == "房产税" })
}

// buildLandUseTaxAnalysis builds the fixed urban-land-use-tax section.
// Parameter ctx is the finalized report context. Return value combines returned
// land tax-source facts and matching paid-tax records.
func buildLandUseTaxAnalysis(ctx *reportContext) []Block {
	return declarationLineBlocks(ctx, "土地使用税", func(name string) bool { return name == "城镇土地使用税" })
}

func paymentRowsForTax(facts []paymentFact, match func(string) bool) [][]string {
	rows := make([][]string, 0)
	for _, fact := range facts {
		if match(fact.TaxName) {
			rows = append(rows, []string{normalizeQuarter(fact.Period), moneyText(fact.Amount)})
		}
	}
	return rows
}

func compactOtherTaxRows(name string, rows [][]string) [][]string {
	if len(rows) == 0 {
		return [][]string{{name, "—", "当前报告未取得可展示记录"}}
	}
	result := make([][]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, []string{name, row[0] + "实缴" + row[1], "已取得税局缴款记录"})
	}
	return result
}

func periodAmountTrendRows(rows [][]string, final string) [][]string {
	if len(rows) == 0 {
		return [][]string{{"—", "—", "—", "当前报告未取得可展示记录"}}
	}
	result := make([][]string, 0, len(rows))
	for index, row := range rows {
		change := "—"
		if index > 0 {
			change = "较上期变化"
		}
		result = append(result, []string{row[0], row[1], change, final})
	}
	return result
}

// buildSpecificTaxAnalysis builds one fixed other-tax section. Parameters are
// the report context, module code, Chinese title, numeric and text definitions,
// and a paid-tax matcher. Return value contains only exact returned facts.
func buildSpecificTaxAnalysis(ctx *reportContext, moduleCode, title string, metrics []metricDefinition, texts []textMetricDefinition, paymentMatch func(string) bool) []Block {
	blocks := make([]Block, 0, 3)
	if rows := metricRows(ctx.modules[moduleCode].Raw, metrics); len(rows) > 0 {
		blocks = append(blocks, Block{Kind: BlockTable, Title: title + "税源及申报金额", Headers: []string{"所属期", "项目", "申报值"}, Rows: rows})
	}
	if rows := textMetricRows(ctx.modules[moduleCode].Raw, texts); len(rows) > 0 {
		blocks = append(blocks, Block{Kind: BlockTable, Title: title + "税源属性", Headers: []string{"所属期", "项目", "申报值"}, Rows: rows})
	}
	paymentRows := make([][]string, 0)
	for _, payment := range ctx.payments {
		if paymentMatch(payment.TaxName) {
			paymentRows = append(paymentRows, []string{payment.Period, payment.TaxName, moneyText(payment.Amount), textOr(payment.TaxPeriod.Basis, "税款所属期")})
		}
	}
	if len(paymentRows) > 0 {
		blocks = append(blocks, Block{Kind: BlockTable, Title: title + "实缴记录", Headers: []string{"归集月份", "税费种", "实缴金额", "归集口径"}, Rows: paymentRows})
	}
	if len(blocks) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{fmt.Sprintf("当前报告未取得可展示的%s税源、申报或缴款记录。", title)}}}
	}
	blocks = append(blocks, Block{Kind: BlockParagraph, Paragraphs: []string{strings.TrimSpace(title) + "按税局返回的税源、申报和缴款记录展示。"}})
	return blocks
}

// buildCashFlow builds the fixed cash-flow subsection from typed financial
// facts. Parameter ctx contains matched cash-flow and profit values. Return
// value preserves missing values as dashes instead of inventing zeros.
func buildCashFlow(ctx *reportContext) []Block {
	rows := make([][]string, 0, len(ctx.financial))
	for _, fact := range ctx.financial {
		if !fact.HasOperatingCashFlow && !fact.HasNetProfit {
			continue
		}
		trend := "—"
		if fact.HasOperatingCashFlow && fact.HasNetProfit {
			trend = "对比展示"
		}
		rows = append(rows, []string{fact.Period, optionalMoney(fact.OperatingCashFlow, fact.HasOperatingCashFlow), optionalMoney(fact.NetProfit, fact.HasNetProfit), trend})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"—", "—", "—", cashFlowUnavailableText})
	}
	return []Block{{Kind: BlockTable, Headers: []string{"期间", "经营活动现金流量净额", "净利润", "趋势"}, Rows: rows}}
}
