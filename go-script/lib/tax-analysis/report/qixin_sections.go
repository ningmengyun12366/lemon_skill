package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// qixinSubjects 读取统一任务中已经由后端精确核验的启信宝主体。
//
// 参数 ctx 为报告构建上下文；返回企业自身及已核验主要上下游列表。
func qixinSubjects(ctx *reportContext) []map[string]any {
	module, ok := ctx.modules[qixinModuleCode]
	if !ok {
		return nil
	}
	values := arrayAt(module.Raw, qixinSubjectsField)
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if item, ok := value.(map[string]any); ok && boolText(item, qixinSubjectVerifiedField) {
			result = append(result, item)
		}
	}
	return result
}

// qixinOwnSubject 从核验主体中选择当前报告企业。
//
// 参数 ctx 为报告构建上下文；返回企业自身主体，未取得时返回 nil。
func qixinOwnSubject(ctx *reportContext) map[string]any {
	for _, subject := range qixinSubjects(ctx) {
		if mapText(subject, qixinSubjectRoleField) == qixinOwnEnterpriseRole {
			return subject
		}
	}
	return nil
}

// qixinSubjectData 安全读取主体聚合结果中的指定业务子查询。
//
// 参数 subject 为主体聚合对象，key 为标准子查询字段；返回原始子查询数据。
func qixinSubjectData(subject map[string]any, key string) any {
	if subject == nil {
		return nil
	}
	value, _ := mapValue(subject, key)
	return value
}

// qixinCounterpartySubject 按税号优先、完整名称兜底匹配后端已核验的上下游主体。
//
// 参数 ctx 为报告上下文，item 为发票汇总主体，role 为客户或供应商角色；返回匹配主体，未匹配时返回 nil。
func qixinCounterpartySubject(ctx *reportContext, item counterpartyItem, role string) map[string]any {
	expectedTaxID := qixinIdentityText(item.TaxID)
	expectedName := qixinIdentityText(item.Name)
	for _, subject := range qixinSubjects(ctx) {
		if mapText(subject, qixinSubjectRoleField) != role {
			continue
		}
		actualTaxID := qixinIdentityText(mapText(subject, "taxId"))
		actualName := qixinIdentityText(mapText(subject, qixinSubjectNameField))
		if expectedTaxID != "" && actualTaxID == expectedTaxID || expectedTaxID == "" && expectedName != "" && actualName == expectedName {
			return subject
		}
	}
	return nil
}

// qixinInvoiceNameLink only labels a former name when both the tax ID and the
// registry's explicit HistoryNames match. Different names alone prove nothing.
func qixinInvoiceNameLink(ctx *reportContext, subject map[string]any) string {
	if !boolText(subject, qixinSubjectVerifiedField) {
		return ""
	}
	id := invoicePartyTaxID(mapText(subject, "taxId"))
	if id == "" {
		return ""
	}
	detail := qixinSubjectData(subject, qixinDetailField)
	history, _, _ := findFirst(detail, "HistoryNames")
	value, _, _ := findFirst(history, "string")
	var names []string
	switch x := value.(type) {
	case string:
		names = append(names, x)
	case []any:
		for _, v := range x {
			if s, ok := v.(string); ok {
				names = append(names, s)
			}
		}
	}
	current := mapText(subject, qixinSubjectNameField)
	for _, group := range append(append([]invoiceGroup(nil), ctx.issued...), ctx.received...) {
		if invoicePartyTaxID(group.CounterpartyID) != id || invoicePartyName(group.Counterparty) == invoicePartyName(current) {
			continue
		}
		for _, name := range names {
			if invoicePartyName(name) == invoicePartyName(group.Counterparty) {
				return "发票名称" + group.Counterparty + "为工商曾用名，现名" + current + "（同一税号）"
			}
		}
	}
	return ""
}

// qixinIdentityText 规范化主体名称或税号，仅用于已核验主体的完整值比较。
//
// 参数 value 为原始名称或税号；返回去空白并转大写后的比较值。
func qixinIdentityText(value string) string {
	return strings.ToUpper(invoicePartyName(value))
}

// qixinCounterpartyCreditSummary distinguishes coverage from identity lookup
// results. Parameters identify one ranked invoice party. The return value never
// treats an unqueried party or failed query as an absence of public risk.
func qixinCounterpartyCreditSummary(ctx *reportContext, item counterpartyItem, role string) string {
	if invoicePartyName(item.Name) == "个人" {
		return "个人，未纳入工商核验"
	}
	module := ctx.modules[qixinModuleCode]
	for _, value := range arrayAt(module.Raw, qixinSubjectsField) {
		subject, ok := value.(map[string]any)
		if !ok || mapText(subject, qixinSubjectRoleField) != role {
			continue
		}
		taxID := invoicePartyTaxID(item.TaxID)
		matches := taxID != "" && taxID == invoicePartyTaxID(mapText(subject, "taxId")) ||
			taxID == "" && invoicePartyName(item.Name) != "" && invoicePartyName(item.Name) == invoicePartyName(mapText(subject, qixinSubjectNameField))
		if !matches {
			continue
		}
		if !boolText(subject, qixinSubjectVerifiedField) {
			return "已查询，未匹配企业身份"
		}
		text := qixinPublicCreditSummary(subject)
		if link := qixinInvoiceNameLink(ctx, subject); link != "" {
			text = link + "；" + text
		}
		if len(arrayAt(subject, qixinWarningsField)) > 0 {
			text += "；部分公开信息查询失败"
		}
		return text
	}
	if !module.Complete {
		return "本次未取得工商核验结果"
	}
	return "未纳入本次核验"
}

// qixinCurrentAbnormalRows 返回明确当前在列的经营异常记录。
//
// 参数 subject 为已核验主体；返回当前经营异常记录列表。
func qixinCurrentAbnormalRows(subject map[string]any) []map[string]any {
	detail := qixinSubjectData(subject, qixinDetailField)
	value, _, found := findFirst(detail, "AbnormalItems")
	if !found {
		return nil
	}
	rows := qixinItemMaps(value, "InReason", "InDate", "OutReason")
	current := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if qixinListingState(row) == "当前在列" {
			current = append(current, row)
		}
	}
	return current
}

// qixinCurrentSeriousIllegalRows 返回明确当前在列的严重违法失信记录。
//
// 参数 subject 为已核验主体；返回当前严重违法失信记录列表。
func qixinCurrentSeriousIllegalRows(subject map[string]any) []map[string]any {
	rows := qixinItemMaps(qixinSubjectData(subject, qixinSeriousIllegalField), "InReason", "InDate", "InDepartment")
	current := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if qixinListingState(row) == "当前在列" {
			current = append(current, row)
		}
	}
	return current
}

// qixinPublicCreditSummary 汇总报告主表可展示的工商状态和公开信用数量。
//
// 参数 subject 为已核验上下游主体；返回不包含技术字段名的中文摘要。
func qixinPublicCreditSummary(subject map[string]any) string {
	if subject == nil {
		return "未取得精确核验结果"
	}
	detail := qixinSubjectData(subject, qixinDetailField)
	parts := []string{textOr(firstText(detail, "NewStatus", "Status"), "登记状态未获取")}
	if len(qixinListingRows(subject, qixinDetailField)) > 0 {
		parts = append(parts, qixinListingSummary(subject, qixinDetailField, "经营异常"))
	}
	if count := len(qixinItemMaps(qixinSubjectData(subject, qixinExecutionField), "CaseNumber", "Court", "Amount")); count > 0 {
		parts = append(parts, fmt.Sprintf("被执行%d条", count))
	}
	if count := len(qixinItemMaps(qixinSubjectData(subject, qixinDishonestField), "CaseNumber", "ExecutionStatus", "FinalDuty")); count > 0 {
		parts = append(parts, fmt.Sprintf("失信被执行%d条", count))
	}
	if count := len(qixinItemMaps(qixinSubjectData(subject, qixinAdministrativePunishmentField), "Number", "Department", "Content")); count > 0 {
		parts = append(parts, fmt.Sprintf("行政处罚%d条", count))
	}
	if len(qixinListingRows(subject, qixinSeriousIllegalField)) > 0 {
		parts = append(parts, qixinListingSummary(subject, qixinSeriousIllegalField, "严重违法"))
	}
	return strings.Join(parts, "；")
}

// qixinVerifiedRelationSummary 只返回工商详情明确给出的直接股东或分支关系。
//
// 参数 ctx 为报告上下文，item 为发票对手方；返回已验证关系说明，不进行相似度推断。
func qixinVerifiedRelationSummary(ctx *reportContext, item counterpartyItem) string {
	return qixinDirectRelation(ctx, item).summary()
}

type qixinRelationState uint8

const (
	qixinRelationInsufficient qixinRelationState = iota
	qixinRelationNotFound
	qixinRelationShareholder
	qixinRelationBranch
)

func (state qixinRelationState) summary() string {
	switch state {
	case qixinRelationShareholder:
		return "企业直接股东"
	case qixinRelationBranch:
		return "企业分支机构"
	case qixinRelationNotFound:
		return "已核验直接股东及分支机构范围内未发现关系"
	default:
		return "关系核验覆盖不足"
	}
}

// qixinDirectRelation keeps actual links, a bounded negative search, and
// missing coverage separate from user-facing wording.
func qixinDirectRelation(ctx *reportContext, item counterpartyItem) qixinRelationState {
	name := qixinIdentityText(item.Name)
	if name == "" {
		return qixinRelationInsufficient
	}
	for _, partner := range qixinPartnerMaps(ctx) {
		if qixinIdentityText(mapText(partner, "Name")) == name {
			return qixinRelationShareholder
		}
	}
	for _, branch := range qixinBranchRows(ctx) {
		if len(branch) > 0 && qixinIdentityText(branch[0]) == name {
			return qixinRelationBranch
		}
	}
	detail := qixinSubjectData(qixinOwnSubject(ctx), qixinDetailField)
	_, _, partnersPresent := findFirst(detail, "Partners")
	_, _, branchesPresent := findFirst(detail, "Branches")
	if partnersPresent && branchesPresent {
		return qixinRelationNotFound
	}
	return qixinRelationInsufficient
}

// includeQixinOwn 判断章节是否已经取得企业自身工商主体。
//
// 参数 ctx 为报告构建上下文；返回是否生成章节及省略原因。
func includeQixinOwn(ctx *reportContext) (bool, OmissionReason) {
	if qixinOwnSubject(ctx) == nil {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

// includeQixinPartners 判断直接股东关系章节是否存在可展示节点。
//
// 参数 ctx 为报告构建上下文；返回是否生成章节及省略原因。
func includeQixinPartners(ctx *reportContext) (bool, OmissionReason) {
	if len(qixinPartnerRows(ctx)) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

// includeQixinOrganization 判断工商组织信息章节是否至少有一种可展示数据。
//
// 参数 ctx 为本次报告构建上下文；返回值依次表示是否生成章节及省略原因。
func includeQixinOrganization(ctx *reportContext) (bool, OmissionReason) {
	if len(qixinPartnerRows(ctx)) == 0 && len(qixinEmployeeRows(ctx)) == 0 && len(qixinBranchRows(ctx)) == 0 {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

// includeQixinChanges 判断企业变更章节是否可读取工商主体详情。
//
// 参数 ctx 为报告构建上下文；返回是否生成章节及省略原因。
func includeQixinChanges(ctx *reportContext) (bool, OmissionReason) {
	if qixinOwnSubject(ctx) == nil {
		return false, OmittedDataInsufficient
	}
	return true, ""
}

// buildQixinEnterpriseProfile 合并工商公开信息与税务档案形成企业信息表。
//
// 参数 ctx 为报告构建上下文；返回中文业务标签组成的企业信息内容块。
func buildQixinEnterpriseProfile(ctx *reportContext) []Block {
	subject := qixinOwnSubject(ctx)
	detail := qixinSubjectData(subject, qixinDetailField)
	taxProfile := ctx.modules["taxpayer_profile"].Raw
	rows := [][]string{
		{"企业名称", textOr(firstText(detail, "Name"), "未获取")},
		{"统一社会信用代码", textOr(firstText(detail, "CreditNo"), "未获取")},
		{"法定代表人", textOr(firstText(detail, "OperName"), "未获取")},
		{"成立日期", textOr(firstText(detail, "StartDate"), "未获取")},
		{"登记状态", textOr(firstText(detail, "NewStatus", "Status"), "未获取")},
		{"注册资本", textOr(firstText(detail, "RegistCapi"), "未获取")},
		{"企业类型", textOr(firstText(detail, "EconKind"), "未获取")},
		{"行业分类", textOr(firstText(detail, "InduL4", "InduL3", "InduL2", "InduL1", "Industry"), "未获取")},
		{"注册地址", textOr(firstText(detail, "Address"), "未获取")},
		{"经营范围", textOr(firstText(detail, "Scope"), "未获取")},
		{"主管税务机关", textOr(firstText(taxProfile, "zgswjgmc", "ZgswjMc", "competentTaxAuthorityName", "taxAuthorityName"), "未获取")},
		{"纳税人类型", normalizedTaxpayerTypeDisplay(ctx.taxpayer, taxProfile)},
		{"主要税费种", qixinTaxTypeSummary(taxProfile)},
		{"主体信息核对", qixinProfileIdentityCheck(detail, taxProfile)},
	}
	return []Block{{Kind: BlockTable, Title: "工商公开基本信息", Headers: []string{"项目", "企业信息"}, Rows: rows}}
}

// normalizedTaxpayerTypeDisplay returns the already resolved taxpayer type for
// report display. Parameters are the deterministic kind and the raw tax profile.
// Return value uses a descriptive profile field only when classification is not
// possible, so visible content cannot contradict section applicability.
func normalizedTaxpayerTypeDisplay(kind taxpayerKind, taxProfile any) string {
	if kind != taxpayerUnknown {
		return taxpayerKindDisplayName(kind)
	}
	return textOr(firstText(taxProfile, "taxpayerTypeName", "taxpayerType", "nsrlxmc", "zzsnsrlx", "Nsrzglx"), "未获取")
}

// taxpayerKindDisplayName returns the Chinese report label for the normalized
// taxpayer kind. Parameter kind is the deterministic taxpayer classification.
// Return value is suitable for visible report content and never exposes an
// internal enum value.
func taxpayerKindDisplayName(kind taxpayerKind) string {
	switch kind {
	case taxpayerGeneral:
		return "一般纳税人"
	case taxpayerSmall:
		return "小规模纳税人"
	default:
		return "未获取"
	}
}

// qixinTaxTypeSummary 汇总纳税人档案中实际返回的税费种名称。
//
// 参数 root 为纳税人档案模块数据；返回去重后的税费种名称或“未获取”。
func qixinTaxTypeSummary(root any) string {
	names := make([]string, 0)
	for _, row := range findAllMaps(root, func(object map[string]any) bool {
		return mapText(object, "zsxmMc", "taxName", "taxTypeName", "taxCategoryName") != ""
	}) {
		names = append(names, mapText(row, "zsxmMc", "taxName", "taxTypeName", "taxCategoryName"))
	}
	return textOr(strings.Join(uniqueSorted(names), "、"), "未获取")
}

// qixinProfileIdentityCheck 比较工商和税务档案的主体名称及统一社会信用代码。
//
// 参数 detail 为工商详情，taxProfile 为税务档案；返回一致、待核实或未完整核验说明。
func qixinProfileIdentityCheck(detail, taxProfile any) string {
	businessName := qixinIdentityText(firstText(detail, "Name"))
	businessTaxID := qixinIdentityText(firstText(detail, "CreditNo"))
	taxName := qixinIdentityText(firstText(taxProfile, "nsrmc", "enterpriseName", "companyName"))
	taxID := qixinIdentityText(firstText(taxProfile, "nsrsbh", "uscc", "shxydm"))
	if businessName == "" || businessTaxID == "" || taxName == "" || taxID == "" {
		return "工商与税务档案未完整核验"
	}
	if businessName == taxName && businessTaxID == taxID {
		return "工商与税务档案主体信息一致"
	}
	return "工商与税务档案主体信息存在差异，需核实"
}

// qixinPartnerMaps 读取工商详情中具有出资或持股证据的直接股东记录。
//
// 参数 ctx 为报告构建上下文；返回直接股东业务对象列表。
func qixinPartnerMaps(ctx *reportContext) []map[string]any {
	subject := qixinOwnSubject(ctx)
	if subject == nil {
		return nil
	}
	return findAllMaps(qixinSubjectData(subject, qixinDetailField), func(object map[string]any) bool {
		return mapText(object, "Name") != "" && (mapText(object, "StockPercent") != "" || mapValueExists(object, "ShouldCapiItems", "RealCapiItems"))
	})
}

// qixinPartnerRows 将直接股东记录转换为统一百分数和出资口径的表格行。
//
// 参数 ctx 为报告构建上下文；返回按持股比例降序排列的中文表格行。
func qixinPartnerRows(ctx *reportContext) [][]string {
	maps := qixinPartnerMaps(ctx)
	type row struct {
		share float64
		data  []string
	}
	rows := make([]row, 0, len(maps))
	for _, object := range maps {
		share, shareKnown := qixinOwnershipPercent(mapText(object, "StockPercent"))
		shareText := qixinOwnershipPercentText(share, shareKnown)
		rows = append(rows, row{share: share, data: []string{
			mapText(object, "Name"), textOr(mapText(object, "StockType", "IdentifyType"), "未获取"),
			qixinCapitalText(object, "ShouldCapi", "ShouldCapiItems", "TotalShouldCapi"),
			qixinCapitalText(object, "RealCapi", "RealCapiItems", "TotalRealCapi"),
			shareText, qixinCapitalDate(object, "ShouldCapiDate", "ShouldCapiItems"),
		}})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].share > rows[j].share })
	result := make([][]string, len(rows))
	for index := range rows {
		result[index] = rows[index].data
	}
	return result
}

// qixinCollectionMaps 读取工商详情中的指定集合并筛选业务记录。
//
// 参数 ctx 为报告上下文，collection 为 WSDL 集合字段，predicate 为业务记录判定函数；
// 返回值为集合内满足条件的对象列表，集合不存在时返回空列表。
func qixinCollectionMaps(ctx *reportContext, collection string, predicate func(map[string]any) bool) []map[string]any {
	detail := qixinSubjectData(qixinOwnSubject(ctx), qixinDetailField)
	value, _, found := findFirst(detail, collection)
	if !found {
		return nil
	}
	return findAllMaps(value, predicate)
}

// qixinEmployeeRows 将主要人员集合转换为报告表格行。
//
// 参数 ctx 为报告上下文；返回值为人员姓名和职务两列的去重表格行。
func qixinEmployeeRows(ctx *reportContext) [][]string {
	items := qixinCollectionMaps(ctx, qixinEmployeesCollectionField, func(object map[string]any) bool {
		return mapText(object, "Name") != "" && mapText(object, "JobTitle") != ""
	})
	rows := make([][]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		name, title := mapText(item, "Name"), mapText(item, "JobTitle")
		key := name + "\x00" + title
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, []string{name, title})
	}
	return rows
}

// qixinBranchRows 将分支机构集合转换为报告表格行。
//
// 参数 ctx 为报告上下文；返回值为分支机构名称和公开工商标识两列的去重表格行。
func qixinBranchRows(ctx *reportContext) [][]string {
	items := qixinCollectionMaps(ctx, qixinBranchesCollectionField, func(object map[string]any) bool {
		return mapText(object, "Name") != ""
	})
	rows := make([][]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		name, identifier := mapText(item, "Name"), textOr(mapText(item, "Id"), "未获取")
		key := name + "\x00" + identifier
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, []string{name, identifier})
	}
	return rows
}

// buildQixinOrganization 构建股东、主要人员和分支机构的组合章节。
//
// 参数 ctx 为报告上下文；返回值为仅包含实际取得数据的表格和必要口径说明。
func buildQixinOrganization(ctx *reportContext) []Block {
	partnerRows := qixinPartnerRows(ctx)
	return []Block{{Kind: BlockTable, Title: "股东及出资信息", Headers: []string{"股东", "类型", "认缴金额", "实缴金额", "持股比例", "认缴期限"}, Rows: partnerRows}}
}

// buildQixinRelations 构建仅包含已验证直接股东节点的关系图和证据表。
//
// 参数 ctx 为报告构建上下文；返回关系图及已验证关系表内容块。
func buildQixinRelations(ctx *reportContext) []Block {
	partners := qixinPartnerMaps(ctx)
	own := qixinOwnSubject(ctx)
	if own == nil || len(partners) == 0 {
		chart := requiredEmptyChart("qixin-equity-network", chartKindNetwork, "股权及关联关系图", "数据不足", "当前报告未取得可展示的直接股东关系。")
		return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}, {Kind: BlockTable, Headers: []string{"主体", "关系/线索", "识别结论"}}}
	}
	labels := []string{mapText(own, qixinSubjectNameField)}
	values := []float64{qixinFullOwnershipPercentage}
	rows := make([][]string, 0, len(partners))
	for _, partner := range partners {
		name := mapText(partner, "Name")
		share, ok := qixinOwnershipPercent(mapText(partner, "StockPercent"))
		shareText := qixinOwnershipPercentText(share, ok)
		labels = append(labels, name)
		if ok {
			values = append(values, share)
		} else {
			values = append(values, 0)
		}
		rows = append(rows, []string{name, "直接股东，持股" + shareText, "已取得工商登记信息"})
	}
	chart := &Chart{ID: "qixin-equity-network", Kind: chartKindNetwork, Title: "股权及关联关系图", Labels: labels, Series: []Series{{Name: "持股比例（%）", Values: values, Color: chartColorPrimary}}}
	return []Block{{Kind: BlockChart, Title: chart.Title, Chart: chart}, {Kind: BlockTable, Title: "已验证关系", Headers: []string{"主体", "关系/线索", "识别结论"}, Rows: rows}}
}

// buildQixinTaxViolations 构建重大税收违法及税务行政处罚事实表。
//
// 参数 ctx 为报告构建上下文；返回税务违法与处罚内容块。
func buildQixinTaxViolations(ctx *reportContext) []Block {
	subject := qixinOwnSubject(ctx)
	rows := [][]string{}
	for _, object := range qixinItemMaps(qixinSubjectData(subject, qixinTaxViolationField), "IllegalFact", "Result", "CheckDepartment") {
		rows = append(rows, []string{textOr(mapText(object, "Time", "CreatedTime"), "未获取"), textOr(mapText(object, "Type", "Property"), "重大税收违法"), textOr(mapText(object, "IllegalFact"), "未获取"), "—", "—", "—", textOr(mapText(object, "Result"), "未获取"), textOr(mapText(object, "CheckDepartment"), "未获取")})
	}
	for _, object := range qixinItemMaps(qixinSubjectData(subject, qixinAdministrativePunishmentField), "Number", "Department", "Content") {
		if !containsAny(mapText(object, "Department"), "税", "税务") {
			continue
		}
		rows = append(rows, []string{textOr(mapText(object, "Date", "PublishDate"), "未获取"), textOr(mapText(object, "Number"), "税务行政处罚"), textOr(mapText(object, "Reason", "Content", "Description"), "未获取"), textOr(mapText(object, "BasedOn"), "未获取"), qixinPenaltyAmountText(object, "PunishAmnt"), qixinPenaltyAmountText(object, "IllegalIncomeAmnt"), qixinReadableStatus(object, "Status"), textOr(mapText(object, "Department"), "未获取")})
	}
	result := "未查询到相关记录"
	conclusion := "未见明显异常"
	if len(rows) > 0 {
		details := make([]string, 0, len(rows))
		for _, row := range rows {
			details = append(details, strings.Join([]string{row[0], row[1], row[4], row[5], row[7]}, "；"))
		}
		result = strings.Join(details, "；")
		conclusion = "需关注"
	}
	return []Block{{Kind: BlockTable, Title: "税务处罚及违法公开信息", Headers: []string{"检测事项", "检测结果", "结论"}, Rows: [][]string{{"税务处罚及税收违法信息", result, conclusion}}}}
}

// buildQixinCompliance 构建经营异常、严重违法和非税务行政处罚事实表。
//
// 参数 ctx 为报告构建上下文；返回经营信用内容块。
func buildQixinCompliance(ctx *reportContext) []Block {
	subject := qixinOwnSubject(ctx)
	rows := [][]string{}
	for _, object := range qixinItemMaps(qixinSubjectData(subject, qixinDetailField), "InReason", "InDate", "OutReason") {
		rows = append(rows, []string{"经营异常", textOr(mapText(object, "InDate"), "未获取"), "—", textOr(mapText(object, "InReason"), "未获取"), "—", "—", "—", textOr(mapText(object, "OutDate"), "未移出/未获取"), textOr(joinNonEmpty(mapText(object, "Department"), mapText(object, "OutReason")), "—")})
	}
	for _, object := range qixinItemMaps(qixinSubjectData(subject, qixinSeriousIllegalField), "InReason", "InDate", "InDepartment") {
		rows = append(rows, []string{"严重违法失信", textOr(mapText(object, "InDate"), "未获取"), "—", textOr(mapText(object, "InReason"), "未获取"), "—", "—", "—", textOr(mapText(object, "OutDate"), "未移出/未获取"), textOr(joinNonEmpty(mapText(object, "InDepartment", "OutDepartment"), mapText(object, "OutReason")), "—")})
	}
	for _, object := range qixinItemMaps(qixinSubjectData(subject, qixinAdministrativePunishmentField), "Number", "Department", "Content") {
		if containsAny(mapText(object, "Department"), "税", "税务") {
			continue
		}
		rows = append(rows, []string{"行政处罚", textOr(mapText(object, "Date", "PublishDate"), "未获取"), textOr(mapText(object, "Number"), "未获取"), textOr(mapText(object, "Reason", "Content", "Description"), "未获取"), textOr(mapText(object, "BasedOn"), "未获取"), qixinPenaltyAmountText(object, "PunishAmnt"), qixinPenaltyAmountText(object, "IllegalIncomeAmnt"), qixinReadableStatus(object, "Status"), textOr(mapText(object, "Department"), "未获取")})
	}
	result := "未查询到相关记录"
	conclusion := "未见明显异常"
	if len(rows) > 0 {
		details := make([]string, 0, len(rows))
		for _, row := range rows {
			details = append(details, strings.Join([]string{row[0], row[1], row[2], row[3], row[4], row[5], row[8]}, "；"))
		}
		result = strings.Join(details, "；")
		conclusion = "需关注"
	}
	return []Block{{Kind: BlockTable, Title: "行政处罚及经营信用", Headers: []string{"检测事项", "检测结果", "结论"}, Rows: [][]string{{"行政处罚及经营异常", result, conclusion}}}}
}

// qixinChangeRecords 提取企业自身的全部工商变更，第二个返回值表示已返回变更集合。
// 报告期间不限制工商历史；保留来源顺序，不合并同日或重复记录。
func qixinChangeRecords(ctx *reportContext) ([]map[string]any, bool) {
	value, _, found := findFirst(qixinSubjectData(qixinOwnSubject(ctx), qixinDetailField), qixinChangesCollectionField)
	if !found {
		return nil, false
	}
	return qixinItemMaps(value, "ChangeItem", "ChangeDate", "BeforeContent", "AfterContent"), true
}

// buildQixinChanges 构建全部工商变更明细表，并给出客观涉税关注方向。
//
// 参数 ctx 为报告构建上下文；返回完整表格，无记录时返回查询结果说明。
func buildQixinChanges(ctx *reportContext) []Block {
	rows := [][]string{}
	records, _ := qixinChangeRecords(ctx)
	for _, object := range records {
		rawDate := mapText(object, "ChangeDate")
		date := textOr(normalizeReportDate(rawDate, false), textOr(rawDate, "未获取"))
		item := textOr(mapText(object, "ChangeItem"), "未获取")
		rows = append(rows, []string{date, item, textOr(mapText(object, "BeforeContent"), "未获取"), textOr(mapText(object, "AfterContent"), "未获取"), qixinChangeRecordAttention(object)})
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"未查询到企业变更记录。"}}}
	}
	return []Block{{Kind: BlockTable, Headers: []string{"变更日期", "事项", "变更前", "变更后", "关注"}, Rows: rows}}
}

// buildQixinJudicial 构建企业自身及主要上下游的执行公开信息表。
//
// 参数 ctx 为报告构建上下文；返回被执行和失信被执行内容块。
func buildQixinJudicial(ctx *reportContext) []Block {
	ownCount, upstreamCount := 0, 0
	for _, subject := range qixinSubjects(ctx) {
		role := mapText(subject, qixinSubjectRoleField)
		for range qixinItemMaps(qixinSubjectData(subject, qixinExecutionField), "CaseNumber", "Court", "Amount") {
			if role == qixinOwnEnterpriseRole {
				ownCount++
			} else {
				upstreamCount++
			}
		}
		for range qixinItemMaps(qixinSubjectData(subject, qixinDishonestField), "CaseNumber", "Court", "ExecutionStatus") {
			if role == qixinOwnEnterpriseRole {
				ownCount++
			} else {
				upstreamCount++
			}
		}
	}
	attention := "未见明显异常"
	if ownCount+upstreamCount > 0 {
		attention = "需关注"
	}
	return []Block{{Kind: BlockTable, Title: "企业及重要上下游执行信息", Headers: []string{"事项", "企业自身", "主要上下游", "关注"}, Rows: [][]string{{"被执行及失信被执行", fmt.Sprintf("%d条", ownCount), fmt.Sprintf("%d条", upstreamCount), attention}}}}
}

// qixinReadableStatus returns a human-readable public-information status and
// suppresses undocumented numeric or machine codes. Parameters: object is one
// QiXin record and aliases are candidate status fields. Return value: the first
// readable status, or “未获取” when the interface provides only opaque codes.
func qixinReadableStatus(object map[string]any, aliases ...string) string {
	for _, alias := range aliases {
		value := strings.TrimSpace(mapText(object, alias))
		if value == "" || isOpaqueStatusCode(value) {
			continue
		}
		return value
	}
	return "未获取"
}

// isOpaqueStatusCode identifies values that contain no Chinese explanation.
// Parameters: value is the interface status text. Return value: true for pure
// numeric, punctuation, or Latin machine-code values.
func isOpaqueStatusCode(value string) bool {
	for _, char := range value {
		if char >= '\u4e00' && char <= '\u9fff' {
			return false
		}
	}
	return true
}

// buildQixinIndustry 构建工商行业分类层级表并说明发票纠偏口径。
//
// 参数 ctx 为报告构建上下文；返回行业分类内容块。
func buildQixinIndustry(ctx *reportContext) []Block {
	detail := qixinSubjectData(qixinOwnSubject(ctx), qixinDetailField)
	rows := [][]string{}
	for index, aliases := range [][]string{{"InduL1", "InduCodeL1"}, {"InduL2", "InduCodeL2"}, {"InduL3", "InduCodeL3"}, {"InduL4", "InduCodeL4"}} {
		name := firstText(detail, aliases[0])
		code := firstText(detail, aliases[1])
		if name != "" || code != "" {
			rows = append(rows, []string{fmt.Sprintf("第%d级", index+1), textOr(code, "未获取"), textOr(name, "未获取")})
		}
	}
	if len(rows) == 0 {
		return []Block{{Kind: BlockParagraph, Paragraphs: []string{"当前启信宝工商详情未返回行业分类编码。主要业务仍以销项发票品类为准。"}}}
	}
	return []Block{{Kind: BlockTable, Title: "工商行业分类", Headers: []string{"层级", "行业代码", "行业名称"}, Rows: rows}, {Kind: BlockParagraph, Paragraphs: []string{"工商行业分类作为经营特征参考，主要业务结构以已取得的销项发票品类数据展示。"}}}
}

// qixinItemMaps 从WCF集合包装中筛选至少包含一个目标字段的业务记录。
//
// 参数 root 为任意子查询数据，aliases 为判定记录的字段别名；返回业务对象列表。
func qixinItemMaps(root any, aliases ...string) []map[string]any {
	return findAllMaps(root, func(object map[string]any) bool {
		for _, alias := range aliases {
			if mapText(object, alias) != "" {
				return true
			}
		}
		return false
	})
}

// mapValueExists 判断对象是否包含任一候选字段且值非空。
//
// 参数 object 为业务对象，aliases 为候选字段；返回是否存在可用值。
func mapValueExists(object map[string]any, aliases ...string) bool {
	_, ok := mapValue(object, aliases...)
	return ok
}

// qixinCapitalText 按汇总字段或出资明细计算认缴、实缴展示金额。
//
// 参数 object 为股东记录，amountKey、collectionKey、totalKey 为金额来源字段；返回金额文本。
func qixinCapitalText(object map[string]any, amountKey, collectionKey, totalKey string) string {
	if value := strings.TrimSpace(mapText(object, totalKey)); value != "" {
		return value
	}
	total, known := 0.0, false
	value, _ := mapValue(object, collectionKey)
	for _, row := range findAllMaps(value, func(item map[string]any) bool { _, ok := mapNumber(item, amountKey); return ok }) {
		if amount, ok := mapNumber(row, amountKey); ok {
			total += amount
			known = true
		}
	}
	return optionalMoney(total, known)
}

// qixinCapitalDate 从股东或出资明细中读取最晚约定出资日期。
//
// 参数 object 为股东记录，dateKey 为日期字段，collectionKey 为出资集合字段；返回日期文本。
func qixinCapitalDate(object map[string]any, dateKey, collectionKey string) string {
	value, _ := mapValue(object, collectionKey)
	dates := []string{}
	for _, row := range findAllMaps(value, func(item map[string]any) bool { return mapText(item, dateKey) != "" }) {
		dates = append(dates, mapText(row, dateKey))
	}
	return textOr(strings.Join(uniqueSorted(dates), "、"), "未获取")
}

// qixinAmountText 将 WCF 可空金额字段转换为报告金额文本。
//
// 参数 object 为处罚或执行记录，aliases 为金额字段候选名；返回格式化金额，缺失时返回破折号。
func qixinAmountText(object map[string]any, aliases ...string) string {
	if value, ok := mapNumber(object, aliases...); ok {
		return optionalMoney(value, true)
	}
	return textOr(mapText(object, aliases...), "—")
}

// qixinPenaltyAmountText 将官方以万元返回的行政处罚金额转换为报告统一的元。
//
// 参数 object 为行政处罚记录，aliases 为处罚金额字段候选名；返回元口径金额，缺失时返回破折号。
func qixinPenaltyAmountText(object map[string]any, aliases ...string) string {
	if value, ok := mapNumber(object, aliases...); ok {
		return optionalMoney(value*qixinTenThousandYuanScale, true)
	}
	return "—"
}

// qixinOwnershipPercent 兼容官方0到1的小数比例及历史样例中的百分数文本。
//
// 参数 value 为 StockPercent 原始值；返回0到100的百分数及字段是否可用。
func qixinOwnershipPercent(value string) (float64, bool) {
	text := strings.TrimSpace(strings.TrimSuffix(value, "%"))
	number, ok := numberValue(text)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	if !strings.HasSuffix(strings.TrimSpace(value), "%") && math.Abs(number) <= qixinFractionalOwnershipMaximum {
		number *= qixinFullOwnershipPercentage
	}
	return number, true
}

// qixinOwnershipPercentText 格式化0到100口径的工商持股比例。
//
// 参数 value 为百分数，known 表示字段是否可用；返回带百分号的展示文本或“未获取”。
func qixinOwnershipPercentText(value float64, known bool) string {
	if !known {
		return "未获取"
	}
	return fmt.Sprintf("%.2f%%", value)
}

// qixinChangeAttention 将工商变更事项映射为客观资料核实方向。
//
// 参数 item 为工商变更事项名称；返回中文关注方向。
func qixinChangeAttention(item string) string {
	switch {
	case containsAny(item, "注册资本", "股东", "出资"):
		return "核对工商与税务登记资料"
	case containsAny(item, "地址", "住所"):
		return "核对登记地址信息"
	case containsAny(item, "法定代表人", "负责人"):
		return "核对办税授权信息"
	case containsAny(item, "经营范围"):
		return "核对实际业务与登记事项"
	default:
		return "留存变更资料"
	}
}

// qixinChangeRecordAttention flags explicit category/content conflicts while
// preserving every source row and avoiding inferred replacement content.
func qixinChangeRecordAttention(row map[string]any) string {
	item := mapText(row, "ChangeItem")
	if containsAny(item, "投资人", "股东") {
		content := mapText(row, "BeforeContent") + "；" + mapText(row, "AfterContent")
		if containsAny(content, "经营范围", "一般项目：", "许可项目：", "一般经营项目", "许可经营项目") {
			return "事项标注与经营范围文字不一致，原登记记录待核实"
		}
		return "核对事项与原登记记录，确认是否涉及股权变更"
	}
	return qixinChangeAttention(item)
}
