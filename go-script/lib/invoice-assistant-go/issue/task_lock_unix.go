//go:build !windows

package issue

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func readTaskFileIdentity(file *os.File) (taskFileIdentity, error) {
	info, err := file.Stat()
	if err != nil {
		return taskFileIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return taskFileIdentity{}, fmt.Errorf("unsupported Unix file metadata %T", info.Sys())
	}
	return taskFileIdentity{
		key:       fmt.Sprintf("unix:%x:%x", uint64(stat.Dev), uint64(stat.Ino)),
		linkCount: uint64(stat.Nlink),
	}, nil
}

func tryLockTaskFile(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

func unlockTaskFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
