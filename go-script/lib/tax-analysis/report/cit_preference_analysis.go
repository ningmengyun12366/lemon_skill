package report

import (
	"fmt"
	"strings"
)

const citSmallEnterprisePreferenceLabel = "符合条件的小型微利企业减免企业所得税"

// citPreferenceAmountFields selects the same main-form amounts for the report
// and analysis packet. Explicit zero and the authority's project label survive;
// a missing benefit row is never manufactured from another period's form.
func citPreferenceAmountFields(ctx *reportContext) []TaxFieldFact {
	var result []TaxFieldFact
	seen := map[string]bool{}
	for _, f := range ctx.taxFields {
		if f.Status != "present" || (f.Module != "cit_annual_returns" && f.Module != "cit_quarterly_returns") {
			continue
		}
		main := isAnnualCITMainForm(f) || (f.Module == "cit_quarterly_returns" && (f.FormCode == citMainFormCode || strings.Contains(f.FormName, citMainFormNameFragment)))
		if !main || !declarationLabelMatches(f.ColumnLabel, []string{"金额", "本年累计", "本年累计金额"}) {
			continue
		}
		label := normalizeFinancialMetricLabel(f.RowLabel)
		if !declarationLabelMatches(label, []string{"减免所得税额", "减免税额", "抵免所得税额", "小型微利企业减免所得税额", "高新技术企业减免所得税额", citSmallEnterprisePreferenceLabel}) {
			continue
		}
		key := f.RecordID + "|" + label
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, f)
	}
	return result
}

// analysisCITPreferences carries declared benefit projects and qualification
// selections into section 6.7. Amounts reuse taxFields; checkbox text reuses the
// existing exact-label tax-table reader and remains a declaration, not approval.
func analysisCITPreferences(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "cit.preferences"}
	for _, f := range citPreferenceAmountFields(ctx) {
		prefix := "cit-preference." + analysisToken(f.Module+"|"+f.RecordID+"|"+f.RowLabel)
		source := "report-model.json/taxFields[id=" + f.ID + "]"
		project := d.text(prefix+".project", "申报优惠项目或汇总栏", "object", f.RowLabel, source)
		period := d.text(prefix+".period", "优惠申报所属期", "period", f.Period.Display, source)
		amount := d.number(prefix+".amount", "优惠申报金额", "amount", "元", f.Period.Display, "", "同一申报表、行次及金额列的原值；汇总栏与具体项目不相加", f.Value, source)
		d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(f)}
		form := d.text(prefix+".form", "优惠金额申报来源", "source", joinNonEmpty(f.FormName, "行次"+textOr(f.Line, "未标注"), f.ColumnLabel), source)
		refs := []string{project, period, amount, form}
		if f.FiledAt != "" {
			refs = append(refs, d.text(prefix+".filed-at", "优惠申报日期", "period", f.FiledAt, source))
		}
		d.focus(prefix, "分别说明本所属期实际列示的优惠项目和金额；汇总栏与项目明细不相加。明确零金额不替代项目有无或适用资格的判断。", refs...)
	}
	for _, module := range []string{"cit_quarterly_returns", "cit_annual_returns"} {
		records, _ := namedCollectionMapsWithPresence(ctx.modules[module].Raw, "effectiveRecords")
		for recordIndex, record := range records {
			period, ok := taxPeriodFromRecord(record, false)
			if !ok || (ctx.task.StartTime != "" && period.Start < ctx.task.StartTime) ||
				(ctx.task.EndTime != "" && period.End > ctx.task.EndTime) {
				continue
			}
			for detailIndex, item := range arrayAt(record, "details") {
				detail, ok := item.(map[string]any)
				if !ok || strings.EqualFold(mapText(detail, "status"), "FAILED") {
					continue
				}
				formName := mapText(detail, "name", "label")
				if !strings.Contains(formName, citMainFormNameFragment) && !strings.Contains(formName, "企业所得税年度纳税申报基础信息表") {
					continue
				}
				value, _ := mapValue(detail, "data")
				selection, found := labeledTableText(value, []string{"小型微利企业"})
				if !found {
					continue
				}
				prefix := "cit-qualification." + analysisToken(module+"|"+mapText(record, "sbuuid")+"|"+period.Start+"|"+period.End+"|"+mapText(detail, "code"))
				source := fmt.Sprintf("source-snapshot.json/moduleResults[code=%s]/data/effectiveRecords[%d]/details[%d]/data", module, recordIndex, detailIndex)
				project := d.text(prefix+".name", "优惠资格申报项目", "object", "小型微利企业", source)
				state := d.text(prefix+".selection", "小型微利企业申报勾选原文", "state", selection, source)
				periodRef := d.text(prefix+".period", "资格申报所属期", "period", period.Display, source)
				form := d.text(prefix+".form", "资格申报来源", "source", formName, source)
				refs := []string{project, state, periodRef, form}
				if filedAt := mapText(record, "nssbrq", "lrrq"); filedAt != "" {
					refs = append(refs, d.text(prefix+".filed-at", "资格申报日期", "period", filedAt, source))
				}
				d.focus(prefix, "该状态是本所属期申报表的勾选原文，不能视为优惠适用条件和备查资料已经核验，也不能用其他期间的金额补齐本表未取得的优惠栏。", refs...)
			}
		}
	}
	return d
}
