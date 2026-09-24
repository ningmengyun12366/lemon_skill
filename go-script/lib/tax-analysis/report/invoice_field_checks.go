package report

import (
	"fmt"
)

// invoiceFieldCheckRows reports the coverage and results of each Word-required
// check separately. Missing rate/identity evidence is never a zero-risk verdict.
func invoiceFieldCheckRows(ctx *reportContext) [][]string {
	rows := [][]string{}
	ownName := firstText(ctx.modules["taxpayer_profile"].Raw, "nsrmc", "taxpayerName")
	for _, direction := range []struct{ module, name, ownTax, ownName, otherTax, otherName string }{
		{"issued_invoices", "销项", "xfsbh", "xfmc", "gfsbh", "gfmc"},
		{"received_invoices", "进项", "gfsbh", "gfmc", "xfsbh", "xfmc"},
	} {
		module := ctx.modules[direction.module]
		checks, identityMismatch, nameMismatch, multipleCodes, multipleRates := mergedInvoiceChecks(module, ctx.task.USCC, ownName)
		if checks.Records == 0 {
			rows = append(rows, []string{direction.name + "发票字段检查", "未取得可核验的发票资料", "数据不足"})
			continue
		}
		identityKnown, missingItem, missingCode, missingRate, directAbnormal := checks.IdentityKnown, checks.MissingItem, checks.MissingCode, checks.MissingRate, checks.DirectAbnormal
		identityComplete := module.Complete && identityKnown == checks.Records && ctx.task.USCC != "" && ownName != ""
		// Classification, tax-rate eligibility and historical counterparty status
		// are not supplied by these counts. Zero clues cannot prove those checks clear.
		rows = append(rows,
			[]string{direction.name + "抬头及税号一致性", fmt.Sprintf("完整取得身份信息%d张，本企业税号不一致%d张，名称不一致%d张", identityKnown, identityMismatch, nameMismatch), invoiceFieldCheckStatus(identityMismatch > 0 || nameMismatch > 0, identityComplete)},
			[]string{direction.name + "品名与分类编码", fmt.Sprintf("缺品名%d行，缺编码%d行，同品名对应多编码%d组", missingItem, missingCode, multipleCodes), invoiceFieldCheckStatus(multipleCodes > 0, false)},
			[]string{direction.name + "同类业务税率/征收率", fmt.Sprintf("缺税率%d行，同对手方同月同编码多税率%d组", missingRate, multipleRates), invoiceFieldCheckStatus(multipleRates > 0, false)},
			[]string{direction.name + "异常主体及凭证", fmt.Sprintf("已取得的票面风险状态中标注异常凭证/失控%d张", directAbnormal), invoiceFieldCheckStatus(directAbnormal > 0, false)},
		)
	}
	return rows
}

// invoiceFieldCheckStatus preserves observed clues and missing evidence in the
// existing risk/status column; it does not create findings or risk thresholds.
func invoiceFieldCheckStatus(hasClue, complete bool) string {
	if hasClue {
		return "待核实"
	}
	if complete {
		return "未见明显异常"
	}
	return "数据不足"
}
