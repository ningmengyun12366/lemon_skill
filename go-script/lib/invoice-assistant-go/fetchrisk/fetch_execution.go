package fetchrisk

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

var newAPIClient = common.NewAPIClient

var ErrFetchTaskAlreadyRunning = errors.New("fetch task is already running")

type FetchCommandOptions struct {
	ConfirmDuplicate      bool
	Client                *common.APIClient
	TaskLockTimeout       time.Duration
	SubmissionLockTimeout time.Duration
	PollInterval          time.Duration
	PollTimeout           time.Duration
	Now                   func() time.Time
}

type fetchRuntime struct {
	confirmDuplicate      bool
	client                *common.APIClient
	taskLockTimeout       time.Duration
	submissionLockTimeout time.Duration
	pollInterval          time.Duration
	pollTimeout           time.Duration
	now                   func() time.Time
}

/**
 * bindFetchTaskEnterprise 校验任务快照并为所有取票调用派生企业专用客户端。
 *
 * @param state 已加载且已加锁的取票任务。
 * @param execution 当前命令的运行参数。
 * @returns 校验通过后的企业快照。
 */
func bindFetchTaskEnterprise(state *TaskState, execution *fetchRuntime) (map[string]any, error) {
	enterpriseContext, err := common.EnterpriseContextFromTask(state.Data)
	if err != nil {
		return nil, err
	}
	params, err := IntentParams(state.Data)
	if err != nil {
		return nil, err
	}
	taskUSCC := strings.TrimSpace(common.StringValue(params["uscc"]))
	contextUSCC := strings.TrimSpace(common.StringValue(enterpriseContext["uscc"]))
	if taskUSCC == "" || taskUSCC != contextUSCC {
		return nil, fmt.Errorf("task intent uscc=%s does not match enterprise_context uscc=%s", taskUSCC, contextUSCC)
	}
	requestContext, err := common.EnterpriseRequestContextFromSnapshot(enterpriseContext, common.CapabilityCode)
	if err != nil {
		return nil, err
	}
	execution.client = common.NewEnterpriseAPIClient(execution.client, common.LoadWorkspaceConfig(), requestContext)
	return enterpriseContext, nil
}

func resolveFetchRuntime(options FetchCommandOptions) fetchRuntime {
	client := options.Client
	if client == nil {
		client = newAPIClient()
	}
	taskLockTimeout := options.TaskLockTimeout
	if taskLockTimeout <= 0 {
		taskLockTimeout = 500 * time.Millisecond
	}
	submissionLockTimeout := options.SubmissionLockTimeout
	if submissionLockTimeout <= 0 {
		submissionLockTimeout = 5 * time.Second
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	pollTimeout := options.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = common.MaxPollDuration
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return fetchRuntime{
		confirmDuplicate:      options.ConfirmDuplicate,
		client:                client,
		taskLockTimeout:       taskLockTimeout,
		submissionLockTimeout: submissionLockTimeout,
		pollInterval:          pollInterval,
		pollTimeout:           pollTimeout,
		now:                   now,
	}
}

func acquireFetchTaskCommandLock(taskFile string, execution fetchRuntime) (*common.ProcessLock, string, error) {
	abs, err := filepath.Abs(taskFile)
	if err != nil {
		return nil, "", err
	}
	key := filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	ctx, cancel := context.WithTimeout(context.Background(), execution.taskLockTimeout)
	defer cancel()
	lock, err := common.AcquireProcessLock(ctx, "invoice-fetch-task", key)
	if err != nil {
		if errors.Is(err, common.ErrProcessLockTimeout) {
			return nil, "", fmt.Errorf("%w：原取票任务仍在执行，请等待原任务完成后继续查询，不得重复发起", ErrFetchTaskAlreadyRunning)
		}
		return nil, "", fmt.Errorf("锁定取票任务失败: %w", err)
	}
	return lock, abs, nil
}
