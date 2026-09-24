package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	taxBurdenMonthlyTitle   = "全税种税负率月趋势"
	taxBurdenQuarterlyTitle = "全税种税负率季度趋势"
	taxBurdenMixedTitle     = "全税种税负率趋势"
	taxBurdenSectionKey     = "tax-payment.monthly-burden"
)

// burdenRevenue retains the denominator and the source inputs before rounding.
type burdenRevenue struct {
	value  float64
	known  bool
	basis  string
	label  string
	inputs []CalculationInput
}

type taxBurdenPoint struct {
	period               TaxPeriod
	revenue              burdenRevenue
	selfPaid, allPaid    float64
	paidKnown, selfKnown bool
	notice               string
	paymentInputs        []CalculationInput
	selfPaymentInputs    []CalculationInput
}

// burdenVATFacts includes earlier same-year declarations only as arithmetic
// references. They never extend the requested report axis.
func burdenVATFacts(ctx *reportContext) []vatFact {
	facts := append([]vatFact(nil), ctx.vat...)
	facts = append(facts, vatFacts(yearToDateReferenceModule(ctx.modules["vat_returns"]))...)
	return dedupeVATFacts(facts)
}

func burdenPeriod(period TaxPeriod, display string) TaxPeriod {
	if period.Start == "" || period.End == "" {
		start, end := periodBounds(display)
		if start == "" || end == "" {
			return TaxPeriod{}
		}
		period.Start, period.End = start, end
	}
	period.BucketMonth = extractMonth(period.End)
	period.Cycle = filingCycle(period.Start, period.End, "", "")
	period.Display = taxPeriodDisplay(period.Start, period.End, period.Cycle)
	if period.Cycle == taxCycleQuarterly {
		period.Display = normalizeQuarter(period.End)
	}
	return period
}

// burdenFilingPeriods also retains declarations whose revenue fields are
// missing, so their cycle does not disappear along with an unavailable metric.
func burdenFilingPeriods(ctx *reportContext) []TaxPeriod {
	byBounds := map[string]TaxPeriod{}
	add := func(p TaxPeriod) {
		p = burdenPeriod(p, p.Display)
		if p.Cycle == taxCycleMonthly || p.Cycle == taxCycleQuarterly {
			byBounds[p.Start+"|"+p.End] = p
		}
	}
	for _, f := range ctx.vat {
		add(burdenPeriod(f.TaxPeriod, f.Period))
	}
	records, _ := namedCollectionMapsWithPresence(ctx.modules["vat_returns"].Raw, "effectiveRecords")
	for _, record := range records {
		if p, ok := taxPeriodFromRecord(record, false); ok {
			add(p)
		}
	}
	for _, f := range ctx.declarations {
		if f.TaxName == "增值税" {
			add(f.Period)
		}
	}
	result := make([]TaxPeriod, 0, len(byBounds))
	for _, key := range sortedMapKeys(byBounds) {
		result = append(result, byBounds[key])
	}
	return result
}

// burdenPeriods follows actual VAT filing cycles, preserving missing periods
// and mid-range monthly/quarterly switches without duplicating any month.
func burdenPeriods(ctx *reportContext) []TaxPeriod {
	filings := burdenFilingPeriods(ctx)
	quarterlyOnly := len(filings) > 0
	for _, p := range filings {
		quarterlyOnly = quarterlyOnly && p.Cycle == taxCycleQuarterly
	}
	start, e1 := time.Parse("2006-01-02", ctx.task.StartTime)
	end, e2 := time.Parse("2006-01-02", ctx.task.EndTime)
	if e1 != nil || e2 != nil {
		return nil
	}
	var result []TaxPeriod
	for cursor := start; !cursor.After(end); {
		qStart, qEnd := quarterPeriodBounds(normalizeQuarter(cursor.Format("2006-01-02")))
		quarter := quarterlyOnly
		for _, p := range filings {
			if p.Cycle == taxCycleQuarterly && p.Start == qStart && p.End == qEnd {
				quarter = true
			}
		}
		until := time.Date(cursor.Year(), cursor.Month()+1, 0, 0, 0, 0, 0, cursor.Location())
		if quarter {
			until, _ = time.Parse("2006-01-02", qEnd)
		}
		if until.After(end) {
			until = end
		}
		p := burdenPeriod(TaxPeriod{Start: cursor.Format("2006-01-02"), End: until.Format("2006-01-02"), Basis: periodBasisTaxPeriod}, "")
		// An incomplete boundary quarter remains an explicit date range. It
		// cannot consume a full-quarter income or masquerade as a monthly return.
		if quarter && (p.Start != qStart || p.End != qEnd) {
			p.Cycle, p.Display = taxCycleIrregular, p.Start+" 至 "+p.End
		}
		result = append(result, p)
		cursor = until.AddDate(0, 0, 1)
	}
	return result
}

func taxBurdenTitle(ctx *reportContext) string {
	monthly, quarterly := false, false
	for _, p := range burdenFilingPeriods(ctx) {
		monthly = monthly || p.Cycle == taxCycleMonthly
		quarterly = quarterly || p.Cycle == taxCycleQuarterly
	}
	if quarterly && monthly {
		return taxBurdenMixedTitle
	}
	if quarterly {
		return taxBurdenQuarterlyTitle
	}
	return taxBurdenMonthlyTitle
}

func burdenSource(module string, p TaxPeriod) string {
	return fmt.Sprintf("source-snapshot.json/moduleResults[code=%s]/data[所属期=%s至%s]", module, p.Start, p.End)
}

// burdenVATInputs links the exact Chinese row, column group and archived field
// IDs when the archive reproduces the parsed cumulative total. Reference-only
// records without cell archives retain their source module and period instead.
func burdenVATInputs(ctx *reportContext, p TaxPeriod, value float64, subtract bool) []CalculationInput {
	prefix := ""
	if subtract {
		prefix = "扣减上期："
	}
	labels := append(append([]string(nil), vatSalesRowLabels...), smallVATSalesRowLabels...)
	allowed := map[string]bool{}
	for _, label := range labels {
		allowed[canonicalFormLabel(label)] = true
	}
	var inputs []CalculationInput
	total := 0.0
	for _, f := range ctx.taxFields {
		if f.Module != "vat_returns" || f.Period.Start != p.Start || f.Period.End != p.End || f.Status != "present" || !allowed[canonicalFormLabel(f.RowLabel)] || !strings.Contains(f.ColumnLabel, "本年累计") {
			continue
		}
		input := taxCalculationInput(f)
		input.Label = prefix + f.RowLabel + " · " + f.Group + " · " + f.ColumnLabel
		inputs = append(inputs, input)
		total += f.Value
	}
	if len(inputs) > 0 && math.Abs(total-value) < 0.000001 {
		return inputs
	}
	return []CalculationInput{{Source: burdenSource("vat_returns", p), Label: prefix + "增值税主表指定销售额栏次本年累计合计", Value: value, Present: true}}
}

func burdenFinancialInputs(ctx *reportContext, p TaxPeriod, value float64, cumulative bool) []CalculationInput {
	for _, f := range ctx.taxFields {
		if f.Module != "income_statements" || f.Period.Start != p.Start || f.Period.End != p.End || f.Status != "present" || normalizeFinancialMetricLabel(f.RowLabel) != financialMetricRevenue || math.Abs(f.Value-value) > 0.000001 {
			continue
		}
		isCumulative := strings.Contains(f.ColumnLabel, "本年累计")
		if isCumulative != cumulative || (!cumulative && !containsAny(f.ColumnLabel, "本月", "本期")) {
			continue
		}
		return []CalculationInput{taxCalculationInput(f)}
	}
	label := "利润表同期营业收入"
	if cumulative {
		label = "利润表本年累计营业收入"
	}
	return []CalculationInput{{Source: burdenSource("income_statements", p), Label: label, Value: value, Present: true}}
}

// burdenVATRevenue restores a single filing period from the Word-prescribed
// year-to-date VAT row totals. January/Q1 have an explicit zero opening total.
// A missing predecessor is never assumed to be zero. The approved new VAT form
// has only current-period 全部销售额, which is used directly when returned.
func burdenVATRevenue(ctx *reportContext, p TaxPeriod) burdenRevenue {
	result := burdenRevenue{basis: "缺少增值税同期收入或前期累计数据", label: "同期增值税申报销售额"}
	facts := burdenVATFacts(ctx)
	var current *vatFact
	for i := range facts {
		if facts[i].TaxPeriod.Start == p.Start && facts[i].TaxPeriod.End == p.End {
			current = &facts[i]
			break
		}
	}
	if current == nil {
		return result
	}
	currentValue, currentKnown := vatCumulativeSales(ctx, p)
	if currentKnown {
		previousValue, previousKnown := 0.0, p.Start == p.End[:4]+"-01-01"
		start, _ := time.Parse("2006-01-02", p.Start)
		previous := TaxPeriod{End: start.AddDate(0, 0, -1).Format("2006-01-02")}
		for _, f := range facts {
			if f.TaxPeriod.End == previous.End {
				previous = f.TaxPeriod
				break
			}
		}
		if !previousKnown {
			previousValue, previousKnown = vatCumulativeSales(ctx, previous)
		}
		result.inputs = burdenVATInputs(ctx, p, currentValue, false)
		if p.Start != p.End[:4]+"-01-01" {
			if previousKnown {
				result.inputs = append(result.inputs, burdenVATInputs(ctx, previous, previousValue, true)...)
			} else {
				result.inputs = append(result.inputs, CalculationInput{Source: burdenSource("vat_returns", previous), Label: "上个所属期末销售额本年累计合计", Present: false})
			}
		}
		if previousKnown {
			result.value, result.known, result.basis = currentValue-previousValue, true, "增值税指定栏次本年累计差分还原同期收入"
			return result
		}
	}
	for _, field := range ctx.taxFields {
		if field.Module == "vat_returns" && field.RecordID == current.RecordID && field.FormCode == vatNewMainFormCode && canonicalFormLabel(field.RowLabel) == "全部销售额" && current.HasSales {
			return burdenRevenue{value: current.Sales, known: true, label: "同期增值税申报销售额", basis: "新版增值税主表全部销售额本期数", inputs: []CalculationInput{{Source: burdenSource("vat_returns", p), Label: "新版增值税主表全部销售额本期数", Value: current.Sales, Present: true}}}
		}
	}
	return result
}

// burdenQuarterRevenue uses a matching income statement first. VAT is a
// fallback only when no income statement for this quarter was returned, not
// when an existing statement has a zero or missing income field.
func burdenQuarterRevenue(ctx *reportContext, p TaxPeriod) burdenRevenue {
	filed := false
	records, _ := namedCollectionMapsWithPresence(ctx.modules["income_statements"].Raw, "records")
	for _, r := range records {
		if rp, ok := taxPeriodFromRecord(r, false); ok && rp.Start == p.Start && rp.End == p.End {
			filed = true
		}
	}
	for _, f := range ctx.financial {
		fp := burdenPeriod(f.TaxPeriod, f.Period)
		if fp.Start != p.Start || fp.End != p.End {
			continue
		}
		filed = filed || f.HasRevenue || f.HasCumulativeRevenue
		if f.HasRevenue {
			return burdenRevenue{value: f.Revenue, known: true, label: "同期财报营业收入", basis: "同期利润表本期营业收入", inputs: burdenFinancialInputs(ctx, p, f.Revenue, false)}
		}
		if !f.HasCumulativeRevenue {
			continue
		}
		inputs := burdenFinancialInputs(ctx, p, f.CumulativeRevenue, true)
		if p.Start == p.End[:4]+"-01-01" {
			return burdenRevenue{value: f.CumulativeRevenue, known: true, label: "同期财报营业收入", basis: "首季利润表本年累计营业收入", inputs: inputs}
		}
		prior := append([]financialFact(nil), ctx.financial...)
		prior = append(prior, financialFacts(yearToDateReferenceModule(ctx.modules["income_statements"]), ModuleSnapshot{})...)
		start, _ := time.Parse("2006-01-02", p.Start)
		for _, previous := range prior {
			pp := burdenPeriod(previous.TaxPeriod, previous.Period)
			priorValue, priorKnown := financialCumulativeRevenue(previous)
			if pp.Cycle != taxCycleQuarterly || pp.End != start.AddDate(0, 0, -1).Format("2006-01-02") || !priorKnown {
				continue
			}
			previousInputs := burdenFinancialInputs(ctx, pp, priorValue, previous.HasCumulativeRevenue)
			for i := range previousInputs {
				previousInputs[i].Label = "扣减上季：" + previousInputs[i].Label
			}
			inputs = append(inputs, previousInputs...)
			return burdenRevenue{value: f.CumulativeRevenue - priorValue, known: true, label: "同期财报营业收入", basis: "利润表本年累计差分还原单季收入", inputs: inputs}
		}
	}
	if filed {
		return burdenRevenue{basis: "已取得季度财报，但缺少同期营业收入或前期累计数据"}
	}
	return burdenVATRevenue(ctx, p)
}

func taxBurdenPoints(ctx *reportContext) []taxBurdenPoint {
	var points []taxBurdenPoint
	filings := burdenFilingPeriods(ctx)
	for _, p := range burdenPeriods(ctx) {
		point := taxBurdenPoint{period: p, selfKnown: true}
		switch p.Cycle {
		case taxCycleMonthly:
			point.revenue = burdenVATRevenue(ctx, p)
		case taxCycleQuarterly:
			point.revenue = burdenQuarterRevenue(ctx, p)
		default:
			point.notice = "所属期不完整，待补充同期数据"
		}
		for _, other := range filings {
			if other.Start <= p.End && other.End >= p.Start && (other.Start != p.Start || other.End != p.End) {
				point.notice = "申报所属期重叠或不完整，待核实"
			}
		}
		for _, paid := range ctx.payments {
			month, usable := paymentTrendMonth(paid)
			if !usable {
				continue
			}
			if month < extractMonth(p.Start) || month > extractMonth(p.End) {
				continue
			}
			point.paidKnown = true
			point.allPaid += paid.Amount
			if paid.SelfPaid {
				point.selfPaid += paid.Amount
				point.selfPaymentInputs = append(point.selfPaymentInputs, CalculationInput{Source: paid.SourceRef, Label: paid.TaxName + "企业承担实缴金额", Value: paid.Amount, Present: true})
			}
			point.selfKnown = point.selfKnown && paid.TaxName != ""
			point.paymentInputs = append(point.paymentInputs, CalculationInput{Source: paid.SourceRef, Label: paid.TaxName + "实缴金额", Value: paid.Amount, Present: true})
			if paid.TaxPeriod.Basis == periodBasisPaymentDate {
				point.notice = "部分缴款缺少税款所属期，待核实同期归属"
			}
		}
		if point.notice == "" {
			switch {
			case !point.revenue.known:
				point.notice = point.revenue.basis
			case point.revenue.value == 0:
				point.notice = "同期收入为零，不计算税负率"
			case !point.paidKnown:
				point.notice = "待补充同期缴款数据"
			}
		}
		points = append(points, point)
	}
	return points
}

func (p taxBurdenPoint) ratios() (self, all float64, selfOK, allOK bool) {
	allOK = p.notice == "" && p.revenue.known && p.revenue.value != 0 && p.paidKnown
	selfOK = allOK && p.selfKnown
	if allOK {
		all = p.allPaid / p.revenue.value
	}
	if selfOK {
		self = p.selfPaid / p.revenue.value
	}
	return
}

// taxBurdenBlocks renders the actual month/quarter axis and two rates. All
// denominator inputs and formulas remain in analysis assets, not extra tables.
func taxBurdenBlocks(ctx *reportContext) []Block {
	points := taxBurdenPoints(ctx)
	title := taxBurdenTitle(ctx)
	rows := make([][]string, 0, len(points))
	groups := map[string][]taxBurdenPoint{}
	split := len(completeTrendPeriods(ctx.task.StartTime, ctx.task.EndTime, false)) > 18
	for _, p := range points {
		self, all, selfOK, allOK := p.ratios()
		notice := textOr(p.notice, "—")
		if allOK && !selfOK {
			notice = "税费性质待核实，暂不计算企业自身税负率"
		}
		rows = append(rows, []string{p.period.Display, optionalPercent(self, selfOK), optionalPercent(all, allOK), notice})
		year := ""
		if split {
			year = p.period.End[:4]
		}
		groups[year] = append(groups[year], p)
	}
	var blocks []Block
	for _, year := range sortedMapKeys(groups) {
		id, chartTitle := "monthly-tax-burden", title
		if year != "" {
			id += "-" + year
			chartTitle = year + "年" + title
		}
		chart := &Chart{ID: id, Kind: chartKindMultiLine, Title: chartTitle, Unit: "%", Series: []Series{{Name: "企业自身综合税负率", Color: chartColorPrimary}, {Name: "全税费缴纳率", Color: chartColorPositive}}}
		for _, p := range groups[year] {
			self, all, selfOK, allOK := p.ratios()
			chart.Labels = append(chart.Labels, p.period.Display)
			chart.Series[0].Values = append(chart.Series[0].Values, self*100)
			chart.Series[0].Available = append(chart.Series[0].Available, selfOK)
			chart.Series[1].Values = append(chart.Series[1].Values, all*100)
			chart.Series[1].Available = append(chart.Series[1].Available, allOK)
		}
		blocks = append(blocks, Block{Kind: BlockChart, Title: chartTitle, Chart: chart})
	}
	tableTitle := "综合税负率"
	if title == taxBurdenMonthlyTitle {
		tableTitle = "月度综合税负率"
	}
	if title == taxBurdenQuarterlyTitle {
		tableTitle = "季度综合税负率"
	}
	return append(blocks, Block{Kind: BlockTable, Title: tableTitle, Headers: []string{"所属期", "企业自身综合税负率", "全税费缴纳率", "异常情况"}, Rows: rows})
}

func taxBurdenTitleMatches(title string) bool {
	for _, expected := range []string{taxBurdenMonthlyTitle, taxBurdenQuarterlyTitle, taxBurdenMixedTitle} {
		if title == expected {
			return true
		}
		if strings.HasSuffix(title, expected) {
			prefix := strings.TrimSuffix(title, expected)
			if _, err := time.Parse("2006年", prefix); err == nil {
				return true
			}
		}
	}
	return false
}

func analysisTaxBurden(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: taxBurdenSectionKey, Constraints: []string{"按真实月报/季报所属期计算；累计还原不得跨年或跳过缺失前期，季度收入不得平均拆月。", "季度/年度集中缴款按所属期结束月归集；仅有缴款日期的记录不用于同属期税负。"}}
	for _, p := range taxBurdenPoints(ctx) {
		prefix := "burden." + analysisToken(p.period.Start+"|"+p.period.End)
		add := func(key, name, unit, algorithm string, value float64, inputs []CalculationInput) {
			var refs []string
			for _, input := range inputs {
				if input.Source != "" {
					refs = append(refs, input.Source)
				}
			}
			sort.Strings(refs)
			d.number(prefix+key, name, "calculation", unit, p.period.Display, "", algorithm, value, refs...)
			d.Metrics[len(d.Metrics)-1].Inputs = inputs
		}
		if p.revenue.known {
			add(".revenue", p.revenue.label, "元", p.revenue.basis, p.revenue.value, p.revenue.inputs)
		}
		if p.paidKnown {
			add(".paid", "同期全部税费实缴额", "元", "所属期结束月落入本期的缴款金额合计", p.allPaid, p.paymentInputs)
		}
		self, all, selfOK, allOK := p.ratios()
		inputs := append(append([]CalculationInput(nil), p.paymentInputs...), p.revenue.inputs...)
		if selfOK {
			selfPaymentInputs := p.selfPaymentInputs
			if len(selfPaymentInputs) == 0 {
				// A known zero after excluding every payment still needs the
				// original evidence; preserve its amount and mark the exclusion.
				for _, input := range p.paymentInputs {
					input.Label = "排除代扣代缴：" + input.Label
					selfPaymentInputs = append(selfPaymentInputs, input)
				}
			}
			selfInputs := append(append([]CalculationInput(nil), selfPaymentInputs...), p.revenue.inputs...)
			add(".self-paid", "同期企业承担税费实缴额", "元", "剔除代扣代缴项目后的实缴金额合计", p.selfPaid, selfPaymentInputs)
			add(".self-rate", "企业自身综合税负率", "%", "企业承担税费实缴额÷"+p.revenue.label+"×100，不含代扣代缴", self*100, selfInputs)
		}
		if allOK {
			add(".all-rate", "全税费缴纳率", "%", "全部税费实缴额÷"+p.revenue.label+"×100", all*100, inputs)
		}
		if p.notice != "" {
			d.Constraints = append(d.Constraints, p.period.Display+"："+p.notice)
		}
	}
	return d
}
