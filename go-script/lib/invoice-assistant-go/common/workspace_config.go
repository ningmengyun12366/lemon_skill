package common

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func UpdateWorkspaceConfig(workspaceDir, apikey string, pairs []string, extraJSON string) (map[string]any, error) {
	extra := map[string]any{}
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("--set must be key=value: %s", pair)
		}
		extra[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if strings.TrimSpace(extraJSON) != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(extraJSON), &parsed); err != nil {
			return nil, err
		}
		for k, v := range parsed {
			extra[k] = v
		}
	}
	if err := validateWorkspaceConfigUpdate(extra); err != nil {
		return nil, err
	}
	workspace := ""
	if strings.TrimSpace(workspaceDir) != "" {
		workspace = NormalizeWorkspaceDir(workspaceDir)
		abs := workspace
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return nil, err
		}
	}
	path := ConfigPath()
	config := map[string]any{}
	err := updateWorkspaceConfigTransaction(workspaceConfigUpdate{
		Path:     path,
		Snapshot: config,
		Mutate: func(latest map[string]any) error {
			if workspace != "" {
				latest[WorkspaceKey] = workspace
			}
			if apikey != "" {
				latest[APIKey] = strings.TrimSpace(apikey)
			}
			for key, value := range extra {
				latest[key] = value
			}
			normalizeConfig(latest)
			latest["default_workspace_dir"] = DefaultWorkspaceDir()
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return map[string]any{
		"status":         "success",
		"config_path":    path,
		"workspace_dir":  StringValue(config[WorkspaceKey]),
		"apikey_present": strings.TrimSpace(StringValue(config[APIKey])) != "",
		"updated_keys":   keys,
		"config":         config,
	}, nil
}

/**
 * validateWorkspaceConfigUpdate 拒绝重新写入已废弃的工作区企业状态。
 *
 * @param extra workspace-config-writer 准备写入的扩展字段。
 * @returns 字段合法时返回 nil，否则返回明确错误。
 */
func validateWorkspaceConfigUpdate(extra map[string]any) error {
	legacyKeys := map[string]struct{}{
		"uscc": {}, CompanyInfoKey: {}, "enterpriseName": {}, "areaCode": {},
		"agencyUSCC": {}, "loginType": {}, "identityType": {}, "personalAccount": {},
		"password": {}, "nsrzg": {},
	}
	for key := range extra {
		if _, blocked := legacyKeys[key]; blocked {
			return fmt.Errorf("workspace-config no longer accepts enterprise field %q; provide uscc in each task", key)
		}
	}
	return nil
}

func legacyTaskPreflightCheck(enterpriseName string) map[string]any {
	config := LoadWorkspaceConfig()
	workspace := strings.TrimSpace(StringValue(config[WorkspaceKey]))
	if workspace == "" {
		workspace = DefaultWorkspaceDir()
	} else {
		workspace = NormalizeWorkspaceDir(workspace)
	}
	_ = os.MkdirAll(workspace, 0o755)
	return map[string]any{
		"status":          "success",
		"workspace_dir":   workspace,
		"enterprise_name": enterpriseName,
		"config_path":     ConfigPath(),
		"apikey_present":  strings.TrimSpace(StringValue(config[APIKey])) != "",
		"message":         "任务执行前初始化检测通过",
	}
}
