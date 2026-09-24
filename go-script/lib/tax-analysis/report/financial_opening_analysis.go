package report

import (
	"fmt"
	"sort"
	"strings"
)

// addFinancialOpeningAnalysis takes both ends from one source balance-sheet
// version. Explicit year-opening columns, not the first quarter-end, establish
// the annual baseline. A later annual filing and Q4 filing remain distinct.
func addFinancialOpeningAnalysis(sections map[string]*SectionAnalysisData, fields []TaxFieldFact) {
	groups := map[string][]TaxFieldFact{}
	for _, f := range fields {
		if f.Module != "balance_sheets" || len(f.Period.End) < 4 {
			continue
		}
		switch normalizeFinancialMetricLabel(f.RowLabel) {
		case financialMetricAssets, financialMetricLiabilities, financialMetricReceivables, financialMetricInventory, financialMetricContractDebt:
		default:
			continue
		}
		key := strings.Join([]string{f.RecordID, f.Version, f.FiledAt, f.FormCode, f.Period.Start, f.Period.End}, "|")
		groups[key] = append(groups[key], f)
	}
	byYear := map[string][]string{}
	for key, rows := range groups {
		byYear[rows[0].Period.End[:4]] = append(byYear[rows[0].Period.End[:4]], key)
	}
	for _, year := range sortedMapKeys(byYear) {
		keys := byYear[year]
		sort.Slice(keys, func(i, j int) bool {
			a, b := groups[keys[i]][0], groups[keys[j]][0]
			if (a.Period.Cycle == taxCycleAnnual) != (b.Period.Cycle == taxCycleAnnual) {
				return a.Period.Cycle == taxCycleAnnual
			}
			if a.Period.End != b.Period.End {
				return a.Period.End > b.Period.End
			}
			if a.FiledAt != b.FiledAt {
				return a.FiledAt > b.FiledAt
			}
			return keys[i] < keys[j]
		})
		rows := groups[keys[0]]
		source := rows[0]
		if len(keys) > 1 {
			other := groups[keys[1]][0]
			if other.Period.Cycle == source.Period.Cycle && other.Period.End == source.Period.End && other.FiledAt == source.FiledAt {
				for _, section := range []string{"financial.debt-asset", "financial.working-capital", "financial.summary"} {
					sections[section].Constraints = append(sections[section].Constraints, year+"同一属期存在无法唯一选择的资产负债表版本，不拼接年初年末结论。")
				}
				continue
			}
		}
		values := map[string]TaxFieldFact{}
		ambiguous := map[string]bool{}
		for _, f := range rows {
			col := canonicalFormLabel(f.ColumnLabel)
			boundary := ""
			if financialStringSliceContains([]string{"年初余额", "上年年末余额", "年初数"}, col) {
				boundary = "opening"
			}
			if financialStringSliceContains([]string{"期末余额", "期末数"}, col) {
				boundary = "closing"
			}
			if boundary == "" {
				continue
			}
			key := normalizeFinancialMetricLabel(f.RowLabel) + "|" + boundary
			if previous, ok := values[key]; ok && (previous.Value != f.Value || previous.Status != f.Status) {
				ambiguous[key] = true
			}
			values[key] = f
		}
		for _, m := range []struct{ key, label, section string }{
			{financialMetricReceivables, "应收账款", "financial.working-capital"},
			{financialMetricInventory, "存货", "financial.working-capital"},
			{financialMetricContractDebt, "合同负债", "financial.working-capital"},
			{"leverage", "资产负债率", "financial.debt-asset"},
		} {
			for _, section := range []string{m.section, "financial.summary"} {
				d := sections[section]
				prefix := "financial-opening." + analysisToken(year+m.key)
				var refs []string
				var endpoints []float64
				for _, boundary := range []string{"opening", "closing"} {
					labels := []string{m.key}
					if m.key == "leverage" {
						labels = []string{financialMetricLiabilities, financialMetricAssets}
					}
					var inputs []CalculationInput
					for _, label := range labels {
						key := label + "|" + boundary
						f, exists := values[key]
						if !exists || f.Status != "present" || ambiguous[key] {
							break
						}
						inputs = append(inputs, taxCalculationInput(f))
					}
					if len(inputs) != len(labels) {
						break
					}
					value, unit := inputs[0].Value, "元"
					if m.key == "leverage" {
						if inputs[1].Value == 0 {
							break
						}
						value, unit = value/inputs[1].Value*100, "%"
					}
					period := year + "年初"
					if boundary == "closing" {
						period = source.Period.End + "期末"
					}
					id := d.number(prefix+"."+boundary, m.label+period, "amount", unit, period, "", "同一资产负债表版本的明确年初或期末字段", value)
					d.Metrics[len(d.Metrics)-1].Inputs = inputs
					for _, input := range inputs {
						d.Metrics[len(d.Metrics)-1].SourceRefs = append(d.Metrics[len(d.Metrics)-1].SourceRefs, input.Source)
					}
					refs = append(refs, id)
					endpoints = append(endpoints, value)
				}
				if len(endpoints) != 2 {
					continue
				}
				unit := "元"
				if m.key == "leverage" {
					unit = "个百分点"
				}
				refs = append(refs, d.number(prefix+".change", m.label+"较年初变化", "difference", unit, source.Period.End, year+"-01-01", "同一报表版本期末值−明确年初值", endpoints[1]-endpoints[0], refs...))
				d.focus(prefix, fmt.Sprintf("说明%s从明确年初值到%s期末的变化及单位；所用报表为%s，报送日期%s。不能将第一季度末当年初，也不能混用年度报表与第四季度报表。", m.label, source.Period.End, source.Period.Display, source.FiledAt), refs...)
			}
		}
	}
}
