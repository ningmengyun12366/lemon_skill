package report

import (
	"fmt"
	"os"
	"path/filepath"
)

// lockAnalysisWorkspace serializes local section writes and report rendering.
// The OS releases ownership on process exit; the persistent file is not a lease.
func lockAnalysisWorkspace(directory string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(directory, ".analysis-write.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockWorkspaceFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("工作区正在写入或渲染，请完成后重试：%w", err)
	}
	return func() { _ = f.Close() }, nil
}

// atomicWorkspaceWrite replaces one complete UTF-8 JSON document only after
// the temporary file is flushed. Failed writes retain the previous document.
func atomicWorkspaceWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".analysis-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceWorkspaceFile(f.Name(), path)
}
