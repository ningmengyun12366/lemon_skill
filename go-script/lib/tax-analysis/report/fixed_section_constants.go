package report

const (
	associationNoClueText         = "本期未识别到与已核验直接股东或分支机构存在明确关系的交易对手方。"
	cashFlowUnavailableText       = "当前报告未取得现金流量表数据，补充数据后可进行经营活动现金流分析。"
	deductionNoRecordText         = "用途确认及抵扣模块调用成功，本分析期间未返回可展示的凭证确认或抵扣记录。"
	annualAdjustmentNoDataText    = "当前企业所得税年度汇算申报数据未返回可展示的主要纳税调整项目明细。"
	taxExceptionNoFindingText     = "按当前已取得的申报、缴款及欠税退税数据，未识别到可展示的申报更正、逾期或欠税记录。"
	taxExceptionDegradedText      = "欠税及退税数据未完整取得，本节仅展示已取得的申报和缴款记录，暂不形成欠税或退税结论。"
	maximumDeductionAttentionRows = 20
)

var (
	annualAdjustmentMetrics = []metricDefinition{
		{name: "纳税调整增加额", aliases: []string{"taxAdjustmentIncrease", "adjustmentIncrease", "纳税调整增加额"}, money: true},
		{name: "纳税调整减少额", aliases: []string{"taxAdjustmentDecrease", "adjustmentDecrease", "纳税调整减少额"}, money: true},
		{name: "业务招待费纳税调整", aliases: []string{"businessEntertainmentAdjustment", "entertainmentAdjustment", "业务招待费纳税调整额"}, money: true},
		{name: "广告费和业务宣传费纳税调整", aliases: []string{"advertisingAdjustment", "promotionAdjustment", "广告费和业务宣传费纳税调整额"}, money: true},
		{name: "职工薪酬纳税调整", aliases: []string{"employeeCompensationAdjustment", "salaryAdjustment", "职工薪酬纳税调整额"}, money: true},
		{name: "资产折旧摊销纳税调整", aliases: []string{"depreciationAdjustment", "amortizationAdjustment", "资产折旧摊销纳税调整额"}, money: true},
		{name: "资产损失纳税调整", aliases: []string{"assetLossAdjustment", "资产损失纳税调整额"}, money: true},
	}
	stampTaxMetrics = []metricDefinition{
		{name: "计税依据", aliases: []string{"taxBasis", "taxableAmount", "计税依据"}, money: true},
		{name: "应纳税额", aliases: []string{"taxPayable", "declaredTax", "应纳税额"}, money: true},
	}
	stampTaxTextMetrics = []textMetricDefinition{
		{name: "税目", aliases: []string{"taxItemName", "taxCategoryName", "税目"}},
		{name: "税率", aliases: []string{"taxRate", "levyRate", "税率"}},
	}
	propertyTaxMetrics = []metricDefinition{
		{name: "房产原值", aliases: []string{"originalPropertyValue", "propertyOriginalValue", "房产原值"}, money: true},
		{name: "租金收入", aliases: []string{"rentalIncome", "rentIncome", "租金收入"}, money: true},
		{name: "计税依据", aliases: []string{"taxBasis", "taxableAmount", "计税依据"}, money: true},
		{name: "应纳税额", aliases: []string{"taxPayable", "declaredTax", "应纳税额"}, money: true},
	}
	propertyTaxTextMetrics = []textMetricDefinition{
		{name: "计税方式", aliases: []string{"taxMethod", "calculationMethod", "计税方式"}},
		{name: "房产坐落", aliases: []string{"propertyAddress", "address", "房产坐落"}},
	}
	landUseTaxMetrics = []metricDefinition{
		{name: "计税土地面积", aliases: []string{"taxableLandArea", "landArea", "计税土地面积"}, money: false},
		{name: "单位税额", aliases: []string{"unitTax", "taxStandard", "单位税额"}, money: false},
		{name: "应纳税额", aliases: []string{"taxPayable", "declaredTax", "应纳税额"}, money: true},
	}
	landUseTaxTextMetrics = []textMetricDefinition{
		{name: "土地等级", aliases: []string{"landGrade", "taxGrade", "土地等级"}},
		{name: "土地坐落", aliases: []string{"landAddress", "address", "土地坐落"}},
	}
	arrearsExplicitAmountAliases = []string{"arrearsAmount", "taxArrearsAmount", "qsje", "欠税金额"}
	arrearsGenericAmountAliases  = []string{"amount", "金额"}
	arrearsTaxNameAliases        = []string{"taxName", "taxTypeName", "zsxmMc", "税种"}
	arrearsExplicitStatusAliases = []string{"arrearsStatus", "欠税状态"}
	arrearsSemanticAliases       = []string{"recordType", "businessType", "category", "title", "name", "reason", "type"}
	deductionAmountAliases       = []string{"yxdkse", "deductibleTax", "confirmedTax", "deductionTax", "kydkse", "taxAmount"}
	deductionStatusAliases       = []string{"gxzt", "gxztMc", "confirmationStatus", "deductionStatus", "status"}
	deductionPurposeAliases      = []string{"purpose", "useStatus", "ytMc", "用途"}
	deductionInvoiceAliases      = []string{"sdphm", "digitalInvoiceNumber", "invoiceNumber", "invoiceNo", "fphm"}
	deductionPeriodAliases       = []string{"skssq", "period", "taxPeriod", "skssqq", "month"}
)
