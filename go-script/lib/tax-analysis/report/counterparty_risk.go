package report

import (
	"fmt"
	"strings"
)

const (
	customerConcentrationRule = "INVOICE-CUSTOMER-CONCENTRATION-001"
	supplierConcentrationRule = "INVOICE-SUPPLIER-CONCENTRATION-001"
	customerCreditRule        = "QIXIN-CUSTOMER-CREDIT-001"
	supplierCreditRule        = "QIXIN-SUPPLIER-CREDIT-001"
	concentrationFiveLow      = 0.40
	concentrationTenLow       = 0.60
	concentrationFiveMedium   = 0.60
	concentrationTenMedium    = 0.80
	customerExecutionShare    = 0.05
	supplierExecutionShare    = 0.10
	dishonestShare            = 0.10
)

var concentrationLabels = []string{"第一大占比", "前五大占比", "前十大占比"}

func concentrationShares(facts []invoiceGroup) (shares []float64, amounts []float64, total float64) {
	values := counterpartyAmounts(facts)
	shares, amounts = make([]float64, 3), make([]float64, 3)
	total = invoiceNetAmount(facts)
	for i, n := range []int{1, 5, 10} {
		for j := 0; j < min(n, len(values)); j++ {
			amounts[i] += values[j].amount
		}
		if total > 0 {
			shares[i] = amounts[i] / total
		}
	}
	return
}

func concentrationLevel(five, ten float64) RiskLevel {
	if five >= concentrationFiveMedium || ten >= concentrationTenMedium {
		return RiskMedium
	}
	if five >= concentrationFiveLow || ten >= concentrationTenLow {
		return RiskLow
	}
	return ""
}

func evaluateConcentration(ctx *reportContext, role string) RuleEvaluation {
	id, module, facts, label := customerConcentrationRule, "issued_invoices", ctx.issued, "客户"
	if role == qixinImportantSupplierRole {
		id, module, facts, label = supplierConcentrationRule, "received_invoices", ctx.received, "供应商"
	}
	e := RuleEvaluation{RuleID: id, Outcome: OutcomeDataInsufficient, Evidence: label + "发票覆盖不足，不按样本确定集中度等级。"}
	shares, _, total := concentrationShares(facts)
	if !ctx.invoiceCoverage[module].Complete || total <= 0 {
		return e
	}
	e.Outcome = OutcomeClear
	e.Evidence = fmt.Sprintf("第一大%s，前五大%s，前十大%s。", percentText(shares[0]), percentText(shares[1]), percentText(shares[2]))
	level := concentrationLevel(shares[1], shares[2])
	if level == "" {
		return e
	}
	recommendation := "结合前五大客户合同与订单延续情况核对收入依赖及回款安排，按客户跟踪订单金额与回款，不因集中度直接认定税务违规。"
	if role == qixinImportantSupplierRole {
		recommendation = "核对前五大供应商的交付保障、替代货源与预付款安排，按供应商跟踪采购占比和交付，结合代理或长期采购协议评估供货依赖。"
	}
	period := analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime)
	e.Outcome = OutcomeRisk
	e.Finding = &Finding{ID: id, RiskThemeID: id, Title: label + "交易集中度达到管理预警范围", Level: level, ModuleCode: module, Period: period, Periods: []string{period}, Occurrences: concentrationQuarterOccurrences(facts, ctx.task.StartTime, ctx.task.EndTime), Evidence: e.Evidence, Recommendation: recommendation, Confidence: "中", VerificationRequired: true}
	return e
}

// qixinExecutionState never infers unresolved debt from the mere presence of a
// historical record. Opaque source codes stay unknown; explicit conflicts do too.
func qixinExecutionState(row map[string]any) string {
	unresolved, resolved := false, false
	for _, alias := range []string{"ExecutionStatus", "Status", "CaseStatus", "FulfillmentStatus"} {
		value := mapText(row, alias)
		if isOpaqueStatusCode(value) {
			continue
		}
		for _, phrase := range []string{"未全部履行", "未结案", "尚未履行", "未履行", "未执行", "执行中", "未完成", "部分履行", "正在执行"} {
			if strings.Contains(value, phrase) {
				unresolved = true
				// “未全部履行” contains “全部履行”; remove the negative
				// phrase before checking independently reported resolved states.
				value = strings.ReplaceAll(value, phrase, "")
			}
		}
		resolved = resolved || containsAny(value, "已结案", "已履行完毕", "全部履行", "已全部履行", "履行完毕", "已修复", "已撤销", "执行完毕")
	}
	if resolved && unresolved {
		return "状态冲突"
	}
	if resolved {
		return "已处理"
	}
	if unresolved {
		return "未履行完毕"
	}
	return "当前状态未取得"
}

func counterpartyCreditEvaluation(ctx *reportContext, subject map[string]any) RuleEvaluation {
	role := mapText(subject, qixinSubjectRoleField)
	id, minimum, label := customerCreditRule, customerExecutionShare, "客户"
	if role == qixinImportantSupplierRole {
		id, minimum, label = supplierCreditRule, supplierExecutionShare, "供应商"
	}
	e := RuleEvaluation{RuleID: id, Outcome: OutcomeClear}
	share, known := invoiceCounterpartyShare(ctx, role, subject)
	module := "issued_invoices"
	if role == qixinImportantSupplierRole {
		module = "received_invoices"
	}
	if !known || !ctx.invoiceCoverage[module].Complete {
		e.Outcome = OutcomeDataInsufficient
		e.Evidence = "交易占比缺少完整可比发票范围"
		return e
	}
	var qualified []map[string]any
	var evidence []string
	level := RiskLevel("")
	for _, field := range []string{qixinExecutionField, qixinDishonestField} {
		rows := qixinItemMaps(qixinSubjectData(subject, field), "CaseNumber", "Court", "Amount", "ExecutionStatus")
		if qixinSubjectData(subject, field) == nil {
			e.Outcome = OutcomeDataInsufficient
		}
		for _, row := range rows {
			state := qixinExecutionState(row)
			if state == "当前状态未取得" || state == "状态冲突" {
				e.Outcome = OutcomeDataInsufficient
				continue
			}
			if state != "未履行完毕" {
				continue
			}
			threshold := minimum
			if field == qixinDishonestField {
				threshold = dishonestShare
			}
			if share < threshold {
				continue
			}
			qualified = append(qualified, row)
			if level == "" {
				level = RiskMedium
			}
			if role == qixinImportantSupplierRole && field == qixinDishonestField && isLeadingDependentSupplier(ctx, subject) {
				level = RiskHigh
			}
			evidence = append(evidence, fmt.Sprintf("%s，%s，%s", mapText(row, "CaseNumber"), mapText(row, "CaseDate", "Date", "PublishDate"), qixinReadableStatus(row, "ExecutionStatus", "Status", "CaseStatus", "FulfillmentStatus")))
		}
	}
	if len(qualified) == 0 {
		e.Evidence = "未识别达到本角色交易占比门槛且已明确仍未履行完毕的执行记录；状态缺失记录保留待核实。"
		return e
	}
	name := mapText(subject, qixinSubjectNameField)
	recommendation := "按所列案号确认当前履行余额、客户回款及赊销敞口，结合未履行状态调整信用期或收款条件，以案件处理结果和回款记录核验完成。"
	if role == qixinImportantSupplierRole {
		recommendation = "按所列案号确认履行余额与供应商交付能力，核对预付款敞口及备选供应安排，以案件处理结果、交付记录和预付款回收核验完成。"
	}
	periods := periodsFromQixinRows(qualified, "CaseDate", "Date", "PublishDate")
	e.Outcome = OutcomeRisk
	e.Evidence = fmt.Sprintf("%s交易占比%s，公开记录明确尚未履行完毕：%s。", name, percentText(share), strings.Join(evidence, "；"))
	e.Finding = &Finding{ID: id + "-" + analysisToken(name), RiskThemeID: id + "-" + analysisToken(name), Title: label + "存在未履行完毕的公开执行信息", Level: level, ModuleCode: qixinModuleCode, Period: periodDisplay(periods, "公开信息当前状态"), Periods: periods, Amount: qixinAmountTotal(qualified), Evidence: e.Evidence, Recommendation: recommendation, Confidence: "中", VerificationRequired: true}
	return e
}

func evaluateCounterpartyRisks(ctx *reportContext) []RuleEvaluation {
	results := []RuleEvaluation{evaluateConcentration(ctx, qixinImportantCustomerRole), evaluateConcentration(ctx, qixinImportantSupplierRole)}
	for _, subject := range qixinSubjects(ctx) {
		role := mapText(subject, qixinSubjectRoleField)
		if role == qixinImportantCustomerRole || role == qixinImportantSupplierRole {
			results = append(results, counterpartyCreditEvaluation(ctx, subject))
		}
		if role == qixinImportantSupplierRole {
			results = append(results, supplierSeriousIllegalEvaluation(ctx, subject))
		}
	}
	return results
}

func counterpartyExecutionSummary(subject map[string]any) string {
	var parts []string
	for _, field := range []string{qixinExecutionField, qixinDishonestField} {
		rows := qixinItemMaps(qixinSubjectData(subject, field), "CaseNumber", "Court", "ExecutionStatus")
		if len(rows) == 0 {
			continue
		}
		// Keep full cases in local analysis evidence. The customer table shows
		// one representative per type, prioritizing unresolved then recent cases.
		chosen := rows[0]
		for _, row := range rows[1:] {
			current := qixinExecutionState(row) == "未履行完毕"
			chosenCurrent := qixinExecutionState(chosen) == "未履行完毕"
			if (current && !chosenCurrent) || (current == chosenCurrent && mapText(row, "CaseDate", "Date", "PublishDate") > mapText(chosen, "CaseDate", "Date", "PublishDate")) {
				chosen = row
			}
		}
		label := "被执行记录摘要"
		if field == qixinDishonestField {
			label = "失信被执行记录摘要"
		}
		parts = append(parts, label+"："+strings.Join([]string{textOr(mapText(chosen, "CaseNumber"), "案号未取得"), textOr(mapText(chosen, "CaseDate", "Date", "PublishDate"), "日期未取得"), qixinExecutionState(chosen)}, "，"))
	}
	return strings.Join(parts, "；")
}

// addCounterpartyExecutionEvidence supplies each role's own cases to AI. The
// role-specific section cannot borrow an unrelated customer's/supplier's risk.
func addCounterpartyExecutionEvidence(d *SectionAnalysisData, ctx *reportContext, subject map[string]any, role string) {
	name := mapText(subject, qixinSubjectNameField)
	for _, field := range []string{qixinExecutionField, qixinDishonestField} {
		for index, row := range qixinItemMaps(qixinSubjectData(subject, field), "CaseNumber", "Court", "Amount", "ExecutionStatus") {
			caseNo, date := mapText(row, "CaseNumber"), mapText(row, "CaseDate", "Date", "PublishDate")
			key := "credit-case." + analysisToken(name+"|"+field+"|"+caseNo+"|"+date+fmt.Sprint(index))
			source := "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects[name=" + name + "]/" + field
			refs := []string{d.text(key+".name", "执行记录主体", "object", name, source), d.text(key+".state", "执行记录当前状态", "state", qixinExecutionState(row), source)}
			if caseNo != "" {
				refs = append(refs, d.text(key+".case", "执行案号", "object", caseNo, source))
			}
			if date != "" {
				refs = append(refs, d.text(key+".date", "案件记录日期", "period", date, source))
			}
			if share, known := invoiceCounterpartyShare(ctx, role, subject); known {
				refs = append(refs, d.number(key+".share", "本报告同方向交易占比", "share", "%", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "同方向精确主体交易额/同方向发票总额×100", share*100, counterpartyInvoiceSource(role)))
			}
			if amount, known := mapNumber(row, "Amount"); known {
				d.number(key+".amount", "案件公开金额", "amount", "元", date, "", "原执行金额，不当作当前未履行余额", amount, source)
			}
			if len(d.Focus) < 2 {
				d.focus(key, "说明具体客户或供应商的交易占比、案件日期/案号和当前处理状态，结合本角色门槛解释回款、赊销或供货含义；未知状态必须保留待核实，不能将历史公开金额当当前欠款。", refs...)
			}
		}
	}
}

func counterpartyInvoiceSource(role string) string {
	module := "issued_invoices"
	if role == qixinImportantSupplierRole {
		module = "received_invoices"
	}
	return "source-snapshot.json/moduleResults[code=" + module + "]/data/chunks/groups"
}

// qixinListingState requires an explicit current status. A missing removal
// date alone is insufficient to assert that an old listing is still active.
func qixinListingState(row map[string]any) string {
	removed := mapText(row, "OutDate", "RemoveDate") != ""
	current := false
	for _, alias := range []string{"Status", "CurrentStatus", "ListStatus"} {
		status := qixinReadableStatus(row, alias)
		removed = removed || containsAny(status, "已移出", "已修复", "已撤销")
		current = current || containsAny(status, "未移出", "仍在", "当前在列", "列入中") || status == "列入" || status == "在列"
	}
	if removed && current {
		return "状态冲突"
	}
	if removed {
		return "已移出或处理"
	}
	if current {
		return "当前在列"
	}
	return "当前状态未取得"
}

// Other public credit events remain review evidence when current status,
// business relevance or delivery dependence cannot be confirmed from the API.
func addCounterpartyOtherCreditEvidence(d *SectionAnalysisData, ctx *reportContext, subject map[string]any, role string) {
	name := mapText(subject, qixinSubjectNameField)
	for _, field := range []string{qixinSeriousIllegalField, qixinAdministrativePunishmentField} {
		for index, row := range counterpartyOtherCreditRows(subject, field) {
			date := qixinCreditText(row, "InDate", "Date", "PublishDate")
			prefix := "credit-other." + analysisToken(name+"|"+field+"|"+date+"|"+fmt.Sprint(index))
			source := fmt.Sprintf("source-snapshot.json/moduleResults[code=qixin_external]/data/subjects[name=%s]/%s[%d]", name, field, index)
			state, label := qixinListingState(row), "严重违法记录"
			if field == qixinAdministrativePunishmentField {
				state, label = qixinReadableStatus(row, "Status", "CurrentStatus"), "行政处罚记录"
			}
			refs := []string{d.text(prefix+".name", label+"主体", "object", name, source), d.text(prefix+".state", label+"处理状态", "state", state, source)}
			if date != "" {
				refs = append(refs, d.text(prefix+".date", label+"日期", "period", date, source))
			}
			if field == qixinAdministrativePunishmentField {
				refs = append(refs, addQixinPenaltyDetails(d, row, prefix, source)...)
			} else if description := qixinCreditText(row, "InReason", "Reason", "Content"); description != "" {
				refs = append(refs, d.text(prefix+".reason", label+"公开原因", "state", description, source))
			}
			if share, known := invoiceCounterpartyShare(ctx, role, subject); known {
				refs = append(refs, d.number(prefix+".share", "本报告同方向交易占比", "share", "%", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "精确主体交易额/同方向总额×100", share*100, counterpartyInvoiceSource(role)))
			}
			if len(d.Focus) < 3 {
				d.focus(prefix, "说明该主体事件时间、当前处理状态和交易占比；区分历史已处理、明确当前在列与状态未知，结合已取得业务相关性和依赖程度解释影响，不将所有历史处罚统一升级。", refs...)
			}
		}
	}
}

func supplierSeriousIllegalEvaluation(ctx *reportContext, subject map[string]any) RuleEvaluation {
	e := RuleEvaluation{RuleID: supplierCreditRule, Outcome: OutcomeClear}
	rows := qixinItemMaps(qixinSubjectData(subject, qixinSeriousIllegalField), "InReason", "InDate", "InDepartment")
	if len(rows) == 0 {
		return e
	}
	share, known := invoiceCounterpartyShare(ctx, qixinImportantSupplierRole, subject)
	if !known || !ctx.invoiceCoverage["received_invoices"].Complete {
		e.Outcome = OutcomeDataInsufficient
		return e
	}
	var qualified []map[string]any
	var evidence []string
	for _, row := range rows {
		state := qixinListingState(row)
		if state == "当前状态未取得" || state == "状态冲突" {
			e.Outcome = OutcomeDataInsufficient
		} else if state == "当前在列" && isLeadingDependentSupplier(ctx, subject) {
			qualified = append(qualified, row)
			evidence = append(evidence, mapText(row, "InDate")+"，"+mapText(row, "InReason"))
		}
	}
	if len(qualified) == 0 {
		e.Evidence = "未同时确认严重违法当前在列与高度供货依赖；历史记录及状态未知保留核实。"
		return e
	}
	name := mapText(subject, qixinSubjectNameField)
	periods := periodsFromQixinRows(qualified, "InDate")
	e.Outcome = OutcomeRisk
	e.Evidence = fmt.Sprintf("第一大供应商%s交易占比%s，供应商集中度达到中等级管理预警范围，严重违法记录明确当前在列：%s。", name, percentText(share), strings.Join(evidence, "；"))
	e.Finding = &Finding{ID: supplierCreditRule + "-serious-" + analysisToken(name), RiskThemeID: supplierCreditRule + "-serious-" + analysisToken(name), Title: "高度依赖的供应商存在当前严重违法记录", Level: RiskHigh, ModuleCode: qixinModuleCode, Period: periodDisplay(periods, "公开信息当前状态"), Periods: periods, Evidence: e.Evidence, Recommendation: "按列入事项核对供应商当前经营及交付能力，落实替代货源与预付款安排，以当前名单状态、交付保障资料及替代采购记录核验。", Confidence: "中", VerificationRequired: true}
	return e
}

// A leading supplier is highly relied on only when the actual supplier
// concentration meets the existing Word boundary; no new percentage is added.
func isLeadingDependentSupplier(ctx *reportContext, subject map[string]any) bool {
	if ctx.suppliers == nil || len(ctx.suppliers.Items) == 0 {
		return false
	}
	first := ctx.suppliers.Items[0]
	if qixinIdentityText(first.Name) != qixinIdentityText(mapText(subject, "name")) {
		return false
	}
	shares, _, total := concentrationShares(ctx.received)
	return total > 0 && concentrationLevel(shares[1], shares[2]) == RiskMedium
}

// concentrationQuarterOccurrences recomputes concentration from the invoices in
// each quarter. An annual ratio is never assigned to its first month.
func concentrationQuarterOccurrences(facts []invoiceGroup, start, end string) []RiskOccurrence {
	groups := map[string][]invoiceGroup{}
	for _, f := range facts {
		month := extractMonth(f.Month)
		if month != "" {
			q := quarterlyPeriodLabel(month, start, end)
			groups[q] = append(groups[q], f)
		}
	}
	var result []RiskOccurrence
	for _, q := range sortedMapKeys(groups) {
		shares, _, total := concentrationShares(groups[q])
		if total > 0 && concentrationLevel(shares[1], shares[2]) != "" {
			a, b := periodBounds(q)
			result = append(result, RiskOccurrence{Period: q, Start: a, End: b, Kind: "period_assessment", Count: 1, Level: concentrationLevel(shares[1], shares[2])})
		}
	}
	return result
}
