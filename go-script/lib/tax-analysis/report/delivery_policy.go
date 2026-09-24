package report

import (
	"fmt"
	"regexp"
	"strings"
)

var missingDeliveryValue = regexp.MustCompile(`^(?:[—–-]|未取得.*|未获取.*|未返回.*|未识别.*|未发现.*|未见.*|数据不足.*|暂无.*|尚无.*|无法.*|不适用.*|本期未.*|当前.*未.*)$`)
var numericDeliveryValue = regexp.MustCompile(`[+-]?\d[\d,]*(?:\.\d+)?%?`)

// Fixed texts are centralized so successful empty queries never trigger a
// speculative checklist. Keys are semantic section IDs, not tax form codes.
var deliveryMissingText = map[string]string{
	"financial.period-expense":  "本期未取得同一报表期完整的收入、成本及费用数据，暂无法计算相关财务比率。",
	"cit.annual":                "本期未取得企业所得税年度纳税调整明细，暂无法分析纳税调整情况。",
	"cit.preferences":           "本期未取得企业所得税年度优惠申报明细，暂无法分析优惠项目及申报金额。",
	"cit.research-expense":      "本期未取得研发加计扣除申报数据，暂无法分析研发加计扣除申报情况。",
	"cit.cost-purchases":        "本期未取得工资申报收入，暂无法计算财务成本费用扣除工资后与购进发票的差额。",
	"other-tax.stamp":           "本期未取得印花税申报或缴款数据，暂无法分析印花税申报缴纳情况。",
	"other-tax.property":        "本期未取得房产税税源、申报或缴款数据，暂无法分析房产税情况。",
	"other-tax.land-use":        "本期未取得城镇土地使用税税源、申报或缴款数据，暂无法分析城镇土地使用税情况。",
	"other-tax.union-fund":      "本期未取得工会经费申报或缴款数据，暂无法分析工会经费情况。",
	"chapter.financial":         "本期未取得完整的税务报送财务报表，暂无法开展财务指标分析。",
	"financial.cash-flow":       "本期未取得经营活动现金流量数据，暂无法分析经营活动现金流。",
	"vat.financial-comparison":  "本期未取得可与增值税销售额按同一期间比较的财务报表营业收入，暂无法分析两者差异。",
	"cit.financial-revenue":     "本期未取得可按同一期间比较的财务报表营业收入及企业所得税收入，暂无法分析两者差异。",
	"cit.profit-taxable":        "本期未取得可按同一期间比较的财务报表利润总额及企业所得税实际利润额，暂无法分析两者差异。",
	"invoice.year-end-reversal": "本次分析期间未覆盖年末开票及次年红冲所需的完整期间，暂无法进行相关分析。",
}
var deliveryClearText = map[string]string{
	"enterprise.changes":         "未查询到企业变更记录。",
	"invoice.red-output":         "本期未识别到红字发票。",
	"invoice.sensitive-services": "本期未识别到需要重点关注的敏感服务类发票事项。",
	"invoice.output-clues":       "本期未识别到疑似虚开销项发票的明显线索。",
	"invoice.input-clues":        "本期未识别到疑似取得虚假进项发票的明显线索。",
	"tax-payment.exceptions":     "本期已取得的申报及缴款记录中，未发现申报更正、逾期申报或欠税记录。",
	"enterprise.tax-violations":  "本期查询到的公开信息中，未发现企业税务处罚或重大税收违法记录。",
	"enterprise.compliance":      "本期查询到的公开信息中，未发现企业行政处罚、经营异常或严重违法失信记录。",
}

// applyDeliveryPolicies derives the final public structure from evidence before
// the Agent writes anything. Existing TaxFields/Calculations remain untouched.
func applyDeliveryPolicies(ctx *reportContext, sections []Section) []Section {
	sections = assignNarrativePolicies(sections, ctx.evaluations)
	for i := range sections {
		s := &sections[i]
		if s.Key == "chapter.financial" && !hasFinancialReportData(ctx) {
			setDeliveryNotice(s, NarrativeDataGap, deliveryMissingText[s.Key])
			continue
		}
		if s.Level == 1 {
			continue
		}
		if strings.HasPrefix(s.Key, "risk.") {
			continue
		}
		if strings.HasPrefix(s.Key, "remediation.") {
			continue
		}
		if s.Narrative.Mode == NarrativeRisk {
			continue
		}
		if s.Key == "vat.input-deduction" || s.Key == "vat.end-credit" {
			if ctx.taxpayer == taxpayerSmall {
				label := "进项抵扣"
				if s.Key == "vat.end-credit" {
					label = "留抵税额"
				}
				setDeliveryNotice(s, NarrativeNotApplicable, "企业当前为小规模纳税人，不涉及一般计税项目的"+label+"分析。")
				continue
			}
			if ctx.taxpayer == taxpayerUnknown {
				setDeliveryNotice(s, NarrativeDataGap, "本期未取得明确的纳税人资格信息，暂无法判断本项分析是否适用。")
				continue
			}
		}
		if s.Key == "invoice.specific-fields" && len(s.Blocks) == 1 && s.Blocks[0].Kind == BlockParagraph && s.Blocks[0].Title == "数据说明" {
			setDeliveryNotice(s, NarrativeDataGap, strings.Join(s.Blocks[0].Paragraphs, ""))
			continue
		}
		if s.Key == "financial.summary" && len(s.Blocks) > 0 {
			s.Narrative = NarrativePolicy{Mode: NarrativeAttention, MinItems: 1, MaxItems: descriptionMaxItems}
			continue
		}
		if notice := deliveryDependencyGap(ctx, *s); notice != "" {
			setDeliveryNotice(s, NarrativeDataGap, notice)
			continue
		}
		coverageInvalid := false
		for _, source := range sectionInvoiceSources(s.Key) {
			coverage := ctx.invoiceCoverage[source]
			if !coverage.Reconciled {
				setDeliveryNotice(s, NarrativeDataGap, coverage.Notice)
				coverageInvalid = true
				break
			}
			if !coverage.Complete {
				s.SampleOnly = true
				s.DataNotice = coverage.Notice
			}
		}
		if coverageInvalid {
			continue
		}
		if s.Key == "invoice.association-clues" {
			state := associationRelationState(ctx)
			if state == qixinRelationNotFound {
				if ctx.modules["issued_invoices"].Complete && ctx.modules["received_invoices"].Complete && ctx.invoiceCoverage["issued_invoices"].Complete && ctx.invoiceCoverage["received_invoices"].Complete {
					setDeliveryNotice(s, NarrativeNoIssue, associationNoClueText)
				} else {
					setDeliveryNotice(s, NarrativeDataGap, "已取得交易对手在企业直接股东及分支机构名单内未发现关系；交易数据覆盖不完整，不能代表全部交易。")
				}
				continue
			}
			if state == qixinRelationInsufficient {
				setDeliveryNotice(s, NarrativeDataGap, "未取得完整的企业直接股东或分支机构资料，暂无法判断关联交易线索。")
				continue
			}
		}
		if s.Key == "industry.summary" {
			if len(ctx.categories) == 0 {
				setDeliveryNotice(s, NarrativeDataGap, "本期未取得可用于归纳主要业务的发票品类，暂无法开展本项分析。")
			} else {
				s.Narrative = NarrativePolicy{Mode: NarrativeAttention, MinItems: 1, MaxItems: descriptionMaxItems}
				if s.SampleOnly {
					s.Blocks = append([]Block{{Kind: BlockParagraph, Title: "数据说明", Paragraphs: []string{s.DataNotice}}}, s.Blocks...)
				}
			}
			continue
		}
		if s.SampleOnly && sectionHasStructuredFacts(*s) {
			s.Narrative = factNarrativePolicyForSection(*s)
			s.Blocks = append([]Block{{Kind: BlockParagraph, Title: "数据说明", Paragraphs: []string{s.DataNotice}}}, s.Blocks...)
			markSampleBlocks(s)
			continue
		}
		if clear := deliveryNoIssue(ctx, *s); clear != "" {
			setDeliveryNotice(s, NarrativeNoIssue, clear)
			continue
		}
		if !sectionHasStructuredFacts(*s) {
			notice := deliveryMissingText[s.Key]
			if s.Key == "financial.period-expense" {
				for _, f := range ctx.financial {
					if f.HasResearchExpense && !f.ResearchInManagementSet {
						notice = "本期无法判断研发费用是否已包含在管理费用中，暂无法计算去重后的期间费用率。"
						break
					}
				}
			}
			if notice == "" {
				notice = fmt.Sprintf("本期未取得%s所需的完整数据，暂无法开展本项分析。", s.BaseTitle)
			}
			setDeliveryNotice(s, NarrativeDataGap, notice)
			continue
		}
		if deliveryHasAttention(ctx, *s) {
			s.Narrative = NarrativePolicy{Mode: NarrativeAttention, MinItems: 1, MaxItems: 3}
		} else if sectionNeedsExplanation(ctx, s.Key) {
			s.Narrative.MinItems = 1
		}
	}
	assessment := assessOverallRisk(ctx.findings, ctx.evaluations)
	for i := range sections {
		if sections[i].Key != "risk.overall-evaluation" {
			continue
		}
		if assessment.Level == overallCoverageLimited {
			sections[i].Narrative = NarrativePolicy{Mode: NarrativeDataGap}
			sections[i].DataNotice = overallCoverageLimited
		}
	}
	return sections
}

// Only evidence-bearing explanation tasks become mandatory. This does not
// turn pure display chapters into prose or impose semantic checks on AI text.
func sectionNeedsExplanation(ctx *reportContext, key string) bool {
	switch key {
	case "vat.financial-comparison":
		for _, f := range ctx.financial {
			revenue, rok, vat, vok := financialVATPeriodValues(ctx, f)
			if vok && rok && abs(vat-revenue) >= invoiceAggregationTolerance {
				return true
			}
		}
	case "cit.quarter-contribution":
		for _, f := range ctx.cit {
			if f.HasTax && f.CumulativeTax == 0 || f.HasProfit && f.CumulativeProfit < 0 || f.HasResearch && f.CumulativeResearch != 0 {
				return true
			}
		}
	}
	return false
}

// setDeliveryNotice replaces placeholders with the single V4 state statement.
func setDeliveryNotice(s *Section, mode NarrativeMode, text string) {
	s.Narrative = NarrativePolicy{Mode: mode}
	s.DataNotice = ""
	if mode == NarrativeDataGap {
		s.DataNotice = text
	}
	s.Blocks = []Block{{Kind: BlockParagraph, Title: sectionNarrativeTitle(mode), Paragraphs: []string{text}}}
}

func deliveryDependencyGap(ctx *reportContext, s Section) string {
	switch s.Key {
	case "tax-payment.monthly-burden":
		if !hasCompleteDeliveryRow(s, []int{1}) && !hasCompleteDeliveryRow(s, []int{2}) {
			return "本期未取得可按实际申报周期对齐的营业收入与税费数据，暂无法计算综合税负率；已有税费金额趋势见前述章节。"
		}
	case "other-tax.stamp", "other-tax.property", "other-tax.land-use", "other-tax.union-fund":
		tax := map[string]string{"other-tax.stamp": "印花税", "other-tax.property": "房产税", "other-tax.land-use": "土地使用税", "other-tax.union-fund": "工会"}[s.Key]
		for _, f := range ctx.declarations {
			if strings.Contains(f.TaxName, tax) {
				return ""
			}
		}
		for _, f := range ctx.payments {
			if strings.Contains(f.TaxName, tax) {
				return ""
			}
		}
		for _, f := range ctx.taxLines {
			if strings.Contains(f.TaxName, tax) {
				return ""
			}
		}
		if s.Key == "other-tax.property" && len(declaredSourceBlocks(ctx, "property_tax", "房产税")) > 0 {
			return ""
		}
		if s.Key == "other-tax.land-use" && len(declaredSourceBlocks(ctx, "urban_land_use_tax", "土地使用税")) > 0 {
			return ""
		}
		return deliveryMissingText[s.Key]
	case "cit.cost-purchases":
		return deliveryMissingText[s.Key]
	case "vat.financial-comparison", "cit.financial-revenue", "cit.profit-taxable":
		if !hasCompleteDeliveryRow(s, []int{1, 2, 3}) {
			return deliveryMissingText[s.Key]
		}
	case "financial.cash-flow":
		for _, f := range ctx.financial {
			if f.HasOperatingCashFlow {
				return ""
			}
		}
		return deliveryMissingText[s.Key]
	case "financial.debt-asset":
		for _, f := range ctx.financial {
			if f.HasAssets && f.HasLiabilities && f.Assets != 0 {
				return ""
			}
		}
		return "本期未取得同一报表期完整的资产总额及负债总额，暂无法计算资产负债率。"
	case "invoice.year-end-reversal":
		if len(ctx.yearEndReversal.Observations) == 0 {
			return deliveryMissingText[s.Key]
		}
		for _, o := range ctx.yearEndReversal.Observations {
			if o.AmountDataComplete && o.LinkDataComplete {
				return ""
			}
		}
		return "本期未取得完整的蓝红票金额或对应原蓝票关联信息，暂无法计算对应红冲比例。"
	}
	return ""
}

func hasCompleteDeliveryRow(s Section, columns []int) bool {
	for _, b := range s.Blocks {
		if b.Kind != BlockTable {
			continue
		}
		for _, row := range b.Rows {
			valid := true
			for _, c := range columns {
				if c >= len(row) || !deliveryValuePresent(row[c]) || !numericDeliveryValue.MatchString(row[c]) {
					valid = false
					break
				}
			}
			if valid {
				return true
			}
		}
	}
	return false
}

func deliveryValuePresent(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !missingDeliveryValue.MatchString(value) &&
		!containsAny(value, "未取得", "未获取", "未返回", "暂无可", "数据不足", "暂无法", "暂不分析", "尚无足够")
}

// tableHasDeliveryFacts excludes labels, period-only placeholders and generated
// commentary columns. Explicit numeric zero remains valid source evidence.
func tableHasDeliveryFacts(block Block) bool {
	for _, row := range block.Rows {
		for column, value := range row {
			if column == 0 || column >= len(block.Headers) {
				continue
			}
			header := block.Headers[column]
			if containsAny(header, "关注", "风险", "结论", "说明", "分析", "趋势", "备注", "识别") {
				continue
			}
			if deliveryValuePresent(value) {
				return true
			}
		}
	}
	return false
}

// deliveryNoIssue needs successful, relevant collection evidence. Empty financial
// metrics and incomplete invoice samples cannot be promoted to NO_ISSUE.
func deliveryNoIssue(ctx *reportContext, s Section) string {
	text := deliveryClearText[s.Key]
	if sectionHasStructuredFacts(s) {
		return ""
	}
	if text == "" {
		return ""
	}
	if s.Key == "enterprise.changes" {
		if _, available := qixinChangeRecords(ctx); !available {
			return ""
		}
	}
	if strings.HasPrefix(s.Key, "invoice.") {
		for _, source := range []string{"issued_invoices", "received_invoices"} {
			if !ctx.modules[source].Complete {
				return ""
			}
		}
		if !ctx.invoiceCoverage["issued_invoices"].Complete {
			return ""
		}
	}
	if strings.HasPrefix(s.Key, "enterprise.") && len(qixinSubjects(ctx)) == 0 {
		return ""
	}
	if s.Key == "tax-payment.exceptions" && (!ctx.modules["filing_obligations"].Complete || !ctx.modules["payments"].Complete || !ctx.modules[degradableModuleCode].Complete) {
		return ""
	}
	if s.Key == "tax-payment.exceptions" && len(filingEvents(ctx.modules["filing_obligations"])) > 0 {
		return ""
	}
	if s.Key == "invoice.sensitive-services" {
		// Presence of actual rows keeps the section as a factual/attention sample.
		if sectionHasStructuredFacts(s) {
			return ""
		}
		return text
	}
	if len(s.RuleIDs) > 0 {
		found := false
		for _, e := range ctx.evaluations {
			if !stringSet(s.RuleIDs)[e.RuleID] {
				continue
			}
			found = true
			if e.Outcome != OutcomeClear {
				return ""
			}
		}
		if !found {
			return ""
		}
		return text
	}
	if !sectionHasStructuredFacts(s) {
		return text
	}
	return ""
}

func deliveryHasAttention(ctx *reportContext, s Section) bool {
	switch s.Key {
	case "invoice.sensitive-services", "invoice.specific-fields", "enterprise.judicial", "tax-payment.exceptions", "financial.debt-asset", "financial.working-capital", "financial.cash-flow":
		return sectionHasStructuredFacts(s)
	case "vat.input-deduction":
		return len(deductionAttentionRows(deductionConfirmationFacts(ctx.modules["deduction_confirmations"].Raw))) > 0
	}
	return false
}

func markSampleBlocks(s *Section) {
	for i := range s.Blocks {
		b := &s.Blocks[i]
		if b.Kind == BlockTable && b.Title != "" {
			b.Title += "（已取得样本）"
		}
		if b.Chart != nil {
			b.Chart.Title += "（已取得样本）"
			b.Chart.Summary = "仅反映已取得样本，不能代表企业整体情况。"
		}
	}
}
