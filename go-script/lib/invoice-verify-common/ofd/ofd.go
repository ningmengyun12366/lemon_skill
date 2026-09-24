package ofd

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxZipEntries              = 1024
	maxXMLFiles                = 256
	maxXMLFileBytes            = 5 * 1024 * 1024
	maxTotalXMLBytes           = 20 * 1024 * 1024
	maxResourceImages          = 64
	maxResourceImageBytes      = 20 * 1024 * 1024
	maxTotalResourceImageBytes = 100 * 1024 * 1024
)

const (
	StatusExtracted   = "extracted"
	StatusUnsupported = "unsupported"
	StatusInvalid     = "invalid_ofd"
)

type InvoiceFields struct {
	InvoiceCode      string
	InvoiceNumber    string
	InvoiceDate      string
	SellerName       string
	SellerTaxNo      string
	BuyerName        string
	BuyerTaxNo       string
	AmountWithoutTax string
	TaxAmount        string
	TotalAmount      string
	CheckCode        string
	InvoiceTypeCode  string
}

type Result struct {
	Status  string
	Fields  InvoiceFields
	Variant string
	Reason  string
}

type ResourceImage struct {
	Name string
	Data []byte
}

func ExtractInvoiceFields(path string) (Result, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return Result{Status: StatusInvalid, Reason: "OFD 文件不是有效 ZIP 包"}, nil
	}
	defer reader.Close()
	if len(reader.File) > maxZipEntries {
		return Result{Status: StatusUnsupported, Reason: fmt.Sprintf("OFD ZIP 条目过多，超过 %d 个限制", maxZipEntries)}, nil
	}

	files := make([]*zip.File, 0, len(reader.File))
	totalXMLBytes := uint64(0)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(file.Name), ".xml") {
			if len(files) >= maxXMLFiles {
				return Result{Status: StatusUnsupported, Reason: fmt.Sprintf("OFD XML 文件过多，超过 %d 个限制", maxXMLFiles)}, nil
			}
			if file.UncompressedSize64 > maxXMLFileBytes {
				return Result{Status: StatusUnsupported, Reason: fmt.Sprintf("OFD XML 文件过大，超过 %d MB 限制", maxXMLFileBytes/(1024*1024))}, nil
			}
			totalXMLBytes += file.UncompressedSize64
			if totalXMLBytes > maxTotalXMLBytes {
				return Result{Status: StatusUnsupported, Reason: fmt.Sprintf("OFD XML 总大小过大，超过 %d MB 限制", maxTotalXMLBytes/(1024*1024))}, nil
			}
			files = append(files, file)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		left := strings.ToLower(files[i].Name)
		right := strings.ToLower(files[j].Name)
		if left == right {
			return files[i].Name < files[j].Name
		}
		return left < right
	})

	for _, file := range files {
		fields, err := extractFromXML(file)
		if err != nil {
			return Result{}, err
		}
		if isKnownInvoice(fields) {
			return Result{Status: StatusExtracted, Fields: fields, Variant: "generic_invoice_xml"}, nil
		}
	}
	return Result{Status: StatusUnsupported, Reason: "未识别的 OFD 发票 XML 结构"}, nil
}

func ExtractResourceImages(path string) ([]ResourceImage, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil
	}
	defer reader.Close()
	if len(reader.File) > maxZipEntries {
		return nil, fmt.Errorf("OFD ZIP 条目过多，超过 %d 个限制", maxZipEntries)
	}

	files := make([]*zip.File, 0)
	totalImageBytes := uint64(0)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !isImageResource(file.Name) {
			continue
		}
		if len(files) >= maxResourceImages {
			continue
		}
		if file.UncompressedSize64 > maxResourceImageBytes {
			continue
		}
		totalImageBytes += file.UncompressedSize64
		if totalImageBytes > maxTotalResourceImageBytes {
			continue
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		left := strings.ToLower(files[i].Name)
		right := strings.ToLower(files[j].Name)
		if left == right {
			return files[i].Name < files[j].Name
		}
		return left < right
	})

	images := make([]ResourceImage, 0, len(files))
	readImageBytes := 0
	for _, file := range files {
		entry, err := file.Open()
		if err != nil {
			return nil, err
		}
		payload, readErr := io.ReadAll(io.LimitReader(entry, maxResourceImageBytes+1))
		closeErr := entry.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(payload) > maxResourceImageBytes {
			readImageBytes += len(payload)
			if readImageBytes > maxTotalResourceImageBytes {
				break
			}
			continue
		}
		readImageBytes += len(payload)
		if readImageBytes > maxTotalResourceImageBytes {
			break
		}
		images = append(images, ResourceImage{Name: file.Name, Data: payload})
	}
	return images, nil
}

func isImageResource(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg":
		return true
	default:
		return false
	}
}

func extractFromXML(file *zip.File) (InvoiceFields, error) {
	entry, err := file.Open()
	if err != nil {
		return InvoiceFields{}, err
	}
	defer entry.Close()
	payload, err := io.ReadAll(io.LimitReader(entry, maxXMLFileBytes+1))
	if err != nil {
		return InvoiceFields{}, err
	}
	if len(payload) > maxXMLFileBytes {
		return InvoiceFields{}, fmt.Errorf("OFD XML 文件过大，超过 %d MB 限制", maxXMLFileBytes/(1024*1024))
	}

	decoder := xml.NewDecoder(bytes.NewReader(payload))
	values := make(map[string]string)
	var stack []xml.Name
	var texts []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return InvoiceFields{}, fmt.Errorf("解析 OFD XML 失败: %s", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			stack = append(stack, typed.Name)
			texts = append(texts, "")
		case xml.CharData:
			if len(texts) > 0 {
				texts[len(texts)-1] += string(typed)
			}
		case xml.EndElement:
			if len(stack) == 0 || len(texts) == 0 {
				continue
			}
			name := stack[len(stack)-1]
			text := strings.TrimSpace(texts[len(texts)-1])
			stack = stack[:len(stack)-1]
			texts = texts[:len(texts)-1]
			if text != "" {
				values[strings.ToLower(name.Local)] = text
			}
		}
	}

	return InvoiceFields{
		InvoiceCode:      firstValue(values, "invoicecode", "fpdm", "发票代码"),
		InvoiceNumber:    firstValue(values, "invoicenumber", "invoiceno", "fphm", "发票号码"),
		InvoiceDate:      normalizeDate(firstValue(values, "invoicedate", "issuedate", "kprq", "开票日期")),
		SellerName:       firstValue(values, "sellername", "xsfmc", "销售方名称"),
		SellerTaxNo:      firstValue(values, "sellertaxno", "xsfnsrsbh", "销售方税号"),
		BuyerName:        firstValue(values, "buyername", "gmfmc", "购买方名称"),
		BuyerTaxNo:       firstValue(values, "buyertaxno", "gmfnsrsbh", "购买方税号"),
		AmountWithoutTax: firstValue(values, "amountwithouttax", "hjje", "不含税金额"),
		TaxAmount:        firstValue(values, "taxamount", "hjse", "税额"),
		TotalAmount:      firstValue(values, "totalamount", "jshj", "价税合计"),
		CheckCode:        firstValue(values, "checkcode", "jym", "校验码"),
		InvoiceTypeCode:  firstValue(values, "invoicetypecode", "fppzdm", "票种代码"),
	}, nil
}

func firstValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[strings.ToLower(key)]); value != "" {
			return value
		}
	}
	return ""
}

func isKnownInvoice(fields InvoiceFields) bool {
	return fields.InvoiceNumber != "" && fields.InvoiceDate != "" && fields.TotalAmount != ""
}

func normalizeDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 8 && allDigits(value) {
		return value[:4] + "-" + value[4:6] + "-" + value[6:8]
	}
	return value
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
