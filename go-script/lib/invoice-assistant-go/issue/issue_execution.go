package issue

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

type IssueExecution struct {
	BaseClient      *common.APIClient
	WorkspaceConfig map[string]any
	LockTimeout     time.Duration
}

func resolveIssueTaskPath(taskFile string) (string, error) {
	path := strings.TrimSpace(taskFile)
	if path == "" {
		current, err := CurrentTaskFilePath()
		if err != nil {
			return "", err
		}
		path = current
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve invoice task path: %w", err)
	}
	return abs, nil
}
