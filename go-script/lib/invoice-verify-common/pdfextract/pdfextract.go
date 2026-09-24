package pdfextract

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"regexp"
	"strconv"
)

const (
	StatusExtracted   = "extracted"
	StatusRendered    = "rendered"
	StatusUnsupported = "unsupported"
	StatusInvalid     = "invalid_pdf"
)

type RasterImage struct {
	Index  int
	Ext    string
	Data   []byte
	Width  int
	Height int
}

type Result struct {
	Status string
	Images []RasterImage
	Reason string
}

func readPDFPayload(path string) ([]byte, Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, Result{}, err
	}
	if info.Size() > maxPDFBytes {
		return nil, Result{Status: StatusUnsupported, Reason: fmt.Sprintf("PDF 文件过大，超过 %d MB 限制", maxPDFBytes/(1024*1024))}, nil
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, Result{}, err
	}
	if !bytes.HasPrefix(bytes.TrimSpace(payload), []byte("%PDF")) {
		return nil, Result{Status: StatusInvalid, Reason: "文件不是 PDF"}, nil
	}
	return payload, Result{}, nil
}

func ExtractEmbeddedRasterImages(path string) (Result, error) {
	payload, preflight, err := readPDFPayload(path)
	if err != nil {
		return Result{}, err
	}
	if preflight.Status != "" {
		return preflight, nil
	}

	blocks := imageStreams(payload)
	images := make([]RasterImage, 0, len(blocks))
	unsupportedImageFound := false
	for _, block := range blocks {
		if !bytes.Contains(block.dictionary, []byte("/Subtype")) || !bytes.Contains(block.dictionary, []byte("/Image")) {
			continue
		}
		width := dictionaryInt(block.dictionary, "Width")
		height := dictionaryInt(block.dictionary, "Height")
		switch {
		case hasDictionaryName(block.dictionary, "DCTDecode"):
			if len(block.data) == 0 {
				unsupportedImageFound = true
				continue
			}
			actualWidth, actualHeight, err := jpegDimensions(block.data)
			if err != nil || !pixelsWithinLimit(actualWidth, actualHeight) {
				unsupportedImageFound = true
				continue
			}
			if width <= 0 || height <= 0 {
				width, height = actualWidth, actualHeight
			}
			if !pixelsWithinLimit(width, height) {
				unsupportedImageFound = true
				continue
			}
			images = append(images, RasterImage{Index: len(images) + 1, Ext: ".jpg", Data: append([]byte(nil), block.data...), Width: width, Height: height})
		case hasDictionaryName(block.dictionary, "FlateDecode"):
			img, err := flateImage(block.dictionary, block.data)
			if err != nil {
				unsupportedImageFound = true
				continue
			}
			var buffer bytes.Buffer
			if err := png.Encode(&buffer, img); err != nil {
				return Result{}, err
			}
			images = append(images, RasterImage{Index: len(images) + 1, Ext: ".png", Data: buffer.Bytes(), Width: img.Bounds().Dx(), Height: img.Bounds().Dy()})
		default:
			unsupportedImageFound = true
		}
	}

	if len(images) > 0 {
		return Result{Status: StatusExtracted, Images: images}, nil
	}
	if unsupportedImageFound {
		return Result{Status: StatusUnsupported, Reason: "PDF 内嵌图片压缩格式需要页面渲染处理"}, nil
	}
	return Result{Status: StatusUnsupported, Reason: "PDF 页面需要栅格化或仅包含矢量内容"}, nil
}

type streamBlock struct {
	dictionary []byte
	data       []byte
}

func imageStreams(payload []byte) []streamBlock {
	var blocks []streamBlock
	position := 0
	for {
		streamRelative := bytes.Index(payload[position:], []byte("stream"))
		if streamRelative < 0 {
			break
		}
		streamIndex := position + streamRelative
		dictClose := bytes.LastIndex(payload[:streamIndex], []byte(">>"))
		dictOpen := bytes.LastIndex(payload[:streamIndex], []byte("<<"))
		if dictOpen < 0 || dictClose < dictOpen {
			position = streamIndex + len("stream")
			continue
		}
		dictionary := payload[dictOpen : dictClose+2]
		dataStart := streamIndex + len("stream")
		if dataStart < len(payload) && payload[dataStart] == '\r' {
			dataStart++
			if dataStart < len(payload) && payload[dataStart] == '\n' {
				dataStart++
			}
		} else if dataStart < len(payload) && payload[dataStart] == '\n' {
			dataStart++
		}

		length := dictionaryInt(dictionary, "Length")
		dataEnd := -1
		if length >= 0 && dataStart+length <= len(payload) {
			dataEnd = dataStart + length
		} else if endRelative := bytes.Index(payload[dataStart:], []byte("endstream")); endRelative >= 0 {
			dataEnd = dataStart + endRelative
		}
		if dataEnd < dataStart {
			position = dataStart
			continue
		}
		data := bytes.TrimRight(payload[dataStart:dataEnd], "\r\n")
		blocks = append(blocks, streamBlock{dictionary: dictionary, data: data})
		position = dataEnd
	}
	return blocks
}

func dictionaryInt(dictionary []byte, name string) int {
	expression := regexp.MustCompile(`/` + regexp.QuoteMeta(name) + `\s+(\d+)`)
	matches := expression.FindSubmatch(dictionary)
	if len(matches) != 2 {
		return -1
	}
	value, err := strconv.Atoi(string(matches[1]))
	if err != nil {
		return -1
	}
	return value
}

func hasDictionaryName(dictionary []byte, name string) bool {
	return bytes.Contains(dictionary, []byte("/"+name))
}

func flateImage(dictionary []byte, data []byte) (image.Image, error) {
	width := dictionaryInt(dictionary, "Width")
	height := dictionaryInt(dictionary, "Height")
	bits := dictionaryInt(dictionary, "BitsPerComponent")
	if width <= 0 || height <= 0 || bits != 8 {
		return nil, fmt.Errorf("unsupported image dimensions")
	}
	if !pixelsWithinLimit(width, height) {
		return nil, fmt.Errorf("image dimensions too large")
	}
	pixels := width * height
	components := 0
	switch {
	case hasDictionaryName(dictionary, "DeviceRGB"):
		components = 3
	case hasDictionaryName(dictionary, "DeviceGray"):
		components = 1
	default:
		return nil, fmt.Errorf("unsupported color space")
	}
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	needed := pixels * components
	decoded, err := io.ReadAll(io.LimitReader(reader, int64(needed)+1))
	if closeErr := reader.Close(); closeErr != nil {
		return nil, closeErr
	}
	if err != nil {
		return nil, err
	}
	if len(decoded) > needed {
		return nil, fmt.Errorf("image stream too large")
	}
	if len(decoded) < needed {
		return nil, fmt.Errorf("image stream too short")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * components
			if components == 1 {
				gray := decoded[offset]
				img.SetRGBA(x, y, color.RGBA{R: gray, G: gray, B: gray, A: 255})
				continue
			}
			img.SetRGBA(x, y, color.RGBA{R: decoded[offset], G: decoded[offset+1], B: decoded[offset+2], A: 255})
		}
	}
	return img, nil
}

func jpegDimensions(data []byte) (int, int, error) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	return config.Width, config.Height, nil
}

func pixelsWithinLimit(width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	if width > maxPagePixels/height {
		return false
	}
	return width*height <= maxPagePixels
}
