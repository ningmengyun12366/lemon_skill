package report

// analysisIndustryScope exposes the bases already used by the invoice chart
// and category table. It does not allocate financial revenue to invoice classes
// or change either surface's existing percentage formula.
func analysisIndustryScope(ctx *reportContext, section string) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: section}
	period := analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime)
	total, known := 0.0, len(ctx.issued) > 0
	for _, g := range ctx.issued {
		total += g.Amount
		known = known && g.HasAmount
	}
	source := "source-snapshot.json/moduleResults[code=issued_invoices]/data/chunks/groups"
	totalRef := d.number("industry-invoice.scope.total", "同期间销项发票不含税净额", "amount", "元", period, "", "与业务结构图一致，按销项统计组有符号金额合计；不等于财报营业收入", total, source)
	if !known {
		d.Metrics[len(d.Metrics)-1].Status = "missing"
	}
	selected := 0.0
	for _, c := range ctx.categories {
		selected += c.Amount
	}
	d.number("industry-invoice.scope.selected", "表列主要品类金额合计", "sum", "元", period, "", "沿用业务结构表已列品类金额合计", selected, source+"/allocations")
	if !known || len(ctx.categories) == 0 {
		d.Metrics[len(d.Metrics)-1].Status = "missing"
	}
	// The category table uses all positive identified category net amounts,
	// whereas the completed chart uses the full issued amount including Other.
	byName := map[string]float64{}
	for _, a := range invoiceCategoryAllocations(ctx.issued) {
		if !isUnidentifiedInvoiceCategory(a.Name) {
			byName[a.Name] += a.Amount
		}
	}
	basis := 0.0
	for _, name := range sortedMapKeys(byName) {
		if byName[name] > 0 {
			basis += byName[name]
		}
	}
	d.number("industry-invoice.scope.category-base", "品类表占比分母：正净额已识别品类合计", "amount", "元", period, "", "沿用主要业务表占比的原分母，不替换为财报营业收入或图表含其他总额", basis, source+"/allocations")
	if !known || basis <= 0 {
		d.Metrics[len(d.Metrics)-1].Status = "missing"
	}
	// Reuse the chart's eligibility and Other value, including invalid/negative
	// cases; a missing chart never becomes a fabricated zero remainder.
	chart := &Chart{Labels: []string{}, Series: []Series{{Values: []float64{}}}}
	for _, c := range ctx.categories {
		chart.Labels = append(chart.Labels, c.Name)
		chart.Series[0].Values = append(chart.Series[0].Values, c.Amount)
	}
	completeBusinessPie(ctx, chart)
	if known && len(ctx.categories) > 0 && len(chart.Series) == 1 {
		other := 0.0
		if len(chart.Labels) > len(ctx.categories) {
			other = chart.Series[0].Values[len(ctx.categories)]
		}
		d.number("industry-invoice.scope.other", "图表其他金额（主要品类以外）", "amount", "元", period, "", "直接沿用业务结构图补全的其他金额；其他不等于未识别或未分类收入", other, totalRef)
	}
	d.text("industry-invoice.scope.coverage", "业务分类覆盖口径", "coverage", "业务表列主要已识别品类；图表其他为未单列部分，并非全部未识别。当前分类不能分配财报营业收入，不能用财报收入减发票分类额推算未分类收入。", totalRef)
	for _, f := range ctx.financial {
		if !f.HasRevenue {
			continue
		}
		d.number("industry-financial."+analysisToken(f.Period)+".revenue", "财报营业收入（独立口径，不作为发票分类分母）", "amount", "元", f.Period, "", "沿用已归一化的财报所属期收入；年度与月季分别列示，不重复累加", f.Revenue, "income_statements/"+f.Period)
	}
	return d
}
