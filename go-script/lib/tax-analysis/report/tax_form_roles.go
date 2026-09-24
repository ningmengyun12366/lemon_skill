package report

import "strings"

// Annual detail codes may be local child numbers. Recognize the verified form
// name within the annual CIT module while retaining the original code as evidence.
// These roles route forms only; tax classification still uses project codes.
func isAnnualCITMainForm(fact TaxFieldFact) bool {
	if fact.Module != "cit_annual_returns" {
		return false
	}
	return strings.Contains(fact.FormCode, "A100000") || declarationLabelMatches(fact.FormName, []string{
		"年度纳税申报主表", "中华人民共和国企业所得税年度纳税申报表",
	})
}

func isCITResearchForm(fact TaxFieldFact) bool {
	return (fact.Module == "cit_annual_returns" || fact.Module == "filing_obligations") && (strings.Contains(fact.FormCode, "A107012") ||
		canonicalFormLabel(fact.FormName) == "研发费用加计扣除优惠明细表")
}
