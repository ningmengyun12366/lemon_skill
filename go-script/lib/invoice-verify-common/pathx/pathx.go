package pathx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const TimestampLayout = "20060102150405"

func Normalize(path string) (string, error) {
	expanded, err := ExpandUser(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	if expanded == "" {
		expanded = "."
	}
	absolute, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func ExpandUser(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func ResolveOutput(rawOutput, defaultDir, defaultFilename, requiredExt string) (string, error) {
	if strings.TrimSpace(defaultFilename) == "" {
		return "", errors.New("default filename is required")
	}
	if strings.TrimSpace(defaultDir) == "" {
		defaultDir = "."
	}
	requiredExt = normalizeExt(requiredExt)

	text := strings.TrimSpace(rawOutput)
	if text == "" {
		return Normalize(filepath.Join(defaultDir, defaultFilename))
	}

	candidate, err := ExpandUser(text)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
		return Normalize(filepath.Join(candidate, defaultFilename))
	}
	if requiredExt != "" && strings.EqualFold(filepath.Ext(candidate), requiredExt) {
		return Normalize(candidate)
	}
	return Normalize(filepath.Join(candidate, defaultFilename))
}

func ResolveRequiredFile(rawOutput, requiredExt string) (string, error) {
	text := strings.TrimSpace(rawOutput)
	if text == "" {
		return "", errors.New("output file is required")
	}
	requiredExt = normalizeExt(requiredExt)
	path, err := Normalize(text)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		return "", fmt.Errorf("output must be a %s file: %s", requiredExt, path)
	}
	if requiredExt != "" && !strings.EqualFold(filepath.Ext(path), requiredExt) {
		return "", fmt.Errorf("output must use %s suffix: %s", requiredExt, path)
	}
	return path, nil
}

func BuildDatedFilename(prefix, ext string, now time.Time) string {
	ext = normalizeExt(ext)
	return prefix + now.Format(TimestampLayout) + ext
}

func WithCollisionSuffix(path string, now time.Time) (string, error) {
	normalized, err := Normalize(path)
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(normalized); errors.Is(statErr, os.ErrNotExist) {
		return normalized, nil
	} else if statErr != nil {
		return "", statErr
	}

	dir := filepath.Dir(normalized)
	ext := filepath.Ext(normalized)
	base := strings.TrimSuffix(filepath.Base(normalized), ext)
	stamp := now.Format(TimestampLayout)
	for i := 1; i < 1000; i++ {
		suffix := stamp
		if i > 1 {
			suffix = fmt.Sprintf("%s_%d", stamp, i)
		}
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%s%s", base, suffix, ext))
		if _, statErr := os.Stat(candidate); errors.Is(statErr, os.ErrNotExist) {
			return candidate, nil
		} else if statErr != nil {
			return "", statErr
		}
	}
	return "", fmt.Errorf("could not find collision-free path for %s", normalized)
}

func EnsureParent(path string) error {
	normalized, err := Normalize(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(filepath.Dir(normalized), 0o755)
}

func normalizeExt(ext string) string {
	ext = strings.TrimSpace(ext)
	if ext == "" || strings.HasPrefix(ext, ".") {
		return ext
	}
	return "." + ext
}
