package report

import (
	"math"
	"strings"
)

// invoiceComparisonStatistics counts the same final differences used by the
// table. Input, output and combined scopes are separate even in the same month.
type invoiceComparisonStatistics struct {
	key, label              string
	missing, nonzero        int
	periods, nonzeroPeriods []string
	total, ticket, filing   float64
	refs                    []string
}

func (s *invoiceComparisonStatistics) add(period string, difference, ticket, filing float64, refs ...string) {
	s.periods = append(s.periods, period)
	s.refs = append(s.refs, refs...)
	s.ticket += ticket
	s.filing += filing
	s.total += difference
	if math.Round(difference*100) != 0 {
		s.nonzero++
		s.nonzeroPeriods = appendUnique(s.nonzeroPeriods, period)
	}
}

func (s invoiceComparisonStatistics) appendFacts(d *SectionAnalysisData) {
	if len(s.periods)+s.missing == 0 {
		return
	}
	key := "tax-comparison-summary." + s.key
	var summaryRefs []string
	for _, metric := range []struct {
		key, name, unit string
		value           int
	}{
		{"records", "可比记录数", "条", len(s.periods)},
		{"periods", "可比所属期数", "期", len(uniqueSorted(s.periods))},
		{"nonzero-records", "非零差额记录数", "条", s.nonzero},
		{"nonzero-periods", "非零差额所属期数", "期", len(s.nonzeroPeriods)},
		{"missing", "缺少比较数据记录数", "条", s.missing},
	} {
		id := d.number(key+"."+metric.key, s.label+metric.name, "coverage", metric.unit, "", "", "仅统计"+s.label+"；合计同月进销项分别算记录，所属期去重；未知不算零，非零按两位金额精度", float64(metric.value), s.refs...)
		d.Metrics[len(d.Metrics)-1].Scope = uniqueSorted(s.periods)
		summaryRefs = append(summaryRefs, id)
	}
	summaryRefs = append(summaryRefs, d.text(key+".nonzero-scope", s.label+"非零差额期间", "period", textOr(strings.Join(uniqueSorted(s.nonzeroPeriods), "、"), "无"), s.refs...))
	if s.key != "all" && len(s.periods) > 0 && disjointAnalysisPeriods(s.periods) {
		for _, metric := range []struct {
			key, name string
			value     float64
		}{
			{"difference", "可比期差额合计", s.total},
			{"ticket", "可比期发票对应税额合计", s.ticket},
			{"filing", "可比期申报对应税额合计", s.filing},
		} {
			if s.key == "output" && metric.key == "filing" {
				metric.name = "可比期申报销项税额合计（扣除已知未开票税额后）"
			}
			if s.key == "output" && metric.key == "difference" {
				metric.name = "可比期调节后差额合计"
			}
			summaryRefs = append(summaryRefs, d.number(key+"."+metric.key, s.label+metric.name, "total", "元", strings.Join(s.periods, "、"), "", "仅合计同方向不重叠的完整可比期；销项有已知未开票税额时先调节", math.Round(metric.value*100)/100, s.refs...))
		}
	}
	d.focus(key, "以下统计仅属于"+s.label+"；进项说明不能使用进销项合计的记录数，合计记录数也不等于所属期数。缺少比较数据的记录单独说明，不纳入零差额。", summaryRefs...)
}
