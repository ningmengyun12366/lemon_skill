package report

import (
	"strings"
	"time"
)

const (
	financialStandardEnterprise = "企业会计准则"
	financialStandardSmall      = "小企业会计准则"
	financialStandardOther      = "其他准则（按当期口径）"
	financialPeriodBasis        = "企业会计准则流量字段按年内累计还原当期：首期直接取累计，后期减同年相邻月/季累计；小企业及其他准则的本期字段按所属期直接取值。累计原值另存，资产负债表余额不差分，报送日期不改变所属期。"
)

// financialAccountingStandard uses the statement's identity, never a company
// name or the ambiguous current-amount header. Other standards follow the
// explicitly agreed current-period default; explicit YTD columns stay YTD.
func financialAccountingStandard(record map[string]any) string {
	code := mapText(record, "zlbsxlDm", "YzpzzlDm")
	name := mapText(record, "zlbsxlmc", "YzpzzlMc")
	if code == "ZL1001003" || strings.Contains(name, financialStandardSmall) {
		return financialStandardSmall
	}
	if code == "ZL1001001" || strings.Contains(name, financialStandardEnterprise) {
		return financialStandardEnterprise
	}
	previews, _ := mapValue(record, financialPreviewField)
	if list, ok := previews.([]any); ok {
		for _, value := range list {
			preview, ok := value.(map[string]any)
			if !ok {
				continue
			}
			label := mapText(preview, financialPreviewLabelField, "name")
			if strings.Contains(label, financialStandardSmall) {
				return financialStandardSmall
			}
			if strings.Contains(label, financialStandardEnterprise) {
				return financialStandardEnterprise
			}
		}
	}
	return financialStandardOther
}

type financialFlowTarget struct {
	metric string
	value  *float64
	known  *bool
}

func financialFlowTargets(f *financialFact) []financialFlowTarget {
	targets := []financialFlowTarget{
		{financialMetricRevenue, &f.Revenue, &f.HasRevenue},
		{financialMetricCost, &f.Cost, &f.HasCost},
		{financialMetricProfit, &f.Profit, &f.HasProfit},
		{financialMetricNetProfit, &f.NetProfit, &f.HasNetProfit},
		{financialMetricSalesExpense, &f.SalesExpense, &f.HasSalesExpense},
		{financialMetricAdminExpense, &f.ManagementExpense, &f.HasManagementExpense},
		{financialMetricRDExpense, &f.ResearchExpense, &f.HasResearchExpense},
		{financialMetricFinanceCost, &f.FinanceExpense, &f.HasFinanceExpense},
		{financialMetricOperatingCF, &f.OperatingCashFlow, &f.HasOperatingCashFlow},
	}
	if f.ProfitComponents == nil {
		f.ProfitComponents = map[string]*knownValue{}
	}
	for _, component := range financialProfitComponents {
		v := f.ProfitComponents[component.label]
		if v == nil {
			v = &knownValue{}
			f.ProfitComponents[component.label] = v
		}
		targets = append(targets, financialFlowTarget{component.label, &v.value, &v.known})
	}
	return targets
}

func financialFlowMetric(metric string) bool {
	return financialMetricAllowedInForm(metric, financialIncomeLabel) || metric == financialMetricOperatingCF
}

func setFinancialCumulative(f *financialFact, metric string, value float64) {
	if f.CumulativeFlows == nil {
		f.CumulativeFlows = map[string]knownValue{}
	}
	if !f.CumulativeFlows[metric].known {
		f.CumulativeFlows[metric] = knownValue{value, true}
	}
	if metric == financialMetricRevenue {
		setFinancialValue(&f.CumulativeRevenue, &f.HasCumulativeRevenue, value)
	}
}

// normalizeFinancialFlowAmounts snapshots all source values before calculating
// any differences. References may supply the preceding period but are filtered
// out by the caller. Missing predecessors never become zero or annual fallbacks.
func normalizeFinancialFlowAmounts(facts map[string]*financialFact) {
	for _, f := range facts {
		f.SourceFlows = map[string]knownValue{}
		if f.HasCumulativeRevenue {
			setFinancialCumulative(f, financialMetricRevenue, f.CumulativeRevenue)
		}
		for _, field := range financialFlowTargets(f) {
			f.SourceFlows[field.metric] = knownValue{*field.value, *field.known}
			if f.AccountingStandard == financialStandardEnterprise && *field.known {
				setFinancialCumulative(f, field.metric, *field.value)
			}
		}
	}
	for _, f := range facts {
		if f.AccountingStandard != financialStandardEnterprise {
			continue
		}
		first := len(f.TaxPeriod.Start) == 10 && f.TaxPeriod.Start[5:] == "01-01"
		var previous *financialFact
		if start, err := time.Parse("2006-01-02", f.TaxPeriod.Start); err == nil && !first {
			priorEnd := start.AddDate(0, 0, -1).Format("2006-01-02")
			for _, candidate := range facts {
				if candidate.AccountingStandard == f.AccountingStandard && candidate.TaxPeriod.Cycle == f.TaxPeriod.Cycle &&
					candidate.TaxPeriod.End == priorEnd && candidate.TaxPeriod.Start[:4] == f.TaxPeriod.Start[:4] {
					previous = candidate
					break
				}
			}
		}
		if previous != nil {
			f.PreviousSourceRecordID = previous.SourceRecordID
			f.PreviousCumulativeFlows = previous.CumulativeFlows
		}
		for _, field := range financialFlowTargets(f) {
			cumulative := f.CumulativeFlows[field.metric]
			*field.value, *field.known = 0, false
			if !cumulative.known {
				continue
			}
			if first {
				*field.value, *field.known = cumulative.value, true
			} else if prior := f.PreviousCumulativeFlows[field.metric]; prior.known {
				priorValue := prior.value
				if financialProfitComponent(field.metric) {
					currentSign, previousSign := f.ProfitComponentSigns[field.metric], previous.ProfitComponentSigns[field.metric]
					if currentSign == 0 || previousSign == 0 {
						// A later comparable period must not inherit a subtraction
						// made from an earlier row with an unknown presentation sign.
						continue
					}
					priorValue *= previousSign / currentSign
				}
				*field.value, *field.known = cumulative.value-priorValue, true
			}
		}
	}
}

// addFinancialNormalizationFacts supplies the original YTD, previous YTD and
// computed current amount to the agent, without changing original tax fields.
func addFinancialNormalizationFacts(d *SectionAnalysisData, facts []financialFact, metrics ...string) {
	for _, f := range facts {
		if f.AccountingStandard != financialStandardEnterprise {
			continue
		}
		for _, field := range financialFlowTargets(&f) {
			if !financialStringSliceContains(metrics, field.metric) {
				continue
			}
			cumulative := f.CumulativeFlows[field.metric]
			if !cumulative.known {
				continue
			}
			key := "financial-period." + analysisToken(f.Period+field.metric)
			source := "income_statements/records[id=" + f.SourceRecordID + "]/" + field.metric
			refs := []string{d.number(key+".ytd", field.metric+"原年内累计", "source-amount", "元", f.Period, "", "企业会计准则税局返回的年内累计原值", cumulative.value, source)}
			if previous := f.PreviousCumulativeFlows[field.metric]; previous.known {
				refs = append(refs, d.number(key+".previous-ytd", field.metric+"相邻上期累计", "source-amount", "元", f.Period, "", "同年同周期相邻上期累计", previous.value, "income_statements/records[id="+f.PreviousSourceRecordID+"]/"+field.metric))
			}
			if *field.known {
				d.number(key+".current", field.metric+"还原当期金额", "amount", "元", f.Period, "", financialPeriodBasis, *field.value, refs...)
			} else {
				id := d.text(key+".missing-previous", field.metric+"当期金额状态", "coverage", f.Period+"缺少同年相邻上期累计，不能还原当期金额；本期累计原值仍可用于同截止日累计比较", refs...)
				d.focus(key, "指出缺少相邻累计的期间及影响，不用年报、去年同期或零代替缺失值。", id)
			}
		}
	}
}
