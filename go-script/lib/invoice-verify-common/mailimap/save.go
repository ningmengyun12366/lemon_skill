package mailimap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteUniqueFile(dir, filename string, content []byte) (string, string, error) {
	safeName, err := SanitizeFilename(filename)
	if err != nil {
		return "", "", err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		return "", "", err
	}
	ext := filepath.Ext(safeName)
	stem := strings.TrimSuffix(safeName, ext)
	for attempt := 0; attempt < 10000; attempt++ {
		candidateName := safeName
		if attempt > 0 {
			candidateName = fmt.Sprintf("%s (%d)%s", stem, attempt, ext)
		}
		candidatePath := filepath.Join(absDir, candidateName)
		if err := ensureChildPath(absDir, candidatePath); err != nil {
			return "", "", err
		}
		file, err := os.OpenFile(candidatePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", "", err
		}
		if _, err := file.Write(content); err != nil {
			_ = file.Close()
			return "", "", err
		}
		if err := file.Close(); err != nil {
			return "", "", err
		}
		return candidatePath, candidateName, nil
	}
	return "", "", fmt.Errorf("too many duplicate filenames for %s", safeName)
}

func ensureChildPath(absDir, candidatePath string) error {
	absCandidate, err := filepath.Abs(candidatePath)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absDir, absCandidate)
	if err != nil {
		return err
	}
	if relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." || filepath.IsAbs(relative) {
		return fmt.Errorf("unsafe attachment path: %s", candidatePath)
	}
	return nil
}
