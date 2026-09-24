package report

import (
	"fmt"
	"sort"
	"strings"
)

// invoiceCounterpartyShare uses the same exact subject and invoice denominator
// as the customer/supplier chapters. Public-credit payload shares are not used.
func invoiceCounterpartyShare(ctx *reportContext, role string, subject map[string]any) (float64, bool) {
	item, _, ok := invoiceCounterpartyAmounts(ctx, role, subject)
	return item.Share, ok
}

// invoiceCounterpartyAmounts preserves the same matching used by all credit
// sections and exposes the actual share operands for narrative evidence.
func invoiceCounterpartyAmounts(ctx *reportContext, role string, subject map[string]any) (counterpartyItem, float64, bool) {
	analysis := ctx.customers
	if role == qixinImportantSupplierRole {
		analysis = ctx.suppliers
	}
	if analysis == nil {
		return counterpartyItem{}, 0, false
	}
	groups := ctx.issued
	if role == qixinImportantSupplierRole {
		groups = ctx.received
	}
	total := invoiceNetAmount(groups)
	if total <= 0 {
		return counterpartyItem{}, 0, false
	}
	for _, item := range analysis.Items {
		if qixinCounterpartySubject(ctx, item, role) == nil {
			continue
		}
		name, taxID := mapText(subject, "name"), mapText(subject, "taxId", "taxID", "uscc")
		if (item.TaxID != "" && taxID != "" && qixinIdentityText(item.TaxID) == qixinIdentityText(taxID)) || ((item.TaxID == "" || taxID == "") && name != "" && item.Name == name) {
			return item, total, true
		}
	}
	return counterpartyItem{}, 0, false
}

// financialTrendFacts separates annual returns from comparable period trends.
// Annual values remain in the section tables as annual summaries.
func financialTrendFacts(facts []financialFact) []financialFact {
	var result []financialFact
	for _, fact := range facts {
		if fact.TaxPeriod.Cycle == taxCycleAnnual || strings.Contains(fact.Period, "年度") {
			continue
		}
		result = append(result, fact)
	}
	return result
}

func financialSummaryBlocks(ctx *reportContext) []Block {
	facts := financialTrendFacts(ctx.financial)
	if len(facts) == 0 {
		return nil
	}
	last := facts[len(facts)-1]
	text := last.Period + "毛利率" + optionalPercent(safeRatio(last.Revenue-last.Cost, last.Revenue), last.HasRevenue && last.HasCost && last.Revenue != 0) + "，资产负债率" + optionalPercent(safeRatio(last.Liabilities, last.Assets), last.HasAssets && last.HasLiabilities && last.Assets != 0) + "。"
	if last.HasOperatingCashFlow {
		text += "同期经营活动现金流量净额" + moneyText(last.OperatingCashFlow) + "元。"
	} else {
		text += "未取得同期现金流量表，暂无法结合现金流评价利润质量及偿付能力。"
	}
	return []Block{{Kind: BlockParagraph, Title: "财务指标综合提示", Paragraphs: []string{text}}}
}

func redInvoiceSummaryBlocks(ctx *reportContext) []Block {
	count, amountKnown := 0, true
	var red []invoiceGroup
	for _, fact := range ctx.issued {
		if fact.Red || (fact.HasAmount && fact.Amount < 0) {
			count += fact.Count
			amountKnown = amountKnown && fact.HasAmount
			red = append(red, fact)
		}
	}
	rows := [][]string{{"红字发票数量", fmt.Sprintf("%d张", count)}, {"红字金额（绝对值）", optionalMoney(ctx.redIssuedAmount, amountKnown)}, {"红字金额/蓝字金额", optionalPercent(safeRatio(ctx.redIssuedAmount, ctx.blueIssuedAmount), amountKnown && ctx.blueIssuedAmount > 0)}}
	categories := aggregateInvoiceCategories(red)
	sort.SliceStable(categories, func(i, j int) bool { return abs(categories[i].amount) > abs(categories[j].amount) })
	for index, item := range categories {
		if index >= 3 {
			break
		}
		rows = append(rows, []string{"主要红字品类 · " + item.name, moneyText(abs(item.amount))})
	}
	// Sort red amounts by magnitude; net-negative invoices otherwise sort backwards.
	positive := append([]invoiceGroup(nil), red...)
	for i := range positive {
		positive[i].Amount = abs(positive[i].Amount)
	}
	for index, item := range counterpartyAmounts(positive) {
		if index >= 3 {
			break
		}
		rows = append(rows, []string{"主要红字客户 · " + item.name, moneyText(item.amount)})
	}
	return []Block{{Kind: BlockTable, Title: "本期红字发票汇总", Headers: []string{"指标", "本期数据"}, Rows: rows}}
}

// completeBusinessPie preserves the all-invoice denominator when only the top
// categories are displayed. Negative category totals cannot form a valid pie.
func completeBusinessPie(ctx *reportContext, chart *Chart) {
	if chart == nil || len(chart.Series) != 1 {
		return
	}
	total, selected := 0.0, 0.0
	for _, fact := range ctx.issued {
		if fact.HasAmount {
			total += fact.Amount
		}
	}
	for _, value := range chart.Series[0].Values {
		if value < 0 {
			chart.Labels = nil
			chart.Series = nil
			chart.EmptyMessage = "业务净额含负值，暂不绘制占比图。"
			return
		}
		selected += value
	}
	if selected-total >= invoiceAggregationTolerance || total <= 0 {
		chart.Labels, chart.Series = nil, nil
		chart.EmptyMessage = "业务净额不能形成有效占比，按金额表列示。"
		return
	}
	if total-selected >= invoiceAggregationTolerance {
		chart.Labels = append(chart.Labels, "其他")
		chart.Series[0].Values = append(chart.Series[0].Values, total-selected)
	}
}

// comparableTrendBlocks keeps annual returns out of quarter/month trend axes.
// Tables still carry the annual return, including annual financial revisions.
func comparableTrendBlocks(sectionID string, blocks []Block) []Block {
	if !stringSet([]string{"6.1", "6.6", "8.1", "8.2", "8.3", "8.4", "8.5"})[sectionID] {
		return blocks
	}
	for i := range blocks {
		chart := blocks[i].Chart
		if chart == nil {
			continue
		}
		keep := []int{}
		labels := []string{}
		for j, label := range chart.Labels {
			if strings.Contains(label, "年度") {
				continue
			}
			keep = append(keep, j)
			labels = append(labels, label)
		}
		if len(keep) == len(chart.Labels) {
			continue
		}
		chart.Labels = labels
		for j := range chart.Series {
			series := &chart.Series[j]
			values, available := []float64{}, []bool{}
			for _, index := range keep {
				if index < len(series.Values) {
					values = append(values, series.Values[index])
				}
				if index < len(series.Available) {
					available = append(available, series.Available[index])
				}
			}
			series.Values = values
			if series.Available != nil {
				series.Available = available
			}
		}
	}
	return blocks
}
