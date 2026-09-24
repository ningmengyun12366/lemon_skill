package pdfextract

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

const DefaultRenderDPI = 200

var (
	maxPDFBytes         int64 = 50 * 1024 * 1024
	maxRenderedPages          = 20
	maxPagePixels             = 50_000_000
	maxTotalPixels            = 250_000_000
	maxRenderedPNGBytes       = 100 * 1024 * 1024
)

// RenderPages renders every PDF page sequentially using the go-pdfium
// WebAssembly backend and returns PNG bytes that can be fed into the existing
// QR image decoder. No external renderer or CGO dependency is required.
func RenderPages(path string) (Result, error) {
	return RenderPagesInDPI(path, DefaultRenderDPI)
}

func RenderPagesInDPI(path string, dpi int) (result Result, err error) {
	if dpi <= 0 {
		return Result{}, fmt.Errorf("PDF 渲染 DPI 必须大于 0")
	}

	payload, preflight, err := readPDFPayload(path)
	if err != nil {
		return Result{}, err
	}
	if preflight.Status != "" {
		return preflight, nil
	}

	pool, err := webassembly.Init(webassembly.Config{
		FSConfig: wazero.NewFSConfig(),
		MinIdle:  0,
		MaxIdle:  1,
		MaxTotal: 1,
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	})
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if closeErr := pool.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	instance, err := pool.GetInstance(30 * time.Second)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if closeErr := instance.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	document, err := instance.OpenDocument(&requests.OpenDocument{File: &payload})
	if err != nil {
		return Result{Status: StatusInvalid, Reason: "PDF 无法解析: " + err.Error()}, nil
	}
	defer func() {
		_, closeErr := instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: document.Document})
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	pageCount, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: document.Document})
	if err != nil {
		return Result{}, err
	}
	if pageCount.PageCount <= 0 {
		return Result{Status: StatusInvalid, Reason: "PDF 没有页面"}, nil
	}
	if pageCount.PageCount > maxRenderedPages {
		return Result{Status: StatusUnsupported, Reason: fmt.Sprintf("PDF 页数过多，超过 %d 页限制", maxRenderedPages)}, nil
	}

	images := make([]RasterImage, 0, pageCount.PageCount)
	totalPixels := 0
	totalPNGBytes := 0
	for pageIndex := 0; pageIndex < pageCount.PageCount; pageIndex++ {
		pagePixels, err := renderedPagePixels(instance, document.Document, pageIndex, dpi)
		if err != nil {
			return Result{}, err
		}
		if pagePixels > maxPagePixels {
			return Result{Status: StatusUnsupported, Reason: fmt.Sprintf("PDF 第 %d 页渲染尺寸过大", pageIndex+1)}, nil
		}
		totalPixels += pagePixels
		if totalPixels > maxTotalPixels {
			return Result{Status: StatusUnsupported, Reason: "PDF 总渲染尺寸过大"}, nil
		}

		imageData, width, height, renderErr := renderPage(instance, document.Document, pageIndex, dpi)
		if renderErr != nil {
			return Result{}, renderErr
		}
		totalPNGBytes += len(imageData)
		if totalPNGBytes > maxRenderedPNGBytes {
			return Result{Status: StatusUnsupported, Reason: fmt.Sprintf("PDF 渲染图片过大，超过 %d MB 限制", maxRenderedPNGBytes/(1024*1024))}, nil
		}
		images = append(images, RasterImage{
			Index:  pageIndex + 1,
			Ext:    ".png",
			Data:   imageData,
			Width:  width,
			Height: height,
		})
	}

	return Result{Status: StatusRendered, Images: images}, nil
}

func renderedPagePixels(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, pageIndex, dpi int) (int, error) {
	size, err := instance.GetPageSizeInPixels(&requests.GetPageSizeInPixels{
		DPI: dpi,
		Page: requests.Page{ByIndex: &requests.PageByIndex{
			Document: document,
			Index:    pageIndex,
		}},
	})
	if err != nil {
		return 0, fmt.Errorf("读取 PDF 第 %d 页尺寸失败: %w", pageIndex+1, err)
	}
	if size.Width <= 0 || size.Height <= 0 {
		return 0, fmt.Errorf("读取 PDF 第 %d 页尺寸失败: 页面尺寸无效", pageIndex+1)
	}
	return size.Width * size.Height, nil
}

func renderPage(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, pageIndex, dpi int) ([]byte, int, int, error) {
	renderedPage, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
		DPI: dpi,
		Page: requests.Page{ByIndex: &requests.PageByIndex{
			Document: document,
			Index:    pageIndex,
		}},
	})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("渲染 PDF 第 %d 页失败: %w", pageIndex+1, err)
	}
	defer renderedPage.Cleanup()

	if renderedPage.Result.Image == nil {
		return nil, 0, 0, fmt.Errorf("渲染 PDF 第 %d 页失败: 未返回图片", pageIndex+1)
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, renderedPage.Result.Image); err != nil {
		return nil, 0, 0, fmt.Errorf("编码 PDF 第 %d 页图片失败: %w", pageIndex+1, err)
	}
	return append([]byte(nil), buffer.Bytes()...), renderedPage.Result.Width, renderedPage.Result.Height, nil
}
