package common

import (
	"errors"
	"fmt"
	"math"
	"time"
)

const MaxPollDuration = 30 * time.Minute

type TaskPollErrorKind string

const (
	TaskPollRequestError TaskPollErrorKind = "request"
	TaskPollFailed       TaskPollErrorKind = "failed"
	TaskPollTimeout      TaskPollErrorKind = "timeout"
)

type TaskPollError struct {
	Kind   TaskPollErrorKind
	Latest map[string]any
	Err    error
	msg    string
}

func (err *TaskPollError) Error() string {
	if err == nil {
		return ""
	}
	if err.msg != "" {
		return err.msg
	}
	if err.Err != nil {
		return err.Err.Error()
	}
	return string(err.Kind)
}

func (err *TaskPollError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func TaskPollErrorKindOf(err error) (TaskPollErrorKind, bool) {
	var pollErr *TaskPollError
	if !errors.As(err, &pollErr) {
		return "", false
	}
	return pollErr.Kind, true
}

func CalculatePollDelay(attempt int, initial time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if initial <= 0 {
		initial = time.Second
	}
	return time.Duration(math.Pow(2, float64(attempt-1))) * initial
}

func PollTaskStatus(client *APIClient, statusURL string, taskID int, uscc, failedMessage, timeoutMessage string, pollInterval, timeout time.Duration) (map[string]any, error) {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if timeout <= 0 {
		timeout = MaxPollDuration
	}
	deadline := time.Now().Add(timeout)
	var latest map[string]any
	attempt := 1
	for time.Now().Before(deadline) || time.Now().Equal(deadline) {
		resp, err := client.PostSuccessJSON(statusURL, map[string]any{"taskID": taskID, "uscc": uscc}, "query task status", 20*time.Second)
		if err != nil {
			return nil, &TaskPollError{Kind: TaskPollRequestError, Latest: latest, Err: err}
		}
		latest = resp
		data, _ := resp["data"].(map[string]any)
		status := intValue(data["status"])
		if status == 1 {
			return resp, nil
		}
		if status == -1 {
			message := fmt.Sprintf("%s: %v", failedMessage, data["msg"])
			return resp, &TaskPollError{Kind: TaskPollFailed, Latest: resp, msg: message}
		}
		delay := CalculatePollDelay(attempt, pollInterval)
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if delay > remaining {
			delay = remaining
		}
		time.Sleep(delay)
		attempt++
	}
	message := fmt.Sprintf("%s not completed within %s; latest=%v", timeoutMessage, timeout, latest)
	return latest, &TaskPollError{Kind: TaskPollTimeout, Latest: latest, msg: message}
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		var out int
		_, _ = fmt.Sscan(v, &out)
		return out
	default:
		return 0
	}
}
