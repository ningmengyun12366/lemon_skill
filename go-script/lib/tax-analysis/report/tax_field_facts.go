package report

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// TaxFieldFact retains the authority's Chinese row/column and original value.
// Coordinates are zero-based in the expanded HTML table. Facts are technical
// evidence in report-model.json; renderers only consume the business tables.
type TaxFieldFact struct {
	ID           string    `json:"id"`
	Module       string    `json:"module"`
	USCC         string    `json:"uscc,omitempty"`
	ProjectCodes []string  `json:"projectCodes,omitempty"`
	ItemCode     string    `json:"itemCode,omitempty"`
	RecordID     string    `json:"recordId"`
	Version      string    `json:"version,omitempty"`
	FiledAt      string    `json:"filedAt,omitempty"`
	Period       TaxPeriod `json:"period"`
	FormCode     string    `json:"formCode"`
	FormName     string    `json:"formName"`
	RowLabel     string    `json:"rowLabel"`
	Line         string    `json:"line,omitempty"`
	ColumnLabel  string    `json:"columnLabel"`
	Group        string    `json:"columnGroup,omitempty"`
	RowGroup     string    `json:"rowGroup,omitempty"`
	Raw          string    `json:"raw"`
	Value        float64   `json:"value"`
	Status       string    `json:"status"`
	Unit         string    `json:"unit"`
	Path         string    `json:"path"`
	Table        int       `json:"table"`
	Row          int       `json:"row"`
	Column       int       `json:"column"`
}

var taxNumericColumnLabels = []string{
	"一般项目本期数", "即征即退项目本期数", "即证即退项目本期数", "本年累计数", "本期已缴（费）额", "年初数", "期末数",
	"本月数", "本期数", "本年累计", "本年累计金额", "本年金额", "上年金额", "本期金额", "本月金额",
	"账载金额", "税收金额", "调增金额", "调减金额", "金额", "税额", "销售额", "销项(应纳)税额", "价税合计",
	"计税依据", "计税（费）依据", "税率", "税（费）率", "应纳税额", "减免税额", "已缴税额", "应补（退）税额",
	"应税项", "减除项", "应税所得率",
	"税（费）率或单位税额", "小微企业“六税两费”减征比例", "小微企业“六税两费”减征额",
	"增值税税额", "增值税限额减免金额", "增值税免抵税额", "留抵退税本期扣除额", "减免税（费）额", "本期抵免金额",
	"本期应纳税（费）额", "本期减免税（费）额", "本期已缴税（费）额", "本期应补（退）税（费）额",
	"减征比例", "减征额", "房产原值", "其中：出租房产原值", "其中：出租房产面积", "申报租金收入", "出租面积",
	"占用土地面积", "税额标准", "土地等级", "期末余额", "年初余额", "期初余额",
	"本年享受优惠的资产原值", "账载折旧\\摊销金额", "纳税调减金额", "享受加速政策优惠金额",
	"年初余额", "期初余额", "上年年末余额", "本期发生额", "本期实际抵减税额", "本期实际扣除金额", "期末余额", "免税额",
	"免征增值税项目销售额", "扣除后免税销售额", "免税销售额对应的进项税额",
	"本期应扣除金额", "本期应抵减税额", "本期可抵减额", "本期调减额", "本期实际抵减额",
	"服务、不动产和无形资产扣除项目本期实际扣除金额", "本期服务、不动产和无形资产价税合计额（免税销售额）", "含税(免税)销售额",
	"减税性质代码及名称", "免税性质代码及名称", "减免性质代码",
	"本年折旧、摊销额", "税收折旧、摊销额",
}

// taxFieldFacts extracts only declared detail/previews and preserves the form
// boundary. Duplicate delivery of the same declaration in multiple modules is
// retained by module in evidence, while business aggregation deduplicates IDs.
func taxFieldFacts(modules map[string]ModuleSnapshot) []TaxFieldFact {
	var result []TaxFieldFact
	codes := make([]string, 0, len(modules))
	for code := range modules {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		for _, collection := range []string{"effectiveRecords", "records", "referenceAnnualRecords"} {
			records, present := namedCollectionMapsWithPresence(modules[code].Raw, collection)
			if !present {
				continue
			}
			for index, record := range records {
				result = append(result, recordTaxFieldFacts(code, fmt.Sprintf("%s.%s[%d]", code, collection, index), record)...)
			}
		}
		if root, ok := modules[code].Raw.(map[string]any); ok {
			if history, ok := root["historicalCapitalBooks"].(map[string]any); ok {
				records, _ := namedCollectionMapsWithPresence(history, "records")
				for index, record := range records {
					result = append(result, recordTaxFieldFacts(code, fmt.Sprintf("%s.historicalCapitalBooks.records[%d]", code, index), record)...)
				}
			}
		}
	}
	return result
}

func recordTaxFieldFacts(module, path string, record map[string]any) []TaxFieldFact {
	period, ok := taxPeriodFromRecord(record, false)
	if !ok {
		return nil
	}
	var result []TaxFieldFact
	for _, collection := range []string{"details", "previews"} {
		for index, item := range arrayAt(record, collection) {
			detail, ok := item.(map[string]any)
			if !ok {
				continue
			}
			base := TaxFieldFact{Module: module, USCC: mapText(record, "uscc"), ProjectCodes: taxProjectCodes(record), ItemCode: mapText(record, "zspmDm"), RecordID: mapText(record, "sbuuid", "zlbscjuuid"), Version: mapText(record, "pzxh"), FiledAt: mapText(record, "nssbrq", "lrrq"), Period: period, FormCode: mapText(detail, "code"), FormName: mapText(detail, "name", "label"), Path: fmt.Sprintf("%s.%s[%d].data", path, collection, index), Status: "missing", Unit: "元"}
			if strings.EqualFold(mapText(detail, "status"), "FAILED") {
				base.Status = "failed"
				result = append(result, base)
				continue
			}
			value, _ := mapValue(detail, "data")
			if !isTaxTable(value) {
				result = append(result, structuredTaxFieldFacts(base, value, base.Path)...)
				continue
			}
			rows, err := decodeTaxTableRows(value)
			if err != nil {
				continue
			}
			for _, row := range rows {
				label, line := formBusinessRowLabel(rows, row)
				rowGroup := ""
				if base.FormCode == vatAnnexOneFormCode || base.FormCode == vatNewAnnexOneFormCode {
					for column := range row.Grid {
						if header, _ := formNumericHeader(rows, row, column); header != "" {
							rowGroup = strings.Join(row.Grid[:column], " · ")
							break
						}
					}
					if base.FormCode == vatNewAnnexOneFormCode {
						for column, cell := range row.Grid {
							if vatNewAnnexLinePattern.MatchString(canonicalFormLabel(cell)) && column > 0 {
								line, label = canonicalFormLabel(cell), row.Grid[column-1]
								break
							}
						}
					}
				}
				if label == "" {
					continue
				}
				for column, raw := range row.Grid {
					header, group := formNumericHeader(rows, row, column)
					if header == "" {
						continue
					}
					fact := base
					fact.RowLabel = label
					if module == "balance_sheets" {
						fact.RowLabel = financialCellRowLabel(rows, row, column)
						if fact.RowLabel == "" {
							continue
						}
					}
					fact.Line = line
					fact.ColumnLabel = header
					fact.Group = group
					fact.RowGroup = rowGroup
					fact.Table = row.Table
					fact.Row = row.Index
					fact.Column = column
					fact.Raw = raw
					fact.Value, ok = parseFormNumber(raw)
					if containsAny(header, "性质代码") && raw != "" {
						fact.Value = 0
						fact.Status = "present_text"
						fact.Unit = "文本"
					} else if ok {
						fact.Status = "present"
					} else if formCellNotApplicable(raw) {
						fact.Status = "not_applicable"
					} else if strings.TrimSpace(raw) != "" {
						if declarationLabelMatches(header, []string{"土地等级", "减税性质代码及名称", "免税性质代码及名称", "减免性质代码"}) {
							fact.Status = "present_text"
							fact.Unit = "文本"
						} else {
							continue
						}
					}
					ratioLabel := canonicalFormLabel(header)
					rowIsRatio := declarationLabelMatches(fact.RowLabel, []string{"税率", "征收率", "税（费）率", "分支机构本期分摊比例", "优惠幅度"})
					if containsAny(ratioLabel, "率", "比例") || rowIsRatio || (ok && strings.HasSuffix(canonicalFormLabel(raw), "%")) {
						fact.Unit = "比例"
						alreadyFraction := financialStringSliceContains([]string{"BDA0611153", "BDA0611154"}, base.FormCode) && declarationLabelMatches(header, []string{"税（费）率"})
						if !alreadyFraction && !strings.Contains(raw, "%") && strings.Contains(ratioLabel, "%") {
							fact.Value /= 100
						}
					}
					if containsAny(canonicalFormLabel(header), "面积") {
						fact.Unit = "平方米"
					}
					if isResearchProjectCount(fact) {
						fact.Unit = "项"
					}
					if containsAny(header, "税额标准") {
						fact.Unit = "元/平方米"
					}
					if len(base.ProjectCodes) == 1 && base.ProjectCodes[0] == "10112" && declarationLabelMatches(header, []string{"税率"}) {
						fact.Unit = "元/平方米"
					}
					if len(base.ProjectCodes) == 1 && base.ProjectCodes[0] == "10112" && declarationLabelMatches(header, []string{"计税依据"}) {
						fact.Unit = "原表计税依据口径"
					}
					fact.ID = fmt.Sprintf("tax-field-%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%d|%d", base.RecordID, base.Path, row.Table, row.Index, column))))
					result = append(result, fact)
				}
			}
		}
	}
	return result
}

// financialCellRowLabel keeps the liability/equity side of a paired balance
// sheet attached to its own Chinese label, not the asset label on the left.
func financialCellRowLabel(rows []formTableRow, row formTableRow, column int) string {
	amountColumns := formColumnIndexes(rows, row, []string{"期末余额", "年初余额", "期初余额"})
	isAmountColumn := func(index int) bool {
		for _, candidate := range amountColumns {
			if candidate == index {
				return true
			}
		}
		return false
	}
	// Skip the current amount block, then search only its own label/line cells.
	// An earlier amount block is a hard boundary even when this side is blank.
	index := column - 1
	for index >= 0 && isAmountColumn(index) {
		index--
	}
	for ; index >= 0 && !isAmountColumn(index); index-- {
		text := row.Grid[index]
		if _, numeric := parseFormNumber(text); numeric {
			continue
		}
		if strings.IndexFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
			return text
		}
	}
	return ""
}

func formBusinessRowLabel(rows []formTableRow, row formTableRow) (string, string) {
	if formNonBusinessRow(row) {
		return "", ""
	}
	for _, cell := range row.Cells {
		if containsAny(cell, "经办人", "代理机构", "身份证件", "办理人员", "是否自行申报", "联系电话", "受理税务机关") {
			return "", ""
		}
	}
	for _, cell := range row.Cells {
		if declarationLabelMatches(cell, []string{"合计", "总计"}) {
			line := ""
			if len(row.Cells) > 0 && isDeclarationLineNumber(canonicalFormLabel(row.Cells[0])) {
				line = canonicalFormLabel(row.Cells[0])
			}
			return "合计", line
		}
	}
	// Horizontal tax tables identify the business item via its own Chinese header.
	for _, header := range []string{"税目", "征收品目", "税（费）种"} {
		columns := formColumnIndexes(rows, row, []string{header})
		if len(columns) >= 1 && columns[0] >= 0 && columns[0] < len(row.Grid) {
			label := row.Grid[columns[0]]
			if label != "" && !declarationLabelMatches(label, []string{header}) {
				return label, row.Cells[0]
			}
		}
	}
	for _, cell := range row.Cells {
		if strings.Contains(cell, "%") && containsAny(cell, "税率", "征收率") {
			return cell, ""
		}
	}
	line := ""
	for _, cell := range row.Cells {
		text := canonicalFormLabel(cell)
		if text == "" {
			continue
		}
		if _, ok := parseFormNumber(text); ok {
			if line == "" {
				line = text
			}
			continue
		}
		if isDeclarationLineNumber(text) || strings.ContainsAny(text, "=＝") || containsAny(text, "——") {
			continue
		}
		// A vertical grouping label precedes the actual business label in VAT.
		if financialStringSliceContains([]string{"销售额", "税款计算", "税款缴纳", "附加税费", "利润总额计算", "应纳税所得额计算", "应纳税额计算", "实际应补（退）税额计算", "分支机构", "一、计税依据", "二、税款计算", "三、附加税费"}, text) {
			continue
		}
		if declarationLabelMatches(text, []string{"行次", "项目", "栏次", "序号", "税种"}) {
			return "", ""
		}
		return cell, line
	}
	return "", ""
}

// formNonBusinessRow excludes column numbering/formulas before amount extraction.
// Chinese business labels such as “研发费用（1+2）” remain valid business rows.
func formNonBusinessRow(row formTableRow) bool {
	hasChinese := false
	for _, cell := range row.Cells {
		for _, r := range cell {
			if unicode.Is(unicode.Han, r) && r != '且' && r != '或' {
				hasChinese = true
				break
			}
		}
	}
	return !hasChinese || formIsSectionTitle(row)
}

func formNumericHeader(rows []formTableRow, target formTableRow, column int) (string, string) {
	header, group := "", ""
	if formIsSectionTitle(target) {
		return "", ""
	}
	column -= formTaxRowOffset(rows, target)
	if column < 0 {
		return "", ""
	}
	// Do not carry a horizontal tax header into later free-form policy boxes.
	if len(formRawColumnIndexes(rows, target, []string{"税（费）种"})) > 0 && !formHasTaxBusinessName(target) {
		return "", ""
	}
	for _, row := range rows {
		if row.Table != target.Table || row.Index >= target.Index {
			continue
		}
		if formIsSectionTitle(row) {
			header, group = "", ""
			continue
		}
		if column >= len(row.Grid) {
			continue
		}
		if !formIsHeaderRow(row) {
			continue
		}
		cell := row.Grid[column]
		if declarationLabelMatches(cell, taxNumericColumnLabels) {
			header = cell
		}
		if financialStringSliceContains([]string{"一般项目", "即征即退项目", "货物及劳务", "服务、不动产和无形资产", "开具增值税专用发票", "开具其他发票", "未开具发票", "纳税检查调整", "合计"}, canonicalFormLabel(cell)) {
			group = cell
		}
	}
	return header, group
}

// A merged authority HTML table may contain several titled forms. A title is
// not a business value and ends the preceding form's numeric header scope.
func formIsSectionTitle(row formTableRow) bool {
	title := ""
	for _, cell := range row.Cells {
		text := canonicalFormLabel(cell)
		if text == "" {
			continue
		}
		if title != "" && title != text {
			return false
		}
		title = text
	}
	return containsAny(title, "明细表", "申报表")
}

func formHasTaxBusinessName(row formTableRow) bool {
	for _, cell := range row.Cells {
		if declarationLabelMatches(cell, []string{"合计"}) {
			return true
		}
		name := canonicalFormLabel(cell)
		if taxBusinessNames[name] {
			return true
		}
		if index := strings.LastIndex(name, "/"); index >= 0 && taxBusinessNames[name[index+1:]] {
			return true
		}
	}
	return false
}

func formIsHeaderRow(row formTableRow) bool {
	for _, cell := range row.Cells {
		if _, numeric := parseFormNumber(cell); numeric {
			return false
		}
	}
	return true
}
