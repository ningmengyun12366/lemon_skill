package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type JSONObject map[string]any

func ReadJSONFile(path string) (JSONObject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("JSON file must contain an object: %s", path)
	}
	return obj, nil
}

func WriteJSONFile(path string, payload JSONObject) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func NowISO() string {
	return time.Now().Format("2006-01-02T15:04:05")
}

func StringValue(v any) string {
	if v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func ObjectValue(root map[string]any, key string) map[string]any {
	value, ok := root[key].(map[string]any)
	if ok {
		return value
	}
	value = map[string]any{}
	root[key] = value
	return value
}
