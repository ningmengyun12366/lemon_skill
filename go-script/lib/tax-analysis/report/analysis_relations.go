package report

import (
	"fmt"
	"math"
	"strings"
)

const (
	purchaseRatioHigh       = 105.0
	purchaseRatioLow        = 70.0
	purchaseRatioDeviation  = 20.0
	analysisBaselinePeriods = 3
)

// addSectionAnalysis supplies business-specific comparisons. Thresholds here
// select attention topics from the audited Word; they never create findings.
func addSectionAnalysis(d *SectionAnalysisData, model Model, section Section) {
	addTableAnalysis(d, section)
	switch section.Key {
	case "invoice.amount-trend":
		addPurchaseRatioAnalysis(d, section)
	case "invoice.red-output":
		addRedConcentrationAnalysis(d, section)
	case "vat.cycle-comparison":
		addVATDirectionAnalysis(d, model)
	case "vat.burden-trend", taxBurdenSectionKey:
		addBurdenFocus(d, section)
		if section.Key == taxBurdenSectionKey {
			addTaxBurdenRelativeHistory(d)
		}
	case "cit.annual":
		addAdjustmentAnalysis(d, model)
	case "vat.invoice-declaration-difference":
		addInvoiceDeclarationScope(d, section)
	case "financial.period-expense":
		addMarginCombinationAnalysis(d, section)
	case "invoice.business-match":
		d.Constraints = append(d.Constraints, "英文名称、免税税率或经营范围中的进出口条款不证明交易方境外身份或该笔业务属于出口；没有已核验身份依据时使用原主体名称。")
	case "invoice.suppliers", "invoice.customers":
		d.Constraints = append(d.Constraints, "主体总额与品名总额相同不能证明归属。只有实际同一汇总组的主体与品名关联才可作对应分析，否则分别描述，不从相等金额推断采购频次、月份或全部商品来源。不要重述前十名单及每项金额，集中度在专门章节分析。")
	case "financial.statements":
		addRevenueProfitDirections(d, section)
	case "financial.working-capital":
		addBalanceConflictAnalysis(d, model, section)
	case "tax-payment.total-trend":
		addPaymentContributionAnalysis(d, model, section)
	case "tax-payment.composition":
		addPaymentCompositionAnalysis(d, section)
	case "financial.summary":
		d.RelatedSections = []string{"financial.statements", "financial.period-expense", "financial.debt-asset", "financial.working-capital", "financial.cash-flow"}
	case "industry.summary":
		d.RelatedSections = []string{"industry.structure", "industry.business-1", "industry.business-2", "industry.business-3"}
		d.Constraints = append(d.Constraints, "归纳前述主要业务已取得的结构与分析结果。缺同行基准只限制同行比较，不取消企业自身业务总结；企业整体指标不得硬分摊给某品类，样本覆盖不足时只归纳已取得范围。")
	case "industry.business-1", "industry.business-2", "industry.business-3":
		d.RelatedSections = []string{"industry.structure", "invoice.amount-trend", "invoice.customer-concentration", "invoice.supplier-concentration", "financial.period-expense", "financial.working-capital"}
		d.Constraints = append(d.Constraints, "关联章节指标属于企业整体，不得分摊到本品类；没有本业务归属及可验证关联时，本节可不写文字。")
	case "cit.research-expense":
		addResearchCumulativeAnalysis(d, model)
		d.Constraints = append(d.Constraints, "实际字段为研发加计扣除额；季报为截至季度末累计，年度为汇算，不能当研发费用或相加。")
	case "other-tax.union-fund":
		addTableAmountHistory(d, section, "申报金额")
	}
}

// addRevenueProfitDirections identifies each comparable revenue/profit direction
// conflict rather than inferring a trend from only the first and last period.
func addRevenueProfitDirections(d *SectionAnalysisData, s Section) {
	c := firstAnalysisChart(s)
	if c == nil {
		return
	}
	revenue, profit := -1, -1
	for i, series := range c.Series {
		if series.Name == "营业收入" {
			revenue = i
		}
		if series.Name == "利润总额" {
			profit = i
		}
	}
	if revenue < 0 || profit < 0 {
		return
	}
	var refs []string
	for i := 1; i < len(c.Labels); i++ {
		if !adjacentAnalysisPeriods(c.Labels[i-1], c.Labels[i]) {
			continue
		}
		if !analysisPointAvailable(*c, c.Series[revenue], i) || !analysisPointAvailable(*c, c.Series[revenue], i-1) || !analysisPointAvailable(*c, c.Series[profit], i) || !analysisPointAvailable(*c, c.Series[profit], i-1) {
			continue
		}
		r, p := c.Series[revenue].Values[i]-c.Series[revenue].Values[i-1], c.Series[profit].Values[i]-c.Series[profit].Values[i-1]
		if r <= 0 || p >= 0 {
			continue
		}
		prefix := "income-profit-direction." + analysisToken(c.Labels[i])
		refs = append(refs, d.number(prefix+".revenue-change", "收入上升区间的收入增加额", "change", "元", c.Labels[i], c.Labels[i-1], "本期营业收入−相邻上期营业收入", r, analysisChartRef(s, *c, c.Series[revenue], i), analysisChartRef(s, *c, c.Series[revenue], i-1)))
		refs = append(refs, d.number(prefix+".profit-change", "同区间利润总额变化", "change", "元", c.Labels[i], c.Labels[i-1], "本期利润总额−相邻上期利润总额", p, analysisChartRef(s, *c, c.Series[profit], i), analysisChartRef(s, *c, c.Series[profit], i-1)))
	}
	if len(refs) > 0 {
		d.focus("income-profit-direction", "说明收入上升但利润下降的具体相邻区间及两项变化金额，再结合已有成本费用解释；没有成因证据时保留核实方向。", refs...)
	}
}

// addPaymentCompositionAnalysis supplies the exact chart denominator and shares
// so annual settlements cannot be omitted from the prose denominator.
func addPaymentCompositionAnalysis(d *SectionAnalysisData, s Section) {
	c := firstAnalysisChart(s)
	if c == nil || len(c.Series) == 0 {
		return
	}
	total := 0.0
	for i := range c.Labels {
		if analysisPointAvailable(*c, c.Series[0], i) {
			total += c.Series[0].Values[i]
		}
	}
	if total <= 0 {
		return
	}
	totalRef := d.number("payment-composition.total", "税费构成合计", "amount", "元", "分析期间", "", "同一构成图所有项目之和", total, "report-model.json/sections[semanticId=tax-payment.composition]")
	var refs []string
	refs = append(refs, totalRef)
	for i, label := range c.Labels {
		if !analysisPointAvailable(*c, c.Series[0], i) {
			continue
		}
		prefix := "payment-composition." + analysisToken(label)
		amount := d.number(prefix+".amount", label+"构成金额", "amount", "元", "分析期间", "", "沿用构成图金额", c.Series[0].Values[i], analysisChartRef(s, *c, c.Series[0], i))
		share := d.number(prefix+".share", label+"占全部税费构成比例", "share", "%", "分析期间", "", "该税种金额/同一构成图合计×100", c.Series[0].Values[i]/total*100, amount, totalRef)
		refs = append(refs, amount, share)
	}
	d.focus("payment-composition", "使用本节完整构成合计和占比解释主要税种；实际存在年度单列缴款时一并纳入分母，是否发生以缴款记录为准。", refs...)
}

// addTaxBurdenRelativeHistory keeps relative percentage deviation distinct
// from the percentage-point difference already calculated for ratio charts.
func addTaxBurdenRelativeHistory(d *SectionAnalysisData) {
	byID := map[string]AnalysisMetric{}
	for _, m := range d.Metrics {
		byID[m.ID] = m
	}
	initial := append([]AnalysisMetric(nil), d.Metrics...)
	best := 0.0
	var bestRefs []string
	for _, base := range initial {
		if !strings.HasSuffix(base.ID, ".baseline-mean") || base.Unit != "%" || base.Value <= 0 || base.Status != "present" {
			continue
		}
		prefix := strings.TrimSuffix(base.ID, ".baseline-mean")
		current, ok := byID[prefix+".value"]
		if !ok || current.Status != "present" {
			continue
		}
		key := strings.TrimPrefix(prefix, "fact:"+d.SectionID+".metric.")
		value := (current.Value - base.Value) / base.Value * 100
		id := d.number(key+".baseline-relative-deviation", "综合税负率较前3个可比期均值相对偏离", "deviation", "%", base.Period, base.BasePeriod, "(本期税负率−前3个连续可比期税负率均值)÷该均值×100；不同于百分点差", value, current.ID, base.ID)
		if math.Abs(value) > math.Abs(best) {
			best = value
			bestRefs = []string{prefix + ".period", current.ID, base.ID, id}
		}
	}
	if len(bestRefs) > 0 {
		d.focus("burden-relative-history", "说明同一税负率相对前3个可比期均值的偏离及基期，不将相对百分比与百分点混用，不据此新增未确认风险。", bestRefs...)
	}
}

func addBurdenFocus(d *SectionAnalysisData, s Section) {
	addBurdenComparisonBundles(d)
	// Describe the largest observed deviation, without inventing a new risk
	// threshold or treating absolute low burden as noncompliance.
	best := -1
	for i, m := range d.Metrics {
		if strings.HasSuffix(m.ID, ".baseline-deviation") && m.Status == "present" && m.Unit == "百分点" && math.Abs(m.Value) >= 0.005 && (best < 0 || math.Abs(m.Value) > math.Abs(d.Metrics[best].Value)) {
			best = i
		}
	}
	if best < 0 {
		return
	}
	m := d.Metrics[best]
	prefix := strings.TrimSuffix(m.ID, ".baseline-deviation")
	basePeriod := d.text("burden-baseline.periods", "税负基准期间", "period", m.BasePeriod, m.SourceRefs...)
	d.focus("burden-history", "给出税负相对可比历史的明显变化、期间和基准，整组使用同一目标期的基准月份、均值及偏离，不与其他月份组拼接；不直接判断税负违规，没有原因证据不猜测。", prefix+".period", prefix+".value", prefix+".baseline-mean", m.ID, basePeriod, prefix+".baseline-comparison")
}

// addBurdenComparisonBundles keeps each rolling baseline and its result together
// in the reading card, so prose does not pair one window with another's delta.
func addBurdenComparisonBundles(d *SectionAnalysisData) {
	byID := map[string]AnalysisMetric{}
	for _, m := range d.Metrics {
		byID[m.ID] = m
	}
	initial := append([]AnalysisMetric(nil), d.Metrics...)
	for _, delta := range initial {
		if !strings.HasSuffix(delta.ID, ".baseline-deviation") || delta.Status != "present" || delta.Unit != "百分点" {
			continue
		}
		prefix := strings.TrimSuffix(delta.ID, ".baseline-deviation")
		current, cok := byID[prefix+".value"]
		mean, mok := byID[prefix+".baseline-mean"]
		if !cok || !mok || current.Status != "present" || mean.Status != "present" {
			continue
		}
		key := strings.TrimPrefix(prefix, "fact:"+d.SectionID+".metric.")
		summary := fmt.Sprintf("目标期%s，税负率%s%%；基准期%s，均值%s%%；本期减该均值%s个百分点。", delta.Period, current.Display, delta.BasePeriod, mean.Display, delta.Display)
		d.text(key+".baseline-comparison", "同组税负历史比较", "comparison", summary, current.ID, mean.ID, delta.ID)
	}
}

func addAdjustmentAnalysis(d *SectionAnalysisData, model Model) {
	totals := map[string]TaxFieldFact{}
	group := func(f TaxFieldFact) string {
		return fmt.Sprintf("%s|%s|%d|%s", f.RecordID, f.FormCode, f.Table, canonicalFormLabel(f.ColumnLabel))
	}
	valid := func(f TaxFieldFact) bool {
		return f.Module == "cit_annual_returns" && f.Status == "present" && (strings.Contains(f.FormCode, "A105000") || strings.Contains(f.FormName, "纳税调整项目明细表")) && declarationLabelMatches(f.ColumnLabel, []string{"调增金额", "调减金额"})
	}
	for _, f := range model.TaxFields {
		if valid(f) && canonicalFormLabel(f.RowLabel) == "合计" {
			totals[group(f)] = f
		}
	}
	best := map[string]TaxFieldFact{}
	bundles := map[string][]string{}
	for _, f := range model.TaxFields {
		if !valid(f) || f.Value <= 0 || canonicalFormLabel(f.RowLabel) == "合计" || strings.Contains(f.RowLabel, "类调整项目") {
			continue
		}
		prefix := "adjustment." + analysisToken(f.ID)
		source := "report-model.json/taxFields[id=" + f.ID + "]"
		direction := canonicalFormLabel(f.ColumnLabel)
		p := d.text(prefix+".period", "纳税调整期间", "period", f.Period.Display, source)
		name := d.text(prefix+".name", "纳税调整项目", "object", f.RowLabel, source)
		amount := d.number(prefix+".amount", f.RowLabel+direction, "amount", "元", f.Period.Display, "", "沿用已确认的纳税调整项目明细字段", f.Value, source)
		d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(f)}
		refs := []string{p, name, amount}
		total, ok := totals[group(f)]
		if ok && total.Value > 0 {
			id := d.number(prefix+".share", f.RowLabel+"占同方向调整合计比例", "share", "%", f.Period.Display, "", "本项目调增(或调减)/同报表同方向合计×100；不以净调整额作分母", f.Value/total.Value*100, source, "report-model.json/taxFields[id="+total.ID+"]")
			d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(f), taxCalculationInput(total)}
			refs = append(refs, id)
		}
		key := group(f)
		if f.Value > best[key].Value {
			best[key] = f
			bundles[key] = refs
		}
	}
	for _, key := range sortedMapKeys(best) {
		if len(d.Focus) >= 3 {
			break
		}
		d.focus("adjustment-"+analysisToken(key), "说明主要纳税调整项目、方向、金额及同方向合计中的占比，不把其他调整直接认定为无票或不合规。", bundles[key]...)
	}
}

// addTableAmountHistory uses the same generic arithmetic for an amount series
// whose original presentation is a table, retaining actual table references.
func addTableAmountHistory(d *SectionAnalysisData, s Section, header string) {
	for bi, b := range s.Blocks {
		column := -1
		for i, h := range b.Headers {
			if h == header {
				column = i
			}
		}
		if column < 0 {
			continue
		}
		c := Chart{ID: "table-history." + analysisToken(b.Title+"|"+header), Unit: "元"}
		series := Series{Name: header}
		mapping := map[string]string{}
		for ri, row := range b.Rows {
			if len(row) <= column {
				continue
			}
			if start, _ := periodBounds(row[0]); start == "" {
				continue
			}
			i := len(c.Labels)
			c.Labels = append(c.Labels, row[0])
			v, ok := parseFormNumber(row[column])
			series.Values = append(series.Values, v)
			series.Available = append(series.Available, ok)
			mapping[fmt.Sprintf("fact:%s.b%d.chart.s1.p%d", s.Key, bi+1, i+1)] = fmt.Sprintf("fact:%s.b%d.r%d.c%d", s.Key, bi+1, ri+1, column+1)
			mapping[fmt.Sprintf("fact:%s.b%d.chart.label.p%d", s.Key, bi+1, i+1)] = fmt.Sprintf("fact:%s.b%d.r%d.c1", s.Key, bi+1, ri+1)
		}
		c.Series = []Series{series}
		start := len(d.Metrics)
		addChartAnalysis(d, c, bi)
		for i := start; i < len(d.Metrics); i++ {
			for j, ref := range d.Metrics[i].SourceRefs {
				if real, ok := mapping[ref]; ok {
					d.Metrics[i].SourceRefs[j] = real
				}
			}
		}
	}
}

// addTableAnalysis exposes semantic cells and same-period difference coverage.
// It derives differences/growth only from amount columns, never from rounded
// percentage columns or text containing several unrelated numbers.
func addTableAnalysis(d *SectionAnalysisData, s Section) {
	if s.Key == "enterprise.changes" {
		return
	}
	for bi, b := range s.Blocks {
		if b.Kind != BlockTable {
			continue
		}
		for ci, header := range b.Headers {
			if ci == 0 {
				continue
			}
			count, nonzero := 0, 0
			periods, nonzeroPeriods := map[string]string{}, map[string]bool{}
			maxAbs, total := 0.0, 0.0
			var scope, refs []string
			for ri, row := range b.Rows {
				if ci >= len(row) || len(row) == 0 {
					continue
				}
				v, ok := parseFormNumber(row[ci])
				if !ok {
					continue
				}
				unit := "元"
				if strings.Contains(row[ci], "%") {
					unit = "%"
					// parseFormNumber returns a fraction for an explicit percent.
					v *= 100
				} else if strings.HasSuffix(header, "期数") || strings.HasSuffix(row[0], "期数") || containsAny(header+"|"+row[0], "期间数") {
					unit = "期"
				} else if containsAny(header+row[0], "客户数量", "供应商数量", "企业数量", "家数") {
					unit = "家"
				} else if containsAny(header+row[0], "数量", "笔数", "次数", "份数", "条数") {
					unit = "笔"
				}
				identity := row[0]
				for k := 1; k < ci; k++ {
					if _, numeric := parseFormNumber(row[k]); !numeric && row[k] != "—" {
						identity += "|" + row[k]
					}
				}
				// Repeated labels in multi-year tables are separate facts. Include
				// the frozen cell address so their evidence cannot overwrite itself.
				key := "table." + analysisToken(b.Title+"|"+header) + "." + analysisToken(fmt.Sprintf("%s|b%d|r%d|c%d", identity, bi+1, ri+1, ci+1))
				ref := fmt.Sprintf("fact:%s.b%d.r%d.c%d", s.Key, bi+1, ri+1, ci+1)
				period := row[0]
				if start, _ := periodBounds(period); start == "" {
					period = ""
				}
				algorithm := "最终业务表中的已确认金额/展示值；不由已舍入比例推算金额"
				if strings.HasPrefix(s.Key, "industry.business-") && row[0] == "覆盖期间数" {
					algorithm = "按存在该品类非零开票交易记录的月份去重计数；正红相抵后月净额为零的月份仍计入；该指标描述业务发生月份，资料完整性由发票查询覆盖情况另行判断"
				}
				d.number(key+".value", header+" · "+row[0], "table-value", unit, period, "", algorithm, v, ref)
				if !containsAny(header, "差额", "差异") || unit != "元" || period == "" || s.Key == "invoice.tax-comparison" {
					continue
				}
				count++
				start, end := periodBounds(period)
				periodKey := start + ":" + end
				periods[periodKey] = period
				total += v
				maxAbs = math.Max(maxAbs, math.Abs(v))
				if math.Abs(v) >= invoiceAggregationTolerance {
					nonzero++
					nonzeroPeriods[periodKey] = true
				}
				scope = append(scope, period)
				refs = append(refs, ref)
			}
			if count > 0 {
				prefix := "comparison." + analysisToken(b.Title+"|"+header)
				d.number(prefix+".record-count", header+"比较记录数", "coverage", "条", "", "", "有效差额单元格数；同月不同方向分别计一条", float64(count), refs...)
				d.number(prefix+".nonzero-record-count", header+"非零比较记录数", "coverage", "条", "", "", "绝对差额达到金额容差的比较记录数", float64(nonzero), refs...)
				d.number(prefix+".count", header+"可比期间数", "coverage", "期", "", "", "按所属期起止日期去重；同月不同方向只计一期", float64(len(periods)), refs...)
				d.number(prefix+".nonzero", header+"非零期间数", "coverage", "期", "", "", "至少一条比较达到金额容差的去重期间数", float64(len(nonzeroPeriods)), refs...)
				d.number(prefix+".maximum", header+"最大绝对差额", "extremum", "元", "", "", "比较全部有效期间的绝对差额", maxAbs, refs...)
				// Annual and quarterly values are not additive, so no total is
				// exposed unless every observed period is disjoint.
				if disjointAnalysisPeriods(scope) {
					sum := d.number(prefix+".sum", header+"有符号合计", "total", "元", "", "", "不重叠可比期间差额按原正负号相加；不是绝对差额之和", total, refs...)
					d.text(prefix+".signed-summary", header+"合计与绝对差额的区别", "basis",
						fmt.Sprintf("有符号差额合计%s元；最大绝对差额%s元。合计保留负号；正负抵销不表示各期无差异。", analysisNumber(total, "元"), analysisNumber(maxAbs, "元")), sum, metricID(s.Key, prefix+".maximum"))
				}
				scope = nil
				for _, key := range sortedMapKeys(periods) {
					scope = append(scope, periods[key])
				}
				for i := range d.Metrics {
					if strings.HasPrefix(d.Metrics[i].ID, metricID(s.Key, prefix)) {
						d.Metrics[i].Scope = scope
					}
				}
			}
		}
	}
}
func disjointAnalysisPeriods(periods []string) bool {
	for i, p := range periods {
		a, b := periodBounds(p)
		if a == "" {
			return false
		}
		for _, q := range periods[:i] {
			c, e := periodBounds(q)
			if a <= e && c <= b {
				return false
			}
		}
	}
	return true
}

func addPurchaseRatioAnalysis(d *SectionAnalysisData, s Section) {
	c := firstAnalysisChart(s)
	if c == nil || len(c.Series) < 2 {
		return
	}
	var sales, purchases *Series
	for i := range c.Series {
		switch c.Series[i].Name {
		case "销项不含税金额":
			sales = &c.Series[i]
		case "进项不含税金额":
			purchases = &c.Series[i]
		}
	}
	if sales == nil || purchases == nil {
		return
	}
	ratios := make([]float64, len(c.Labels))
	known := make([]bool, len(c.Labels))
	signals := make([]bool, len(c.Labels))
	best := map[bool]int{}
	bestDeviation := map[bool]float64{}
	bundles := map[int][]string{}
	for i, p := range c.Labels {
		if !analysisPointAvailable(*c, *sales, i) || !analysisPointAvailable(*c, *purchases, i) || sales.Values[i] <= invoiceAggregationTolerance {
			continue
		}
		known[i] = true
		ratios[i] = purchases.Values[i] / sales.Values[i] * 100
		prefix := "purchase-ratio." + analysisToken(p)
		inputs := []string{analysisChartRef(s, *c, *sales, i), analysisChartRef(s, *c, *purchases, i)}
		period := d.text(prefix+".period", "购销比较期间", "period", p, inputs...)
		ratio := d.number(prefix+".value", "购进/销项金额比例", "ratio", "%", p, "", "进项不含税金额/销项不含税金额×100", ratios[i], inputs...)
		diff := d.number(prefix+".difference", "购进超过销项的差额", "difference", "元", p, "", "进项不含税金额−销项不含税金额", purchases.Values[i]-sales.Values[i], inputs...)
		if i < analysisBaselinePeriods {
			continue
		}
		valid := true
		for j := i - analysisBaselinePeriods; j < i; j++ {
			if !known[j] || !adjacentAnalysisPeriods(c.Labels[j], c.Labels[j+1]) {
				valid = false
			}
		}
		if !valid {
			continue
		}
		baseline := medianAnalysis(ratios[i-analysisBaselinePeriods : i])
		deviation := ratios[i] - baseline
		base := d.number(prefix+".baseline", "前3个连续可比月比例中位数", "baseline", "%", p, strings.Join(c.Labels[i-analysisBaselinePeriods:i], "、"), "先逐月计算购进/销项比例，再取前3月中位数", baseline)
		for j := i - analysisBaselinePeriods; j < i; j++ {
			d.Metrics[len(d.Metrics)-1].SourceRefs = append(d.Metrics[len(d.Metrics)-1].SourceRefs, metricID(s.Key, "purchase-ratio."+analysisToken(c.Labels[j])+".value"))
		}
		dev := d.number(prefix+".deviation", "相对前3月中位数偏离", "deviation", "百分点", p, "", "本期比例−前3个连续可比月比例中位数", deviation, ratio, base)
		signals[i] = (ratios[i] > purchaseRatioHigh || ratios[i] < purchaseRatioLow) && math.Abs(deviation) >= purchaseRatioDeviation
		if signals[i] {
			streak := 1
			for j := i - 1; j >= 0 && signals[j] && (ratios[j] > purchaseRatioHigh) == (ratios[i] > purchaseRatioHigh) && adjacentAnalysisPeriods(c.Labels[j], c.Labels[j+1]); j-- {
				streak++
			}
			n := d.number(prefix+".streak", "同类购销偏离连续期数", "streak", "期", p, "", "统计相邻且同处购销比例高位或低位异常区间的月份；各月均满足比例阈值及相对各自前3个连续可比月中位数的绝对偏离条件；相对中位数的偏离正负不要求一致", float64(streak), dev)
			bundles[i] = []string{period, ratio, diff, base, dev, n}
			side := ratios[i] > purchaseRatioHigh
			if math.Abs(deviation) > bestDeviation[side] {
				best[side] = i
				bestDeviation[side] = math.Abs(deviation)
			}
		}
	}
	for _, side := range []bool{true, false} {
		if index, ok := best[side]; ok {
			d.focus(fmt.Sprintf("purchase-deviation-%t", side), "分别说明购销比例高位或低位异常区间中偏离幅度最明显月份的购销差额、比例、前期基准和偏离幅度；连续期数描述持续处于同一异常区间，相对各期中位数的上升或下降方向另行说明。", bundles[index]...)
		}
	}
}
func analysisChartRef(s Section, c Chart, series Series, index int) string {
	for bi, b := range s.Blocks {
		if b.Chart == nil || b.Chart.ID != c.ID {
			continue
		}
		for si, x := range b.Chart.Series {
			if x.Name == series.Name {
				return fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", s.Key, bi+1, si+1, index+1)
			}
		}
	}
	if index >= 0 && index < len(c.Labels) {
		for bi, b := range s.Blocks {
			if b.Chart == nil {
				continue
			}
			for si, x := range b.Chart.Series {
				if x.Name != series.Name {
					continue
				}
				for j, label := range b.Chart.Labels {
					if label == c.Labels[index] && analysisPointAvailable(*b.Chart, x, j) {
						return fmt.Sprintf("fact:%s.b%d.chart.s%d.p%d", s.Key, bi+1, si+1, j+1)
					}
				}
			}
		}
	}
	return ""
}

func addRedConcentrationAnalysis(d *SectionAnalysisData, s Section) {
	total := 0.0
	totalRef := ""
	best := 0.0
	name, nameRef, amountRef := "", "", ""
	for bi, b := range s.Blocks {
		for ri, row := range b.Rows {
			if len(row) < 2 {
				continue
			}
			v, ok := parseFormNumber(row[1])
			if !ok {
				continue
			}
			ref := fmt.Sprintf("fact:%s.b%d.r%d.c2", s.Key, bi+1, ri+1)
			if row[0] == "红字金额（绝对值）" {
				total = v
				totalRef = ref
			}
			if strings.HasPrefix(row[0], "主要红字客户 · ") && v > best {
				best = v
				name = strings.TrimPrefix(row[0], "主要红字客户 · ")
				nameRef = fmt.Sprintf("fact:%s.b%d.r%d.c1", s.Key, bi+1, ri+1)
				amountRef = ref
			}
		}
	}
	if total <= 0 || name == "" {
		return
	}
	object := d.text("red-customer.name", "主要红字客户", "object", name, nameRef)
	amount := d.number("red-customer.amount", "该客户红字绝对金额", "amount", "元", "", "", "同一客户的红字绝对金额汇总", best, amountRef)
	share := d.number("red-customer.share", "该客户占红字总额比例", "share", "%", "", "", "该客户红字绝对金额/全部红字绝对金额×100；与红字/蓝字比例区分", best/total*100, amountRef, totalRef)
	d.focus("red-customer", "定位红字金额最多的客户及其占全部红字金额比例，不猜红字原因。", object, amount, share)
}

func addVATDirectionAnalysis(d *SectionAnalysisData, model Model) {
	s := sectionByKey(model, "vat.period-overview")
	for bi, b := range s.Blocks {
		salesCol, taxCol := -1, -1
		for i, h := range b.Headers {
			if h == "申报销售额" {
				salesCol = i
			}
			if h == "本期应补（退）增值税" {
				taxCol = i
			}
		}
		if salesCol < 0 || taxCol < 0 {
			continue
		}
		counts := map[string]int{}
		bundles := map[string][]string{}
		maxStreak := map[string]int{}
		previousDirection := ""
		run := 0
		for ri := 1; ri < len(b.Rows); ri++ {
			row, prev := b.Rows[ri], b.Rows[ri-1]
			if len(row) <= max(salesCol, taxCol) || len(prev) <= max(salesCol, taxCol) || !adjacentAnalysisPeriods(prev[0], row[0]) {
				previousDirection = ""
				run = 0
				continue
			}
			a, ok1 := parseFormNumber(row[salesCol])
			a0, ok2 := parseFormNumber(prev[salesCol])
			z, ok3 := parseFormNumber(row[taxCol])
			z0, ok4 := parseFormNumber(prev[taxCol])
			if !ok1 || !ok2 || !ok3 || !ok4 || a0 <= invoiceAggregationTolerance || z0 <= invoiceAggregationTolerance {
				previousDirection = ""
				run = 0
				continue
			}
			ga, gz := (a-a0)/a0*100, (z-z0)/z0*100
			direction := "同向或持平"
			if ga > 0 && gz < 0 {
				direction = "销售增而税额降"
			}
			if ga < 0 && gz > 0 {
				direction = "销售降而税额增"
			}
			prefix := "direction." + analysisToken(row[0])
			refs := []string{fmt.Sprintf("fact:%s.b%d.r%d.c%d", s.Key, bi+1, ri+1, salesCol+1), fmt.Sprintf("fact:%s.b%d.r%d.c%d", s.Key, bi+1, ri, salesCol+1), fmt.Sprintf("fact:%s.b%d.r%d.c%d", s.Key, bi+1, ri+1, taxCol+1), fmt.Sprintf("fact:%s.b%d.r%d.c%d", s.Key, bi+1, ri, taxCol+1)}
			p := d.text(prefix+".period", "变化期间", "period", row[0], refs...)
			x := d.number(prefix+".sales", "销售额环比", "growth", "%", row[0], prev[0], "本期销售额/上期销售额−1，再乘100", ga, refs[:2]...)
			y := d.number(prefix+".tax", "应补退增值税环比", "growth", "%", row[0], prev[0], "本期税额/上期税额−1，再乘100", gz, refs[2:]...)
			if direction == "同向或持平" {
				previousDirection = ""
				run = 0
				continue
			}
			counts[direction]++
			if previousDirection == direction {
				run++
			} else {
				run = 1
			}
			previousDirection = direction
			maxStreak[direction] = max(maxStreak[direction], run)
			bundles[direction] = []string{p, x, y}
		}
		var focusRefs []string
		for _, direction := range []string{"销售增而税额降", "销售降而税额增"} {
			if counts[direction] == 0 {
				continue
			}
			key := analysisToken(direction)
			n := d.number("direction."+key+".count", direction+"发生期数", "coverage", "期", "", "", "对全部可比期间按方向分别计数", float64(counts[direction]))
			streak := d.number("direction."+key+".streak", direction+"最长连续期数", "streak", "期", "", "", "分别统计同类方向的最长连续期；缺数、持平或相反方向打断", float64(maxStreak[direction]))
			focusRefs = append(focusRefs, append(bundles[direction], n, streak)...)
		}
		if len(focusRefs) > 0 {
			d.focus("opposite-directions", "分别说明销售增税降与销售降税增的幅度、发生次数及各自连续期数，不合并成连续背离。", focusRefs...)
		}
	}
}

func addMarginCombinationAnalysis(d *SectionAnalysisData, s Section) {
	c := firstAnalysisChart(s)
	if c == nil || len(c.Series) < 3 {
		return
	}
	for i := len(c.Labels) - 1; i > 0; i-- {
		if !adjacentAnalysisPeriods(c.Labels[i-1], c.Labels[i]) {
			continue
		}
		ok := true
		delta := make([]float64, 3)
		for j := 0; j < 3; j++ {
			if !analysisPointAvailable(*c, c.Series[j], i) || !analysisPointAvailable(*c, c.Series[j], i-1) {
				ok = false
				break
			}
			delta[j] = c.Series[j].Values[i] - c.Series[j].Values[i-1]
		}
		if !ok || !(delta[0] > 0 && delta[1] < 0 && delta[2] > 0) {
			continue
		}
		refs := []string{d.text("margin-combination.period", "财务比率组合变化期间", "period", c.Labels[i]), d.text("margin-combination.base", "财务比率比较基期", "period", c.Labels[i-1])}
		for j := 0; j < 3; j++ {
			refs = append(refs, d.number("margin-combination."+analysisToken(c.Series[j].Name), c.Series[j].Name+"较上季变化", "change", "百分点", c.Labels[i], c.Labels[i-1], "本季原始比率−上季原始比率，展示末端舍入", delta[j], analysisChartRef(s, *c, c.Series[j], i), analysisChartRef(s, *c, c.Series[j], i-1)))
		}
		d.focus("latest-margin-divergence", "指出最近发生的毛利率上升、利润率下降、费用率上升组合及各自百分点，不用首尾改善替代。", refs...)
		return
	}
}

func addInvoiceDeclarationScope(d *SectionAnalysisData, s Section) {
	c := firstAnalysisChart(s)
	if c == nil {
		return
	}
	invoice, invoiceOK := findAnalysisSeries(*c, "销项发票金额")
	declared, declaredOK := findAnalysisSeries(*c, "申报销售额")
	if !invoiceOK || !declaredOK {
		return
	}
	count, nonzero := 0, 0
	sum, maxDiff := 0.0, 0.0
	var scope, refs []string
	for i, p := range c.Labels {
		if !analysisPointAvailable(*c, invoice, i) || !analysisPointAvailable(*c, declared, i) {
			continue
		}
		diff := declared.Values[i] - invoice.Values[i]
		count++
		sum += diff
		maxDiff = math.Max(maxDiff, math.Abs(diff))
		if math.Abs(diff) >= invoiceAggregationTolerance {
			nonzero++
		}
		scope = append(scope, p)
		refs = append(refs, analysisChartRef(s, *c, declared, i), analysisChartRef(s, *c, invoice, i))
	}
	if count == 0 {
		return
	}
	d.text("comparison.periods", "完整可比范围", "period", strings.Join(scope, "、"), refs...)
	d.number("comparison.count", "票申完整可比期数", "coverage", "期", "", "", "同时存在票面及申报销售额的期间数", float64(count), refs...)
	d.number("comparison.nonzero", "票申非零差异期数", "coverage", "期", "", "", "逐期绝对差额达到金额容差的期数", float64(nonzero), refs...)
	d.number("comparison.maximum", "票申最大绝对差额", "extremum", "元", "", "", "全部可比期间的绝对差额最大值", maxDiff, refs...)
	if disjointAnalysisPeriods(scope) {
		d.number("comparison.total", "申报销售额减销项发票金额合计差额", "difference", "元", "", "", "不重叠可比期间的申报销售额−销项发票金额合计；与本节表格方向一致，尚未扣除未开票销售额", sum, refs...)
	}
	if nonzero == 0 {
		d.text("comparison.relationship", "完整可比范围核对结论", "relationship", fmt.Sprintf("已核对的%d个可比期间票申销售额均一致", count), refs...)
	}
}

// Same closing date does not establish that the later annual return replaces
// a quarterly statement. Keep both values, suppress affected derived claims.
func addBalanceConflictAnalysis(d *SectionAnalysisData, model Model, s Section) {
	conflicting := map[string]bool{}
	for bi, b := range s.Blocks {
		for ri, row := range b.Rows {
			if len(row) < 2 {
				continue
			}
			_, end := periodBounds(row[0])
			if end == "" {
				continue
			}
			for rj := 0; rj < ri; rj++ {
				prev := b.Rows[rj]
				if len(prev) < 2 {
					continue
				}
				_, otherEnd := periodBounds(prev[0])
				if end != otherEnd || row[0] == prev[0] {
					continue
				}
				for ci := 1; ci < min(len(row), len(prev)); ci++ {
					if ci >= len(b.Headers) {
						continue
					}
					a, ok := parseFormNumber(row[ci])
					z, ok2 := parseFormNumber(prev[ci])
					if !ok || !ok2 || math.Abs(a-z) < invoiceAggregationTolerance {
						continue
					}
					conflicting[end+"|"+b.Headers[ci]] = true
					prefix := "balance-conflict." + analysisToken(end+"|"+b.Headers[ci])
					refs := []string{fmt.Sprintf("fact:%s.b%d.r%d.c%d", s.Key, bi+1, ri+1, ci+1), fmt.Sprintf("fact:%s.b%d.r%d.c%d", s.Key, bi+1, rj+1, ci+1)}
					p := d.text(prefix+".period", "余额冲突时点", "period", end, refs...)
					name := d.text(prefix+".account", "冲突科目", "object", b.Headers[ci], refs...)
					x := d.number(prefix+".annual", row[0]+"余额", "source-version", "元", row[0], "", "保留原报表余额，不据日期自动覆盖其他报表", a, refs[0])
					y := d.number(prefix+".quarter", prev[0]+"余额", "source-version", "元", prev[0], "", "保留原报表余额", z, refs[1])
					difference := d.number(prefix+".difference", "两份报表余额差额", "source-version", "元", row[0], prev[0], "所列第一份余额减第二份余额，仅表示版本差异，不是期间增长", a-z, x, y)
					d.focus(prefix, "同一时点的报表余额不同，说明具体科目、两份数值和脚本差额；替代关系未确认前不作该时点增长或收入联动结论。", p, name, x, y, difference)
				}
			}
		}
	}
	for i := range d.Metrics {
		m := &d.Metrics[i]
		_, end := periodBounds(m.Period)
		_, baseEnd := periodBounds(m.BasePeriod)
		if m.Kind == "value" || m.Kind == "table-value" || m.Kind == "source-version" || m.Kind == "period" || m.Kind == "object" {
			continue
		}
		for key := range conflicting {
			parts := strings.SplitN(key, "|", 2)
			if strings.HasPrefix(m.Name, parts[1]) && (end == parts[0] || baseEnd == parts[0] || m.Kind == "extremum") {
				m.Status = "conflicting"
			}
		}
	}
	// An unavailable maximum/minimum must not leave a usable "peak period"
	// behind; that label would still invite a conclusion from conflicting data.
	conflictIDs := map[string]bool{}
	for _, metric := range d.Metrics {
		if metric.Status == "conflicting" {
			conflictIDs[metric.ID] = true
		}
	}
	for i := range d.Metrics {
		metric := &d.Metrics[i]
		if strings.HasSuffix(metric.ID, ".period") && conflictIDs[strings.TrimSuffix(metric.ID, ".period")] {
			metric.Status = "conflicting"
		}
	}
	if len(conflicting) > 0 {
		d.Constraints = append(d.Constraints, "年末季报与年报均有原始值，未取得替代关系。不得选择其中一份计算该时点的增长、占收入比或最大最小值；不影响此前无冲突季度。")
	}
	// Earlier quarters remain comparable. Income is a same-quarter flow; a
	// balance/revenue ratio is explicitly a comparison measure, not turnover.
	c := firstAnalysisChart(s)
	income := firstAnalysisChart(sectionByKey(model, "financial.statements"))
	if c == nil || income == nil || len(income.Series) == 0 {
		return
	}
	for _, series := range c.Series {
		for i, p := range c.Labels {
			if !analysisPointAvailable(*c, series, i) {
				continue
			}
			_, end := periodBounds(p)
			if conflicting[end+"|"+series.Name] {
				continue
			}
			for j, q := range income.Labels {
				if p != q || !analysisPointAvailable(*income, income.Series[0], j) || income.Series[0].Values[j] <= 0 {
					continue
				}
				prefix := "balance-income." + analysisToken(series.Name+"|"+p)
				d.number(prefix+".share", series.Name+"期末余额/同季收入", "ratio", "%", p, "", "同季末余额/同季营业收入×100；不是周转率", series.Values[i]/income.Series[0].Values[j]*100, analysisChartRef(s, *c, series, i), analysisChartRef(sectionByKey(model, "financial.statements"), *income, income.Series[0], j))
				if i > 0 && j > 0 && analysisPointAvailable(*c, series, i-1) && analysisPointAvailable(*income, income.Series[0], j-1) && adjacentAnalysisPeriods(c.Labels[i-1], p) && c.Labels[i-1] == income.Labels[j-1] && series.Values[i-1] > 0 && income.Series[0].Values[j-1] > 0 {
					_, prevEnd := periodBounds(c.Labels[i-1])
					if conflicting[prevEnd+"|"+series.Name] {
						continue
					}
					d.number(prefix+".growth-gap", series.Name+"增幅与收入增幅之差", "deviation", "百分点", p, c.Labels[i-1], "(本季末余额/上季末余额−本季收入/上季收入)×100", (series.Values[i]/series.Values[i-1]-income.Series[0].Values[j]/income.Series[0].Values[j-1])*100, analysisChartRef(s, *c, series, i), analysisChartRef(s, *c, series, i-1), analysisChartRef(sectionByKey(model, "financial.statements"), *income, income.Series[0], j), analysisChartRef(sectionByKey(model, "financial.statements"), *income, income.Series[0], j-1))
				}
			}
		}
	}
}

func addPaymentContributionAnalysis(d *SectionAnalysisData, model Model, s Section) {
	c := firstAnalysisChart(s)
	if c == nil || len(c.Series) == 0 {
		return
	}
	peak := -1
	for i := range c.Labels {
		if analysisPointAvailable(*c, c.Series[0], i) && (peak < 0 || c.Series[0].Values[i] > c.Series[0].Values[peak]) {
			peak = i
		}
	}
	if peak < 0 || c.Series[0].Values[peak] <= 0 {
		return
	}
	p := c.Labels[peak]
	major := sectionByKey(model, "tax-payment.major-trend")
	chart := firstAnalysisChart(major)
	if chart == nil {
		return
	}
	for i, q := range chart.Labels {
		if q != p {
			continue
		}
		best := -1
		for j, series := range chart.Series {
			if analysisPointAvailable(*chart, series, i) && (best < 0 || series.Values[i] > chart.Series[best].Values[i]) {
				best = j
			}
		}
		if best < 0 {
			return
		}
		series := chart.Series[best]
		period := d.text("payment-peak.period", "缴税峰值期间", "period", p)
		total := d.number("payment-peak.total", "峰值税费缴纳总额", "extremum", "元", p, "", "完整已知总额序列最大值", c.Series[0].Values[peak], analysisChartRef(s, *c, c.Series[0], peak))
		name := d.text("payment-peak.tax", "峰值月份主要税种", "object", series.Name)
		amount := d.number("payment-peak.tax-amount", "峰值月该税种实缴金额", "amount", "元", p, "", "同归集期间实际缴款金额", series.Values[i], analysisChartRef(major, *chart, series, i))
		share := d.number("payment-peak.share", "该税种占峰值月总额比例", "share", "%", p, "", "同月该税种实缴/全部实缴税费×100", series.Values[i]/c.Series[0].Values[peak]*100, amount, total)
		focusRefs := []string{period, total, name, amount, share}
		if peak > 0 && i > 0 && analysisPointAvailable(*c, c.Series[0], peak-1) && analysisPointAvailable(*chart, series, i-1) && adjacentAnalysisPeriods(c.Labels[peak-1], p) && chart.Labels[i-1] == c.Labels[peak-1] {
			totalChange := c.Series[0].Values[peak] - c.Series[0].Values[peak-1]
			taxChange := series.Values[i] - series.Values[i-1]
			base := d.text("payment-peak.base", "峰值比较基期", "period", c.Labels[peak-1])
			delta := d.number("payment-peak.change", "峰值月总税费较上月增加额", "change", "元", p, c.Labels[peak-1], "本月全部税费−上月全部税费", totalChange, total, analysisChartRef(s, *c, c.Series[0], peak-1))
			taxDelta := d.number("payment-peak.tax-change", "主要税种较上月增加额", "change", "元", p, c.Labels[peak-1], "本月该税种−上月该税种", taxChange, amount, analysisChartRef(major, *chart, series, i-1))
			focusRefs = append(focusRefs, base, delta, taxDelta)
			if totalChange > invoiceAggregationTolerance {
				contribution := d.number("payment-peak.increment-share", "主要税种占总税费增量比例", "share", "%", p, c.Labels[peak-1], "该税种增加额/总税费增加额×100；与峰值月金额构成比例区分", taxChange/totalChange*100, taxDelta, delta)
				focusRefs = append(focusRefs, contribution)
			}
		}
		d.focus("peak-contribution", "峰值总额对应到具体税种金额及其占比，有相邻可比月时进一步说明主要税种的增量贡献。", focusRefs...)
		return
	}
}
