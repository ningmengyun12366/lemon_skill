package report

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const workspaceRenderResultFilename = "render-result.json"

type renderReceipt struct {
	Fingerprint string `json:"fingerprint"`
	DOCXSHA256  string `json:"docxSHA256"`
	PDFSHA256   string `json:"pdfSHA256"`
	Result      Result `json:"result"`
}

// renderFingerprint includes the executable (rendering code and pinned asset
// versions), frozen input, narrative, output directory and explicit output date.
// Bundled fonts, cover and renderer are still validated on a cache hit.
func renderFingerprint(validation ValidationResult, options GenerateOptions) (string, error) {
	fonts, err := LoadFonts(options.FontDir)
	if err != nil {
		return "", err
	}
	if _, err = LoadReportAssets(fonts.Directory); err != nil {
		return "", err
	}
	runtime, err := ResolveChartRendererPath(fonts.Directory, options.ChartRendererPath)
	if err != nil {
		return "", err
	}
	if _, err = validateChartRendererBundle(runtime); err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	code, err := os.ReadFile(exe)
	if err != nil {
		return "", err
	}
	input := []string{RuleVersion, AIContractVersion, digestBytes(code), validation.SourceDigest, validation.ModelDigest, validation.AnalysisDataDigest, validation.AnalysisDigest, options.OutputDir, options.Now.String()}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return digestBytes(payload), nil
}

func reusableRenderReceipt(path, fingerprint, outputDir string) (Result, bool) {
	r, _, err := loadWorkspaceJSON[renderReceipt](path)
	if err != nil || r.Fingerprint != fingerprint || r.Result.Status != "success" {
		return Result{}, false
	}
	for _, file := range []struct{ path, digest string }{{r.Result.DOCXPath, r.DOCXSHA256}, {r.Result.PDFPath, r.PDFSHA256}} {
		if filepath.Dir(file.path) != outputDir {
			return Result{}, false
		}
		payload, err := os.ReadFile(file.path)
		if err != nil || digestBytes(payload) != file.digest {
			return Result{}, false
		}
	}
	r.Result.Reused = true
	return r.Result, true
}

func saveRenderReceipt(path, fingerprint string, result Result) error {
	docx, err := os.ReadFile(result.DOCXPath)
	if err != nil {
		return err
	}
	pdf, err := os.ReadFile(result.PDFPath)
	if err != nil {
		return err
	}
	return writeWorkspaceJSON(path, renderReceipt{fingerprint, digestBytes(docx), digestBytes(pdf), result})
}
