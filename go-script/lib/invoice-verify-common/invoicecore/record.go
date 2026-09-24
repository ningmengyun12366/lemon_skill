package invoicecore

import (
	"encoding/json"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

func buildRecord(entry invoiceinput.Entry, verifyResult *verifyapi.Result, verifyStatus, verifyTime string) IntermediateRecord {
	record := IntermediateRecord{
		ImportTime:       entry.ImportTime,
		Filename:         entry.Filename,
		FilePath:         entry.Path,
		InvoiceCode:      entry.QRData.InvoiceCode,
		InvoiceNumber:    entry.QRData.InvoiceNumber,
		CheckCode:        entry.QRData.CheckCode,
		InvoiceDate:      entry.QRData.InvoiceDate,
		TotalAmount:      entry.QRData.TotalAmount,
		VerifyStatus:     verifyStatus,
		VerifyTime:       verifyTime,
		ExtractionStatus: entry.ExtractionStatus,
		ExtractionReason: entry.ErrorMessage,
	}
	if verifyResult != nil && verifyResult.Success {
		data := verifyResult.Data
		record.InvoiceType = data.InvoiceType
		record.InvoiceStatus = data.InvoiceStatus
		record.SellerName = data.SellerName
		record.SellerTaxNo = data.SellerTaxNo
		record.BuyerName = data.BuyerName
		record.BuyerTaxNo = data.BuyerTaxNo
		record.AmountWithoutTax = data.AmountWithoutTax
		record.TaxAmount = data.TaxAmount
		record.TotalAmount = data.TotalAmount
		if data.LineItems != nil {
			payload, err := json.Marshal(data.LineItems)
			if err == nil {
				record.LineItemsJSON = string(payload)
			}
		}
	}
	return record
}

func (record IntermediateRecord) RowMap() map[string]string {
	return map[string]string{
		"导入时间":                             record.ImportTime,
		"文件名":                              record.Filename,
		"文件路径":                             record.FilePath,
		"发票类型":                             record.InvoiceType,
		"发票代码":                             record.InvoiceCode,
		"发票号码":                             record.InvoiceNumber,
		"校验码":                              record.CheckCode,
		"发票状态":                             record.InvoiceStatus,
		"开票日期":                             record.InvoiceDate,
		"销售方名称":                            record.SellerName,
		"销售方税号":                            record.SellerTaxNo,
		"购买方名称":                            record.BuyerName,
		"购买方税号":                            record.BuyerTaxNo,
		"不含税金额":                            record.AmountWithoutTax,
		"税额":                               record.TaxAmount,
		"商品明细":                             record.LineItemsJSON,
		"价税合计":                             record.TotalAmount,
		excelx.TaxBureauVerifyStatusHeader: record.VerifyStatus,
		excelx.VerifyTimeHeader:            record.VerifyTime,
	}
}
