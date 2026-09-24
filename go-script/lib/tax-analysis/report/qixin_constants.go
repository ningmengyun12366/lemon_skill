package report

// 启信宝报告适配器的模块字段、业务角色和确定性规则边界。
const (
	qixinModuleCode                    = "qixin_external"
	qixinSubjectsField                 = "subjects"
	qixinWarningsField                 = "warnings"
	qixinSubjectNameField              = "name"
	qixinSubjectRoleField              = "role"
	qixinSubjectShareField             = "transactionShare"
	qixinSubjectVerifiedField          = "identityVerified"
	qixinDetailField                   = "detail"
	qixinExecutionField                = "execution"
	qixinDishonestField                = "dishonest"
	qixinTaxViolationField             = "taxViolation"
	qixinAdministrativePunishmentField = "adminPunishment"
	qixinSeriousIllegalField           = "seriousIllegal"
	qixinEmployeesCollectionField      = "Employees"
	qixinBranchesCollectionField       = "Branches"
	qixinChangesCollectionField        = "Changerecords"
	qixinOwnEnterpriseRole             = "企业自身"
	qixinImportantCustomerRole         = "主要客户"
	qixinImportantSupplierRole         = "主要供应商"
	qixinFullOwnershipPercentage       = 100.0
	qixinFractionalOwnershipMaximum    = 1.0
	qixinTenThousandYuanScale          = 10000.0
	qixinRuleBusinessStatus            = "QIXIN-BUSINESS-STATUS-001"
	qixinRuleTaxViolation              = "QIXIN-TAX-VIOLATION-001"
	qixinRuleTaxPenalty                = "QIXIN-TAX-PENALTY-001"
	qixinRuleOperatingAbnormal         = "QIXIN-OPERATING-ABNORMAL-001"
	qixinRuleAdministrativePenalty     = "QIXIN-ADMIN-PENALTY-001"
	qixinRuleSeriousIllegal            = "QIXIN-SERIOUS-ILLEGAL-001"
	qixinRuleOwnExecution              = "QIXIN-OWN-EXECUTION-001"
)

// 启信宝状态字段没有统一枚举，以下词表只用于识别接口明确表达的当前或已处理状态。
var (
	qixinAbnormalBusinessStatusKeywords = []string{"注销", "吊销", "撤销", "责令关闭", "清算", "停业"}
	qixinUnhandledStatusKeywords        = []string{"未处理", "未履行", "未缴纳", "未执行", "执行中", "处理中"}
	qixinResolvedStatusKeywords         = []string{"已处理", "已履行", "已缴纳", "已执行", "已整改", "已结案", "已撤销", "失效"}
)
