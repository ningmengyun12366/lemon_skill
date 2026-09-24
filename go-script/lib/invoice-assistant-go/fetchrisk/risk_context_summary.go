package fetchrisk

type riskContextRow struct {
	FileID             string
	Direction          string
	InvoiceNo          string
	IssueMonth         string
	InvoiceType        string
	Status             string
	PositiveFlag       string
	HasPositiveFlag    bool
	BuyerName          string
	SellerName         string
	Amount             float64
	Tax                float64
	Total              float64
	InvoiceAmount      float64
	InvoiceTax         float64
	InvoiceTotal       float64
	HasDetailAmount    bool
	RowIndex           int
	AmountBasisDiffers bool
}

func buildRiskContextSummary(records []any, fileMetas []any) map[string]any {
	rows := parseRiskContextRows(records, directionByFileID(fileMetas))
	overview := &moneyAgg{}
	directionStats := map[string]*moneyAgg{"input": {}, "output": {}, "unknown": {}}
	statusByDirection := map[string]map[string]int{
		"input":  {"normal": 0, "reversed": 0, "void": 0, "unknown": 0},
		"output": {"normal": 0, "reversed": 0, "void": 0, "unknown": 0},
	}
	typeStats := map[string]map[string]*moneyAgg{"input": {}, "output": {}, "unknown": {}}
	monthly := map[string]map[string]*moneyAgg{}
	customerAmount, customerCount := map[string]float64{}, map[string]int{}
	supplierAmount, supplierCount := map[string]float64{}, map[string]int{}
	redFlushCount, voidCount, unknownDirectionCount := 0, 0, 0
	missingIssueDateCount, amountBasisDiffCount, negativeOrZeroCount := 0, 0, 0
	largeRecords := make([]riskContextRow, 0, len(rows))
	redFlushExamples := []any{}
	amountBasisExamples := []any{}

	for _, row := range rows {
		addMoneyAgg(overview, row.Amount, row.Tax, row.Total)
		dir := normalizedRiskDirection(row.Direction)
		addMoneyAgg(directionStats[dir], row.Amount, row.Tax, row.Total)
		addTypeAgg(typeStats[dir], row.InvoiceType, row.Amount, row.Tax, row.Total)
		if row.IssueMonth == "" {
			missingIssueDateCount++
		} else {
			addMonthlyAgg(monthly, row.IssueMonth, dir, row.Amount, row.Tax, row.Total)
		}
		statusKey := invoiceStatusKey(row.Status, row.PositiveFlag, row.Amount, row.HasPositiveFlag)
		if statusKey == "" {
			statusKey = "unknown"
		}
		if dir == "input" || dir == "output" {
			statusByDirection[dir][statusKey]++
		}
		switch statusKey {
		case "reversed":
			redFlushCount++
			redFlushExamples = appendRiskExample(redFlushExamples, row, "红冲或负数发票")
		case "void":
			voidCount++
		}
		if dir == "unknown" {
			unknownDirectionCount++
		}
		if row.AmountBasisDiffers {
			amountBasisDiffCount++
			amountBasisExamples = appendRiskExample(amountBasisExamples, row, "发票合计字段与明细字段存在差异")
		}
		if row.Total <= 0 {
			negativeOrZeroCount++
		}
		if row.Total != 0 {
			largeRecords = append(largeRecords, row)
		}
		if dir == "output" && row.BuyerName != "" && row.Amount > 0 {
			customerAmount[row.BuyerName] += row.Amount
			customerCount[row.BuyerName]++
		}
		if dir == "input" && row.SellerName != "" && row.Amount > 0 {
			supplierAmount[row.SellerName] += row.Amount
			supplierCount[row.SellerName]++
		}
	}

	return map[string]any{
		"overview": map[string]any{
			"record_count":  len(rows),
			"amount_basis":  "line_detail_preferred",
			"basis_note":    "明细金额/明细税额优先；缺失时回退发票金额/发票税额；价税合计缺失时按金额+税额计算。",
			"total":         moneyAggMap(overview),
			"input":         moneyAggMap(directionStats["input"]),
			"output":        moneyAggMap(directionStats["output"]),
			"unknown":       moneyAggMap(directionStats["unknown"]),
			"input_tax_gap": round2(directionStats["output"].Tax - directionStats["input"].Tax),
		},
		"monthly_direction_stats": buildMonthlyDirectionStats(monthly),
		"type_stats":              buildRiskTypeStats(typeStats),
		"status_stats": map[string]any{
			"red_flush_count": redFlushCount,
			"void_count":      voidCount,
			"by_direction":    statusByDirection,
		},
		"counterparty_stats": map[string]any{
			"top_customers":                topCounterpartyMaps(topCounterpartiesList(customerAmount, customerCount, 10)),
			"top_suppliers":                topCounterpartyMaps(topCounterpartiesList(supplierAmount, supplierCount, 10)),
			"customer_concentration_top5":  concentrationRatio(customerAmount, 5),
			"customer_concentration_top10": concentrationRatio(customerAmount, 10),
			"supplier_concentration_top5":  concentrationRatio(supplierAmount, 5),
			"supplier_concentration_top10": concentrationRatio(supplierAmount, 10),
		},
		"anomaly_indicators": map[string]any{
			"unknown_direction_count":           unknownDirectionCount,
			"missing_issue_date_count":          missingIssueDateCount,
			"red_flush_count":                   redFlushCount,
			"void_count":                        voidCount,
			"negative_or_zero_total_count":      negativeOrZeroCount,
			"amount_basis_difference_count":     amountBasisDiffCount,
			"input_tax_greater_than_output_tax": directionStats["input"].Tax > directionStats["output"].Tax,
			"large_amount_examples":             topRiskExamples(largeRecords, 10),
			"red_flush_examples":                redFlushExamples,
			"amount_basis_difference_examples":  amountBasisExamples,
		},
	}
}
