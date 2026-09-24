package report

import "fmt"

// Only year-end outgoing linkage observations are present in invoice-summary.
// They must not be promoted to whole-report or incoming-invoice match coverage.
func analysisRedLinkCoverage(ctx *reportContext) []SectionAnalysisData {
	var result []SectionAnalysisData
	for _, key := range []string{"invoice.red-output", "invoice.year-end-reversal", "vat.special-items"} {
		d := SectionAnalysisData{SectionID: key}
		source := "source-snapshot.json/moduleResults[code=issued_invoices]/data/yearEndObservations"
		d.text("red-link.scope", "红蓝票关联资料覆盖范围", "coverage", "仅可能取得跨年销项观察窗口；全报告期逐票关联数量、全量匹配比例及进项关联均未提供", source)
		if len(ctx.yearEndReversal.Observations) == 0 {
			d.text("red-link.status", "红蓝票关联状态", "state", "本期未取得跨年关联观察；无法确认报告期红票与原蓝票的匹配情况", source)
		}
		for _, observation := range ctx.yearEndReversal.Observations {
			prefix := fmt.Sprintf("red-link.%d", observation.BaseYear)
			period := yearEndReversalPeriod(observation.BaseYear)
			ref := fmt.Sprintf("%s[baseYear=%d]", source, observation.BaseYear)
			status := "关联资料不完整，不推断全量匹配或未匹配金额"
			if observation.LinkDataComplete {
				status = "观察窗口关联资料完整，仅可判断与该年12月原蓝票的对应关系"
			}
			if !observation.AmountDataComplete {
				status = "观察窗口金额资料不足，关联金额及比例无法完整核实"
			}
			d.text(prefix+".period", "销项关联观察期间", "period", period, ref)
			d.text(prefix+".status", "销项关联观察状态", "state", status, ref)
			if observation.AmountDataComplete {
				d.number(prefix+".red-amount", "观察窗口次年1—3月红字总额", "amount", "元", period, "", "跨年观察原金额；不是全报告期红字合计", observation.NextQuarterRedAmount, ref)
				if observation.LinkDataComplete || observation.LinkedRedAmount > 0 {
					d.number(prefix+".linked-amount", "已识别对应本年12月蓝票的次年红字金额", "amount", "元", period, "", "仅反映已识别的跨年对应金额；不等于全部红字匹配覆盖", observation.LinkedRedAmount, ref)
				}
			}
		}
		d.Constraints = []string{"本节与3.15、3.16、5.6共用红蓝票关联覆盖事实。同月或同季度同时有红票和蓝票，不证明是对应原票；部分观察匹配不能写成报告期全部匹配。", "缺少关联资料时未知不记0；次年红字总额减已对应12月蓝票金额不等于未匹配金额，可能对应其他期蓝票。"}
		result = append(result, d)
	}
	return result
}

func analysisInvoiceFieldQuality(ctx *reportContext) SectionAnalysisData {
	d := SectionAnalysisData{SectionID: "invoice.field-quality"}
	d.Constraints = []string{"本企业名称/税号校验：销项比对销售方，进项比对购买方；名称不一致应核对本企业历史名称，不能转述成对手方更名。"}
	ownName := firstText(ctx.modules["taxpayer_profile"].Raw, "nsrmc", "taxpayerName")
	for _, direction := range []struct{ module, name string }{{"issued_invoices", "销项"}, {"received_invoices", "进项"}} {
		checks, taxMismatch, nameMismatch, codes, rates := mergedInvoiceChecks(ctx.modules[direction.module], ctx.task.USCC, ownName)
		if checks.Records == 0 {
			continue
		}
		source := direction.module + "/chunks/checks"
		for _, item := range []struct {
			key, label, unit string
			count            int
		}{
			{"tax-id", "本企业税号不一致", "张", taxMismatch}, {"name", "本企业名称不一致", "张", nameMismatch},
			{"item", "缺品名", "行", checks.MissingItem}, {"code", "缺编码", "行", checks.MissingCode},
			{"rate", "缺税率", "行", checks.MissingRate}, {"codes", "同品名对应多编码", "组", codes},
			{"rates", "同对手方同月同编码多税率", "组", rates}, {"abnormal", "票面风险标注异常凭证/失控", "张", checks.DirectAbnormal},
		} {
			if item.count <= 0 {
				continue
			}
			key := "field-quality." + direction.module + "." + item.key
			ref := d.number(key, direction.name+item.label, "field-check", item.unit, "", "", "沿用字段核验表的同方向计数；张、行、组不混加", float64(item.count), source)
			d.focus(key, "优先解释已取得的非零字段核验事项及影响范围，区分本企业名称/税号、字段缺失和多编码/多税率；不只重复另一方向全部为零的项目，不把资料缺项直接判为原票违法。", ref)
		}
	}
	return d
}
