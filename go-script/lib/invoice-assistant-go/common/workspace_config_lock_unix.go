//go:build !windows

package common

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func tryLockWorkspaceConfig(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

func unlockWorkspaceConfig(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func atomicReplaceWorkspaceConfigFile(source, destination string) error {
	return os.Rename(source, destination)
}

var replaceWorkspaceConfigFile = atomicReplaceWorkspaceConfigFile

func syncWorkspaceConfigParent(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
