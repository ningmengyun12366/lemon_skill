package report

import "fmt"

type partyIdentityStatus string

const (
	partyIdentityVerified  partyIdentityStatus = "verified"
	partyIdentityUnmatched partyIdentityStatus = "unmatched"
	partyIdentityUnqueried partyIdentityStatus = "unqueried"
)

type AnalysisQueryCoverage struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Outcome     string `json:"outcome"`
	RecordCount *int   `json:"recordCount,omitempty"`
	TotalCount  *int   `json:"totalCount,omitempty"`
}

type AnalysisPartyCoverage struct {
	Name           string                  `json:"name"`
	Role           string                  `json:"role"`
	Identity       string                  `json:"identity"`
	IdentityStatus partyIdentityStatus     `json:"identityStatus"`
	Queries        []AnalysisQueryCoverage `json:"queries,omitempty"`
	EvidenceRefs   []string                `json:"evidenceRefs"`
}

// analysisPartyCoverage includes unmatched/unqueried invoice counterparties,
// rather than inferring complete coverage from the verified subject subset.
func analysisPartyCoverage(ctx *reportContext, d *SectionAnalysisData, item counterpartyItem, role string) {
	c := AnalysisPartyCoverage{Name: item.Name, Role: role, Identity: "未纳入本次核验", IdentityStatus: partyIdentityUnqueried}
	var matched map[string]any
	for _, raw := range arrayAt(ctx.modules[qixinModuleCode].Raw, qixinSubjectsField) {
		subject, ok := raw.(map[string]any)
		if !ok || mapText(subject, qixinSubjectRoleField) != role {
			continue
		}
		id := invoicePartyTaxID(item.TaxID)
		if id != "" && id == invoicePartyTaxID(mapText(subject, "taxId")) ||
			id == "" && invoicePartyName(item.Name) != "" && invoicePartyName(item.Name) == invoicePartyName(mapText(subject, "name")) {
			matched = subject
			break
		}
	}
	if matched != nil {
		c.IdentityStatus = partyIdentityUnmatched
		c.Identity = "已查询，未精确匹配身份"
		if boolText(matched, qixinSubjectVerifiedField) {
			c.IdentityStatus = partyIdentityVerified
			c.Identity = "已精确匹配身份"
			if link := qixinInvoiceNameLink(ctx, matched); link != "" {
				c.Identity += "；" + link
			}
			if detail, returned := mapValue(matched, qixinDetailField); returned && !failedQixinPayload(detail) {
				status := "已返回，未取得工商详情"
				if object, ok := detail.(map[string]any); ok && len(object) > 0 {
					status = "已取得工商详情；经营异常另行统计"
				}
				c.Queries = append(c.Queries, AnalysisQueryCoverage{Name: "工商详情", Status: status, Outcome: "returned"})
			} else {
				c.Queries = append(c.Queries, AnalysisQueryCoverage{Name: "工商详情", Status: "未取得有效工商详情查询结果", Outcome: "unavailable"})
			}
			for _, query := range []struct{ field, name string }{
				{qixinDetailField, "经营异常"}, {qixinExecutionField, "被执行"},
				{qixinDishonestField, "失信被执行"}, {qixinAdministrativePunishmentField, "行政处罚"},
				{qixinSeriousIllegalField, "严重违法"},
			} {
				result := qixinQueryRecords(matched, query.field)
				q := AnalysisQueryCoverage{Name: query.name, Status: result.Reason, Outcome: "unavailable", TotalCount: result.Total}
				count := len(result.Rows)
				if result.Known || count > 0 {
					q.RecordCount = &count
					q.Outcome, q.Status = "records", fmt.Sprintf("已返回，查到%d条记录；不代表当前均未处理", count)
					if count == 0 {
						q.Outcome, q.Status = "empty", "已返回，本项未查询到记录"
					}
					if !result.Known {
						q.Outcome, q.Status = "partial", fmt.Sprintf("已取得%d条记录；%s", count, result.Reason)
					}
				}
				if result.Known && count > 0 && (query.field == qixinDetailField || query.field == qixinSeriousIllegalField) {
					q.Status = qixinListingSummary(matched, query.field, query.name)
				}
				c.Queries = append(c.Queries, q)
			}
		}
	}
	prefix := "coverage." + analysisToken(role+"|"+item.TaxID+"|"+item.Name)
	source := "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects"
	c.EvidenceRefs = append(c.EvidenceRefs, d.text(prefix+".identity", item.Name+"工商核验覆盖", "coverage", c.Identity, source))
	for i, query := range c.Queries {
		c.EvidenceRefs = append(c.EvidenceRefs, d.text(fmt.Sprintf("%s.query%d", prefix, i), item.Name+query.Name+"查询覆盖", "coverage", query.Status, source))
	}
	d.Coverage = append(d.Coverage, c)
}

func analysisCounterpartyCoverage(ctx *reportContext) []SectionAnalysisData {
	var result []SectionAnalysisData
	for _, group := range []struct {
		analysis *counterpartyAnalysis
		role     string
		sections []string
	}{
		{ctx.customers, qixinImportantCustomerRole, []string{"invoice.customers", "invoice.customer-credit", "enterprise.judicial"}},
		{ctx.suppliers, qixinImportantSupplierRole, []string{"invoice.suppliers", "invoice.supplier-credit", "enterprise.judicial"}},
	} {
		if group.analysis == nil {
			continue
		}
		for _, key := range group.sections {
			d := SectionAnalysisData{SectionID: key}
			for _, item := range group.analysis.Items {
				analysisPartyCoverage(ctx, &d, item, group.role)
			}
			d.Constraints = []string{"工商核验仅覆盖coverage中所列主体，记录数与查询完成分开；records/partial表示已有记录，empty只说明该主体该项无记录，unavailable不能记为零。", "优先说明已查到的事件，再说明未核验范围；不能概括所有已匹配主体均为空。未匹配身份的英文名称不证明境外注册或出口业务。"}
			d.Constraints = append(d.Constraints, "发票金额及集中度按已取得的同方向发票统计，工商/信用核验只约束身份及公开信用结论；未匹配主体仍参与发票金额统计。")
			result = append(result, d)
		}
	}
	return result
}

// addCoverageSummary counts only this section's explicit subject set, rather
// than mixing task-wide warnings or successful subjects from another ranking.
func addCoverageSummary(d *SectionAnalysisData) {
	if len(d.Coverage) == 0 {
		return
	}
	counts := map[partyIdentityStatus]int{partyIdentityVerified: 0, partyIdentityUnmatched: 0, partyIdentityUnqueried: 0}
	var refs []string
	for _, party := range d.Coverage {
		counts[party.IdentityStatus]++
		refs = append(refs, party.EvidenceRefs...)
	}
	d.number("coverage-summary.total", "本节核验名单主体数", "coverage", "家", "本节名单", "", "仅统计本节coverage；其他任务warning不混入", float64(len(d.Coverage)), refs...)
	for _, item := range []struct{ key, name string }{{"verified", "本节身份已精确匹配主体数"}, {"unmatched", "本节已查询但身份未匹配主体数"}, {"unqueried", "本节未纳入查询主体数"}} {
		d.number("coverage-summary."+item.key, item.name, "coverage", "家", "本节名单", "", "按本节主体身份状态分别计数；已匹配不代表所有查询无记录", float64(counts[partyIdentityStatus(item.key)]), refs...)
	}
	d.text("coverage-summary.scope", "工商信用核验与发票统计范围", "coverage", "主体核验数量仅说明工商身份及公开信用查询覆盖；发票金额、交易占比及集中度仍按已取得的同方向发票统计，不排除工商未匹配或未查询主体。", refs...)
}
