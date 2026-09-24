package report

// 报告业务规则中的已确认数量和比例边界。
const (
	counterpartyTopFiveCount          = 5
	counterpartyTopTenCount           = 10
	counterpartyTopFiveShareThreshold = 0.60
	degradableModuleCode              = "arrears_and_refunds"
	degradableModuleWarning           = "欠税及退税数据未完整取得，本报告仅使用已成功返回的客观数据；无法确认的欠税或退税事项不作无异常判断。"
	emptyTaxSourceMessage             = "data为空"
	emptyTaxSourceWarning             = "房产税或城镇土地使用税税源接口未返回数据，相关章节仅展示数据不足状态，不作无异常或不适用判断。"
	reportTimezoneOffsetSeconds       = 8 * 60 * 60
	reportDateLayout                  = "2006-01-02"
	reportTimestampLayout             = "2006-01-02 15:04:05"
)

// emptyTaxSourceModuleCodes lists tax-source modules whose exact `data为空`
// terminal result represents an empty source, not a transport or login failure.
var emptyTaxSourceModuleCodes = map[string]bool{
	"property_tax":       true,
	"urban_land_use_tax": true,
}

// 技术状态码只用于数据适配，不直接进入面向用户的报告。
var nonDisplayFactKeys = map[string]bool{
	"status": true,
}
