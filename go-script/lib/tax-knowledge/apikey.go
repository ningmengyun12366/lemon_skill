package taxknowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	configSubdir   = ".invoice-config"
	configFilename = "workspace-config.json"
	apiKeyField    = "apikey"
)

type workspaceConfig struct {
	APIKey string `json:"apikey"`
}

func ResolveAPIKey(explicit string, searchDirs []string) (APIKey, error) {
	if strings.TrimSpace(explicit) != "" {
		key, err := NewAPIKey(explicit)
		if err != nil {
			return APIKey{}, fmt.Errorf("parse explicit api key: %w", err)
		}
		return key, nil
	}
	for _, dir := range searchDirs {
		key, found, err := resolveAPIKeyFromDir(dir)
		if err != nil {
			return APIKey{}, err
		}
		if found {
			return key, nil
		}
	}
	return APIKey{}, ErrMissingAPIKey
}

func resolveAPIKeyFromDir(dir string) (APIKey, bool, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return APIKey{}, false, nil
	}
	for _, candidate := range workspaceConfigCandidates(trimmed) {
		payload, err := os.ReadFile(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return APIKey{}, false, fmt.Errorf("read workspace config %s: %w", candidate, err)
		}
		var config workspaceConfig
		if err := json.Unmarshal(payload, &config); err != nil {
			return APIKey{}, false, fmt.Errorf("parse workspace config %s: %w", candidate, err)
		}
		if strings.TrimSpace(config.APIKey) == "" {
			continue
		}
		key, err := NewAPIKey(config.APIKey)
		if err != nil {
			return APIKey{}, false, fmt.Errorf("parse workspace config %s field %s: %w", candidate, apiKeyField, err)
		}
		return key, true, nil
	}
	return APIKey{}, false, nil
}

func workspaceConfigCandidates(dir string) []string {
	return []string{
		filepath.Join(dir, configSubdir, configFilename),
		filepath.Join(dir, configFilename),
	}
}
