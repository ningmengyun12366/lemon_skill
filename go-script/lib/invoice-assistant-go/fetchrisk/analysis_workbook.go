package fetchrisk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

type analysisWorkbookWriter struct {
	file        *excelize.File
	titleStyle  int
	headerStyle int
}

type analysisWorkbookTable struct {
	Title   string
	Headers []any
	Rows    [][]any
}

const analysisSheetName = "发票统计分析"

func writeAnalysisWorkbook(path string, analysis map[string]any) error {
	f := excelize.NewFile()
	writer, err := newAnalysisWorkbookWriter(f)
	if err != nil {
		return err
	}
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, analysisSheetName); err != nil {
		return err
	}
	row, err := writer.writeOverviewSheet(analysisSheetName, 1, analysis)
	if err != nil {
		return err
	}
	row, err = writer.writeDirectionSheet(analysisSheetName, row+1, analysis)
	if err != nil {
		return err
	}
	row, err = writer.writeTypeSheet(analysisSheetName, row+1, analysis)
	if err != nil {
		return err
	}
	row, err = writer.writeStatusSheet(analysisSheetName, row+1, analysis)
	if err != nil {
		return err
	}
	row, err = writer.writeTimeSheet(analysisSheetName, row+1, analysis)
	if err != nil {
		return err
	}
	if _, err = writer.writeCounterpartySheet(analysisSheetName, row+1, analysis); err != nil {
		return err
	}
	index, err := f.GetSheetIndex(analysisSheetName)
	if err != nil {
		return err
	}
	f.SetActiveSheet(index)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return f.SaveAs(path)
}

func newAnalysisWorkbookWriter(f *excelize.File) (*analysisWorkbookWriter, error) {
	titleStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 13},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "left"},
	})
	if err != nil {
		return nil, err
	}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "left", WrapText: true},
	})
	if err != nil {
		return nil, err
	}
	return &analysisWorkbookWriter{file: f, titleStyle: titleStyle, headerStyle: headerStyle}, nil
}

func (w *analysisWorkbookWriter) writeOverviewSheet(sheet string, row int, a map[string]any) (int, error) {
	overview := objectFromAny(a["overview"])
	input := objectFromAny(overview["input"])
	output := objectFromAny(overview["output"])
	rows := [][]any{
		{"发票总张数", fmt.Sprintf("%d 张", intAny(overview["invoice_total_count"])), fmt.Sprintf("进项 %d 张 + 销项 %d 张", intAny(input["count"]), intAny(output["count"]))},
		{"附件总数", fmt.Sprintf("%d 份", intAny(overview["attachment_total_count"])), "数电发票无纸质附件"},
		{"总金额", formatMoneyAny(overview["amount_total"]), fmt.Sprintf("进项 %s + 销项 %s", formatMoneyAny(input["amount"]), formatMoneyAny(output["amount"]))},
		{"总税额", formatMoneyAny(overview["tax_total"]), fmt.Sprintf("进项 %s + 销项 %s", formatMoneyAny(input["tax"]), formatMoneyAny(output["tax"]))},
		{"价税合计", formatMoneyAny(overview["price_tax_total"]), fmt.Sprintf("进项 %s + 销项 %s", formatMoneyAny(input["total"]), formatMoneyAny(output["total"]))},
	}
	return w.appendTable(sheet, row, analysisWorkbookTable{Title: "总体统计", Headers: []any{"统计项目", "数值", "备注"}, Rows: rows})
}

func (w *analysisWorkbookWriter) writeDirectionSheet(sheet string, row int, a map[string]any) (int, error) {
	overview := objectFromAny(a["overview"])
	input := objectFromAny(overview["input"])
	output := objectFromAny(overview["output"])
	ratioMap := objectFromAny(objectFromAny(a["direction_stats"])["ratio"])
	rows := [][]any{
		{"进项发票", fmt.Sprintf("%d 张", intAny(input["count"])), formatMoneyAny(input["amount"]), formatMoneyAny(input["tax"]), formatMoneyAny(input["total"])},
		{"销项发票", fmt.Sprintf("%d 张", intAny(output["count"])), formatMoneyAny(output["amount"]), formatMoneyAny(output["tax"]), formatMoneyAny(output["total"])},
		{"进销比", "-", percentOrDash(ratioMap["amount"], output["amount"]), percentOrDash(ratioMap["tax"], output["tax"]), percentOrDash(ratioMap["total"], output["total"])},
	}
	return w.appendTable(sheet, row, analysisWorkbookTable{Title: "方向统计", Headers: []any{"发票方向", "张数", "金额（元）", "税额（元）", "价税合计（元）"}, Rows: rows})
}

func removeFileIfExists(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
