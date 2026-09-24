package report

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// wordChartContract freezes the Word-required chart component for one source
// section. Required may be false only when Word explicitly permits a table-only
// fallback for a single comparable period.
type wordChartContract struct {
	TitleMatcher    func(string) bool
	Kind            string
	Title           string
	Series          []string
	Required        bool
	DynamicSeries   bool
	MaximumSeries   int
	AllowedMultiple bool
}

// wordSectionContract is the executable counterpart of
// references/word-report-contract.md. Tests use it to freeze titles, chart
// contracts and table headers; production rendering does not revalidate these
// static shapes on every run.
type wordSectionContract struct {
	Title        string
	TitleMatcher func(string) bool
	Chart        *wordChartContract
	Tables       [][]string
}

var wordChapterTitles = map[string]string{
	"1": "风险概览", "2": "企业综合信息", "3": "发票及购销链风险分析",
	"4": "税费申报缴纳全景", "5": "增值税风险分析", "6": "企业所得税风险分析",
	"7": "其他税种及附加风险", "8": "税务报送财务指标分析", "9": "行业专项风险分析",
	"10": "风险整改建议", "11": "综合结论",
}

var wordSectionContracts = map[string]wordSectionContract{
	"1.1":  {Title: "综合风险评价", Tables: [][]string{{"风险等级", "数量", "主要领域"}}},
	"1.2":  {Title: "风险等级分布", Chart: chartContract(chartKindDonut, "风险等级分布", "已完成检测数量")},
	"1.3":  {Title: "各模块风险分布", Chart: chartContract(chartKindStackedBar, "各模块风险分布", string(RiskHigh), string(RiskMedium), string(RiskLow))},
	"1.4":  {Title: "重点风险事项", Tables: [][]string{{"序号", "风险事项", "等级", "异常期间", "异常金额或情况"}, {"风险事项", "等级", "判断依据", "建议方向"}}},
	"1.5":  {Title: "风险变化趋势", Tables: [][]string{{"风险事项", "{dynamic-periods}", "趋势"}}},
	"2.1":  {Title: "工商及税务基本信息", Tables: [][]string{{"项目", "企业信息"}}},
	"2.2":  {Title: "股东及出资信息", Tables: [][]string{{"股东", "类型", "认缴金额", "实缴金额", "持股比例", "认缴期限"}}},
	"2.3":  {Title: "股权穿透及疑似关联方", Chart: dynamicChartContract(chartKindNetwork, "股权及关联关系图", true), Tables: [][]string{{"主体", "关系/线索", "识别结论"}}},
	"2.4":  {Title: "近三年纳税信用等级", Chart: chartContract(chartKindLine, "纳税信用等级变化", "信用等级序位")},
	"2.6":  {Title: "税务处罚及税收违法信息", Tables: [][]string{{"检测事项", "检测结果", "结论"}}},
	"2.7":  {Title: "行政处罚及经营异常", Tables: [][]string{{"检测事项", "检测结果", "结论"}}},
	"2.8":  {Title: "企业变更信息", Tables: [][]string{{"变更日期", "事项", "变更前", "变更后", "关注"}}},
	"2.9":  {Title: "司法执行及诉讼风险", Tables: [][]string{{"事项", "企业自身", "主要上下游", "关注"}}},
	"2.10": {Title: "行业及经营模式识别", Chart: dynamicChartContract(chartKindPie, "主要业务结构", true), Tables: [][]string{{"业务类型", "占比", "业务特征", "适用分析"}}},
	"3.1":  {TitleMatcher: suffixTitle("发票总体情况"), Tables: [][]string{{"项目", "{period}金额/数量", "说明"}}},
	"3.2":  {Title: "进销项金额趋势", Chart: chartContract(chartKindMultiLine, "进销项不含税金额趋势", "销项不含税金额", "进项不含税金额"), Tables: [][]string{{"所属期", "销项金额", "购进金额", "购销差额", "购进/销项比"}}},
	"3.3":  {Title: "进销项税额对比", Chart: dynamicChartContract(chartKindMultiLine, "进销项税额趋势", true), Tables: [][]string{{"所属期", "比较项目", "发票对应税额", "申报对应税额", "差额", "差异率"}}},
	"3.4":  {Title: "购销商品及服务匹配", Tables: [][]string{{"购进主要内容", "销项主要内容", "匹配判断", "关注"}}},
	"3.5":  {Title: "开票内容与经营业务匹配", Tables: [][]string{{"检测维度", "结果", "结论"}}},
	"3.6":  {TitleMatcher: counterpartyTitleMatcher("客户"), Chart: dynamicChartContract(chartKindHorizontalBar, "主要客户排名", true), Tables: [][]string{{"客户名称", "销项金额", "占比", "公开信用", "关联线索"}}},
	"3.7":  {TitleMatcher: counterpartyTitleMatcher("供应商"), Chart: dynamicChartContract(chartKindHorizontalBar, "主要供应商排名", true), Tables: [][]string{{"供应商名称", "进项金额", "占比", "公开信用", "关联线索"}}},
	"3.8":  {Title: "客户集中度分析", Chart: chartContract(chartKindGroupedColumn, "客户集中度", "客户集中度（%）"), Tables: [][]string{{"指标", "{period}", "风险提示"}}},
	"3.9":  {Title: "供应商集中度分析", Chart: chartContract(chartKindGroupedColumn, "供应商集中度", "供应商集中度（%）"), Tables: [][]string{{"指标", "{period}", "风险提示"}}},
	"3.10": {Title: "客户公开信用风险", Tables: [][]string{{"客户", "交易占比", "公开风险", "建议"}}},
	"3.11": {Title: "供应商公开信用风险", Tables: [][]string{{"供应商", "交易占比", "公开风险", "建议"}}},
	"3.12": {Title: "疑似关联交易线索", Tables: [][]string{{"识别线索", "涉及主体", "结论"}}},
	"3.13": {Title: "发票抬头、税号、品名、编码及税率", Tables: [][]string{{"检测事项", "检测结果", "风险等级"}}},
	"3.14": {Title: "特定业务发票字段及备注", Tables: [][]string{{"业务类型", "必填字段", "资料覆盖", "核验范围"}}},
	"3.15": {Title: "红字发票总体分析", Chart: chartContract(chartKindDualAxis, "红字发票金额及红字率趋势", "红字金额", "红字率"), Tables: [][]string{{"指标", "本期数据"}}},
	"3.16": {Title: "年末开票及次年红冲分析", Tables: [][]string{{"项目", "金额", "占比/说明"}}},
	"3.17": {Title: "敏感服务类发票分析", Chart: chartContract(chartKindGroupedColumn, "敏感服务类发票分类", "不含税金额"), Tables: [][]string{{"发票类别", "{period}金额", "关注事项"}}},
	"3.18": {Title: "疑似虚开销项发票线索", Tables: [][]string{{"组合指标", "检测结果", "结论"}}},
	"3.19": {Title: "疑似取得虚假进项发票线索", Tables: [][]string{{"组合指标", "检测结果", "结论"}}},
	"4.1":  {Title: "税费种及申报周期", Tables: [][]string{{"税费种", "申报周期", "{period}分析内容"}}},
	"4.2":  {TitleMatcher: suffixTitle("各期税费明细"), Tables: [][]string{{"所属期", "增值税", "附加税费", "企业所得税", "印花税", "房产税", "土地使用税", "工会经费", "合计"}}},
	"4.3":  {Title: "主要税种税额趋势", Chart: dynamicMultiChartContract(chartKindMultiLine, "主要税种税额趋势", 3)},
	"4.4":  {Title: "税费缴纳总额趋势", Chart: dynamicMultiChartContract(chartKindLine, "税费缴纳总额趋势", 1)},
	"4.5":  {Title: "税费构成", Chart: dynamicChartContract(chartKindDonut, "税费构成", true)},
	"4.6":  {TitleMatcher: taxBurdenTitleMatches, Chart: &wordChartContract{Kind: chartKindMultiLine, Title: taxBurdenMonthlyTitle, TitleMatcher: taxBurdenTitleMatches, Required: true, AllowedMultiple: true, Series: []string{"企业自身综合税负率", "全税费缴纳率"}}, Tables: [][]string{{"所属期", "企业自身综合税负率", "全税费缴纳率", "异常情况"}}},
	"4.7":  {Title: "申报更正、逾期及欠税情况", Tables: [][]string{{"事项", "{period}情况", "风险结论"}}},
	"5.1":  {Title: "各期销售额、进项及应纳税额", Tables: [][]string{{"所属期", "申报销售额", "销项发票金额", "认证购进金额", "认证购进税额", "本期应补（退）增值税", "税负率"}}},
	"5.2":  {Title: "增值税税负率月度趋势", Chart: chartContract(chartKindLine, "增值税税负率趋势", "增值税税负率")},
	"5.3":  {Title: "销售额与应纳税额变动", Tables: [][]string{{"比较期间", "销售额变动", "应纳税额变动", "判断"}}},
	"5.4":  {Title: "销项发票与申报销售额", Chart: chartContract(chartKindMultiLine, "销项发票与申报销售额", "销项发票金额", "申报销售额"), Tables: [][]string{{"所属期", "销项发票金额", "申报销售额", "差异额", "差异率"}}},
	"5.5":  {Title: "财务报表收入与增值税销售额", Tables: [][]string{{"所属期（单月/单季）", "税务报送财务报表营业收入", "增值税申报销售额", "差异", "关注"}}},
	"5.6": {Title: "未开票收入及红字发票", TitleMatcher: func(title string) bool {
		return title == "未开票收入及红字发票" || title == "票申差额及红字发票"
	}, Tables: [][]string{{"项目", "{period}金额", "分析"}}},
	"5.7":  {Title: "进项抵扣风险", Tables: [][]string{{"检测事项", "结果", "风险等级"}}},
	"5.8":  {Title: "留抵税额风险", Chart: chartContract(chartKindLine, "期末留抵税额趋势", "期末留抵税额"), Tables: [][]string{{"季度末", "期末留抵税额", "变化", "关注"}}},
	"5.9":  {Title: "免税、简易计税及差额征税", Tables: [][]string{{"项目", "申报情况", "结论"}}},
	"6.1":  {Title: citContributionTitle, Chart: chartContract(chartKindDualAxis, citContributionChartTitle, citContributionTaxLabel, "贡献率"), Tables: [][]string{citContributionHeaders}},
	"6.2":  {Title: "财务报表收入与企业所得税收入", Tables: [][]string{{"季度", "财务报表营业收入", "企税申报营业收入", "差异额", "风险"}}},
	"6.3":  {Title: "财务报表成本与购进发票", Tables: [][]string{financialCostComparisonHeaders}},
	"6.4":  {Title: "利润总额与应纳税所得额", Tables: [][]string{quarterlyCITProfitHeaders}},
	"6.5":  {Title: "纳税调整项目", Tables: [][]string{{"调整项目", "{period}申报金额", "关注"}}},
	"6.6":  {Title: "研发加计扣除申报情况", Chart: optionalChartContract(chartKindGroupedColumn, "研发加计扣除申报趋势", "研发加计扣除额"), Tables: [][]string{{"季度/年度", "研发加计扣除额", "占营业收入比例", "趋势"}}},
	"6.7":  {Title: "企业所得税优惠", Tables: [][]string{{"优惠项目", "申报情况", "关注"}}},
	"6.8":  {Title: "税前扣除凭证关注", Tables: [][]string{{"事项", "发票/申报线索", "关注"}}},
	"7.1":  {Title: "印花税", Tables: [][]string{{"项目", "{period}估算/申报", "覆盖率/结论"}}},
	"7.2":  {Title: "房产税", Tables: [][]string{{"季度", "房产税实缴", "变化", "关注"}}},
	"7.3":  {Title: "城镇土地使用税", Tables: [][]string{{"季度", "实缴税额", "变化", "结论"}}},
	"7.4":  {Title: "城市维护建设税及附加", Chart: chartContract(chartKindDualAxis, "城市维护建设税及附加趋势", "附加税费申报额", "附加与主税计税基础比例"), Tables: [][]string{{"所属期", "附加税费申报合计", "增值税/消费税计税基础", "附加与计税基础比例", "附加实缴合计"}}},
	"7.5":  {Title: "工会经费", Tables: [][]string{{"季度", "申报金额", "实缴金额", "申报额较上季变化"}}},
	"7.6":  {Title: "其他适用税种", Tables: [][]string{{"税种", "企业适用情况", "结论"}}},
	"8.1":  {Title: "收入、成本及利润趋势", Chart: chartContract(chartKindCombo, "收入、成本及利润趋势", "营业收入", "营业成本", "利润总额"), Tables: [][]string{{"期间", "营业收入", "营业成本", "利润总额", "净利润"}}},
	"8.2":  {Title: "毛利率、利润率及费用率", Chart: chartContract(chartKindMultiLine, "毛利率、利润率及费用率趋势", "毛利率", "营业利润率", "期间费用率"), Tables: [][]string{{"期间", "毛利率", "营业利润率", "期间费用率", "趋势"}}},
	"8.3":  {Title: "资产负债率", Chart: chartContract(chartKindLine, "资产负债率趋势", "资产负债率"), Tables: [][]string{{"期间", "资产负债率", "变化", "风险"}}},
	"8.4":  {Title: "应收账款、存货及合同负债", Chart: optionalChartContract(chartKindMultiLine, "应收账款、存货及合同负债趋势", "应收账款", "存货", "合同负债"), Tables: [][]string{{"期间", "应收账款", "存货", "合同负债", "关注"}}},
	"8.5":  {Title: "经营活动现金流", Chart: chartContract(chartKindMultiLine, "经营活动现金流趋势", "经营活动现金流量净额", "净利润"), Tables: [][]string{{"期间", "经营活动现金流量净额", "净利润", "趋势"}}},
	"8.6":  {Title: "财务指标综合提示"},
	"9.1":  {Title: "业务结构分析", Chart: dynamicChartContract(chartKindPie, "主要业务结构", true)},
	"9.2":  {TitleMatcher: businessTitleMatcher, Tables: [][]string{{"指标", "本期数据"}}},
	"9.3":  {TitleMatcher: businessTitleMatcher, Tables: [][]string{{"指标", "本期数据"}}},
	"9.4":  {TitleMatcher: businessTitleMatcher, Tables: [][]string{{"指标", "本期数据"}}},
	"9.5":  {Title: "行业经营特征综合分析", Chart: optionalChartContract(chartKindRadar, "企业与行业经营指标对比", "企业值", "行业基准")},
	"10.1": {Title: "优先处理事项", Tables: [][]string{{"序号", "事项", "等级", "建议完成时限", "建议处理"}}},
	"10.2": {Title: "重点核实事项", Tables: [][]string{{"事项", "等级", "核实重点"}}},
	"10.3": {Title: "持续监测事项"},
	"10.4": {Title: "整改跟踪表", Tables: [][]string{{"风险事项", "责任人", "计划完成日期", "核实结果", "处理措施", "状态"}}},
}

func chartContract(kind, title string, series ...string) *wordChartContract {
	return &wordChartContract{Kind: kind, Title: title, Series: series, Required: true}
}

func optionalChartContract(kind, title string, series ...string) *wordChartContract {
	return &wordChartContract{Kind: kind, Title: title, Series: series}
}

func dynamicChartContract(kind, title string, required bool) *wordChartContract {
	return &wordChartContract{Kind: kind, Title: title, Required: required, DynamicSeries: true}
}

func dynamicMultiChartContract(kind, title string, maximumSeries int) *wordChartContract {
	return &wordChartContract{Kind: kind, Title: title, Required: true, DynamicSeries: true, MaximumSeries: maximumSeries, AllowedMultiple: true}
}

func suffixTitle(suffix string) func(string) bool {
	return func(value string) bool {
		return strings.HasSuffix(strings.TrimSpace(value), suffix) && strings.TrimSpace(value) != suffix
	}
}

func counterpartyTitleMatcher(subject string) func(string) bool {
	return func(value string) bool {
		value = strings.TrimSpace(value)
		return regexp.MustCompile(`^(主要` + regexp.QuoteMeta(subject) + `（\d+家）|前五大` + regexp.QuoteMeta(subject) + `分析|前十大` + regexp.QuoteMeta(subject) + `分析)$`).MatchString(value)
	}
}

func businessTitleMatcher(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.HasPrefix(value, "主要业务")
}

// validateWordReportContract verifies titles, chart kinds/series and table
// headers for the finalized model. Parameter model is shared by DOCX and PDF.
// Return value contains every detected contract deviation in deterministic
// section order.
func validateWordReportContract(model Model) error {
	errorsFound := make([]string, 0)
	for _, section := range model.Sections {
		if section.Level == 1 {
			if expected := wordChapterTitles[section.ID]; expected != "" && section.BaseTitle != expected {
				errorsFound = append(errorsFound, fmt.Sprintf("%s标题=%q，应为%q", section.ID, section.BaseTitle, expected))
			}
			continue
		}
		contract, ok := wordSectionContracts[section.ID]
		if !ok {
			errorsFound = append(errorsFound, section.ID+"未登记WordReportContract")
			continue
		}
		if contract.TitleMatcher != nil {
			if !contract.TitleMatcher(section.BaseTitle) {
				errorsFound = append(errorsFound, fmt.Sprintf("%s动态标题不符合Word：%q", section.ID, section.BaseTitle))
			}
		} else if section.BaseTitle != contract.Title {
			errorsFound = append(errorsFound, fmt.Sprintf("%s标题=%q，应为%q", section.ID, section.BaseTitle, contract.Title))
		}
		// V4 supersedes mandatory empty tables, while retaining field contracts
		// whenever the required source values are actually available.
		if section.Key != "risk.overall-evaluation" && (section.Narrative.Mode == NarrativeDataGap || section.Narrative.Mode == NarrativeNotApplicable || section.Narrative.Mode == NarrativeNoIssue) {
			if len(section.Blocks) != 1 || section.Blocks[0].Kind != BlockParagraph || len(section.Blocks[0].Paragraphs) != 1 {
				errorsFound = append(errorsFound, section.ID+"固定状态只能输出一条说明")
			}
			continue
		}
		chartCount := 0
		tableHeaders := make([][]string, 0)
		for _, block := range section.Blocks {
			if block.Chart != nil {
				chartCount++
				chart := *block.Chart
				if section.SampleOnly {
					chart.Title = strings.TrimSuffix(chart.Title, "（已取得样本）")
				}
				validateContractChart(section.ID, chart, contract.Chart, &errorsFound)
			}
			if block.Kind == BlockTable {
				tableHeaders = append(tableHeaders, block.Headers)
			}
		}
		if contract.Chart == nil && chartCount > 0 {
			errorsFound = append(errorsFound, fmt.Sprintf("%s不得生成图表", section.ID))
		}
		if contract.Chart != nil && contract.Chart.Required && chartCount == 0 {
			errorsFound = append(errorsFound, fmt.Sprintf("%s缺少%s", section.ID, contract.Chart.Kind))
		}
		if contract.Chart != nil && !contract.Chart.AllowedMultiple && chartCount > 1 {
			errorsFound = append(errorsFound, fmt.Sprintf("%s图表数量=%d，应最多1张", section.ID, chartCount))
		}
		if len(tableHeaders) != len(contract.Tables) && !validTaxDetailTables(section.ID, tableHeaders, len(contract.Tables)) && !validEmptyRemediationSection(section, model, tableHeaders) {
			errorsFound = append(errorsFound, fmt.Sprintf("%s表格数量=%d，应为%d", section.ID, len(tableHeaders), len(contract.Tables)))
		}
		for index := 0; index < len(tableHeaders) && index < len(contract.Tables); index++ {
			expected := resolveContractHeadersForValidation(contract.Tables[index], section, model)
			if !equalStrings(tableHeaders[index], expected) {
				errorsFound = append(errorsFound, fmt.Sprintf("%s第%d张表表头=%v，应为%v", section.ID, index+1, tableHeaders[index], expected))
			}
		}
	}
	if len(errorsFound) == 0 {
		return nil
	}
	sort.Strings(errorsFound)
	return fmt.Errorf("Word报告契约校验失败：%s", strings.Join(errorsFound, "；"))
}

// The product's existing zero-finding state is a concise paragraph. Preserve
// that specific state without allowing arbitrary missing Word tables.
func validEmptyRemediationSection(section Section, model Model, headers [][]string) bool {
	if section.ID != "10.1" || len(model.Findings) > 0 || len(headers) > 0 || len(section.Blocks) != 1 {
		return false
	}
	block := section.Blocks[0]
	return block.Kind == BlockParagraph && len(block.Paragraphs) == 1 && block.Paragraphs[0] == "本期未识别到需要优先处理的明确风险事项。"
}

// The approved field-completion plan adds explicit declaration/source tables
// after the original Word summary. Only these exact extra schemas are allowed.
func validTaxDetailTables(sectionID string, headers [][]string, base int) bool {
	if len(headers) != base+1 {
		return false
	}
	if sectionID == "6.2" || sectionID == "6.4" {
		return equalStrings(headers[base], annualComparisonHeaders)
	}
	if sectionID == "5.5" {
		return equalStrings(headers[base], []string{"年度", "税务报送财务报表营业收入", "增值税申报销售额", "差异", "关注"})
	}
	if sectionID == "4.2" && base == 1 {
		return equalStrings(headers[0], headers[1])
	}
	return false
}

func validateContractChart(sectionID string, chart Chart, contract *wordChartContract, errorsFound *[]string) {
	if contract == nil {
		return
	}
	if chart.Kind != contract.Kind {
		*errorsFound = append(*errorsFound, fmt.Sprintf("%s图表%s类型=%s，应为%s", sectionID, chart.ID, chart.Kind, contract.Kind))
	}
	titleMatches := chart.Title == contract.Title || contract.AllowedMultiple && strings.HasSuffix(chart.Title, contract.Title)
	if contract.TitleMatcher != nil {
		titleMatches = contract.TitleMatcher(chart.Title)
	}
	if !titleMatches {
		*errorsFound = append(*errorsFound, fmt.Sprintf("%s图表标题=%q，应为%q", sectionID, chart.Title, contract.Title))
	}
	if len(chart.Series) == 0 {
		return
	}
	if contract.DynamicSeries {
		if contract.MaximumSeries > 0 && len(chart.Series) > contract.MaximumSeries {
			*errorsFound = append(*errorsFound, fmt.Sprintf("%s系列数=%d，最多%d", sectionID, len(chart.Series), contract.MaximumSeries))
		}
		return
	}
	names := make([]string, len(chart.Series))
	for index := range chart.Series {
		names[index] = chart.Series[index].Name
	}
	if !equalStrings(names, contract.Series) {
		*errorsFound = append(*errorsFound, fmt.Sprintf("%s图表系列=%v，应为%v", sectionID, names, contract.Series))
	}
	if sectionID == "3.15" || sectionID == "6.1" {
		for _, series := range chart.Series {
			if series.RenderAs != chartRenderLine {
				*errorsFound = append(*errorsFound, fmt.Sprintf("%s系列%s必须为折线", sectionID, series.Name))
			}
		}
	}
	if sectionID == "8.1" && len(chart.Series) == 3 {
		if chart.Series[0].RenderAs != chartRenderBar || chart.Series[1].RenderAs != chartRenderBar || chart.Series[2].RenderAs != chartRenderLine {
			*errorsFound = append(*errorsFound, "8.1必须为营业收入、营业成本柱状和利润总额折线")
		}
	}
}

func resolveContractHeadersForValidation(spec []string, section Section, model Model) []string {
	if section.Key == "vat.financial-comparison" && len(section.Blocks) > 0 && section.Blocks[0].Title == "年度财务报表收入与增值税销售额" {
		return []string{"年度", "税务报送财务报表营业收入", "增值税申报销售额", "差异", "关注"}
	}
	if len(spec) == 3 && spec[1] == "{dynamic-periods}" {
		return append(append([]string{"风险事项"}, completeTrendPeriods(model.StartTime, model.EndTime, true)...), "趋势")
	}
	period := analysisPeriodText(model.StartTime, model.EndTime)
	result := make([]string, len(spec))
	for index, header := range spec {
		result[index] = strings.ReplaceAll(header, "{period}", period)
	}
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
