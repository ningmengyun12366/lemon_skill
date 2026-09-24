package invoiceinput

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
)

const (
	IntermediateResultFilename = "发票查验_中间结果（系统生成，请勿使用）.xlsx"

	ModeFolder = "folder"
	ModeList   = "list"

	FileTypePDF         = "pdf"
	FileTypeImage       = "image"
	FileTypeOFD         = "ofd"
	FileTypeUnsupported = "unsupported"
	FileTypeXLSXList    = "xlsx_list"
	FileTypeCSVList     = "csv_list"
)

var (
	ImageExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true}
	ListExtensions  = map[string]bool{".xlsx": true, ".csv": true}

	ListInvoiceNumberKeys = []string{"发票号码"}
	ListInvoiceDateKeys   = []string{"开票日期"}
	ListTotalAmountKeys   = []string{"价税合计"}
	ListInvoiceCodeKeys   = []string{"发票代码"}
	ListCheckCodeKeys     = []string{"校验码"}

	IntermediateHeaders = []string{
		"导入时间",
		"文件名",
		"发票类型",
		"发票代码",
		"发票号码",
		"发票状态",
		"开票日期",
		"销售方名称",
		"销售方税号",
		"购买方名称",
		"购买方税号",
		"不含税金额",
		"税额",
		"商品明细",
		"价税合计",
		"税局查验状态",
		"查验时间",
	}
)

type QRData struct {
	InvoiceNumber   string `json:"invoice_number"`
	InvoiceTypeCode string `json:"invoice_type_code"`
	InvoiceCode     string `json:"invoice_code"`
	InvoiceDate     string `json:"invoice_date"`
	CheckCode       string `json:"check_code"`
	TotalAmount     string `json:"total_amount"`
}

type Entry struct {
	Filename         string
	Path             string
	IsSupported      bool
	FileType         string
	QRSuccess        bool
	QRData           QRData
	ErrorMessage     string
	ExtractionStatus string
	ImportTime       string
	SourceIndex      int
}

type ScanResult struct {
	Mode             string
	InputPath        string
	Entries          []Entry
	UnsupportedCount int
}

type Options struct {
	Now func() time.Time
}

func Scan(rawInput string, options Options) (ScanResult, error) {
	inputPath, err := pathx.Normalize(rawInput)
	if err != nil {
		return ScanResult{}, err
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return ScanResult{}, fmt.Errorf("输入路径不存在或该文件不支持查验: %s", inputPath)
	}
	if info.IsDir() {
		return scanFolder(inputPath, options)
	}
	if !info.Mode().IsRegular() {
		return ScanResult{}, fmt.Errorf("输入路径不存在或该文件不支持查验: %s", inputPath)
	}

	ext := strings.ToLower(filepath.Ext(inputPath))
	switch ext {
	case ".xlsx":
		entries, err := readXLSXList(inputPath, options)
		if err != nil {
			return ScanResult{}, err
		}
		return ScanResult{Mode: ModeList, InputPath: inputPath, Entries: entries}, nil
	case ".csv":
		entries, err := readCSVList(inputPath, options)
		if err != nil {
			return ScanResult{}, err
		}
		return ScanResult{Mode: ModeList, InputPath: inputPath, Entries: entries}, nil
	default:
		return ScanResult{}, fmt.Errorf("输入路径不存在或该文件不支持查验: %s", inputPath)
	}
}

func SupportedFileType(path string) (bool, string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case ext == ".pdf":
		return true, FileTypePDF
	case ext == ".ofd":
		return true, FileTypeOFD
	case ImageExtensions[ext]:
		return true, FileTypeImage
	default:
		return false, FileTypeUnsupported
	}
}

func scanFolder(inputPath string, options Options) (ScanResult, error) {
	entries, err := os.ReadDir(inputPath)
	if err != nil {
		return ScanResult{}, err
	}
	files := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			files = append(files, entry)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		left := files[i].Name()
		right := files[j].Name()
		leftLower := strings.ToLower(left)
		rightLower := strings.ToLower(right)
		if leftLower == rightLower {
			return left < right
		}
		return leftLower < rightLower
	})

	result := ScanResult{Mode: ModeFolder, InputPath: inputPath}
	for index, file := range files {
		filename := file.Name()
		fullPath := filepath.Join(inputPath, filename)
		supported, fileType := SupportedFileType(fullPath)
		entry := Entry{
			Filename:    filename,
			Path:        fullPath,
			IsSupported: supported,
			FileType:    fileType,
			ImportTime:  formatImportTime(options),
			SourceIndex: index + 1,
		}
		if !supported {
			entry.ErrorMessage = "该文件不支持查验"
			result.UnsupportedCount++
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func readXLSXList(path string, options Options) ([]Entry, error) {
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer workbook.Close()

	sheetName := workbook.GetSheetName(workbook.GetActiveSheetIndex())
	if strings.TrimSpace(sheetName) == "" {
		sheets := workbook.GetSheetList()
		if len(sheets) == 0 {
			return nil, errors.New("Excel 工作簿无活动工作表")
		}
		sheetName = sheets[0]
	}
	rows, err := workbook.GetRows(sheetName)
	if err != nil {
		return nil, err
	}
	return readListRows(path, FileTypeXLSXList, rows, options)
}

func readCSVList(path string, options Options) ([]Entry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	var rows [][]string
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return readListRows(path, FileTypeCSVList, rows, options)
}

func readListRows(path, fileType string, rows [][]string, options Options) ([]Entry, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	headers := make([]string, len(rows[0]))
	for i, value := range rows[0] {
		headers[i] = strings.TrimSpace(strings.TrimPrefix(cellToString(value), "\ufeff"))
	}

	idxNumber := findHeaderIndex(headers, ListInvoiceNumberKeys)
	idxDate := findHeaderIndex(headers, ListInvoiceDateKeys)
	idxTotal := findHeaderIndex(headers, ListTotalAmountKeys)
	idxCode := findHeaderIndex(headers, ListInvoiceCodeKeys)
	idxCheck := findHeaderIndex(headers, ListCheckCodeKeys)

	var missing []string
	if idxNumber < 0 {
		missing = append(missing, "发票号码")
	}
	if idxDate < 0 {
		missing = append(missing, "开票日期")
	}
	if idxTotal < 0 {
		missing = append(missing, "价税合计")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("发票清单缺少必需列: %s", strings.Join(missing, ", "))
	}

	entries := make([]Entry, 0, len(rows)-1)
	for _, row := range rows[1:] {
		invoiceNumber := valueAt(row, idxNumber)
		if invoiceNumber == "" {
			continue
		}
		invoiceDate := valueAt(row, idxDate)
		totalAmount := valueAt(row, idxTotal)
		invoiceCode := normalizeInvoiceCode(valueAt(row, idxCode))
		checkCode := valueAt(row, idxCheck)

		var messages []string
		if invoiceDate == "" {
			messages = append(messages, "开票日期为空")
		}
		if totalAmount == "" {
			messages = append(messages, "价税合计为空")
		}

		entries = append(entries, Entry{
			Path:        path,
			IsSupported: true,
			FileType:    fileType,
			QRSuccess:   len(messages) == 0,
			QRData: QRData{
				InvoiceNumber:   invoiceNumber,
				InvoiceTypeCode: "",
				InvoiceCode:     invoiceCode,
				InvoiceDate:     invoiceDate,
				CheckCode:       checkCode,
				TotalAmount:     totalAmount,
			},
			ErrorMessage: strings.Join(messages, "；"),
			ImportTime:   formatImportTime(options),
			SourceIndex:  len(entries) + 1,
		})
	}
	return entries, nil
}

func findHeaderIndex(headers []string, candidates []string) int {
	for _, candidate := range candidates {
		for index, header := range headers {
			if header == candidate {
				return index
			}
		}
	}
	for index, header := range headers {
		for _, candidate := range candidates {
			if strings.Contains(header, candidate) {
				return index
			}
		}
	}
	return -1
}

func valueAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return cellToString(row[index])
}

func cellToString(value string) string {
	return strings.TrimSpace(value)
}

func normalizeInvoiceCode(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || text == "--" || strings.EqualFold(text, "nan") || strings.EqualFold(text, "none") {
		return ""
	}
	return text
}

func formatImportTime(options Options) string {
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	return now().Format("2006-01-02 15:04:05")
}
