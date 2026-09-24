package report

// financialBalanceBlocks surfaces separate opening/closing balances as clues.
// Fixed assets are not equated with taxable house original value; advances and
// contract liabilities are neither merged nor directly added to VAT sales.
func financialBalanceBlocks(ctx *reportContext, labels []string, title, note string) []Block {
	var rows [][]string
	for _, fact := range ctx.financial {
		for _, label := range labels {
			balance, ok := fact.Balances[label]
			if !ok {
				continue
			}
			rows = append(rows, []string{fact.Period, label, "期初" + optionalMoney(balance.Opening, balance.HasOpening) + "；期末" + optionalMoney(balance.Closing, balance.HasClosing), "元"})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return []Block{{Kind: BlockTable, Title: title, Headers: []string{"所属期", "税局中文字段", "原值", "单位"}, Rows: rows}, {Kind: BlockParagraph, Paragraphs: []string{note}}}
}

func propertyFinancialBlocks(ctx *reportContext) []Block {
	return financialBalanceBlocks(ctx, []string{"固定资产原价", "固定资产账面价值", "固定资产", "投资性房地产"}, "房产相关财报资产线索", "财报固定资产及投资性房地产为账面总额，仅作房产线索；不能替代房源原值，也不能从其变化直接判定房产漏报。")
}

func advanceFinancialBlocks(ctx *reportContext) []Block {
	return financialBalanceBlocks(ctx, []string{"预收账款", "合同负债"}, "预收及合同负债辅助核对", "预收账款与合同负债分别列示。是否形成纳税义务需结合实际业务和收款时点，不自动将余额或余额增加额补入申报销售额。")
}
