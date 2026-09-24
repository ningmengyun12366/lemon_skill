//go:build !windows

package report

import (
	"os"
	"syscall"
)

func lockWorkspaceFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
func replaceWorkspaceFile(source, destination string) error { return os.Rename(source, destination) }
