package report

import (
	"fmt"
	"time"
)

type yearEndReversalAnalysis struct {
	Observations []yearEndReversalObservation
}

type yearEndReversalObservation struct {
	BaseYear             int
	DecemberBlueAmount   float64
	NextQuarterRedAmount float64
	LinkedRedAmount      float64
	AmountDataComplete   bool
	LinkDataComplete     bool
}

// evaluateYearEndReversalRule evaluates the confirmed three-month year-end reversal rule.
//
// 参数 analysis 为完整跨年观察结果；返回每个适用年度的确定性规则评价，或一项不适用评价。
func evaluateYearEndReversalRule(analysis yearEndReversalAnalysis) []RuleEvaluation {
	if len(analysis.Observations) == 0 {
		return []RuleEvaluation{{RuleID: yearEndReversalRuleID, Outcome: OutcomeNotApplicable, Evidence: "分析期间未完整覆盖任一自然年12月及次年1—3月。"}}
	}
	evaluations := make([]RuleEvaluation, 0, len(analysis.Observations))
	for _, observation := range analysis.Observations {
		outcome, level, assessment := yearEndReversalAssessment(observation)
		evidence := yearEndReversalEvidence(observation, assessment)
		evaluation := RuleEvaluation{RuleID: yearEndReversalRuleID, Outcome: outcome, Evidence: evidence}
		if outcome == OutcomeRisk {
			period := yearEndReversalPeriod(observation.BaseYear)
			finding := Finding{
				ID:                   fmt.Sprintf("%s-%d", yearEndReversalRuleID, observation.BaseYear),
				RiskThemeID:          fmt.Sprintf("%s:%d", yearEndReversalThemeID, observation.BaseYear),
				Title:                fmt.Sprintf("%d年末开票及次年1—3月红冲需关注", observation.BaseYear),
				Level:                level,
				ModuleCode:           yearEndReversalModuleCode,
				Period:               period,
				Periods:              yearEndReversalPeriods(observation.BaseYear),
				Evidence:             evidence,
				Recommendation:       "逐笔核对原蓝字发票、红字发票、合同、交付验收、退货折让或开票错误资料，并复核收入确认和增值税申报所属期。",
				Confidence:           "高",
				VerificationRequired: true,
				Amount:               observation.LinkedRedAmount,
			}
			evaluation.Finding = &finding
		}
		evaluations = append(evaluations, evaluation)
	}
	return evaluations
}

// yearEndReversalAssessment maps one observation to an outcome, risk level and user-visible conclusion.
//
// 参数 observation 为单个跨年观察结果；返回评价结果、风险等级及中文结论。非风险结果的风险等级为空。
func yearEndReversalAssessment(observation yearEndReversalObservation) (EvaluationOutcome, RiskLevel, string) {
	if !observation.AmountDataComplete {
		return OutcomeDataInsufficient, "", "数据不足：跨年观察窗口存在缺少不含税金额的发票记录。"
	}
	if observation.DecemberBlueAmount <= 0 {
		return OutcomeNotApplicable, "", "不适用：当年12月未识别到可计算的销项蓝字开票金额。"
	}
	if observation.NextQuarterRedAmount > 0 && !observation.LinkDataComplete {
		return OutcomeDataInsufficient, "", "数据不足：次年1—3月红字发票未完整返回原蓝字发票号码，只展示红字总量，不计算对应红冲比例。"
	}
	ratio := observation.LinkedRedAmount / observation.DecemberBlueAmount
	switch {
	case ratio < yearEndReversalLowThreshold:
		return OutcomeClear, "", "未见明显异常"
	case ratio <= yearEndReversalMediumThreshold:
		return OutcomeRisk, RiskLow, "低风险"
	case ratio <= yearEndReversalHighThreshold:
		return OutcomeRisk, RiskMedium, "中风险"
	default:
		return OutcomeRisk, RiskHigh, "高风险"
	}
}

// yearEndReversalEvidence formats the exact amounts and formula used by the rule engine.
//
// 参数 observation 为单个观察结果，assessment 为评价结论；返回可进入报告和风险卡的中文证据。
func yearEndReversalEvidence(observation yearEndReversalObservation, assessment string) string {
	if observation.DecemberBlueAmount <= 0 || !observation.AmountDataComplete || (observation.NextQuarterRedAmount > 0 && !observation.LinkDataComplete) {
		return fmt.Sprintf("%d年12月销项蓝字开票金额为%s元，%d年1—3月红字销项金额为%s元。%s", observation.BaseYear, moneyText(observation.DecemberBlueAmount), observation.BaseYear+1, moneyText(observation.NextQuarterRedAmount), assessment)
	}
	ratio := observation.LinkedRedAmount / observation.DecemberBlueAmount
	return fmt.Sprintf("%d年12月销项蓝字开票金额为%s元，%d年1—3月红字销项金额为%s元，其中与该年12月原蓝票对应的红字金额为%s元，对应红冲比例为%s，评价为%s。", observation.BaseYear, moneyText(observation.DecemberBlueAmount), observation.BaseYear+1, moneyText(observation.NextQuarterRedAmount), moneyText(observation.LinkedRedAmount), percentText(ratio), assessment)
}

// yearEndReversalPeriod formats the complete December-to-March display period.
//
// 参数 baseYear 为12月所属年度；返回跨年观察期间的中文文本。
func yearEndReversalPeriod(baseYear int) string {
	return fmt.Sprintf("%d年12月—%d年03月", baseYear, baseYear+1)
}

// yearEndReversalPeriods returns all four monthly buckets used by risk trends.
//
// 参数 baseYear 为12月所属年度；返回当年12月及次年1—3月的标准月份列表。
func yearEndReversalPeriods(baseYear int) []string {
	return []string{fmt.Sprintf("%d-12", baseYear), fmt.Sprintf("%d-01", baseYear+1), fmt.Sprintf("%d-02", baseYear+1), fmt.Sprintf("%d-03", baseYear+1)}
}

// buildYearEndReversal renders the fixed-directory section from the finalized three-month analysis.
//
// 参数 ctx 为报告上下文；返回跨年明细表、计算口径和已命中风险卡，未跨年时返回不适用说明。
func buildYearEndReversal(ctx *reportContext) []Block {
	if len(ctx.yearEndReversal.Observations) == 0 {
		message := "本报告期未完整覆盖年末12月及次年1—3月，暂无法进行年末开票及次年红冲分析。"
		if endDate, err := time.Parse("2006-01-02", ctx.task.EndTime); err == nil && endDate.Month() == time.December {
			message = fmt.Sprintf("本报告期未覆盖%d年1—3月，暂无法进行年末开票及次年红冲分析。", endDate.Year()+1)
		}
		return []Block{{Kind: BlockTable, Headers: []string{"项目", "金额", "占比/说明"}, Rows: [][]string{{"年末开票及次年红冲", "—", message}}}}
	}
	rows := make([][]string, 0, len(ctx.yearEndReversal.Observations))
	for _, observation := range ctx.yearEndReversal.Observations {
		_, _, assessment := yearEndReversalAssessment(observation)
		linkedAmount := "数据不足"
		ratio := "数据不足"
		if observation.AmountDataComplete && (observation.NextQuarterRedAmount == 0 || observation.LinkDataComplete) {
			linkedAmount = moneyText(observation.LinkedRedAmount)
			if observation.DecemberBlueAmount > 0 {
				ratio = percentText(observation.LinkedRedAmount / observation.DecemberBlueAmount)
			} else {
				ratio = "不适用"
			}
		}
		rows = append(rows,
			[]string{yearEndReversalPeriod(observation.BaseYear) + "12月销项蓝字金额", moneyText(observation.DecemberBlueAmount), "—"},
			[]string{"次年1—3月红字金额", moneyText(observation.NextQuarterRedAmount), "—"},
			[]string{"对应12月原蓝票红字金额", linkedAmount, ratio + "；" + assessment},
		)
	}
	blocks := []Block{{Kind: BlockTable, Title: "年末开票及次年1—3月红冲明细", Headers: []string{"项目", "金额", "占比/说明"}, Rows: rows}}
	for _, evaluation := range ctx.evaluations {
		if evaluation.RuleID != yearEndReversalRuleID || evaluation.Outcome != OutcomeRisk || evaluation.Finding == nil {
			continue
		}
		finding := *evaluation.Finding
		blocks = append(blocks, Block{Kind: BlockRiskCard, Title: string(finding.Level), Paragraphs: []string{finding.Evidence, "需核实：" + finding.Recommendation}, Finding: &finding})
	}
	return blocks
}
