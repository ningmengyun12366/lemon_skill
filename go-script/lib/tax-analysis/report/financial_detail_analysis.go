package report

import (
	"math"
	"strings"
)

// These bounded analysis groups never add tables to the customer report.
func analysisFinancialDetails(ctx *reportContext) []SectionAnalysisData {
	cash := SectionAnalysisData{SectionID: "financial.cash-flow"}
	balance := SectionAnalysisData{SectionID: "financial.debt-asset"}
	profit := SectionAnalysisData{SectionID: "financial.period-expense"}
	previous := map[string]financialFact{}
	latestBalance, latestProfit := map[string]AnalysisFocus{}, map[string]AnalysisFocus{}
	for _, f := range ctx.financial {
		addCashProfitPair(&cash, f)
		if refs := addFinancialStructure(&balance, f); len(refs) > 0 {
			latestBalance[f.TaxPeriod.Cycle] = AnalysisFocus{ID: balance.SectionID + ".structure." + analysisToken(f.Period), Required: true,
				Goal: "结合已取得的资产、负债、净资产和合同负债占比说明账面结构；金额构成不证明借款用途、到期压力或实际履约情况。", EvidenceRefs: refs}
		}
		base, exists := previous[f.TaxPeriod.Cycle]
		if exists && adjacentAnalysisPeriods(base.Period, f.Period) {
			if refs := addProfitChangeBreakdown(&profit, base, f); len(refs) > 0 {
				latestProfit[f.TaxPeriod.Cycle] = AnalysisFocus{ID: profit.SectionID + ".profit-change." + analysisToken(f.Period), Required: true,
					Goal: "提炼最近可比期间主要项目对利润增减的金额贡献及尚未分配差额；核平只证明金额关系，不证明项目变动的业务原因。", EvidenceRefs: refs}
			}
		}
		previous[f.TaxPeriod.Cycle] = f
	}
	for _, key := range sortedMapKeys(latestBalance) {
		balance.Focus = append(balance.Focus, latestBalance[key])
	}
	for _, key := range sortedMapKeys(latestProfit) {
		profit.Focus = append(profit.Focus, latestProfit[key])
	}
	cash.Constraints = []string{"cash-profit按真实所属期成组给出双方金额与可比状态；只有明确列为未取得的字段才缺失。完整比较组不能因阅读精简或数值为零而跳过。"}
	profit.Constraints = []string{"profit-change列明已取得项目的账面贡献和未分配差额；未取得项目不补零，数值核平不证明所有明细已核验或经营原因已确认。"}
	return []SectionAnalysisData{cash, balance, profit}
}

func financialDetailAmount(d *SectionAnalysisData, key, name, period string, value float64, known bool, source string) string {
	if known {
		return d.number(key, name, "amount", "元", period, "", "同一财报实际所属期金额；余额不差分，流量按准则还原", value, source)
	}
	id := metricID(d.SectionID, key)
	d.Metrics = append(d.Metrics, AnalysisMetric{ID: id, Name: name, Kind: "amount", Unit: "元", Period: period,
		Status: "missing", Algorithm: "该期间该指标未取得，不能补零或由其他周期倒推", SourceRefs: []string{source}})
	return id
}

func addCashProfitPair(d *SectionAnalysisData, f financialFact) {
	key := "cash-profit." + analysisToken(f.Period)
	source := "income_statements/" + f.Period
	left := financialDetailAmount(d, key+".cash", "经营活动现金流量净额", f.Period, f.OperatingCashFlow, f.HasOperatingCashFlow, source)
	right := financialDetailAmount(d, key+".profit", "净利润", f.Period, f.NetProfit, f.HasNetProfit, source)
	state := f.Period + "："
	if f.HasOperatingCashFlow && f.HasNetProfit {
		d.number(key+".difference", "经营活动现金流量净额减净利润", "difference", "元", f.Period, "", "同所属期经营现金流量净额−净利润；只反映金额关系", f.OperatingCashFlow-f.NetProfit, left, right)
		state += "双方均已取得，可同期间比较"
	} else {
		var missing []string
		if !f.HasOperatingCashFlow {
			missing = append(missing, "经营活动现金流量净额")
		}
		if !f.HasNetProfit {
			missing = append(missing, "净利润")
		}
		state += strings.Join(missing, "、") + "未取得；另一项如已取得仍可陈述"
	}
	d.text(key+".state", "同期间现金流与利润可比状态", "coverage", state, left, right)
}

func addFinancialStructure(d *SectionAnalysisData, f financialFact) []string {
	key, source := "balance-structure."+analysisToken(f.Period), "balance_sheets/"+f.Period
	assets := financialDetailAmount(d, key+".assets", "资产总额", f.Period, f.Assets, f.HasAssets, source)
	debt := financialDetailAmount(d, key+".liabilities", "负债总额", f.Period, f.Liabilities, f.HasLiabilities, source)
	contract := financialDetailAmount(d, key+".contract", "合同负债", f.Period, f.ContractLiabilities, f.HasContractLiabilities, source)
	refs := []string{assets, debt, contract}
	if f.HasAssets && f.HasLiabilities {
		refs = append(refs, d.number(key+".net-assets", "净资产（资产减负债）", "amount", "元", f.Period, "", "同版本期末资产总额−负债总额，不把季度期末当年初", f.Assets-f.Liabilities, assets, debt))
	}
	if f.HasContractLiabilities && f.HasLiabilities && f.Liabilities > 0 {
		refs = append(refs, d.number(key+".contract-share", "合同负债占负债总额比例", "share", "%", f.Period, "", "同版本期末合同负债÷负债总额×100；不是现金来源比例", f.ContractLiabilities/f.Liabilities*100, contract, debt))
	}
	return refs
}

func addProfitChangeBreakdown(d *SectionAnalysisData, base, f financialFact) []string {
	if !f.HasProfit || !base.HasProfit {
		return nil
	}
	key := "profit-change." + analysisToken(base.Period+"|"+f.Period)
	actual := f.Profit - base.Profit
	refs := []string{d.number(key+".total", "利润总额较上期变化", "change", "元", f.Period, base.Period, "当期利润总额−相邻同周期上期利润总额", actual, "income_statements/"+f.Period, "income_statements/"+base.Period)}
	type component struct {
		key, name string
		sign      float64
		value     func(financialFact) (float64, bool)
	}
	components := []component{
		{"revenue", "营业收入", 1, func(f financialFact) (float64, bool) { return f.Revenue, f.HasRevenue }},
		{"cost", "营业成本", -1, func(f financialFact) (float64, bool) { return f.Cost, f.HasCost }},
		{"expenses", "期间费用", -1, financialExpenseTotal},
	}
	for _, c := range financialProfitComponents {
		label := c.label
		components = append(components, component{c.key, label, 1, func(f financialFact) (float64, bool) {
			v := f.ProfitComponents[label]
			sign := f.ProfitComponentSigns[label]
			if v == nil || !v.known || sign == 0 {
				return 0, false
			}
			return v.value * sign, true
		}})
	}
	known := 0.0
	var unavailable []string
	for _, c := range components {
		prior, priorOK := c.value(base)
		current, currentOK := c.value(f)
		if !priorOK || !currentOK {
			unavailable = append(unavailable, c.name)
			continue
		}
		contribution := (current - prior) * c.sign
		known += contribution
		// Show the source amounts beside their effect on profit. A falling
		// expense contributes positively, but must not be described as rising.
		if c.key == "revenue" || c.key == "cost" || c.key == "expenses" {
			priorRef := d.number(key+"."+c.key+"-prior", c.name+"上期金额", "amount", "元", base.Period, "", "同周期比较的上期原金额", prior, "income_statements/"+base.Period+"/"+c.name)
			currentRef := d.number(key+"."+c.key+"-current", c.name+"当期金额", "amount", "元", f.Period, "", "同周期比较的当期原金额", current, "income_statements/"+f.Period+"/"+c.name)
			d.number(key+"."+c.key+"-change", c.name+"自身金额变化", "change", "元", f.Period, base.Period, "当期金额−上期金额；与对利润的贡献方向分别列示", current-prior, currentRef, priorRef)
		}
		id := d.number(key+"."+c.key, c.name+"变动对利润的金额贡献", "contribution", "元", f.Period, base.Period,
			"同周期当期项目减上期项目，按利润表加减方向计入；减值损失沿用原行列报符号；不包含其中子项", contribution,
			"income_statements/"+base.Period+"/"+c.name, "income_statements/"+f.Period+"/"+c.name)
		if math.Abs(contribution) >= invoiceAggregationTolerance {
			refs = append(refs, id)
		}
	}
	allocated := d.number(key+".allocated", "已取得项目对利润变动的贡献合计", "sum", "元", f.Period, base.Period, "已取得且列报方向明确的各独立项目贡献之和；缺失不补零", known, refs...)
	residual := d.number(key+".unallocated", "利润变动尚未分配差额", "difference", "元", f.Period, base.Period, "利润总额变动−已取得项目贡献合计；不是确定的错报或某项缺失金额", actual-known, refs[0], allocated)
	state := "已取得项目的金额关系已核平，尚未核验业务原因"
	if math.Abs(actual-known) >= invoiceAggregationTolerance {
		state = "仍有尚未分配差额，不归因为某个未取得项目"
	}
	if len(unavailable) > 0 {
		state += "；未取得双方可比值或列报方向不明确的项目：" + strings.Join(unavailable, "、") + "；不将这些项目视为零"
	}
	status := d.text(key+".state", "利润变动分解状态", "coverage", state, residual)
	return append(refs, allocated, residual, status)
}
