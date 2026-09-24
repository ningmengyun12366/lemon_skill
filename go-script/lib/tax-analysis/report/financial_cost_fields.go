package report

var financialCostComparisonHeaders = []string{"季度", "财务成本及期间费用", "购进发票金额", "扣除工资后差异", "分析"}

// financialCostWithExpenses follows the financial statement's R&D presentation
// so a research subitem already in administration is never added twice.
func financialCostWithExpenses(fact financialFact) (float64, bool) {
	expenses, known := financialExpenseTotal(fact)
	return sumKnown(optionalValue(fact.Cost, fact.HasCost), optionalValue(expenses, known))
}

// financialExpenseTotal follows the declared R&D presentation for all consumers.
func financialExpenseTotal(fact financialFact) (float64, bool) {
	if !fact.ResearchInManagementSet {
		return 0, false
	}
	values := []knownValue{
		optionalValue(fact.SalesExpense, fact.HasSalesExpense),
		optionalValue(fact.ManagementExpense, fact.HasManagementExpense),
		optionalValue(fact.FinanceExpense, fact.HasFinanceExpense),
	}
	if !fact.ResearchInManagement {
		values = append(values, optionalValue(fact.ResearchExpense, fact.HasResearchExpense))
	}
	return sumKnown(values...)
}
