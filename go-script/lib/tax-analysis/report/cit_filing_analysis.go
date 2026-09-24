package report

import "strings"

// analysisCITFilingBasis gives the tax-contribution chapter the filed cumulative
// basis, separate from the single-quarter comparisons used in the profit chapter.
// It reuses effective declarations and printed formulas; missing operands stay unknown.
func analysisCITFilingBasis(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "cit.quarter-contribution"}
	records, _ := namedCollectionMapsWithPresence(ctx.modules["cit_quarterly_returns"].Raw, "effectiveRecords")
	for index, record := range records {
		period, ok := taxPeriodFromRecord(record, false)
		if !ok || (ctx.task.StartTime != "" && period.Start < ctx.task.StartTime) ||
			(ctx.task.EndTime != "" && period.End > ctx.task.EndTime) {
			continue
		}
		fields := citRecordFields(ctx, "cit_quarterly_returns", record, index)
		prefix := "cit-filing." + analysisToken(period.Start+"|"+period.End)
		var refs []string
		add := func(key, name string, field TaxFieldFact) string {
			id := d.number(prefix+key, name, "amount", "元", period.Display, "", "同一有效预缴申报表原值；"+field.RowLabel+" · "+field.ColumnLabel+"；不作相邻期差分", field.Value, "report-model.json/taxFields[id="+field.ID+"]")
			d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(field)}
			refs = append(refs, id)
			return id
		}
		for _, metric := range []struct {
			key, name string
			labels    []string
		}{
			{".profit", "原申报累计利润总额", []string{"利润总额"}},
			{".actual", "原申报累计实际利润额", []string{"实际利润额"}},
			{".tax-before-relief", "原申报应纳所得税额", []string{"应纳所得税额"}},
			{".relief", "原申报减免所得税额", []string{"减免所得税额"}},
			{".credit", "原申报抵免所得税额", []string{"抵免所得税额"}},
			{".prepaid", "原申报已预缴所得税额", []string{"本年累计已缴纳所得税额", "本年实际已缴纳所得税额", "本年已预缴所得税额", "本年累计实际已缴纳所得税额"}},
			{".payable", "原申报本期应补退所得税额", []string{"实际应补（退）所得税额", "本期应补（退）所得税额"}},
		} {
			if field, found := selectCITAmount(fields, metric.labels, false); found && field.Status == "present" {
				add(metric.key, metric.name, field)
			}
		}
		bridge, exists := quarterlyCITBridges(fields)[normalizeQuarter(period.End)]
		if exists && bridge.known {
			adjustment := 0.0
			var termRefs []string
			for _, label := range sortedMapKeys(bridge.terms) {
				field := bridge.terms[label]
				adjustment += field.Value * bridge.signs[label]
				if field.Value != 0 {
					termRefs = append(termRefs, add(".term."+analysisToken(label), "原申报累计调节项："+field.RowLabel, field))
				}
			}
			inputs := []CalculationInput{taxCalculationInput(bridge.profit), taxCalculationInput(bridge.actual)}
			for _, label := range sortedMapKeys(bridge.terms) {
				inputs = append(inputs, taxCalculationInput(bridge.terms[label]))
			}
			termRefs = append(termRefs, metricID(d.SectionID, prefix+".profit"), metricID(d.SectionID, prefix+".actual"))
			id := d.number(prefix+".residual", "原申报累计利润调节后剩余差额", "difference", "元", period.Display, "", "原申报累计实际利润−累计利润总额−按该表公式符号计算的累计调节净额", bridge.actual.Value-bridge.profit.Value-adjustment, termRefs...)
			d.Metrics[len(d.Metrics)-1].Inputs = inputs
			refs = append(refs, id)
		} else if len(refs) > 0 {
			refs = append(refs, d.text(prefix+".coverage", "累计计税关系数据范围", "coverage", "已列原申报可用字段；主表累计利润调节公式或操作数不完整，不能补零或认定已完全对应", refs...))
		}
		if len(refs) > 0 {
			d.focus(prefix, "解释本所属期申报应补退税额时，使用这张申报表的累计利润、实际取得的调节项及已预缴等原值；单季利润转正不证明累计计税利润转正，累计实际利润的差分不是本期应税余额。只说明已取得的表内对应关系，不自动确认业务资格。", refs...)
		}
	}
	d.Constraints = []string{strings.Join([]string{
		"单季财报/收入比较与申报税额解释分开：前者按当期，后者按原申报累计计税关系。",
		"本期应补退所得税额直接取申报原字段，不对其再差分；零税额本身不产生风险。",
		"没有返回的调节、减免或已预缴字段保持未知，不推测税率和差额成因。",
	}, "")}
	return d
}
