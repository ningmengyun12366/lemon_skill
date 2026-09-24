package runtimeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const skillManifestFilename = "SKILL.md"

// LookupOptions overrides process paths for deterministic callers.
type LookupOptions struct {
	ExecutablePath string
	WorkDir        string
}

// Find resolves one configuration file from the installed Skill package root.
func Find(relativePath string, options LookupOptions) (string, error) {
	cleanRelativePath, err := validateRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	displayRelativePath := filepath.ToSlash(cleanRelativePath)
	starts, err := lookupStarts(options)
	if err != nil {
		return "", err
	}
	seenRoots := map[string]bool{}
	for _, start := range starts {
		root, err := findPackageRoot(start)
		if err != nil {
			return "", err
		}
		if root == "" || seenRoots[root] {
			continue
		}
		seenRoots[root] = true
		candidate := filepath.Join(root, cleanRelativePath)
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return "", fmt.Errorf("runtime config path is a directory: %s", candidate)
			}
			return candidate, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("missing runtime config %s in Skill package %s", displayRelativePath, root)
		}
		return "", fmt.Errorf("inspect runtime config %s: %w", candidate, err)
	}
	return "", fmt.Errorf("installed Skill package root containing %s was not found for runtime config %s", skillManifestFilename, displayRelativePath)
}

func validateRelativePath(relativePath string) (string, error) {
	trimmed := strings.TrimSpace(relativePath)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", errors.New("runtime config path must be relative")
	}
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", errors.New("runtime config path must stay inside the Skill package")
	}
	return cleaned, nil
}

func lookupStarts(options LookupOptions) ([]string, error) {
	var starts []string
	executablePath := strings.TrimSpace(options.ExecutablePath)
	if executablePath == "" {
		path, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable path: %w", err)
		}
		executablePath = path
	}
	if executablePath != "" {
		absolute, err := filepath.Abs(executablePath)
		if err != nil {
			return nil, fmt.Errorf("resolve executable path %s: %w", executablePath, err)
		}
		starts = append(starts, filepath.Dir(absolute))
	}

	workDir := strings.TrimSpace(options.WorkDir)
	if workDir == "" {
		path, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve working directory: %w", err)
		}
		workDir = path
	}
	if workDir != "" {
		absolute, err := filepath.Abs(workDir)
		if err != nil {
			return nil, fmt.Errorf("resolve working directory %s: %w", workDir, err)
		}
		starts = append(starts, absolute)
	}
	return starts, nil
}

func findPackageRoot(start string) (string, error) {
	dir := filepath.Clean(start)
	for {
		manifestPath := filepath.Join(dir, skillManifestFilename)
		info, err := os.Stat(manifestPath)
		if err == nil {
			if info.IsDir() {
				return "", fmt.Errorf("Skill manifest path is a directory: %s", manifestPath)
			}
			return dir, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect Skill manifest %s: %w", manifestPath, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
