package common

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const UserAgent = "invoice-assistant-skill/1.0"

type DownloadJob struct {
	URL            string
	FallbackName   string
	TargetFilename string
}

func SanitizeFilename(filename, fallback string) string {
	var b strings.Builder
	for _, ch := range filename {
		if strings.ContainsRune(`<>:"/\|?*`, ch) {
			b.WriteRune('_')
		} else {
			b.WriteRune(ch)
		}
	}
	out := strings.TrimRight(strings.TrimSpace(b.String()), ".")
	if out == "" {
		return fallback
	}
	return out
}

func FilenameFromURL(rawURL, fallback string, sanitizer func(string) string) string {
	parsed, err := url.Parse(rawURL)
	candidate := fallback
	if err == nil {
		if name := path.Base(parsed.EscapedPath()); name != "." && name != "/" {
			if decoded, decodeErr := url.PathUnescape(name); decodeErr == nil && decoded != "" {
				candidate = decoded
			}
		}
	}
	if sanitizer != nil {
		return sanitizer(candidate)
	}
	return SanitizeFilename(candidate, fallback)
}

func UniqueOutputPath(directory, filename string) string {
	candidate := filepath.Join(directory, filename)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(filename)
	stem := strings.TrimSuffix(filename, ext)
	for i := 1; ; i++ {
		candidate = filepath.Join(directory, fmt.Sprintf("%s_%d%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func DownloadMany(jobs []DownloadJob, destinationDir, fallbackPrefix string, sanitizer func(string) string, timeout time.Duration, resultURLKey string) map[string]any {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if resultURLKey == "" {
		resultURLKey = "url"
	}
	_ = os.MkdirAll(destinationDir, 0o755)
	successes := []any{}
	failures := []any{}
	for idx, job := range jobs {
		if strings.TrimSpace(job.URL) == "" {
			continue
		}
		fallback := job.FallbackName
		if fallback == "" {
			fallback = fmt.Sprintf("%s_%d", fallbackPrefix, idx+1)
		}
		result, err := downloadOne(job, destinationDir, fallback, sanitizer, timeout, resultURLKey)
		if err != nil {
			failures = append(failures, map[string]any{resultURLKey: job.URL, "status": "failed", "error": err.Error(), "target_filename": job.TargetFilename})
			continue
		}
		successes = append(successes, result)
	}
	status := "success"
	if len(successes) > 0 && len(failures) > 0 {
		status = "partial_success"
	} else if len(successes) == 0 && len(failures) > 0 {
		status = "failed"
	}
	return map[string]any{
		"status":        status,
		"target_dir":    destinationDir,
		"total_count":   len(jobs),
		"success_count": len(successes),
		"failed_count":  len(failures),
		"successes":     successes,
		"failures":      failures,
	}
}

func downloadOne(job DownloadJob, destinationDir, fallback string, sanitizer func(string) string, timeout time.Duration, resultURLKey string) (map[string]any, error) {
	filename := job.TargetFilename
	if filename == "" {
		filename = FilenameFromURL(job.URL, fallback, sanitizer)
	}
	if sanitizer != nil {
		filename = sanitizer(filename)
	}
	outputPath := UniqueOutputPath(destinationDir, filename)
	req, err := http.NewRequest(http.MethodGet, job.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return nil, err
	}
	return map[string]any{
		resultURLKey: job.URL,
		"saved_path": outputPath,
		"filename":   filepath.Base(outputPath),
		"status":     "success",
	}, nil
}
