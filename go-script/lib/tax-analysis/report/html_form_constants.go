package report

const (
	vatMainFormCode              = "BDA0610606"
	vatNewMainFormCode           = "BDA0611224"
	vatAnnexOneFormCode          = "BDA0610607"
	vatNewAnnexOneFormCode       = "BDA0611225"
	vatMainFormNameFragment      = "增值税及附加税费申报表（"
	citMainFormCode              = "001"
	citMainFormNameFragment      = "企业所得税月（季）度预缴纳税申报表"
	maximumDeclarationLineNumber = 999
)

var vatSalesRowLabels = []string{
	"（一）按适用税率计税销售额",
	"（二）按简易办法计税销售额",
	"（三）免、抵、退办法出口销售额",
	"（四）免税销售额",
}

var smallVATSalesRowLabels = []string{
	"（一）应征增值税不含税销售额（3%征收率）",
	"（二）应征增值税不含税销售额（5%征收率）",
	"（三）销售使用过的固定资产不含税销售额",
	"（四）免税销售额",
	"（五）出口免税销售额",
	"（六）核定销售额",
}

var vatOutputTaxRowLabels = []string{"销项税额"}
var vatInputTaxRowLabels = []string{"进项税额"}
var vatTaxPayableRowLabels = []string{"应纳税额合计", "应纳税额"}
var vatSupplementTaxRowLabels = []string{"本期应补（退）税额", "本期应补(退)税额"}
var vatCurrentColumnLabels = []string{"本月数", "本期数", "一般项目本期数", "即征即退项目本期数", "即证即退项目本期数"}
var vatEndCreditRowLabels = []string{"期末留抵税额"}

var citIncomeRowLabels = []string{"营业收入"}
var citCostRowLabels = []string{"营业成本", "减：营业成本"}
var citProfitRowLabels = []string{"利润总额"}
var citActualProfitRowLabels = []string{"实际利润额"}
var citTaxRowLabels = []string{"应纳所得税额"}
var citResearchRowLabels = []string{"研发费用", "减：研发费用"}

var htmlSpaceRunes = []string{"\u00a0", "\u2002", "\u2003", "\u2007", "\u202f", "\ufeff"}
