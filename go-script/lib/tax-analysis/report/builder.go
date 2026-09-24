package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	taxanalysis "invoice-assistant-skill/go-script/lib/tax-analysis"
)

var requiredModules = []struct {
	Code string
	Name string
}{
	{"taxpayer_profile", "纳税人档案"},
	{"filing_obligations", "应申报清册"},
	{"vat_returns", "增值税申报"},
	{"cit_quarterly_returns", "企业所得税预缴"},
	{"cit_annual_returns", "企业所得税年度汇算"},
	{"balance_sheets", "资产负债表"},
	{"income_statements", "利润表"},
	{"stamp_tax", "印花税"},
	{"property_tax", "房产税"},
	{"urban_land_use_tax", "城镇土地使用税"},
	{"payments", "缴款记录"},
	{"arrears_and_refunds", "欠税及退税"},
	{"tax_credit", "纳税信用"},
	{"issued_invoices", "开具发票"},
	{"received_invoices", "取得发票"},
	{"deduction_confirmations", "用途确认及抵扣"},
	{qixinModuleCode, "启信宝工商司法公开信息"},
}

// BuildModel normalizes the 17-module response into the single model consumed
// by both document renderers. Arrears/refunds may be degraded after supplemental
// retries, and the deduction module may be marked inapplicable when the tax
// authority confirms a small taxpayer. All other modules must be complete. All
// business calculations finish here; renderers only read the finalized model.
func BuildModel(task taxanalysis.TaskResponse, now time.Time) (Model, error) {
	return BuildModelWithEnterpriseContext(task, now, nil)
}

// BuildModelWithEnterpriseContext normalizes one reportable task. The bound
// enterprise snapshot may supply the company name but never taxpayer status.
// Parameters: task is the 17-module OpenTax result, now is the report time, and
// enterpriseContext is the login-preparation snapshot. Return value is the
// single deterministic model consumed by both renderers.
func BuildModelWithEnterpriseContext(task taxanalysis.TaskResponse, now time.Time, enterpriseContext map[string]any) (Model, error) {
	modules, err := normalizeModules(task)
	if err != nil {
		return Model{}, err
	}
	company := companyName(modules["taxpayer_profile"].Raw)
	if company == "" {
		company = strings.TrimSpace(scalarText(enterpriseContext["enterpriseName"]))
	}
	if company == "" {
		company = strings.TrimSpace(task.USCC)
	}
	ctx := newReportContext(task, modules, enterpriseContext, now)
	evaluations := evaluateRules(modules)
	evaluations = append(evaluations, evaluateYearEndReversalRule(ctx.yearEndReversal)...)
	evaluations = append(evaluations, evaluateInvoiceTaxComparisons(ctx)...)
	evaluations = append(evaluations, evaluateCounterpartyRisks(ctx)...)
	evaluations = append(evaluations, evaluateContinuousVATBurden(ctx)...)
	findings := findingsFromEvaluations(evaluations)
	ctx.evaluations = evaluations
	ctx.findings = findings
	sections, omitted := buildSectionTree(ctx)
	sections = applyDeliveryPolicies(ctx, sections)
	sections = addSectionRiskBands(sections, evaluations)
	model := Model{
		FilingEvents:      filingEvents(modules["filing_obligations"]),
		RiskAssessment:    assessOverallRisk(findings, evaluations),
		AnalysisSources:   analysisSourceData(ctx),
		InvoiceCoverage:   ctx.invoiceCoverage,
		Calculations:      ctx.calculations,
		TaxFields:         ctx.taxFields,
		CompanyName:       company,
		USCC:              strings.TrimSpace(task.USCC),
		StartTime:         task.StartTime,
		EndTime:           task.EndTime,
		GeneratedAt:       now,
		RuleVersion:       RuleVersion,
		TaskID:            task.TaskID,
		Sections:          sections,
		Findings:          findings,
		Evaluations:       evaluations,
		ExecutedRuleCount: assessOverallRisk(findings, evaluations).Total,
		Warnings:          append([]string(nil), ctx.warnings...),
		OmittedReasons:    omitted,
	}
	return model, nil
}

func normalizeModules(task taxanalysis.TaskResponse) (map[string]ModuleSnapshot, error) {
	if task.DataSchemaVersion != "tax-analysis.v2" {
		return nil, fmt.Errorf("税务分析数据格式不支持：%s", task.DataSchemaVersion)
	}
	documents, err := decodeTaxDocuments(task.TaxDocuments)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]taxanalysis.ModuleResult, len(task.ModuleResults))
	for _, module := range task.ModuleResults {
		byCode[module.Code] = module
	}
	missing := make([]string, 0)
	result := make(map[string]ModuleSnapshot, len(requiredModules))
	taxpayer := taxpayerClassificationFromTask(task)
	for _, required := range requiredModules {
		module, ok := byCode[required.Code]
		if !ok || !moduleUsableForReportForTaxpayer(module, taxpayer) {
			missing = append(missing, required.Code)
			continue
		}
		decoded, err := decodeRaw(module.Data)
		if err != nil {
			return nil, fmt.Errorf("decode module %s: %w", module.Code, err)
		}
		decoded, err = resolveTaxDocumentRefs(decoded, documents)
		if err != nil {
			return nil, fmt.Errorf("resolve module %s: %w", module.Code, err)
		}
		if !(required.Code == deductionConfirmationModuleCode && taxpayer == taxpayerSmall && module.Status != taxanalysis.ModuleStatusSuccess) {
			if err = validateModuleSummary(required.Code, decoded); err != nil {
				return nil, err
			}
		}
		result[required.Code] = ModuleSnapshot{
			Code: required.Code, Name: required.Name, UpdatedAt: module.UpdatedAt,
			RecordCount: recordCount(decoded), Facts: collectFacts(decoded, 14), Raw: decoded,
			Complete: module.Status == taxanalysis.ModuleStatusSuccess && len(module.FailedItems) == 0 && !hasModuleFailure(task.FailedItems, required.Code),
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("报告生成条件未满足，以下必要模块未成功：%s", strings.Join(missing, ", "))
	}
	return result, nil
}

// hasModuleFailure checks whether task-level failure details belong to a
// module. Parameters: failedItems is the task failure list and moduleCode is
// the stable module code. Return value: true when at least one item matches.
func hasModuleFailure(failedItems []taxanalysis.FailedItem, moduleCode string) bool {
	for _, failedItem := range failedItems {
		if strings.TrimSpace(failedItem.ModuleCode) == moduleCode {
			return true
		}
	}
	return false
}

// moduleUsableForReport reports whether a returned module can enter the report
// model without taxpayer-specific handling. Parameter module is one OpenTax
// module result. Return value is true for a complete mandatory module or a
// terminal arrears/refunds result.
func moduleUsableForReport(module taxanalysis.ModuleResult) bool {
	if module.Code != degradableModuleCode {
		return module.Status == taxanalysis.ModuleStatusSuccess && len(module.FailedItems) == 0
	}
	switch module.Status {
	case taxanalysis.ModuleStatusSuccess, taxanalysis.ModuleStatusFailed, taxanalysis.ModuleStatusNotSupported:
		return true
	default:
		return false
	}
}

// moduleUsableForReportForTaxpayer applies the one taxpayer-specific module
// exception required by the tax authority. Parameters are an OpenTax module and
// the confirmed taxpayer kind. Return value is true when the module is normally
// reportable, or when a small taxpayer has only the expected deduction-feature
// rejection.
func moduleUsableForReportForTaxpayer(module taxanalysis.ModuleResult, taxpayer taxpayerKind) bool {
	if moduleUsableForReport(module) {
		return true
	}
	if emptyTaxSourceModuleCodes[module.Code] && moduleHasOnlyFailureMessage(module, emptyTaxSourceMessage) {
		return true
	}
	if module.Code != deductionConfirmationModuleCode || taxpayer != taxpayerSmall {
		return false
	}
	if module.Status != taxanalysis.ModuleStatusFailed && module.Status != taxanalysis.ModuleStatusNotSupported {
		return false
	}
	if len(module.FailedItems) == 0 {
		return false
	}
	for _, failedItem := range module.FailedItems {
		if !strings.Contains(strings.TrimSpace(failedItem.Message), smallTaxpayerAuthorityMessageFragment) {
			return false
		}
	}
	return true
}

// moduleHasOnlyFailureMessage reports whether a terminal module contains one or
// more failures with the same exact message. Parameters are the module result and
// expected message. Return value is false for mixed errors or a missing failure.
func moduleHasOnlyFailureMessage(module taxanalysis.ModuleResult, expected string) bool {
	if module.Status != taxanalysis.ModuleStatusFailed && module.Status != taxanalysis.ModuleStatusNotSupported {
		return false
	}
	if len(module.FailedItems) == 0 {
		return false
	}
	for _, failedItem := range module.FailedItems {
		if strings.TrimSpace(failedItem.Message) != expected {
			return false
		}
	}
	return true
}

// taxpayerClassificationFromTask resolves qualification from tax-authority data
// already present in the aggregate response. Parameter task is the completed
// OpenTax task. Return value never reads bound-enterprise list metadata.
func taxpayerClassificationFromTask(task taxanalysis.TaskResponse) taxpayerKind {
	module, ok := moduleResultsByCode(task.ModuleResults)["taxpayer_profile"]
	if !ok {
		return taxpayerUnknown
	}
	raw, err := decodeRaw(module.Data)
	if err != nil {
		return taxpayerUnknown
	}
	classification, _ := taxpayerClassificationForTask(task, ModuleSnapshot{Code: module.Code, Raw: raw})
	return classification
}

func moduleBlocks(module ModuleSnapshot) []Block {
	statusText := "成功"
	if !module.Complete {
		statusText = "部分数据已取得"
	}
	rows := [][]string{
		{"模块", module.Name},
		{"状态", statusText},
		{"更新时间", textOr(formatReportFactValue("updatedAt", module.UpdatedAt), "未获取")},
		{"识别记录数", fmt.Sprintf("%d", module.RecordCount)},
	}
	blocks := []Block{{Kind: BlockTable, Title: "数据覆盖", Headers: []string{"项目", "结果"}, Rows: rows}}
	factRows := make([][]string, 0, len(module.Facts))
	seenFacts := map[string]bool{}
	for _, fact := range module.Facts {
		if strings.TrimSpace(fact.Label) == "" {
			continue
		}
		key := strings.TrimSpace(fact.Label) + "\x00" + strings.TrimSpace(fact.Value)
		if seenFacts[key] {
			continue
		}
		seenFacts[key] = true
		factRows = append(factRows, []string{fact.Label, fact.Value})
	}
	if len(factRows) > 0 {
		blocks = append(blocks, Block{Kind: BlockTable, Title: "业务数据摘要", Headers: []string{"业务指标", "结果"}, Rows: factRows})
	} else if module.Complete {
		blocks = append(blocks, Block{Kind: BlockParagraph, Paragraphs: []string{"接口调用成功，本分析期间未返回可展示记录。"}})
	} else {
		blocks = append(blocks, Block{Kind: BlockParagraph, Paragraphs: []string{"本节仅展示当前已取得的客观信息，未取得的数据不作无异常判断。"}})
	}
	return blocks
}

func companyName(root any) string {
	value, _, found := findFirst(root, "nsrmc", "enterpriseName", "companyName", "taxpayerName")
	if !found {
		return ""
	}
	return strings.TrimSpace(scalarText(value))
}

func textOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func bookmark(key string) string {
	value := strings.ToLower(strings.TrimSpace(key))
	value = strings.NewReplacer(".", "_", "-", "_", "/", "_", " ", "_").Replace(value)
	return "section_" + value
}
