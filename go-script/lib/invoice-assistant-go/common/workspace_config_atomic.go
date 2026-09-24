package common

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func writeWorkspaceConfigAtomically(path string, payload map[string]any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return workspaceConfigPersistenceError("marshal", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		return workspaceConfigPersistenceError("temp-write", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		return closeWorkspaceConfigTemp(temp, "temp-write", err)
	}
	if _, err := temp.Write(data); err != nil {
		return closeWorkspaceConfigTemp(temp, "temp-write", err)
	}
	if err := temp.Sync(); err != nil {
		return closeWorkspaceConfigTemp(temp, "sync", err)
	}
	if err := temp.Close(); err != nil {
		return workspaceConfigPersistenceError("sync", err)
	}
	if err := replaceWorkspaceConfigFile(tempPath, path); err != nil {
		return workspaceConfigPersistenceError("replace", err)
	}
	if err := syncWorkspaceConfigParent(path); err != nil {
		return workspaceConfigPersistenceError("parent-sync", err)
	}
	return nil
}

func closeWorkspaceConfigTemp(temp *os.File, stage string, cause error) error {
	primary := workspaceConfigPersistenceError(stage, cause)
	if closeErr := temp.Close(); closeErr != nil {
		return errors.Join(primary, workspaceConfigPersistenceError(stage, closeErr))
	}
	return primary
}
