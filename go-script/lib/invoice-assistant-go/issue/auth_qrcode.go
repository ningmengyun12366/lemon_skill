package issue

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/makiuchi-d/gozxing"
	goqr "github.com/makiuchi-d/gozxing/qrcode"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	localQRCodeSize     = 320
	qrcodeImagePathKey  = "qrcode_image_path"
	qrcodeImageFileMode = 0o600
)

var qrcodeTypeLabels = map[string]string{
	"0": "电子税局APP",
	"1": "国家网络身份认证APP",
}

var scanStatusLabels = map[string]string{
	"1": "等待扫描",
	"2": "扫码成功",
	"3": "二维码失效",
}

var authStatusLabels = map[string]string{
	"-1": "认证失败",
	"0":  "处理中",
	"1":  "认证成功",
	"2":  "处理中",
}

func normalizeQRCodeEntries(resp map[string]any) []map[string]string {
	list, ok := asList(GetPath(resp, "data.slrzxxList"))
	if !ok {
		return nil
	}
	out := []map[string]string{}
	for _, itemAny := range list {
		item, _ := itemAny.(map[string]any)
		entry := normalizeQRCodeEntry(item)
		if entry["rzid"] != "" && entry["qrcodeType"] != "" {
			out = append(out, entry)
		}
	}
	return out
}

func normalizeQRCodeEntry(item map[string]any) map[string]string {
	qrContent := strings.TrimSpace(common.StringValue(firstNonNil(
		item["content"],
		item["qrcodeUrl"],
		item["qrCodeUrl"],
		item["url"],
		item["qrcode"],
	)))
	qrType := strings.TrimSpace(common.StringValue(firstNonNil(item["qrcodeType"], item["type"], item["userType"])))
	rzid := strings.TrimSpace(common.StringValue(item["rzid"]))
	label := qrcodeTypeLabels[qrType]
	if label == "" {
		label = strings.TrimSpace(common.StringValue(firstNonNil(item["qrcodeTypeLabel"], item["label"], item["name"])))
	}
	if label == "" && qrType != "" {
		label = "未知类型(" + qrType + ")"
	}
	return map[string]string{
		"rzid":             rzid,
		"qrcodeType":       qrType,
		"qrcodeTypeLabel":  label,
		"type":             qrType,
		"label":            label,
		"content":          qrContent,
		"url":              qrContent,
		qrcodeImagePathKey: strings.TrimSpace(common.StringValue(item[qrcodeImagePathKey])),
	}
}

/**
 * buildLocalQRCodePNG 在内存中生成具有明确白色背景的本地二维码 PNG。
 * @param content 税局返回的二维码原始内容。
 * @returns 可直接写入本地文件的 PNG 字节；编码失败时返回错误。
 */
func buildLocalQRCodePNG(content string) ([]byte, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("二维码内容为空")
	}
	matrix, err := goqr.NewQRCodeWriter().Encode(
		content,
		gozxing.BarcodeFormat_QR_CODE,
		localQRCodeSize,
		localQRCodeSize,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("生成本地二维码失败: %w", err)
	}

	// 不直接把 BitMatrix 交给 PNG 编码器，显式绘制白底可避免不同图片查看器出现黑色背景或异常横幅。
	raster := image.NewGray(matrix.Bounds())
	for index := range raster.Pix {
		raster.Pix[index] = 0xff
	}
	for y := matrix.Bounds().Min.Y; y < matrix.Bounds().Max.Y; y++ {
		for x := matrix.Bounds().Min.X; x < matrix.Bounds().Max.X; x++ {
			if matrix.Get(x, y) {
				raster.Pix[raster.PixOffset(x, y)] = 0x00
			}
		}
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, raster); err != nil {
		return nil, fmt.Errorf("编码本地二维码 PNG 失败: %w", err)
	}
	return buffer.Bytes(), nil
}

/**
 * persistLocalQRCodeImages 将认证二维码保存到认证 Markdown 同目录并回填绝对路径。
 * @param markdownPath 本次认证 Markdown 的目标路径。
 * @param entries 已规范化的二维码条目。
 * @returns 所有二维码均成功写入本地文件时返回 nil，否则返回明确错误。
 */
func persistLocalQRCodeImages(markdownPath string, entries []map[string]string) error {
	absoluteMarkdownPath, err := filepath.Abs(strings.TrimSpace(markdownPath))
	if err != nil {
		return fmt.Errorf("解析二维码 Markdown 路径失败: %w", err)
	}
	imageDir := filepath.Dir(absoluteMarkdownPath)
	if err := ensureWritableDir(imageDir); err != nil {
		return err
	}
	filePrefix := strings.TrimSuffix(filepath.Base(absoluteMarkdownPath), filepath.Ext(absoluteMarkdownPath))
	for index, entry := range entries {
		content := strings.TrimSpace(firstString(entry["content"], entry["url"]))
		payload, err := buildLocalQRCodePNG(content)
		if err != nil {
			return fmt.Errorf("生成第 %d 个认证二维码失败: %w", index+1, err)
		}
		qrType := sanitizeSegment(firstString(entry["qrcodeType"], entry["type"]))
		imagePath := filepath.Join(imageDir, fmt.Sprintf("%s-%d-%s.png", filePrefix, index+1, qrType))
		if err := os.WriteFile(imagePath, payload, qrcodeImageFileMode); err != nil {
			return fmt.Errorf("保存第 %d 个认证二维码失败: %w", index+1, err)
		}
		entry[qrcodeImagePathKey] = imagePath
		delete(entry, "qrcodeLink")
		delete(entry, "render_url")
	}
	return nil
}

func normalizeScanStatusEntries(resp map[string]any) []map[string]string {
	list, ok := asList(GetPath(resp, "data.qrCodeStatusList"))
	if !ok {
		return nil
	}
	out := []map[string]string{}
	for _, itemAny := range list {
		item, _ := itemAny.(map[string]any)
		entry := normalizeScanStatusEntry(item)
		if entry["rzid"] != "" && entry["status"] != "" && entry["qrcodeType"] != "" {
			out = append(out, entry)
		}
	}
	return out
}

func normalizeScanStatusEntry(item map[string]any) map[string]string {
	qrType := strings.TrimSpace(common.StringValue(firstNonNil(item["qrcodeType"], item["type"], item["userType"])))
	status := strings.TrimSpace(common.StringValue(firstNonNil(item["status"], item["scanStatus"], item["qrCodeStatus"])))
	rzid := strings.TrimSpace(common.StringValue(item["rzid"]))
	label := qrcodeTypeLabels[qrType]
	if label == "" {
		label = strings.TrimSpace(common.StringValue(firstNonNil(item["qrcodeTypeLabel"], item["label"], item["name"])))
	}
	if label == "" && qrType != "" {
		label = "未知类型(" + qrType + ")"
	}
	statusLabel := scanStatusLabels[status]
	if statusLabel == "" && status != "" {
		statusLabel = "未知状态(" + status + ")"
	}
	return map[string]string{
		"rzid":            rzid,
		"qrcodeType":      qrType,
		"qrcodeTypeLabel": label,
		"type":            qrType,
		"label":           label,
		"status":          status,
		"statusLabel":     statusLabel,
		"status_label":    statusLabel,
	}
}

func hasSuccessfulScan(entries []map[string]string) bool {
	for _, entry := range entries {
		if entry["status"] == "2" {
			return true
		}
	}
	return false
}

func storedSuccessfulScanStatusEntries(authResult map[string]any) []map[string]string {
	for _, key := range []string{"scan_status_list", "scan_status_entries"} {
		if entries, ok := authResult[key].([]map[string]string); ok && hasSuccessfulScan(entries) {
			return entries
		}
		list, ok := asList(authResult[key])
		if !ok {
			continue
		}
		entries := make([]map[string]string, 0, len(list))
		for _, itemAny := range list {
			item, _ := itemAny.(map[string]any)
			entry := normalizeScanStatusEntry(item)
			if entry["rzid"] != "" && entry["qrcodeType"] != "" && entry["status"] != "" {
				entries = append(entries, entry)
			}
		}
		if hasSuccessfulScan(entries) {
			return entries
		}
	}
	return nil
}

func buildVerifyStatus(entries []map[string]string, authResult map[string]any) string {
	if hasSuccessfulScan(entries) {
		authFinished := legacyPythonTruthy(GetPath(authResult, "data.isAuth"))
		if authResult != nil && issueDataStatusEquals(authResult, 1) && authFinished && responseCode(authResult) == 0 {
			return "auth_passed"
		}
		if authResult != nil && issueDataStatusEquals(authResult, -1) {
			return "auth_failed"
		}
		if authResult != nil {
			return "auth_pending"
		}
		return "scan_success"
	}
	for _, entry := range entries {
		if entry["status"] == "3" {
			return "qrcode_expired"
		}
	}
	return "awaiting_scan"
}

func buildCheckPayload(authResult map[string]any, context map[string]any) (map[string]any, error) {
	payload := map[string]any{}
	for k, v := range context {
		payload[k] = v
	}
	entries := qrcodeEntriesFromAuthResult(authResult)
	if len(entries) == 0 {
		_, hasQRCodeList := authResult["qrcode_list"]
		if !hasQRCodeList {
			_, hasQRCodeList = authResult["qrcode_entries"]
		}
		if hasQRCodeList {
			return nil, fmt.Errorf("当前任务中的二维码列表无有效 rzid 或 qrcodeType")
		}
		return nil, fmt.Errorf("当前任务尚未生成二维码，请先执行 start 子命令")
	}
	qrCodeList := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		rzid := strings.TrimSpace(entry["rzid"])
		qrType := strings.TrimSpace(firstString(entry["qrcodeType"], entry["type"]))
		if rzid != "" && qrType != "" {
			qrCodeList = append(qrCodeList, map[string]string{"rzid": rzid, "qrcodeType": qrType})
		}
	}
	if len(qrCodeList) > 0 {
		payload["qrCodeList"] = qrCodeList
	}
	if len(qrCodeList) == 0 {
		return nil, fmt.Errorf("当前任务中的二维码列表无有效 rzid 或 qrcodeType")
	}
	return payload, nil
}

func qrcodeEntriesFromAuthResult(authResult map[string]any) []map[string]string {
	if entries, ok := authResult["qrcode_list"].([]map[string]string); ok {
		return entries
	}
	if entries, ok := authResult["qrcode_entries"].([]map[string]string); ok {
		return entries
	}
	for _, key := range []string{"qrcode_list", "qrcode_entries"} {
		if list, ok := asList(authResult[key]); ok {
			out := make([]map[string]string, 0, len(list))
			for _, itemAny := range list {
				item, _ := itemAny.(map[string]any)
				entry := normalizeQRCodeEntry(item)
				if entry["rzid"] != "" && entry["qrcodeType"] != "" {
					out = append(out, entry)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return nil
}
