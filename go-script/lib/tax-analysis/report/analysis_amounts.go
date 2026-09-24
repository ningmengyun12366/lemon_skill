package report

import (
	"sort"
	"strings"
	"time"
)

type analysisPeriodAmount struct {
	Period TaxPeriod
	Value  float64
	Known  bool
	Source string
}

// analysisVATOverview uses the same declaration and confirmation-period readers
// as buildVATOverview. Certified purchase tax is not declaration input tax.
func analysisVATOverview(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "vat.period-overview"}
	series := map[string][]analysisPeriodAmount{}
	for _, f := range ctx.vat {
		_, tax, _, known := certifiedPurchasesForPeriod(ctx, f.TaxPeriod)
		for _, m := range []struct {
			key    string
			value  float64
			known  bool
			source string
		}{
			{"sales", f.Sales, f.HasSales, "vat_returns/" + f.RecordID},
			{"certified-tax", tax, known, "deduction_confirmations/purchases[confirmationPeriod=" + f.Period + "]"},
			{"supplement-tax", f.SupplementTax, f.HasSupplementTax, "vat_returns/" + f.RecordID},
		} {
			series[m.key] = append(series[m.key], analysisPeriodAmount{f.TaxPeriod, m.value, m.known, m.source})
		}
	}
	for _, m := range []struct{ key, name string }{{"sales", "申报销售额"}, {"certified-tax", "认证购进税额"}, {"supplement-tax", "申报应补（退）增值税"}} {
		addPeriodAmountSummary(&d, "vat-summary."+m.key, m.name, series[m.key], ctx.task.StartTime, ctx.task.EndTime)
	}
	d.Constraints = []string{"合计、最高值和最高期间使用vat-summary；认证购进税额来自抵扣确认，不能借用申报进项税额。", "申报应补退税额、税款实缴和滞纳金是不同口径，不相互替代；部分期间小计不能称全年合计，月报、季报、年报重叠不能重复相加。"}
	return d
}

// addPeriodAmountSummary keeps missing periods distinct from explicit zeros.
// Overlapping returns cannot form an additive total; extrema compare one cycle.
func addPeriodAmountSummary(d *SectionAnalysisData, key, name string, points []analysisPeriodAmount, start, end string) {
	points = append([]analysisPeriodAmount(nil), points...)
	sort.SliceStable(points, func(i, j int) bool { return points[i].Period.Start < points[j].Period.Start })
	var refs, scope []string
	var known []analysisPeriodAmount
	total, correction := 0.0, 0.0
	last, overlap, complete := "", false, start != "" && end != ""
	next := start
	for _, p := range points {
		if p.Period.Start == "" || p.Period.End == "" || p.Period.Start < start || p.Period.End > end {
			complete = false
			continue
		}
		if last != "" && p.Period.Start <= last {
			overlap = true
		}
		if p.Period.End > last {
			last = p.Period.End
		}
		if !p.Known {
			complete = false
			continue
		}
		if p.Period.Start != next {
			complete = false
		}
		t, err := time.Parse("2006-01-02", p.Period.End)
		if err != nil {
			complete = false
		} else {
			next = t.AddDate(0, 0, 1).Format("2006-01-02")
		}
		value := p.Value - correction
		sum := total + value
		correction, total = (sum-total)-value, sum
		refs = appendUnique(refs, p.Source)
		scope = append(scope, p.Period.Display)
		known = append(known, p)
	}
	if len(known) == 0 {
		return
	}
	complete = complete && last == end && !overlap
	label := name + "已取得期间小计"
	if complete {
		label = name + "报告期间合计"
	}
	id := d.number(key+".total", label, "sum", "元", analysisPeriodText(start, end), "", "仅累加不重叠所属期原始金额；保留负数，缺失不补零", total, refs...)
	d.Metrics[len(d.Metrics)-1].Scope = scope
	if overlap {
		d.Metrics[len(d.Metrics)-1].Status = "conflicting"
	}
	coverage := "仅已取得期间；不能称为完整报告期间合计"
	if complete {
		coverage = "完整覆盖报告期间"
	}
	if overlap {
		coverage = "所属期重叠，合计不可用；原始分期值分别保留"
	}
	status := d.text(key+".coverage", name+"合计覆盖范围", "coverage", coverage, refs...)
	d.focus(key, "引用本指标合计时同时保留覆盖范围；最高值沿用脚本给出的同周期比较及期间。", id, status)
	cycles := map[string][]analysisPeriodAmount{}
	for _, p := range known {
		cycles[p.Period.Cycle] = append(cycles[p.Period.Cycle], p)
	}
	for _, cycle := range sortedMapKeys(cycles) {
		ps := cycles[cycle]
		for _, maximum := range []bool{true, false} {
			best := ps[0].Value
			for _, p := range ps[1:] {
				if maximum && p.Value > best || !maximum && p.Value < best {
					best = p.Value
				}
			}
			var periods, sources, compared []string
			for _, p := range ps {
				compared = append(compared, p.Period.Display)
				sources = appendUnique(sources, p.Source)
				if p.Value == best {
					periods = appendUnique(periods, p.Period.Display)
				}
			}
			suffix, label := "minimum", "最小值"
			if maximum {
				suffix, label = "maximum", "最大值"
			}
			prefix := key + "." + analysisToken(cycle) + "." + suffix
			x := d.number(prefix, name+"已取得同周期"+label, "extremum", "元", strings.Join(periods, "、"), "", "在已取得同周期期间中比较原始金额；并列期间全部列出", best, sources...)
			d.Metrics[len(d.Metrics)-1].Scope = compared
			p := d.text(prefix+".period", name+label+"所属期", "period", strings.Join(periods, "、"), x)
			d.focus(prefix, "极值按已取得的同周期范围陈述，缺失期间不参与；并列最高/最低不得只选其中一期。", x, p)
		}
	}
}
