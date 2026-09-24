package issue

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	issueTasksDirName   = "invoice-issue/tasks"
	issueRunsDirName    = "invoice-issue"
	issueSceneID        = "invoice-issue"
	attachmentsDirName  = "发票附件"
	previewDirName      = "发票预览"
	temporaryDirName    = "temporary"
	currentTaskFilename = "current-task-path.json"
)

func BuildTaskFilePath(workspaceDir, taskID string) (string, error) {
	workspaceDir = common.NormalizeWorkspaceDir(workspaceDir)
	tasksDir := filepath.Join(workspaceDir, filepath.FromSlash(issueTasksDirName))
	if err := ensureWritableDir(tasksDir); err != nil {
		return "", err
	}
	if strings.TrimSpace(taskID) == "" {
		taskID = time.Now().Format("20060102150405") + "-" + shortHash()
	}
	return filepath.Join(tasksDir, taskID+".json"), nil
}

func BuildStorageDirs(workspaceDir, uscc, taskTimestamp string) (map[string]any, error) {
	workspaceDir = common.NormalizeWorkspaceDir(workspaceDir)
	folder := sanitizeSegment(uscc) + "_" + taskTimestamp
	root := filepath.Join(workspaceDir, issueRunsDirName, folder)
	storage := map[string]any{
		"root_dir":        root,
		"attachments_dir": filepath.Join(root, attachmentsDirName),
		"preview_dir":     filepath.Join(root, previewDirName),
		"temporary_dir":   filepath.Join(workspaceDir, issueRunsDirName, temporaryDirName, taskTimestamp),
	}
	for _, value := range storage {
		if err := ensureWritableDir(common.StringValue(value)); err != nil {
			return nil, err
		}
	}
	return storage, nil
}

func CurrentTaskFilePath() (string, error) {
	workspace, err := ResolveWorkspaceDir()
	if err != nil {
		return "", err
	}
	pointer := filepath.Join(workspace, filepath.FromSlash(issueTasksDirName), currentTaskFilename)
	data, err := common.ReadJSONFile(pointer)
	if err != nil {
		return "", fmt.Errorf("尚未初始化开票任务，请先执行单张或批量检查脚本创建任务状态 JSON")
	}
	taskPath := strings.TrimSpace(common.StringValue(data["task_file_path"]))
	if taskPath == "" {
		return "", fmt.Errorf("当前任务指针为空，请重新执行检查脚本")
	}
	return taskPath, nil
}

func SetCurrentTaskFilePath(taskFilePath, workspaceDir string) error {
	if strings.TrimSpace(workspaceDir) == "" {
		var err error
		workspaceDir, err = ResolveWorkspaceDir()
		if err != nil {
			return err
		}
	} else {
		workspaceDir = common.NormalizeWorkspaceDir(workspaceDir)
	}
	pointer := filepath.Join(workspaceDir, filepath.FromSlash(issueTasksDirName), currentTaskFilename)
	return common.WriteJSONFile(pointer, map[string]any{"task_file_path": taskFilePath})
}

func normalizeTaskStatePaths(data map[string]any) {
	normalizePathValue("", data)
}

func normalizePathValue(key string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			typed[childKey] = normalizePathValue(childKey, childValue)
		}
		return typed
	case []any:
		for idx, item := range typed {
			if pathKey(key) {
				typed[idx] = normalizePathValue(key, item)
			} else {
				typed[idx] = normalizePathValue("", item)
			}
		}
		return typed
	case string:
		if !pathKey(key) || strings.TrimSpace(typed) == "" {
			return typed
		}
		return common.RewriteLegacyWorkspacePath(typed)
	default:
		return value
	}
}

func pathKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	if normalized == "workspace_dir" || normalized == "task_file" || normalized == "task_file_path" || normalized == "input" || normalized == "output" {
		return true
	}
	return strings.Contains(normalized, "path") ||
		strings.HasSuffix(normalized, "_dir") ||
		strings.HasSuffix(normalized, "_file") ||
		strings.HasSuffix(normalized, "_files")
}

func ensureWritableDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	probe := filepath.Join(path, ".task_write_probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return err
	}
	return os.Remove(probe)
}

func extractUSCC(raw map[string]any) string {
	uscc := strings.TrimSpace(common.StringValue(raw["uscc"]))
	if uscc == "" {
		return "unknown-uscc"
	}
	return uscc
}

func fileStem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func sanitizeSegment(value string) string {
	re := regexp.MustCompile(`[^0-9A-Za-z_-]`)
	out := re.ReplaceAllString(strings.TrimSpace(value), "_")
	if out == "" {
		return "unknown"
	}
	return out
}

func shortHash() string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s-%d", time.Now().Format(time.RFC3339Nano), time.Now().UnixNano())))
	return hex.EncodeToString(h[:])[:8]
}

func setDefault(obj map[string]any, key string, value any) {
	if _, ok := obj[key]; !ok || obj[key] == nil {
		obj[key] = value
	}
}
