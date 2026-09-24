package report

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var citAdditiveFormula = regexp.MustCompile(`\(([0-9]+(?:[+\-][0-9]+)+)\)`)
var citFormulaTerm = regexp.MustCompile(`[+\-]?[0-9]+`)

type annualCITBridge struct {
	profit, taxable TaxFieldFact
	adjustment      float64
	refs            []string
}

type annualCITFormulaResult struct {
	target  TaxFieldFact
	value   float64
	formula string
}

// addAnnualCITReconciliation checks the additive formulas printed on the actual
// annual main form. It does not hardcode line numbers from a particular company
// or assume an absent adjustment is zero. Source versions are never mixed.
func addAnnualCITReconciliation(d *SectionAnalysisData, fields []TaxFieldFact) []annualCITBridge {
	var bridges []annualCITBridge
	groups := map[string][]TaxFieldFact{}
	for _, f := range fields {
		if f.Module != "cit_annual_returns" || !isAnnualCITMainForm(f) || canonicalFormLabel(f.ColumnLabel) != "金额" {
			continue
		}
		key := strings.Join([]string{f.RecordID, f.Version, f.FiledAt, f.Period.Start, f.Period.End, f.FormCode, strconv.Itoa(f.Table)}, "|")
		groups[key] = append(groups[key], f)
	}
	for _, key := range sortedMapKeys(groups) {
		rows := groups[key]
		byLine := map[string]TaxFieldFact{}
		ambiguous := map[string]bool{}
		for _, f := range rows {
			if f.Line == "" {
				continue
			}
			if prior, exists := byLine[f.Line]; exists && (prior.Value != f.Value || prior.Status != f.Status) {
				ambiguous[f.Line] = true
			}
			byLine[f.Line] = f
		}
		var focus []string
		formulas := map[string]annualCITFormulaResult{}
		checked, closed := 0, true
		for _, target := range rows {
			label := normalizeFinancialMetricLabel(target.RowLabel)
			if label != "纳税调整后所得" && label != "应纳税所得额" {
				continue
			}
			prefix := "cit-bridge." + analysisToken(key+target.Line)
			normalized := strings.NewReplacer("－", "-", "−", "-").Replace(norm.NFKC.String(target.RowLabel))
			formula := citAdditiveFormula.FindStringSubmatch(normalized)
			if len(formula) != 2 || target.Status != "present" || ambiguous[target.Line] {
				closed = false
				focus = append(focus, d.text(prefix+".missing", label+"勾稽范围", "coverage", "未取得完整的主表行次公式或结果金额，尚不能计算调节后差额", "report-model.json/taxFields[id="+target.ID+"]"))
				continue
			}
			value, known := 0.0, true
			var inputs []CalculationInput
			var refs []string
			for _, term := range citFormulaTerm.FindAllString(formula[1], -1) {
				line := strings.TrimLeft(term, "+-")
				f, exists := byLine[line]
				if !exists || f.Status != "present" || ambiguous[line] {
					known = false
					continue
				}
				sign := 1.0
				if strings.HasPrefix(term, "-") {
					sign = -1
				}
				value += sign * f.Value
				inputs = append(inputs, taxCalculationInput(f))
				refs = append(refs, d.number(prefix+".line-"+line, f.RowLabel, "adjustment-source", "元", f.Period.Display, "", "同一年度主表原值，按主表公式加减", f.Value, "report-model.json/taxFields[id="+f.ID+"]"))
			}
			if !known {
				closed = false
				focus = append(focus, d.text(prefix+".missing", label+"勾稽范围", "coverage", "公式引用的部分调节行金额未取得，不补零，不认定已闭合", refs...))
				continue
			}
			checked++
			formulas[label] = annualCITFormulaResult{target: target, value: value, formula: formula[1]}
			expected := d.number(prefix+".calculated", label+"按主表公式计算金额", "calculated", "元", target.Period.Display, "", "主表行次 "+formula[1], value, refs...)
			d.Metrics[len(d.Metrics)-1].Inputs = inputs
			actual := d.number(prefix+".declared", label+"申报金额", "amount", "元", target.Period.Display, "", "同一主表结果行", target.Value, "report-model.json/taxFields[id="+target.ID+"]")
			residual := d.number(prefix+".residual", label+"调节后剩余差额", "difference", "元", target.Period.Display, "", "按主表公式计算金额−主表申报结果", value-target.Value, expected, actual)
			closed = closed && math.Abs(value-target.Value) < invoiceAggregationTolerance
			focus = append(focus, refs...)
			focus = append(focus, expected, actual, residual)
		}
		if len(focus) == 0 {
			continue
		}
		status := "部分主表勾稽尚未闭合或字段不全，应分别说明已核对部分与剩余差额"
		if checked == 2 && closed {
			status = "利润总额经主表各调节项目至应纳税所得额的申报金额勾稽相符，剩余差额0；相关备查材料尚未核验"
		}
		focus = append(focus, d.text("cit-bridge."+analysisToken(key)+".status", "年度主表调节状态", "state", status, focus...))
		d.focus("cit-bridge-"+analysisToken(key), fmt.Sprintf("先解释%s主表已经取得的非零调整、亏损弥补及两阶段剩余差额。申报金额勾稽与材料核验分开，不能因未取得辅助账而否认表内已闭合关系；也不能用主表闭合消除尚未调节的财报与申报利润差异。", rows[0].Period.Display), focus...)
		first, haveFirst := formulas["纳税调整后所得"]
		last, haveLast := formulas["应纳税所得额"]
		if checked != 2 || !haveFirst || !haveLast || citFormulaCoefficient(last.formula, first.target.Line) != 1 {
			continue
		}
		for _, line := range sortedMapKeys(byLine) {
			profit := byLine[line]
			if normalizeFinancialMetricLabel(profit.RowLabel) != "利润总额" || profit.Status != "present" || ambiguous[profit.Line] || citFormulaCoefficient(first.formula, profit.Line) != 1 {
				continue
			}
			// Carry only adjustment terms through the two-stage formula. Do not
			// substitute taxable minus profit, which would hide a source mismatch.
			bridges = append(bridges, annualCITBridge{profit: profit, taxable: last.target,
				adjustment: first.value - profit.Value + last.value - first.target.Value, refs: append([]string(nil), focus...)})
		}
	}
	return bridges
}

func citFormulaCoefficient(formula, line string) int {
	coefficient := 0
	for _, term := range citFormulaTerm.FindAllString(formula, -1) {
		if strings.TrimLeft(term, "+-") == line {
			if strings.HasPrefix(term, "-") {
				coefficient--
			} else {
				coefficient++
			}
		}
	}
	return coefficient
}

// addAnnualFinancialReconciliation connects the existing form bridge to the
// exact annual financial comparison. Source identity and period must match;
// an internally balanced return does not erase a financial/return mismatch.
func addAnnualFinancialReconciliation(d *SectionAnalysisData, bridges []annualCITBridge) {
	for _, bridge := range bridges {
		key := "financial-comparison.profit-annual-" + analysisToken(bridge.taxable.RowLabel) + "." + analysisToken(bridge.taxable.Period.Display)
		index := -1
		var left, right AnalysisMetric
		for i, m := range d.Metrics {
			switch m.ID {
			case metricID(d.SectionID, key+".left"):
				left = m
			case metricID(d.SectionID, key+".right"):
				right = m
			case metricID(d.SectionID, key+".reconciliation"):
				index = i
			}
		}
		if index < 0 || left.Status != "present" || right.Status != "present" {
			continue
		}
		matched := false
		for _, input := range right.Inputs {
			matched = matched || input.TaxFieldID == bridge.taxable.ID
		}
		if !matched {
			continue
		}
		prefix := "cit-annual-financial-bridge." + analysisToken(bridge.taxable.ID)
		adjustment := d.number(prefix+".adjustment", "年度主表调节净额", "adjustment", "元", bridge.taxable.Period.Display, "", "两阶段主表公式中除利润总额之外的调节项有符号合计", bridge.adjustment, bridge.refs...)
		remaining := math.Round((left.Value+bridge.adjustment-right.Value)*100) / 100
		residual := d.number(prefix+".remaining", "年度财报利润与应纳税所得额调节后剩余差额", "difference", "元", bridge.taxable.Period.Display, "", "年度财报利润总额＋同年度主表调节净额−应纳税所得额", remaining, left.ID, right.ID, adjustment)
		profitDifference := d.number(prefix+".profit-difference", "年度财报与申报利润总额差额", "difference", "元", bridge.taxable.Period.Display, "", "年度财报利润总额−同年度主表利润总额", left.Value-bridge.profit.Value, left.ID, "report-model.json/taxFields[id="+bridge.profit.ID+"]")
		state := "年度主表调节项目已对应，调节后仍有剩余差额；财报与申报利润总额差额另列，相关业务凭证尚未核验"
		if math.Abs(remaining) < invoiceAggregationTolerance {
			state = "年度财报利润与应纳税所得额的原差额经同年度主表调节已在金额上对应，剩余差额为零；财报与申报利润总额仍按独立比较结果核对，业务凭证尚未核验"
		}
		d.Metrics[index].Display = state
		d.Metrics[index].SourceRefs = append(d.Metrics[index].SourceRefs, adjustment, residual, profitDifference)
		d.focus(prefix, "沿用该比较项最终调节状态，先解释已对应金额，再说明剩余差额；凭证未核验不等于金额未对应，数值相等不证明数据同源。", d.Metrics[index].ID, adjustment, residual, profitDifference)
	}
}
