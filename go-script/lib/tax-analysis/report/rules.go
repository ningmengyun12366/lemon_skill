package report

import (
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strings"
)

func evaluateRules(modules map[string]ModuleSnapshot) []RuleEvaluation {
	evaluations := make([]RuleEvaluation, 0, 8)
	evaluations = append(evaluations,
		evaluateFilingRule(modules["filing_obligations"]),
		evaluatePaymentRule(modules["payments"]),
		evaluateArrearsRule(modules["arrears_and_refunds"]),
		evaluateCreditRule(modules["tax_credit"]),
	)
	evaluations = append(evaluations, evaluateSignalRule("INVOICE-OUTPUT-001", "疑似虚开销项发票组合线索", "issued_invoices", collectSuspiciousSignals(modules, true))...)
	evaluations = append(evaluations, evaluateSignalRule("INVOICE-INPUT-001", "疑似取得虚假进项发票组合线索", "received_invoices", collectSuspiciousSignals(modules, false))...)
	if module, ok := modules[qixinModuleCode]; ok {
		evaluations = append(evaluations, evaluateQixinRules(module)...)
	}
	return evaluations
}

func evaluateFindings(modules map[string]ModuleSnapshot) []Finding {
	return findingsFromEvaluations(evaluateRules(modules))
}

func evaluatePaymentRule(module ModuleSnapshot) RuleEvaluation {
	const ruleID = "TAX-PAYMENT-001"
	rows := lateFeePaymentRows(module)
	if len(rows) == 0 {
		return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeClear, Evidence: "税局缴款记录未识别滞纳金。"}
	}
	occurrences := riskOccurrencesFromRows(rows, "lateFeeAmount", "sjje", "amount")
	periods := []string{}
	for _, occurrence := range occurrences {
		periods = appendUnique(periods, occurrence.Period)
	}
	amount := 0.0
	for _, row := range rows {
		if value, ok := mapNumber(row, "lateFeeAmount", "sjje", "amount"); ok {
			amount += math.Abs(value)
		}
	}
	finding := Finding{
		ID: ruleID, RiskThemeID: "payment-late-fee", Title: "存在税收滞纳金缴款记录", Level: RiskHigh,
		ModuleCode: module.Code, Period: strings.Join(periods, "、"), Periods: periods, Amount: amount,
		Occurrences:    occurrences,
		Evidence:       paymentLateFeeEvidence(rows, amount, periods),
		Recommendation: "核对对应税种、所属期和逾期原因，确认相关申报及缴款义务已经处理完毕。", Confidence: "高",
	}
	return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// Both risk evaluation and the reading brief use exactly the same source rows.
func lateFeePaymentRows(module ModuleSnapshot) []map[string]any {
	match := func(object map[string]any) bool {
		if strings.TrimSpace(mapText(object, "skzlDm")) == "20" {
			return true
		}
		amount, amountOK := mapNumber(object, "lateFeeAmount")
		return amountOK && amount > 0
	}
	sourceRows, present := namedCollectionMapsWithPresence(module.Raw, "list")
	var rows []map[string]any
	if present {
		for _, row := range sourceRows {
			if match(row) {
				rows = append(rows, row)
			}
		}
	} else {
		rows = findAllMaps(module.Raw, match)
	}
	return rows
}

func evaluateArrearsRule(module ModuleSnapshot) RuleEvaluation {
	const ruleID = "TAX-ARREARS-001"
	value, _, found := findFirst(module.Raw, "arrearsInfo")
	records, outcome := arrearsRecords(value, found, module.Complete)
	if outcome != OutcomeRisk {
		evidence := "税局欠税查询未返回当前欠税信息。"
		switch outcome {
		case OutcomeNotApplicable:
			evidence = "当前税局欠税查询不适用于该企业。"
		case OutcomeDataInsufficient:
			evidence = "欠税查询结果未完整取得，未作无异常判断。"
		}
		return RuleEvaluation{RuleID: ruleID, Outcome: outcome, Evidence: evidence}
	}
	amount := 0.0
	for _, row := range findAllMaps(records, func(object map[string]any) bool {
		_, ok := mapNumber(object, "amount", "arrearsAmount", "qsje")
		return ok
	}) {
		if item, ok := mapNumber(row, "amount", "arrearsAmount", "qsje"); ok {
			amount += math.Abs(item)
		}
	}
	finding := Finding{
		ID: ruleID, RiskThemeID: "current-tax-arrears", Title: "税局返回当前欠税信息", Level: RiskHigh,
		ModuleCode: module.Code, Period: "当前状态", Amount: amount,
		Evidence:       "税局欠税查询返回当前欠税记录。",
		Recommendation: "立即核对欠税税种、金额、限缴期限及催缴状态，并保留缴清或异议处理资料。", Confidence: "高",
	}
	return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

// arrearsRecords classifies the arrears source independently from refund
// failures. Parameters: value is the arrears source payload, found indicates
// whether it was returned, and moduleComplete indicates whether the aggregate
// module has no failures. Return values: normalized records and the deterministic
// evaluation outcome; missing failed data is never treated as clear.
func arrearsRecords(value any, found, moduleComplete bool) (any, EvaluationOutcome) {
	if !found || value == nil {
		if moduleComplete {
			return nil, OutcomeClear
		}
		return nil, OutcomeDataInsufficient
	}
	if wrapper, ok := value.(map[string]any); ok {
		status := strings.ToUpper(strings.TrimSpace(mapText(wrapper, "status")))
		switch status {
		case "EMPTY", "NO_DATA", "NONE":
			return nil, OutcomeClear
		case "NOT_APPLICABLE", "UNSUPPORTED":
			return nil, OutcomeNotApplicable
		}
		if data, exists := mapValue(wrapper, "data", "list", "records", "items"); exists {
			if nonEmptyValue(data) {
				return data, OutcomeRisk
			}
			if status == "SUCCESS" {
				return nil, OutcomeClear
			}
			if status == "FAILED" || status == "PARTIAL" || status == "ERROR" || !moduleComplete {
				return nil, OutcomeDataInsufficient
			}
			return nil, OutcomeClear
		}
		// Backward-compatible unwrapped record: only a record carrying an
		// arrears amount can form a risk. Wrapper metadata alone never can.
		if _, exists := mapNumber(wrapper, "amount", "arrearsAmount", "qsje"); exists {
			return wrapper, OutcomeRisk
		}
		if status == "SUCCESS" {
			return nil, OutcomeClear
		}
		if status == "FAILED" || status == "PARTIAL" || status == "ERROR" || !moduleComplete {
			return nil, OutcomeDataInsufficient
		}
		return nil, OutcomeClear
	}
	if values, ok := value.([]any); ok {
		if len(values) > 0 {
			return values, OutcomeRisk
		}
		return nil, OutcomeClear
	}
	if moduleComplete {
		return nil, OutcomeClear
	}
	return nil, OutcomeDataInsufficient
}

func evaluateCreditRule(module ModuleSnapshot) RuleEvaluation {
	const ruleID = "TAX-CREDIT-001"
	value, _, found := findFirst(module.Raw, "taxpayerCreditRating", "creditRating", "nsxydj")
	rating := strings.ToUpper(strings.TrimSpace(scalarText(value)))
	if !found || rating == "" {
		return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeDataInsufficient, Evidence: "当前纳税信用等级字段缺失。"}
	}
	level := RiskLevel("")
	switch {
	case isCreditGrade(rating, "D"):
		level = RiskHigh
	case isCreditGrade(rating, "C"):
		level = RiskMedium
	}
	if level == "" {
		return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeClear, Evidence: "当前纳税信用等级未触发C/D级规则。"}
	}
	finding := Finding{
		ID: ruleID, RiskThemeID: "current-tax-credit", Title: "当前纳税信用等级需关注", Level: level,
		ModuleCode: module.Code, Period: "当前状态",
		Evidence:       fmt.Sprintf("税局当前纳税信用等级为 %s。", rating),
		Recommendation: "核对评价结果和待修复事项；本期未取得年度扣分明细，不推测扣分原因。", Confidence: "高",
	}
	return RuleEvaluation{RuleID: ruleID, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence}
}

func evaluateSignalRule(ruleID, title, moduleCode string, signals []ruleSignal) []RuleEvaluation {
	if len(signals) == 0 {
		return []RuleEvaluation{{RuleID: ruleID, Outcome: OutcomeClear, Evidence: "未识别到满足组合条件的税务信号。"}}
	}
	groups := map[string][]ruleSignal{}
	for _, signal := range signals {
		key := signalGroupKey(signal.Counterparty, signal.Period)
		groups[key] = append(groups[key], signal)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	evaluations := make([]RuleEvaluation, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		directSignals := make([]ruleSignal, 0, len(group))
		weakSignals := make([]ruleSignal, 0, len(group))
		for _, signal := range group {
			if signal.Direct {
				directSignals = append(directSignals, signal)
			} else {
				weakSignals = append(weakSignals, signal)
			}
		}
		selected := weakSignals
		if len(directSignals) > 0 {
			// Direct tax-authority compliance evidence forms its own high-risk
			// finding. Weak signals are deliberately excluded from its evidence.
			selected = directSignals
		}
		if len(directSignals) == 0 && len(uniqueSignalTypes(selected)) < 2 {
			continue
		}
		amount := 0.0
		counted := map[string]bool{}
		var sharedEvidence []string
		allShared := true
		displayTypes := make([]string, 0, len(selected))
		evidence := make([]string, 0, len(selected))
		for _, signal := range selected {
			key := signal.EvidenceID
			if key == "" {
				key = signal.Type + "|" + signal.Evidence
			}
			if !counted[key] {
				amount += math.Abs(signal.Amount)
				counted[key] = true
			}
			allShared = allShared && signal.CrossDirectionEvidence
			sharedEvidence = append(sharedEvidence, key)
			displayTypes = append(displayTypes, signalTypeDisplay(signal.Type))
			evidence = append(evidence, signal.Evidence)
		}
		parts := strings.SplitN(key, "|", 2)
		counterparty, period := parts[0], "分析期间"
		if len(parts) == 2 && parts[1] != "" {
			period = parts[1]
		}
		level, confidence, verification := RiskMedium, "中", true
		if len(directSignals) > 0 {
			level, confidence, verification = RiskHigh, "高", false
		}
		findingID := ruleID + "-" + fmt.Sprint(len(evaluations)+1)
		themeID := strings.ToLower(ruleID) + ":" + counterparty + ":" + period
		if allShared {
			themeID = "invoice-evidence:" + fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(uniqueSorted(sharedEvidence), "|"))))
		}
		finding := Finding{
			ID: findingID, RiskThemeID: themeID,
			Title: title + "（" + counterparty + "）", Level: level, ModuleCode: moduleCode,
			Period: period, Periods: []string{period}, Evidence: joinEvidenceFragments(evidence...),
			Recommendation: "核对相关合同、物流、资金及发票用途证据；未完成核实前不得直接认定违法。",
			Confidence:     confidence, VerificationRequired: verification, Amount: amount, SignalTypes: uniqueSorted(displayTypes),
		}
		evaluations = append(evaluations, RuleEvaluation{RuleID: ruleID, Outcome: OutcomeRisk, Finding: &finding, Evidence: finding.Evidence})
	}
	if len(evaluations) == 0 {
		return []RuleEvaluation{{RuleID: ruleID, Outcome: OutcomeClear, Evidence: "仅识别到单个弱信号，未形成组合风险事项。"}}
	}
	return evaluations
}

func uniqueSignalTypes(signals []ruleSignal) []string {
	values := make([]string, 0, len(signals))
	for _, signal := range signals {
		values = append(values, signal.Type)
	}
	return uniqueSorted(values)
}

func findingsFromEvaluations(evaluations []RuleEvaluation) []Finding {
	findings := make([]Finding, 0)
	for _, evaluation := range evaluations {
		if evaluation.Outcome == OutcomeRisk && evaluation.Finding != nil {
			findings = append(findings, *evaluation.Finding)
		}
	}
	return sortedFindings(deduplicateFindings(findings))
}

func deduplicateFindings(findings []Finding) []Finding {
	byTheme := map[string]Finding{}
	priority := map[RiskLevel]int{RiskHigh: 3, RiskMedium: 2, RiskLow: 1}
	for _, finding := range findings {
		key := strings.TrimSpace(finding.RiskThemeID)
		if key == "" {
			key = finding.ID
		}
		current, ok := byTheme[key]
		if !ok {
			byTheme[key] = finding
			continue
		}
		current.Periods = uniqueSorted(append(current.Periods, finding.Periods...))
		current.Occurrences = append(current.Occurrences, finding.Occurrences...)
		current.Period = periodDisplay(current.Periods, textOr(current.Period, finding.Period))
		current.SignalTypes = uniqueSorted(append(current.SignalTypes, finding.SignalTypes...))
		current.Evidence = joinUniqueText(current.Evidence, finding.Evidence)
		current.Amount += finding.Amount
		if priority[finding.Level] > priority[current.Level] ||
			(priority[finding.Level] == priority[current.Level] && confidencePriority(finding.Confidence) < confidencePriority(current.Confidence)) {
			current.ID = finding.ID
			current.Title = finding.Title
			current.Level = finding.Level
			current.ModuleCode = finding.ModuleCode
			current.Recommendation = finding.Recommendation
			current.Confidence = finding.Confidence
			current.VerificationRequired = finding.VerificationRequired
		} else if priority[finding.Level] == priority[current.Level] {
			current.VerificationRequired = current.VerificationRequired || finding.VerificationRequired
		}
		byTheme[key] = current
	}
	result := make([]Finding, 0, len(byTheme))
	for _, finding := range byTheme {
		result = append(result, finding)
	}
	return result
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = true
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func joinUniqueText(values ...string) string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, "；") {
			part = strings.TrimSpace(part)
			if part != "" && !seen[part] {
				seen[part] = true
				result = append(result, part)
			}
		}
	}
	return joinEvidenceFragments(result...)
}

func periodsFromValues(values []any) []string {
	rows := make([]map[string]any, 0)
	for _, value := range values {
		if row, ok := value.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	return periodsFromMaps(rows)
}

func periodsFromMaps(rows []map[string]any) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		period := recordPeriod(row)
		if month := extractMonth(period); month != "" {
			period = month
		}
		if period != "" {
			seen[period] = true
		}
	}
	periods := make([]string, 0, len(seen))
	for period := range seen {
		periods = append(periods, period)
	}
	sort.Strings(periods)
	return periods
}

func periodDisplay(periods []string, fallback string) string {
	if len(periods) == 0 {
		return fallback
	}
	if len(periods) == 1 {
		return periods[0]
	}
	return periods[0] + "—" + periods[len(periods)-1]
}

func isCreditGrade(value string, grade string) bool {
	normalized := strings.NewReplacer(" ", "", "\t", "", "：", ":").Replace(strings.ToUpper(strings.TrimSpace(value)))
	for _, candidate := range []string{grade, grade + "级", "纳税信用" + grade + "级", "纳税信用等级" + grade, "信用等级" + grade} {
		if normalized == candidate {
			return true
		}
	}
	return false
}
