package report

import (
	"fmt"
	"math"
	"strings"
)

// evaluateQixinRules 执行当前产品已确认的工商和公开信用确定性规则。
//
// 参数 module 为统一任务中的启信宝模块快照；返回全部已执行规则评价。
func evaluateQixinRules(module ModuleSnapshot) []RuleEvaluation {
	if module.Raw == nil {
		return nil
	}
	subjects := qixinSubjectsFromRaw(module.Raw)
	if len(subjects) == 0 {
		return nil
	}
	return []RuleEvaluation{
		evaluateQixinBusinessStatus(subjects),
		evaluateQixinTaxViolation(subjects),
		evaluateQixinTaxPenalty(subjects),
		evaluateQixinOperatingAbnormal(subjects),
		evaluateQixinAdministrativePenalty(subjects),
		evaluateQixinSeriousIllegal(subjects),
		evaluateQixinOwnExecution(subjects),
	}
}

// evaluateQixinBusinessStatus 仅在工商状态明确为注销、吊销等异常状态时形成高风险。
//
// 参数 subjects 为已精确核验主体列表；返回企业自身工商状态规则结果。
func evaluateQixinBusinessStatus(subjects []map[string]any) RuleEvaluation {
	own := qixinOwnFromSubjects(subjects)
	detail := qixinSubjectData(own, qixinDetailField)
	status := strings.TrimSpace(firstText(detail, "NewStatus", "Status"))
	if status == "" {
		return RuleEvaluation{RuleID: qixinRuleBusinessStatus, Outcome: OutcomeDataInsufficient, Evidence: "企业工商登记状态字段缺失。"}
	}
	if !containsAny(status, qixinAbnormalBusinessStatusKeywords...) {
		return RuleEvaluation{RuleID: qixinRuleBusinessStatus, Outcome: OutcomeClear, Evidence: "企业工商登记状态未触发注销、吊销等直接状态异常规则。"}
	}
	finding := Finding{ID: qixinRuleBusinessStatus, RiskThemeID: "qixin-business-status", Title: "企业工商登记状态异常", Level: RiskHigh, ModuleCode: qixinModuleCode, Period: "公开信息当前状态", Evidence: "启信宝工商详情返回企业登记状态为“" + status + "”。", Recommendation: "立即核对工商与税务登记当前状态、终止日期及尚未完成的申报缴税义务，并保留状态变更资料。", Confidence: "中", VerificationRequired: true}
	return RuleEvaluation{RuleID: qixinRuleBusinessStatus, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// qixinSubjectsFromRaw 从模块原始数据读取已经精确核验的主体。
//
// 参数 root 为 qixin_external 模块数据；返回可进入报告规则的主体列表。
func qixinSubjectsFromRaw(root any) []map[string]any {
	values := arrayAt(root, qixinSubjectsField)
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if subject, ok := value.(map[string]any); ok && boolText(subject, qixinSubjectVerifiedField) {
			result = append(result, subject)
		}
	}
	return result
}

// qixinOwnFromSubjects 从核验主体列表选择当前报告企业。
//
// 参数 subjects 为已核验主体列表；返回企业自身主体，未取得时返回 nil。
func qixinOwnFromSubjects(subjects []map[string]any) map[string]any {
	for _, subject := range subjects {
		if mapText(subject, qixinSubjectRoleField) == qixinOwnEnterpriseRole {
			return subject
		}
	}
	return nil
}

// evaluateQixinTaxViolation 对企业自身重大税收违法公开记录执行直接规则。
//
// 参数 subjects 为已核验主体列表；返回重大税收违法规则评价。
func evaluateQixinTaxViolation(subjects []map[string]any) RuleEvaluation {
	const ruleID = qixinRuleTaxViolation
	own := qixinOwnFromSubjects(subjects)
	rows := qixinItemMaps(qixinSubjectData(own, qixinTaxViolationField), "IllegalFact", "Result", "CheckDepartment")
	if len(rows) == 0 {
		return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeClear, Evidence: "公开信息查询未发现企业自身重大税收违法公开记录。"}
	}
	periods := periodsFromQixinRows(rows, "Time", "CreatedTime", "UpdatedTime")
	finding := Finding{ID: ruleID, RiskThemeID: "qixin-major-tax-violation", Title: "存在重大税收违法公开记录", Level: RiskHigh, ModuleCode: qixinModuleCode, Period: periodDisplay(periods, "公开信息当前状态"), Periods: periods, Evidence: fmt.Sprintf("启信宝重大税收违法接口返回 %d 条企业自身记录。", len(rows)), Recommendation: "逐项核对违法事实、处理结果、税款及处罚执行状态，并留存整改和信用修复资料。", Confidence: "高"}
	return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// evaluateQixinTaxPenalty 按明确处理状态评价税务机关作出的行政处罚。
//
// 参数 subjects 为已精确核验主体列表；返回税务行政处罚状态规则结果。
func evaluateQixinTaxPenalty(subjects []map[string]any) RuleEvaluation {
	own := qixinOwnFromSubjects(subjects)
	all := qixinItemMaps(qixinSubjectData(own, qixinAdministrativePunishmentField), "Number", "Department", "Content")
	unhandled := make([]map[string]any, 0, len(all))
	resolved := make([]map[string]any, 0, len(all))
	for _, row := range all {
		if !containsAny(mapText(row, "Department"), "税", "税务") {
			continue
		}
		switch qixinPenaltyState(row) {
		case "unhandled":
			unhandled = append(unhandled, row)
		case "resolved":
			resolved = append(resolved, row)
		}
	}
	selected, level, title, verification := unhandled, RiskHigh, "存在尚未处理的税务行政处罚公开记录", true
	if len(selected) == 0 {
		selected, level, title, verification = resolved, RiskLow, "存在已处理的历史税务行政处罚公开记录", false
	}
	if len(selected) == 0 {
		return RuleEvaluation{RuleID: qixinRuleTaxPenalty, Outcome: OutcomeClear, Evidence: "未识别处理状态明确的税务行政处罚记录；状态缺失的记录仅作事实展示。"}
	}
	periods := periodsFromQixinRows(selected, "Date", "PublishDate")
	finding := Finding{ID: qixinRuleTaxPenalty, RiskThemeID: "qixin-tax-penalty", Title: title, Level: level, ModuleCode: qixinModuleCode, Period: periodDisplay(periods, "公开信息当前状态"), Periods: periods, Amount: qixinPenaltyAmountTotal(selected), Evidence: fmt.Sprintf("启信宝行政处罚接口返回 %d 条税务机关作出且处理状态明确的记录。", len(selected)), Recommendation: "逐项核对处罚决定、缴款或整改要求及当前处理状态，并留存履行和信用修复资料。", Confidence: "中", VerificationRequired: verification}
	return RuleEvaluation{RuleID: qixinRuleTaxPenalty, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// evaluateQixinOperatingAbnormal 对尚未移出的经营异常记录形成中风险核实事项。
//
// 参数 subjects 为已精确核验主体列表；返回企业自身经营异常规则结果。
func evaluateQixinOperatingAbnormal(subjects []map[string]any) RuleEvaluation {
	own := qixinOwnFromSubjects(subjects)
	current := qixinCurrentAbnormalRows(own)
	if len(current) == 0 && len(qixinListingRows(own, qixinDetailField)) > 0 {
		counts := qixinListingCount(qixinListingRows(own, qixinDetailField))
		if counts.Unknown+counts.Conflicting > 0 {
			return RuleEvaluation{RuleID: qixinRuleOperatingAbnormal, Outcome: OutcomeDataInsufficient, Evidence: "企业自身经营异常存在历史记录，当前处理状态未确认。"}
		}
	}
	if len(current) == 0 {
		return RuleEvaluation{RuleID: qixinRuleOperatingAbnormal, Outcome: OutcomeClear, Evidence: "未识别企业自身尚未移出的经营异常公开记录。"}
	}
	periods := periodsFromQixinRows(current, "InDate")
	finding := Finding{ID: qixinRuleOperatingAbnormal, RiskThemeID: "qixin-operating-abnormal", Title: "存在当前未移出的经营异常公开记录", Level: RiskMedium, ModuleCode: qixinModuleCode, Period: periodDisplay(periods, "公开信息当前状态"), Periods: periods, Evidence: fmt.Sprintf("启信宝工商详情返回 %d 条明确当前在列的经营异常记录。", len(current)), Recommendation: "核实列入原因、整改材料和移出进度，并评估其对合同履行、开票和上下游合作的实际影响。", Confidence: "中", VerificationRequired: true}
	return RuleEvaluation{RuleID: qixinRuleOperatingAbnormal, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// evaluateQixinAdministrativePenalty 按明确处理状态评价非税务行政处罚。
//
// 参数 subjects 为已精确核验主体列表；返回企业自身非税务行政处罚规则结果。
func evaluateQixinAdministrativePenalty(subjects []map[string]any) RuleEvaluation {
	own := qixinOwnFromSubjects(subjects)
	all := qixinItemMaps(qixinSubjectData(own, qixinAdministrativePunishmentField), "Number", "Department", "Content")
	unhandled := make([]map[string]any, 0, len(all))
	resolved := make([]map[string]any, 0, len(all))
	for _, row := range all {
		if containsAny(mapText(row, "Department"), "税", "税务") {
			continue
		}
		switch qixinPenaltyState(row) {
		case "unhandled":
			unhandled = append(unhandled, row)
		case "resolved":
			resolved = append(resolved, row)
		}
	}
	selected, level, title, verification := unhandled, RiskMedium, "存在尚未处理的行政处罚公开记录", true
	if len(selected) == 0 {
		selected, level, title, verification = resolved, RiskLow, "存在已处理的历史行政处罚公开记录", false
	}
	if len(selected) == 0 {
		return RuleEvaluation{RuleID: qixinRuleAdministrativePenalty, Outcome: OutcomeClear, Evidence: "未识别处理状态明确的非税务行政处罚记录；状态缺失的记录仅作事实展示。"}
	}
	periods := periodsFromQixinRows(selected, "Date", "PublishDate")
	finding := Finding{ID: qixinRuleAdministrativePenalty, RiskThemeID: "qixin-administrative-penalty", Title: title, Level: level, ModuleCode: qixinModuleCode, Period: periodDisplay(periods, "公开信息当前状态"), Periods: periods, Amount: qixinPenaltyAmountTotal(selected), Evidence: fmt.Sprintf("启信宝行政处罚接口返回 %d 条非税务机关作出且处理状态明确的记录。", len(selected)), Recommendation: "核实处罚事项与主营经营、许可资质和整改要求的关系，并保留处理完成证明。", Confidence: "中", VerificationRequired: verification}
	return RuleEvaluation{RuleID: qixinRuleAdministrativePenalty, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// evaluateQixinSeriousIllegal 对尚未移出的严重违法失信记录执行直接规则。
//
// 参数 subjects 为已核验主体列表；返回严重违法失信规则评价。
func evaluateQixinSeriousIllegal(subjects []map[string]any) RuleEvaluation {
	const ruleID = qixinRuleSeriousIllegal
	own := qixinOwnFromSubjects(subjects)
	current := qixinCurrentSeriousIllegalRows(own)
	if len(current) == 0 && len(qixinListingRows(own, qixinSeriousIllegalField)) > 0 {
		counts := qixinListingCount(qixinListingRows(own, qixinSeriousIllegalField))
		if counts.Unknown+counts.Conflicting > 0 {
			return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeDataInsufficient, Evidence: "企业自身严重违法存在历史记录，当前处理状态未确认。"}
		}
	}
	if len(current) == 0 {
		return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeClear, Evidence: "公开信息查询未发现企业自身当前未移出的严重违法失信记录。"}
	}
	periods := periodsFromQixinRows(current, "InDate")
	finding := Finding{ID: ruleID, RiskThemeID: "qixin-serious-illegal", Title: "存在当前严重违法失信公开记录", Level: RiskHigh, ModuleCode: qixinModuleCode, Period: periodDisplay(periods, "公开信息当前状态"), Periods: periods, Evidence: fmt.Sprintf("启信宝严重违法接口返回 %d 条明确当前在列的企业自身记录。", len(current)), Recommendation: "核实列入原因、整改要求和移出条件，完成整改后跟踪公开状态更新。", Confidence: "高", VerificationRequired: true}
	return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// evaluateQixinOwnExecution 评价企业自身被执行和失信被执行公开记录。
//
// 参数 subjects 为已核验主体列表；返回企业自身执行风险规则评价。
func evaluateQixinOwnExecution(subjects []map[string]any) RuleEvaluation {
	const ruleID = qixinRuleOwnExecution
	own := qixinOwnFromSubjects(subjects)
	rows := append(qixinItemMaps(qixinSubjectData(own, qixinExecutionField), "CaseNumber", "Court", "Amount"), qixinItemMaps(qixinSubjectData(own, qixinDishonestField), "CaseNumber", "ExecutionStatus", "FinalDuty")...)
	if len(rows) == 0 {
		return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeClear, Evidence: "公开信息查询未发现企业自身被执行或失信被执行记录。"}
	}
	amount := qixinAmountTotal(rows)
	periods := periodsFromQixinRows(rows, "CaseDate", "Date", "PublishDate")
	finding := Finding{ID: ruleID, RiskThemeID: "qixin-own-execution", Title: "企业自身存在公开执行信息", Level: RiskMedium, ModuleCode: qixinModuleCode, Period: periodDisplay(periods, "公开信息当前状态"), Periods: periods, Amount: amount, Evidence: fmt.Sprintf("启信宝执行接口返回 %d 条企业自身被执行或失信被执行记录。", len(rows)), Recommendation: "逐案核实执行状态、执行金额和履行结果，评估对经营、融资和资金安排的影响。", Confidence: "高", VerificationRequired: true}
	return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// periodsFromQixinRows 提取公开信息记录中可识别的事件日期。
//
// 参数 rows 为公开信息记录，aliases 为日期字段别名；返回去重排序的日期集合。
func periodsFromQixinRows(rows []map[string]any, aliases ...string) []string {
	periods := []string{}
	for _, row := range rows {
		for _, alias := range aliases {
			if value := strings.TrimSpace(mapText(row, alias)); value != "" {
				periods = append(periods, value)
				break
			}
		}
	}
	return uniqueSorted(periods)
}

// qixinAmountTotal 汇总执行记录的元口径金额绝对值。
//
// 参数 rows 为执行或失信记录；返回金额绝对值合计，缺失金额不补零参与单项判断。
func qixinAmountTotal(rows []map[string]any) float64 {
	total := 0.0
	for _, row := range rows {
		if value, ok := mapNumber(row, "Amount"); ok {
			total += math.Abs(value)
		}
	}
	return total
}

// qixinPenaltyAmountTotal 将官方以万元返回的处罚金额汇总成元。
//
// 参数 rows 为行政处罚记录；返回处罚金额绝对值的元口径合计。
func qixinPenaltyAmountTotal(rows []map[string]any) float64 {
	total := 0.0
	for _, row := range rows {
		if value, ok := mapNumber(row, "PunishAmnt"); ok {
			total += math.Abs(value) * qixinTenThousandYuanScale
		}
	}
	return total
}

// qixinPenaltyState 根据失效标志和中文状态将处罚分为未处理、已处理或未知。
//
// 参数 row 为行政处罚记录；返回 unhandled、resolved 或 unknown。
func qixinPenaltyState(row map[string]any) string {
	if boolText(row, "Disabled") {
		return "resolved"
	}
	status := strings.TrimSpace(mapText(row, "Status"))
	if containsAny(status, qixinUnhandledStatusKeywords...) {
		return "unhandled"
	}
	if containsAny(status, qixinResolvedStatusKeywords...) {
		return "resolved"
	}
	return "unknown"
}
