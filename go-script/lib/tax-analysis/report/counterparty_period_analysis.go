package report

import "strings"

// addCounterpartyPeriodEvidence uses the same identity keys as the ranking.
// Observed months and actual party/product links remain local analysis facts,
// not extra detail tables in the customer report.
func addCounterpartyPeriodEvidence(d *SectionAnalysisData, ctx *reportContext, item counterpartyItem, groups []invoiceGroup, module, prefix, nameRef string) {
	key := "name:" + invoicePartyName(item.Name)
	if id := invoicePartyTaxID(item.TaxID); id != "" {
		key = "id:" + id
	}
	keys := invoicePartyKeys(groups)
	amounts := map[string]float64{}
	refs := map[string][]string{}
	known := map[string]bool{}
	for _, chunk := range readInvoiceSummary(ctx.modules[module]).Chunks {
		month := chunk.Month
		if len(month) == 6 {
			month = month[:4] + "-" + month[4:]
		}
		valid := chunk.Total == 0 || len(chunk.Groups) > 0
		for _, group := range chunk.Groups {
			valid = valid && group.HasAmount
		}
		known[month] = valid
	}
	for i, group := range groups {
		if keys[i] != key {
			continue
		}
		source := module + "/chunks/groups[id=" + group.ID + "]"
		if group.HasAmount {
			amounts[group.Month] += group.Amount
			refs[group.Month] = append(refs[group.Month], source)
		}
	}
	var available, missing []string
	for _, month := range deliveryMonths(ctx.task.StartTime, ctx.task.EndTime) {
		periodRefs := append([]string{nameRef}, refs[month]...)
		if !known[month] {
			missing = append(missing, month)
			continue
		}
		available = append(available, month)
		periodRefs = append(periodRefs, module+"/chunks[month="+strings.ReplaceAll(month, "-", "")+"]")
		d.number(prefix+".month."+month, "交易对手分月金额", "amount", "元", month, "", "同方向同主体当月有符号金额；完整查询月无该主体记录记零", amounts[month], periodRefs...)
	}
	d.text(prefix+".month-coverage", "交易对手分月数据覆盖", "coverage", "已取得月份："+textOr(strings.Join(available, "、"), "无")+"；未完整取得月份："+textOr(strings.Join(missing, "、"), "无"), nameRef)
	if len(available) > 0 {
		minimum, maximum := available[0], available[0]
		for _, month := range available[1:] {
			if amounts[month] < amounts[minimum] {
				minimum = month
			}
			if amounts[month] > amounts[maximum] {
				maximum = month
			}
		}
		for _, extremum := range []struct{ key, label, month string }{{"maximum", "最大", maximum}, {"minimum", "最小", minimum}} {
			source := metricID(d.SectionID, prefix+".month."+extremum.month)
			d.text(prefix+".month-"+extremum.key+".period", "已取得月份中交易金额"+extremum.label+"的月份", "period", extremum.month, source)
			d.number(prefix+".month-"+extremum.key+".amount", "已取得月份中交易金额"+extremum.label+"值", "extremum", "元", extremum.month, "", "仅在已完整取得月份的同主体有符号金额中比较；并列时取最早月份", amounts[extremum.month], source)
		}
	}
	addCounterpartyProductEvidence(d, ctx, item, groups, module, prefix, nameRef)
}

// addCounterpartyProductEvidence is shared by the ranked-party section and
// credit context, including signed-net parties outside the ranked projection.
func addCounterpartyProductEvidence(d *SectionAnalysisData, ctx *reportContext, item counterpartyItem, groups []invoiceGroup, module, prefix, nameRef string) {
	key := "name:" + invoicePartyName(item.Name)
	if id := invoicePartyTaxID(item.TaxID); id != "" {
		key = "id:" + id
	}
	products, productRefs := map[string]float64{}, map[string][]string{}
	keys := invoicePartyKeys(groups)
	for i, group := range groups {
		if keys[i] != key {
			continue
		}
		for _, line := range group.Lines {
			if !line.HasAmount || line.Item == "" {
				continue
			}
			products[line.Item] += line.Amount
			productRefs[line.Item] = append(productRefs[line.Item], module+"/chunks/groups[id="+group.ID+"]")
		}
	}
	for _, product := range sortedMapKeys(products) {
		productKey := prefix + ".product." + analysisToken(product)
		sources := append([]string{nameRef}, uniqueSorted(productRefs[product])...)
		label := d.text(productKey+".name", "该交易对手实际发票品名", "object", product, sources...)
		d.number(productKey+".amount", "该交易对手该品名金额", "amount", "元", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "仅合计实际同一主体汇总组中的同品名明细", products[product], label)
	}
}
