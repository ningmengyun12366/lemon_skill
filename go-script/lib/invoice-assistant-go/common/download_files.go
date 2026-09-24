package common

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"invoice-assistant-skill/go-script/lib/skillupdate"
)

func ExecuteDownloadFiles(inputPath string) (map[string]any, error) {
	return executeDownloadFiles(inputPath, RuntimeVersionChecker())
}

func executeDownloadFiles(inputPath string, versionChecker skillupdate.Checker) (map[string]any, error) {
	payload, err := ReadJSONFile(inputPath)
	if err != nil {
		return nil, err
	}
	targetDir := strings.TrimSpace(StringValue(payload["target_dir"]))
	if targetDir == "" {
		return nil, fmt.Errorf("missing required target_dir")
	}
	filesRaw, ok := payload["files"].([]any)
	if !ok || len(filesRaw) == 0 {
		return nil, fmt.Errorf("missing required files")
	}
	workspace := NormalizeWorkspaceDir(StringValue(payload["workspace_dir"]))
	absWorkspace, _ := filepath.Abs(workspace)
	outputRoot := absWorkspace
	if filepath.Base(outputRoot) != "invoice" {
		outputRoot = filepath.Join(outputRoot, "invoice")
	}
	if filepath.IsAbs(targetDir) {
		return nil, fmt.Errorf("target_dir must be relative")
	}
	destination := filepath.Clean(filepath.Join(outputRoot, targetDir))
	if rel, err := filepath.Rel(outputRoot, destination); err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("target_dir is outside invoice root")
	}
	jobs := []DownloadJob{}
	for i, raw := range filesRaw {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("files[%d] must be object", i)
		}
		rawURL := strings.TrimSpace(StringValue(item["url"]))
		if rawURL == "" {
			return nil, fmt.Errorf("files[%d].url is required", i)
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("files[%d].url only supports http/https", i)
		}
		filename := strings.TrimSpace(StringValue(item["filename"]))
		jobs = append(jobs, DownloadJob{URL: rawURL, FallbackName: "downloaded_file", TargetFilename: filename})
	}
	if versionChecker != nil {
		if err := versionChecker.Check(context.Background()); err != nil {
			return nil, err
		}
	}
	return DownloadMany(jobs, destination, "downloaded_file", func(name string) string {
		return SanitizeFilename(name, "downloaded_file")
	}, 0, "url"), nil
}
