//go:build !windows && !linux && !darwin

package report

import (
	"errors"
	"runtime"
)

// renameNoReplace rejects platforms without a supported atomic no-replace
// rename primitive. Parameters: source and destination are unused paths. Return
// value: an explicit unsupported-platform error.
func renameNoReplace(source, destination string) error {
	return errors.New("atomic report commit is unsupported on " + runtime.GOOS)
}
