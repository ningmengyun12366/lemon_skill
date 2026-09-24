package fetchrisk

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

var invoiceColumnWidths = map[string]float64{
	"序号":        6,
	"发票代码":      14,
	"发票号码":      12,
	"数电发票号码":    22,
	"销方识别号":     20,
	"销方名称":      30,
	"购方识别号":     20,
	"购买方名称":     30,
	"开票日期":      20,
	"勾选认证日期":    20,
	"发票金额":      12,
	"发票税额":      12,
	"发票价税合计":    14,
	"发票来源":      18,
	"发票票种":      20,
	"发票状态":      10,
	"是否正数发票":    12,
	"发票风险等级":    12,
	"开票人":       10,
	"备注":        50,
	"税收分类编码":    22,
	"特定业务类型":    22,
	"货物或应税劳务名称": 50,
	"规格型号":      20,
	"单位":        8,
	"数量":        12,
	"单价":        14,
	"明细金额":      14,
	"税率":        8,
	"明细税额":      14,
	"本地发票附件":    70,
}

var invoiceWrapTextColumns = map[string]bool{
	"备注":        true,
	"货物或应税劳务名称": true,
	"销方名称":      true,
	"购买方名称":     true,
	"特定业务类型":    true,
	"本地发票附件":    true,
}

func Summarize(taskFile string) error {
	state, err := LoadTaskState(taskFile)
	if err != nil {
		return err
	}
	artifacts := common.ObjectValue(state.Data, "artifacts")
	outputs := common.ObjectValue(state.Data, "outputs")
	storage, _ := state.Data["storage"].(map[string]any)
	files := stringSlice(artifacts["raw_invoice_local_files"])
	if len(files) == 0 {
		files = listXLSX(common.StringValue(firstNonEmpty(outputs["tax_invoice_excel_dir"], storage["raw_invoice_dir"], artifacts["raw_invoice_dir"])))
	}
	attachmentDir := common.StringValue(firstNonEmpty(outputs["invoice_attachments"], storage["attachments_dir"]))
	summary, err := summarizeFiles(files, listPDFFilenames(attachmentDir))
	if err != nil {
		return err
	}
	root := common.StringValue(firstNonEmpty(storage["root_dir"], state.Data["workspace_dir"]))
	mdPath := filepath.Join(root, "发票数据摘要.md")
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(mdPath, []byte(summary), 0o644); err != nil {
		return err
	}
	outputs["summary_markdown_path"] = mdPath
	markPlannedStepStatus(state.Data, summarizeStep, "success", "")
	if err := state.Save(); err != nil {
		return err
	}
	common.AppendMessageLog("生成发票摘要完成，原样读取markdown内容展示给用户；" + "本地存放路径: " + mdPath)
	fmt.Print(summary)
	return nil
}

func summarizeFiles(files []string, attachmentFileNames ...[]string) (string, error) {
	type agg struct {
		Count, Attachments int
		Amount, Tax, Total float64
	}
	attachments := []string{}
	if len(attachmentFileNames) > 0 {
		attachments = attachmentFileNames[0]
	}
	hasAttachmentColumn := len(attachments) > 0
	byDirection := map[string]map[string]*agg{}
	for _, file := range files {
		rows, err := readSheetRows(file, []string{"发票基础信息", "信息汇总表"})
		if err != nil {
			continue
		}
		if len(rows) < 2 {
			continue
		}
		headers := rows[0]
		direction := inferDirectionKey(filepath.Base(file))
		invoiceNoIdx := findHeader(headers, []string{"数电发票号码", "数电票号码"})
		if _, ok := byDirection[direction]; !ok {
			byDirection[direction] = map[string]*agg{}
		}
		for _, row := range rows[1:] {
			if isTotalRow(row) {
				continue
			}
			status := classifyInvoiceStatus(cellByHeader(headers, row, []string{"发票状态", "状态"}), cellByHeader(headers, row, []string{"是否正数发票"}), parseFloat(cellByHeader(headers, row, []string{"金额", "发票金额", "不含税金额", "不含税金额（元）"})))
			if byDirection[direction][status] == nil {
				byDirection[direction][status] = &agg{}
			}
			a := byDirection[direction][status]
			a.Count++
			a.Amount += parseFloat(cellByHeader(headers, row, []string{"金额", "发票金额", "不含税金额", "不含税金额（元）"}))
			a.Tax += parseFloat(cellByHeader(headers, row, []string{"税额", "发票税额", "发票税额（元）"}))
			a.Total += parseFloat(cellByHeader(headers, row, []string{"价税合计", "发票价税合计", "价税合计（元）"}))
			if hasAttachmentColumn && invoiceNoIdx >= 0 && invoiceNoIdx < len(row) && invoiceHasAttachment(row[invoiceNoIdx], attachments) {
				a.Attachments++
			}
		}
	}
	var b strings.Builder
	b.WriteString("### 发票数据摘要\n\n")
	for _, direction := range []string{"input", "output", "unknown"} {
		items := byDirection[direction]
		if len(items) == 0 {
			continue
		}
		b.WriteString("#### " + directionLabel(direction) + "\n\n")
		if hasAttachmentColumn {
			b.WriteString("| 发票状态 | 份数 | 不含税金额（元） | 发票税额（元） | 价税合计（元） | 发票附件数 |\n| --- | ---: | ---: | ---: | ---: | ---: |\n")
		} else {
			b.WriteString("| 发票状态 | 份数 | 不含税金额（元） | 发票税额（元） | 价税合计（元） |\n| --- | ---: | ---: | ---: | ---: |\n")
		}
		keys := make([]string, 0, len(items))
		for k := range items {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			a := items[key]
			if hasAttachmentColumn {
				b.WriteString(fmt.Sprintf("| %s | %d | %.2f | %.2f | %.2f | %d |\n", key, a.Count, a.Amount, a.Tax, a.Total, a.Attachments))
			} else {
				b.WriteString(fmt.Sprintf("| %s | %d | %.2f | %.2f | %.2f |\n", key, a.Count, a.Amount, a.Tax, a.Total))
			}
		}
		b.WriteString("\n")
	}
	if b.Len() == len("### 发票数据摘要\n\n") {
		b.WriteString("未读取到可汇总的发票数据。\n")
	}
	return b.String(), nil
}

func listPDFFilenames(dir string) []string {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".pdf") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func invoiceHasAttachment(invoiceNo string, attachmentFileNames []string) bool {
	no := normalizeInvoiceNo(invoiceNo)
	if no == "" {
		return false
	}
	for _, name := range attachmentFileNames {
		if strings.Contains(name, no) {
			return true
		}
	}
	return false
}

func normalizeInvoiceNo(value string) string {
	text := strings.TrimSpace(value)
	if strings.HasSuffix(text, ".0") {
		stem := strings.TrimSuffix(text, ".0")
		if stem != "" && regexp.MustCompile(`^\d+$`).MatchString(stem) {
			return stem
		}
	}
	return text
}

func GenerateInvoiceListFromTask(taskFile string) error {
	state, err := LoadTaskState(taskFile)
	if err != nil {
		return err
	}
	outputs := common.ObjectValue(state.Data, "outputs")
	storage, _ := state.Data["storage"].(map[string]any)
	files := stringSlice(outputs["tax_invoice_excel_files"])
	if len(files) == 0 {
		files = listXLSX(common.StringValue(outputs["tax_invoice_excel_dir"]))
	}
	if len(files) == 0 {
		return fmt.Errorf("任务中没有可用于生成发票清单的原始发票 Excel")
	}
	outDir := strings.TrimSpace(common.StringValue(storage["invoice_list_dir"]))
	if outDir == "" {
		return fmt.Errorf("任务文件缺少 storage.invoice_list_dir，无法确定发票清单保存目录")
	}
	deductionFingerprint, err := deductionDependencyFingerprint(state.Data)
	if err != nil {
		return err
	}
	deduction := deductionMapFromFile(common.StringValue(outputs["deduction_data_file"]))
	attachmentDir := common.StringValue(outputs["invoice_attachments"])
	result, err := GenerateInvoiceList(files, outDir, deduction, attachmentDir)
	if err != nil {
		return err
	}
	outputs["invoice_list"] = invoiceListOutputPaths(result)
	outputs["invoice_list_dir"] = outDir
	status := strings.TrimSpace(common.StringValue(result["status"]))
	generateExecution := common.ObjectValue(common.ObjectValue(state.Data, "execution"), generateInvoiceListStep)
	generateExecution["status"] = status
	generateExecution["deduction_dependency_fingerprint"] = deductionFingerprint
	generateExecution["generated_at"] = common.NowISO()
	generateExecution["success_files"] = result["success_files"]
	generateExecution["failed_files"] = result["failed_files"]
	invalidateAfterInvoiceList(state.Data, "发票清单已重新生成，统计和风险上下文需要基于新清单重建")
	markPlannedStepStatus(state.Data, generateInvoiceListStep, status, "")
	if err := state.Save(); err != nil {
		return err
	}
	if status == "failed" {
		return fmt.Errorf("发票清单生成失败: %v", result["failures"])
	}
	common.AppendMessageLog("发票清单生成完成")
	return nil
}

func GenerateInvoiceList(files []string, outDir string, deduction map[string]string, attachmentDirs ...string) (map[string]any, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	attachments := map[string]string{}
	if len(attachmentDirs) > 0 {
		attachments = attachmentMapFromDir(attachmentDirs[0])
	}
	outputs := []any{}
	failures := []any{}
	for _, file := range files {
		direction := inferDirectionKey(filepath.Base(file))
		headers, rows, err := mergedInvoiceRows(file, deduction, attachments, direction != "output")
		if err != nil {
			failures = append(failures, map[string]any{"input": file, "error": err.Error()})
			continue
		}
		out := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))+"-发票清单.xlsx")
		if err := writeInvoiceListWorkbook(out, "发票清单", headers, rows, attachments); err != nil {
			failures = append(failures, map[string]any{"input": file, "error": err.Error()})
			continue
		}
		outputs = append(outputs, map[string]any{"input": file, "output": out, "direction": directionLabel(direction)})
	}
	status := "success"
	if len(failures) > 0 && len(outputs) > 0 {
		status = "partial_success"
	} else if len(outputs) == 0 && len(failures) > 0 {
		status = "failed"
	}
	return map[string]any{"status": status, "output_dir": outDir, "success_files": len(outputs), "failed_files": len(failures), "outputs": outputs, "failures": failures}, nil
}

func mergedInvoiceRows(file string, deduction map[string]string, attachments map[string]string, includeDeduction bool) ([]string, [][]string, error) {
	base, err := readSheetRows(file, []string{"发票基础信息"})
	if err != nil {
		return nil, nil, err
	}
	detail, _ := readSheetRows(file, []string{"信息汇总表"})
	if len(base) < 1 {
		return nil, nil, fmt.Errorf("missing base rows")
	}
	headers := append([]string{}, base[0]...)
	if includeDeduction && !contains(headers, "勾选认证日期") {
		insertAt := findHeader(headers, []string{"开票日期"})
		if insertAt < 0 {
			insertAt = len(headers) - 1
		}
		headers = insertString(headers, insertAt+1, "勾选认证日期")
	} else if !includeDeduction {
		headers = removeString(headers, "勾选认证日期")
	}
	detailMap := map[string]map[string]string{}
	if len(detail) > 1 {
		dh := detail[0]
		keyIdx := findHeader(dh, []string{"数电发票号码"})
		for _, r := range detail[1:] {
			if keyIdx >= 0 && keyIdx < len(r) {
				m := map[string]string{}
				for i, h := range dh {
					if i < len(r) {
						m[h] = r[i]
					}
				}
				detailMap[r[keyIdx]] = m
			}
		}
	}
	addFields := []string{"税收分类编码", "特定业务类型", "货物或应税劳务名称", "规格型号", "单位", "数量", "单价", "明细金额", "税率", "明细税额"}
	for _, f := range addFields {
		if !contains(headers, f) {
			headers = append(headers, f)
		}
	}
	if len(attachments) > 0 && !contains(headers, "本地发票附件") {
		insertAt := findHeader(headers, []string{"备注"})
		if insertAt < 0 {
			insertAt = len(headers) - 1
		}
		headers = insertString(headers, insertAt+1, "本地发票附件")
	}
	rows := [][]string{}
	for _, r := range base[1:] {
		if isTotalRow(r) {
			continue
		}
		rowMap := map[string]string{}
		for i, h := range base[0] {
			if i < len(r) {
				rowMap[h] = r[i]
			}
		}
		no := firstText(rowMap, "数电发票号码", "数电票号码")
		if includeDeduction {
			rowMap["勾选认证日期"] = deduction[no]
		} else {
			delete(rowMap, "勾选认证日期")
		}
		if attachmentPath := strings.TrimSpace(attachments[no]); attachmentPath != "" {
			rowMap["本地发票附件"] = filepath.Base(attachmentPath)
		}
		if dm := detailMap[no]; dm != nil {
			for k, v := range dm {
				name := k
				if k == "金额" {
					name = "明细金额"
				}
				if k == "税额" {
					name = "明细税额"
				}
				rowMap[name] = v
			}
		}
		out := make([]string, len(headers))
		for i, h := range headers {
			out[i] = rowMap[h]
		}
		rows = append(rows, out)
	}
	return headers, rows, nil
}

func AnalyzeInvoiceDataCommand(taskFile string) error {
	state, err := LoadTaskState(taskFile)
	if err != nil {
		return err
	}
	storage, _ := state.Data["storage"].(map[string]any)
	outputs := common.ObjectValue(state.Data, "outputs")
	files := listXLSX(common.StringValue(firstNonEmpty(outputs["tax_invoice_excel_dir"], storage["raw_invoice_dir"], outputs["invoice_list_dir"], storage["invoice_list_dir"])))
	_, analysis := analyzeFiles(files)
	root := common.StringValue(firstNonEmpty(storage["root_dir"], state.Data["workspace_dir"]))
	mdPath := filepath.Join(root, "发票统计分析.md")
	xlsxPath := filepath.Join(root, "发票统计分析.xlsx")
	if err := writeAnalysisWorkbook(xlsxPath, analysis); err != nil {
		return err
	}
	if err := removeFileIfExists(mdPath); err != nil {
		return err
	}
	outputs["analysis_excel_path"] = xlsxPath
	delete(outputs, "analysis_markdown_path")
	if outputRequirementForAnalysis(state.Data) == "获取+统计" {
		if err := finalizeLoadedAnalysisArtifacts(state); err != nil {
			return err
		}
	}
	markPlannedStepStatus(state.Data, analyzeInvoiceDataStep, "success", "")
	if err := state.Save(); err != nil {
		return err
	}
	common.AppendMessageLog("统计分析工作簿已生成；本地存放路径：" + xlsxPath)
	return nil
}

type invoiceAnalysisRow struct {
	Direction      string
	Amount         float64
	Tax            float64
	Total          float64
	InvoiceType    string
	Status         string
	PositiveFlag   string
	HasPositiveCol bool
	IssueDate      time.Time
	HasIssueDate   bool
	BuyerName      string
	SellerName     string
	AttachmentRef  string
}

type moneyAgg struct {
	Count  int
	Amount float64
	Tax    float64
	Total  float64
}

type countAmountAgg struct {
	Count  int
	Amount float64
}

type periodStats struct {
	InputCount   int
	OutputCount  int
	InputAmount  float64
	OutputAmount float64
}

type topCounterparty struct {
	Name   string
	Amount float64
	Count  int
	Ratio  float64
}

func analyzeFiles(files []string) (string, map[string]any) {
	invoiceRows := []invoiceAnalysisRow{}
	attachmentRefs := map[string]bool{}
	for _, file := range files {
		rows, err := readSheetRows(file, []string{"发票基础信息", "发票清单", "信息汇总表", "鍙戠エ鍩虹淇℃伅", "鍙戠エ娓呭崟", "淇℃伅姹囨€昏〃"})
		if err != nil || len(rows) < 2 {
			continue
		}
		headers := rows[0]
		direction := inferDirectionKey(filepath.Base(file))
		positiveIdx := findHeader(headers, []string{"是否正数发票", "鏄惁姝ｆ暟鍙戠エ"})
		for _, row := range rows[1:] {
			if isTotalRow(row) || !hasInvoiceIdentity(headers, row) {
				continue
			}
			amount := parseFloat(cellByHeader(headers, row, amountHeaderKeys()))
			tax := parseFloat(cellByHeader(headers, row, taxHeaderKeys()))
			total := parseFloat(cellByHeader(headers, row, totalHeaderKeys()))
			if total == 0 && (amount != 0 || tax != 0) {
				total = amount + tax
			}
			issueDate, hasIssueDate := parseIssueDate(cellByHeader(headers, row, []string{"开票日期", "日期", "寮€绁ㄦ棩鏈?", "鏃ユ湡"}))
			attachmentRef := strings.TrimSpace(cellByHeader(headers, row, []string{"本地发票附件地址", "本地发票附件", "鏈湴鍙戠エ闄勪欢鍦板潃", "鏈湴鍙戠エ闄勪欢"}))
			if attachmentRef != "" {
				attachmentRefs[attachmentRef] = true
			}
			invoiceRows = append(invoiceRows, invoiceAnalysisRow{
				Direction:      direction,
				Amount:         amount,
				Tax:            tax,
				Total:          total,
				InvoiceType:    cellByHeader(headers, row, []string{"发票票种", "发票类型", "鍙戠エ绁ㄧ", "鍙戠エ绉嶇被"}),
				Status:         cellByHeader(headers, row, []string{"发票状态", "状态", "鍙戠エ鐘舵€?", "鐘舵€?"}),
				PositiveFlag:   cellByHeader(headers, row, []string{"是否正数发票", "鏄惁姝ｆ暟鍙戠エ"}),
				HasPositiveCol: positiveIdx >= 0,
				IssueDate:      issueDate,
				HasIssueDate:   hasIssueDate,
				BuyerName:      cellByHeader(headers, row, []string{"购方名称", "购买方名称", "购方单位名称", "璐柟鍚嶇О", "璐拱鏂瑰悕绉?", "璐柟鍗曚綅鍚嶇О"}),
				SellerName:     cellByHeader(headers, row, []string{"销方名称", "销售方名称", "销方单位名称", "閿€鏂瑰悕绉?", "閿€鍞柟鍚嶇О", "閿€鏂瑰崟浣嶅悕绉?"}),
				AttachmentRef:  attachmentRef,
			})
		}
	}
	analysis := buildInvoiceAnalysis(invoiceRows, len(attachmentRefs))
	return buildAnalysisMarkdown(analysis), analysis
}

func analyzeFilesLegacy(files []string) (string, map[string]any) {
	totalCount := 0
	amount := 0.0
	tax := 0.0
	total := 0.0
	byStatus := map[string]int{}
	byType := map[string]int{}
	for _, file := range files {
		rows, err := readSheetRows(file, []string{"发票基础信息", "发票清单", "信息汇总表"})
		if err != nil || len(rows) < 2 {
			continue
		}
		h := rows[0]
		for _, r := range rows[1:] {
			if isTotalRow(r) {
				continue
			}
			totalCount++
			amount += parseFloat(cellByHeader(h, r, []string{"金额", "发票金额", "不含税金额"}))
			tax += parseFloat(cellByHeader(h, r, []string{"税额", "发票税额"}))
			total += parseFloat(cellByHeader(h, r, []string{"价税合计", "发票价税合计"}))
			byStatus[classifyInvoiceStatus(cellByHeader(h, r, []string{"发票状态", "状态"}), cellByHeader(h, r, []string{"是否正数发票"}), parseFloat(cellByHeader(h, r, []string{"金额", "发票金额"})))]++
			t := cellByHeader(h, r, []string{"发票票种", "发票种类"})
			if t == "" {
				t = "未知"
			}
			byType[t]++
		}
	}
	var b strings.Builder
	b.WriteString("### 发票数据统计分析\n\n#### 总体统计\n\n")
	b.WriteString(fmt.Sprintf("- 发票数量: %d\n- 金额: %.2f\n- 税额: %.2f\n- 价税合计: %.2f\n\n", totalCount, amount, tax, total))
	b.WriteString("#### 按状态统计\n\n| 状态 | 数量 |\n| --- | ---: |\n")
	for _, k := range sortedKeysInt(byStatus) {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", k, byStatus[k]))
	}
	b.WriteString("\n#### 按票种统计\n\n| 票种 | 数量 |\n| --- | ---: |\n")
	for _, k := range sortedKeysInt(byType) {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", k, byType[k]))
	}
	return b.String(), map[string]any{"overview": map[string]any{"count": totalCount, "amount": round2(amount), "tax": round2(tax), "total": round2(total)}, "by_status": byStatus, "by_type": byType}
}

func amountHeaderKeys() []string {
	return []string{"金额", "发票金额", "不含税金额", "不含税金额（元）", "閲戦", "鍙戠エ閲戦", "涓嶅惈绋庨噾棰?", "涓嶅惈绋庨噾棰濓紙鍏冿級"}
}

func taxHeaderKeys() []string {
	return []string{"税额", "发票税额", "发票税额（元）", "绋庨", "鍙戠エ绋庨", "鍙戠エ绋庨锛堝厓锛?"}
}

func totalHeaderKeys() []string {
	return []string{"价税合计", "发票价税合计", "价税合计（元）", "浠风◣鍚堣", "鍙戠エ浠风◣鍚堣", "浠风◣鍚堣锛堝厓锛?"}
}

func hasInvoiceIdentity(headers, row []string) bool {
	return strings.TrimSpace(cellByHeader(headers, row, []string{"数电发票号码", "数电票号码", "销方识别号", "鏁扮數鍙戠エ鍙风爜", "鏁扮數绁ㄥ彿鐮?", "閿€鏂硅瘑鍒彿"})) != ""
}

func buildInvoiceAnalysis(rows []invoiceAnalysisRow, attachmentTotal int) map[string]any {
	overview := moneyAgg{}
	directionStats := map[string]*moneyAgg{"input": {}, "output": {}, "unknown": {}}
	typeStats := map[string]map[string]*countAmountAgg{
		"input":  {"special": {}, "normal": {}, "electronic": {}},
		"output": {"special": {}, "normal": {}, "electronic": {}},
	}
	statusTotal := map[string]int{"normal": 0, "reversed": 0, "void": 0}
	statusByDirection := map[string]map[string]int{
		"input":  {"normal": 0, "reversed": 0, "void": 0},
		"output": {"normal": 0, "reversed": 0, "void": 0},
	}
	periods := map[string]map[string]*periodStats{"month": {}, "quarter": {}, "year": {}}
	customerAmount, supplierAmount := map[string]float64{}, map[string]float64{}
	customerCount, supplierCount := map[string]int{}, map[string]int{}

	for _, row := range rows {
		overview.Count++
		overview.Amount += row.Amount
		overview.Tax += row.Tax
		overview.Total += row.Total
		ds := directionStats[row.Direction]
		if ds == nil {
			ds = directionStats["unknown"]
		}
		ds.Count++
		ds.Amount += row.Amount
		ds.Tax += row.Tax
		ds.Total += row.Total
		if row.Direction == "input" || row.Direction == "output" {
			if key := invoiceTypeBucket(row.InvoiceType); key != "" {
				typeStats[row.Direction][key].Count++
				typeStats[row.Direction][key].Amount += row.Amount
			}
		}
		if key := invoiceStatusKey(row.Status, row.PositiveFlag, row.Amount, row.HasPositiveCol); key != "" {
			statusTotal[key]++
			if row.Direction == "input" || row.Direction == "output" {
				statusByDirection[row.Direction][key]++
			}
		}
		if row.HasIssueDate {
			addPeriodStats(periods["month"], row.IssueDate.Format("2006-01"), row)
			addPeriodStats(periods["quarter"], fmt.Sprintf("%04d-Q%d", row.IssueDate.Year(), (int(row.IssueDate.Month())-1)/3+1), row)
			addPeriodStats(periods["year"], row.IssueDate.Format("2006"), row)
		}
		if row.Direction == "output" && strings.TrimSpace(row.BuyerName) != "" && row.Amount > 0 {
			name := strings.TrimSpace(row.BuyerName)
			customerAmount[name] += row.Amount
			customerCount[name]++
		}
		if row.Direction == "input" && strings.TrimSpace(row.SellerName) != "" && row.Amount > 0 {
			name := strings.TrimSpace(row.SellerName)
			supplierAmount[name] += row.Amount
			supplierCount[name]++
		}
	}

	analysis := map[string]any{
		"overview": map[string]any{
			"count":                  overview.Count,
			"amount":                 round2(overview.Amount),
			"tax":                    round2(overview.Tax),
			"total":                  round2(overview.Total),
			"invoice_total_count":    overview.Count,
			"attachment_total_count": attachmentTotal,
			"amount_total":           round2(overview.Amount),
			"tax_total":              round2(overview.Tax),
			"price_tax_total":        round2(overview.Total),
			"input":                  moneyAggMap(directionStats["input"]),
			"output":                 moneyAggMap(directionStats["output"]),
		},
		"direction_stats": map[string]any{
			"input":  moneyAggMap(directionStats["input"]),
			"output": moneyAggMap(directionStats["output"]),
			"ratio": map[string]any{
				"amount": ratio(directionStats["input"].Amount, directionStats["output"].Amount),
				"tax":    ratio(directionStats["input"].Tax, directionStats["output"].Tax),
				"total":  ratio(directionStats["input"].Total, directionStats["output"].Total),
			},
		},
		"type_stats": map[string]any{"input": typeAggMap(typeStats["input"]), "output": typeAggMap(typeStats["output"])},
		"status_stats": map[string]any{
			"normal_count":    statusTotal["normal"],
			"red_flush_count": statusTotal["reversed"],
			"void_count":      statusTotal["void"],
			"by_direction":    statusByDirection,
		},
		"by_status": map[string]int{"正常发票": statusTotal["normal"], "红冲发票": statusTotal["reversed"], "作废发票": statusTotal["void"]},
	}
	if len(periods["month"]) > 0 {
		monthKeys := sortedPeriodKeys(periods["month"])
		quarterKeys := sortedPeriodKeys(periods["quarter"])
		yearKeys := sortedPeriodKeys(periods["year"])
		analysis["time_stats"] = map[string]any{
			"monthly_series":   buildPeriodSeries(periods["month"], monthKeys, "month"),
			"quarterly_series": buildPeriodSeries(periods["quarter"], quarterKeys, "quarter"),
			"yearly_series":    buildPeriodSeries(periods["year"], yearKeys, "year"),
			"span": map[string]any{
				"month_count":       len(monthKeys),
				"quarter_count":     len(quarterKeys),
				"year_count":        len(yearKeys),
				"include_quarterly": len(quarterKeys) >= 2,
				"include_yearly":    len(yearKeys) >= 2,
			},
		}
	}
	if len(customerAmount) > 0 || len(supplierAmount) > 0 {
		customers := topCounterpartiesList(customerAmount, customerCount, 5)
		suppliers := topCounterpartiesList(supplierAmount, supplierCount, 5)
		analysis["counterparty_stats"] = map[string]any{
			"top5_customers":               topCounterpartyMaps(customers),
			"top5_suppliers":               topCounterpartyMaps(suppliers),
			"customer_concentration":       sumTopRatios(customers),
			"supplier_concentration":       sumTopRatios(suppliers),
			"customer_concentration_top10": concentrationRatio(customerAmount, 10),
			"supplier_concentration_top10": concentrationRatio(supplierAmount, 10),
		}
	}
	return analysis
}

func buildAnalysisMarkdown(a map[string]any) string {
	var b strings.Builder
	overview := objectFromAny(a["overview"])
	direction := objectFromAny(a["direction_stats"])
	typeStats := objectFromAny(a["type_stats"])
	statusStats := objectFromAny(a["status_stats"])
	timeStats := objectFromAny(a["time_stats"])
	counterparty := objectFromAny(a["counterparty_stats"])

	input := objectFromAny(overview["input"])
	output := objectFromAny(overview["output"])
	b.WriteString("发票数据分析已完成\n\n")
	b.WriteString("### 1. 总体统计\n\n| 统计项目 | 数值 | 备注 |\n| --- | --- | --- |\n")
	b.WriteString(fmt.Sprintf("| 发票总张数 | %d 张 | 进项 %d 张 + 销项 %d 张 |\n", intAny(overview["invoice_total_count"]), intAny(input["count"]), intAny(output["count"])))
	b.WriteString(fmt.Sprintf("| 附件总数 | %d 份 | 数电发票无纸质附件 |\n", intAny(overview["attachment_total_count"])))
	b.WriteString(fmt.Sprintf("| 总金额 | %s 元 | 进项 %s 元 + 销项 %s 元 |\n", formatMoneyAny(overview["amount_total"]), formatMoneyAny(input["amount"]), formatMoneyAny(output["amount"])))
	b.WriteString(fmt.Sprintf("| 总税额 | %s 元 | 进项 %s 元 + 销项 %s 元 |\n", formatMoneyAny(overview["tax_total"]), formatMoneyAny(input["tax"]), formatMoneyAny(output["tax"])))
	b.WriteString(fmt.Sprintf("| 价税合计 | %s 元 | 进项 %s 元 + 销项 %s 元 |\n\n", formatMoneyAny(overview["price_tax_total"]), formatMoneyAny(input["total"]), formatMoneyAny(output["total"])))

	b.WriteString("### 2. 方向统计\n\n| 发票方向 | 张数 | 金额（元） | 税额（元） | 价税合计（元） |\n| --- | --- | --- | --- | --- |\n")
	ratioMap := objectFromAny(direction["ratio"])
	b.WriteString(fmt.Sprintf("| 进项发票 | %d 张 | %s | %s | %s |\n", intAny(input["count"]), formatMoneyAny(input["amount"]), formatMoneyAny(input["tax"]), formatMoneyAny(input["total"])))
	b.WriteString(fmt.Sprintf("| 销项发票 | %d 张 | %s | %s | %s |\n", intAny(output["count"]), formatMoneyAny(output["amount"]), formatMoneyAny(output["tax"]), formatMoneyAny(output["total"])))
	b.WriteString(fmt.Sprintf("| 进销比 | - | %s | %s | %s |\n\n", percentOrDash(ratioMap["amount"], output["amount"]), percentOrDash(ratioMap["tax"], output["tax"]), percentOrDash(ratioMap["total"], output["total"])))

	b.WriteString("### 3. 类型统计\n\n")
	for _, dir := range []struct{ key, label string }{{"input", "进项"}, {"output", "销项"}} {
		dirType := objectFromAny(typeStats[dir.key])
		special := objectFromAny(dirType["special"])
		normal := objectFromAny(dirType["normal"])
		displayCount := intAny(special["count"]) + intAny(normal["count"])
		displayAmount := floatAny(special["amount"]) + floatAny(normal["amount"])
		b.WriteString(fmt.Sprintf("- %s发票\n\n| 发票类型 | 张数 | 金额（元） | 占比 |\n| --- | --- | --- | --- |\n", dir.label))
		b.WriteString(fmt.Sprintf("| 数电发票（增值税专用发票） | %d 张 | %s | %s |\n", intAny(special["count"]), formatMoneyAny(special["amount"]), formatPercent(countRatio(intAny(special["count"]), displayCount))))
		b.WriteString(fmt.Sprintf("| 数电发票（普通发票） | %d 张 | %s | %s |\n", intAny(normal["count"]), formatMoneyAny(normal["amount"]), formatPercent(countRatio(intAny(normal["count"]), displayCount))))
		totalRatio := "0.0%"
		if displayCount > 0 {
			totalRatio = "100.0%"
		}
		b.WriteString(fmt.Sprintf("| 合计 | %d 张 | %s | %s |\n\n", displayCount, formatMoney(displayAmount), totalRatio))
	}

	b.WriteString("### 4. 状态统计\n\n| 发票状态 | 进项情况（张数） | 销项情况（张数） | 进项占比 | 销项占比 |\n| --- | --- | --- | --- | --- |\n")
	statusByDir := objectFromAny(statusStats["by_direction"])
	inputStatus := intMapFromAny(statusByDir["input"])
	outputStatus := intMapFromAny(statusByDir["output"])
	inputCount, outputCount := intAny(input["count"]), intAny(output["count"])
	for _, item := range []struct{ key, label string }{{"normal", "正常"}, {"reversed", "已红冲"}, {"void", "作废"}} {
		ic, oc := inputStatus[item.key], outputStatus[item.key]
		b.WriteString(fmt.Sprintf("| %s | %d 张 | %d 张 | %s | %s |\n", item.label, ic, oc, formatPercent(countRatio(ic, inputCount)), formatPercent(countRatio(oc, outputCount))))
	}
	b.WriteString(fmt.Sprintf("| 合计 | %d 张 | %d 张 | 100.0%% | 100.0%% |\n\n", inputCount, outputCount))

	b.WriteString("### 5. 时间统计\n\n")
	appendPeriodTables(&b, sliceMapFromAny(timeStats["monthly_series"]), "月份", "month")
	span := objectFromAny(timeStats["span"])
	if boolAny(span["include_quarterly"]) {
		appendPeriodTables(&b, sliceMapFromAny(timeStats["quarterly_series"]), "季度", "quarter")
	}
	if boolAny(span["include_yearly"]) {
		appendPeriodTables(&b, sliceMapFromAny(timeStats["yearly_series"]), "年度", "year")
	}

	b.WriteString("### 6. 客户/供应商统计\n\n")
	appendTopCounterpartyTable(&b, sliceMapFromAny(counterparty["top5_customers"]), "前五大客户（销项）", "客户名称", "销售金额（元）", "占总销售额比例", floatAny(counterparty["customer_concentration"]))
	appendTopCounterpartyTable(&b, sliceMapFromAny(counterparty["top5_suppliers"]), "前五大供应商（进项）", "供应商名称", "采购金额（元）", "占总采购额比例", floatAny(counterparty["supplier_concentration"]))
	customerTop5 := floatAny(counterparty["customer_concentration"])
	supplierTop5 := floatAny(counterparty["supplier_concentration"])
	b.WriteString("- 客户与供应商集中度\n\n| 统计维度 | 前 5 大占比 | 前 10 大占比 | 集中度评价 |\n| --- | --- | --- | --- |\n")
	b.WriteString(fmt.Sprintf("| 客户集中度 | %s | %s | %s |\n", formatPercent(customerTop5), formatPercent(floatAny(counterparty["customer_concentration_top10"])), concentrationLevel(customerTop5)))
	b.WriteString(fmt.Sprintf("| 供应商集中度 | %s | %s | %s |\n\n", formatPercent(supplierTop5), formatPercent(floatAny(counterparty["supplier_concentration_top10"])), concentrationLevel(supplierTop5)))
	b.WriteString(buildAnalysisSummary(a))
	return strings.TrimRight(b.String(), "\n")
}

func buildAnalysisSummary(a map[string]any) string {
	overview := objectFromAny(a["overview"])
	typeStats := objectFromAny(a["type_stats"])
	statusStats := objectFromAny(a["status_stats"])
	timeStats := objectFromAny(a["time_stats"])
	counterparty := objectFromAny(a["counterparty_stats"])
	inputTypes := objectFromAny(typeStats["input"])
	outputTypes := objectFromAny(typeStats["output"])
	specialCount := intAny(objectFromAny(inputTypes["special"])["count"]) + intAny(objectFromAny(outputTypes["special"])["count"])
	normalCount := intAny(objectFromAny(inputTypes["normal"])["count"]) + intAny(objectFromAny(outputTypes["normal"])["count"])
	electronicCount := intAny(objectFromAny(inputTypes["electronic"])["count"]) + intAny(objectFromAny(outputTypes["electronic"])["count"])
	invoiceTotal := intAny(overview["invoice_total_count"])
	normalStatus := intAny(statusStats["normal_count"])
	timeSentence := "缺少可解析的开票日期，暂无法形成有效时间分布结论。"
	monthly := sliceMapFromAny(timeStats["monthly_series"])
	if len(monthly) > 0 {
		peak, trough := monthly[0], monthly[0]
		for _, item := range monthly {
			if intAny(item["total_count"]) > intAny(peak["total_count"]) {
				peak = item
			}
			if intAny(item["total_count"]) < intAny(trough["total_count"]) {
				trough = item
			}
		}
		span := objectFromAny(timeStats["span"])
		timeSentence = fmt.Sprintf("覆盖 %d 个月（%s 至 %s），其中 %s 开票最活跃（%d 张），%s 相对较低（%d 张）。", intAny(span["month_count"]), textAny(monthly[0]["month"]), textAny(monthly[len(monthly)-1]["month"]), textAny(peak["month"]), intAny(peak["total_count"]), textAny(trough["month"]), intAny(trough["total_count"]))
	}
	customers := sliceMapFromAny(counterparty["top5_customers"])
	suppliers := sliceMapFromAny(counterparty["top5_suppliers"])
	customerName, supplierName := "-", "-"
	if len(customers) > 0 {
		customerName = textAny(customers[0]["name"])
	}
	if len(suppliers) > 0 {
		supplierName = textAny(suppliers[0]["name"])
	}
	typeShare := ""
	if total := specialCount + normalCount + electronicCount; total > 0 {
		typeShare = fmt.Sprintf("，专票占比 %s，普票占比 %s", formatPercent(countRatio(specialCount, total)), formatPercent(countRatio(normalCount, total)))
	}
	return strings.Join([]string{
		"### 统计摘要汇总",
		"",
		fmt.Sprintf("1. 总体概况：本次共统计 %d 张发票、%d 份附件，不含税金额合计 %s 元，税额 %s 元，价税合计 %s 元。", invoiceTotal, intAny(overview["attachment_total_count"]), formatMoneyAny(overview["amount_total"]), formatMoneyAny(overview["tax_total"]), formatMoneyAny(overview["price_tax_total"])),
		fmt.Sprintf("2. 发票类型与状态：专票 %d 张、普票 %d 张、电子票 %d 张%s；状态上正常发票 %d 张（%s）、红冲 %d 张、作废 %d 张。", specialCount, normalCount, electronicCount, typeShare, normalStatus, formatPercent(countRatio(normalStatus, invoiceTotal)), intAny(statusStats["red_flush_count"]), intAny(statusStats["void_count"])),
		"3. 时间分布特征：" + timeSentence,
		fmt.Sprintf("4. 客户与供应商结构：前五大客户集中度为 %s（首位客户：%s），前五大供应商集中度为 %s（首位供应商：%s）；建议重点关注高集中对象的业务连续性与合规稳定性。", formatPercent(floatAny(counterparty["customer_concentration"])), customerName, formatPercent(floatAny(counterparty["supplier_concentration"])), supplierName),
	}, "\n")
}

func BuildInvoiceContextDataCommand(taskFile string) error {
	state, err := LoadTaskState(taskFile)
	if err != nil {
		return err
	}
	deductionFingerprint, err := ensureFreshInvoiceListForRisk(state)
	if err != nil {
		return err
	}
	storage, _ := state.Data["storage"].(map[string]any)
	outputs := common.ObjectValue(state.Data, "outputs")
	artifacts := common.ObjectValue(state.Data, "artifacts")
	inDir := common.StringValue(firstNonEmpty(outputs["invoice_list_dir"], storage["invoice_list_dir"], artifacts["invoice_list_dir"]))
	payload := buildContextPayload(inDir)
	if intAny(payload["record_count"]) == 0 {
		return fmt.Errorf("发票清单中没有可用于风险分析的记录，请检查 generate-invoice-list 执行结果")
	}
	root := common.StringValue(firstNonEmpty(storage["root_dir"], state.Data["workspace_dir"]))
	out := filepath.Join(root, "发票清单分析版数据.json")
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
	raw, _ := json.MarshalIndent(payload, "", "  ")
	if err := os.WriteFile(out, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	outputs["invoice_context_data_path"] = out
	outputs["risk_context_summary"] = payload["risk_summary"]
	delete(artifacts, "invoice_context_data_path")
	delete(artifacts, "invoice_context_data")
	delete(artifacts, "invoice_context_warnings")
	buildExecution := common.ObjectValue(common.ObjectValue(state.Data, "execution"), buildInvoiceContextDataStep)
	buildExecution["status"] = "success"
	buildExecution["deduction_dependency_fingerprint"] = deductionFingerprint
	buildExecution["record_count"] = payload["record_count"]
	buildExecution["generated_at"] = common.NowISO()
	invalidateAfterRiskContext(state.Data, "风险上下文已重新生成，风险报告需要基于新上下文重建")
	markPlannedStepStatus(state.Data, buildInvoiceContextDataStep, "success", "")
	if err := state.Save(); err != nil {
		return err
	}
	common.AppendMessageLog("生成风险分析数据源成功")
	return nil
}

func buildContextPayload(dir string) map[string]any {
	files := listXLSX(dir)
	records := []any{}
	fileMetas := []any{}
	for idx, file := range files {
		rows, err := readSheetRows(file, []string{"发票清单", "发票基础信息"})
		if err != nil || len(rows) < 2 {
			continue
		}
		fileID := fmt.Sprintf("file_%d", idx+1)
		fileMetas = append(fileMetas, map[string]any{"file_id": fileID, "path": file, "name": filepath.Base(file), "direction_tag": inferDirectionKey(filepath.Base(file))})
		h := rows[0]
		for i, r := range rows[1:] {
			if isTotalRow(r) {
				continue
			}
			obj := map[string]any{"file_id": fileID, "row_index": i + 2}
			for j, col := range h {
				if j < len(r) {
					obj[col] = r[j]
				}
			}
			records = append(records, obj)
		}
	}
	return map[string]any{"status": "success", "record_count": len(records), "files": fileMetas, "records": records, "risk_summary": buildRiskContextSummary(records, fileMetas), "warnings": []any{}}
}

func readSheetRows(file string, candidates []string) ([][]string, error) {
	f, err := excelize.OpenFile(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := ""
	for _, c := range candidates {
		if _, err := f.GetRows(c); err == nil {
			sheet = c
			break
		}
	}
	if sheet == "" {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, fmt.Errorf("no sheets")
		}
		sheet = sheets[0]
	}
	return f.GetRows(sheet)
}

func writeWorkbook(path, sheet string, headers []string, rows [][]string) error {
	f := excelize.NewFile()
	defaultSheet := f.GetSheetName(0)
	f.SetSheetName(defaultSheet, sheet)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			f.SetCellValue(sheet, cell, v)
		}
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return f.SaveAs(path)
}

func writeInvoiceListWorkbook(path, sheet string, headers []string, rows [][]string, attachments map[string]string) error {
	f := excelize.NewFile()
	defaultSheet := f.GetSheetName(0)
	f.SetSheetName(defaultSheet, sheet)
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "left", WrapText: true},
	})
	normalStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "left"},
	})
	wrapStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "left", WrapText: true},
	})
	hyperlinkStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "0563C1", Underline: "single"},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "left", WrapText: true},
	})
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, invoiceColumnWidth(h))
	}
	invoiceNoCol := findHeader(headers, []string{"数电发票号码", "数电票号码"})
	attachmentCol := findHeader(headers, []string{"本地发票附件"})
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			f.SetCellValue(sheet, cell, v)
			style := normalStyle
			if c < len(headers) && invoiceWrapTextColumns[headers[c]] {
				style = wrapStyle
			}
			_ = f.SetCellStyle(sheet, cell, cell, style)
		}
		if invoiceNoCol < 0 || attachmentCol < 0 || invoiceNoCol >= len(row) {
			continue
		}
		attachmentPath := strings.TrimSpace(attachments[strings.TrimSpace(row[invoiceNoCol])])
		if attachmentPath == "" {
			continue
		}
		cell, _ := excelize.CoordinatesToCellName(attachmentCol+1, r+2)
		target := attachmentPath
		if rel, err := filepath.Rel(filepath.Dir(path), attachmentPath); err == nil {
			target = filepath.ToSlash(rel)
		}
		_ = f.SetCellHyperLink(sheet, cell, target, "External")
		_ = f.SetCellStyle(sheet, cell, cell, hyperlinkStyle)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return f.SaveAs(path)
}

func invoiceColumnWidth(header string) float64 {
	if width, ok := invoiceColumnWidths[header]; ok {
		return width
	}
	return 15
}

func listXLSX(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, _ := os.ReadDir(dir)
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".xlsx") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func attachmentMapFromDir(dir string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(dir) == "" {
		return out
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	pattern := regexp.MustCompile(`(?i)^dzfp_([^_]+)_.+\.pdf$`)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") {
			continue
		}
		match := pattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 || strings.TrimSpace(match[1]) == "" {
			continue
		}
		if _, exists := out[match[1]]; exists {
			continue
		}
		out[match[1]] = filepath.Join(dir, entry.Name())
	}
	return out
}
func cellByHeader(headers, row []string, keys []string) string {
	idx := findHeader(headers, keys)
	if idx >= 0 && idx < len(row) {
		return row[idx]
	}
	return ""
}
func findHeader(headers []string, keys []string) int {
	for i, h := range headers {
		for _, k := range keys {
			if h == k || strings.Contains(h, k) {
				return i
			}
		}
	}
	return -1
}
func parseFloat(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func classifyInvoiceStatus(status, positive string, amount float64) string {
	if strings.Contains(status, "作废") {
		return "作废发票"
	}
	if strings.Contains(status, "红冲") || strings.EqualFold(positive, "否") || amount < 0 {
		return "红冲发票"
	}
	return "正常发票"
}
func inferDirectionKey(name string) string {
	n := NormalizeInvoiceFilenameLabel(name)
	if strings.Contains(n, "进项") {
		return "input"
	}
	if strings.Contains(n, "销项") {
		return "output"
	}
	return "unknown"
}
func directionLabel(k string) string {
	if k == "input" {
		return "进项发票"
	}
	if k == "output" {
		return "销项发票"
	}
	return "未知方向发票"
}
func isTotalRow(row []string) bool {
	for _, v := range row {
		if strings.Contains(v, "合计") {
			return true
		}
	}
	return false
}
func contains(items []string, target string) bool {
	for _, v := range items {
		if v == target {
			return true
		}
	}
	return false
}
func insertString(items []string, idx int, value string) []string {
	if idx < 0 || idx > len(items) {
		idx = len(items)
	}
	out := append([]string{}, items[:idx]...)
	out = append(out, value)
	out = append(out, items[idx:]...)
	return out
}
func removeString(items []string, value string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}
func firstText(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if strings.TrimSpace(m[k]) != "" {
			return strings.TrimSpace(m[k])
		}
	}
	return ""
}
func deductionMap(raw any) map[string]string {
	out := map[string]string{}
	for _, item := range anySlice(raw) {
		if obj, ok := item.(map[string]any); ok {
			no := common.StringValue(obj["sdphm"])
			date := common.StringValue(obj["gxsj"])
			if no != "" && date != "" {
				out[no] = date
			}
		}
	}
	return out
}
func deductionMapFromFile(path string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(path) == "" {
		return out
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return out
	}
	if obj, ok := payload.(map[string]any); ok {
		return deductionMap(obj["list"])
	}
	return deductionMap(payload)
}
func invoiceListOutputPaths(result map[string]any) []any {
	paths := []any{}
	for _, item := range anySlice(result["outputs"]) {
		if obj, ok := item.(map[string]any); ok {
			if path := strings.TrimSpace(common.StringValue(obj["output"])); path != "" {
				paths = append(paths, path)
			}
		}
	}
	return paths
}
func stringSlice(raw any) []string {
	out := []string{}
	for _, v := range anySlice(raw) {
		if s := strings.TrimSpace(common.StringValue(v)); s != "" {
			out = append(out, s)
		}
	}
	return out
}
func firstNonEmpty(values ...any) any {
	for _, v := range values {
		if strings.TrimSpace(common.StringValue(v)) != "" {
			return v
		}
	}
	return nil
}

func parseIssueDate(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006.01.02", "2006-01-02 15:04:05", "2006/01/02 15:04:05", "20060102"} {
		if t, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func invoiceTypeBucket(value string) string {
	text := strings.TrimSpace(value)
	switch {
	case strings.Contains(text, "专用") || strings.Contains(text, "涓撶敤"):
		return "special"
	case strings.Contains(text, "普通") || strings.Contains(text, "鏅€"):
		return "normal"
	case strings.Contains(text, "电子") || strings.Contains(text, "数电") || strings.Contains(text, "鐢靛瓙") || strings.Contains(text, "鏁扮數"):
		return "electronic"
	default:
		return ""
	}
}

func invoiceStatusKey(status, positive string, amount float64, hasPositiveCol bool) string {
	raw := strings.TrimSpace(status)
	if strings.Contains(raw, "作废") || strings.Contains(raw, "浣滃簾") {
		return "void"
	}
	isPositive, ok := parsePositiveFlag(positive)
	if hasPositiveCol {
		if (strings.Contains(raw, "正常") || strings.Contains(raw, "姝ｅ父")) && ok && !isPositive {
			return "reversed"
		}
		if (strings.Contains(raw, "正常") || strings.Contains(raw, "姝ｅ父") || strings.Contains(raw, "红冲") || strings.Contains(raw, "绾㈠啿")) && ok && isPositive {
			return "normal"
		}
		if strings.Contains(raw, "红冲") || strings.Contains(raw, "绾㈠啿") {
			return "reversed"
		}
		return ""
	}
	if strings.Contains(raw, "红冲") || strings.Contains(raw, "绾㈠啿") || amount < 0 {
		return "reversed"
	}
	if amount > 0 && (strings.Contains(raw, "正常") || strings.Contains(raw, "姝ｅ父") || raw == "") {
		return "normal"
	}
	return ""
}

func parsePositiveFlag(value string) (bool, bool) {
	text := strings.ToLower(strings.TrimSpace(value))
	switch text {
	case "是", "鏄?", "true", "1", "y", "yes":
		return true, true
	case "否", "鍚?", "false", "0", "n", "no":
		return false, true
	default:
		return false, false
	}
}

func addPeriodStats(items map[string]*periodStats, key string, row invoiceAnalysisRow) {
	item := items[key]
	if item == nil {
		item = &periodStats{}
		items[key] = item
	}
	if row.Direction == "input" {
		item.InputCount++
		item.InputAmount += row.Amount
	} else if row.Direction == "output" {
		item.OutputCount++
		item.OutputAmount += row.Amount
	}
}

func moneyAggMap(a *moneyAgg) map[string]any {
	if a == nil {
		a = &moneyAgg{}
	}
	return map[string]any{"count": a.Count, "amount": round2(a.Amount), "tax": round2(a.Tax), "total": round2(a.Total)}
}

func typeAggMap(items map[string]*countAmountAgg) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"special", "normal", "electronic"} {
		item := items[key]
		if item == nil {
			item = &countAmountAgg{}
		}
		out[key] = map[string]any{"count": item.Count, "amount": round2(item.Amount)}
	}
	return out
}

func ratio(num, den float64) float64 {
	if den <= 0 {
		return 0
	}
	return math.Round((num/den)*10000) / 10000
}

func sortedPeriodKeys(items map[string]*periodStats) []string {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func buildPeriodSeries(items map[string]*periodStats, keys []string, keyName string) []any {
	out := []any{}
	var prevCount int
	var prevAmount float64
	hasPrev := false
	for _, key := range keys {
		item := items[key]
		totalCount := item.InputCount + item.OutputCount
		totalAmount := item.InputAmount + item.OutputAmount
		entry := map[string]any{
			keyName:         key,
			"input_count":   item.InputCount,
			"output_count":  item.OutputCount,
			"total_count":   totalCount,
			"input_amount":  round2(item.InputAmount),
			"output_amount": round2(item.OutputAmount),
			"total_amount":  round2(totalAmount),
		}
		if hasPrev && prevCount > 0 {
			entry["count_mom"] = ratio(float64(totalCount-prevCount), float64(prevCount))
		}
		if hasPrev && prevAmount > 0 {
			entry["amount_mom"] = ratio(totalAmount-prevAmount, prevAmount)
		}
		out = append(out, entry)
		prevCount = totalCount
		prevAmount = totalAmount
		hasPrev = true
	}
	return out
}

func topCounterpartiesList(amounts map[string]float64, counts map[string]int, n int) []topCounterparty {
	total := 0.0
	for _, amount := range amounts {
		total += amount
	}
	items := make([]topCounterparty, 0, len(amounts))
	for name, amount := range amounts {
		items = append(items, topCounterparty{Name: name, Amount: round2(amount), Count: counts[name], Ratio: ratio(amount, total)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Amount == items[j].Amount {
			return items[i].Name < items[j].Name
		}
		return items[i].Amount > items[j].Amount
	})
	if len(items) > n {
		items = items[:n]
	}
	return items
}

func topCounterpartyMaps(items []topCounterparty) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{"name": item.Name, "amount": item.Amount, "count": item.Count, "ratio": item.Ratio})
	}
	return out
}

func sumTopRatios(items []topCounterparty) float64 {
	total := 0.0
	for _, item := range items {
		total += item.Ratio
	}
	return round4(total)
}

func concentrationRatio(amounts map[string]float64, n int) float64 {
	total := 0.0
	for _, amount := range amounts {
		total += amount
	}
	items := topCounterpartiesList(amounts, map[string]int{}, n)
	sum := 0.0
	for _, item := range items {
		sum += item.Amount
	}
	return ratio(sum, total)
}

func appendPeriodTables(b *strings.Builder, series []map[string]any, label, keyName string) {
	if len(series) == 0 {
		b.WriteString(fmt.Sprintf("- %s暂无可统计数据\n\n", label))
		return
	}
	b.WriteString(fmt.Sprintf("- %s发票数量变化\n\n| %s | 进项张数 | 销项张数 | 总张数 | 环比变化（总张数） |\n| --- | --- | --- | --- | --- |\n", label, label))
	for _, item := range series {
		b.WriteString(fmt.Sprintf("| %s | %d 张 | %d 张 | %d 张 | %s |\n", textAny(item[keyName]), intAny(item["input_count"]), intAny(item["output_count"]), intAny(item["total_count"]), momText(item["count_mom"])))
	}
	b.WriteString(fmt.Sprintf("\n- %s金额变化（单位：元）\n\n| %s | 进项金额 | 销项金额 | 总金额 | 环比变化（总金额） |\n| --- | --- | --- | --- | --- |\n", label, label))
	for _, item := range series {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", textAny(item[keyName]), formatMoneyAny(item["input_amount"]), formatMoneyAny(item["output_amount"]), formatMoneyAny(item["total_amount"]), momText(item["amount_mom"])))
	}
	b.WriteString("\n")
}

func appendTopCounterpartyTable(b *strings.Builder, items []map[string]any, title, nameHeader, amountHeader, ratioHeader string, concentration float64) {
	b.WriteString(fmt.Sprintf("- %s\n\n| 排名 | %s | %s | 发票张数 | %s |\n| --- | --- | --- | --- | --- |\n", title, nameHeader, amountHeader, ratioHeader))
	sumAmount := 0.0
	sumCount := 0
	for idx, item := range items {
		sumAmount += floatAny(item["amount"])
		sumCount += intAny(item["count"])
		b.WriteString(fmt.Sprintf("| %d | %s | %s | %d 张 | %s |\n", idx+1, textAny(item["name"]), formatMoneyAny(item["amount"]), intAny(item["count"]), formatPercent(floatAny(item["ratio"]))))
	}
	b.WriteString(fmt.Sprintf("| 合计 | - | %s | %d 张 | %s |\n\n", formatMoney(sumAmount), sumCount, formatPercent(concentration)))
}

func objectFromAny(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	if m, ok := value.(map[string]map[string]int); ok {
		out := map[string]any{}
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	return map[string]any{}
}

func intMapFromAny(value any) map[string]int {
	out := map[string]int{}
	switch m := value.(type) {
	case map[string]int:
		for k, v := range m {
			out[k] = v
		}
	case map[string]any:
		for k, v := range m {
			out[k] = intAny(v)
		}
	}
	return out
}

func sliceMapFromAny(value any) []map[string]any {
	out := []map[string]any{}
	for _, item := range anySlice(value) {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func intAny(value any) int {
	return intFrom(value)
}

func floatAny(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		return parseFloat(v)
	default:
		return 0
	}
}

func boolAny(value any) bool {
	v, _ := value.(bool)
	return v
}

func textAny(value any) string {
	return strings.TrimSpace(common.StringValue(value))
}

func formatMoneyAny(value any) string {
	return formatMoney(floatAny(value))
}

func formatMoney(value float64) string {
	return fmt.Sprintf("%.2f", round2(value))
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}

func countRatio(count, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(count) / float64(total)
}

func percentOrDash(ratioValue, denominator any) string {
	if round2(floatAny(denominator)) == 0 {
		return "-"
	}
	return formatPercent(floatAny(ratioValue))
}

func momText(value any) string {
	if value == nil {
		return "-"
	}
	v := floatAny(value)
	text := formatPercent(v)
	if v > 0 {
		return "+" + text
	}
	return text
}

func concentrationLevel(v float64) string {
	switch {
	case v >= 0.7:
		return "极高"
	case v >= 0.5:
		return "较高"
	case v >= 0.3:
		return "中等"
	case v >= 0.15:
		return "较低"
	default:
		return "极低"
	}
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

func sortedKeysInt(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func round2(v float64) float64 { return math.Round(v*100) / 100 }
