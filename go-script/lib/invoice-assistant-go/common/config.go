package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"invoice-assistant-skill/go-script/lib/runtimeconfig"
	"invoice-assistant-skill/go-script/lib/skillupdate"
)

const (
	ConfigSubdir   = ".invoice-config"
	ConfigFilename = "workspace-config.json"
	WorkspaceKey   = "workspace_dir"
	APIKey         = "apikey"
	// CompanyInfoKey 仅用于识别并清理旧版工作区配置，不再作为企业上下文来源。
	CompanyInfoKey          = "companyInfo"
	EnterpriseContextKey    = "enterprise_context"
	DefaultWorkspaceDirname = "invoice-workspace"
	// AccountIDMapKey 仅用于识别并清理旧版客户端认证缓存。
	AccountIDMapKey        = "account_id_map"
	RuntimeConfigFilename  = "runtime-config.json"
	DefaultBaseURLKey      = "default_base_url"
	PortalBaseURLKey       = "portal_base_url"
	OpenAPISkillBaseURLKey = "open_api_skill_base_url"
	OpenAPIPageURLKey      = "open_api_page_url"
	SkillNameKey           = "skill_name"
	InstallDocURLKey       = "install_doc_url"
	EnvironmentKey         = "environment"
	EnvironmentProd        = "prod"
)

const InvoiceIssueCapabilityCode = "invoice_issue"

var (
	runtimeConfig, runtimeConfigErr = LoadRuntimeConfig()
	CapabilityCode                  = runtimeConfig.CapabilityCode
	DefaultBaseURL                  = runtimeConfig.DefaultBaseURL
	PortalBaseURL                   = runtimeConfig.PortalBaseURL
	versionCheckerFactory           = skillupdate.Shared
	additionalRuntimeEnvironments   = map[string]bool{}
)

// reloadRuntimeConfigState refreshes every cached value after optional runtime modes are registered.
func reloadRuntimeConfigState() {
	loadedConfig, err := LoadRuntimeConfig()
	runtimeConfig = loadedConfig
	runtimeConfigErr = err
	CapabilityCode = loadedConfig.CapabilityCode
	DefaultBaseURL = loadedConfig.DefaultBaseURL
	PortalBaseURL = loadedConfig.PortalBaseURL
	defaultBindingURL = PortalBaseURL + "/workspace"
	defaultSignupURL = PortalBaseURL + "/workspace"
	defaultLoginURL = PortalBaseURL + "/shareVerification"
}

type RuntimeConfig struct {
	Environment    string
	CapabilityCode string
	DefaultBaseURL string
	PortalBaseURL  string
	SkillName      string
	InstallDocURL  string
}

// RuntimeVersionChecker returns the shared gate for this packaged skill process.
func RuntimeVersionChecker() skillupdate.Checker {
	if runtimeConfigErr != nil {
		configErr := runtimeConfigErr
		return skillupdate.CheckFunc(func(context.Context) error { return configErr })
	}
	return versionCheckerFactory(runtimeConfig.SkillName, runtimeConfig.InstallDocURL)
}

func SkillRoot() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	wd, _ := os.Getwd()
	return wd
}

func SkillInstallDir() string {
	candidates := []string{}
	if exeRoot := SkillRoot(); exeRoot != "" {
		candidates = append(candidates, exeRoot)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	for _, candidate := range candidates {
		if packageRoot := findSkillPackageRoot(candidate); packageRoot != "" {
			return packageRoot
		}
	}
	return SkillRoot()
}

func findSkillPackageRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		dir = start
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func DefaultWorkspaceDir() string {
	return filepath.Join(filepath.Dir(filepath.Dir(SkillInstallDir())), DefaultWorkspaceDirname)
}

func legacyDefaultWorkspaceDir() string {
	return filepath.Join(filepath.Dir(SkillInstallDir()), DefaultWorkspaceDirname)
}

func NormalizeWorkspaceDir(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return DefaultWorkspaceDir()
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return workspace
	}
	if samePath(abs, legacyDefaultWorkspaceDir()) {
		return DefaultWorkspaceDir()
	}
	return abs
}

func RewriteLegacyWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || isURLLike(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	oldRoot, err := filepath.Abs(legacyDefaultWorkspaceDir())
	if err != nil {
		oldRoot = filepath.Clean(legacyDefaultWorkspaceDir())
	}
	if !samePath(abs, oldRoot) {
		rel, relErr := filepath.Rel(oldRoot, abs)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return abs
		}
		return filepath.Join(DefaultWorkspaceDir(), rel)
	}
	return DefaultWorkspaceDir()
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil {
		left = leftAbs
	}
	if rightErr == nil {
		right = rightAbs
	}
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func isURLLike(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func ConfigPath() string {
	return filepath.Join(DefaultWorkspaceDir(), ConfigSubdir, ConfigFilename)
}

func RuntimeConfigPath() string {
	path, _ := runtimeconfig.Find(RuntimeConfigFilename, runtimeconfig.LookupOptions{})
	return path
}

func MustLoadRuntimeConfig() RuntimeConfig {
	config, err := LoadRuntimeConfig()
	if err != nil {
		panic(err)
	}
	return config
}

func LoadRuntimeConfig() (RuntimeConfig, error) {
	path, err := runtimeconfig.Find(RuntimeConfigFilename, runtimeconfig.LookupOptions{})
	if err != nil {
		return RuntimeConfig{}, err
	}
	return loadRuntimeConfigFromPath(path)
}

func loadRuntimeConfigFromPath(path string) (RuntimeConfig, error) {
	payload, err := ReadJSONFile(path)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("load runtime config %s: %w", path, err)
	}
	return parseRuntimeConfig(path, payload)
}

func parseRuntimeConfig(path string, payload JSONObject) (RuntimeConfig, error) {
	required := func(keys ...string) (string, error) {
		for _, key := range keys {
			value := strings.TrimSpace(StringValue(payload[key]))
			if value != "" {
				return value, nil
			}
		}
		key := strings.Join(keys, `" or "`)
		return "", fmt.Errorf("runtime config %s missing required field %q", path, key)
	}
	capabilityCode, err := required("capability_code")
	if err != nil {
		return RuntimeConfig{}, err
	}
	defaultBaseURL, err := required(DefaultBaseURLKey, OpenAPISkillBaseURLKey)
	if err != nil {
		return RuntimeConfig{}, err
	}
	portalBaseURL, err := required(PortalBaseURLKey, OpenAPIPageURLKey)
	if err != nil {
		return RuntimeConfig{}, err
	}
	skillName, err := required(SkillNameKey)
	if err != nil {
		return RuntimeConfig{}, err
	}
	installDocURL, err := required(InstallDocURLKey)
	if err != nil {
		return RuntimeConfig{}, err
	}
	environment := strings.TrimSpace(StringValue(payload[EnvironmentKey]))
	if environment == "" {
		environment = EnvironmentProd
	}
	if environment != EnvironmentProd && !additionalRuntimeEnvironments[environment] {
		return RuntimeConfig{}, fmt.Errorf("runtime config %s field %q must be %q", path, EnvironmentKey, EnvironmentProd)
	}
	return RuntimeConfig{
		Environment:    environment,
		CapabilityCode: capabilityCode,
		DefaultBaseURL: strings.TrimRight(defaultBaseURL, "/"),
		PortalBaseURL:  strings.TrimRight(portalBaseURL, "/"),
		SkillName:      skillName,
		InstallDocURL:  installDocURL,
	}, nil
}

func LoadWorkspaceConfig() map[string]any {
	path := ConfigPath()
	payload, err := ReadJSONFile(path)
	if err != nil {
		return DefaultConfig()
	}
	normalizeConfig(payload)
	return payload
}

func DefaultConfig() map[string]any {
	return map[string]any{
		APIKey:                  "",
		WorkspaceKey:            "",
		"default_workspace_dir": DefaultWorkspaceDir(),
	}
}

/**
 * normalizeConfig 规范化工作区配置，并从内存视图移除旧版企业选择状态。
 *
 * @param config 从 workspace-config.json 读取或准备写回的配置对象。
 * @returns 无返回值；直接更新传入对象。
 */
func normalizeConfig(config map[string]any) {
	if _, ok := config[APIKey]; !ok {
		config[APIKey] = ""
	}
	if _, ok := config[WorkspaceKey]; !ok {
		config[WorkspaceKey] = ""
	} else if workspace := strings.TrimSpace(StringValue(config[WorkspaceKey])); workspace != "" {
		config[WorkspaceKey] = NormalizeWorkspaceDir(workspace)
	}
	// 旧版企业选择状态只在内存中清理；下一次配置写入时会自然从磁盘移除。
	delete(config, "uscc")
	delete(config, CompanyInfoKey)
	delete(config, AccountIDMapKey)
	for _, key := range []string{"enterpriseName", "areaCode", "agencyUSCC", "loginType", "identityType", "personalAccount", "password", "nsrzg"} {
		delete(config, key)
	}
}

func HasMeaningfulValue(value any) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}
