package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"invoice-assistant-skill/go-script/lib/runtimeconfig"
)

const (
	runtimeConfigFilename      = "runtime-config.json"
	taxAnalysisSkillName       = "tax-analysis"
	taxAnalysisProdEnvironment = "prod"
	taxAnalysisTestEnvironment = "test"
)

type runtimeConfig struct {
	BaseURL         string `json:"base_url"`
	DefaultBaseURL  string `json:"default_base_url"`
	OpenAPISkillURL string `json:"open_api_skill_base_url"`
	Environment     string `json:"environment"`
	SkillName       string `json:"skill_name"`
	InstallDocURL   string `json:"install_doc_url"`
}

func loadRuntimeConfig() (runtimeConfig, error) {
	path, err := runtimeconfig.Find(runtimeConfigFilename, runtimeconfig.LookupOptions{})
	if err != nil {
		return runtimeConfig{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("read runtime config: %w", err)
	}
	defer file.Close()
	var config runtimeConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return runtimeConfig{}, fmt.Errorf("parse runtime config: %w", err)
	}
	for _, value := range []string{config.BaseURL, config.DefaultBaseURL, config.OpenAPISkillURL} {
		if strings.TrimSpace(value) != "" {
			config.BaseURL = strings.TrimRight(strings.TrimSpace(value), "/")
			break
		}
	}
	if config.BaseURL == "" {
		return runtimeConfig{}, errors.New("runtime config missing base URL")
	}
	config.Environment = strings.TrimSpace(config.Environment)
	if config.Environment == "" {
		config.Environment = taxAnalysisProdEnvironment
	}
	if config.Environment != taxAnalysisProdEnvironment && config.Environment != taxAnalysisTestEnvironment {
		return runtimeConfig{}, fmt.Errorf("unsupported runtime environment %q", config.Environment)
	}
	if strings.TrimSpace(config.SkillName) == "" {
		config.SkillName = taxAnalysisSkillName
	}
	if strings.TrimSpace(config.InstallDocURL) == "" {
		return runtimeConfig{}, errors.New("runtime config missing install_doc_url")
	}
	return config, nil
}
