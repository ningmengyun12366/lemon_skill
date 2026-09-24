package excelx

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"

	"github.com/xuri/excelize/v2"
)

const (
	DefaultSheetName = "发票查验结果"
	AmountFormat     = "0.00"
	SourcePathKey    = "文件路径"
)

type WorkbookSpec struct {
	SheetName     string
	Headers       []string
	Rows          []map[string]any
	AmountHeaders map[string]bool
	Widths        map[string]float64
	Hyperlinks    bool
}

func NewWorkbook(spec WorkbookSpec) (*excelize.File, error) {
	if len(spec.Headers) == 0 {
		return nil, errors.New("headers are required")
	}
	sheet := spec.SheetName
	if strings.TrimSpace(sheet) == "" {
		sheet = DefaultSheetName
	}

	f := excelize.NewFile()
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheet); err != nil {
		return nil, err
	}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	amountStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: stringPtr(AmountFormat)})
	if err != nil {
		return nil, err
	}
	linkStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Color: "0000FF", Underline: "single"}})
	if err != nil {
		return nil, err
	}

	for idx, header := range spec.Headers {
		cell, err := excelize.CoordinatesToCellName(idx+1, 1)
		if err != nil {
			return nil, err
		}
		if err := f.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
		if err := f.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return nil, err
		}
		if width, ok := spec.Widths[header]; ok {
			col, err := excelize.ColumnNumberToName(idx + 1)
			if err != nil {
				return nil, err
			}
			if err := f.SetColWidth(sheet, col, col, width); err != nil {
				return nil, err
			}
		}
	}

	for rowIdx, row := range spec.Rows {
		for colIdx, header := range spec.Headers {
			cell, err := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			if err != nil {
				return nil, err
			}
			value := row[header]
			if spec.Hyperlinks && header == "文件名" && nonEmpty(value) && strings.TrimSpace(fmt.Sprint(row[SourcePathKey])) != "" {
				formula := hyperlinkFormula(fmt.Sprint(row[SourcePathKey]), fmt.Sprint(value))
				if err := f.SetCellFormula(sheet, cell, formula); err != nil {
					return nil, err
				}
				if err := f.SetCellStyle(sheet, cell, cell, linkStyle); err != nil {
					return nil, err
				}
				continue
			}
			if header == "发票代码" {
				value = DisplayInvoiceCode(value)
			}
			if spec.AmountHeaders[header] {
				if amount, ok := CoerceAmount(value); ok {
					if err := f.SetCellValue(sheet, cell, amount); err != nil {
						return nil, err
					}
					if err := f.SetCellStyle(sheet, cell, cell, amountStyle); err != nil {
						return nil, err
					}
					continue
				}
			}
			if err := f.SetCellValue(sheet, cell, value); err != nil {
				return nil, err
			}
		}
	}

	return f, nil
}

func WriteWorkbook(path string, spec WorkbookSpec, now time.Time) (string, error) {
	f, err := NewWorkbook(spec)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	return SaveWithCollisionFallback(f, path, now)
}

func SaveWithCollisionFallback(f *excelize.File, target string, now time.Time) (string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	normalized, err := pathx.Normalize(target)
	if err != nil {
		return "", err
	}
	if err := pathx.EnsureParent(normalized); err != nil {
		return "", err
	}
	if err := f.SaveAs(normalized); err == nil {
		return normalized, nil
	} else if !isOccupiedFileError(err) {
		return "", err
	}
	fallback, err := pathx.WithCollisionSuffix(normalized, now)
	if err != nil {
		return "", err
	}
	if err := f.SaveAs(fallback); err != nil {
		return "", err
	}
	return fallback, nil
}

func ReadWorkbookSummary(path string, selectedWidthHeaders []string) (WorkbookSummary, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return WorkbookSummary{}, err
	}
	defer func() { _ = f.Close() }()
	return SummarizeWorkbook(f, selectedWidthHeaders)
}

func SummarizeWorkbook(f *excelize.File, selectedWidthHeaders []string) (WorkbookSummary, error) {
	sheets := f.GetSheetList()
	summary := WorkbookSummary{Sheets: sheets}
	if len(sheets) == 0 {
		return summary, nil
	}
	sheet := sheets[0]
	rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return WorkbookSummary{}, err
	}
	if len(rows) == 0 {
		return summary, nil
	}
	summary.Headers = trimTrailingEmpty(rows[0])
	for rowIdx := 2; rowIdx <= len(rows); rowIdx++ {
		row := make(map[string]string, len(summary.Headers))
		for colIdx, header := range summary.Headers {
			cell, err := excelize.CoordinatesToCellName(colIdx+1, rowIdx)
			if err != nil {
				return WorkbookSummary{}, err
			}
			formula, _ := f.GetCellFormula(sheet, cell)
			if formula != "" {
				row[header] = formula
			} else {
				value, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
				if err != nil {
					return WorkbookSummary{}, err
				}
				row[header] = value
			}
			styleID, err := f.GetCellStyle(sheet, cell)
			if err != nil {
				return WorkbookSummary{}, err
			}
			style, err := f.GetStyle(styleID)
			if err != nil {
				return WorkbookSummary{}, err
			}
			if style.CustomNumFmt != nil && *style.CustomNumFmt != "" {
				summary.NumberFormats = append(summary.NumberFormats, CellFormat{Cell: cell, Header: header, Format: *style.CustomNumFmt})
			}
			if formula != "" && strings.HasPrefix(strings.ToUpper(formula), "HYPERLINK(") {
				target, display := parseHyperlinkFormula(formula)
				summary.Hyperlinks = append(summary.Hyperlinks, CellHyperlink{Cell: cell, Header: header, Target: target, Display: display})
			}
		}
		summary.Rows = append(summary.Rows, row)
	}
	widthHeaders := stringSet(selectedWidthHeaders...)
	for colIdx, header := range summary.Headers {
		if !widthHeaders[header] {
			continue
		}
		col, err := excelize.ColumnNumberToName(colIdx + 1)
		if err != nil {
			return WorkbookSummary{}, err
		}
		width, err := f.GetColWidth(sheet, col)
		if err != nil {
			return WorkbookSummary{}, err
		}
		summary.Widths = append(summary.Widths, ColumnWidth{Header: header, Width: width})
	}
	return summary, nil
}

func IsMissingInvoiceCode(value any) bool {
	if value == nil {
		return true
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return true
	}
	switch strings.ToLower(text) {
	case "nan", "none":
		return true
	default:
		return false
	}
}

func DisplayInvoiceCode(value any) any {
	if IsMissingInvoiceCode(value) {
		return "--"
	}
	return value
}

func CoerceAmount(value any) (float64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case bool:
		return 0, false
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		amount := float64(typed)
		return amount, !math.IsNaN(amount) && !math.IsInf(amount, 0)
	case string:
		text := strings.TrimSpace(strings.ReplaceAll(typed, ",", ""))
		if text == "" {
			return 0, false
		}
		amount, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
			return 0, false
		}
		return amount, true
	default:
		return CoerceAmount(fmt.Sprint(value))
	}
}

func isOccupiedFileError(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "permission denied") || strings.Contains(message, "being used") || strings.Contains(message, "access is denied")
}

func hyperlinkFormula(path, display string) string {
	if target, ok := fileURLTarget(path); ok {
		return fmt.Sprintf(`HYPERLINK("%s","%s")`, escapeFormulaString(target), escapeFormulaString(display))
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	target := filepath.ToSlash(absPath)
	if filepath.VolumeName(absPath) != "" && !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	if filepath.VolumeName(absPath) != "" {
		target = "file://" + target
	}
	return fmt.Sprintf(`HYPERLINK("%s","%s")`, escapeFormulaString(target), escapeFormulaString(display))
}

func fileURLTarget(rawPath string) (string, bool) {
	trimmed := strings.TrimSpace(rawPath)
	parsed, err := url.Parse(trimmed)
	if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
		return "", false
	}
	pathValue := parsed.Path
	if parsed.Host != "" {
		return "file://" + parsed.Host + pathValue, true
	}
	return "file://" + pathValue, true
}

func escapeFormulaString(value string) string {
	return strings.ReplaceAll(value, `"`, `""`)
}

func nonEmpty(value any) bool {
	return strings.TrimSpace(fmt.Sprint(value)) != ""
}

func stringPtr(value string) *string {
	return &value
}

func trimTrailingEmpty(values []string) []string {
	end := len(values)
	for end > 0 && values[end-1] == "" {
		end--
	}
	return values[:end]
}

var hyperlinkRe = regexp.MustCompile(`(?i)^HYPERLINK\("((?:[^"]|"")*)",\s*"((?:[^"]|"")*)"\)$`)

func parseHyperlinkFormula(formula string) (string, string) {
	match := hyperlinkRe.FindStringSubmatch(strings.TrimSpace(formula))
	if len(match) != 3 {
		return "", ""
	}
	return strings.ReplaceAll(match[1], `""`, `"`), strings.ReplaceAll(match[2], `""`, `"`)
}
