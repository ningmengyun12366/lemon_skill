package report

import (
	"fmt"
	"strings"
)

// buildConfirmedRemediationWithChart builds Word section 10.1 without adding
// a chart. Parameters contain all confirmed findings. Return value uses the
// exact five-column priority table required by the report contract.
func buildConfirmedRemediationWithChart(ctx *reportContext) []Block {
	findings := impactSortedFindings(ctx.findings)
	if len(findings) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"本期未识别到需要优先处理的明确风险事项。"}}}
	}
	rows := make([][]string, 0, len(findings))
	for index, finding := range findings {
		rows = append(rows, []string{
			fmt.Sprint(index + 1),
			finding.Title,
			string(finding.Level),
			remediationDeadline(finding.Level),
			textOr(strings.TrimSpace(finding.Recommendation), "结合税局记录核实并完成处理"),
		})
	}
	return []Block{{Kind: BlockTable, TableVariant: TableVariantRemediation, Headers: []string{"序号", "事项", "等级", "建议完成时限", "建议处理"}, Rows: rows}}
}

// buildAttentionItems builds Word section 10.2 from findings that explicitly
// require verification. Parameters are the report context. Return value never
// promotes an ordinary observation to a remediation item.
func buildAttentionItems(ctx *reportContext) []Block {
	rows := make([][]string, 0)
	for _, finding := range impactSortedFindings(ctx.findings) {
		if !finding.VerificationRequired {
			continue
		}
		rows = append(rows, []string{finding.Title, string(finding.Level), textOr(strings.TrimSpace(finding.Evidence), "核实相关业务资料与税局记录")})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"本期无重点核实事项", "—", "—"})
	}
	return []Block{{Kind: BlockTable, Headers: []string{"事项", "等级", "核实重点"}, Rows: rows}}
}

// buildMonitoringItems builds Word section 10.3 as a concise monitoring list.
// Parameter ctx supplies only facts already shown in prior chapters. Return
// value contains no table or chart.
func buildMonitoringItems(ctx *reportContext) []Block {
	items := make([]string, 0, 5)
	if ctx.redIssuedAmount > 0 {
		items = append(items, "持续观察红字发票金额及原因结构。")
	}
	if ctx.customers != nil && len(ctx.customers.Items) > 0 {
		items = append(items, "持续观察主要客户集中度变化。")
	}
	if ctx.suppliers != nil && len(ctx.suppliers.Items) > 0 {
		items = append(items, "持续观察主要供应商集中度变化。")
	}
	items = append(items, "持续核对申报、缴款与发票数据的一致性。")
	if len(items) < 3 {
		items = append(items, "持续关注企业公开信用信息的变化。")
	}
	return []Block{{Kind: BlockParagraph, Paragraphs: items}}
}

// buildRemediationTracking builds Word section 10.4. Parameter ctx supplies
// confirmed findings. Return value preserves the exact six-column tracking
// table, including one neutral row when no finding exists.
func buildRemediationTracking(ctx *reportContext) []Block {
	rows := make([][]string, 0, len(ctx.findings))
	for _, finding := range impactSortedFindings(ctx.findings) {
		rows = append(rows, []string{finding.Title, "待指定", "待确定", "待核实", textOr(strings.TrimSpace(finding.Recommendation), "待确定"), "未开始"})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"无", "—", "—", "—", "—", "—"})
	}
	return []Block{{Kind: BlockTable, Headers: []string{"风险事项", "责任人", "计划完成日期", "核实结果", "处理措施", "状态"}, Rows: rows}}
}

// counterpartyConcentrationText summarizes only the displayed concentration
// facts. Parameter items is the ranked list. Return value avoids operational or
// supply-chain resilience inferences.
func counterpartyConcentrationText(items []counterpartyItem) string {
	if len(items) == 0 {
		return "暂无可展示数据"
	}
	total := 0.0
	for _, item := range items {
		total += item.Share
	}
	return fmt.Sprintf("首位占比%s，已展示主体合计占比%s", percentText(items[0].Share), percentText(total))
}

// buildDataPendingItems centralizes report-data gaps so they are not repeated
// as enterprise remediation in every chapter. Parameters are the report
// context. Return value contains only gaps that affect this report.
func buildDataPendingItems(ctx *reportContext) []Block {
	rows := make([][]string, 0)
	appendRow := func(item, impact, next string) {
		rows = append(rows, []string{item, impact, next})
	}
	if ctx.taxpayer == taxpayerUnknown {
		appendRow("纳税人资格", "一般纳税人专项章节暂无法判断是否适用", "待税局资格信息返回后自动分析")
	}
	if !hasFinancialReportData(ctx) {
		appendRow("财务报表", "财务指标章节本期暂不分析", "取得税务报送财务报表后自动分析")
	}
	if len(ctx.categories) == 0 {
		appendRow("发票品类", "业务结构及行业章节本期暂不分析", "优先核查报告接口的品类字段映射")
	}
	if invoiceCategoryFieldMissing(ctx.issued) || invoiceCategoryFieldMissing(ctx.received) {
		appendRow("发票关键字段", "当前报告接口未完整返回品类或项目字段", "优先核查数据采集和字段映射")
	}
	if !ctx.modules[degradableModuleCode].Complete {
		appendRow("欠税及退税", "仅展示已取得记录，不作无异常判断", "待对应税局来源恢复后补充分析")
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"本期没有需要集中列示的数据待完善事项。"}}}
	}
	return []Block{{Kind: BlockTable, Title: "数据待完善清单", Headers: []string{"数据项目", "影响范围", "后续处理"}, Rows: rows}}
}

// invoiceCategoryFieldMissing reports whether at least one invoice lacks a
// returned category or project name. Parameter facts uses the canonical report
// scope. Return value describes report-interface coverage, not original invoice
// compliance.
func invoiceCategoryFieldMissing(facts []invoiceGroup) bool {
	for _, fact := range facts {
		if !invoiceHasCategoryData(fact) {
			return true
		}
	}
	return false
}
