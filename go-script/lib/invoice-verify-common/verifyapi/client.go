package verifyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/config"
	"invoice-assistant-skill/go-script/lib/skillupdate"
)

const (
	DefaultCapabilityCode = "invoice_check"

	RateLimitMessage  = "您的访问过于频繁"
	DefaultTimeout    = 10 * time.Second
	DefaultMaxRetries = 2
)

type Request struct {
	FPDM   string
	FPHM   string
	JYM    string
	JSHJ   string
	BHSJE  string
	KPRQ   string
	BSWJLX []string
}

type InvoiceData struct {
	InvoiceCode      string
	InvoiceNumber    string
	SellerName       string
	SellerTaxNo      string
	BuyerName        string
	BuyerTaxNo       string
	AmountWithoutTax string
	TaxAmount        string
	TotalAmount      string
	LineItems        any
	InvoiceType      string
	InvoiceStatus    string
}

type Result struct {
	Success       bool
	Message       string
	IsRateLimited bool
	Data          InvoiceData
	Attempts      int
	StatusCode    int
}

type Client struct {
	endpoint          string
	apiKey            string
	capabilityCode    string
	allowLoopbackHTTP bool
	httpClient        *http.Client
	timeout           time.Duration
	maxRetries        int
	retryDelay        func(int, retryCategory) time.Duration
	versionChecker    skillupdate.Checker
}

type Option func(*Client)

func NewClient(apiKey string, options ...Option) (*Client, error) {
	if _, err := BuildAuthorizationHeader(apiKey); err != nil {
		return nil, err
	}
	client := &Client{
		apiKey:            strings.TrimSpace(apiKey),
		capabilityCode:    DefaultCapabilityCode,
		allowLoopbackHTTP: true,
		httpClient:        http.DefaultClient,
		timeout:           DefaultTimeout,
		maxRetries:        DefaultMaxRetries,
		retryDelay:        defaultRetryDelay,
	}
	for _, option := range options {
		option(client)
	}
	if strings.TrimSpace(client.endpoint) == "" {
		return nil, errors.New("缺少查验接口运行配置：config/verify-api.json 未设置 verify_api_endpoint")
	}
	if strings.TrimSpace(client.capabilityCode) == "" {
		return nil, errors.New("缺少查验接口运行配置：config/verify-api.json 未设置 invoice_check_code")
	}
	if strings.ContainsAny(client.capabilityCode, ";\r\n") {
		return nil, errors.New("查验接口 invoice_check_code 格式不正确")
	}
	if err := validateEndpointSecurity(client.endpoint, client.allowLoopbackHTTP); err != nil {
		return nil, err
	}
	client.capabilityCode = strings.TrimSpace(client.capabilityCode)
	client.httpClient = withSecureRedirects(client.httpClient, client.allowLoopbackHTTP)
	return client, nil
}

func NewClientFromConfig(apiKey string, cfg config.Config, options ...Option) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	allOptions := make([]Option, 0, len(options)+1)
	allOptions = append(allOptions, WithConfig(cfg))
	allOptions = append(allOptions, options...)
	return NewClient(apiKey, allOptions...)
}

func WithConfig(cfg config.Config) Option {
	return func(client *Client) {
		client.endpoint = cfg.VerifyAPIEndpoint
		client.capabilityCode = cfg.InvoiceCheckCode
		client.allowLoopbackHTTP = config.AllowsLoopbackHTTP(cfg.Environment)
		if strings.TrimSpace(cfg.SkillName) != "" || strings.TrimSpace(cfg.InstallDocURL) != "" {
			client.versionChecker = skillupdate.Shared(cfg.SkillName, cfg.InstallDocURL)
		}
	}
}

// WithVersionChecker injects the gate executed before the first business request.
func WithVersionChecker(checker skillupdate.Checker) Option {
	return func(client *Client) {
		client.versionChecker = checker
	}
}

func WithEndpoint(endpoint string) Option {
	return func(client *Client) {
		client.endpoint = endpoint
	}
}

func WithCapabilityCode(capabilityCode string) Option {
	return func(client *Client) {
		client.capabilityCode = capabilityCode
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(client *Client) {
		if httpClient != nil {
			client.httpClient = httpClient
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(client *Client) {
		client.timeout = timeout
	}
}

func WithMaxRetries(maxRetries int) Option {
	return func(client *Client) {
		if maxRetries < 0 {
			maxRetries = 0
		}
		client.maxRetries = maxRetries
	}
}

func WithRetryDelay(delay time.Duration) Option {
	return func(client *Client) {
		client.retryDelay = func(int, retryCategory) time.Duration { return delay }
	}
}

func BuildAuthorizationHeader(apiKey string, capabilityCodes ...string) (string, error) {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return "", errors.New("缺少 API Key，无法执行发票查验请求；请求未发送。")
	}
	if strings.ContainsAny(apiKey, ";\r\n") {
		return "", errors.New("API Key 格式不正确：不能为空，且不能包含分号或换行符；请求未发送。")
	}
	capabilityCode := DefaultCapabilityCode
	if len(capabilityCodes) > 0 {
		capabilityCode = strings.TrimSpace(capabilityCodes[0])
	}
	if capabilityCode == "" || strings.ContainsAny(capabilityCode, ";\r\n") {
		return "", errors.New("invoice_check_code 格式不正确；请求未发送。")
	}
	return "apikey=" + trimmed + "; capabilityCode=" + capabilityCode, nil
}

func validateEndpointSecurity(endpoint string, allowLoopbackHTTP bool) error {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("查验接口地址格式不正确")
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return nil
	}
	if allowLoopbackHTTP && strings.EqualFold(parsed.Scheme, "http") && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return errors.New("查验接口地址必须使用 HTTPS；当前运行模式未启用回环地址 HTTP")
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func withSecureRedirects(httpClient *http.Client, allowLoopbackHTTP bool) *http.Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	copyClient := *httpClient
	previous := httpClient.CheckRedirect
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := validateEndpointSecurity(req.URL.String(), allowLoopbackHTTP); err != nil {
			return err
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &copyClient
}

func (client *Client) Verify(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client.versionChecker != nil {
		if err := client.versionChecker.Check(ctx); err != nil {
			return Result{}, err
		}
	}
	var last Result
	for attempt := 0; ; attempt++ {
		result, category, err := client.callOnce(ctx, request)
		if err != nil {
			return Result{}, err
		}
		result.Attempts = attempt + 1
		last = result
		if result.Success || category == retryNone || attempt >= client.maxRetries {
			if category == retryRateLimit && !result.Success {
				last.Message = "发票查验接口调用频繁，请稍后再试"
				last.IsRateLimited = true
			}
			return last, nil
		}
		if err := sleep(ctx, client.retryDelay(attempt+1, category)); err != nil {
			last.Message = "查验接口请求取消"
			return last, nil
		}
	}
}

func (client *Client) callOnce(ctx context.Context, request Request) (Result, retryCategory, error) {
	apiPayload, err := json.Marshal(normalizeRequest(request))
	if err != nil {
		return Result{}, retryNone, fmt.Errorf("marshal invoice verify payload: %w", err)
	}
	authorization, err := BuildAuthorizationHeader(client.apiKey, client.capabilityCode)
	if err != nil {
		return Result{Message: err.Error()}, retryNone, nil
	}
	attemptCtx := ctx
	cancel := func() {}
	if client.timeout > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, client.timeout)
	}
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, client.endpoint, bytes.NewReader(apiPayload))
	if err != nil {
		return Result{}, retryNone, fmt.Errorf("build invoice verify request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", authorization)

	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return Result{Message: "查验接口请求取消"}, retryNone, nil
		}
		if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			return Result{Message: "查验接口请求超时"}, retryHTTPError, nil
		}
		return Result{Message: "查验接口请求异常: " + err.Error()}, retryHTTPError, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Result{StatusCode: response.StatusCode, Message: "读取查验接口响应失败: " + err.Error()}, retryNone, nil
	}

	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return Result{StatusCode: response.StatusCode, Message: responseMessage(body, fmt.Sprintf("查验失败: HTTP %d", response.StatusCode))}, retryNone, nil
		}
		return Result{StatusCode: response.StatusCode, Message: "查验接口响应 JSON 格式错误"}, retryNone, nil
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		category := retryCategoryForEnvelope(envelope)
		return Result{StatusCode: response.StatusCode, Message: apiFailureMessage(envelope.Message), IsRateLimited: category == retryRateLimit}, category, nil
	}

	if envelope.Code != 0 {
		category := retryCategoryForEnvelope(envelope)
		return Result{StatusCode: response.StatusCode, Message: apiFailureMessage(envelope.Message), IsRateLimited: category == retryRateLimit}, category, nil
	}
	data, errMessage := parseInvoiceData(envelope.Data)
	if errMessage != "" {
		return Result{StatusCode: response.StatusCode, Message: errMessage}, retryNone, nil
	}
	return Result{Success: true, StatusCode: response.StatusCode, Message: "查验成功", Data: data}, retryNone, nil
}

type requestPayload struct {
	FPDM   string   `json:"fpdm"`
	FPHM   string   `json:"fphm"`
	JYM    string   `json:"jym"`
	JSHJ   string   `json:"jshj"`
	BHSJE  string   `json:"bhsje"`
	KPRQ   string   `json:"kprq"`
	BSWJLX []string `json:"bswjlx"`
}

func normalizeRequest(request Request) requestPayload {
	jym := strings.TrimSpace(request.JYM)
	if len(jym) > 6 {
		jym = jym[len(jym)-6:]
	}
	bswjlx := request.BSWJLX
	if bswjlx == nil {
		bswjlx = []string{}
	}
	return requestPayload{
		FPDM:   strings.TrimSpace(request.FPDM),
		FPHM:   strings.TrimSpace(request.FPHM),
		JYM:    jym,
		JSHJ:   strings.TrimSpace(request.JSHJ),
		BHSJE:  strings.TrimSpace(request.BHSJE),
		KPRQ:   NormalizeIssueDateOrTrim(request.KPRQ),
		BSWJLX: bswjlx,
	}
}

type apiEnvelope struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Data    apiData `json:"data"`
}

type apiData struct {
	FPPMXX json.RawMessage `json:"fppmxx"`
	FPPZDM string          `json:"fppzDm"`
	FPZTDM string          `json:"fpztDm"`
}

func parseInvoiceData(data apiData) (InvoiceData, string) {
	if len(data.FPPMXX) == 0 || bytes.Equal(bytes.TrimSpace(data.FPPMXX), []byte("null")) {
		return InvoiceData{}, "查验接口未返回fppmxx数据"
	}
	var raw map[string]any
	if err := json.Unmarshal(data.FPPMXX, &raw); err != nil {
		return InvoiceData{}, "fppmxx 数据格式错误，期望为对象"
	}
	return InvoiceData{
		InvoiceCode:      stringValue(raw, "fpdm"),
		InvoiceNumber:    stringValue(raw, "fphm"),
		SellerName:       stringValue(raw, "xsfmc"),
		SellerTaxNo:      stringValue(raw, "xsfnsrsbh"),
		BuyerName:        stringValue(raw, "gmfmc"),
		BuyerTaxNo:       stringValue(raw, "gmfnsrsbh"),
		AmountWithoutTax: stringValue(raw, "hjje"),
		TaxAmount:        stringValue(raw, "hjse"),
		TotalAmount:      stringValue(raw, "jshj"),
		LineItems:        lineItems(rawValueOrNil(raw, "mxzbList")),
		InvoiceType:      invoiceType(data.FPPZDM),
		InvoiceStatus:    invoiceStatus(data.FPZTDM),
	}, ""
}

func stringValue(raw map[string]any, key string) string {
	value, ok := rawValue(raw, key)
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%v", typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func rawValueOrNil(raw map[string]any, key string) any {
	value, ok := rawValue(raw, key)
	if !ok {
		return nil
	}
	return value
}

func rawValue(raw map[string]any, key string) (any, bool) {
	if value, ok := raw[key]; ok {
		return value, true
	}
	for field, value := range raw {
		if strings.EqualFold(field, key) {
			return value, true
		}
	}
	return nil, false
}

func lineItems(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		return typed
	case string:
		var parsed any
		if err := json.Unmarshal([]byte(typed), &parsed); err == nil {
			if list, ok := parsed.([]any); ok {
				return list
			}
		}
	}
	return nil
}

func invoiceType(code string) string {
	switch code {
	case "01":
		return "增值税专用发票"
	case "02":
		return "普通发票"
	case "03":
		return "机动车销售统一发票"
	case "04":
		return "二手车销售统一发票"
	case "05":
		return "铁路电子客票"
	case "06":
		return "航空运输客票电子行程单"
	default:
		return ""
	}
}

func invoiceStatus(code string) string {
	switch code {
	case "01":
		return "正常"
	case "02":
		return "作废"
	case "03":
		return "红冲"
	default:
		return ""
	}
}

func responseMessage(body []byte, fallback string) string {
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err == nil {
		if message, ok := decoded["message"].(string); ok && strings.TrimSpace(message) != "" {
			return "查验失败: " + message
		}
	}
	return fallback
}
