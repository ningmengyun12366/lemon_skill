package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"invoice-assistant-skill/go-script/lib/runtimeconfig"
)

const (
	EnvironmentProd = "prod"

	RuntimeConfigRelativePath = "config/verify-api.json"
)

type Config struct {
	Environment       string `json:"environment"`
	VerifyAPIEndpoint string `json:"verify_api_endpoint"`
	InvoiceCheckCode  string `json:"invoice_check_code"`
	SkillName         string `json:"skill_name,omitempty"`
	InstallDocURL     string `json:"install_doc_url,omitempty"`
	OpenAPISkillURL   string `json:"open_api_skill_base_url,omitempty"`
	OpenAPIPageURL    string `json:"open_api_page_url,omitempty"`
	OpenAPIContactURL string `json:"open_api_contact_service_url,omitempty"`
}

type RuntimeLookupOptions struct {
	WorkDir        string
	ExecutablePath string
}

var secretLikeKeyPattern = regexp.MustCompile(`(?i)^(api[_-]?key|password|token|authorization)$`)

var additionalEnvironments = map[string]bool{}

var loopbackHTTPEnvironments = map[string]bool{}

func LoadFile(path string) (Config, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(payload, &topLevel); err != nil {
		return Config{}, fmt.Errorf("parse config JSON: %w", err)
	}
	if topLevel == nil {
		return Config{}, errors.New("config JSON must be an object")
	}
	for key := range topLevel {
		if secretLikeKeyPattern.MatchString(key) {
			return Config{}, fmt.Errorf("config contains forbidden secret-like key %q", key)
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config JSON: %w", err)
	}
	cfg.Environment = strings.TrimSpace(cfg.Environment)
	if cfg.Environment == "" {
		cfg.Environment = EnvironmentProd
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

func LoadRuntime() (Config, error) {
	path, err := FindRuntimeConfig(RuntimeLookupOptions{})
	if err != nil {
		return Config{}, err
	}
	cfg, err := LoadFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.ValidateSkillUpdate(); err != nil {
		return Config{}, fmt.Errorf("validate skill update config: %w", err)
	}
	return cfg, nil
}

func FindRuntimeConfig(options RuntimeLookupOptions) (string, error) {
	return runtimeconfig.Find(RuntimeConfigRelativePath, runtimeconfig.LookupOptions{
		ExecutablePath: options.ExecutablePath,
		WorkDir:        options.WorkDir,
	})
}

func (cfg Config) Validate() error {
	environment := strings.TrimSpace(cfg.Environment)
	if environment != EnvironmentProd && !additionalEnvironments[environment] {
		return fmt.Errorf("environment must be %q", EnvironmentProd)
	}
	if strings.TrimSpace(cfg.VerifyAPIEndpoint) == "" {
		return errors.New("verify_api_endpoint is required")
	}
	if err := validateEndpointSecurity(cfg.VerifyAPIEndpoint, AllowsLoopbackHTTP(environment)); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.InvoiceCheckCode) == "" {
		return errors.New("invoice_check_code is required")
	}
	if strings.ContainsAny(cfg.InvoiceCheckCode, ";\r\n") {
		return errors.New("invoice_check_code must not contain semicolon or newline")
	}
	return nil
}

// AllowsLoopbackHTTP reports whether an explicitly registered runtime mode permits loopback HTTP.
func AllowsLoopbackHTTP(environment string) bool {
	return loopbackHTTPEnvironments[strings.TrimSpace(environment)]
}

// ValidateSkillUpdate ensures a packaged runtime can enforce its remote version gate.
func (cfg Config) ValidateSkillUpdate() error {
	if strings.TrimSpace(cfg.SkillName) == "" {
		return errors.New("skill_name is required")
	}
	if strings.TrimSpace(cfg.InstallDocURL) == "" {
		return errors.New("install_doc_url is required")
	}
	return nil
}

func validateEndpointSecurity(endpoint string, allowLoopbackHTTP bool) error {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("verify_api_endpoint must be a valid URL with scheme and host")
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return nil
	}
	if allowLoopbackHTTP && strings.EqualFold(parsed.Scheme, "http") && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return errors.New("verify_api_endpoint must use HTTPS; loopback HTTP is not enabled for this runtime mode")
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
