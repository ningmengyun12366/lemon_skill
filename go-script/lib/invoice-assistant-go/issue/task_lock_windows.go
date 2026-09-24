//go:build windows

package issue

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func readTaskFileIdentity(file *os.File) (taskFileIdentity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return taskFileIdentity{}, err
	}
	return taskFileIdentity{
		key: fmt.Sprintf(
			"windows:%08x:%08x%08x",
			info.VolumeSerialNumber,
			info.FileIndexHigh,
			info.FileIndexLow,
		),
		linkCount: uint64(info.NumberOfLinks),
	}, nil
}

func tryLockTaskFile(file *os.File) (bool, error) {
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

func unlockTaskFile(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}
