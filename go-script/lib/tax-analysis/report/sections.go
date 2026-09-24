package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	taxanalysis "invoice-assistant-skill/go-script/lib/tax-analysis"
)

type reportContext struct {
	invoiceCoverage  map[string]InvoiceCoverage
	task             taxanalysis.TaskResponse
	modules          map[string]ModuleSnapshot
	evaluations      []RuleEvaluation
	findings         []Finding
	taxpayer         taxpayerKind
	credits          []creditGrade
	issued           []invoiceGroup
	received         []invoiceGroup
	vat              []vatFact
	filings          []filingFact
	declarations     []filingFact
	payments         []paymentFact
	cit              []citFact
	financial        []financialFact
	taxFields        []TaxFieldFact
	taxLines         []taxDeclarationLine
	calculations     []TaxCalculation
	customers        *counterpartyAnalysis
	suppliers        *counterpartyAnalysis
	categories       []businessCategory
	redIssuedAmount  float64
	blueIssuedAmount float64
	yearEndReversal  yearEndReversalAnalysis
	warnings         []string
	generatedAt      time.Time
}

type sectionDefinition struct {
	key       string
	sourceID  string
	priority  int
	title     func(*reportContext) string
	include   func(*reportContext) (bool, OmissionReason)
	build     func(*reportContext) []Block
	ruleIDs   []string
	moduleIDs []string
}

type chapterDefinition struct {
	key        string
	sourceID   string
	title      string
	overview   func(*reportContext) []string
	standalone bool
	children   []sectionDefinition
}

func newReportContext(task taxanalysis.TaskResponse, modules map[string]ModuleSnapshot, enterpriseContext map[string]any, generatedAt time.Time) *reportContext {
	_ = enterpriseContext
	taxpayer, taxpayerWarning := taxpayerClassificationForTask(task, modules["taxpayer_profile"])
	declarations := declarationFacts(modules, task.StartTime, task.EndTime)
	ctx := &reportContext{
		task:         task,
		modules:      modules,
		generatedAt:  generatedAt,
		taxpayer:     taxpayer,
		credits:      creditGrades(modules["tax_credit"]),
		issued:       invoiceGroups(modules["issued_invoices"], true),
		received:     invoiceGroups(modules["received_invoices"], false),
		vat:          vatFacts(modules["vat_returns"]),
		filings:      declarations,
		declarations: declarations,
		payments:     paymentFacts(modules["payments"], task.StartTime, task.EndTime),
		cit:          citFacts(modules["cit_quarterly_returns"]),
		financial:    financialFacts(modules["income_statements"], modules["balance_sheets"]),
		customers:    analyzeCounterparties(modules["issued_invoices"], true),
		suppliers:    analyzeCounterparties(modules["received_invoices"], false),
		categories:   businessCategories(modules["issued_invoices"]),
	}
	ctx.taxFields = taxFieldFacts(modules)
	ctx.invoiceCoverage = buildInvoiceCoverage(ctx)
	for index := range ctx.taxFields {
		if ctx.taxFields[index].USCC == "" {
			ctx.taxFields[index].USCC = task.USCC
		}
	}
	ctx.taxLines = taxDeclarationLines(ctx.taxFields)
	ctx.declarations = declarationsWithDetailAmounts(ctx.declarations, ctx.taxLines, ctx.vat, task.StartTime, task.EndTime)
	ctx.filings = ctx.declarations
	if taxpayerWarning != "" {
		ctx.warnings = append(ctx.warnings, taxpayerWarning)
	}
	ctx.yearEndReversal = yearEndReversalAnalysis{Observations: readInvoiceSummary(ctx.modules["issued_invoices"]).YearEndObservations}
	for _, fact := range ctx.issued {
		if !fact.HasAmount {
			continue
		}
		if fact.Red || fact.Amount < 0 {
			ctx.redIssuedAmount += abs(fact.Amount)
		} else {
			ctx.blueIssuedAmount += fact.Amount
		}
	}
	if ctx.taxpayer == taxpayerUnknown {
		ctx.warnings = append(ctx.warnings, unknownTaxpayerQualificationWarning)
	}
	if module, ok := modules[degradableModuleCode]; ok && !module.Complete {
		ctx.warnings = append(ctx.warnings, degradableModuleWarning)
	}
	for moduleCode := range emptyTaxSourceModuleCodes {
		if module, ok := modules[moduleCode]; ok && !module.Complete {
			ctx.warnings = append(ctx.warnings, emptyTaxSourceWarning)
			break
		}
	}
	for _, value := range arrayAt(modules[qixinModuleCode].Raw, qixinWarningsField) {
		if warning := strings.TrimSpace(scalarText(value)); warning != "" {
			ctx.warnings = append(ctx.warnings, warning)
		}
	}
	return ctx
}

func buildSectionTree(ctx *reportContext) ([]Section, map[string]OmissionReason) {
	omitted := map[string]OmissionReason{}
	sections := make([]Section, 0, 40)
	for _, chapter := range reportChapters() {
		if chapter.sourceID == "10" && len(ctx.findings) == 0 {
			omitted[chapter.sourceID] = OmittedNoRisk
			for _, child := range chapter.children {
				omitted[child.sourceID] = OmittedNoRisk
			}
			continue
		}
		children := append([]sectionDefinition(nil), chapter.children...)
		sort.SliceStable(children, func(i, j int) bool { return children[i].priority < children[j].priority })
		included := make([]sectionDefinition, 0, len(children))
		for _, definition := range children {
			include, reason := true, OmissionReason("")
			if definition.include != nil {
				include, reason = definition.include(ctx)
			}
			if include {
				included = append(included, definition)
			} else {
				omitted[definition.sourceID] = reason
			}
		}
		if len(included) == 0 && !chapter.standalone {
			omitted[chapter.sourceID] = OmittedNotApplicable
			continue
		}
		sections = append(sections, Section{
			Key: chapter.key, ID: chapter.sourceID, DisplayNumber: chapter.sourceID, Level: 1,
			BaseTitle: chapter.title, Title: chineseChapterTitle(chapter.sourceID, chapter.title),
			Bookmark: bookmark(chapter.key), Blocks: chapterOverviewBlocks(chapter.overview(ctx)),
		})
		for index, definition := range included {
			displayNumber := fmt.Sprintf("%s.%d", chapter.sourceID, index+1)
			baseTitle := definition.title(ctx)
			sections = append(sections, Section{
				Key: definition.key, ID: definition.sourceID, DisplayNumber: displayNumber, Level: 2,
				BaseTitle: baseTitle, Title: displayNumber + " " + baseTitle,
				Bookmark: bookmark(definition.key), ModuleIDs: append([]string(nil), definition.moduleIDs...), RuleIDs: append([]string(nil), definition.ruleIDs...), Blocks: comparableTrendBlocks(definition.sourceID, definition.build(ctx)),
			})
		}
	}
	includedSources := make(map[string]bool, len(sections))
	for _, section := range sections {
		includedSources[section.ID] = true
	}
	for _, sourceID := range potentialSectionIDs() {
		if includedSources[sourceID] {
			continue
		}
		if _, recorded := omitted[sourceID]; !recorded {
			omitted[sourceID] = OmittedDataInsufficient
		}
	}
	return sections, omitted
}

func potentialSectionIDs() []string {
	return []string{
		"1", "1.1", "1.2", "1.3", "1.4", "1.5",
		"2", "2.1", "2.2", "2.3", "2.4", "2.6", "2.7", "2.8", "2.9", "2.10",
		"3", "3.1", "3.2", "3.3", "3.4", "3.5", "3.6", "3.7", "3.8", "3.9", "3.10", "3.11", "3.12", "3.13", "3.14", "3.15", "3.16", "3.17", "3.18", "3.19",
		"4", "4.1", "4.2", "4.3", "4.4", "4.5", "4.6", "4.7",
		"5", "5.1", "5.2", "5.3", "5.4", "5.5", "5.6", "5.7", "5.8", "5.9",
		"6", "6.1", "6.2", "6.3", "6.4", "6.5", "6.6", "6.7", "6.8",
		"7", "7.1", "7.2", "7.3", "7.4", "7.5", "7.6",
		"8", "8.1", "8.2", "8.3", "8.4", "8.5", "8.6",
		"9", "9.1", "9.2", "9.3", "9.4", "9.5",
		"10", "10.1", "10.2", "10.3", "10.4", "11",
	}
}

// chapterOverviewBlocks omits empty chapter prose. Parameter paragraphs is the
// Word-permitted overview text. Return value contains one paragraph block only
// when visible content exists.
func chapterOverviewBlocks(paragraphs []string) []Block {
	visible := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if value := strings.TrimSpace(paragraph); value != "" {
			visible = append(visible, value)
		}
	}
	if len(visible) == 0 {
		return nil
	}
	return []Block{{Kind: BlockParagraph, Paragraphs: visible}}
}

func reportChapters() []chapterDefinition {
	allRuleIDs := []string{"TAX-FILING-001", "TAX-PAYMENT-001", "TAX-ARREARS-001", "TAX-CREDIT-001", "INVOICE-OUTPUT-001", "INVOICE-INPUT-001", qixinRuleBusinessStatus, qixinRuleTaxViolation, qixinRuleTaxPenalty, qixinRuleOperatingAbnormal, qixinRuleAdministrativePenalty, qixinRuleSeriousIllegal, qixinRuleOwnExecution, customerCreditRule, supplierCreditRule, customerConcentrationRule, supplierConcentrationRule, invoiceOutputReconciliationRule, invoiceInputReconciliationRule, vatContinuousBurdenRule}
	always := func(*reportContext) (bool, OmissionReason) { return true, "" }
	withRisks := func(ctx *reportContext) (bool, OmissionReason) {
		if len(ctx.findings) == 0 {
			return false, OmittedNoRisk
		}
		return true, ""
	}
	return []chapterDefinition{
		{
			key: "chapter.risk-overview", sourceID: "1", title: "风险概览",
			overview: func(*reportContext) []string {
				return nil
			},
			children: []sectionDefinition{
				{key: "risk.overall-evaluation", sourceID: "1.1", priority: 10, title: staticTitle("综合风险评价"), include: always, build: buildOverallEvaluation, ruleIDs: allRuleIDs},
				{key: "risk.level-distribution", sourceID: "1.2", priority: 20, title: staticTitle("风险等级分布"), include: withRisks, build: buildRiskDistribution, ruleIDs: allRuleIDs},
				{key: "risk.module-distribution", sourceID: "1.3", priority: 30, title: staticTitle("各模块风险分布"), include: withRisks, build: buildModuleRiskDistribution, ruleIDs: allRuleIDs},
				{key: "risk.items", sourceID: "1.4", priority: 40, title: staticTitle("重点风险事项"), include: withRisks, build: buildRiskTables, ruleIDs: allRuleIDs},
				{key: "risk.trend", sourceID: "1.5", priority: 50, title: staticTitle("风险变化趋势"), include: includeRiskTrend, build: buildRiskTrend, ruleIDs: allRuleIDs},
			},
		},
		{
			key: "chapter.enterprise", sourceID: "2", title: "企业综合信息",
			overview: func(ctx *reportContext) []string {
				return []string{fmt.Sprintf("工商及税务信息来源于企业授权数据及公开信息；公开信息截至报告生成日%s，可能存在更新时差。", ctx.generatedAt.Format("2006-01-02"))}
			},
			children: []sectionDefinition{
				{key: "enterprise.profile", sourceID: "2.1", priority: 10, title: staticTitle("工商及税务基本信息"), include: always, build: buildEnterpriseProfile, moduleIDs: []string{"taxpayer_profile", "tax_credit", qixinModuleCode}, ruleIDs: []string{qixinRuleBusinessStatus}},
				{key: "enterprise.shareholders", sourceID: "2.2", priority: 20, title: staticTitle("股东及出资信息"), include: always, build: buildQixinOrganization, moduleIDs: []string{qixinModuleCode}},
				{key: "enterprise.equity-relations", sourceID: "2.3", priority: 30, title: staticTitle("股权穿透及疑似关联方"), include: always, build: buildQixinRelations, moduleIDs: []string{qixinModuleCode}},
				{key: "enterprise.credit-trend", sourceID: "2.4", priority: 40, title: staticTitle("近三年纳税信用等级"), include: includeCreditTrend, build: buildCreditTrend, moduleIDs: []string{"tax_credit"}},
				{key: "enterprise.tax-violations", sourceID: "2.6", priority: 50, title: staticTitle("税务处罚及税收违法信息"), include: always, build: buildQixinTaxViolations, moduleIDs: []string{qixinModuleCode, "filing_obligations"}, ruleIDs: []string{qixinRuleTaxViolation, qixinRuleTaxPenalty}},
				{key: "enterprise.compliance", sourceID: "2.7", priority: 60, title: staticTitle("行政处罚及经营异常"), include: always, build: buildQixinCompliance, moduleIDs: []string{qixinModuleCode}, ruleIDs: []string{qixinRuleBusinessStatus, qixinRuleOperatingAbnormal, qixinRuleAdministrativePenalty, qixinRuleSeriousIllegal}},
				{key: "enterprise.changes", sourceID: "2.8", priority: 70, title: staticTitle("企业变更信息"), include: always, build: buildQixinChanges, moduleIDs: []string{qixinModuleCode}},
				{key: "enterprise.judicial", sourceID: "2.9", priority: 80, title: staticTitle("司法执行及诉讼风险"), include: always, build: buildQixinJudicial, moduleIDs: []string{qixinModuleCode, "issued_invoices", "received_invoices"}},
				{key: "enterprise.industry", sourceID: "2.10", priority: 90, title: staticTitle("行业及经营模式识别"), include: always, build: buildQixinIndustryWithChart, moduleIDs: []string{qixinModuleCode, "issued_invoices"}},
			},
		},
		{
			key: "chapter.invoice", sourceID: "3", title: "发票及购销链风险分析",
			overview: func(*reportContext) []string {
				return []string{"发票分析采用本报告统一口径；公开信用信息仅用于提示已核验主体的公开记录，不替代交易真实性判断。"}
			},
			children: []sectionDefinition{
				{key: "invoice.overall", sourceID: "3.1", priority: 10, title: invoiceOverallTitle, include: always, build: buildInvoiceOverall, moduleIDs: []string{"issued_invoices", "received_invoices"}},
				{key: "invoice.amount-trend", sourceID: "3.2", priority: 20, title: staticTitle("进销项金额趋势"), include: always, build: buildInvoiceAmountTrendWithFigmaChart, moduleIDs: []string{"issued_invoices", "received_invoices"}},
				{key: "invoice.tax-comparison", sourceID: "3.3", priority: 30, title: staticTitle("进销项税额对比"), include: always, build: buildInvoiceTaxComparisonWithFigmaChart, moduleIDs: []string{"issued_invoices", "deduction_confirmations", "vat_returns"}, ruleIDs: []string{invoiceOutputReconciliationRule, invoiceInputReconciliationRule}},
				{key: "invoice.category-match", sourceID: "3.4", priority: 40, title: staticTitle("购销商品及服务匹配"), include: always, build: buildInvoiceCategoryMatchWithChart, moduleIDs: []string{"issued_invoices", "received_invoices"}},
				{key: "invoice.business-match", sourceID: "3.5", priority: 50, title: staticTitle("开票内容与经营业务匹配"), include: always, build: buildInvoiceBusinessMatch, moduleIDs: []string{"issued_invoices", qixinModuleCode}},
				{key: "invoice.customers", sourceID: "3.6", priority: 60, title: customerTitle, include: always, build: buildCustomers, moduleIDs: []string{"issued_invoices"}},
				{key: "invoice.suppliers", sourceID: "3.7", priority: 70, title: supplierTitle, include: always, build: buildSuppliers, moduleIDs: []string{"received_invoices"}},
				{key: "invoice.customer-concentration", sourceID: "3.8", priority: 80, title: staticTitle("客户集中度分析"), include: always, build: buildCustomerConcentrationWithChart, moduleIDs: []string{"issued_invoices"}, ruleIDs: []string{customerConcentrationRule}},
				{key: "invoice.supplier-concentration", sourceID: "3.9", priority: 90, title: staticTitle("供应商集中度分析"), include: always, build: buildSupplierConcentrationWithChart, moduleIDs: []string{"received_invoices"}, ruleIDs: []string{supplierConcentrationRule}},
				{key: "invoice.customer-credit", sourceID: "3.10", priority: 100, title: staticTitle("客户公开信用风险"), include: always, build: buildQixinCustomerCredit, moduleIDs: []string{"issued_invoices", qixinModuleCode}, ruleIDs: []string{customerCreditRule}},
				{key: "invoice.supplier-credit", sourceID: "3.11", priority: 110, title: staticTitle("供应商公开信用风险"), include: always, build: buildQixinSupplierCredit, moduleIDs: []string{"received_invoices", qixinModuleCode}, ruleIDs: []string{supplierCreditRule}},
				{key: "invoice.association-clues", sourceID: "3.12", priority: 115, title: staticTitle("疑似关联交易线索"), include: always, build: buildAssociationClues, moduleIDs: []string{"issued_invoices", "received_invoices", qixinModuleCode}},
				{key: "invoice.field-quality", sourceID: "3.13", priority: 120, title: staticTitle("发票抬头、税号、品名、编码及税率"), include: always, build: buildInvoiceFieldQuality, moduleIDs: []string{"issued_invoices", "received_invoices"}},
				{key: "invoice.specific-fields", sourceID: "3.14", priority: 130, title: staticTitle("特定业务发票字段及备注"), include: includeSpecificInvoiceFields, build: buildSpecificInvoiceFields, moduleIDs: []string{"issued_invoices", "received_invoices"}},
				{key: "invoice.red-output", sourceID: "3.15", priority: 140, title: staticTitle("红字发票总体分析"), include: includeRedOutput, build: buildRedOutputWithChart, moduleIDs: []string{"issued_invoices"}},
				{key: "invoice.year-end-reversal", sourceID: yearEndReversalSectionSourceID, priority: yearEndReversalSectionPriority, title: staticTitle("年末开票及次年红冲分析"), include: always, build: buildYearEndReversal, moduleIDs: []string{"issued_invoices"}, ruleIDs: []string{yearEndReversalRuleID}},
				{key: "invoice.sensitive-services", sourceID: "3.17", priority: sensitiveServicesSectionPriority, title: staticTitle("敏感服务类发票分析"), include: always, build: buildSensitiveServicesWithChart, moduleIDs: []string{"issued_invoices", "received_invoices"}},
				{key: "invoice.output-clues", sourceID: "3.18", priority: outputCluesSectionPriority, title: staticTitle("疑似虚开销项发票线索"), include: always, build: func(ctx *reportContext) []Block { return buildSignalSection(ctx, "INVOICE-OUTPUT-001") }, moduleIDs: []string{"issued_invoices", "received_invoices", qixinModuleCode}, ruleIDs: []string{"INVOICE-OUTPUT-001"}},
				{key: "invoice.input-clues", sourceID: "3.19", priority: inputCluesSectionPriority, title: staticTitle("疑似取得虚假进项发票线索"), include: always, build: func(ctx *reportContext) []Block { return buildSignalSection(ctx, "INVOICE-INPUT-001") }, moduleIDs: []string{"received_invoices", "issued_invoices", "deduction_confirmations", qixinModuleCode}, ruleIDs: []string{"INVOICE-INPUT-001"}},
			},
		},
		{
			key: "chapter.tax-payment", sourceID: "4", title: "税费申报缴纳全景",
			overview: func(*reportContext) []string {
				return []string{"申报和缴款状态以税局返回记录为准；缴款记录优先按税款所属期展示，未返回所属期时按缴款日期标注。"}
			},
			children: []sectionDefinition{
				{key: "tax-payment.filing-status", sourceID: "4.1", priority: 10, title: staticTitle("税费种及申报周期"), include: always, build: buildFilingCycleMatrix, moduleIDs: []string{"filing_obligations"}, ruleIDs: []string{"TAX-FILING-001"}},
				{key: "tax-payment.period-detail", sourceID: "4.2", priority: 20, title: paymentPeriodTitle, include: always, build: buildPaymentDetail, moduleIDs: []string{"filing_obligations", "vat_returns", "cit_quarterly_returns", "cit_annual_returns", "stamp_tax", "property_tax", "urban_land_use_tax", "payments"}},
				{key: "tax-payment.major-trend", sourceID: "4.3", priority: 30, title: staticTitle("主要税种税额趋势"), include: always, build: buildMajorTaxTrendWithChart, moduleIDs: []string{"payments"}},
				{key: "tax-payment.total-trend", sourceID: "4.4", priority: 40, title: staticTitle("税费缴纳总额趋势"), include: always, build: buildTotalPaymentTrendWithChart, moduleIDs: []string{"payments"}},
				{key: "tax-payment.composition", sourceID: "4.5", priority: 50, title: staticTitle("税费构成"), include: always, build: buildPaymentCompositionWithChart, moduleIDs: []string{"payments"}},
				{key: taxBurdenSectionKey, sourceID: "4.6", priority: 60, title: taxBurdenTitle, include: always, build: buildMonthlyTaxBurdenWithChart, moduleIDs: []string{"payments", "vat_returns", "income_statements"}},
				{key: "tax-payment.exceptions", sourceID: "4.7", priority: 70, title: staticTitle("申报更正、逾期及欠税情况"), include: always, build: buildTaxPaymentExceptions, moduleIDs: []string{"filing_obligations", "payments", degradableModuleCode}, ruleIDs: []string{"TAX-FILING-001", "TAX-PAYMENT-001", "TAX-ARREARS-001"}},
			},
		},
		{
			key: "chapter.vat", sourceID: "5", title: "增值税风险分析",
			overview: vatOverview,
			children: []sectionDefinition{
				{key: "vat.period-overview", sourceID: "5.1", priority: 10, title: staticTitle("各期销售额、进项及应纳税额"), include: always, build: buildVATOverview, moduleIDs: []string{"vat_returns"}},
				{key: "vat.burden-trend", sourceID: "5.2", priority: 20, title: staticTitle("增值税税负率月度趋势"), include: always, build: buildVATBurdenTrendWithChart, moduleIDs: []string{"vat_returns"}, ruleIDs: []string{vatContinuousBurdenRule}},
				{key: "vat.cycle-comparison", sourceID: "5.3", priority: 30, title: staticTitle("销售额与应纳税额变动"), include: always, build: buildVATCycleComparison, moduleIDs: []string{"vat_returns"}},
				{key: "vat.invoice-declaration-difference", sourceID: "5.4", priority: 40, title: staticTitle("销项发票与申报销售额"), include: always, build: buildVATInvoiceDifferenceWithChart, moduleIDs: []string{"vat_returns", "issued_invoices"}},
				{key: "vat.financial-comparison", sourceID: "5.5", priority: 50, title: staticTitle("财务报表收入与增值税销售额"), include: always, build: buildFinancialVATComparison, moduleIDs: []string{"vat_returns", "income_statements"}},
				{key: "vat.special-items", sourceID: "5.6", priority: 60, title: func(ctx *reportContext) string {
					if ctx.taxpayer == taxpayerSmall {
						return "票申差额及红字发票"
					}
					return "未开票收入及红字发票"
				}, include: always, build: buildVATSpecialItems, moduleIDs: []string{"vat_returns"}},
				{key: "vat.input-deduction", sourceID: "5.7", priority: 70, title: staticTitle("进项抵扣风险"), include: always, build: buildInputDeductionRisk, moduleIDs: []string{"taxpayer_profile", "deduction_confirmations"}},
				{key: "vat.end-credit", sourceID: "5.8", priority: 80, title: staticTitle("留抵税额风险"), include: always, build: buildEndCreditWithChart, moduleIDs: []string{"taxpayer_profile", "vat_returns"}},
				{key: "vat.rates-reliefs", sourceID: "5.9", priority: 90, title: staticTitle("免税、简易计税及差额征税"), include: always, build: buildVATRatesAndReliefs, moduleIDs: []string{"vat_returns"}},
			},
		},
		{
			key: "chapter.cit", sourceID: "6", title: "企业所得税风险分析",
			overview: func(*reportContext) []string { return nil },
			children: []sectionDefinition{
				{key: "cit.quarter-contribution", sourceID: "6.1", priority: 10, title: staticTitle(citContributionTitle), include: always, build: buildCITContributionWithChart, moduleIDs: []string{"cit_quarterly_returns", "cit_annual_returns"}},
				{key: "cit.financial-revenue", sourceID: "6.2", priority: 20, title: staticTitle("财务报表收入与企业所得税收入"), include: always, build: buildCITFinancialRevenueComparison, moduleIDs: []string{"cit_quarterly_returns", "income_statements"}},
				{key: "cit.cost-purchases", sourceID: "6.3", priority: 30, title: staticTitle("财务报表成本与购进发票"), include: always, build: buildFinancialCostPurchaseComparison, moduleIDs: []string{"income_statements", "received_invoices"}},
				{key: "cit.profit-taxable", sourceID: "6.4", priority: 40, title: staticTitle("利润总额与应纳税所得额"), include: always, build: buildCITProfitTaxableComparison, moduleIDs: []string{"cit_quarterly_returns"}},
				{key: "cit.annual", sourceID: "6.5", priority: 50, title: staticTitle("纳税调整项目"), include: always, build: buildCITAdjustments, moduleIDs: []string{"cit_annual_returns"}},
				{key: "cit.research-expense", sourceID: "6.6", priority: 60, title: staticTitle("研发加计扣除申报情况"), include: always, build: buildCITResearchExpenseWithChart, moduleIDs: []string{"cit_annual_returns", "income_statements"}},
				{key: "cit.preferences", sourceID: "6.7", priority: 70, title: staticTitle("企业所得税优惠"), include: always, build: buildCITPreferences, moduleIDs: []string{"cit_annual_returns"}},
				{key: "cit.deduction-evidence", sourceID: "6.8", priority: 80, title: staticTitle("税前扣除凭证关注"), include: always, build: buildCITDeductionEvidence, moduleIDs: []string{"cit_annual_returns", "received_invoices"}},
			},
		},
		{
			key: "chapter.other-taxes", sourceID: "7", title: "其他税种及附加风险",
			overview: func(*reportContext) []string {
				return []string{"本章展示税局返回的税源、申报和缴款记录；数据不足的项目单独标注。"}
			},
			children: []sectionDefinition{
				{key: "other-tax.stamp", sourceID: "7.1", priority: 10, title: staticTitle("印花税"), include: always, build: buildStampTaxAnalysis, moduleIDs: []string{"stamp_tax", "payments"}},
				{key: "other-tax.property", sourceID: "7.2", priority: 20, title: staticTitle("房产税"), include: always, build: buildPropertyTaxAnalysis, moduleIDs: []string{"property_tax", "payments"}},
				{key: "other-tax.land-use", sourceID: "7.3", priority: 30, title: staticTitle("城镇土地使用税"), include: always, build: buildLandUseTaxAnalysis, moduleIDs: []string{"urban_land_use_tax", "payments"}},
				{key: "other-tax.surtax", sourceID: "7.4", priority: 40, title: staticTitle("城市维护建设税及附加"), include: always, build: buildSurtaxPaymentsWithChart, moduleIDs: []string{"payments"}},
				{key: "other-tax.union-fund", sourceID: "7.5", priority: 50, title: staticTitle("工会经费"), include: always, build: buildUnionFundPayments, moduleIDs: []string{"payments"}},
				{key: "other-tax.other", sourceID: "7.6", priority: 60, title: staticTitle("其他适用税种"), include: always, build: buildOtherTaxPayments, moduleIDs: []string{"payments", "filing_obligations"}},
			},
		},
		{
			key: "chapter.financial", sourceID: "8", title: "税务报送财务指标分析", standalone: true,
			overview: func(ctx *reportContext) []string {
				if !hasFinancialReportData(ctx) {
					return []string{"财务报表数据未获取，本期暂不分析。"}
				}
				return []string{"本章依据税务报送财务报表展示财务指标；口径不明确的指标仅展示已有数据。"}
			},
			children: []sectionDefinition{
				{key: "financial.statements", sourceID: "8.1", priority: 10, title: staticTitle("收入、成本及利润趋势"), include: includeFinancialReportData, build: buildFinancialOverviewWithChart, moduleIDs: []string{"income_statements", "balance_sheets"}},
				{key: "financial.period-expense", sourceID: "8.2", priority: 20, title: staticTitle("毛利率、利润率及费用率"), include: includeFinancialReportData, build: buildPeriodExpenseWithChart, moduleIDs: []string{"income_statements"}},
				{key: "financial.debt-asset", sourceID: "8.3", priority: 30, title: staticTitle("资产负债率"), include: includeFinancialReportData, build: buildDebtAssetRatioWithChart, moduleIDs: []string{"balance_sheets"}},
				{key: "financial.working-capital", sourceID: "8.4", priority: 40, title: staticTitle("应收账款、存货及合同负债"), include: includeFinancialReportData, build: buildWorkingCapitalMetricsWithChart, moduleIDs: []string{"balance_sheets"}},
				{key: "financial.cash-flow", sourceID: "8.5", priority: 50, title: staticTitle("经营活动现金流"), include: includeFinancialReportData, build: buildCashFlowWithChart, moduleIDs: []string{"balance_sheets", "income_statements"}},
				{key: "financial.summary", sourceID: "8.6", priority: 60, title: staticTitle("财务指标综合提示"), include: includeFinancialReportData, build: buildFinancialSummary, moduleIDs: []string{"income_statements", "balance_sheets"}},
			},
		},
		{
			key: "chapter.industry", sourceID: "9", title: "行业专项风险分析",
			overview: func(ctx *reportContext) []string {
				if len(ctx.categories) == 0 {
					return []string{"本期未取得发票品类数据，暂无法分析业务结构。"}
				}
				return []string{"本章依据销项发票品类展示主要业务结构；没有可靠行业对标数据时不作行业风险判断。"}
			},
			children: industrySections(),
		},
		{
			key: "chapter.remediation", sourceID: "10", title: "风险整改建议",
			overview: func(*reportContext) []string { return nil },
			children: []sectionDefinition{
				{key: "remediation.confirmed", sourceID: "10.1", priority: 10, title: staticTitle("优先处理事项"), include: always, build: buildConfirmedRemediationWithChart, ruleIDs: allRuleIDs},
				{key: "remediation.tracking", sourceID: "10.4", priority: 40, title: staticTitle("整改跟踪表"), include: always, build: buildRemediationTracking, ruleIDs: allRuleIDs},
			},
		},
		{
			key: "chapter.conclusion", sourceID: "11", title: "综合结论", standalone: true,
			overview: buildConclusion,
		},
	}
}

func industrySections() []sectionDefinition {
	children := []sectionDefinition{{key: "industry.structure", sourceID: "9.1", priority: 10, title: staticTitle("业务结构分析"), include: includeIndustryCategoryData, build: buildIndustryStructureWithChart, moduleIDs: []string{"issued_invoices"}}}
	for index := 0; index < 3; index++ {
		rank := index
		children = append(children, sectionDefinition{
			key: fmt.Sprintf("industry.business-%d", index+1), sourceID: fmt.Sprintf("9.%d", index+2), priority: 20 + index,
			title: func(ctx *reportContext) string { return ctx.categories[rank].Name },
			include: func(ctx *reportContext) (bool, OmissionReason) {
				if len(ctx.categories) <= rank {
					return false, OmittedNotApplicable
				}
				return true, ""
			},
			build: func(ctx *reportContext) []Block { return buildBusinessCategory(ctx.categories[rank], rank) }, moduleIDs: []string{"issued_invoices"},
		})
	}
	children = append(children, sectionDefinition{key: "industry.summary", sourceID: "9.5", priority: 50, title: staticTitle("行业经营特征综合分析"), include: includeReliableIndustryBenchmark, build: buildIndustrySummaryWithRadar, moduleIDs: []string{"issued_invoices", qixinModuleCode}})
	return children
}

func staticTitle(title string) func(*reportContext) string {
	return func(*reportContext) string { return title }
}

func chineseChapterTitle(sourceID, title string) string {
	numerals := map[string]string{"1": "一", "2": "二", "3": "三", "4": "四", "5": "五", "6": "六", "7": "七", "8": "八", "9": "九", "10": "十", "11": "十一"}
	return numerals[sourceID] + "、" + title
}

func includeCreditTrend(ctx *reportContext) (bool, OmissionReason) {
	if len(ctx.credits) < 2 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func includeInvoiceTrend(ctx *reportContext) (bool, OmissionReason) {
	if invoiceTrendChart(ctx.modules, ctx.task.StartTime, ctx.task.EndTime) == nil {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func includeCustomers(ctx *reportContext) (bool, OmissionReason) {
	if ctx.customers == nil {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func includeSuppliers(ctx *reportContext) (bool, OmissionReason) {
	if ctx.suppliers == nil {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func includeRedOutput(ctx *reportContext) (bool, OmissionReason) {
	if ctx.redIssuedAmount <= 0 {
		return false, OmittedNoRisk
	}
	return true, ""
}

func includePaymentChart(ctx *reportContext) (bool, OmissionReason) {
	if paymentCompositionChart(ctx.payments) == nil {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

// includeArrearsAnalysis decides whether section 4.7 has a reliable arrears
// outcome. Parameters: ctx is the finalized report context. Return values:
// inclusion and the semantic omission reason used by both DOCX and PDF.
func includeArrearsAnalysis(ctx *reportContext) (bool, OmissionReason) {
	for _, evaluation := range ctx.evaluations {
		if evaluation.RuleID != "TAX-ARREARS-001" {
			continue
		}
		switch evaluation.Outcome {
		case OutcomeDataInsufficient:
			return false, OmittedDataInsufficient
		case OutcomeNotApplicable:
			return false, OmittedNotApplicable
		default:
			return true, ""
		}
	}
	return false, OmittedDataInsufficient
}

func includeGeneralTaxpayer(ctx *reportContext) (bool, OmissionReason) {
	switch ctx.taxpayer {
	case taxpayerGeneral:
		return true, ""
	case taxpayerSmall:
		return false, OmittedNotApplicable
	default:
		return false, OmittedDataInsufficient
	}
}

func includeIndustry(ctx *reportContext) (bool, OmissionReason) {
	if len(ctx.categories) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func includeRiskTrend(ctx *reportContext) (bool, OmissionReason) {
	if len(ctx.findings) == 0 {
		return false, OmittedNoRisk
	}
	if analysisMonthCount(ctx.task.StartTime, ctx.task.EndTime) < 6 || len(riskPeriods(ctx.findings)) == 0 {
		return false, OmittedNotApplicable
	}
	return true, ""
}

// hasFinancialReportData reports whether the task contains at least one usable
// income-statement or balance-sheet metric. Empty report containers do not keep
// six empty financial subsections in the directory.
func hasFinancialReportData(ctx *reportContext) bool {
	for _, fact := range ctx.financial {
		if financialFactHasMetric(fact) {
			return true
		}
	}
	return false
}

func includeFinancialReportData(ctx *reportContext) (bool, OmissionReason) {
	if !hasFinancialReportData(ctx) {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func includeIndustryCategoryData(ctx *reportContext) (bool, OmissionReason) {
	if len(ctx.categories) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

func customerTitle(ctx *reportContext) string {
	if ctx.customers == nil {
		return "主要客户（0家）"
	}
	return ctx.customers.Title
}

func supplierTitle(ctx *reportContext) string {
	if ctx.suppliers == nil {
		return "主要供应商（0家）"
	}
	return ctx.suppliers.Title
}
func invoiceOverallTitle(ctx *reportContext) string {
	return analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "发票总体情况"
}
func paymentPeriodTitle(ctx *reportContext) string {
	return analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "各期税费明细"
}
func vatCycleTitle(ctx *reportContext) string {
	seen := map[string]bool{}
	for _, fact := range ctx.vat {
		if cycle := strings.TrimSpace(fact.Cycle); cycle != "" {
			seen[cycle] = true
		}
	}
	cycles := make([]string, 0, len(seen))
	for _, cycle := range []string{taxCycleMonthly, taxCycleQuarterly, taxCycleAnnual, taxCycleDaily, taxCycleIrregular} {
		if seen[cycle] {
			cycles = append(cycles, cycle)
		}
	}
	if len(cycles) == 0 {
		cycles = append(cycles, taxCycleMonthly)
	}
	return strings.Join(cycles, "、") + "销售额与应纳税额对比"
}
func vatOverview(ctx *reportContext) []string {
	base := "本章展示税局返回的增值税申报和抵扣记录；票申差异在申报销售额有效时计算。"
	if ctx.taxpayer == taxpayerSmall {
		base += " 税局数据明确企业为小规模纳税人，进项抵扣和期末留抵章节不适用，保留不适用说明，不生成空表图。"
	}
	return []string{base}
}

// buildConclusion supplies only source facts for internal model callers.
// The public workflow replaces these blocks with validated V4 Agent output.
func buildConclusion(ctx *reportContext) []string {
	result := []string{"报告企业：" + textOr(companyName(ctx.modules["taxpayer_profile"].Raw), ctx.task.USCC) + "。", "分析期间：" + analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime) + "。"}
	if len(ctx.findings) == 0 {
		return append(result, "本期已取得的数据中未确认风险；未取得完整数据的事项不作判断。")
	}
	for _, finding := range priorityFindings(ctx.findings) {
		if len(result) >= 5 {
			break
		}
		result = append(result, finding.Title+"："+finding.Recommendation)
	}
	return result
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func joinNonEmpty(values ...string) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return strings.Join(result, "；")
}
