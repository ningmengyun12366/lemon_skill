package fetchrisk

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
	TasksDirName         = "invoice-fetch-and-risk-analysis/tasks"
	RunsDirName          = "invoice-fetch-and-risk-analysis"
	TemporaryRootDirName = "invoice-fetch-and-risk-analysis/temporary"
	SceneID              = "invoice-fetch-risk-analysis"
)

type TaskState struct {
	Path string
	Data map[string]any
}

func LoadTaskState(taskFile string) (*TaskState, error) {
	abs, err := filepath.Abs(taskFile)
	if err != nil {
		return nil, err
	}
	data, err := common.ReadJSONFile(abs)
	if err != nil {
		return nil, err
	}
	stem := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	setDefault(data, "task_id", stem)
	setDefault(data, "execution", map[string]any{})
	setDefault(data, "artifacts", map[string]any{})
	setDefault(data, "outputs", map[string]any{})
	setDefault(data, "intent", map[string]any{})
	setDefault(data, "steps", []any{})
	setDefault(data, "created_at", common.NowISO())
	setDefault(data, "updated_at", common.NowISO())
	data["task_file_path"] = abs
	normalizeTaskStatePaths(data)
	return &TaskState{Path: abs, Data: data}, nil
}

func (s *TaskState) Save() error {
	abs, err := filepath.Abs(s.Path)
	if err != nil {
		return err
	}
	s.Path = abs
	s.Data["task_id"] = strings.TrimSuffix(filepath.Base(s.Path), filepath.Ext(s.Path))
	s.Data["task_file_path"] = s.Path
	s.Data["updated_at"] = common.NowISO()
	setDefault(s.Data, "created_at", s.Data["updated_at"])
	normalizeTaskStatePaths(s.Data)
	return common.WriteJSONFile(s.Path, s.Data)
}

/**
 * CreateTaskState 创建包含不可变企业快照的取票任务状态。
 *
 * @param workspaceDir 本次任务的工作目录。
 * @param params 已由服务端企业记录补全的业务参数。
 * @param steps 本次任务的执行步骤。
 * @param enterpriseContext 通过企业列表精确匹配得到的企业快照。
 * @returns 尚未写盘的任务状态对象。
 */
func CreateTaskState(workspaceDir string, params map[string]any, steps []map[string]any, enterpriseContext map[string]any) (*TaskState, error) {
	if err := common.ValidateEnterpriseContext(enterpriseContext); err != nil {
		return nil, err
	}
	params = common.ApplyEnterpriseContext(params, enterpriseContext)
	workspaceDir = common.NormalizeWorkspaceDir(workspaceDir)
	taskPath, err := BuildTaskFilePath(workspaceDir)
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSuffix(filepath.Base(taskPath), filepath.Ext(taskPath))
	uscc := strings.TrimSpace(common.StringValue(params["uscc"]))
	if uscc == "" {
		uscc = "unknown-uscc"
	}
	storage, err := BuildStorageDirs(workspaceDir, uscc, taskID)
	if err != nil {
		return nil, err
	}
	absWorkspace, _ := filepath.Abs(workspaceDir)
	state := map[string]any{
		"task_id":                   taskID,
		"scene_id":                  SceneID,
		"workspace_dir":             absWorkspace,
		"task_file_path":            taskPath,
		"storage":                   storage,
		common.EnterpriseContextKey: common.ApplyEnterpriseContext(map[string]any{}, enterpriseContext),
		"intent": map[string]any{
			"params": params,
		},
		"steps":      prepareTaskSteps(steps),
		"execution":  map[string]any{},
		"artifacts":  map[string]any{},
		"outputs":    map[string]any{},
		"created_at": common.NowISO(),
		"updated_at": common.NowISO(),
	}
	return &TaskState{Path: taskPath, Data: state}, nil
}

func BuildTaskFilePath(workspaceDir string) (string, error) {
	workspaceDir = common.NormalizeWorkspaceDir(workspaceDir)
	absWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", err
	}
	tasksDir := filepath.Join(absWorkspace, filepath.FromSlash(TasksDirName))
	if err := ensureWritableDir(tasksDir); err != nil {
		return "", err
	}
	return filepath.Join(tasksDir, fmt.Sprintf("%s-%s.json", time.Now().Format("20060102150405"), shortHash())), nil
}

func BuildStorageDirs(workspaceDir, uscc, taskID string) (map[string]any, error) {
	workspaceDir = common.NormalizeWorkspaceDir(workspaceDir)
	absWorkspace, _ := filepath.Abs(workspaceDir)
	folder := sanitizeSegment(uscc) + "-" + time.Now().Format("20060102150405")
	root := filepath.Join(absWorkspace, RunsDirName, folder)
	storage := map[string]any{
		"root_dir":         root,
		"raw_invoice_dir":  filepath.Join(root, "税局原始发票数据"),
		"attachments_dir":  filepath.Join(root, "发票附件"),
		"invoice_list_dir": filepath.Join(root, "发票清单"),
		"temporary_dir":    filepath.Join(absWorkspace, filepath.FromSlash(TemporaryRootDirName), sanitizeSegment(taskID)),
	}
	for _, value := range storage {
		if err := ensureWritableDir(common.StringValue(value)); err != nil {
			return nil, err
		}
	}
	return storage, nil
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
		if !pathKey(key) || isRemoteURL(typed) || strings.TrimSpace(typed) == "" {
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

func isRemoteURL(value string) bool {
	text := strings.TrimSpace(value)
	lower := strings.ToLower(text)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func IntentParams(data map[string]any) (map[string]any, error) {
	intent, ok := data["intent"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("task JSON missing intent")
	}
	params, ok := intent["params"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("task JSON missing intent.params")
	}
	return params, nil
}

func RequiredString(params map[string]any, key string) (string, error) {
	value, err := common.RequiredParam(params, key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(common.StringValue(value)), nil
}

func RequiredInt(params map[string]any, key string) (int, error) {
	value, err := common.RequiredParam(params, key)
	if err != nil {
		return 0, err
	}
	if key == "areaCode" {
		return common.NormalizeStandardAreaCode(value)
	}
	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		var out int
		_, scanErr := fmt.Sscan(v, &out)
		return out, scanErr
	default:
		return 0, fmt.Errorf("param %s must be an integer", key)
	}
}
