package taxanalysis

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	invoicecommon "invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const requestPacingNamespace = "tax-analysis-http"

// requestSender carries the parent cancellation into the shared login client.
func (c *Client) requestSender(ctx context.Context) func(*http.Request, time.Duration) (*http.Response, error) {
	return func(req *http.Request, timeout time.Duration) (*http.Response, error) {
		return c.sendRequest(req.WithContext(ctx), timeout)
	}
}

// sendRequest serializes request starts across processes using the same origin
// and API Key. Queue time precedes the HTTP timeout; reading the body still has
// that timeout. The lock is released by the OS if a process exits unexpectedly.
func (c *Client) sendRequest(req *http.Request, timeout time.Duration) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	key, statePath, err := c.requestPacingState()
	if err != nil {
		return nil, err
	}
	lock, err := invoicecommon.AcquireProcessLock(req.Context(), requestPacingNamespace, key)
	if err != nil {
		if ctxErr := req.Context().Err(); ctxErr != nil {
			return nil, fmt.Errorf("等待接口调用间隔: %w", ctxErr)
		}
		return nil, fmt.Errorf("获取接口调用锁: %w", err)
	}
	if err := waitForRequestInterval(req.Context(), statePath, c.requestInterval); err != nil {
		return nil, errors.Join(err, lock.Release())
	}
	if err := req.Context().Err(); err != nil {
		return nil, errors.Join(err, lock.Release())
	}
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(statePath, []byte(stamp), 0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("保存接口调用时间: %w", err), lock.Release())
	}
	if timeout <= 0 {
		timeout = c.timeout
	}
	requestCtx, cancel := context.WithTimeout(req.Context(), timeout)
	client := *c.httpClient
	client.Timeout = timeout
	response, sendErr := client.Do(req.WithContext(requestCtx))
	releaseErr := lock.Release()
	if sendErr != nil || releaseErr != nil {
		cancel()
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, errors.Join(sendErr, releaseErr)
	}
	response.Body = &requestBody{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

func (c *Client) requestPacingState() (string, string, error) {
	origin, err := url.Parse(c.baseURL)
	if err != nil {
		return "", "", err
	}
	key := strings.ToLower(origin.Scheme+"://"+origin.Host) + "\x00" + c.apiKey
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("读取接口调用缓存目录: %w", err)
	}
	dir := filepath.Join(cacheDir, "invoice-assistant-skill", "tax-analysis-requests")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(key))
	return key, filepath.Join(dir, fmt.Sprintf("%x.last", digest)), nil
}

// waitForRequestInterval waits a full interval when requests are too close.
// An incomplete timestamp or a backwards clock also requires one full wait.
// The caller holds the cross-process lock, so no other sender can jump the queue.
func waitForRequestInterval(ctx context.Context, statePath string, interval time.Duration) error {
	raw, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取接口调用时间: %w", err)
	}
	nanos, parseErr := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if parseErr == nil && time.Since(time.Unix(0, nanos)) >= interval {
		return nil
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("等待接口调用间隔: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

type requestBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (body *requestBody) Close() error {
	defer body.cancel()
	return body.ReadCloser.Close()
}
