package report

import (
	"math"
	"sort"
	"strings"
)

// financialTrendAnalysis calculates directions from unrounded period amounts.
// Annual rows never enter month/quarter trends; gaps split continuous segments.
func financialTrendAnalysis(ctx *reportContext) []SectionAnalysisData {
	amounts := SectionAnalysisData{SectionID: "financial.statements"}
	ratios := SectionAnalysisData{SectionID: "financial.period-expense"}
	for _, metric := range []struct {
		key, name, unit string
		d               *SectionAnalysisData
		value           func(financialFact) (float64, bool)
	}{
		{"revenue", "营业收入", "元", &amounts, func(f financialFact) (float64, bool) { return f.Revenue, f.HasRevenue }},
		{"cost", "营业成本", "元", &amounts, func(f financialFact) (float64, bool) { return f.Cost, f.HasCost }},
		{"profit", "利润总额", "元", &amounts, func(f financialFact) (float64, bool) { return f.Profit, f.HasProfit }},
		{"gross-margin", "毛利率", "百分点", &ratios, func(f financialFact) (float64, bool) {
			return safeRatio(f.Revenue-f.Cost, f.Revenue) * 100, f.HasRevenue && f.HasCost && f.Revenue != 0
		}},
	} {
		byCycle := map[string][]financialFact{}
		for _, f := range financialTrendFacts(ctx.financial) {
			if f.TaxPeriod.Cycle == taxCycleMonthly || f.TaxPeriod.Cycle == taxCycleQuarterly {
				byCycle[f.TaxPeriod.Cycle] = append(byCycle[f.TaxPeriod.Cycle], f)
			}
		}
		for _, cycle := range sortedMapKeys(byCycle) {
			facts := byCycle[cycle]
			sort.SliceStable(facts, func(i, j int) bool { return facts[i].TaxPeriod.End < facts[j].TaxPeriod.End })
			addFinancialTrendSeries(metric.d, facts, metric.key+"."+analysisToken(cycle), metric.name, metric.unit, metric.value)
		}
	}
	return []SectionAnalysisData{amounts, ratios}
}

func addFinancialTrendSeries(d *SectionAnalysisData, facts []financialFact, key, name, unit string, value func(financialFact) (float64, bool)) {
	key = "trend-direction." + key
	var up, down, flat, scope, refs []string
	segmentStart := -1
	flushSegment := func(end int) {
		if segmentStart < 0 || end <= segmentStart {
			return
		}
		last := facts[end]
		b, _ := value(last)
		// Compare the segment end with earlier bases as well, so a discussion
		// starting mid-segment can use the same explicit scope across metrics.
		// Adjacent pairs are already present and need not be duplicated here.
		for start := segmentStart; start < end-1; start++ {
			first := facts[start]
			a, _ := value(first)
			id := d.number(key+".segment."+analysisToken(first.Period+"|"+last.Period), name+"连续可比区间首尾变化", "change", unit, last.Period, first.Period, "区间末期值−首期值；区间内逐期完整且相邻，不代表每一期同向变化", b-a, "income_statements/"+first.Period, "income_statements/"+last.Period)
			refs = append(refs, id)
		}
	}
	for i, f := range facts {
		current, known := value(f)
		if !known {
			flushSegment(i - 1)
			segmentStart = -1
			continue
		}
		scope = append(scope, f.Period)
		if segmentStart < 0 {
			segmentStart = i
			continue
		}
		previous := facts[i-1]
		if !adjacentAnalysisPeriods(previous.Period, f.Period) {
			flushSegment(i - 1)
			segmentStart = i
			continue
		}
		prior, _ := value(previous)
		change := current - prior
		pair := previous.Period + " → " + f.Period
		switch rounded := math.Round(change * 100); {
		case rounded > 0:
			up = append(up, pair)
		case rounded < 0:
			down = append(down, pair)
		default:
			flat = append(flat, pair)
		}
		refs = append(refs, d.number(key+".change."+analysisToken(f.Period), name+"相邻期变化", "change", unit, f.Period, previous.Period, "本期值−相邻同周期前期值；比率使用未舍入金额计算", change, "income_statements/"+f.Period, "income_statements/"+previous.Period))
	}
	flushSegment(len(facts) - 1)
	if len(refs) == 0 {
		return
	}
	var summaryRefs []string
	for _, direction := range []struct {
		key, name string
		pairs     []string
	}{{"up", "上升", up}, {"down", "下降", down}, {"flat", "持平", flat}} {
		id := d.number(key+"."+direction.key+"-count", name+"相邻期"+direction.name+"次数", "coverage", "次", "", "", "仅比较相邻且数据完整的同周期记录，变化按两位展示精度判方向；年度不参加", float64(len(direction.pairs)), refs...)
		d.Metrics[len(d.Metrics)-1].Scope = append([]string(nil), scope...)
		periods := d.text(key+"."+direction.key+"-periods", name+direction.name+"区间", "period", textOr(strings.Join(direction.pairs, "、"), "无"), id)
		summaryRefs = append(summaryRefs, id, periods)
	}
	addFinancialDirectionRuns(d, facts, key, name, unit, value)
	d.focus(key, "方向次数对应已列可比范围。全程连续同向要求全部相邻期同向；局部连续同向使用脚本列明的连续区间，不能因全年有升有降就说没有连续上升或下降。区间首尾变化不代表区间内每一期同向。", append(summaryRefs, refs...)...)
}

// Runs count adjacent changes rather than observations. Missing values and
// nonadjacent periods break a run; its base is not itself a rising/falling period.
func addFinancialDirectionRuns(d *SectionAnalysisData, facts []financialFact, key, name, unit string, value func(financialFact) (float64, bool)) {
	direction, start, end := 0, -1, -1
	flush := func() {
		if start < 0 || end-start < 2 {
			return
		}
		base, last := facts[start], facts[end]
		a, _ := value(base)
		b, _ := value(last)
		word := "上升"
		if direction < 0 {
			word = "下降"
		}
		prefix := key + ".run." + analysisToken(base.Period+"|"+last.Period)
		var scope []string
		for i := start + 1; i <= end; i++ {
			scope = append(scope, facts[i].Period)
		}
		refs := []string{
			d.text(prefix+".base", name+"连续"+word+"基期", "period", base.Period),
			d.text(prefix+".periods", name+"连续"+word+"的变化所属期", "period", strings.Join(scope, "、")),
			d.number(prefix+".count", name+"连续"+word+"次数", "coverage", "次", last.Period, base.Period, "连续同向的相邻变化次数；观察点数比变化次数多一，基期不算一次变化", float64(end-start)),
			d.number(prefix+".change", name+"连续"+word+"区间变化", "change", unit, last.Period, base.Period, "连续同向区间末值−基期值", b-a, "income_statements/"+base.Period, "income_statements/"+last.Period),
		}
		d.focus(prefix, "描述局部连续变化时区分基期与实际发生变化的期间；后续反向不抹掉此前连续区间，不能把局部区间称为全年持续同向。", refs...)
	}
	for i := 1; i < len(facts); i++ {
		a, aOK := value(facts[i-1])
		b, bOK := value(facts[i])
		next := 0
		if aOK && bOK && adjacentAnalysisPeriods(facts[i-1].Period, facts[i].Period) {
			switch change := math.Round((b - a) * 100); {
			case change > 0:
				next = 1
			case change < 0:
				next = -1
			}
		}
		if next == 0 || next != direction {
			flush()
			start, end = -1, -1
			if next != 0 {
				start = i - 1
			}
		}
		direction = next
		if next != 0 {
			end = i
		}
	}
	flush()
}
