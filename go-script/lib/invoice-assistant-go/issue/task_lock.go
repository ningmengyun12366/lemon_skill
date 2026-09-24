package issue

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrTaskLockTimeout = errors.New("invoice task lock timeout")

var ErrTaskFileHasMultipleHardlinks = errors.New("invoice task file has multiple hardlinks; refusing unsafe cross-process issue")

var taskLockContentionHook = func() {}

type taskFileLock struct {
	lockFile *os.File
	taskFile *os.File
}

type taskFileIdentity struct {
	key       string
	linkCount uint64
}

func acquireTaskFileLock(ctx context.Context, taskPath string) (*taskFileLock, error) {
	taskFile, identity, err := openTaskFileIdentity(taskPath)
	if err != nil {
		return nil, err
	}
	lockPath, err := taskLockPath(identity.key)
	if err != nil {
		_ = taskFile.Close()
		return nil, err
	}
	lockFile, err := openTaskLockFile(lockPath)
	if err != nil {
		_ = taskFile.Close()
		return nil, err
	}
	for {
		locked, lockErr := tryLockTaskFile(lockFile)
		if lockErr != nil {
			_ = lockFile.Close()
			_ = taskFile.Close()
			return nil, fmt.Errorf("lock invoice task: %w", lockErr)
		}
		if locked {
			return &taskFileLock{lockFile: lockFile, taskFile: taskFile}, nil
		}
		taskLockContentionHook()
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = lockFile.Close()
			_ = taskFile.Close()
			return nil, fmt.Errorf("%w: %v", ErrTaskLockTimeout, ctx.Err())
		case <-timer.C:
		}
	}
}

func (lock *taskFileLock) release() error {
	unlockErr := unlockTaskFile(lock.lockFile)
	lockCloseErr := lock.lockFile.Close()
	taskCloseErr := lock.taskFile.Close()
	return errors.Join(unlockErr, lockCloseErr, taskCloseErr)
}

func taskFileIdentityForPath(taskPath string) (string, error) {
	file, identity, err := openTaskFileIdentity(taskPath)
	if err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close invoice task identity handle: %w", err)
	}
	return identity.key, nil
}

func taskLockPathForTask(taskPath string) (string, error) {
	identity, err := taskFileIdentityForPath(taskPath)
	if err != nil {
		return "", err
	}
	return taskLockPath(identity)
}

func openTaskFileIdentity(taskPath string) (*os.File, taskFileIdentity, error) {
	file, err := os.Open(taskPath)
	if err != nil {
		return nil, taskFileIdentity{}, fmt.Errorf("open invoice task for locking: %w", err)
	}
	identity, err := readTaskFileIdentity(file)
	if err != nil {
		_ = file.Close()
		return nil, taskFileIdentity{}, fmt.Errorf("read invoice task file identity: %w", err)
	}
	if identity.linkCount > 1 {
		_ = file.Close()
		return nil, taskFileIdentity{}, fmt.Errorf("%w (links=%d)", ErrTaskFileHasMultipleHardlinks, identity.linkCount)
	}
	return file, identity, nil
}

func taskLockPath(identity string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve invoice task lock cache: %w", err)
	}
	lockDir := filepath.Join(cacheDir, "invoice-assistant-skill", "task-locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return "", fmt.Errorf("create invoice task lock directory: %w", err)
	}
	if err := os.Chmod(lockDir, 0o700); err != nil {
		return "", fmt.Errorf("secure invoice task lock directory: %w", err)
	}
	info, err := os.Lstat(lockDir)
	if err != nil {
		return "", fmt.Errorf("inspect invoice task lock directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("invoice task lock path is not a secure directory")
	}
	digest := sha256.Sum256([]byte(identity))
	return filepath.Join(lockDir, fmt.Sprintf("%x.lock", digest)), nil
}

func openTaskLockFile(lockPath string) (*os.File, error) {
	if info, err := os.Lstat(lockPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("invoice task lock path is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect invoice task lock file: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open invoice task lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure invoice task lock: %w", err)
	}
	return file, nil
}
