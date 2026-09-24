package excelx

const (
	TaxBureauVerifyStatusHeader = "税局查验状态"
	VerifyTimeHeader            = "查验时间"
)

var IntermediateVerifyHeaders = []string{
	"导入时间",
	"文件名",
	"发票类型",
	"发票代码",
	"发票号码",
	"校验码",
	"发票状态",
	"开票日期",
	"销售方名称",
	"销售方税号",
	"购买方名称",
	"购买方税号",
	"不含税金额",
	"税额",
	"商品明细",
	"价税合计",
	TaxBureauVerifyStatusHeader,
	VerifyTimeHeader,
}

var BatchManageFinalHeaders = []string{
	"导入时间",
	"文件名",
	"发票类型",
	"发票代码",
	"发票号码",
	"开票日期",
	"销售方名称",
	"销售方税号",
	"购买方名称",
	"购买方税号",
	"发票商品名称",
	"不含税金额",
	"税额",
	"价税合计",
	TaxBureauVerifyStatusHeader,
	VerifyTimeHeader,
	"是否重复风险",
	"报销状态",
	"入账状态",
	"处理建议",
}

var ReimbursementFinalHeaders = []string{
	"序号",
	"报销批次号",
	"报销日期",
	"报销人",
	"部门",
	"费用分类",
	"报销事由",
	"发票类型",
	"开票日期",
	"销售方名称",
	"发票代码",
	"发票号码",
	"发票状态",
	"发票商品名称",
	"发票金额",
	"发票税额",
	"价税合计",
	TaxBureauVerifyStatusHeader,
	VerifyTimeHeader,
	"是否重复风险",
	"文件名",
	"备注",
}

var AnalysisInputHeaders = []string{
	"导入时间",
	"文件名",
	"发票类型",
	"发票代码",
	"发票号码",
	"发票状态",
	"开票日期",
	"销售方名称",
	"销售方税号",
	"购买方名称",
	"购买方税号",
	"不含税金额",
	"税额",
	"商品明细",
	"价税合计",
	TaxBureauVerifyStatusHeader,
	VerifyTimeHeader,
}

var AnalysisFinalHeaders = []string{
	"导入时间",
	"文件名",
	"发票类型",
	"发票代码",
	"发票号码",
	"发票状态",
	"开票日期",
	"销售方名称",
	"销售方税号",
	"购买方名称",
	"购买方税号",
	"不含税金额",
	"税额",
	"价税合计",
	TaxBureauVerifyStatusHeader,
	VerifyTimeHeader,
	"是否重复风险",
}

var IntermediateAmountHeaders = stringSet("不含税金额", "税额", "价税合计")
var FinalAmountHeaders = stringSet("不含税金额", "税额", "价税合计")
var ReimbursementAmountHeaders = stringSet("发票金额", "发票税额", "价税合计")

var IntermediateWidths = map[string]float64{
	"导入时间":                      20,
	"文件名":                       30,
	"发票类型":                      20,
	"发票代码":                      15,
	"发票号码":                      15,
	"校验码":                       24,
	"发票状态":                      10,
	"开票日期":                      12,
	"销售方名称":                     30,
	"销售方税号":                     20,
	"购买方名称":                     30,
	"购买方税号":                     20,
	"不含税金额":                     12,
	"税额":                        12,
	"商品明细":                      40,
	"价税合计":                      12,
	TaxBureauVerifyStatusHeader: 25,
	VerifyTimeHeader:            20,
}

var BatchManageWidths = map[string]float64{
	"导入时间":                      20,
	"文件名":                       30,
	"发票类型":                      20,
	"发票代码":                      15,
	"发票号码":                      15,
	"开票日期":                      12,
	"销售方名称":                     30,
	"销售方税号":                     20,
	"购买方名称":                     30,
	"购买方税号":                     20,
	"发票商品名称":                    30,
	"不含税金额":                     12,
	"税额":                        12,
	"价税合计":                      12,
	TaxBureauVerifyStatusHeader: 25,
	VerifyTimeHeader:            20,
	"是否重复风险":                    20,
	"报销状态":                      12,
	"入账状态":                      12,
	"处理建议":                      20,
}

var ReimbursementWidths = map[string]float64{
	"序号":                        8,
	"报销批次号":                     18,
	"报销日期":                      12,
	"报销人":                       12,
	"部门":                        12,
	"费用分类":                      18,
	"报销事由":                      24,
	"发票类型":                      18,
	"开票日期":                      12,
	"销售方名称":                     30,
	"发票代码":                      16,
	"发票号码":                      16,
	"发票状态":                      10,
	"发票商品名称":                    30,
	"发票金额":                      12,
	"发票税额":                      12,
	"价税合计":                      12,
	TaxBureauVerifyStatusHeader: 26,
	VerifyTimeHeader:            20,
	"是否重复风险":                    18,
	"文件名":                       30,
	"备注":                        18,
}

var AnalysisWidths = map[string]float64{
	"导入时间":                      20,
	"文件名":                       30,
	"发票类型":                      20,
	"发票代码":                      15,
	"发票号码":                      15,
	"发票状态":                      10,
	"开票日期":                      12,
	"销售方名称":                     30,
	"销售方税号":                     20,
	"购买方名称":                     30,
	"购买方税号":                     20,
	"不含税金额":                     12,
	"税额":                        12,
	"价税合计":                      12,
	TaxBureauVerifyStatusHeader: 25,
	VerifyTimeHeader:            20,
	"是否重复风险":                    20,
}

func stringSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
