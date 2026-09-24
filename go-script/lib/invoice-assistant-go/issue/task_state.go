package issue

import (
	"fmt"
	"path/filepath"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

type TaskState struct {
	Path string
	Data map[string]any
}

func ResolveWorkspaceDir() (string, error) {
	config := common.LoadWorkspaceConfig()
	workspace := common.NormalizeWorkspaceDir(common.StringValue(config[common.WorkspaceKey]))
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	if err := ensureWritableDir(abs); err != nil {
		return "", err
	}
	return abs, nil
}

/**
 * CreateTaskState 创建包含不可变企业快照的开票任务状态。
 *
 * @param rawInput 已由服务端企业记录补全的开票输入。
 * @param taskFile 可选的任务文件路径。
 * @param taskType single 或 batch。
 * @param enterpriseContext 通过企业列表精确匹配得到的企业快照。
 * @returns 尚未写入检查结果的任务状态对象。
 */
func CreateTaskState(rawInput map[string]any, taskFile, taskType string, enterpriseContext map[string]any) (*TaskState, error) {
	if err := common.ValidateEnterpriseContext(enterpriseContext); err != nil {
		return nil, err
	}
	rawInput = common.ApplyEnterpriseContext(rawInput, enterpriseContext)
	normalized, err := NormalizeRawInput(rawInput)
	if err != nil {
		return nil, err
	}
	workspace, err := ResolveWorkspaceDir()
	if err != nil {
		return nil, err
	}
	path := taskFile
	if strings.TrimSpace(path) == "" {
		path, err = BuildTaskFilePath(workspace, "")
		if err != nil {
			return nil, err
		}
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	taskID := fileStem(absPath)
	taskTimestamp := strings.Split(taskID, "-")[0]
	storage, err := BuildStorageDirs(workspace, extractUSCC(normalized), taskTimestamp)
	if err != nil {
		return nil, err
	}
	if taskType == "" {
		taskType = "batch"
	}
	now := common.NowISO()
	return &TaskState{
		Path: absPath,
		Data: map[string]any{
			"task_id":                   taskID,
			"scene_id":                  issueSceneID,
			"task_type":                 taskType,
			"task_status":               "created",
			"workspace_dir":             workspace,
			"task_file_path":            absPath,
			"storage":                   storage,
			common.EnterpriseContextKey: common.ApplyEnterpriseContext(map[string]any{}, enterpriseContext),
			"raw_input_json":            normalized,
			"check_result":              map[string]any{},
			"issue_result":              map[string]any{},
			"created_at":                now,
			"updated_at":                now,
		},
	}, nil
}

func LoadTaskState(taskFile string) (*TaskState, error) {
	path := taskFile
	if strings.TrimSpace(path) == "" {
		current, err := CurrentTaskFilePath()
		if err != nil {
			return nil, err
		}
		path = current
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := common.ReadJSONFile(abs)
	if err != nil {
		return nil, err
	}
	workspace := strings.TrimSpace(common.StringValue(data["workspace_dir"]))
	if workspace == "" {
		workspace, _ = ResolveWorkspaceDir()
	} else {
		workspace = common.NormalizeWorkspaceDir(workspace)
	}
	rawInput, _ := data["raw_input_json"].(map[string]any)
	taskID := fileStem(abs)
	taskTimestamp := strings.Split(taskID, "-")[0]
	setDefault(data, "task_id", taskID)
	setDefault(data, "scene_id", issueSceneID)
	setDefault(data, "task_type", "batch")
	setDefault(data, "task_status", "created")
	setDefault(data, "workspace_dir", workspace)
	setDefault(data, "task_file_path", abs)
	if _, ok := data["storage"].(map[string]any); !ok {
		storage, _ := BuildStorageDirs(workspace, extractUSCC(rawInput), taskTimestamp)
		data["storage"] = storage
	}
	setDefault(data, "raw_input_json", map[string]any{})
	setDefault(data, "check_result", map[string]any{})
	setDefault(data, "issue_result", map[string]any{})
	setDefault(data, "created_at", common.NowISO())
	setDefault(data, "updated_at", common.NowISO())
	normalizeTaskStatePaths(data)
	return &TaskState{Path: abs, Data: data}, nil
}

func (s *TaskState) Save(pretty bool) error {
	abs, err := filepath.Abs(s.Path)
	if err != nil {
		return err
	}
	s.Path = abs
	s.Data["task_id"] = fileStem(abs)
	s.Data["task_file_path"] = abs
	if workspace := strings.TrimSpace(common.StringValue(s.Data["workspace_dir"])); workspace != "" {
		s.Data["workspace_dir"] = common.NormalizeWorkspaceDir(workspace)
	}
	normalizeTaskStatePaths(s.Data)
	s.Data["updated_at"] = common.NowISO()
	setDefault(s.Data, "created_at", s.Data["updated_at"])
	if err := common.WriteJSONFile(abs, s.Data); err != nil {
		return err
	}
	_ = pretty
	return SetCurrentTaskFilePath(abs, common.StringValue(s.Data["workspace_dir"]))
}

/**
 * NormalizeRawInput 清理开票输入并校验脚本已注入完整企业字段。
 *
 * @param rawInput 已应用 enterprise_context 的开票输入。
 * @returns 去除脚本内部字段并标准化 areaCode 的输入副本。
 */
func NormalizeRawInput(rawInput map[string]any) (map[string]any, error) {
	normalized := map[string]any{}
	for k, v := range rawInput {
		if k == common.EnterpriseContextKey {
			continue
		}
		normalized[k] = v
	}
	uscc := strings.TrimSpace(common.StringValue(normalized["uscc"]))
	if uscc == "" {
		return nil, fmt.Errorf("missing required param: uscc")
	}
	missing := []string{}
	for _, key := range []string{"areaCode", "personalAccount", "loginType"} {
		if !common.HasMeaningfulValue(normalized[key]) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("task enterprise input is missing %s", strings.Join(missing, "、"))
	}
	if _, ok := normalized["areaCode"]; ok {
		code, err := common.NormalizeStandardAreaCode(normalized["areaCode"])
		if err != nil {
			return nil, fmt.Errorf("开票任务参数 areaCode 非标准编码：%w", err)
		}
		normalized["areaCode"] = code
	}
	return normalized, nil
}
