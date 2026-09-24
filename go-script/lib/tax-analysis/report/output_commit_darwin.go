//go:build darwin

package report

import "golang.org/x/sys/unix"

// renameNoReplace atomically moves a completed temporary report into place
// without replacing an existing file. Parameters: source is the temporary file
// in the output directory; destination is the selected final path. Return value:
// nil only when the move succeeds and destination did not already exist.
func renameNoReplace(source, destination string) error {
	return unix.RenamexNp(source, destination, unix.RENAME_EXCL)
}
