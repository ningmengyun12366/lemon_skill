package common

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrProcessLockTimeout = errors.New("process lock timeout")

// ProcessLock is a cross-process advisory lock keyed by a namespace and value.
// Lock files live in the user cache rather than in a task workspace.
type ProcessLock struct {
	lock *workspaceConfigLock
}

func AcquireProcessLock(ctx context.Context, namespace, key string) (*ProcessLock, error) {
	lockPath, err := processLockPath(namespace, key)
	if err != nil {
		return nil, err
	}
	lock, err := acquireWorkspaceConfigLock(ctx, lockPath)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("%w: %v", ErrProcessLockTimeout, err)
		}
		return nil, err
	}
	return &ProcessLock{lock: lock}, nil
}

func (lock *ProcessLock) Release() error {
	if lock == nil || lock.lock == nil {
		return nil
	}
	return lock.lock.release()
}

func processLockPath(namespace, key string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve process lock cache: %w", err)
	}
	lockDir := filepath.Join(cacheDir, "invoice-assistant-skill", "process-locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return "", fmt.Errorf("create process lock directory: %w", err)
	}
	if err := os.Chmod(lockDir, 0o700); err != nil {
		return "", fmt.Errorf("secure process lock directory: %w", err)
	}
	info, err := os.Lstat(lockDir)
	if err != nil {
		return "", fmt.Errorf("inspect process lock directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("process lock path is not a secure directory")
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(namespace) + "\x00" + key))
	lockPath := filepath.Join(lockDir, fmt.Sprintf("%x.lock", digest))
	if lockInfo, statErr := os.Lstat(lockPath); statErr == nil {
		if lockInfo.Mode()&os.ModeSymlink != 0 || !lockInfo.Mode().IsRegular() {
			return "", fmt.Errorf("process lock path is not a regular file")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect process lock file: %w", statErr)
	}
	return lockPath, nil
}
