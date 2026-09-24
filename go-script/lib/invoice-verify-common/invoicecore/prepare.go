package invoicecore

import (
	"context"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/ofd"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pdfextract"
	invoiceqr "invoice-assistant-skill/go-script/lib/invoice-verify-common/qrcode"
)

type prepareEntryFunc func(invoiceinput.Entry, string) invoiceinput.Entry

type prepareEntriesOptions struct {
	Mode    string
	Prepare prepareEntryFunc
}

func prepareEntries(ctx context.Context, entries []invoiceinput.Entry, options prepareEntriesOptions) ([]invoiceinput.Entry, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prepare := options.Prepare
	if prepare == nil {
		prepare = prepareEntry
	}
	prepared := make([]invoiceinput.Entry, len(entries))
	jobCh := make(chan int)
	doneCh := make(chan struct{}, prepareEntryWorkerCount(len(entries)))
	workerCount := cap(doneCh)
	for range workerCount {
		go func() {
			defer func() {
				doneCh <- struct{}{}
			}()
			for index := range jobCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				prepared[index] = prepare(entries[index], options.Mode)
			}
		}()
	}

sendLoop:
	for index := range entries {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobCh <- index:
		}
	}
	close(jobCh)
	for range workerCount {
		<-doneCh
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return prepared, nil
}

func prepareEntry(entry invoiceinput.Entry, mode string) invoiceinput.Entry {
	if mode == invoiceinput.ModeList || !entry.IsSupported || entry.QRSuccess {
		return entry
	}
	switch entry.FileType {
	case invoiceinput.FileTypeImage:
		result := invoiceqr.DecodeInvoiceImageFile(entry.Path)
		return applyQRResult(entry, result, excelx.UnsupportedVerifyStatus)
	case invoiceinput.FileTypePDF:
		return preparePDFEntry(entry)
	case invoiceinput.FileTypeOFD:
		return prepareOFDEntry(entry)
	default:
		entry.IsSupported = false
		entry.ErrorMessage = excelx.UnsupportedVerifyStatus
		return entry
	}
}

func preparePDFEntry(entry invoiceinput.Entry) invoiceinput.Entry {
	embedded, err := pdfextract.ExtractEmbeddedRasterImages(entry.Path)
	if err != nil {
		entry.ErrorMessage = "PDF 读取失败: " + err.Error()
		return entry
	}
	if embedded.Status == pdfextract.StatusInvalid {
		entry.ExtractionStatus = embedded.Status
		entry.ErrorMessage = "PDF 读取失败: " + embedded.Reason
		return entry
	}
	if embedded.Status == pdfextract.StatusExtracted {
		prepared, ok, embeddedFailure := decodePDFImages(entry, embedded.Images)
		if ok {
			return prepared
		}
		entry = applyPDFDecodeFailure(entry, embeddedFailure)
	}

	rendered, err := pdfextract.RenderPages(entry.Path)
	if err != nil {
		entry.ExtractionStatus = pdfextract.StatusUnsupported
		entry.ErrorMessage = "PDF 渲染失败: " + err.Error()
		return entry
	}
	if rendered.Status == pdfextract.StatusInvalid {
		entry.ExtractionStatus = rendered.Status
		entry.ErrorMessage = "PDF 读取失败: " + rendered.Reason
		return entry
	}
	if rendered.Status == pdfextract.StatusUnsupported {
		entry.ExtractionStatus = rendered.Status
		entry.ErrorMessage = "PDF 无法渲染: " + rendered.Reason
		return entry
	}
	if rendered.Status == pdfextract.StatusRendered {
		prepared, ok, renderedFailure := decodePDFImages(entry, rendered.Images)
		if ok {
			return prepared
		}
		entry = applyPDFDecodeFailure(entry, renderedFailure)
	}
	if entry.ExtractionStatus == invoiceqr.StatusInvalidContent {
		return entry
	}
	entry.ExtractionStatus = rendered.Status
	entry.ErrorMessage = "PDF 页面未找到二维码"
	return entry
}

func decodePDFImages(entry invoiceinput.Entry, images []pdfextract.RasterImage) (invoiceinput.Entry, bool, invoiceqr.Result) {
	var failure invoiceqr.Result
	for _, image := range images {
		result := invoiceqr.DecodeInvoiceImageBytes(image.Data)
		if result.Status == invoiceqr.StatusSuccess {
			return applyQRResult(entry, result, ""), true, result
		}
		if result.Status == invoiceqr.StatusInvalidContent {
			failure = result
		}
	}
	return entry, false, failure
}

func applyPDFDecodeFailure(entry invoiceinput.Entry, result invoiceqr.Result) invoiceinput.Entry {
	if result.Status == invoiceqr.StatusInvalidContent {
		return applyQRResult(entry, result, excelx.UnsupportedVerifyStatus)
	}
	return entry
}

func prepareOFDEntry(entry invoiceinput.Entry) invoiceinput.Entry {
	result, err := ofd.ExtractInvoiceFields(entry.Path)
	if err != nil {
		entry.ErrorMessage = "OFD 读取失败: " + err.Error()
		return entry
	}
	entry.ExtractionStatus = result.Status
	if result.Status == ofd.StatusExtracted {
		entry.QRSuccess = true
		entry.QRData = invoiceinput.QRData{
			InvoiceNumber:   result.Fields.InvoiceNumber,
			InvoiceTypeCode: result.Fields.InvoiceTypeCode,
			InvoiceCode:     result.Fields.InvoiceCode,
			InvoiceDate:     result.Fields.InvoiceDate,
			CheckCode:       result.Fields.CheckCode,
			TotalAmount:     result.Fields.TotalAmount,
		}
		return entry
	}

	images, err := ofd.ExtractResourceImages(entry.Path)
	if err != nil {
		entry.ErrorMessage = "OFD 图片资源读取失败: " + err.Error()
		return entry
	}
	var invalidContent invoiceqr.Result
	for _, image := range images {
		decoded := invoiceqr.DecodeInvoiceImageBytes(image.Data)
		if decoded.Status == invoiceqr.StatusSuccess {
			prepared := applyQRResult(entry, decoded, "")
			prepared.ExtractionStatus = "ofd_resource_qr"
			return prepared
		}
		if decoded.Status == invoiceqr.StatusInvalidContent {
			invalidContent = decoded
		}
	}
	if invalidContent.Status == invoiceqr.StatusInvalidContent {
		return applyQRResult(entry, invalidContent, excelx.UnsupportedVerifyStatus)
	}
	if len(images) > 0 {
		entry.ErrorMessage = "OFD 图片资源未找到二维码"
		return entry
	}
	entry.ErrorMessage = "OFD 暂不支持: " + result.Reason
	return entry
}

func applyQRResult(entry invoiceinput.Entry, result invoiceqr.Result, fallback string) invoiceinput.Entry {
	if result.Status == invoiceqr.StatusSuccess {
		entry.QRSuccess = true
		entry.QRData = result.Data
		entry.ErrorMessage = ""
		entry.ExtractionStatus = result.Status
		return entry
	}
	entry.QRSuccess = false
	entry.ExtractionStatus = result.Status
	message := result.ErrorMessage
	if strings.TrimSpace(message) == "" {
		message = fallback
	}
	entry.ErrorMessage = message
	return entry
}

func verifyStatusForUnverifiedEntry(entry invoiceinput.Entry) string {
	if isUnsupportedOrNoQRCodeEntry(entry) {
		return excelx.UnsupportedVerifyStatus
	}
	status := strings.TrimSpace(entry.ErrorMessage)
	if status == "" {
		return excelx.UnsupportedVerifyStatus
	}
	return status
}

func isUnsupportedOrNoQRCodeEntry(entry invoiceinput.Entry) bool {
	if !entry.IsSupported || entry.ExtractionStatus == invoiceqr.StatusNoQRCode {
		return true
	}
	return strings.Contains(entry.ErrorMessage, "未找到二维码")
}
