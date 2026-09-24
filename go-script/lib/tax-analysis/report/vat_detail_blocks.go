package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func vatDetailRows(ctx *reportContext, transfer bool) [][]string {
	amounts := map[string]float64{}
	texts := map[string]string{}
	labels := map[string][]string{}
	periods := map[string]string{}
	for _, fact := range ctx.taxFields {
		if fact.Module != "vat_returns" || (fact.Status != "present" && fact.Status != "present_text") {
			continue
		}
		match := false
		if transfer {
			match = containsAny(fact.RowLabel, "进项税额转出", "异常凭证转出", "上期留抵税额", "红字专用发票信息表") || fact.FormCode == "BDA0610608"
		} else {
			match = containsAny(fact.RowLabel, "免税销售额", "按简易办法计税销售额", "出口免税销售额") || containsAny(fact.FormName, "扣除项目", "税额抵减", "减免税申报明细")
		}
		if !match {
			continue
		}
		if !declarationLabelMatches(fact.ColumnLabel, []string{"本月数", "本期数", "本年累计", "税额", "金额", "本期实际扣除金额", "本期实际抵减税额", "免税额", "期初余额", "期末余额", "本期发生额", "本期应扣除金额", "本期应抵减税额", "本期可抵减额", "本期实际抵减额", "本期调减额", "减税性质代码及名称", "免税性质代码及名称", "减免性质代码", "免税销售额对应的进项税额", "扣除后免税销售额", "免征增值税项目销售额"}) {
			continue
		}
		key := fact.Period.Display + "|" + fact.FormCode + "|" + fact.RowLabel + "|" + fact.ColumnLabel + "|" + fact.Group
		amounts[key] += fact.Value
		if fact.Status == "present_text" {
			texts[key] = fact.Raw
		}
		labels[key] = []string{fact.RowLabel, fact.Period.Display + " · " + fact.Group + " · " + fact.ColumnLabel}
		periods[key] = fact.Period.Display
	}
	keys := make([]string, 0, len(amounts))
	for key := range amounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	type valueGroup struct {
		label, column, value string
		periods              []string
	}
	groups := map[string]*valueGroup{}
	var order []string
	for _, key := range keys {
		value := moneyText(amounts[key])
		if text, ok := texts[key]; ok {
			value = text
		}
		column := strings.TrimPrefix(labels[key][1], periods[key]+" · ")
		// Include form identity so equal Chinese labels from two annexes do not
		// become an invented common field. Only identical values are compacted.
		parts := strings.Split(key, "|")
		groupKey := parts[1] + "|" + labels[key][0] + "|" + column + "|" + value
		if groups[groupKey] == nil {
			groups[groupKey] = &valueGroup{label: labels[key][0], column: column, value: value}
			order = append(order, groupKey)
		}
		groups[groupKey].periods = appendUnique(groups[groupKey].periods, periods[key])
	}
	var rows [][]string
	for _, key := range order {
		group := groups[key]
		status := "按申报原值展示；相同数值合并列示期间"
		if transfer {
			status = "结合异常凭证及转出证据核实，不据总额认定逐票已处理"
		}
		rows = append(rows, []string{group.label, compactEqualValuePeriods(group.periods) + " · " + group.column + "：" + group.value, status})
	}
	return rows
}

func compactEqualValuePeriods(periods []string) string {
	values := append([]string(nil), periods...)
	sort.Strings(values)
	if len(values) > 2 {
		first, e1 := time.Parse("2006-01", values[0])
		last, e2 := time.Parse("2006-01", values[len(values)-1])
		if e1 == nil && e2 == nil && (last.Year()-first.Year())*12+int(last.Month()-first.Month())+1 == len(values) {
			return fmt.Sprintf("%s至%s（%d期逐期同值）", values[0], values[len(values)-1], len(values))
		}
	}
	return strings.Join(values, "、")
}

func vatSpecialRows(ctx *reportContext) [][]string {
	amounts := map[string]float64{}
	known := map[string]bool{}
	for _, fact := range ctx.taxFields {
		if vatUnbilledBusinessField(fact) {
			key := fact.Period.Display + " · 未开票销售额"
			amounts[key] += fact.Value
			known[key] = true
		}
	}
	if ctx.taxpayer == taxpayerSmall {
		for _, fact := range ctx.vat {
			issued, issuedKnown := invoiceAmountForTaxPeriod(ctx.issued, fact.TaxPeriod)
			if !issuedKnown && ctx.modules["issued_invoices"].Complete && fact.TaxPeriod.Start >= ctx.task.StartTime && fact.TaxPeriod.End <= ctx.task.EndTime {
				issuedKnown = true
			}
			if fact.HasSales && issuedKnown {
				key := fact.Period + " · 票申差额"
				amounts[key], known[key] = fact.Sales-issued, true
			}
		}
	}
	for _, invoice := range ctx.issued {
		if invoice.HasAmount && (invoice.Amount < 0 || invoice.Red) {
			key := invoice.Month + " · 红字发票净额"
			amounts[key] += invoice.Amount
			known[key] = true
		}
	}
	keys := make([]string, 0, len(known))
	for key := range known {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var rows [][]string
	for _, key := range keys {
		note := "未开票销售额取申报表附列一全部业务，红字金额保留原始正负号"
		if strings.Contains(key, "票申差额") {
			note = "申报销售额减销项不含税净额；缺少完整口径解释时不直接认定未开票收入、漏报或异常"
		}
		rows = append(rows, []string{key, moneyText(amounts[key]), note})
	}
	return rows
}

// vatBusinessRates only reports rates from nonzero sales rows, never a header
// or the presence of an unused rate row in the template.
func vatBusinessRateRows(ctx *reportContext) [][]string {
	var rows [][]string
	seen := map[string]bool{}
	for _, fact := range ctx.taxFields {
		if fact.Module != "vat_returns" || (fact.FormCode != vatAnnexOneFormCode && fact.FormCode != vatNewAnnexOneFormCode) || fact.Status != "present" || fact.Value == 0 || canonicalFormLabel(fact.ColumnLabel) != "销售额" || canonicalFormLabel(fact.Group) != "合计" {
			continue
		}
		if !strings.Contains(fact.RowLabel, "%") {
			continue
		}
		key := fact.Period.Display + fact.RowLabel
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, []string{fact.RowLabel, fmt.Sprintf("%s有申报销售额%s", fact.Period.Display, moneyText(fact.Value)), "有效业务行适用税率/征收率"})
	}
	return rows
}
