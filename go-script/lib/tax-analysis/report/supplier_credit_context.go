package report

import "strings"

// addSupplierCreditPurchaseContext projects existing party/product facts only
// for verified suppliers with public-credit events. Monthly records remain in
// the supplier section and are not copied into the credit reading material.
func addSupplierCreditPurchaseContext(ctx *reportContext, data []SectionAnalysisData) {
	var source, target *SectionAnalysisData
	for i := range data {
		switch data[i].SectionID {
		case "invoice.suppliers":
			source = &data[i]
		case "invoice.supplier-credit":
			target = &data[i]
		}
	}
	if source == nil || target == nil {
		return
	}
	added := false
	projected := map[string]bool{}
	for _, subject := range qixinSubjects(ctx) {
		if mapText(subject, qixinSubjectRoleField) != qixinImportantSupplierRole || !supplierHasCreditEvents(subject) {
			continue
		}
		item, ok := supplierCreditInvoiceItem(ctx, subject)
		if !ok {
			continue
		}
		prefix := "counterparty." + analysisToken(item.Name+"|"+item.TaxID)
		if projected[prefix] {
			continue
		}
		projected[prefix] = true
		sourcePrefix := metricID(source.SectionID, prefix)
		targetPrefix := metricID(target.SectionID, prefix)
		start := len(target.Metrics)
		selected := []AnalysisMetric{}
		for _, metric := range source.Metrics {
			if metric.ID == sourcePrefix+".name" || metric.ID == sourcePrefix+".amount" || strings.HasPrefix(metric.ID, sourcePrefix+".product.") {
				selected = append(selected, metric)
			}
		}
		// Credit queries can include a party outside the ranked positive-net list.
		// Reuse the same product extractor without manufacturing monthly facts.
		if len(selected) == 0 {
			fallback := SectionAnalysisData{SectionID: target.SectionID}
			name := fallback.text(prefix+".name", "发票交易主体", "object", item.Name, counterpartyInvoiceSource(qixinImportantSupplierRole))
			fallback.number(prefix+".amount", "交易对手金额", "amount", "元", analysisPeriodText(ctx.task.StartTime, ctx.task.EndTime), "", "同方向精确主体的有符号金额合计", item.Amount, counterpartyInvoiceSource(qixinImportantSupplierRole))
			addCounterpartyProductEvidence(&fallback, ctx, item, ctx.received, "received_invoices", prefix, name)
			target.Metrics = append(target.Metrics, fallback.Metrics...)
		}
		for _, metric := range selected {
			originalID := metric.ID
			metric.ID = strings.Replace(metric.ID, sourcePrefix, targetPrefix, 1)
			metric.SourceRefs = append([]string{originalID}, metric.SourceRefs...)
			target.Metrics = append(target.Metrics, metric)
		}
		name := mapText(subject, qixinSubjectNameField)
		for i := start; i < len(target.Metrics); i++ {
			if supplierCreditProductParty(target.SectionID, target.Metrics[i].ID) != "" {
				scope := appendUnique(append([]string(nil), target.Metrics[i].Scope...), item.Name)
				target.Metrics[i].Scope = appendUnique(scope, name)
			}
		}
		target.text(prefix+".credit-subject", "上述发票交易对应的已核验信用主体", "identity", name, metricID(target.SectionID, prefix+".name"), "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects[name="+name+"]")
		added = true
	}
	if added {
		target.RelatedSections = appendUnique(target.RelatedSections, source.SectionID)
		target.Constraints = appendUnique(target.Constraints, "采购背景沿用同一主体的发票品名、期间及有符号净额；品名已取得不等于采购用途或履约已核实，订单、交付、预付款及合同资料须另行核对。")
	}
}

func supplierHasCreditEvents(subject map[string]any) bool {
	for _, field := range []string{qixinExecutionField, qixinDishonestField} {
		if len(qixinItemMaps(qixinSubjectData(subject, field), "CaseNumber", "Court", "Amount", "ExecutionStatus")) > 0 {
			return true
		}
	}
	for _, field := range []string{qixinSeriousIllegalField, qixinAdministrativePunishmentField} {
		if len(counterpartyOtherCreditRows(subject, field)) > 0 {
			return true
		}
	}
	abnormal, _, _ := findFirst(qixinSubjectData(subject, qixinDetailField), "AbnormalItems")
	return len(qixinItemMaps(abnormal, "InReason", "InDate", "OutDate")) > 0
}

// Match the same resolved invoice identity used for ranking. Tax IDs take
// precedence; the missing-ID fallback requires an unambiguous exact name.
func supplierCreditInvoiceItem(ctx *reportContext, subject map[string]any) (counterpartyItem, bool) {
	id, name := invoicePartyTaxID(mapText(subject, "taxId")), invoicePartyName(mapText(subject, qixinSubjectNameField))
	keys := invoicePartyKeys(ctx.received)
	key := "id:" + id
	foundID := false
	if id != "" {
		for _, candidate := range keys {
			foundID = foundID || candidate == key
		}
	}
	if !foundID {
		if name == "" {
			return counterpartyItem{}, false
		}
		for _, group := range ctx.received {
			if invoicePartyName(group.Counterparty) == name && invoicePartyTaxID(group.CounterpartyID) != "" {
				return counterpartyItem{}, false
			}
		}
		identities := map[string]bool{}
		for _, other := range qixinSubjects(ctx) {
			if mapText(other, qixinSubjectRoleField) == qixinImportantSupplierRole && invoicePartyName(mapText(other, qixinSubjectNameField)) == name {
				identities[invoicePartyTaxID(mapText(other, "taxId"))] = true
			}
		}
		if len(identities) != 1 {
			return counterpartyItem{}, false
		}
		key = "name:" + name
	}
	item, found := counterpartyItem{}, false
	for i, group := range ctx.received {
		if keys[i] != key || !group.HasAmount {
			continue
		}
		if !found {
			item.Name = textOr(strings.TrimSpace(group.Counterparty), id)
			if strings.HasPrefix(key, "id:") {
				item.TaxID = strings.TrimPrefix(key, "id:")
			}
		}
		found = true
		item.Amount += group.Amount
	}
	return item, found
}
