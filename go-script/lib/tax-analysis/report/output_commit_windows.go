//go:build windows

package report

import "golang.org/x/sys/windows"

// renameNoReplace atomically moves a completed temporary report into place
// without replacing an existing file. Parameters: source is the temporary file
// in the output directory; destination is the selected final path. Return value:
// nil only when the move succeeds and destination did not already exist.
func renameNoReplace(source, destination string) error {
	sourcePointer, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPointer, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFile(sourcePointer, destinationPointer)
}
