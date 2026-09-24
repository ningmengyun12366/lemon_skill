package report

import "strings"

type sensitiveServiceRule struct {
	category  string
	keywords  []string
	attention string
}

// Specific service names precede generic fees. Each invoice line belongs to
// one category only; classification describes a review topic, not a risk.
var sensitiveServiceRules = []sensitiveServiceRule{
	{"咨询", []string{"咨询"}, "服务内容及成果"},
	{"会议", []string{"会议", "会展", "展览"}, "会议主题及参会资料"},
	{"培训", []string{"培训"}, "培训内容、参加人员及业务相关性"},
	{"广告", []string{"广告"}, "业务相关性及扣除口径"},
	{"推广", []string{"推广"}, "推广内容、成果及业务相关性"},
	{"人力资源服务", []string{"人力资源", "劳务派遣"}, "服务人员及结算依据"},
	{"餐饮住宿服务", []string{"餐饮", "餐费", "住宿"}, "招待费与差旅费区分"},
	{"旅游娱乐服务", []string{"旅游", "娱乐"}, "企业所得税及业务合理性"},
	{"预付卡", []string{"预付卡"}, "员工福利或赠送用途"},
	{"技术服务", []string{"技术服务"}, "服务内容、交付成果及业务相关性"},
	{"劳务", []string{"劳务"}, "服务人员、实际工作及结算依据"},
	{"服务费", []string{"服务费"}, "服务内容、成果及业务相关性"},
}

type sensitiveServiceAmount struct {
	amount float64
	count  int
}

func sensitiveServiceCategory(text string) string {
	for _, rule := range sensitiveServiceRules {
		for _, keyword := range rule.keywords {
			if strings.Contains(text, keyword) {
				return rule.category
			}
		}
	}
	return ""
}

func sensitiveServiceAttention(category string) string {
	for _, rule := range sensitiveServiceRules {
		if rule.category == category {
			return rule.attention
		}
	}
	return "服务内容及业务相关性"
}

// sensitiveServiceTotals sums signed line amounts once per direction. Category
// matching is first-match, so a line is never counted under two service labels.
func sensitiveServiceTotals(groups []invoiceGroup) (map[string]sensitiveServiceAmount, sensitiveServiceAmount) {
	byCategory := map[string]sensitiveServiceAmount{}
	var total sensitiveServiceAmount
	for _, invoice := range groups {
		for _, line := range invoiceFactLines(invoice) {
			category := sensitiveServiceCategory(invoiceLineSearchText(line))
			if category == "" {
				continue
			}
			item := byCategory[category]
			item.count += line.Count
			total.count += line.Count
			if line.HasAmount {
				item.amount += line.Amount
				total.amount += line.Amount
			}
			byCategory[category] = item
		}
	}
	return byCategory, total
}
