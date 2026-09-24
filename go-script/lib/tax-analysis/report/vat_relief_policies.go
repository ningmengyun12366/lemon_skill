package report

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type vatReliefPolicy struct {
	Name, Kind string
	Periods    []string
	Rows       [][]TaxFieldFact
}

// policyText removes PDF/HTML line-wrap spaces between Chinese characters,
// retaining meaningful spaces in Latin text. The untouched value stays in taxFields.
func policyText(raw string) string {
	runes := []rune(strings.TrimSpace(raw))
	var result []rune
	for i := 0; i < len(runes); i++ {
		if unicode.IsSpace(runes[i]) {
			j := i
			for j < len(runes) && unicode.IsSpace(runes[j]) {
				j++
			}
			if len(result) > 0 && j < len(runes) && !(unicode.Is(unicode.Han, result[len(result)-1]) && unicode.Is(unicode.Han, runes[j])) {
				result = append(result, ' ')
			}
			i = j - 1
		} else {
			result = append(result, runes[i])
		}
	}
	return string(result)
}

// vatReliefPolicies binds policy names and money to the same physical row of
// an effective VAT return. Merged cells and repeated modules never multiply it.
func vatReliefPolicies(ctx *reportContext) []vatReliefPolicy {
	rows := map[string][]TaxFieldFact{}
	effective := map[string]bool{}
	for _, v := range ctx.vat {
		effective[v.RecordID] = true
	}
	for _, f := range ctx.taxFields {
		if f.Module != "vat_returns" || (len(effective) > 0 && !effective[f.RecordID]) ||
			!(f.FormCode == "BDA0610758" || strings.Contains(f.FormName, "增值税减免税申报明细表")) {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%s|%d|%d", f.RecordID, f.Period.Start, f.Period.End, f.FormCode, f.Table, f.Row)
		rows[key] = append(rows[key], f)
	}
	groups := map[string]*vatReliefPolicy{}
	for _, key := range sortedMapKeys(rows) {
		fields := rows[key]
		name, kind := "", ""
		for _, f := range fields {
			if f.Status != "present_text" {
				continue
			}
			// “出口免税”第1行是固定栏目，不是企业下拉选择的优惠政策。
			if f.Line == "1" && canonicalFormLabel(f.Raw) == "出口免税" {
				continue
			}
			switch canonicalFormLabel(f.ColumnLabel) {
			case "免税性质代码及名称":
				name, kind = policyText(f.Raw), "免税"
			case "减税性质代码及名称":
				name, kind = policyText(f.Raw), "减税"
			}
			if name != "" {
				break
			}
		}
		if name == "" || containsAny(name, "合计", "总计") || strings.HasPrefix(canonicalFormLabel(name), "其中") ||
			financialStringSliceContains([]string{"请选择", "免税项目", "减税项目", "免税性质代码及名称", "减税性质代码及名称"}, normalizeFinancialMetricLabel(name)) {
			continue
		}
		groupKey := kind + "|" + name
		if groups[groupKey] == nil {
			groups[groupKey] = &vatReliefPolicy{Name: name, Kind: kind}
		}
		group := groups[groupKey]
		group.Periods = appendUnique(group.Periods, fields[0].Period.Display)
		group.Rows = append(group.Rows, fields)
	}
	var result []vatReliefPolicy
	for _, key := range sortedMapKeys(groups) {
		sort.Strings(groups[key].Periods)
		result = append(result, *groups[key])
	}
	return result
}

func vatPolicyTableRows(ctx *reportContext) [][]string {
	var rows [][]string
	for _, p := range vatReliefPolicies(ctx) {
		name := p.Name
		if index := strings.Index(name, "《"); index > 0 {
			name = strings.TrimSpace(name[:index])
		}
		coverage := strings.Join(p.Periods, "、")
		if len(p.Periods) > 1 {
			coverage = fmt.Sprintf("%s至%s，已取得%d期", p.Periods[0], p.Periods[len(p.Periods)-1], len(p.Periods))
		}
		rows = append(rows, []string{"已申报" + p.Kind + "政策", name, coverage})
	}
	return rows
}

func analysisVATReliefPolicies(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "vat.rates-reliefs"}
	for _, policy := range vatReliefPolicies(ctx) {
		key := "relief-policy." + analysisToken(policy.Kind+policy.Name)
		var sources []string
		for _, row := range policy.Rows {
			for _, f := range row {
				if f.Status == "present_text" && containsAny(f.ColumnLabel, "性质代码及名称") {
					sources = appendUnique(sources, "report-model.json/taxFields[id="+f.ID+"]")
					break
				}
			}
		}
		refs := []string{d.text(key+".name", "申报选择的"+policy.Kind+"政策及原文依据", "policy", policy.Name, sources...), d.text(key+".periods", "取得该政策的申报所属期", "coverage", strings.Join(policy.Periods, "、"), sources...)}
		for _, label := range []string{"免征增值税项目销售额", "扣除后免税销售额", "免税额", "本期实际抵减税额"} {
			sum, complete := 0.0, true
			var valueRefs []string
			for _, row := range policy.Rows {
				found := false
				for _, f := range row {
					if f.Status != "present" || canonicalFormLabel(f.ColumnLabel) != label {
						continue
					}
					id := d.number(key+"."+analysisToken(f.ID), label, "amount", "元", f.Period.Display, "", "减免明细表同一政策行原值；不与主表或其他金额栏重复相加", f.Value, "report-model.json/taxFields[id="+f.ID+"]")
					d.Metrics[len(d.Metrics)-1].Inputs = []CalculationInput{taxCalculationInput(f)}
					valueRefs = append(valueRefs, id)
					sum += f.Value
					found = true
					break // merged columns of one business amount are one value
				}
				complete = complete && found
			}
			if complete && len(valueRefs) > 0 {
				refs = append(refs, d.number(key+".total."+analysisToken(label), "已取得政策期间的"+label+"合计", "amount", "元", strings.Join(policy.Periods, "、"), "", "同政策、同金额栏按已取得申报期间相加；不代表其他缺失期间", sum, valueRefs...))
			}
		}
		d.focus(key, "说明所选政策、期间和金额，并结合实际开票业务核对对应关系；政策在不同期间切换时指出具体核对方向，不直接认定选错或资格已验证。", refs...)
	}
	d.Constraints = []string{"免税性质代码及名称、减税性质代码及名称来自申报减免明细表；原文只有政策名称时，不编造独立代码。", "只把同一政策行和明确对应期间的金额联系起来，不把报告全部免税销售额自动归给某一政策；减税和免税分开，不重复累计合并单元格或跨模块副本。", "政策已选择不代表实际业务资格及共同进项分摊已核验；有政策名称时不得再声称未取得所选政策。"}
	addVATReliefTransferFacts(&d, ctx)
	var businessRefs []string
	if scope := firstText(qixinSubjectData(qixinOwnSubject(ctx), qixinDetailField), "Scope"); scope != "" {
		businessRefs = append(businessRefs, d.text("relief-business.scope", "工商经营范围", "object", scope, "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects[role=企业自身]/detail/Scope"))
	}
	for _, category := range ctx.categories {
		businessRefs = append(businessRefs, d.text("relief-business."+analysisToken(category.Name), "已取得销项业务类别", "object", category.Name, "issued_invoices/chunks/groups"))
	}
	if len(vatReliefPolicies(ctx)) > 0 && len(businessRefs) > 0 {
		d.focus("relief-business", "将分期政策与已取得的业务类别、经营范围对应，指出政策切换需要核实的业务事项；经营范围和发票品名不能代替资质或逐笔业务证明。", businessRefs...)
	}
	return d
}

// Transfer totals and exempt-use rows are different metrics. Only effective
// returns and one physical row value per period enter the sums.
func addVATReliefTransferFacts(d *SectionAnalysisData, ctx *reportContext) {
	effective := map[string]bool{}
	for _, v := range ctx.vat {
		effective[v.RecordID] = true
	}
	for _, label := range []string{"本期进项税额转出额", "其中：免税项目用"} {
		byPeriod := map[string][]TaxFieldFact{}
		for _, f := range ctx.taxFields {
			if f.Module == "vat_returns" && effective[f.RecordID] && f.Status == "present" && (f.FormCode == "BDA0610608" || containsAny(f.FormName, "附列资料（二）", "附列资料二")) && canonicalFormLabel(f.RowLabel) == label && declarationLabelMatches(f.ColumnLabel, []string{"税额"}) {
				byPeriod[f.Period.Display] = append(byPeriod[f.Period.Display], f)
			}
		}
		sum, complete := 0.0, len(ctx.vat) > 0
		var refs, periods []string
		for _, v := range ctx.vat {
			fields := byPeriod[v.Period]
			if len(fields) == 0 {
				complete = false
				continue
			}
			value, consistent := fields[0].Value, true
			var sources []string
			for _, f := range fields {
				consistent = consistent && f.Value == value
				sources = appendUnique(sources, "report-model.json/taxFields[id="+f.ID+"]")
			}
			if !consistent {
				complete = false
				continue
			}
			id := d.number("relief-transfer."+analysisToken(label+v.Period), label, "amount", "元", v.Period, "", "附表二对应用途行本期税额原值；重复副本只取一次", value, sources...)
			refs = append(refs, id)
			periods = append(periods, v.Period)
			sum += value
		}
		if len(refs) > 0 {
			key := "relief-transfer." + analysisToken(label)
			refs = append(refs, d.number(key+".total", label+"已取得期间合计", "amount", "元", strings.Join(periods, "、"), "", "仅合计已取得且无冲突期间；不代表缺失期间", sum, refs...))
			coverage := "已覆盖全部有效增值税申报期间"
			if !complete {
				coverage = "部分申报期间未取得本行金额，合计仅限已取得期间"
			}
			d.text(key+".coverage", label+"覆盖", "coverage", coverage, refs...)
			d.focus(key, "区分进项转出总额与免税项目用途行；已取得的零值不能写成缺失，总转出额不能全部归因于免税。是否需要共同进项分摊及其依据仍须业务归集和凭证核验。", refs...)
		}
	}
	d.Constraints = append(d.Constraints, "已取得的申报转出数据与未核验的共同进项分摊凭证分开描述；不得笼统称转出资料未取得。")
}
