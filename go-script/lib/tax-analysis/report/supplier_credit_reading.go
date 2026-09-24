package report

import "strings"

func supplierCreditProductParty(sectionID, metric string) string {
	if sectionID != "invoice.supplier-credit" || !strings.HasPrefix(metric, metricID(sectionID, "counterparty.")) {
		return ""
	}
	party, _, ok := strings.Cut(metric, ".product.")
	if !ok {
		return ""
	}
	return party
}

func supplierCreditProductAmounts(s SectionAnalysisData) map[string][]AnalysisMetric {
	parties := map[string][]AnalysisMetric{}
	for _, m := range s.Metrics {
		if party := supplierCreditProductParty(s.SectionID, m.ID); party != "" && strings.HasSuffix(m.ID, ".amount") {
			parties[party] = append(parties[party], m)
		}
	}
	return parties
}

// The ordinary product reading rule is applied separately to each credit
// subject. Public events and the complete frozen product archive stay intact.
func supplierCreditReadingSelection(s SectionAnalysisData) map[string]bool {
	parties := supplierCreditProductAmounts(s)
	if len(parties) == 0 {
		return nil
	}
	selected := map[string]bool{}
	for _, f := range s.Focus {
		for _, ref := range f.EvidenceRefs {
			selected[readingGroupID(ref)] = true
		}
	}
	for _, amounts := range parties {
		selectReadingAmountGroups(amounts, selected)
	}
	return selected
}

func addSupplierCreditReadingCoverage(s *SectionAnalysisData, selected map[string]bool) {
	parties := supplierCreditProductAmounts(*s)
	for _, party := range sortedMapKeys(parties) {
		amounts := parties[party]
		summary := summarizeReadingAmounts(amounts, selected)
		key := strings.TrimPrefix(party, metricID(s.SectionID, "")) + ".reading-products"
		start := len(s.Metrics)
		s.number(key+".total-count", "该主体已取得品名组数", "count", "组", summary.period, "", "该主体完整底稿中已取得的品名金额指标计数", float64(summary.count), summary.refs...)
		s.number(key+".shown-count", "该主体重点阅读品名组数", "count", "组", summary.period, "", "该主体绝对金额前十、重点、负数及缺失品名的并集", float64(summary.shownCount), summary.refs...)
		if !summary.complete {
			s.Metrics = append(s.Metrics, AnalysisMetric{ID: metricID(s.SectionID, key+".coverage"), Name: "该主体重点阅读品名绝对金额覆盖率", Kind: "share", Unit: "%", Period: summary.period, Status: "missing", Algorithm: "存在金额缺失的品名，不能计算完整净额及覆盖率", SourceRefs: summary.refs})
		} else {
			s.number(key+".total-amount", "该主体已取得品名净额", "amount", "元", summary.period, "", "仅合计该主体已取得品名明细的有符号金额，不代表完整业务覆盖", summary.total, summary.refs...)
			s.number(key+".shown-amount", "该主体重点阅读品名净额", "amount", "元", summary.period, "", "重点阅读品名并集的有符号金额合计", summary.shown, summary.refs...)
			if summary.totalAbs > 0 {
				s.number(key+".coverage", "该主体重点阅读品名绝对金额覆盖率", "share", "%", summary.period, "", "所选品名绝对金额之和/该主体全部已取得品名绝对金额之和×100；不代表完整业务或履约核验比例", summary.shownAbs/summary.totalAbs*100, summary.refs...)
			}
		}
		for i := start; i < len(s.Metrics); i++ {
			s.Metrics[i].Scope = append([]string(nil), amounts[0].Scope...)
		}
	}
}
