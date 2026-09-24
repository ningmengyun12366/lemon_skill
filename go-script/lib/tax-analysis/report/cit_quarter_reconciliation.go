package report

import (
	"math"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type citQuarterBridge struct {
	period, filedAt string
	profit, actual  TaxFieldFact
	terms           map[string]TaxFieldFact
	signs           map[string]float64
	known           bool
}

// quarterlyCITBridges reads the formula printed on each version of A200000.
// It keeps parent adjustment rows only; detail rows never get added twice.
func quarterlyCITBridges(fields []TaxFieldFact) map[string]citQuarterBridge {
	groups := map[string][]TaxFieldFact{}
	for _, f := range fields {
		if f.Module != "cit_quarterly_returns" || !containsAny(f.FormCode+f.FormName, "A200000") || !declarationLabelMatches(f.ColumnLabel, []string{"本年累计", "本年累计金额", "金额"}) {
			continue
		}
		key := f.RecordID + "|" + f.Version + "|" + f.FiledAt + "|" + f.FormCode + "|" + f.Period.End
		groups[key] = append(groups[key], f)
	}
	result := map[string]citQuarterBridge{}
	for _, key := range sortedMapKeys(groups) {
		rows := groups[key]
		b := citQuarterBridge{period: normalizeQuarter(rows[0].Period.End), filedAt: rows[0].FiledAt, terms: map[string]TaxFieldFact{}, signs: map[string]float64{}}
		byLine, ambiguous := map[string]TaxFieldFact{}, map[string]bool{}
		for _, f := range rows {
			if prior, ok := byLine[f.Line]; ok && (prior.Value != f.Value || prior.Status != f.Status) {
				ambiguous[f.Line] = true
			}
			byLine[f.Line] = f
			label := canonicalFormLabel(f.RowLabel)
			if strings.HasPrefix(label, "利润总额") {
				b.profit = f
			}
			if strings.HasPrefix(label, "实际利润额") {
				b.actual = f
			}
		}
		formula := citAdditiveFormula.FindStringSubmatch(strings.NewReplacer("－", "-", "−", "-").Replace(norm.NFKC.String(b.actual.RowLabel)))
		if len(formula) != 2 || b.actual.Status != "present" || b.profit.Status != "present" {
			continue
		}
		b.known = !ambiguous[b.actual.Line] && !ambiguous[b.profit.Line]
		hasProfit := false
		for _, term := range citFormulaTerm.FindAllString(formula[1], -1) {
			line := strings.TrimLeft(term, "+-")
			f, ok := byLine[line]
			if !ok || f.Status != "present" || ambiguous[line] {
				b.known = false
				continue
			}
			sign := 1.0
			if strings.HasPrefix(term, "-") {
				sign = -1
			}
			if line == b.profit.Line && sign == 1 {
				hasProfit = true
				continue
			}
			label := normalizeFinancialMetricLabel(f.RowLabel)
			if _, exists := b.terms[label]; exists {
				b.known = false
			}
			b.terms[label], b.signs[label] = f, sign
		}
		b.known = b.known && hasProfit
		prior, exists := result[b.period]
		if !exists || b.filedAt > prior.filedAt {
			result[b.period] = b
		} else if b.filedAt == prior.filedAt && (b.actual.Value != prior.actual.Value || b.profit.Value != prior.profit.Value) {
			prior.known = false
			result[b.period] = prior
		}
	}
	return result
}

// Restore each formula operand with its adjacent same-year predecessor before
// explaining a single-quarter gap. Missing baselines never become zero.
func addQuarterlyCITReconciliation(d *SectionAnalysisData, ctx *reportContext) {
	bridges := quarterlyCITBridges(ctx.taxFields)
	for _, current := range ctx.cit {
		period := current.Period
		b, ok := bridges[period]
		if !ok || !b.known {
			continue
		}
		q := quarterNumber(period)
		previous := bridges[period[:4]+"Q"+string(rune('0'+q-1))]
		if q > 1 && !previous.known {
			continue
		}
		prefix := "cit-quarter-bridge." + analysisToken(period)
		var refs []string
		adjustment, complete := 0.0, true
		for _, label := range sortedMapKeys(b.terms) {
			f, value := b.terms[label], b.terms[label].Value
			sources := []string{"report-model.json/taxFields[id=" + f.ID + "]"}
			if q > 1 {
				p, exists := previous.terms[label]
				if !exists || previous.signs[label] != b.signs[label] {
					complete = false
					break
				}
				value -= p.Value
				sources = append(sources, "report-model.json/taxFields[id="+p.ID+"]")
			}
			adjustment += value * b.signs[label]
			if value != 0 {
				id := d.number(prefix+".term."+analysisToken(label), f.RowLabel+"（还原单季）", "adjustment", "元", period, "", "本季累计−同年相邻上季累计；首季直接取累计", value, sources...)
				refs = append(refs, id)
			}
		}
		if q > 1 && len(b.terms) != len(previous.terms) {
			complete = false
		}
		if !complete {
			continue
		}
		profit, actual := b.profit.Value, b.actual.Value
		if q > 1 {
			profit -= previous.profit.Value
			actual -= previous.actual.Value
		}
		profitSources := []string{"report-model.json/taxFields[id=" + b.profit.ID + "]"}
		actualSources := []string{"report-model.json/taxFields[id=" + b.actual.ID + "]"}
		if q > 1 {
			profitSources = append(profitSources, "report-model.json/taxFields[id="+previous.profit.ID+"]")
			actualSources = append(actualSources, "report-model.json/taxFields[id="+previous.actual.ID+"]")
		}
		p := d.number(prefix+".profit", "预缴利润总额还原单季", "amount", "元", period, "", "同年相邻累计差分", profit, profitSources...)
		a := d.number(prefix+".actual", "预缴累计实际利润额的单季变动", "amount", "元", period, "", "同年相邻累计差分，仅用于单季对比，不是本期申报计税余额", actual, actualSources...)
		gap := d.number(prefix+".difference", "预缴实际利润与利润总额差额", "difference", "元", period, "", "实际利润−利润总额", actual-profit, p, a)
		explained := d.number(prefix+".explained", "申报表内调整已解释金额", "adjustment", "元", period, "", "各父级调节行还原单季后按主表公式符号相加", adjustment, refs...)
		residual := actual - profit - adjustment
		r := d.number(prefix+".residual", "预缴利润差额扣除表内调节的剩余差额", "difference", "元", period, "", "原差额−申报表内已解释金额", residual, gap, explained)
		refs = append(refs, p, a, gap, explained, r)
		financial, financialKnown := financialValueForQuarter(ctx.financial, period, func(f financialFact) (float64, bool) { return f.Profit, f.HasProfit })
		financialResidual := ""
		if financialKnown {
			financialResidual = d.number(prefix+".financial-residual", "财报利润与实际利润扣除表内调节的剩余差额", "difference", "元", period, "", "财报单季利润−实际利润+主表调整净额；保留财报与预缴利润的差额", financial-actual+adjustment, explained, a, "income_statements/"+period)
		}
		state := "已建立同所属期主表调节关系；备查凭证未核验，申报勾稽不等于业务资格已验证"
		if math.Abs(residual) >= invoiceAggregationTolerance {
			state = "同所属期主表调节后仍有剩余差额，尚未完全勾稽；已取得调节项目与剩余差额分别列示"
		}
		d.text(prefix+".status", "季度预缴表内调节状态", "state", state, refs...)
		d.focus(prefix, "解释同季财报与预缴累计差分的金额关系，区分表内调节和剩余差额。实际利润额的单季变动不是本期申报计税余额；解释应补退税额须用原申报累计计税关系，不能因差分为正推断存在未纳税利润。", refs...)
		// Replace the generic uncertainty only for the exact comparison/period.
		for i := range d.Metrics {
			m := &d.Metrics[i]
			key := "financial-comparison.filing-actual-profit." + analysisToken(period) + ".reconciliation"
			if m.ID == metricID(d.SectionID, key) {
				m.Display, m.SourceRefs = state, append(m.SourceRefs, r)
			}
			financialKey := "financial-comparison.financial-actual-profit." + analysisToken(period) + ".reconciliation"
			if financialResidual != "" && m.ID == metricID(d.SectionID, financialKey) {
				m.Display = "预缴利润至实际利润的表内调节已计算；财报与实际利润的差额扣除该部分后剩余金额另列，业务凭证未核验"
				m.SourceRefs = append(m.SourceRefs, financialResidual)
			}
		}
	}
}
