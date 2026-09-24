package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const workspaceConfigLockTimeout = 5 * time.Second

type WorkspaceConfigPersistenceError struct {
	Stage string
	Err   error
}

func (err *WorkspaceConfigPersistenceError) Error() string {
	return fmt.Sprintf("persist workspace config (%s): %v", err.Stage, err.Err)
}

func (err *WorkspaceConfigPersistenceError) Unwrap() error { return err.Err }

type workspaceConfigUpdate struct {
	Path     string
	Snapshot map[string]any
	Mutate   func(map[string]any) error
}

func updateWorkspaceConfigTransaction(update workspaceConfigUpdate) error {
	ctx, cancel := context.WithTimeout(context.Background(), workspaceConfigLockTimeout)
	defer cancel()
	return updateWorkspaceConfigTransactionWithContext(ctx, update)
}

func updateWorkspaceConfigTransactionWithContext(ctx context.Context, update workspaceConfigUpdate) (retErr error) {
	if err := os.MkdirAll(filepath.Dir(update.Path), 0o755); err != nil {
		return workspaceConfigPersistenceError("lock", err)
	}
	lock, err := acquireWorkspaceConfigLock(ctx, update.Path+".lock")
	if err != nil {
		return workspaceConfigPersistenceError("lock", err)
	}
	defer func() {
		if releaseErr := lock.release(); releaseErr != nil {
			retErr = errors.Join(retErr, workspaceConfigPersistenceError("unlock", releaseErr))
		}
	}()

	latest, err := readLatestWorkspaceConfig(update.Path)
	if err != nil {
		return err
	}
	if err := update.Mutate(latest); err != nil {
		return workspaceConfigPersistenceError("mutate", err)
	}
	latest["updated_at"] = NowISO()
	if err := writeWorkspaceConfigAtomically(update.Path, latest); err != nil {
		return err
	}
	if update.Snapshot != nil {
		synchronizeConfigSnapshot(update.Snapshot, latest)
	}
	return nil
}

func readLatestWorkspaceConfig(path string) (map[string]any, error) {
	latest, err := ReadJSONFile(path)
	if err == nil {
		normalizeConfig(latest)
		return latest, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	return nil, workspaceConfigPersistenceError("read", err)
}

func synchronizeConfigSnapshot(snapshot, latest map[string]any) {
	for key := range snapshot {
		delete(snapshot, key)
	}
	for key, value := range latest {
		snapshot[key] = value
	}
}

func workspaceConfigPersistenceError(stage string, err error) error {
	return &WorkspaceConfigPersistenceError{Stage: stage, Err: err}
}
