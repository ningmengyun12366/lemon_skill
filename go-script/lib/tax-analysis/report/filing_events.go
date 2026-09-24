package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	filingNotDue                 = "not_due"
	filingOverdue                = "overdue_unfiled"
	filingLateFiled              = "late_filed"
	filingCorrected              = "corrected"
	filingUnknown                = "unknown"
	filingRepeatMinimum          = 2
	filingCorrectionMinimum      = 3
	filingCorrectionWindowMonths = 6
)

// FilingEvent keeps source-specific deadlines and states separate from payment
// events. Unknown deadlines never become a statutory date or zero overdue days.
type FilingEvent struct {
	ID             string    `json:"id"`
	TaxName        string    `json:"taxName"`
	Period         TaxPeriod `json:"period"`
	Kind           string    `json:"kind"`
	Deadline       string    `json:"deadline,omitempty"`
	FiledAt        string    `json:"filedAt,omitempty"`
	ObservedAt     string    `json:"observedAt,omitempty"`
	OverdueDays    int       `json:"overdueDays,omitempty"`
	HasOverdueDays bool      `json:"hasOverdueDays"`
	Source         string    `json:"source"`
	SourceStatus   string    `json:"sourceStatus,omitempty"`
}

func filingEvents(module ModuleSnapshot) []FilingEvent {
	var result []FilingEvent
	seen := map[string]bool{}
	for _, collection := range []string{"undeclaredRecords", "declaredRecords", "recordHistory"} {
		rows, _ := namedCollectionMapsWithPresence(module.Raw, collection)
		for index, row := range rows {
			period, _ := taxPeriodFromRecord(row, false)
			e := FilingEvent{TaxName: textOr(mapText(row, "taxType", "zsxmMc", "zsxmmc"), "未识别税费种"), Period: period,
				Deadline:     normalizeReportDate(mapText(row, "filingDeadline", "sbqx"), false),
				FiledAt:      normalizeReportDate(mapText(row, "nssbrq", "sbrq", "declarationDate", "filingDate"), false),
				ObservedAt:   normalizeReportDate(mapText(row, "observedAt"), false),
				Source:       fmt.Sprintf("source-snapshot.json/moduleResults[code=filing_obligations]/data/%s[%d]", collection, index),
				SourceStatus: mapText(row, "sourceStatus", "status", "gzlx", "declarationStatus"), Kind: filingUnknown}
			if collection == "undeclaredRecords" {
				// Only this dedicated query supplies this state contract. Numeric status
				// values from older/general interfaces have no cross-source meaning.
				if mapText(row, "sourceType") == "DescribeQnsbwcx" {
					state := mapText(row, "filingState")
					if (state == filingNotDue || state == filingOverdue) && e.Deadline != "" && e.ObservedAt != "" {
						if e.Deadline < e.ObservedAt {
							e.Kind = filingOverdue
						} else {
							e.Kind = filingNotDue
						}
					}
				}
			} else if containsAny(mapText(row, "gzlx", "status", "declarationStatus"), "更正") {
				e.Kind = filingCorrected
			}
			if collection != "undeclaredRecords" && e.Kind != filingCorrected && e.Deadline != "" && e.FiledAt != "" && e.FiledAt > e.Deadline {
				e.Kind = filingLateFiled
			}
			end := e.FiledAt
			if e.Kind == filingOverdue {
				end = e.ObservedAt
			}
			if (e.Kind == filingOverdue || e.Kind == filingLateFiled) && e.Deadline != "" && end != "" && end > e.Deadline {
				deadline, err1 := time.Parse(time.DateOnly, e.Deadline)
				actual, err2 := time.Parse(time.DateOnly, end)
				if err1 == nil && err2 == nil {
					e.OverdueDays = int(actual.Sub(deadline).Hours() / 24)
					e.HasOverdueDays = true
				}
			}
			if collection != "undeclaredRecords" && e.Kind == filingUnknown {
				continue
			}
			key := strings.Join([]string{e.TaxName, e.Period.Start, e.Period.End, e.Kind, e.FiledAt, mapText(row, "sbuuid", "pzxh", "uuid", "id", "recordId")}, "|")
			if seen[key] {
				continue
			}
			seen[key] = true
			e.ID = "filing-event." + analysisToken(key)
			result = append(result, e)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func filingEventText(e FilingEvent) string {
	label := map[string]string{filingNotDue: "尚未到申报期限，未申报不作为逾期", filingOverdue: "截至查询日逾期未申报", filingLateFiled: "逾期申报，已有申报记录", filingCorrected: "申报更正记录", filingUnknown: "未申报记录的期限或业务状态未完整取得，暂不能确定是否逾期"}[e.Kind]
	parts := []string{e.TaxName, textOr(e.Period.Display, "所属期未取得"), label}
	if e.Deadline != "" {
		parts = append(parts, "申报截止日"+e.Deadline)
	}
	if e.FiledAt != "" {
		parts = append(parts, "申报日期"+e.FiledAt)
	}
	if e.ObservedAt != "" {
		parts = append(parts, "查询日期"+e.ObservedAt)
	}
	if e.HasOverdueDays {
		parts = append(parts, fmt.Sprintf("逾期%d天", e.OverdueDays))
	}
	return strings.Join(parts, "；")
}

func evaluateFilingRule(module ModuleSnapshot) RuleEvaluation {
	const ruleID = "TAX-FILING-001"
	result := RuleEvaluation{RuleID: ruleID, Outcome: OutcomeClear, Evidence: "已取得的申报记录未触发逾期或重复更正规则；未取得应申报清册的税费种不作无漏报判断。"}
	events := filingEvents(module)
	overdue := map[string]map[string]bool{}
	corrections := map[string][]FilingEvent{}
	var late []FilingEvent
	uncertain := !module.Complete
	for _, e := range events {
		switch e.Kind {
		case filingOverdue, filingLateFiled:
			if overdue[e.TaxName] == nil {
				overdue[e.TaxName] = map[string]bool{}
			}
			overdue[e.TaxName][e.Period.Start+"|"+e.Period.End] = true
			if e.Kind == filingLateFiled {
				late = append(late, e)
			} else {
				uncertain = true
			}
		case filingCorrected:
			if e.FiledAt != "" {
				corrections[e.TaxName] = append(corrections[e.TaxName], e)
			} else {
				uncertain = true
			}
		case filingUnknown:
			uncertain = true
		}
	}
	level := RiskLevel("")
	reason := ""
	var selected []FilingEvent
	for _, tax := range sortedMapKeys(overdue) {
		if len(overdue[tax]) >= filingRepeatMinimum {
			level = RiskMedium
			reason = "同税费种重复逾期达到2次"
			for _, e := range events {
				if e.TaxName == tax && (e.Kind == filingOverdue || e.Kind == filingLateFiled) {
					selected = append(selected, e)
				}
			}
		}
	}
	for _, tax := range sortedMapKeys(corrections) {
		rows := corrections[tax]
		sort.Slice(rows, func(i, j int) bool { return rows[i].FiledAt < rows[j].FiledAt })
		for i, e := range rows {
			date, _ := time.Parse(time.DateOnly, e.FiledAt)
			limit := date.AddDate(0, filingCorrectionWindowMonths, 0).Format(time.DateOnly)
			j := i
			for j < len(rows) && rows[j].FiledAt < limit {
				j++
			}
			if j-i >= filingCorrectionMinimum {
				level = RiskMedium
				reason = joinEvidenceFragments(reason, "同税费种6个月内更正达到3次，是否为必要系统性更正尚待核实")
				selected = append(selected, rows[i:j]...)
				break
			}
		}
	}
	if level == "" && len(late) > 0 {
		level = RiskLow
		reason = "单次逾期且已取得补报记录"
		selected = late
	}
	if level == "" {
		if uncertain {
			result.Outcome = OutcomeDataInsufficient
			result.Evidence = "部分申报事项缺少期限、申报处理结果或完整清册；不将未到期未申报或未知状态判为高风险。"
		}
		return result
	}
	var periods, texts []string
	var occurrences []RiskOccurrence
	seen := map[string]bool{}
	for _, e := range selected {
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		periods = appendUnique(periods, e.Period.Display)
		texts = append(texts, filingEventText(e))
		occurrences = append(occurrences, RiskOccurrence{Period: e.Period.Display, Start: e.Period.Start, End: e.Period.End, Kind: "event", Count: 1})
	}
	f := Finding{ID: ruleID, RiskThemeID: "filing-confirmed-exception", Title: "申报逾期及重复更正事项", Level: level, ModuleCode: module.Code, Periods: periods, Period: periodDisplay(periods, "分析期间"), Occurrences: occurrences,
		Evidence: joinEvidenceFragments(append(texts, "判级原因："+reason)...), Recommendation: "按具体税费种和所属期核对申报回执、截止日及更正原因；仅对尚未处理的义务补报，已补报事项保留回执，必要系统性更正另附说明。", Confidence: "高", VerificationRequired: strings.Contains(reason, "更正")}
	result.Outcome = OutcomeRisk
	result.Finding = &f
	result.Evidence = f.Evidence
	return result
}

func analysisFilingEvents(ctx *reportContext) []SectionAnalysisData {
	var result []SectionAnalysisData
	for _, section := range []string{"tax-payment.filing-status", "tax-payment.exceptions"} {
		d := SectionAnalysisData{SectionID: section}
		events := filingEvents(ctx.modules["filing_obligations"])
		for _, e := range events {
			refs := []string{d.text(e.ID+".event", "申报事项及可得处理状态", "state", filingEventText(e), e.Source)}
			if e.HasOverdueDays {
				refs = append(refs, d.number(e.ID+".days", "逾期天数", "duration", "天", e.Period.Display, "", "实际申报日（未申报则为查询日）减去源申报截止日", float64(e.OverdueDays), e.Source))
			}
			if e.Kind != filingNotDue && len(d.Focus) < 3 {
				d.focus(e.ID, "说明具体税费种、所属期、事项性质、可得期限/日期和当前状态。未知不写逾期；更正不等于逾期；缺期限不编造逾期天数。", refs...)
			}
		}
		d.Constraints = []string{"未到申报期限的未申报记录不属于逾期。未取得应申报清册不得认定无漏报。", "滞纳金缴款与申报异常分别核对；历史滞纳金缴款不能证明当前欠税或对应本金已结清。", "风险等级引用规则结果；正文讲清具体事件及触发原因，不只复述异常记录数量。"}
		result = append(result, d)
	}
	return result
}

func paymentLateFeeEvidence(rows []map[string]any, amount float64, periods []string) string {
	var details []string
	for _, row := range rows {
		period, _ := taxPeriodFromRecord(row, false)
		v, known := mapNumber(row, "lateFeeAmount", "sjje", "amount")
		details = appendUnique(details, fmt.Sprintf("%s，所属期%s，滞纳金%s元，缴款日期%s", textOr(mapText(row, "zsxmMc", "taxType", "taxName"), "税费种未取得"), textOr(period.Display, "未取得"), optionalMoney(v, known), textOr(normalizeReportDate(mapText(row, "jkrq", "paymentDate", "paidAt"), false), "未取得")))
	}
	return fmt.Sprintf("%s。共%d笔滞纳金缴款，合计%s元；按已发生逾期缴款事项判级，历史缴款不等于当前欠税，对应本金处理状态须另行核对。", strings.Join(details, "；"), len(rows), moneyText(amount))
}
