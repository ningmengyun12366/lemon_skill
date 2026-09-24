package report

// figmaChartSpec is one required mapping from a Word section to the component
// whose layout is authoritative in Figma. It is used by regression tests and
// the synthetic visual fixture, never to force a real section to appear.
type figmaChartSpec struct {
	SectionID   string
	ChartID     string
	Kind        string
	Title       string
	FixtureOnly bool
}

var figmaChartCatalog = []figmaChartSpec{
	{SectionID: "1.2", ChartID: "risk-distribution", Kind: chartKindDonut, Title: "风险等级分布"},
	{SectionID: "1.3", ChartID: "module-risk", Kind: chartKindStackedBar, Title: "各模块风险分布"},
	{SectionID: "2.3", ChartID: "qixin-equity-network", Kind: chartKindNetwork, Title: "股权及关联关系图"},
	{SectionID: "2.4", ChartID: "credit-trend", Kind: chartKindLine, Title: "纳税信用等级变化"},
	{SectionID: "2.10", ChartID: "enterprise-business-structure", Kind: chartKindPie, Title: "主要业务结构"},
	{SectionID: "3.2", ChartID: "invoice-trend", Kind: chartKindMultiLine, Title: "进销项不含税金额趋势"},
	{SectionID: "3.3", ChartID: "invoice-tax-trend", Kind: chartKindMultiLine, Title: "进销项税额趋势"},
	{SectionID: "3.6", ChartID: "top-customers", Kind: chartKindHorizontalBar, Title: "主要客户排名"},
	{SectionID: "3.7", ChartID: "top-suppliers", Kind: chartKindHorizontalBar, Title: "主要供应商排名"},
	{SectionID: "3.8", ChartID: "customer-concentration", Kind: chartKindGroupedColumn, Title: "客户集中度"},
	{SectionID: "3.9", ChartID: "supplier-concentration", Kind: chartKindGroupedColumn, Title: "供应商集中度"},
	{SectionID: "3.15", ChartID: "red-output-trend", Kind: chartKindDualAxis, Title: "红字发票金额及红字率趋势"},
	{SectionID: "3.17", ChartID: "sensitive-services", Kind: chartKindGroupedColumn, Title: "敏感服务类发票分类"},
	{SectionID: "4.3", ChartID: "major-tax-trend", Kind: chartKindMultiLine, Title: "主要税种税额趋势"},
	{SectionID: "4.4", ChartID: "total-payment-trend", Kind: chartKindLine, Title: "税费缴纳总额趋势"},
	{SectionID: "4.5", ChartID: "tax-composition", Kind: chartKindDonut, Title: "税费构成"},
	{SectionID: "4.6", ChartID: "monthly-tax-burden", Kind: chartKindMultiLine, Title: "全税种税负率月趋势"},
	{SectionID: "5.2", ChartID: "vat-burden-trend", Kind: chartKindLine, Title: "增值税税负率趋势"},
	{SectionID: "5.4", ChartID: "vat-invoice-declaration-difference", Kind: chartKindMultiLine, Title: "销项发票与申报销售额"},
	{SectionID: "5.8", ChartID: "vat-end-credit", Kind: chartKindLine, Title: "期末留抵税额趋势"},
	{SectionID: "6.1", ChartID: "cit-contribution", Kind: chartKindDualAxis, Title: citContributionChartTitle},
	{SectionID: "6.6", ChartID: "cit-research-expense", Kind: chartKindGroupedColumn, Title: "研发加计扣除申报趋势"},
	{SectionID: "7.4", ChartID: "surtax-trend", Kind: chartKindDualAxis, Title: "城市维护建设税及附加趋势"},
	{SectionID: "8.1", ChartID: "financial-income-cost-profit-trend", Kind: chartKindCombo, Title: "收入、成本及利润趋势"},
	{SectionID: "8.2", ChartID: "financial-margin-expense-trend", Kind: chartKindMultiLine, Title: "毛利率、利润率及费用率趋势"},
	{SectionID: "8.3", ChartID: "debt-asset-ratio", Kind: chartKindLine, Title: "资产负债率趋势"},
	{SectionID: "8.4", ChartID: "working-capital-trend", Kind: chartKindMultiLine, Title: "应收账款、存货及合同负债趋势"},
	{SectionID: "8.5", ChartID: "operating-cash-flow", Kind: chartKindMultiLine, Title: "经营活动现金流趋势"},
	{SectionID: "9.1", ChartID: "business-structure", Kind: chartKindPie, Title: "主要业务结构"},
	{SectionID: "9.5", ChartID: "industry-radar", Kind: chartKindRadar, Title: "企业与行业经营指标对比"},
}
