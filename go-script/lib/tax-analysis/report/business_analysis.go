package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

type businessEvidence struct {
	Name, Code                     string
	Amount                         float64
	Count                          int
	RedAmount                      float64
	RedCount                       int
	RedRefs                        []string
	Rates, Refs, RateRefs, Periods []string
}

// businessEvidenceGroups retains actual product names, codes and tax rates
// before the display tables summarize categories. No inferred BOM is created.
func businessEvidenceGroups(ctx *reportContext, module string) []businessEvidence {
	summary := readInvoiceSummary(ctx.modules[module])
	rates := map[string][]string{}
	rateRefs := map[string][]string{}
	for _, chunk := range summary.Chunks {
		for index, raw := range chunk.Checks.Rates {
			var entry []json.RawMessage
			if json.Unmarshal(raw, &entry) != nil || len(entry) != 2 {
				continue
			}
			var key string
			var values []string
			if json.Unmarshal(entry[0], &key) == nil && json.Unmarshal(entry[1], &values) == nil {
				rates[key] = append(rates[key], values...)
				rateRefs[key] = append(rateRefs[key], fmt.Sprintf("source-snapshot.json/moduleResults[code=%s]/data/chunks[month=%s]/checks/rates[%d]", module, chunk.Month, index))
			}
		}
	}
	groups := map[string]*businessEvidence{}
	facts := ctx.issued
	if module == "received_invoices" {
		facts = ctx.received
	}
	for _, g := range facts {
		for i, line := range g.Lines {
			if !line.HasAmount {
				continue
			}
			name := textOr(line.Item, line.Category)
			if name == "" {
				continue
			}
			key := name + "|" + line.TaxCode
			if groups[key] == nil {
				groups[key] = &businessEvidence{Name: name, Code: line.TaxCode}
			}
			b := groups[key]
			b.Amount += line.Amount
			b.Count += line.Count
			if g.Red {
				b.RedAmount += line.Amount
				b.RedCount += line.Count
				b.RedRefs = append(b.RedRefs, fmt.Sprintf("source-snapshot.json/moduleResults[code=%s]/data/chunks/groups[id=%s]", module, g.ID))
			}
			b.Periods = append(b.Periods, g.Month)
			rateKey := g.CounterpartyID + "|" + g.Month + "|" + line.TaxCode
			b.Rates = append(b.Rates, rates[rateKey]...)
			b.RateRefs = append(b.RateRefs, rateRefs[rateKey]...)
			b.Refs = append(b.Refs, fmt.Sprintf("source-snapshot.json/moduleResults[code=%s]/data/chunks/groups[id=%s]/lines[%d]", module, g.ID, i))
		}
	}
	var result []businessEvidence
	for _, key := range sortedMapKeys(groups) {
		b := groups[key]
		b.Rates = uniqueSorted(b.Rates)
		b.RateRefs = uniqueSorted(b.RateRefs)
		b.Periods = uniqueSorted(b.Periods)
		result = append(result, *b)
	}
	// Order by magnitude without discarding small categories from the evidence file.
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && abs(result[j].Amount) > abs(result[j-1].Amount); j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	return result
}

// candidateBusinessLink supplies a business explanation to review, not a
// finding that the purchased item was used to produce a particular sold item.
func candidateBusinessLink(input, output string) string {
	if canonicalFormLabel(input) == canonicalFormLabel(output) {
		return "同类商品；可核对贸易转售或生产耗用"
	}
	if containsAny(input, "元件", "芯片", "电阻", "电容", "电路", "配件", "半导体", "模块") && containsAny(output, "电子", "设备", "计算机", "网络", "通信", "仪器", "振荡", "组件", "电路") {
		return "原料或电子部件可用于设备、组件生产或集成；实际用途需业务资料确认"
	}
	if containsAny(input, "钢材", "铝材", "铜材", "塑料", "五金") && containsAny(output, "设备", "机械", "部件", "五金", "制品") {
		return "原材料可用于加工制造；实际耗用需业务资料确认"
	}
	if containsAny(input, "软件", "技术服务", "开发", "系统") && containsAny(output, "软件", "技术", "集成", "信息", "系统") {
		return "外购软件或技术服务可支持开发、集成或项目交付"
	}
	if containsAny(input, "运输", "物流", "仓储", "包装") && !containsAny(output, "服务", "咨询", "劳务") {
		return "运输、仓储或包装可支持货物购销"
	}
	return ""
}

func analysisBusinessMatching(ctx *reportContext) []SectionAnalysisData {
	purchase := businessEvidenceGroups(ctx, "received_invoices")
	sales := businessEvidenceGroups(ctx, "issued_invoices")
	scope := firstText(qixinSubjectData(qixinOwnSubject(ctx), qixinDetailField), "Scope")
	var result []SectionAnalysisData
	for _, key := range []string{"invoice.category-match", "invoice.business-match"} {
		d := SectionAnalysisData{SectionID: key}
		scopeRef := d.text("business.scope", "工商经营范围", "object", textOr(scope, "未取得"), "qixin_external/subjects[role=企业自身]/detail/Scope")
		if scope != "" {
			clauses := strings.FieldsFunc(scope, func(c rune) bool { return c == '；' || c == ';' || c == '。' })
			for i, clause := range clauses {
				if strings.TrimSpace(clause) != "" {
					d.text(fmt.Sprintf("business.scope.clause-%d", i+1), "工商经营范围原文条款", "object", strings.TrimSpace(clause), scopeRef)
				}
			}
		}
		var mainInput, mainOutput []string
		for _, direction := range []string{"购进", "销项"} {
			items := sales
			if direction == "购进" {
				items = purchase
			}
			if key == "invoice.business-match" && direction == "购进" {
				continue
			}
			total := 0.0
			for _, item := range items {
				total += item.Amount
			}
			for index, item := range items {
				prefix := "business." + analysisToken(direction+"|"+item.Name+"|"+item.Code)
				name := d.text(prefix+".name", direction+"实际品名", "object", item.Name, item.Refs...)
				amount := d.number(prefix+".amount", direction+"该品名金额", "amount", "元", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "相同方向、品名与编码的有符号不含税金额合计", item.Amount, item.Refs...)
				count := d.number(prefix+".count", direction+"该品名明细条数", "coverage", "笔", "", "", "服务端对应明细行计数；不等于发票份数", float64(item.Count), item.Refs...)
				refs := []string{name, amount, count}
				if item.RedCount > 0 {
					refs = append(refs, d.number(prefix+".red-amount", direction+"该品名已确认红字金额", "amount", "元", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "仅汇总服务端red=true的同品名明细有符号金额", item.RedAmount, item.RedRefs...))
					refs = append(refs, d.number(prefix+".red-count", direction+"该品名已确认红字明细条数", "count", "条", "", "", "red=true汇总组内该品名明细行计数，不等于发票份数", float64(item.RedCount), item.RedRefs...))
				}
				if total > 0 {
					refs = append(refs, d.number(prefix+".share", direction+"该品名金额占比", "share", "%", "", "", "该品名金额/同方向已识别品名金额×100", item.Amount/total*100, item.Refs...))
				}
				if item.Code != "" {
					refs = append(refs, d.text(prefix+".code", "税收分类编码", "object", item.Code, item.Refs...))
				}
				if len(item.Rates) > 0 {
					refs = append(refs, d.text(prefix+".rates", "实际票面税率或征收率", "state", strings.Join(item.Rates, "、"), item.RateRefs...))
				}
				refs = append(refs, d.number(prefix+".months", "该品名出现月份数", "coverage", "月", "", "", "相同品名与编码的不同开票月份数", float64(len(item.Periods)), item.Refs...))
				if item.Amount < 0 && item.RedCount > 0 {
					d.Focus = append(d.Focus, AnalysisFocus{ID: key + "." + prefix + ".red", Goal: "负数品名已取得明确红字标识，说明红字金额与净额；尚未取得的原蓝票关系或红冲原因须单独说明，不能写成没有红字数据。", EvidenceRefs: refs})
				}
				if index == 0 {
					if direction == "购进" {
						mainInput = refs
					} else {
						mainOutput = refs
					}
				}
			}
		}
		if key == "invoice.category-match" && len(mainInput) > 0 && len(mainOutput) > 0 {
			d.focus("business-input-output", "结合主要采购与销售品名、金额占比、出现频次及经营范围，解释原料/部件到成品、贸易转售、外包或物流投入的可能业务联系；不得只按是否同名下结论，也不得声称已核实实际BOM。", append(append(mainInput, mainOutput...), scopeRef)...)
		} else if key == "invoice.business-match" && len(mainOutput) > 0 {
			d.focus("business-scope", "结合主要开票品名、编码、税率、金额占比与工商经营范围解释业务一致性；名称或子串不匹配不能认定超范围，必要的合同/业务资质未知时说明判断边界。", append(mainOutput, scopeRef)...)
		}
		d.Constraints = []string{"business.scope 保存完整经营范围，条款指标保存原文。客户正文只解释相关业务，不叙述脚本、底稿截断或工具执行过程。", "候选业务联系用于解释方向，不代表已核验合同、生产配方、实际材料耗用或服务成果。名称、税收编码和经营范围分别保留，不用子串命中直接认定合规。", "未取得业务用途或长期购销链路证据时，不计算已验证商业匹配度，不把无直接同名销项认定为无商业联系。", "税率只读取服务端已保存的票面税率集合，不用税额除金额推定法定税率。"}
		result = append(result, d)
	}
	return result
}

func analysisCounterpartyDependencies(ctx *reportContext) []SectionAnalysisData {
	var result []SectionAnalysisData
	for _, role := range []string{qixinImportantCustomerRole, qixinImportantSupplierRole} {
		facts, analysis, key, concentrationKey, module := ctx.issued, ctx.customers, "invoice.customers", "invoice.customer-concentration", "issued_invoices"
		if role == qixinImportantSupplierRole {
			facts, analysis, key, concentrationKey, module = ctx.received, ctx.suppliers, "invoice.suppliers", "invoice.supplier-concentration", "received_invoices"
		}
		d := SectionAnalysisData{SectionID: key}
		c := SectionAnalysisData{SectionID: concentrationKey}
		shares, amounts, total := concentrationShares(facts)
		var concentrationRefs []string
		for i, label := range concentrationLabels {
			if total <= 0 {
				continue
			}
			amount := c.number(fmt.Sprintf("concentration.%d.amount", i), label+"对应交易金额", "amount", "元", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "按同方向交易对手金额排名求和", amounts[i], module+"/chunks/groups")
			denominator := c.number(fmt.Sprintf("concentration.%d.total", i), "同方向发票总额", "total", "元", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "同方向有符号不含税金额合计", total, module+"/chunks/groups")
			concentrationRefs = append(concentrationRefs, c.number(fmt.Sprintf("concentration.%d.share", i), label, "share", "%", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "前N大金额/同方向总额×100", shares[i]*100, amount, denominator))
		}
		if len(concentrationRefs) > 0 {
			assessment := "发票覆盖不足，仅描述样本占比，不确定企业整体集中度等级"
			if ctx.invoiceCoverage[module].Complete {
				assessment = "一般"
				if level := concentrationLevel(shares[1], shares[2]); level != "" {
					assessment = string(level) + "管理预警，待核实经营依赖"
				}
			}
			concentrationRefs = append(concentrationRefs, c.text("concentration.assessment", "集中度规则结果及适用范围", "state", assessment, concentrationRefs...))
			c.focus("concentration-boundary", "说明主要交易占比与已计算的集中程度，不在正文展示阈值或公式；一般状态不得写集中度偏高。订单稳定性、替代供货及单一来源依赖须有协议或业务证据，不能从占比直接确认或排除。", concentrationRefs...)
		}
		if analysis != nil {
			for index, item := range analysis.Items {
				prefix := "counterparty." + analysisToken(item.Name+"|"+item.TaxID)
				name := d.text(prefix+".name", "主要交易对手", "object", item.Name, module+"/chunks/groups")
				amount := d.number(prefix+".amount", "交易对手金额", "amount", "元", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "同方向精确主体的有符号金额合计", item.Amount, module+"/chunks/groups")
				share := d.number(prefix+".share", "同方向交易占比", "share", "%", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "本主体金额/同方向总额×100", item.Share*100, module+"/chunks/groups")
				if total <= 0 {
					d.Metrics[len(d.Metrics)-1].Status = "unavailable"
				}
				relation := d.text(prefix+".relation", "已核验直接关系范围", "relationship", qixinVerifiedRelationSummary(ctx, item), "qixin_external/subjects[role=企业自身]/detail")
				addCounterpartyPeriodEvidence(&d, ctx, item, facts, module, prefix, name)
				if index == 0 {
					d.focus("counterparty-dependency", "说明已开票交易的主体和金额结构，区分已确认关系、已查未发现与未覆盖；已有真实品名关联可说明采购内容。没有订单、合同或交付证据时，仅提出对应对象的核实方向，不断言成本、议价或履约已受到影响，也不能因占比低排除业务依赖。", name, amount, share, relation)
				}
			}
		}
		d.RelatedSections = []string{concentrationKey}
		d.Constraints = append(d.Constraints, "本节已提供对手方分月金额与实际品名关联；只能将覆盖中缺失的月份说成未取得。期内开票变化不证明新增或流失客户，也不证明合同、回款或供货能力。")
		d.Constraints = append(d.Constraints, "直接股东或分支机构以报告企业自身名单为限，不等于完成对手方全部股权核验。", "发票金额占比只证明金额结构，不能确认或排除订单/回款依赖、不可替代性或履约影响；这些属于待核实的经营问题。")
		c.Constraints = []string{fmt.Sprintf("内部规则：前五大<%.0f%%且前十大<%.0f%%为一般；前五大≥%.0f%%或前十大≥%.0f%%为低；前五大≥%.0f%%或前十大≥%.0f%%优先判中。第一大占比不套用前五大阈值；规则与公式不写入客户报告。", concentrationFiveLow*100, concentrationTenLow*100, concentrationFiveLow*100, concentrationTenLow*100, concentrationFiveMedium*100, concentrationTenMedium*100), "发票覆盖不足只反映样本，不给企业整体依赖等级；框架客户、代理与项目型集中业务需要实际协议证据。", "集中度支持金额和占比结论，不证明替代能力强弱、未来供货中断、订单流失或经营稳定；这些只能在取得相应业务证据后分析，建议可核对相关合同但不可预言后果。"}
		result = append(result, d, c)
	}
	return result
}
