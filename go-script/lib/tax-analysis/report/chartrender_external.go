package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type chartRendererPayload struct {
	Kind           string                       `json:"kind"`
	Title          string                       `json:"title"`
	Labels         []string                     `json:"labels"`
	Series         []chartRendererSeriesPayload `json:"series"`
	Points         []ChartPoint                 `json:"points,omitempty"`
	Unit           string                       `json:"unit,omitempty"`
	SecondaryUnit  string                       `json:"secondaryUnit,omitempty"`
	CenterLabel    string                       `json:"centerLabel,omitempty"`
	CenterValue    string                       `json:"centerValue,omitempty"`
	EmptyTitle     string                       `json:"emptyTitle,omitempty"`
	EmptyMessage   string                       `json:"emptyMessage,omitempty"`
	ReferenceLabel string                       `json:"referenceLabel,omitempty"`
}

type chartRendererSeriesPayload struct {
	Name      string    `json:"name"`
	Values    []float64 `json:"values"`
	Available []bool    `json:"available,omitempty"`
	Color     string    `json:"color"`
	Colors    []string  `json:"colors,omitempty"`
	Axis      string    `json:"axis,omitempty"`
	RenderAs  string    `json:"renderAs,omitempty"`
}

// ResolveChartRendererPath locates the packaged portable Node.js runtime next
// to the report assets. Parameters: fontDirectory is the verified report font
// directory and explicitPath optionally overrides the runtime path. Return
// value: an absolute, hash-verified runtime path. There is no PATH or system
// runtime fallback.
func ResolveChartRendererPath(fontDirectory, explicitPath string) (string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		return validateChartRendererPath(explicitPath)
	}
	if strings.TrimSpace(fontDirectory) == "" {
		return "", fmt.Errorf("报告生成失败：缺少组件库图表渲染器目录")
	}
	platform, err := chartRendererPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	assetDirectory := filepath.Dir(filepath.Clean(fontDirectory))
	name := chartRendererRuntimeBaseName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return validateChartRendererPath(filepath.Join(assetDirectory, chartRendererDirectory, chartRendererRuntimeDirectory, platform, name))
}

// chartRendererPlatform maps a Go target to the packaged runtime directory.
// Parameters: goos and goarch are Go platform identifiers. Return value: the
// stable asset platform or an unsupported-platform error.
func chartRendererPlatform(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "windows/amd64":
		return "windows-amd64", nil
	case "linux/amd64":
		return "linux-amd64", nil
	case "linux/arm64":
		return "linux-arm64", nil
	case "darwin/amd64":
		return "darwin-amd64", nil
	case "darwin/arm64":
		return "darwin-arm64", nil
	default:
		return "", fmt.Errorf("报告生成失败：当前平台没有组件库图表渲染器：%s/%s", goos, goarch)
	}
}

// validateChartRendererPath verifies the current platform's portable runtime.
// Parameters: path is the proposed runtime executable. Return value: its
// absolute path after size and SHA-256 verification.
func validateChartRendererPath(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	platform, platformErr := chartRendererPlatform(runtime.GOOS, runtime.GOARCH)
	if platformErr != nil {
		return "", platformErr
	}
	expected, ok := chartRendererRuntimeAssets[platform]
	if !ok {
		return "", fmt.Errorf("报告生成失败：缺少当前平台便携运行时清单：%s", platform)
	}
	if err := validateChartRendererAsset(absolute, expected, "便携 Node.js 运行时"); err != nil {
		return "", err
	}
	return absolute, nil
}

// validateChartRendererBundle verifies every executable renderer dependency.
// Parameters: runtimePath is the verified portable runtime path. Return value:
// the absolute renderer entry module path.
func validateChartRendererBundle(runtimePath string) (string, error) {
	bundleDirectory := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(runtimePath))))
	for relativePath, expected := range chartRendererBundleAssets {
		path := filepath.Join(bundleDirectory, filepath.FromSlash(relativePath))
		if err := validateChartRendererAsset(path, expected, "组件库图表渲染文件"); err != nil {
			return "", err
		}
	}
	return filepath.Join(bundleDirectory, chartRendererScriptName), nil
}

// validateChartRendererAsset verifies one pinned runtime or renderer file.
// Parameters: path is the local asset, expected contains its fixed size and
// SHA-256, and label is used in user-facing errors. Return value: nil when the
// asset exactly matches its manifest entry.
func validateChartRendererAsset(path string, expected chartRendererAsset, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("报告生成失败：未找到%s %s；请按安装文档重新安装：%w", label, path, err)
	}
	if !info.Mode().IsRegular() || info.Size() != expected.Size {
		return fmt.Errorf("报告生成失败：%s无效：%s", label, path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("报告生成失败：无法读取%s：%w", label, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("报告生成失败：校验%s失败：%w", label, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("报告生成失败：关闭%s失败：%w", label, closeErr)
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("报告生成失败：%s SHA-256 校验失败：%s", label, path)
	}
	return nil
}

// RenderCharts sends the common report chart model to the packaged JavaScript
// renderer through its private portable Node.js runtime. Parameters: ctx
// controls all renderer processes, model supplies chart data, fonts supplies
// verified Noto fonts, and rendererPath identifies the private runtime. Return
// value: one PNG byte slice per chart ID for shared DOCX/PDF embedding.
func RenderCharts(ctx context.Context, model Model, fonts FontFiles, rendererPath string) (map[string][]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	validatedRenderer, err := validateChartRendererPath(rendererPath)
	if err != nil {
		return nil, err
	}
	rendererScript, err := validateChartRendererBundle(validatedRenderer)
	if err != nil {
		return nil, err
	}
	temporaryDirectory, err := os.MkdirTemp("", "tax-analysis-chart-renderer-*")
	if err != nil {
		return nil, fmt.Errorf("create chart renderer workspace: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)

	result := map[string][]byte{}
	chartIndex := 0
	for _, section := range model.Sections {
		for _, block := range section.Blocks {
			if block.Chart == nil {
				continue
			}
			payload := rendererPayload(*block.Chart)
			input, marshalErr := json.Marshal(payload)
			if marshalErr != nil {
				return nil, fmt.Errorf("marshal chart %s: %w", block.Chart.ID, marshalErr)
			}
			outputName := fmt.Sprintf("chart-%04d.png", chartIndex)
			chartIndex++
			command := exec.CommandContext(ctx, validatedRenderer,
				rendererScript,
				"--regular-font", fonts.RegularPath,
				"--bold-font", fonts.BoldPath,
				"--output-file", outputName,
			)
			command.Dir = temporaryDirectory
			command.Stdin = bytes.NewReader(input)
			var stderr bytes.Buffer
			command.Stderr = &stderr
			command.Stdout = io.Discard
			if runErr := command.Run(); runErr != nil {
				detail := strings.TrimSpace(stderr.String())
				if detail != "" {
					return nil, fmt.Errorf("render chart %s: %w: %s", block.Chart.ID, runErr, detail)
				}
				return nil, fmt.Errorf("render chart %s: %w", block.Chart.ID, runErr)
			}
			outputPath := filepath.Join(temporaryDirectory, outputName)
			imageBytes, readErr := os.ReadFile(outputPath)
			if readErr != nil {
				return nil, fmt.Errorf("read chart %s output: %w", block.Chart.ID, readErr)
			}
			configuration, decodeErr := png.DecodeConfig(bytes.NewReader(imageBytes))
			if decodeErr != nil {
				return nil, fmt.Errorf("validate chart %s PNG: %w", block.Chart.ID, decodeErr)
			}
			if configuration.Width != chartPixelWidth || configuration.Height != chartPixelHeight {
				return nil, fmt.Errorf("validate chart %s PNG: expected %dx%d, got %dx%d", block.Chart.ID, chartPixelWidth, chartPixelHeight, configuration.Width, configuration.Height)
			}
			result[block.Chart.ID] = imageBytes
		}
	}
	return result, nil
}

func rendererPayload(chart Chart) chartRendererPayload {
	series := make([]chartRendererSeriesPayload, len(chart.Series))
	for index, item := range chart.Series {
		series[index] = chartRendererSeriesPayload{
			Name: item.Name, Values: append([]float64(nil), item.Values...), Available: append([]bool(nil), item.Available...), Color: item.Color, Colors: append([]string(nil), item.Colors...),
			Axis: item.Axis, RenderAs: item.RenderAs,
		}
	}
	return chartRendererPayload{
		Kind: chart.Kind, Title: chart.Title, Labels: append([]string(nil), chart.Labels...), Series: series,
		Points: append([]ChartPoint(nil), chart.Points...), Unit: chart.Unit, SecondaryUnit: chart.SecondaryUnit,
		CenterLabel: chart.CenterLabel, CenterValue: chart.CenterValue, EmptyTitle: chart.EmptyTitle, EmptyMessage: chart.EmptyMessage,
		ReferenceLabel: chart.ReferenceLabel,
	}
}
