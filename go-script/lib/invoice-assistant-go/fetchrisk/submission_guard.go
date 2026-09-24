package fetchrisk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const fetchDuplicateGuardKey = "duplicate_guard"

const (
	fetchInvoicesStep    = "fetch_invoices"
	fetchAttachmentsStep = "fetch_attachments"
	fetchDeductionStep   = "fetch_deduction"
)

var guardedFetchSteps = []string{fetchInvoicesStep, fetchAttachmentsStep, fetchDeductionStep}

type fetchSubmissionSpec struct {
	Step          string
	IDKey         string
	CreateURL     string
	CreateAction  string
	CreateTimeout time.Duration
	BuildPayload  func(map[string]any, time.Time) (map[string]any, error)
}

type fetchSubmissionMatch struct {
	TaskFile     string
	LocalTaskID  string
	RemoteTaskID int
	Status       string
	Fingerprint  string
	StartedAt    time.Time
	Unknown      bool
}

func invoiceSubmissionSpec() fetchSubmissionSpec {
	return fetchSubmissionSpec{
		Step: fetchInvoicesStep, IDKey: "sync_task_id", CreateURL: createInvoiceTaskURL,
		CreateAction: "创建发票同步任务", CreateTimeout: 30 * time.Second,
		BuildPayload: buildInvoiceCreatePayload,
	}
}

func attachmentSubmissionSpec() fetchSubmissionSpec {
	return fetchSubmissionSpec{
		Step: fetchAttachmentsStep, IDKey: "attachment_task_id", CreateURL: createAttachmentTaskURL,
		CreateAction: "创建附件下载任务", CreateTimeout: 30 * time.Second,
		BuildPayload: buildAttachmentCreatePayload,
	}
}

func deductionSubmissionSpec() fetchSubmissionSpec {
	return fetchSubmissionSpec{
		Step: fetchDeductionStep, IDKey: "deduction_task_id", CreateURL: createDeductionTaskURL,
		CreateAction: "创建勾选数据任务", CreateTimeout: 20 * time.Second,
		BuildPayload: buildDeductionCreatePayload,
	}
}

func buildInvoiceCreatePayload(params map[string]any, _ time.Time) (map[string]any, error) {
	uscc, err := RequiredString(params, "uscc")
	if err != nil {
		return nil, err
	}
	areaCode, err := RequiredInt(params, "areaCode")
	if err != nil {
		return nil, err
	}
	personalAccount, err := RequiredString(params, "personalAccount")
	if err != nil {
		return nil, err
	}
	start, err := RequiredString(params, "invoiceStartTime")
	if err != nil {
		return nil, err
	}
	end, err := RequiredString(params, "invoiceEndTime")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"uscc": uscc, "areaCode": areaCode, "personalAccount": personalAccount,
		"syncTaskType": 500, "invoiceStartTime": start, "invoiceEndTime": end,
	}, nil
}

func buildAttachmentCreatePayload(params map[string]any, _ time.Time) (map[string]any, error) {
	uscc, err := RequiredString(params, "uscc")
	if err != nil {
		return nil, err
	}
	areaCode, err := RequiredInt(params, "areaCode")
	if err != nil {
		return nil, err
	}
	personalAccount, err := RequiredString(params, "personalAccount")
	if err != nil {
		return nil, err
	}
	start, err := RequiredString(params, "invoiceStartTime")
	if err != nil {
		return nil, err
	}
	end, err := RequiredString(params, "invoiceEndTime")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"uscc": uscc, "areaCode": areaCode, "personalAccount": personalAccount,
		"invoiceStartDate": start, "invoiceEndDate": end,
	}, nil
}

func buildDeductionCreatePayload(params map[string]any, now time.Time) (map[string]any, error) {
	uscc, err := RequiredString(params, "uscc")
	if err != nil {
		return nil, err
	}
	areaCode, err := RequiredInt(params, "areaCode")
	if err != nil {
		return nil, err
	}
	personalAccount, err := RequiredString(params, "personalAccount")
	if err != nil {
		return nil, err
	}
	start, err := RequiredString(params, "invoiceStartTime")
	if err != nil {
		return nil, err
	}
	skssqStart, err := NormalizeMonthValue(start, "invoiceStartTime")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"uscc": uscc, "areaCode": areaCode, "personalAccount": personalAccount,
		"skssqStart": skssqStart, "skssqEnd": now.Format("200601"), "needInvoiceItemVO": true,
	}, nil
}

func fetchSubmissionFingerprint(step string, payload map[string]any) (string, error) {
	raw, err := json.Marshal(map[string]any{"step": step, "payload": payload})
	if err != nil {
		return "", fmt.Errorf("计算取票任务指纹失败: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func acquireFetchSubmissionCommandLock(spec fetchSubmissionSpec, payload map[string]any, execution fetchRuntime) (*common.ProcessLock, string, error) {
	fingerprint, err := fetchSubmissionFingerprint(spec.Step, payload)
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), execution.submissionLockTimeout)
	defer cancel()
	lock, err := common.AcquireProcessLock(ctx, "invoice-fetch-submission", fingerprint)
	if err != nil {
		if errors.Is(err, common.ErrProcessLockTimeout) {
			return nil, "", fmt.Errorf("相同取票任务仍在执行，请继续等待并查询原任务，不得重复发起")
		}
		return nil, "", fmt.Errorf("锁定取票提交失败: %w", err)
	}
	return lock, fingerprint, nil
}

func ensureFetchRemoteTask(state *TaskState, spec fetchSubmissionSpec, payload map[string]any, fingerprint string, execution fetchRuntime) (taskID int, retErr error) {
	executionState := common.ObjectValue(state.Data, "execution")
	stepState := common.ObjectValue(executionState, spec.Step)
	currentTaskID := intFrom(stepState[spec.IDKey])
	currentStatus := strings.TrimSpace(common.StringValue(stepState["status"]))
	if currentTaskID > 0 && fetchStepTerminal(currentStatus) {
		common.AppendMessageLog(fmt.Sprintf("当前任务已记录远端任务ID %d，继续复用原任务结果，不重复发起", currentTaskID))
		return currentTaskID, nil
	}
	current := matchFetchSubmission(state.Path, state.Data, spec, fingerprint, execution.now())

	match := current
	if match == nil || match.RemoteTaskID == 0 {
		historical, findErr := findActiveFetchSubmission(state, spec, fingerprint, execution.now())
		if findErr != nil {
			return 0, findErr
		}
		if historical != nil && (match == nil || historical.RemoteTaskID > 0) {
			match = historical
		}
	}
	forceDuplicate := false
	if match != nil {
		forceDuplicate = authorizeAndConsumeDuplicate(state.Data, spec.Step, execution.confirmDuplicate, execution.now())
		if !forceDuplicate && match.RemoteTaskID > 0 {
			adoptFetchSubmission(state.Data, stepState, spec, fingerprint, match, execution.now())
			if err := state.Save(); err != nil {
				return 0, err
			}
			common.AppendMessageLog(fmt.Sprintf("发现相同参数的未完成%s，复用远端任务ID %d 并继续查询，不重复发起", spec.CreateAction, match.RemoteTaskID))
			return match.RemoteTaskID, nil
		}
		if !forceDuplicate {
			message := "相同参数的取票任务已发起，但未取得可续查的远端 task ID；不得自动重复发起。只有用户明确确认一定要重新发起后，才能追加 --confirm-duplicate"
			recordFetchDuplicateGuard(state.Data, match, spec.Step, fingerprint, "confirmation_required", true, message, execution.now())
			if err := state.Save(); err != nil {
				return 0, err
			}
			return 0, fmt.Errorf("%s", message)
		}
	}

	if forceDuplicate && match != nil {
		archiveRemoteTaskID(stepState, match.RemoteTaskID)
		delete(stepState, spec.IDKey)
		previousGuard := common.ObjectValue(stepState, "submission_guard")
		delete(previousGuard, "remote_task_id")
		delete(previousGuard, "task_id_saved_at")
		recordFetchDuplicateGuard(state.Data, match, spec.Step, fingerprint, "forced_duplicate", false, "用户已确认重新发起取票流程", execution.now())
	}
	submissionGuard := common.ObjectValue(stepState, "submission_guard")
	submissionGuard["fingerprint"] = fingerprint
	submissionGuard["create_api_called"] = true
	submissionGuard["started_at"] = execution.now().Format(time.RFC3339)
	delete(submissionGuard, "last_error")
	stepState["status"] = "submitting"
	if err := state.Save(); err != nil {
		return 0, err
	}

	resp, err := execution.client.PostSuccessJSON(spec.CreateURL, payload, spec.CreateAction, spec.CreateTimeout)
	if err != nil {
		stepState["status"] = "submission_unknown"
		submissionGuard["last_error"] = err.Error()
		submissionGuard["last_error_at"] = execution.now().Format(time.RFC3339)
		return 0, errors.Join(err, state.Save())
	}
	data, _ := resp["data"].(map[string]any)
	taskID = intFrom(firstNonNil(data["taskID"], data["taskId"]))
	if taskID == 0 {
		err = fmt.Errorf("%s成功但未返回 taskID", spec.CreateAction)
		stepState["status"] = "submission_unknown"
		submissionGuard["last_error"] = err.Error()
		submissionGuard["last_error_at"] = execution.now().Format(time.RFC3339)
		return 0, errors.Join(err, state.Save())
	}
	stepState[spec.IDKey] = taskID
	stepState["status"] = "processing"
	submissionGuard["remote_task_id"] = taskID
	submissionGuard["task_id_saved_at"] = execution.now().Format(time.RFC3339)
	if err := state.Save(); err != nil {
		return 0, err
	}
	return taskID, nil
}

func findActiveFetchSubmission(state *TaskState, spec fetchSubmissionSpec, fingerprint string, now time.Time) (*fetchSubmissionMatch, error) {
	workspace := common.NormalizeWorkspaceDir(common.StringValue(state.Data["workspace_dir"]))
	tasksDir := filepath.Join(workspace, filepath.FromSlash(TasksDirName))
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("检查历史取票任务失败: %w", err)
	}
	currentPath, _ := filepath.Abs(state.Path)
	var known *fetchSubmissionMatch
	var unknown *fetchSubmissionMatch
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(tasksDir, entry.Name())
		abs, _ := filepath.Abs(path)
		if sameFetchPath(abs, currentPath) {
			continue
		}
		candidate, err := common.ReadJSONFile(abs)
		if err != nil {
			return nil, fmt.Errorf("读取历史取票任务 %s 失败，已停止提交: %w", abs, err)
		}
		if scene := strings.TrimSpace(common.StringValue(candidate["scene_id"])); scene != "" && scene != SceneID {
			continue
		}
		match := matchFetchSubmission(abs, candidate, spec, fingerprint, now)
		if match == nil {
			continue
		}
		if match.RemoteTaskID > 0 {
			if known == nil || match.StartedAt.After(known.StartedAt) {
				known = match
			}
		} else if unknown == nil || match.StartedAt.After(unknown.StartedAt) {
			unknown = match
		}
	}
	if known != nil {
		return known, nil
	}
	return unknown, nil
}

func matchFetchSubmission(taskFile string, data map[string]any, spec fetchSubmissionSpec, fingerprint string, now time.Time) *fetchSubmissionMatch {
	execution, _ := data["execution"].(map[string]any)
	stepState, _ := execution[spec.Step].(map[string]any)
	if stepState == nil {
		return nil
	}
	status := strings.TrimSpace(common.StringValue(stepState["status"]))
	if fetchStepTerminal(status) {
		return nil
	}
	submissionGuard, _ := stepState["submission_guard"].(map[string]any)
	candidateFingerprint := strings.TrimSpace(common.StringValue(submissionGuard["fingerprint"]))
	remoteTaskID := intFrom(stepState[spec.IDKey])
	apiCalled := boolFromAny(submissionGuard["create_api_called"])
	if candidateFingerprint == "" && (remoteTaskID > 0 || apiCalled) {
		params, err := IntentParams(data)
		if err == nil {
			payload, buildErr := spec.BuildPayload(params, now)
			if buildErr == nil {
				candidateFingerprint, _ = fetchSubmissionFingerprint(spec.Step, payload)
			}
		}
	}
	if candidateFingerprint != fingerprint || (remoteTaskID == 0 && !apiCalled) {
		return nil
	}
	startedAt := firstFetchTime(submissionGuard["started_at"], data["updated_at"], data["created_at"])
	return &fetchSubmissionMatch{
		TaskFile: taskFile, LocalTaskID: common.StringValue(data["task_id"]), RemoteTaskID: remoteTaskID,
		Status: status, Fingerprint: fingerprint, StartedAt: startedAt, Unknown: remoteTaskID == 0,
	}
}

func adoptFetchSubmission(taskData, stepState map[string]any, spec fetchSubmissionSpec, fingerprint string, match *fetchSubmissionMatch, now time.Time) {
	stepState[spec.IDKey] = match.RemoteTaskID
	stepState["status"] = "processing"
	submissionGuard := common.ObjectValue(stepState, "submission_guard")
	submissionGuard["fingerprint"] = fingerprint
	submissionGuard["remote_task_id"] = match.RemoteTaskID
	submissionGuard["resumed_from_task_id"] = match.LocalTaskID
	submissionGuard["resumed_from_task_file"] = match.TaskFile
	submissionGuard["resumed_at"] = now.Format(time.RFC3339)
	recordFetchDuplicateGuard(taskData, match, spec.Step, fingerprint, "resume_existing", false, "已接管原远端任务并继续查询", now)
}

func fetchStepTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case "success", "partial_success", "failed", "skipped":
		return true
	default:
		return false
	}
}

func authorizeAndConsumeDuplicate(taskData map[string]any, step string, explicit bool, now time.Time) bool {
	guard := common.ObjectValue(taskData, fetchDuplicateGuardKey)
	consumed := common.ObjectValue(guard, "consumed_steps")
	if boolFromAny(consumed[step]) {
		return false
	}
	if explicit && !boolFromAny(guard["confirmed_duplicate"]) {
		guard["confirmed_duplicate"] = true
		guard["confirmed_scope"] = "workflow"
		guard["confirmed_at"] = now.Format(time.RFC3339)
		authorized := common.ObjectValue(guard, "authorized_steps")
		for _, guardedStep := range guardedFetchSteps {
			authorized[guardedStep] = true
		}
	}
	authorized := common.ObjectValue(guard, "authorized_steps")
	if !boolFromAny(authorized[step]) {
		return false
	}
	consumed[step] = true
	guard["last_consumed_step"] = step
	guard["last_consumed_at"] = now.Format(time.RFC3339)
	return true
}

func recordFetchDuplicateGuard(taskData map[string]any, match *fetchSubmissionMatch, step, fingerprint, action string, blocked bool, message string, now time.Time) {
	guard := common.ObjectValue(taskData, fetchDuplicateGuardKey)
	guard["action"] = action
	guard["blocked"] = blocked
	guard["step"] = step
	guard["fingerprint"] = fingerprint
	guard["message"] = message
	guard["checked_at"] = now.Format(time.RFC3339)
	if match == nil {
		return
	}
	guard["matching_task_id"] = match.LocalTaskID
	guard["matching_task_file"] = match.TaskFile
	guard["matching_status"] = match.Status
	guard["matching_remote_task_id"] = match.RemoteTaskID
}

func archiveRemoteTaskID(stepState map[string]any, taskID int) {
	if taskID <= 0 {
		return
	}
	seen := map[int]bool{}
	values := []int{}
	for _, item := range anySlice(stepState["historical_remote_task_ids"]) {
		id := intFrom(item)
		if id > 0 && !seen[id] {
			seen[id] = true
			values = append(values, id)
		}
	}
	if !seen[taskID] {
		values = append(values, taskID)
	}
	sort.Ints(values)
	out := make([]any, 0, len(values))
	for _, id := range values {
		out = append(out, id)
	}
	stepState["historical_remote_task_ids"] = out
}

func markFetchPollingResult(state *TaskState, step string, pollErr error, now time.Time) error {
	stepState := common.ObjectValue(common.ObjectValue(state.Data, "execution"), step)
	guard := common.ObjectValue(stepState, "submission_guard")
	guard["last_polled_at"] = now.Format(time.RFC3339)
	if pollErr == nil {
		guard["remote_status"] = "success"
		delete(guard, "last_poll_error")
		return state.Save()
	}
	guard["last_poll_error"] = pollErr.Error()
	kind, _ := common.TaskPollErrorKindOf(pollErr)
	guard["last_poll_error_kind"] = string(kind)
	if kind == common.TaskPollFailed {
		stepState["status"] = "failed"
	} else {
		stepState["status"] = "processing"
	}
	return errors.Join(pollErr, state.Save())
}

func firstFetchTime(values ...any) time.Time {
	for _, value := range values {
		text := strings.TrimSpace(common.StringValue(value))
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}

func sameFetchPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
