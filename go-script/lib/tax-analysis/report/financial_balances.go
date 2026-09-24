package report

type financialBalance struct {
	Opening    float64
	Closing    float64
	HasOpening bool
	HasClosing bool
}

var financialSupplementaryBalanceLabels = []string{"实收资本（或股本）", "实收资本", "资本公积", "固定资产原价", "固定资产账面价值", "固定资产", "投资性房地产", "预收账款", "合同负债", "资产总计", "负债合计"}

func applyFinancialBalanceRow(fact *financialFact, row financialJSONRow) {
	label := canonicalFormLabel(row.label)
	if !financialStringSliceContains(financialSupplementaryBalanceLabels, label) {
		return
	}
	if fact.Balances == nil {
		fact.Balances = map[string]financialBalance{}
	}
	openingKeys, closingKeys := []string{"ncyeZc", "ncye", "openingBalance"}, financialAssetValueKeys
	if row.labelKey == "qyxmmc" {
		openingKeys = []string{"ncyeQy", "ncye", "openingBalance"}
		closingKeys = financialLiabilityValueKeys
	}
	balance := fact.Balances[label]
	balance.Opening, balance.HasOpening = mapNumber(row.values, openingKeys...)
	balance.Closing, balance.HasClosing = mapNumber(row.values, closingKeys...)
	fact.Balances[label] = balance
}

func applyFinancialHTMLBalance(fact *financialFact, rows []formTableRow, row formTableRow, label string) {
	canonical := canonicalFormLabel(label)
	if !financialStringSliceContains(financialSupplementaryBalanceLabels, canonical) {
		return
	}
	if fact.Balances == nil {
		fact.Balances = map[string]financialBalance{}
	}
	balance := fact.Balances[canonical]
	balance.Opening, balance.HasOpening = financialHTMLAmount(rows, row, label, []string{"年初余额", "期初余额", "年初数", "上年年末余额"})
	balance.Closing, balance.HasClosing = financialHTMLAmount(rows, row, label, []string{"期末余额", "期末数"})
	fact.Balances[canonical] = balance
}
