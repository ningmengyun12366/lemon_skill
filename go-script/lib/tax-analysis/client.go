package taxanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/skillupdate"
)

type ClientConfig struct {
	BaseURL           string
	APIKey            string
	HTTPClient        *http.Client
	Timeout           time.Duration
	VersionChecker    skillupdate.Checker
	AllowLoopbackHTTP bool
}

type Client struct {
	baseURL         string
	apiKey          string
	httpClient      *http.Client
	timeout         time.Duration
	versionChecker  skillupdate.Checker
	requestInterval time.Duration
}

type apiEnvelope struct {
	Code    *int            `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func NewClient(config ClientConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	apiKey := strings.TrimSpace(config.APIKey)
	if baseURL == "" {
		return nil, errors.New("missing tax analysis API base URL")
	}
	if apiKey == "" || strings.ContainsAny(apiKey, ";\r\n") {
		return nil, errors.New("invalid or missing API key")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return nil, errors.New("invalid tax analysis API base URL")
	}
	loopback := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !(config.AllowLoopbackHTTP && loopback) {
		return nil, errors.New("tax analysis API base URL must use HTTPS")
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:         baseURL,
		apiKey:          apiKey,
		httpClient:      httpClient,
		timeout:         timeout,
		versionChecker:  config.VersionChecker,
		requestInterval: MinimumRequestInterval,
	}, nil
}

// CreateTask starts or reuses one deterministic user-company-range task.
func (c *Client) CreateTask(ctx context.Context, request CreateTaskRequest) (TaskResponse, error) {
	if err := validateCreateRequest(request); err != nil {
		return TaskResponse{}, err
	}
	return c.post(ctx, CreateTaskPath, request.USCC, request)
}

// QueryTask returns the current partial result without extending Redis TTL.
func (c *Client) QueryTask(ctx context.Context, request QueryTaskRequest) (TaskResponse, error) {
	if strings.TrimSpace(request.USCC) == "" || strings.TrimSpace(request.TaskID) == "" {
		return TaskResponse{}, errors.New("uscc and taskId are required")
	}
	return c.post(ctx, QueryTaskPath, request.USCC, request)
}

// Run creates first, then polls query until a terminal status or context timeout.
func (c *Client) Run(ctx context.Context, request CreateTaskRequest, interval time.Duration) (TaskResponse, error) {
	created, err := c.CreateTask(ctx, request)
	if err != nil || created.Terminal() {
		return created, err
	}
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return created, ctx.Err()
		case <-ticker.C:
			current, queryErr := c.QueryTask(ctx, QueryTaskRequest{USCC: request.USCC, TaskID: created.TaskID})
			if queryErr != nil {
				return created, queryErr
			}
			created = current
			if current.Terminal() {
				return current, nil
			}
		}
	}
}

func (c *Client) post(ctx context.Context, path string, uscc string, payload any) (TaskResponse, error) {
	if c.versionChecker != nil {
		if err := c.versionChecker.Check(ctx); err != nil {
			return TaskResponse{}, err
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TaskResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return TaskResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", buildAuthorization(c.apiKey, uscc))
	response, err := c.sendRequest(req, c.timeout)
	if err != nil {
		return TaskResponse{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return TaskResponse{}, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return TaskResponse{}, fmt.Errorf("tax analysis API returned HTTP %d", response.StatusCode)
	}
	return decodeTaskResponse(raw)
}

func decodeTaskResponse(raw []byte) (TaskResponse, error) {
	var envelope apiEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return TaskResponse{}, errors.New("tax analysis API returned non-JSON response")
	}
	if envelope.Code == nil {
		return TaskResponse{}, errors.New("tax analysis API response missing code")
	}
	if *envelope.Code != BusinessSuccessCode {
		if strings.TrimSpace(envelope.Message) == "" {
			envelope.Message = "tax analysis API business request failed"
		}
		return TaskResponse{}, &APIError{Code: *envelope.Code, Message: envelope.Message}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return TaskResponse{}, errors.New("tax analysis API response missing data")
	}
	var result TaskResponse
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return TaskResponse{}, errors.New("tax analysis API response contract mismatch")
	}
	if result.TaskID == "" {
		return TaskResponse{}, errors.New("tax analysis API response missing taskId")
	}
	if result.ModuleResults == nil {
		result.ModuleResults = []ModuleResult{}
	}
	if result.FailedItems == nil {
		result.FailedItems = []FailedItem{}
	}
	return result, nil
}

func buildAuthorization(apiKey, uscc string) string {
	return "apikey=" + strings.TrimSpace(apiKey) + "; uscc=" + strings.TrimSpace(uscc) + "; capabilityCode=" + DefaultCapabilityCode
}

func validateCreateRequest(request CreateTaskRequest) error {
	if strings.TrimSpace(request.USCC) == "" {
		return errors.New("uscc is required")
	}
	start, err := time.Parse(MonthLayout, request.StartMonth)
	if err != nil {
		return errors.New("startMonth must use YYYY-MM")
	}
	end, err := time.Parse(MonthLayout, request.EndMonth)
	if err != nil {
		return errors.New("endMonth must use YYYY-MM")
	}
	if end.Before(start) {
		return errors.New("endMonth cannot be earlier than startMonth")
	}
	if start.Year() < MinimumStartYear {
		return fmt.Errorf("startMonth cannot be earlier than %d-01", MinimumStartYear)
	}
	if inclusiveCalendarMonthCount(start, end) > MaximumRangeMonths {
		return fmt.Errorf("tax analysis month range cannot exceed %d inclusive months", MaximumRangeMonths)
	}
	return nil
}

// ValidateCreateTaskRequest 校验税务分析任务的企业和自然月范围，不发起网络请求。
//
// @param request 待创建的税务分析任务参数。
// @returns 参数满足契约时返回 nil，否则返回可读的校验错误。
func ValidateCreateTaskRequest(request CreateTaskRequest) error {
	return validateCreateRequest(request)
}

// inclusiveCalendarMonthCount returns the number of calendar months in the
// inclusive start-to-end range. Callers must reject reversed ranges first.
func inclusiveCalendarMonthCount(start, end time.Time) int {
	return (end.Year()-start.Year())*12 + int(end.Month()-start.Month()) + 1
}
