package report

import "fmt"

// analysisSourceData preserves useful detail before customer-facing tables
// intentionally summarize it. It reuses the existing normalized contexts and
// exact identity matching; no additional API or alternative tax mapper runs.
func analysisSourceData(ctx *reportContext) []SectionAnalysisData {
	judicial := SectionAnalysisData{SectionID: "enterprise.judicial"}
	caseFocusCount := 0
	for _, subject := range qixinSubjects(ctx) {
		name, role := mapText(subject, qixinSubjectNameField), mapText(subject, qixinSubjectRoleField)
		addJudicialOtherCreditSummary(&judicial, subject)
		tradeAdded := false
		for _, field := range []string{qixinExecutionField, qixinDishonestField} {
			rows := qixinItemMaps(qixinSubjectData(subject, field), "CaseNumber", "Court", "Amount", "ExecutionStatus")
			for _, row := range rows {
				if !tradeAdded {
					addJudicialTradeFacts(&judicial, ctx, subject, role)
					tradeAdded = true
				}
				caseNo := mapText(row, "CaseNumber")
				date := mapText(row, "CaseDate", "Date", "PublishDate")
				key := "judicial." + analysisToken(name+"|"+field+"|"+caseNo+"|"+date)
				source := "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects[name=" + name + "]/" + field + "[CaseNumber=" + caseNo + "]"
				refs := []string{judicial.text(key+".name", "公开记录主体", "object", name, source), judicial.text(key+".role", "主体与报告企业关系", "object", role, source)}
				if caseNo != "" {
					refs = append(refs, judicial.text(key+".case", "执行案号", "object", caseNo, source))
				}
				if date != "" {
					refs = append(refs, judicial.text(key+".date", "公开记录日期", "period", date, source))
				}
				if amount, ok := mapNumber(row, "Amount"); ok {
					refs = append(refs, judicial.number(key+".amount", "公开记录执行金额", "amount", "元", date, "", "公开记录原金额，不代表企业欠税或当前未履行余额", amount, source))
				}
				status := qixinReadableStatus(row, "ExecutionStatus")
				if status == "" {
					status = "未取得当前履行状态"
				}
				refs = append(refs, judicial.text(key+".status", "当前履行状态", "state", status, source))
				if caseFocusCount < 3 {
					judicial.focus(key, "指明公开执行记录的主体、时间/案号、原始执行金额及可得状态。案件金额与本企业采购或销售金额分别说明，不将案件金额套入交易占比，也不当作当前欠款。", refs...)
					caseFocusCount++
				}
			}
		}
	}
	judicial.Constraints = []string{"公开记录日期与当前状态分别列示；没有当前履行状态时，不写仍未履行、已履行或现在欠款。", "本节聚焦司法执行和失信。其他信用记录摘要只用于跨章节核对，不能从身份匹配或执行查询为空推断行政处罚、经营异常均无记录，不重复其他信用章节的事件清单。"}
	result := append([]SectionAnalysisData{judicial, analysisSensitivePurchases(ctx), analysisTaxBurden(ctx), analysisInvoiceTaxComparison(ctx), analysisVATContinuity(ctx), analysisVATOverview(ctx)}, analysisCounterpartyEvents(ctx)...)
	result = append(result, analysisFinancialAmounts(ctx)...)
	result = append(result, analysisFinancialDetails(ctx)...)
	result = append(result, financialTrendAnalysis(ctx)...)
	result = append(result, analysisFinancialAnnualFlows(ctx))
	result = append(result, analysisLateFeePayments(ctx))
	result = append(result, analysisCITFilingBasis(ctx))
	result = append(result, analysisCITPreferences(ctx))
	result = append(result, analysisVATReliefPolicies(ctx))
	result = append(result, analysisVATSalesReconciliation(ctx))
	result = append(result, citAdjustmentAnalysis(ctx), analysisRefundRecords(ctx))
	result = append(result, analysisBusinessMatching(ctx)...)
	result = append(result, analysisFinancialComparisons(ctx)...)
	result = append(result, analysisFilingEvents(ctx)...)
	result = append(result, analysisFinancialAttention(ctx)...)
	result = append(result, analysisRedLinkCoverage(ctx)...)
	result = append(result, analysisInvoiceFieldQuality(ctx))
	result = append(result, analysisCounterpartyCoverage(ctx)...)
	result = append(result, analysisIndustrySummary(ctx))
	result = append(result, analysisIndustryScope(ctx, "industry.structure"), analysisIndustryScope(ctx, "industry.summary"))
	result = append(result, analysisCounterpartyDependencies(ctx)...)
	addSupplierCreditPurchaseContext(ctx, result)
	return result
}

// Trade facts are grouped by party, separately from each public case. Both
// operands use the same exact identity and signed denominator as the ranking.
func addJudicialTradeFacts(d *SectionAnalysisData, ctx *reportContext, subject map[string]any, role string) {
	if role != qixinImportantSupplierRole && role != qixinImportantCustomerRole {
		return
	}
	item, total, ok := invoiceCounterpartyAmounts(ctx, role, subject)
	if !ok {
		return
	}
	key := "judicial-trade." + analysisToken(role+"|"+item.TaxID+"|"+item.Name)
	direction := "销项"
	if role == qixinImportantSupplierRole {
		direction = "进项"
	}
	period := analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime)
	source := counterpartyInvoiceSource(role)
	name := d.text(key+".name", "发票交易主体", "object", item.Name, source)
	amount := d.number(key+".amount", "该主体"+direction+"发票交易金额", "amount", "元", period, "", "同方向精确主体的有符号发票金额，不是案件执行金额", item.Amount, name, source)
	denominator := d.number(key+".denominator", direction+"发票总额（交易占比分母）", "amount", "元", period, "", "同方向全部已取得发票有符号净额，包括未识别及负数主体", total, source)
	share := d.number(key+".share", "该主体"+direction+"发票交易占比", "share", "%", period, "", "该主体同方向发票金额÷同方向发票总额×100；执行案件金额不参与", item.Amount/total*100, amount, denominator)
	d.focus(key, "该组占比只描述本企业与该主体的发票交易规模，分子为主体交易金额；公开执行案件金额在案件组中单独说明。", name, amount, denominator, share)
}

// Positive records from other queries prevent a judicial-only empty result
// from being described as an all-credit clean bill. Unknown queries stay unknown.
func addJudicialOtherCreditSummary(d *SectionAnalysisData, subject map[string]any) {
	name := mapText(subject, qixinSubjectNameField)
	for _, item := range []struct{ field, label string }{{qixinDetailField, "经营异常"}, {qixinSeriousIllegalField, "严重违法"}, {qixinAdministrativePunishmentField, "行政处罚"}} {
		result := qixinQueryRecords(subject, item.field)
		if len(result.Rows) == 0 {
			continue
		}
		text := fmt.Sprintf("%s已取得%s记录%d条；具体日期和当前处理状态见相应信用事项，不代表当前仍在处罚或异常状态。", name, item.label, len(result.Rows))
		if item.field != qixinAdministrativePunishmentField {
			text = name + "：" + qixinListingSummary(subject, item.field, item.label)
		}
		d.text("other-credit."+analysisToken(name+item.field), "其他信用查询已有记录", "state", text, "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects[name="+name+"]/"+item.field)
	}
}

// Industry benchmarks are optional. The own-business summary retains the exact
// category facts used by 9.1–9.4, without allocating company costs to products.
func analysisIndustrySummary(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "industry.summary"}
	var refs []string
	for _, category := range ctx.categories {
		key := "business-summary." + analysisToken(category.Name)
		name := d.text(key+".name", "已识别主要业务", "object", category.Name, "issued_invoices/chunks/groups")
		amount := d.number(key+".amount", "该业务销项净额", "amount", "元", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "沿用业务结构同品类有符号金额合计", category.Amount, name)
		share := d.number(key+".share", "该业务销项金额占比", "share", "%", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "沿用业务结构该品类金额/同方向总额×100", category.Share*100, name)
		refs = append(refs, name, amount, share)
	}
	if len(refs) > 0 {
		d.focus("own-business-summary", "归纳已开票业务的结构及已证实差异，选关键事实而非重读占比表，并提出与已有结论对应的持续监测指标。开票品类占比不是企业整体收入、成本或税负的归属比例；没有相应归属资料不判断某品类决定企业整体经营或税负。", refs...)
	}
	return d
}

func analysisCounterpartyEvents(ctx *reportContext) []SectionAnalysisData {
	var result []SectionAnalysisData
	for _, role := range []string{qixinImportantCustomerRole, qixinImportantSupplierRole} {
		key := "invoice.customer-credit"
		if role == qixinImportantSupplierRole {
			key = "invoice.supplier-credit"
		}
		d := SectionAnalysisData{SectionID: key}
		for _, subject := range qixinSubjects(ctx) {
			if mapText(subject, qixinSubjectRoleField) != role {
				continue
			}
			name := mapText(subject, qixinSubjectNameField)
			if link := qixinInvoiceNameLink(ctx, subject); link != "" {
				d.text("identity-name."+analysisToken(name), "交易对手曾用名及现名对应", "identity", link, "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects")
			}
			addQixinListingCounts(&d, subject)
			addCounterpartyCreditRank(&d, ctx, subject, role)
			addCounterpartyExecutionEvidence(&d, ctx, subject, role)
			addCounterpartyOtherCreditEvidence(&d, ctx, subject, role)
			abnormal, _, _ := findFirst(qixinSubjectData(subject, qixinDetailField), "AbnormalItems")
			for _, row := range qixinItemMaps(abnormal, "InReason", "InDate", "OutDate") {
				date, reason := mapText(row, "InDate"), mapText(row, "InReason")
				prefix := "credit-event." + analysisToken(name+"|"+date+"|"+reason)
				source := "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects[name=" + name + "]/detail/AbnormalItems[InDate=" + date + "]"
				refs := []string{d.text(prefix+".name", "有经营异常记录的交易对手", "object", name, source)}
				refs = append(refs, d.text(prefix+".state", "经营异常当前处理状态", "state", qixinListingState(row), source))
				if date != "" {
					refs = append(refs, d.text(prefix+".date", "经营异常列入日期", "period", date, source))
				}
				if reason != "" {
					refs = append(refs, d.text(prefix+".reason", "公开列入原因", "state", reason, source))
				}
				if share, ok := invoiceCounterpartyShare(ctx, role, subject); ok {
					refs = append(refs, d.number(prefix+".share", "该交易对手同方向交易占比", "share", "%", "", "", "已精确核验交易占比×100", share*100, counterpartyInvoiceSource(role)))
				}
				if len(d.Focus) < 3 {
					d.focus(prefix, "定位有经营异常记录的交易对手、交易占比、列入时间及公开原因；未取得移出日期不等于已证实当前尚未处理。", refs...)
				}
			}
		}
		d.Constraints = []string{"只使用精确匹配的交易对手；未返回移出日期时保留当前状态待核实，不将登记存续等同于没有经营异常。"}
		result = append(result, d)
	}
	return result
}

// Rank uses the full signed invoice ranking, not the queried or matched subset.
// Equal amounts share the same rank; identity matching remains unchanged.
func addCounterpartyCreditRank(d *SectionAnalysisData, ctx *reportContext, subject map[string]any, role string) {
	item, _, ok := invoiceCounterpartyAmounts(ctx, role, subject)
	if !ok {
		return
	}
	groups := ctx.issued
	if role == qixinImportantSupplierRole {
		groups = ctx.received
	}
	rank := 1
	for _, other := range invoiceCounterpartyItems(groups) {
		if other.Amount > item.Amount {
			rank++
		}
	}
	key := "credit-trade." + analysisToken(item.TaxID+"|"+item.Name)
	ref := counterpartyInvoiceSource(role)
	d.text(key+".name", "交易排名主体", "object", item.Name, ref)
	d.number(key+".rank", "同方向全体交易对手金额排名", "rank", "名", "", "", "一加同方向全体交易对手中净额更大的主体数；同额并列，未查或未匹配主体仍参与排名", float64(rank), ref)
	d.number(key+".amount", "该主体同方向交易净额", "amount", "元", "", "", "与发票排名表同源有符号净额", item.Amount, ref)
	d.number(key+".share", "该主体同方向交易占比", "share", "%", "", "", "与发票排名表同分母的净额占比", item.Share*100, ref)
}

func analysisSensitivePurchases(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "invoice.sensitive-services"}
	type group struct {
		amount         float64
		refs           []string
		name, category string
		count          int
	}
	groups := map[string]*group{}
	for _, invoice := range ctx.received {
		for i, line := range invoiceFactLines(invoice) {
			if !line.HasAmount {
				continue
			}
			category := sensitiveServiceCategory(invoiceLineSearchText(line))
			if category == "" {
				continue
			}
			key := category + "|" + invoice.CounterpartyID + "|" + invoice.Counterparty
			g := groups[key]
			if g == nil {
				g = &group{name: invoice.Counterparty, category: category}
				groups[key] = g
			}
			g.amount += line.Amount
			g.count += line.Count
			g.refs = append(g.refs, fmt.Sprintf("source-snapshot.json/moduleResults[code=received_invoices]/data/chunks/groups[id=%s]/lines[%d]", invoice.ID, i))
		}
	}
	// A whole-report financial revenue denominator is available only when an
	// actual financial statement covers exactly this requested period.
	income, incomeOK := 0.0, false
	incomeRef := ""
	for _, f := range ctx.financial {
		if f.HasRevenue && f.TaxPeriod.Start == ctx.task.StartTime && f.TaxPeriod.End == ctx.task.EndTime && f.Revenue > 0 {
			income, incomeOK = f.Revenue, true
			incomeRef = "income_statements/" + f.Period + "/营业收入"
			break
		}
	}
	best := ""
	byCategory, total := sensitiveServiceTotals(ctx.received)
	period := ctx.task.StartTime + "至" + ctx.task.EndTime
	source := "source-snapshot.json/moduleResults[code=received_invoices]/发票汇总"
	ref := d.number("sensitive-total.purchases.amount", "进项购进敏感服务不含税金额", "amount", "元", period, "", "仅进项匹配明细的有符号不含税金额相加", total.amount, source)
	d.number("sensitive-total.purchases.count", "进项购进敏感服务明细条数", "coverage", "条", period, "", "匹配明细条数，不是发票份数", float64(total.count), source)
	if incomeOK {
		d.number("sensitive-total.purchases.revenue-share", "敏感服务购进/同期财报营业收入", "share", "%", period, "", "仅进项购进金额/同期间营业收入×100", total.amount/income*100, ref, incomeRef)
	}
	for _, category := range sortedMapKeys(byCategory) {
		item := byCategory[category]
		prefix := "sensitive-category." + analysisToken(category)
		d.text(prefix+".category", "购进发票类别", "object", category, source)
		amount := d.number(prefix+".amount", category+"购进不含税金额", "amount", "元", period, "", "与本节图表和表格共用进项分类汇总，红字按有符号金额抵减", item.amount, source)
		d.text(prefix+".attention", "该类服务关注事项", "scope", sensitiveServiceAttention(category), source)
		if incomeOK {
			d.number(prefix+".revenue-share", category+"购进/同期财报营业收入", "share", "%", period, "", "该类进项购进金额/同期间营业收入×100", item.amount/income*100, amount, incomeRef)
		}
	}
	d.focus("sensitive-total", "图表、表格与分析统一采用本次查询期间内的进项购进金额；选主要类别及供应商说明关注方向。", metricID(d.SectionID, "sensitive-total.purchases.amount"), metricID(d.SectionID, "sensitive-total.purchases.count"))
	covered := map[string]bool{}
	parties := invoiceCounterpartyItems(ctx.received)
	for _, key := range sortedMapKeys(groups) {
		g := groups[key]
		if !covered[g.name] {
			covered[g.name] = true
			for _, party := range parties {
				if party.Name == g.name {
					analysisPartyCoverage(ctx, &d, party, qixinImportantSupplierRole)
				}
			}
		}
		prefix := "sensitive-purchase." + analysisToken(key)
		d.text(prefix+".supplier", "敏感服务购进供应商", "object", g.name, g.refs...)
		d.text(prefix+".category", "敏感服务购进类别", "object", g.category, g.refs...)
		d.number(prefix+".amount", "该供应商该类服务购进金额", "amount", "元", ctx.task.StartTime+"至"+ctx.task.EndTime, "", "仅进项对应明细的不含税有符号金额相加，不混入销项", g.amount, g.refs...)
		d.number(prefix+".count", "该类购进明细条数", "coverage", "笔", ctx.task.StartTime+"至"+ctx.task.EndTime, "", "按符合当前类别的明细行计数，不等于发票份数", float64(g.count), g.refs...)
		if incomeOK {
			d.number(prefix+".share", "该供应商该类购进/同报告期间财报收入", "share", "%", ctx.task.StartTime+"至"+ctx.task.EndTime, "", "该供应商该类进项金额/同范围财报营业收入×100", g.amount/income*100, append(g.refs, incomeRef)...)
		}
		if best == "" || g.amount > groups[best].amount {
			best = key
		}
	}
	if best != "" && groups[best].amount > 0 {
		prefix := "sensitive-purchase." + analysisToken(best)
		refs := []string{metricID(d.SectionID, prefix+".supplier"), metricID(d.SectionID, prefix+".category"), metricID(d.SectionID, prefix+".amount"), metricID(d.SectionID, prefix+".count")}
		if incomeOK {
			refs = append(refs, metricID(d.SectionID, prefix+".share"))
		}
		d.focus("sensitive-purchase-object", "选出敏感服务购进金额较集中的供应商、类别、金额和条数；同口径收入可得时量化占比，不由品名认定不合理。", refs...)
	}
	d.Constraints = []string{"本节图表、表格、分类金额和正文均只使用进项；时间范围为本次报告的完整查询期间，不固定或截取近12个月。", "未取得覆盖整个报告期间的可比财报收入时，不计算该范围的购进/收入比例；合同、用途和交付证据缺失时不推断业务性质。"}
	return d
}
