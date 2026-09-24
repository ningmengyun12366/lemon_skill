package reportpdf

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
)

const PDFOutputPlaceholder = "{PDF_OUTPUT_FILE}"

type Options struct {
	InputTextFile string
	Output        string
}

func Run(options Options) (contract.ReportPDFSuccess, error) {
	inputPath, err := resolveInputTextFile(options.InputTextFile)
	if err != nil {
		return contract.ReportPDFSuccess{}, err
	}
	outputPath, err := pathx.ResolveRequiredFile(options.Output, ".pdf")
	if err != nil {
		return contract.ReportPDFSuccess{}, err
	}

	templateText, err := readUTF8ReportText(inputPath)
	if err != nil {
		return contract.ReportPDFSuccess{}, err
	}
	if isBlankReportText(templateText) {
		return contract.ReportPDFSuccess{}, errors.New("PDF生成失败：输入文本为空")
	}

	finalReportText := strings.ReplaceAll(templateText, PDFOutputPlaceholder, outputPath)
	if err := RenderFile(finalReportText, outputPath); err != nil {
		return contract.ReportPDFSuccess{}, err
	}

	return contract.ReportPDFSuccess{
		PDFOutputFile:   outputPath,
		FinalReportText: finalReportText,
	}, nil
}

func RenderFile(reportText, outputPath string) error {
	if isBlankReportText(reportText) {
		return errors.New("PDF生成失败：输入文本为空")
	}
	resolvedOutput, err := pathx.ResolveRequiredFile(outputPath, ".pdf")
	if err != nil {
		return err
	}
	if err := requireExistingParent(resolvedOutput); err != nil {
		return err
	}

	fonts, err := loadBundledFonts()
	if err != nil {
		return err
	}
	pdfBytes, err := renderPDFBytes(reportText, fonts)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) || len(pdfBytes) == 0 {
		return errors.New("PDF生成失败：输出内容无效")
	}
	if err := os.WriteFile(resolvedOutput, pdfBytes, 0o600); err != nil {
		return fmt.Errorf("PDF生成失败：%s：%w", resolvedOutput, err)
	}
	return nil
}

func isBlankReportText(text string) bool {
	return strings.TrimFunc(text, func(r rune) bool {
		return r == '\uFEFF' || unicode.IsSpace(r)
	}) == ""
}

func resolveInputTextFile(raw string) (string, error) {
	path, err := pathx.Normalize(raw)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("input-text-file file does not exist: %s", path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("input-text-file must be a file: %s", path)
	}
	if !strings.EqualFold(filepath.Ext(path), ".txt") {
		return "", fmt.Errorf("input-text-file must use .txt suffix: %s", path)
	}
	return path, nil
}

func readUTF8ReportText(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("PDF生成失败：%s：%w", path, err)
	}
	if !utf8.Valid(payload) {
		return "", fmt.Errorf("PDF生成失败：%s：不是有效的 UTF-8 文本", path)
	}
	return string(payload), nil
}

func requireExistingParent(path string) error {
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("PDF生成失败：%s：父目录不存在", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("PDF生成失败：%s：父路径不是目录", path)
	}
	return nil
}
