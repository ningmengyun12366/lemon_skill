package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

type workspaceConfigLock struct {
	file *os.File
}

func acquireWorkspaceConfigLock(ctx context.Context, path string) (*workspaceConfigLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		locked, lockErr := tryLockWorkspaceConfig(file)
		if lockErr != nil {
			return nil, errors.Join(lockErr, file.Close())
		}
		if locked {
			return &workspaceConfigLock{file: file}, nil
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, errors.Join(fmt.Errorf("workspace config lock timeout: %w", ctx.Err()), file.Close())
		case <-timer.C:
		}
	}
}

func (lock *workspaceConfigLock) release() error {
	return errors.Join(unlockWorkspaceConfig(lock.file), lock.file.Close())
}
