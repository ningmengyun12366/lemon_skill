package report

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// declarationsWithDetailAmounts replaces each list total with declared detail
// amounts for the same record. It never adds a combined total to its children.
func declarationsWithDetailAmounts(original []filingFact, lines []taxDeclarationLine, vats []vatFact, start, end string) []filingFact {
	replaced := map[string]bool{}
	byKey := map[string]*filingFact{}
	var order []string
	for _, line := range lines {
		amount, ok := line.amount("应补（退）税额", "本期应补（退）税（费）额")
		if !ok || !monthWithinAnalysisPeriod(line.Period.BucketMonth, start, end) {
			continue
		}
		key := line.RecordID + "|" + line.TaxName
		if byKey[key] == nil {
			byKey[key] = &filingFact{RecordID: line.RecordID, Period: line.Period, TaxName: line.TaxName, Status: "已申报", HasDeclared: true, EffectiveOnly: true, SourceModule: "filing_obligations"}
			order = append(order, key)
		}
		byKey[key].Declared = math.Round((byKey[key].Declared+amount)*100) / 100
		if line.RecordID != "" {
			replaced[line.RecordID] = true
		}
	}
	for _, vat := range vats {
		if !vat.HasTaxPayable {
			continue
		}
		key := vat.RecordID + "|增值税|" + vat.Period
		byKey[key] = &filingFact{RecordID: vat.RecordID, Period: vat.TaxPeriod, TaxName: "增值税", Status: "已申报", Declared: vat.TaxPayable, HasDeclared: true, EffectiveOnly: true, SourceModule: "vat_returns"}
		order = append(order, key)
		if vat.RecordID != "" {
			replaced[vat.RecordID] = true
		}
	}
	var result []filingFact
	for _, fact := range original {
		if !replaced[fact.RecordID] {
			result = append(result, fact)
		}
	}
	for _, key := range order {
		result = append(result, *byKey[key])
	}
	result = dedupeFilingFacts(result, start, end)
	sortFilingFacts(result)
	return result
}

type taxDeclarationLine struct {
	Period      TaxPeriod
	RecordID    string
	ProjectCode string
	TaxName     string
	Item        string
	Fields      map[string]TaxFieldFact
}

// taxDeclarationLines groups one authority business row without recomputing
// declared rounding. The original project and the item's verified Chinese name
// stay separate; broad other-income rows can resolve an exact union item.
func taxDeclarationLines(fields []TaxFieldFact) []taxDeclarationLine {
	byKey := map[string]*taxDeclarationLine{}
	var order []string
	for _, fact := range fields {
		if strings.Contains(fact.Path, "historicalCapitalBooks") || strings.Contains(fact.Path, "referenceAnnualRecords") {
			continue
		}
		if financialStringSliceContains([]string{"合计", "总计", "-", "—", ""}, canonicalFormLabel(fact.RowLabel)) {
			continue
		}
		if !declarationLabelMatches(fact.ColumnLabel, []string{"计税依据", "计税（费）依据", "税率", "税（费）率", "税（费）率或单位税额", "应纳税额", "减免税额", "已缴税额", "应补（退）税额", "本期应纳税（费）额", "本期减免税（费）额", "本期已缴税（费）额", "本期已缴（费）额", "本期应补（退）税（费）额", "减征比例", "减征额", "增值税税额", "增值税限额减免金额", "增值税免抵税额", "留抵退税本期扣除额", "减免税（费）额", "本期抵免金额"}) {
			continue
		}
		project := ""
		if len(fact.ProjectCodes) == 1 {
			project = fact.ProjectCodes[0]
		}
		// Multi-tax annexes have an explicit business tax row. Resolve that row
		// through the dedicated project dictionary, never through yzpzzlDm.
		for _, candidate := range analysisTaxDictionary.Projects {
			if canonicalFormLabel(fact.RowLabel) == canonicalFormLabel(candidate.Name) {
				project = candidate.Code
				break
			}
		}
		if project == "" {
			continue
		}
		name := taxProjectName(project)
		if project == "39900" {
			for _, item := range analysisTaxDictionary.Items {
				if item.ProjectCode == project && (canonicalFormLabel(fact.RowLabel) == canonicalFormLabel(item.Name) || strings.HasSuffix(canonicalFormLabel(fact.RowLabel), "/"+canonicalFormLabel(item.Name))) {
					name = item.Name
					break
				}
			}
		}
		key := fmt.Sprintf("%s|%s|%s|%s|%d|%d", fact.RecordID, fact.Period.Start, fact.Period.End, fact.FormCode, fact.Table, fact.Row)
		line := byKey[key]
		if line == nil {
			line = &taxDeclarationLine{Period: fact.Period, RecordID: fact.RecordID, ProjectCode: project, TaxName: name, Item: fact.RowLabel, Fields: map[string]TaxFieldFact{}}
			byKey[key] = line
			order = append(order, key)
		}
		line.Fields[canonicalFormLabel(fact.ColumnLabel)] = fact
	}
	var result []taxDeclarationLine
	for _, key := range order {
		line := *byKey[key]
		if _, known := line.amount("应纳税额", "本期应纳税（费）额"); known {
			result = append(result, line)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		return a.Period.Start+a.Period.End+a.TaxName+a.Item < b.Period.Start+b.Period.End+b.TaxName+b.Item
	})
	return result
}

func (line taxDeclarationLine) amount(labels ...string) (float64, bool) {
	for _, label := range labels {
		if field, ok := line.Fields[canonicalFormLabel(label)]; ok && field.Status == "present" {
			return field.Value, true
		}
		keys := make([]string, 0, len(line.Fields))
		for key := range line.Fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			field := line.Fields[key]
			if declarationLabelMatches(key, []string{label}) && field.Status == "present" {
				return field.Value, true
			}
		}
	}
	return 0, false
}

func (line taxDeclarationLine) value(labels ...string) string {
	value, known := line.amount(labels...)
	if len(labels) > 0 && labels[0] == "计税依据" {
		value, known = line.basis()
	}
	return optionalMoney(value, known)
}

func (line taxDeclarationLine) basis() (float64, bool) {
	if value, ok := line.amount("计税依据", "计税（费）依据"); ok {
		return value, true
	}
	value := 0.0
	for index, label := range []string{"增值税税额", "增值税限额减免金额", "增值税免抵税额", "留抵退税本期扣除额"} {
		part, ok := line.amount(label)
		if !ok {
			return 0, false
		}
		if index == 3 {
			value -= part
		} else {
			value += part
		}
	}
	return value, true
}

func (line taxDeclarationLine) recomputedDue() (float64, bool) {
	basis, a := line.basis()
	rate, b := line.amount("税（费）率", "税率")
	relief, c := line.amount("减免税（费）额", "本期减免税（费）额", "减免税额")
	reduction, d := line.amount("减征额")
	credit, e := line.amount("本期抵免金额")
	paid, f := line.amount("本期已缴税（费）额", "本期已缴（费）额", "已缴税额")
	if !(a && b && c && d && e && f) {
		return 0, false
	}
	return math.Round((math.Round(basis*rate*100)/100-relief-reduction-credit-paid)*100) / 100, true
}

func declarationLineBlocks(ctx *reportContext, title string, match func(string) bool) []Block {
	return []Block{otherTaxSummaryBlock(ctx, title, match)}
}

func (line taxDeclarationLine) rateText() string {
	rate, known := line.amount("税率", "税（费）率", "税（费）率或单位税额")
	if !known {
		return "—"
	}
	if line.ProjectCode == "10112" {
		return moneyText(rate) + "元/平方米"
	}
	// Preserve small declared rates such as 0.025%; percentage charts may round
	// for presentation, but the declaration's rate must remain distinguishable.
	precision := 2
	decimal := strconv.FormatFloat(rate, 'f', -1, 64)
	if point := strings.IndexByte(decimal, '.'); point >= 0 {
		precision = max(precision, len(decimal)-point-3)
	}
	return strconv.FormatFloat(rate*100, 'f', precision, 64) + "%"
}

func otherTaxSummaryBlock(ctx *reportContext, title string, match func(string) bool) Block {
	period := analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime)
	block := Block{Kind: BlockTable, Title: title + "概览", Headers: []string{"项目", period + "金额", "趋势"}}
	switch title {
	case "印花税":
		block.Headers = []string{"项目", period + "估算/申报", "覆盖率/结论"}
	case "房产税":
		block.Headers = []string{"季度", "房产税实缴", "变化", "关注"}
	case "土地使用税":
		block.Headers = []string{"季度", "实缴税额", "变化", "结论"}
	case "附加税费":
		block.Headers = []string{"项目", period + "金额", "与增值税关系"}
	}
	if title == "房产税" || title == "土地使用税" {
		block.Rows = quarterlyOtherTaxPaymentRows(ctx, match)
		return block
	}
	paid := aggregatePaymentsByTaxName(ctx.payments, match)
	for _, row := range paid {
		block.Rows = append(block.Rows, []string{row[0], row[1], "实缴；实缴金额按税款所属期归集"})
	}
	if len(block.Rows) == 0 {
		block.Rows = [][]string{{title, "—", "本期未取得缴款记录，不能据此认定未申报或未缴税"}}
	}
	if title == "印花税" {
		block.Rows = append(block.Rows, capitalBookSummaryRows(ctx)...)
		block.Rows = append(block.Rows, stampContractCoverageRows(ctx)...)
	}
	if title == "附加税费" {
		block.Rows = append(block.Rows, surtaxReconciliationRows(ctx)...)
	}
	return block
}

// surtaxReconciliationRows exposes the original return and an independently
// rounded recomputation. A cent-level difference stays a reconciliation clue;
// the authority's declared amount is never rewritten.
func surtaxReconciliationRows(ctx *reportContext) [][]string {
	var rows [][]string
	for _, line := range ctx.taxLines {
		if !containsAny(line.TaxName, "城市维护建设税", "教育费附加") {
			continue
		}
		expected, expectedOK := line.recomputedDue()
		actual, actualOK := line.amount("本期应补（退）税（费）额", "应补（退）税额")
		known := expectedOK && actualOK
		var inputs []CalculationInput
		for _, key := range sortedMapKeys(line.Fields) {
			inputs = append(inputs, taxCalculationInput(line.Fields[key]))
		}
		ctx.recordCalculation("7.4", line.Period.Display, line.TaxName+"应补退复算差额 · "+line.RecordID, "申报应补退 - (按分舍入的计税依据×税率 - 减免 - 减征 - 抵免 - 已缴)", inputs, actual-expected, known)
		primaryPaid, primaryKnown := 0.0, false
		for _, payment := range ctx.payments {
			if payment.TaxName != "增值税" && payment.TaxName != "消费税" {
				continue
			}
			if payment.TaxPeriod.Start == line.Period.Start && payment.TaxPeriod.End == line.Period.End {
				primaryPaid += payment.Amount
				primaryKnown = true
			}
		}
		rows = append(rows, []string{line.Period.Display + " · " + line.TaxName + "复算", "申报应补退" + optionalMoney(actual, actualOK) + "；复算" + optionalMoney(expected, expectedOK) + "；差额" + optionalMoney(actual-expected, known), "同属期增值税/消费税实缴" + optionalMoney(primaryPaid, primaryKnown) + "；主税实缴与附加依据分别核实，保留分项舍入"})
	}
	return rows
}

// declaredSourceBlocks surfaces the exact declared source row, keeping area,
// original value and rent independent of taxable bases and source validity.
func declaredSourceBlocks(ctx *reportContext, module, title string) []Block {
	var rows [][]string
	for _, fact := range ctx.taxFields {
		if fact.Module != module || (fact.Status != "present" && fact.Status != "present_text") {
			continue
		}
		if !declarationLabelMatches(fact.ColumnLabel, []string{"房产原值", "其中：出租房产原值", "申报租金收入", "占用土地面积", "税额标准", "土地等级", "出租面积", "其中：出租房产面积"}) {
			continue
		}
		rows = append(rows, []string{fact.Period.Display, fact.ColumnLabel, fact.Raw, fact.Unit})
	}
	if len(rows) == 0 {
		return nil
	}
	return []Block{{Kind: BlockTable, Title: title + "申报税源字段", Headers: []string{"所属期", "税局中文字段", "原值", "单位"}, Rows: rows}}
}
