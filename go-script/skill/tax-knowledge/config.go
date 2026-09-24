package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"invoice-assistant-skill/go-script/lib/runtimeconfig"
)

const runtimeConfigFilename = "runtime-config.json"

const (
	taxKnowledgeProdEnvironment = "prod"
	taxKnowledgeSkillName       = "tax-knowledge"
)

var additionalRuntimeEnvironments = map[string]bool{}

type runtimeConfig struct {
	BaseURL         string `json:"base_url"`
	DefaultBaseURL  string `json:"default_base_url"`
	OpenAPISkillURL string `json:"open_api_skill_base_url"`
	Environment     string `json:"environment"`
	SkillName       string `json:"skill_name"`
	InstallDocURL   string `json:"install_doc_url"`
}

func loadBaseURL() (string, error) {
	config, err := loadRuntimeConfig()
	if err != nil {
		return "", err
	}
	return config.BaseURL, nil
}

func loadRuntimeConfig() (runtimeConfig, error) {
	path, err := findRuntimeConfig()
	if err != nil {
		return runtimeConfig{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("read runtime config %s: %w", path, err)
	}
	defer file.Close()
	var config runtimeConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return runtimeConfig{}, fmt.Errorf("parse runtime config %s: %w", path, err)
	}
	for _, value := range []string{config.BaseURL, config.DefaultBaseURL, config.OpenAPISkillURL} {
		if strings.TrimSpace(value) != "" {
			config.BaseURL = strings.TrimRight(strings.TrimSpace(value), "/")
			break
		}
	}
	if config.BaseURL == "" {
		return runtimeConfig{}, fmt.Errorf("runtime config %s missing base URL", path)
	}
	config.Environment = strings.TrimSpace(config.Environment)
	if config.Environment == "" {
		config.Environment = taxKnowledgeProdEnvironment
	}
	if config.Environment != taxKnowledgeProdEnvironment && !additionalRuntimeEnvironments[config.Environment] {
		return runtimeConfig{}, fmt.Errorf("runtime config %s environment must be %q", path, taxKnowledgeProdEnvironment)
	}
	config.SkillName = strings.TrimSpace(config.SkillName)
	if config.SkillName == "" {
		return runtimeConfig{}, fmt.Errorf("runtime config %s missing skill_name", path)
	}
	config.InstallDocURL = strings.TrimSpace(config.InstallDocURL)
	if config.InstallDocURL == "" {
		return runtimeConfig{}, fmt.Errorf("runtime config %s missing install_doc_url", path)
	}
	return config, nil
}

func findRuntimeConfig() (string, error) {
	return runtimeconfig.Find(runtimeConfigFilename, runtimeconfig.LookupOptions{})
}

func workspaceConfigSearchDirs() []string {
	var starts []string
	if executable, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(executable))
	}
	if workDir, err := os.Getwd(); err == nil {
		starts = append(starts, workDir)
	}
	return ancestorDirs(starts)
}

func ancestorDirs(starts []string) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, start := range starts {
		dir, err := filepath.Abs(start)
		if err != nil {
			dir = filepath.Clean(start)
		}
		for {
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return dirs
}
