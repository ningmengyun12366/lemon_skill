package report

import "strings"

// R&D figures are super-deduction declarations, not actual R&D expenditure.
// Increments require adjacent same-year cumulative forms with one selected
// value per quarter. Annual filings and ambiguous versions cannot be subtracted.
func addResearchCumulativeAnalysis(d *SectionAnalysisData, model Model) {
	facts := citResearchAmountFields(model.TaxFields)
	byQuarter := map[string][]TaxFieldFact{}
	for _, f := range facts {
		if f.Module == "cit_quarterly_returns" {
			byQuarter[normalizeQuarter(f.Period.End)] = append(byQuarter[normalizeQuarter(f.Period.End)], f)
		}
	}
	var refs []string
	for _, f := range facts {
		prefix := "research-basis." + analysisToken(f.ID)
		source := "report-model.json/taxFields[id=" + f.ID + "]"
		basis := "截至季度末累计申报加计扣除额，不能当作当季实际研发投入"
		if f.Module == "cit_annual_returns" {
			basis = "年度汇算加计扣除额，单列，不与季度累计数合计"
		}
		r := d.text(prefix+".basis", "研发加计扣除期间口径", "basis", f.Period.Display+"；"+basis, source)
		refs = append(refs, r)
		if f.Module != "cit_quarterly_returns" || !strings.Contains(canonicalFormLabel(f.ColumnLabel), "本年累计") {
			continue
		}
		quarter := normalizeQuarter(f.Period.End)
		if len(quarter) < 6 || len(byQuarter[quarter]) != 1 {
			continue
		}
		q := quarterNumber(quarter)
		if q <= 1 {
			continue
		}
		previousQuarter := quarter[:4] + "Q" + string(rune('0'+q-1))
		previous := byQuarter[previousQuarter]
		if len(previous) != 1 || previous[0].FormCode != f.FormCode || !strings.Contains(canonicalFormLabel(previous[0].ColumnLabel), "本年累计") ||
			f.Version == "" || previous[0].Version == "" || f.FiledAt == "" || previous[0].FiledAt == "" {
			d.text(prefix+".increment-status", "累计申报增量状态", "state", "未取得同年相邻前季唯一同口径累计值，或申报版本/日期不明，不计算本季新增申报额", source)
			continue
		}
		p := previous[0]
		id := d.number(prefix+".increment", "较前季累计申报增加额", "cumulative-increment", "元", quarter, previousQuarter, "本季末累计申报额−同年前季末累计申报额；可能包含追溯调整，不代表当季实际投入", f.Value-p.Value, source, "report-model.json/taxFields[id="+p.ID+"]")
		d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(f), taxCalculationInput(p)}
		refs = append(refs, id)
	}
	if len(refs) > 0 {
		d.focus("research-cumulative-basis", "说明金额为季末累计申报或年度汇算；相邻季末累计相同应表述为累计申报额未变，不能说两季实际研发投入相同。新增申报额也可能包括以前季度调整，年度与季度不得相加。", refs...)
	}
}
