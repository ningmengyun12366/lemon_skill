package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func AppendMessageLog(strs ...string) {
	message := strings.Join(strs, " ")
	fmt.Print(message)
	AppendLogEntry(map[string]any{
		"timestamp": time.Now().Format("2006-01-02T15:04:05"),
		"message":   message,
	})
}

func AppendAPILog(entry map[string]any) {
	if entry == nil {
		return
	}
	if _, ok := entry["timestamp"]; !ok {
		entry["timestamp"] = time.Now().Format("2006-01-02T15:04:05")
	}
	AppendLogEntry(entry)
}

func AppendLogEntry(entry map[string]any) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_ = appendFile(apiLogPath(time.Now()), data)
}

func apiLogPath(now time.Time) string {
	return filepath.Join(DefaultWorkspaceDir(), "logs", "api-"+now.Format("2006-01-02")+".log")
}

func appendFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}
