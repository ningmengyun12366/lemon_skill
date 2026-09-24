package report

import "regexp"

// Printed 1A/1B/1C/1D line identifiers distinguish all-business rate rows from
// their letter-suffixed subsets and the configurable '..' template rows.
var vatNewAnnexLinePattern = regexp.MustCompile(`^1[A-D](?:[0-9]{2}[a-z]?|\.\.)$`)
var vatNewAnnexBusinessLinePattern = regexp.MustCompile(`^1[A-D][0-9]{2}$`)

func vatUnbilledBusinessField(fact TaxFieldFact) bool {
	return fact.Status == "present" && isVATUnbilledBusinessField(fact)
}

// Keep row selection separate from availability so a missing monetary cell
// cannot silently disappear from a supposedly complete reconciliation.
func isVATUnbilledBusinessField(fact TaxFieldFact) bool {
	if fact.Module != "vat_returns" ||
		canonicalFormLabel(fact.Group) != "未开具发票" || canonicalFormLabel(fact.ColumnLabel) != "销售额" {
		return false
	}
	if fact.FormCode == vatNewAnnexOneFormCode {
		return vatNewAnnexBusinessLinePattern.MatchString(fact.Line) && !containsAny(canonicalFormLabel(fact.RowGroup), "其中")
	}
	return fact.FormCode == vatAnnexOneFormCode && !declarationLabelMatches(fact.RowLabel, []string{"合计", "总计"}) &&
		!containsAny(canonicalFormLabel(fact.RowLabel+fact.RowGroup), "其中")
}
