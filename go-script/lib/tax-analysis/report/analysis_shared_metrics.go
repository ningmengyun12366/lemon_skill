package report

import "strings"

// shareAnalysisMetrics reuses the exact calculated values in related chapters.
// It also propagates unavailable balance metrics to the financial summary.
func shareAnalysisMetrics(data *AnalysisData) {
	indices := map[string]int{}
	for i, section := range data.Sections {
		indices[section.SectionID] = i
	}
	for _, link := range []struct{ source, target, prefix string }{
		{"invoice.customer-concentration", "invoice.customers", "concentration."},
		{"invoice.supplier-concentration", "invoice.suppliers", "concentration."},
		{"invoice.suppliers", "invoice.supplier-concentration", "counterparty."},
		{"invoice.tax-comparison", "vat.input-deduction", "tax-comparison-summary.input."},
		{"vat.burden-trend", "vat.period-overview", ""},
		{"financial.working-capital", "financial.summary", ""},
		{"financial.period-expense", "financial.summary", "profit-amounts."},
		{"financial.period-expense", "financial.summary", "profit-change."},
		{"financial.debt-asset", "financial.summary", "balance-structure."},
		{"financial.period-expense", "financial.summary", "margin-combination."},
		{"financial.statements", "financial.summary", "trend-direction."},
		{"financial.statements", "financial.period-expense", "annual-flow."},
		{"financial.statements", "financial.summary", "annual-flow."},
		{"financial.period-expense", "financial.summary", "trend-direction."},
		{"financial.cash-flow", "financial.summary", ""},
		{"cit.profit-taxable", "cit.quarter-contribution", "cit-bridge."},
	} {
		si, sourceOK := indices[link.source]
		ti, targetOK := indices[link.target]
		if !sourceOK || !targetOK {
			continue
		}
		source := data.Sections[si]
		target := &data.Sections[ti]
		shareFocus := link.prefix == "profit-amounts." || link.prefix == "profit-change." || link.prefix == "balance-structure." || strings.HasPrefix(link.prefix, "cit-") || link.prefix == "trend-direction." || link.prefix == "annual-flow." || link.prefix == "tax-comparison-summary.input."
		target.RelatedSections = appendUnique(target.RelatedSections, link.source)
		existing := map[string]int{}
		for i, metric := range target.Metrics {
			existing[metric.ID] = i
		}
		var sharedRefs []string
		for _, metric := range source.Metrics {
			key := strings.TrimPrefix(metric.ID, "fact:"+link.source+".metric.")
			if link.prefix != "" && !strings.HasPrefix(key, link.prefix) {
				continue
			}
			if link.prefix == "counterparty." && (strings.Contains(key, ".month") || (!strings.HasSuffix(key, ".name") && !strings.HasSuffix(key, ".amount"))) {
				continue
			}
			if link.source == "vat.burden-trend" && metric.Kind != "extremum" && !strings.HasSuffix(key, ".maximum.period") && !strings.HasSuffix(key, ".minimum.period") && !strings.HasSuffix(key, ".coverage") {
				continue
			}
			if link.source == "financial.working-capital" && metric.Status == "present" && !strings.HasPrefix(key, "balance-conflict.") {
				continue
			}
			id := metricID(link.target, key)
			if i, ok := existing[id]; ok {
				if metric.Status != "present" {
					target.Metrics[i].Status = metric.Status
				}
				continue
			}
			metric.SourceRefs = append([]string{metric.ID}, metric.SourceRefs...)
			if shareFocus {
				for _, ref := range append([]string(nil), metric.SourceRefs...) {
					if strings.HasPrefix(ref, metricID(link.source, link.prefix)) {
						metric.SourceRefs = appendUnique(metric.SourceRefs, strings.Replace(ref, "fact:"+link.source+".", "fact:"+link.target+".", 1))
					}
				}
			}
			metric.ID = id
			target.Metrics = append(target.Metrics, metric)
			if metric.Status == "present" && (metric.Kind == "extremum" || strings.HasSuffix(key, ".share") || strings.HasSuffix(key, ".difference")) {
				sharedRefs = appendUnique(sharedRefs, id)
			}
		}
		// A related topic is one candidate focus, even when several of its
		// metrics are shared. Keep every metric and reference available.
		if len(sharedRefs) > 0 {
			target.focus("shared."+analysisToken(link.source+"|"+link.prefix), "结合本节问题选用相关章节的已计算结果及其明确期间和范围。", sharedRefs...)
		}
		if shareFocus {
			sharedFocusIndices := map[string]int{}
			for _, focus := range source.Focus {
				var refs []string
				for _, ref := range focus.EvidenceRefs {
					if strings.HasPrefix(ref, metricID(link.source, link.prefix)) {
						refs = append(refs, strings.Replace(ref, "fact:"+link.source+".", "fact:"+link.target+".", 1))
					}
				}
				if len(refs) > 0 {
					if i, ok := sharedFocusIndices[focus.Goal]; ok {
						for _, ref := range refs {
							target.Focus[i].EvidenceRefs = appendUnique(target.Focus[i].EvidenceRefs, ref)
						}
					} else {
						sharedFocusIndices[focus.Goal] = len(target.Focus)
						target.focus("shared."+analysisToken(focus.ID), focus.Goal, refs...)
					}
				}
			}
			for _, constraint := range source.Constraints {
				target.Constraints = appendUnique(target.Constraints, constraint)
			}
		}
		if link.source == "financial.working-capital" {
			target.Constraints = appendUnique(target.Constraints, "财务综合结论沿用各节限制；冲突年末余额不能任选年报覆盖季报，受影响的全年变化及增长率不可用。")
		}
	}
}
