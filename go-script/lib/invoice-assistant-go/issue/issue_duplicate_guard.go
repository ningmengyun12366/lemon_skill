package issue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const duplicateGuardKey = "duplicate_guard"

const duplicateIssueWindow = 5 * time.Minute

type duplicateIssueMatch struct {
	Fingerprint     string
	MatchingTaskID  string
	MatchingStatus  string
	MatchingIssueID int
	MatchState      string
}

func canonicalIssueFingerprint(payload map[string]any) (string, error) {
	canonical := canonicalizeIssueFingerprintValue("", payload)
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("calculate invoice content fingerprint: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func requireNoDuplicateIssueSubmission(state *TaskState, payload map[string]any, confirmDuplicate bool) error {
	match, err := findDuplicateIssueTask(state, payload)
	if err != nil {
		return err
	}
	if match == nil {
		return nil
	}
	if confirmDuplicate {
		message := duplicateWindowConfirmedMessage(match)
		recordDuplicateGuard(state.Data, match, false, true, message)
		return nil
	}
	message := duplicateWindowBlockMessage(match)
	recordDuplicateGuard(state.Data, match, true, false, message)
	return fmt.Errorf("%s", message)
}

func findDuplicateIssueTask(state *TaskState, payload map[string]any) (*duplicateIssueMatch, error) {
	fingerprint, err := canonicalIssueFingerprint(payload)
	if err != nil {
		return nil, err
	}
	currentTime, ok := duplicateIssueTaskTimestamp(state.Data)
	if !ok {
		return nil, fmt.Errorf("current invoice task is missing duplicate guard timestamp")
	}
	workspace := strings.TrimSpace(common.StringValue(state.Data["workspace_dir"]))
	if workspace == "" {
		workspace = filepath.Dir(state.Path)
	}
	tasksDir := filepath.Join(common.NormalizeWorkspaceDir(workspace), filepath.FromSlash(issueTasksDirName))
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("check historical invoice tasks: %w", err)
	}
	currentPath, err := filepath.Abs(state.Path)
	if err != nil {
		return nil, err
	}
	var completed *duplicateIssueMatch
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == currentTaskFilename {
			continue
		}
		path := filepath.Join(tasksDir, entry.Name())
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if samePath(absPath, currentPath) {
			continue
		}
		candidate, err := loadDuplicateIssueCandidate(absPath, fingerprint, currentTime)
		if err != nil {
			return nil, err
		}
		if candidate == nil {
			continue
		}
		if candidate.MatchState == "active" {
			return candidate, nil
		}
		if completed == nil {
			completed = candidate
		}
	}
	return completed, nil
}

func loadDuplicateIssueCandidate(path, fingerprint string, currentTime time.Time) (*duplicateIssueMatch, error) {
	data, err := common.ReadJSONFile(path)
	if err != nil {
		return nil, fmt.Errorf("check historical invoice tasks: %w", err)
	}
	candidateTime, ok := duplicateIssueTaskTimestamp(data)
	if !ok || !duplicateIssueWithinWindow(currentTime, candidateTime) {
		return nil, nil
	}
	payload, _ := GetPath(map[string]any(data), "check_result.invoice_payload_json").(map[string]any)
	if payload == nil {
		return nil, nil
	}
	candidateFingerprint, err := canonicalIssueFingerprint(payload)
	if err != nil {
		return nil, err
	}
	if candidateFingerprint != fingerprint {
		return nil, nil
	}
	matchState, relevant := duplicateIssueTaskState(data)
	if !relevant {
		return nil, nil
	}
	return &duplicateIssueMatch{
		Fingerprint:     fingerprint,
		MatchingTaskID:  firstNonEmpty(common.StringValue(data["task_id"]), fileStem(path)),
		MatchingStatus:  strings.TrimSpace(common.StringValue(data["task_status"])),
		MatchingIssueID: getRecordedIssueTaskID(data),
		MatchState:      matchState,
	}, nil
}

func duplicateIssueTaskState(data map[string]any) (string, bool) {
	status := strings.TrimSpace(common.StringValue(data["task_status"]))
	issueResult, _ := data["issue_result"].(map[string]any)
	completed := legacyPythonTruthy(issueResult["issue_completed"]) || duplicateIssueStatusCompleted(status)
	submitted := issueSubmissionStartedWithoutTaskID(data) || getRecordedIssueTaskID(data) > 0 || legacyPythonTruthy(getSubmissionGuard(data)["issue_api_called"])
	if completed && (submitted || duplicateIssueStatusCompleted(status)) {
		return "completed", true
	}
	if submitted || !duplicateIssueStatusTerminal(status) {
		return "active", true
	}
	return "", false
}

func duplicateIssueTaskTimestamp(data map[string]any) (time.Time, bool) {
	for _, key := range []string{"updated_at", "created_at"} {
		parsed, ok := parseDuplicateIssueTime(common.StringValue(data[key]))
		if ok {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseDuplicateIssueTime(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02T15:04:05", time.RFC3339, time.RFC3339Nano} {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func duplicateIssueWithinWindow(currentTime, candidateTime time.Time) bool {
	delta := currentTime.Sub(candidateTime)
	if delta < 0 {
		delta = -delta
	}
	return delta <= duplicateIssueWindow
}

func duplicateIssueStatusCompleted(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "issued", "completed":
		return true
	default:
		return false
	}
}

func duplicateIssueStatusTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "issued", "completed", "issue_failed", "check_failed", "auth_failed", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func recordDuplicateGuard(taskState map[string]any, match *duplicateIssueMatch, blocked, confirmed bool, message string) {
	guard := map[string]any{
		"blocked":              blocked,
		"confirmed_duplicate":  confirmed,
		"content_fingerprint":  match.Fingerprint,
		"matching_task_id":     match.MatchingTaskID,
		"matching_task_status": match.MatchingStatus,
		"match_state":          match.MatchState,
		"message":              message,
	}
	if match.MatchingIssueID > 0 {
		guard["matching_issue_task_id"] = match.MatchingIssueID
	}
	issueResult := common.ObjectValue(taskState, "issue_result")
	issueResult[duplicateGuardKey] = guard
}

func duplicateWindowBlockMessage(match *duplicateIssueMatch) string {
	if match.MatchState == "completed" {
		return "\u5df2\u6709\u76f8\u540c\u53d1\u7968\u5185\u5bb9\u7684\u5f00\u7968\u4efb\u52a1\u5728 5 \u5206\u949f\u9632\u91cd\u590d\u7a97\u53e3\u5185\u5df2\u5b8c\u6210\uff1b\u5982\u786e\u8ba4\u9700\u8981\u4e8c\u6b21\u91cd\u590d\u5f00\u7968\uff0c\u8bf7\u8ffd\u52a0 --confirm-duplicate \u91cd\u65b0\u63d0\u4ea4\u3002"
	}
	return "\u5df2\u6709\u76f8\u540c\u53d1\u7968\u5185\u5bb9\u7684\u5f00\u7968\u4efb\u52a1\u5728 5 \u5206\u949f\u9632\u91cd\u590d\u7a97\u53e3\u5185\u6b63\u5728\u5904\u7406\u6216\u5df2\u63d0\u4ea4\uff1b\u5982\u786e\u8ba4\u9700\u8981\u4e8c\u6b21\u91cd\u590d\u5f00\u7968\uff0c\u8bf7\u8ffd\u52a0 --confirm-duplicate \u91cd\u65b0\u63d0\u4ea4\u3002"
}

func duplicateWindowConfirmedMessage(match *duplicateIssueMatch) string {
	if match.MatchState == "completed" {
		return "\u5df2\u8bb0\u5f55\u7528\u6237\u5bf9 5 \u5206\u949f\u9632\u91cd\u590d\u7a97\u53e3\u5185\u5df2\u5b8c\u6210\u76f8\u540c\u53d1\u7968\u5185\u5bb9\u7684\u4e8c\u6b21\u91cd\u590d\u5f00\u7968\u786e\u8ba4\uff0c\u672c\u6b21\u7ee7\u7eed\u63d0\u4ea4\u3002"
	}
	return "\u5df2\u8bb0\u5f55\u7528\u6237\u5bf9 5 \u5206\u949f\u9632\u91cd\u590d\u7a97\u53e3\u5185\u6b63\u5728\u5904\u7406\u6216\u5df2\u63d0\u4ea4\u7684\u76f8\u540c\u53d1\u7968\u5185\u5bb9\u7684\u4e8c\u6b21\u91cd\u590d\u5f00\u7968\u786e\u8ba4\uff0c\u672c\u6b21\u7ee7\u7eed\u63d0\u4ea4\u3002"
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
