package report

import (
	"fmt"
	"math"
	"strings"
)

// Enterprise Income Tax Implementation Regulations, Article 43. Both caps
// apply; 40% is not a universal tax adjustment percentage.
const (
	entertainmentExpenseDeductionRate = 0.60
	entertainmentRevenueCapRate       = 0.005
)

func citAdjustmentNote(label string) string {
	switch {
	case strings.Contains(label, "业务招待费"):
		return "核对发生额与销售收入两项扣除限额及申报调增金额"
	case containsAny(label, "广告费", "业务宣传费"):
		return "结合本年限额、以前年度结转与本年结转明细核对，不把结转额当作当年新增支出"
	case containsAny(label, "罚金", "罚款", "滞纳金"):
		return "区分行政罚款、税收滞纳金与经营性违约支出；纳税调增不代表当前欠税"
	case canonicalFormLabel(label) == "合计":
		return "纳税调整汇总；具体项目分别分析"
	case strings.Contains(label, "其他"):
		return "本表未列具体组成，需对应调整明细核对，不能直接认定为无票支出"
	default:
		return "结合本项目账税差异及申报明细核对调整依据"
	}
}

// Same-version physical rows prevent amounts from different returns being mixed.
func citAdjustmentAnalysis(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "cit.annual"}
	rows := map[string][]TaxFieldFact{}
	for _, f := range ctx.taxFields {
		if f.Module == "cit_annual_returns" && (strings.Contains(f.FormCode, "A105000") || strings.Contains(f.FormName, "纳税调整项目明细表")) {
			key := f.RecordID + "|" + f.Version + "|" + f.FormCode + "|" + f.RowLabel
			rows[key] = append(rows[key], f)
		}
	}
	addCITAdjustmentReadingScope(&d, rows)
	for _, key := range sortedMapKeys(rows) {
		fields := rows[key]
		f := fields[0]
		values := map[string]TaxFieldFact{}
		for _, x := range fields {
			if x.Status == "present" {
				values[canonicalFormLabel(x.ColumnLabel)] = x
			}
		}
		if values["调增金额"].Value == 0 && values["调减金额"].Value == 0 || containsAny(f.RowLabel, "类调整项目", "合计") {
			continue
		}
		prefix := "adjustment-detail." + analysisToken(key)
		refs := []string{d.text(prefix+".category", "纳税调整项目", "object", f.RowLabel)}
		for _, column := range []string{"账载金额", "税收金额", "调增金额", "调减金额"} {
			if x, ok := values[column]; ok {
				refs = append(refs, d.number(prefix+"."+analysisToken(column), f.RowLabel+" · "+column, "amount", "元", f.Period.Display, "", "同一年度同一版本纳税调整表原值", x.Value, "report-model.json/taxFields[id="+x.ID+"]"))
			}
		}
		if strings.Contains(f.RowLabel, "业务招待费") {
			refs = append(refs, addEntertainmentCalculation(&d, ctx, values, prefix)...)
		}
		d.focus(prefix, citAdjustmentNote(f.RowLabel), refs...)
	}
	// Advertising carryforward rows keep their own year/period and stated rate.
	for _, f := range ctx.taxFields {
		if f.Module != "cit_annual_returns" || f.Status != "present" || !(strings.Contains(f.FormCode, "A105060") || strings.Contains(f.FormName, "广告费和业务宣传费跨年度纳税调整")) {
			continue
		}
		if !containsAny(f.RowLabel, "结转", "扣除限额", "扣除率", "计算扣除限额的") {
			continue
		}
		unit := "元"
		if strings.Contains(f.RowLabel, "率") {
			unit = "比例"
		}
		d.number("advertising."+analysisToken(f.ID), f.RowLabel, "adjustment-source", unit, f.Period.Display, "", "对应年度广告宣传费附表原值；行业适用比例沿用申报，不默认套统一税率", f.Value, "report-model.json/taxFields[id="+f.ID+"]")
	}
	d.Constraints = []string{"纳税调整项目按本年实际附表分析。存在调增不自动构成风险，申报金额相符不代表业务凭证已经核验。广告宣传费不得跨年合计结转余额或臆造行业扣除比例。"}
	return d
}

// Scope describes the rows behind the nonzero projection without treating an
// absent amount, a failed field or an inapplicable cell as an explicit zero.
func addCITAdjustmentReadingScope(d *SectionAnalysisData, rows map[string][]TaxFieldFact) {
	type formScope struct {
		period                                      string
		items, selected, zero, nonzero, na, unknown int
		missingRows, sources                        []string
	}
	forms := map[string]*formScope{}
	for _, key := range sortedMapKeys(rows) {
		fields := rows[key]
		f := fields[0]
		formKey := strings.Join([]string{f.RecordID, f.Version, f.FormCode, f.Period.Start, f.Period.End}, "|")
		scope := forms[formKey]
		if scope == nil {
			scope = &formScope{period: f.Period.Display}
			forms[formKey] = scope
		}
		for _, field := range fields {
			if field.ID != "" {
				scope.sources = append(scope.sources, "report-model.json/taxFields[id="+field.ID+"]")
			} else if field.Path != "" {
				scope.sources = append(scope.sources, "source-snapshot.json/"+field.Path)
			}
		}
		if f.RowLabel == "" || containsAny(f.RowLabel, "类调整项目", "合计") {
			continue
		}
		scope.items++
		columns := map[string]TaxFieldFact{}
		for _, field := range fields {
			columns[canonicalFormLabel(field.ColumnLabel)] = field
		}
		nonzero, missing := false, false
		for _, column := range []string{"调增金额", "调减金额"} {
			field := columns[column]
			switch field.Status {
			case "present":
				if field.Value == 0 {
					scope.zero++
				} else {
					scope.nonzero++
					nonzero = true
				}
			case "not_applicable":
				scope.na++
			default:
				scope.unknown++
				missing = true
			}
		}
		if nonzero {
			scope.selected++
		}
		if missing {
			scope.missingRows = append(scope.missingRows, f.RowLabel)
		}
	}
	for _, key := range sortedMapKeys(forms) {
		scope := forms[key]
		prefix := "adjustment-scope." + analysisToken(key)
		period := scope.period
		if period == "" {
			period = "本版"
		}
		text := period + "纳税调整项目明细表未取得可解析的项目明细，无法确定非零调整项目范围。"
		if scope.items > 0 {
			text = fmt.Sprintf("%s纳税调整项目明细表已解析%d个非小计项目；本节逐项展示其中有效调增或调减非零的%d项，其余原表项目未逐项展开。调增、调减单元格中，有效非零%d个、有效零值%d个、原表不适用%d个、未取得有效值%d个。", period, scope.items, scope.selected, scope.nonzero, scope.zero, scope.na, scope.unknown)
		}
		ref := d.text(prefix+".selection", "纳税调整明细选取范围", "coverage", text, uniqueSorted(scope.sources)...)
		if len(scope.missingRows) > 0 {
			d.text(prefix+".unavailable", "调增或调减字段未取得有效值的项目", "coverage", strings.Join(uniqueSorted(scope.missingRows), "、"), ref)
		}
	}
}

func addEntertainmentCalculation(d *SectionAnalysisData, ctx *reportContext, values map[string]TaxFieldFact, prefix string) []string {
	book, bok := values["账载金额"]
	declared, dok := values["税收金额"]
	if !bok || book.Value < 0 {
		return nil
	}
	refs := []string{d.number(prefix+".expense-cap", "招待费发生额60%限额", "calculated", "元", book.Period.Display, "", "业务招待费发生额×60%", book.Value*entertainmentExpenseDeductionRate, "report-model.json/taxFields[id="+book.ID+"]")}
	var revenue, deemed TaxFieldFact
	for _, f := range ctx.taxFields {
		if f.RecordID != book.RecordID || f.Version != book.Version || f.Period.Start != book.Period.Start || f.Period.End != book.Period.End || f.Status != "present" {
			continue
		}
		if isAnnualCITMainForm(f) && normalizeFinancialMetricLabel(f.RowLabel) == "营业收入" {
			revenue = f
		}
		if containsAny(f.FormCode+f.FormName, "A105000", "纳税调整项目明细表") && strings.Contains(f.RowLabel, "视同销售收入") && canonicalFormLabel(f.ColumnLabel) == "税收金额" {
			deemed = f
		}
	}
	if revenue.ID == "" || deemed.ID == "" || revenue.Value+deemed.Value < 0 {
		return refs
	}
	base := revenue.Value + deemed.Value
	refs = append(refs, d.number(prefix+".revenue-base", "招待费扣除销售收入基数", "amount", "元", book.Period.Display, "", "年度主表营业收入+视同销售收入税收金额；特殊业务基数另核", base, "report-model.json/taxFields[id="+revenue.ID+"]", "report-model.json/taxFields[id="+deemed.ID+"]"))
	refs = append(refs, d.number(prefix+".revenue-cap", "招待费销售收入5‰限额", "calculated", "元", book.Period.Display, "", "销售收入基数×5‰", base*entertainmentRevenueCapRate, refs[len(refs)-1]))
	allowed := math.Min(book.Value*entertainmentExpenseDeductionRate, base*entertainmentRevenueCapRate)
	refs = append(refs, d.number(prefix+".deductible", "按已取得基数测算招待费可扣除额", "calculated", "元", book.Period.Display, "", "MIN(发生额×60%,销售收入基数×5‰)", allowed, refs...))
	refs = append(refs, d.number(prefix+".increase", "按已取得基数测算招待费调增额", "calculated", "元", book.Period.Display, "", "账载金额−按两项限额测算扣除额", book.Value-allowed, refs...))
	if dok {
		refs = append(refs, d.number(prefix+".residual", "招待费申报扣除与限额测算差额", "difference", "元", book.Period.Display, "", "申报税收金额−限额测算金额；不等于凭证真实性结论", declared.Value-allowed, refs...))
	}
	return refs
}
