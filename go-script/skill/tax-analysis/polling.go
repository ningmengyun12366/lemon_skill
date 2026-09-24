package main

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// validatePollingOptions validates user-provided polling durations and safely
// computes the command timeout. Parameters: interval is the delay between task
// queries, timeout is the maximum duration of one attempt, and attemptCount is
// the number of attempts covered by the command context. Return values: the
// total context timeout and an error when a duration is non-positive, the
// interval exceeds one attempt, or multiplication would overflow.
func validatePollingOptions(interval, timeout time.Duration, attemptCount int) (time.Duration, error) {
	if interval <= 0 {
		return 0, errors.New("轮询间隔必须大于 0")
	}
	if timeout <= 0 {
		return 0, errors.New("轮询超时时间必须大于 0")
	}
	if interval > timeout {
		return 0, errors.New("轮询间隔不得大于单次轮询超时时间")
	}
	if attemptCount <= 0 {
		return 0, fmt.Errorf("轮询尝试次数无效：%d", attemptCount)
	}
	if timeout > time.Duration(math.MaxInt64/int64(attemptCount)) {
		return 0, errors.New("轮询超时时间过大")
	}
	return timeout * time.Duration(attemptCount), nil
}
