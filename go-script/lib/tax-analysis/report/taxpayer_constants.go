package report

type taxpayerKind string

const (
	taxpayerGeneral taxpayerKind = "general"
	taxpayerSmall   taxpayerKind = "small"
	taxpayerUnknown taxpayerKind = "unknown"

	taxpayerGeneralQualificationValue = "0"
	taxpayerSmallQualificationValue   = "1"
	taxpayerGeneralQualificationCode  = "201"

	deductionConfirmationModuleCode       = "deduction_confirmations"
	smallTaxpayerAuthorityMessageFragment = "仅提供给增值税一般纳税人或辅导期一般纳税人使用"
	smallTaxpayerAuthorityMessage         = "本功能仅提供给增值税一般纳税人或辅导期一般纳税人使用。"
	deductionApplicabilityNotApplicable   = "not_applicable"
	deductionGeneralTaxpayerOnlyReason    = "general_taxpayer_only"
	smallTaxpayerAuthorityEvidenceWarning = "税局用途确认接口明确返回本企业不适用一般纳税人功能，报告已按小规模纳税人口径处理。"
	unknownTaxpayerQualificationWarning   = "未能从税局数据确认纳税人类型，已省略仅适用于一般纳税人的增值税章节。"
	taxpayerQualificationConflictWarning  = "税局纳税人资格字段与文字类型字段不一致，报告已按明确的纳税人资格字段口径处理。"
)

// taxpayerQualificationFieldAliases lists authoritative qualification-code
// fields returned by the tax authority, including the nested qualification list.
var taxpayerQualificationFieldAliases = []string{
	"nsrzg",
	"NsrzglxDm",
}

// taxpayerTypeTextFieldAliases lists descriptive taxpayer-type fields used
// only when no authoritative qualification code is available.
var taxpayerTypeTextFieldAliases = []string{
	"taxpayerTypeName",
	"taxpayerType",
	"nsrlxmc",
	"nsrlx",
	"zzsnsrlx",
	"nsrzgmc",
	"taxpayerQualificationName",
	"Nsrzglx",
}
