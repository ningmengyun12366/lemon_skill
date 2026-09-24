package verifyapi

import (
	"context"
	"strings"
	"time"
)

const (
	rateLimitedCode   = 3999
	retryLaterMessage = "请稍后重试"
)

type retryCategory int

const (
	retryNone retryCategory = iota
	retryRateLimit
	retryLater
	retryHTTPError
)

func retryCategoryForEnvelope(envelope apiEnvelope) retryCategory {
	if envelope.Code == rateLimitedCode {
		return retryRateLimit
	}
	if strings.Contains(strings.TrimSpace(envelope.Message), retryLaterMessage) {
		return retryLater
	}
	return retryNone
}

func apiFailureMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		trimmed = "未知错误"
	}
	return "查验失败: " + trimmed
}

func defaultRetryDelay(_ int, category retryCategory) time.Duration {
	if category == retryRateLimit {
		return 30 * time.Second
	}
	return 2 * time.Second
}

func sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
