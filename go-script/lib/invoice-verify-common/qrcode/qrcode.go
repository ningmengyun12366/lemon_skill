package qrcode

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/makiuchi-d/gozxing"
	goqr "github.com/makiuchi-d/gozxing/qrcode"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
)

const (
	StatusSuccess        = "success"
	StatusNoQRCode       = "no_qr"
	StatusCorruptImage   = "corrupt_image"
	StatusUnsupported    = "unsupported_image"
	StatusInvalidContent = "invalid_content"
)

type Result struct {
	Status       string
	Text         string
	Data         invoiceinput.QRData
	ErrorMessage string
}

func DecodeInvoiceImageFile(path string) Result {
	ext := strings.ToLower(filepath.Ext(path))
	if !invoiceinput.ImageExtensions[ext] {
		return Result{Status: StatusUnsupported, ErrorMessage: fmt.Sprintf("不支持的图片格式: %s", ext)}
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return Result{Status: StatusCorruptImage, ErrorMessage: fmt.Sprintf("读取图片失败: %s", err)}
	}
	return DecodeInvoiceImageBytes(payload)
}

func DecodeInvoiceImageBytes(payload []byte) Result {
	img, _, err := image.Decode(bytes.NewReader(payload))
	if err != nil {
		return Result{Status: StatusCorruptImage, ErrorMessage: fmt.Sprintf("读取图片失败: %s", err)}
	}
	return DecodeInvoiceImage(img)
}

func DecodeInvoiceImage(img image.Image) Result {
	text, err := DecodeText(img)
	if err != nil {
		return Result{Status: StatusNoQRCode, ErrorMessage: "未在图片中找到二维码"}
	}
	data, err := ParseInvoiceContent(text)
	if err != nil {
		return Result{Status: StatusInvalidContent, Text: text, ErrorMessage: err.Error()}
	}
	return Result{Status: StatusSuccess, Text: text, Data: data}
}

func DecodeText(img image.Image) (string, error) {
	variants := []image.Image{img, rotateRight(img), rotateRight(rotateRight(img)), rotateLeft(img)}
	var lastErr error
	for _, candidate := range variants {
		text, err := decodeTextOnce(candidate)
		if err == nil {
			return text, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("not found")
	}
	return "", lastErr
}

func ParseInvoiceContent(text string) (invoiceinput.QRData, error) {
	parts := strings.Split(text, ",")
	if len(parts) < 7 {
		return invoiceinput.QRData{}, fmt.Errorf("二维码内容格式不正确，字段数不足: %s", text)
	}
	invoiceTypeCode := strings.TrimSpace(parts[1])
	invoiceCode := strings.TrimSpace(parts[2])
	invoiceNumber := strings.TrimSpace(parts[3])
	totalAmount := strings.TrimSpace(parts[4])
	dateRaw := strings.TrimSpace(parts[5])
	checkCode := strings.TrimSpace(parts[6])

	if invoiceNumber == "" {
		return invoiceinput.QRData{}, fmt.Errorf("二维码中发票号码为空")
	}
	if dateRaw == "" {
		return invoiceinput.QRData{}, fmt.Errorf("二维码中开票日期为空")
	}
	invoiceDate, err := normalizeInvoiceDate(dateRaw)
	if err != nil {
		return invoiceinput.QRData{}, err
	}
	return invoiceinput.QRData{
		InvoiceNumber:   invoiceNumber,
		InvoiceTypeCode: invoiceTypeCode,
		InvoiceCode:     invoiceCode,
		InvoiceDate:     invoiceDate,
		CheckCode:       checkCode,
		TotalAmount:     totalAmount,
	}, nil
}

func normalizeInvoiceDate(dateRaw string) (string, error) {
	if len(dateRaw) == 8 && allDigits(dateRaw) {
		return fmt.Sprintf("%s-%s-%s", dateRaw[:4], dateRaw[4:6], dateRaw[6:8]), nil
	}
	if len(dateRaw) == 10 && dateRaw[4] == '-' && dateRaw[7] == '-' {
		digits := dateRaw[:4] + dateRaw[5:7] + dateRaw[8:10]
		if allDigits(digits) {
			return dateRaw, nil
		}
	}
	return "", fmt.Errorf("开票日期格式不正确: %s", dateRaw)
}

func decodeTextOnce(img image.Image) (string, error) {
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	result, err := goqr.NewQRCodeReader().Decode(bitmap, nil)
	if err != nil {
		return "", err
	}
	return result.GetText(), nil
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func rotateRight(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dy(), bounds.Dx()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(bounds.Max.Y-y-1, x-bounds.Min.X, src.At(x, y))
		}
	}
	return dst
}

func rotateLeft(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dy(), bounds.Dx()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(y-bounds.Min.Y, bounds.Max.X-x-1, src.At(x, y))
		}
	}
	return dst
}
