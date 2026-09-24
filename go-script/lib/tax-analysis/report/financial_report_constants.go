package report

const (
	financialPreviewField       = "previews"
	financialPreviewDataField   = "data"
	financialPreviewLabelField  = "label"
	financialIncomeLabel        = "利润表"
	financialActivityLabel      = "业务活动表"
	financialBalanceLabel       = "资产负债表"
	financialCashFlowLabel      = "现金流量表"
	financialMetricRevenue      = "营业收入"
	financialMetricCost         = "营业成本"
	financialMetricProfit       = "利润总额"
	financialMetricNetProfit    = "净利润"
	financialMetricSalesExpense = "销售费用"
	financialMetricAdminExpense = "管理费用"
	financialMetricRDExpense    = "研发费用"
	financialMetricFinanceCost  = "财务费用"
	financialMetricAssets       = "资产总计"
	financialMetricLiabilities  = "负债合计"
	financialMetricReceivables  = "应收账款"
	financialMetricInventory    = "存货"
	financialMetricContractDebt = "合同负债"
	financialMetricOperatingCF  = "经营活动产生的现金流量净额"
)

var (
	financialRowLabelKeys       = []string{"hmc", "xmmc", "zcxmmc", "qyxmmc", "rowName"}
	financialProfitValueKeys    = []string{"byje", "bqje", "amount", "value"}
	financialAnnualValueKeys    = []string{"byje", "bqje", "bnljje", "bnje", "ljje", "amount", "value"}
	financialAssetValueKeys     = []string{"qmyeZc", "qmye", "endingBalance", "amount", "value"}
	financialLiabilityValueKeys = []string{"qmyeQy", "qmye", "endingBalance", "amount", "value"}
	financialMetricLabels       = append([]string{
		financialMetricRevenue,
		financialMetricCost,
		financialMetricProfit,
		financialMetricNetProfit,
		financialMetricSalesExpense,
		financialMetricAdminExpense,
		financialMetricRDExpense,
		financialMetricFinanceCost,
		financialMetricAssets,
		financialMetricLiabilities,
		financialMetricReceivables,
		financialMetricInventory,
		financialMetricContractDebt,
		financialMetricOperatingCF,
	}, financialProfitComponentLabels()...)
	financialLabelPrefixes = []string{
		"一、", "二、", "三、", "四、", "五、", "六、", "七、", "八、", "九、", "十、",
		"一.", "二.", "三.", "四.", "五.", "六.", "七.", "八.", "九.", "十.",
		"加：", "加:", "减：", "减:",
	}
	financialMetricLabelAliases = map[string]string{
		"营业税金及附加":    "税金及附加",
		"销售收入":       financialMetricRevenue,
		"销售（营业）收入":   financialMetricRevenue,
		"销售(营业)收入":   financialMetricRevenue,
		"销售（营业）成本":   financialMetricCost,
		"销售(营业)成本":   financialMetricCost,
		"研究费用":       financialMetricRDExpense,
		"资产合计":       financialMetricAssets,
		"资产总额":       financialMetricAssets,
		"负债总额":       financialMetricLiabilities,
		"经营活动现金流量净额": financialMetricOperatingCF,
	}
)
