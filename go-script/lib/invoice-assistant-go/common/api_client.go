package common

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/skillupdate"
)

type APIClient struct {
	BaseURL        string
	Client         *http.Client
	HeaderProvider func() map[string]string
	VersionChecker skillupdate.Checker
	// RequestSender optionally controls sending and timeout setup for this caller.
	RequestSender func(*http.Request, time.Duration) (*http.Response, error)
}

const (
	AuthorizationUSCCMissing = "AUTHORIZATION_USCC_MISSING"
	AuthorizationUSCCInvalid = "AUTHORIZATION_USCC_INVALID"
	RequestUSCCMismatch      = "REQUEST_USCC_MISMATCH"
)

type RequestUSCCValidationError struct {
	Code       string
	BodyUSCC   string
	HeaderUSCC string
	Detail     string
}

/**
 * Error 返回稳定错误码和可诊断的 USCC 一致性说明。
 *
 * @returns 供 CLI 和业务编排直接展示的错误文本。
 */
func (err *RequestUSCCValidationError) Error() string {
	if err.Detail != "" {
		return err.Code + ": " + err.Detail
	}
	return err.Code
}

func NewAPIClient() *APIClient {
	return &APIClient{BaseURL: DefaultBaseURL, Client: http.DefaultClient, VersionChecker: RuntimeVersionChecker()}
}

func (c *APIClient) PostJSON(requestURL string, payload map[string]any, timeout time.Duration) (map[string]any, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	requestHeaders := c.requestHeaders()
	if err := ValidateRequestUSCC(requestHeaders, payload); err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if c.VersionChecker != nil {
		if err := c.VersionChecker.Check(context.Background()); err != nil {
			return nil, err
		}
	}
	url := buildRequestURL(c.BaseURL, requestURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range requestHeaders {
		req.Header.Set(k, v)
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	} else if client.Timeout == 0 {
		client = &http.Client{Timeout: timeout, Transport: client.Transport}
	}
	startedAt := time.Now()
	var resp *http.Response
	if c.RequestSender != nil {
		resp, err = c.RequestSender(req, timeout)
	} else {
		resp, err = client.Do(req)
	}
	if err != nil {
		appendFailedAPIRequestLog(http.MethodPost, url, requestHeaders, payload, err, nil, nil, startedAt)
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		appendFailedAPIRequestLog(http.MethodPost, url, requestHeaders, payload, err, resp, nil, startedAt)
		return nil, err
	}
	responseResult := parseAPIResponseLogResult(raw)
	appendSuccessAPIRequestLog(http.MethodPost, url, requestHeaders, payload, resp.StatusCode, responseResult, startedAt)
	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("API returned non-JSON response (%d bytes)", len(raw))
	}
	payloadObj, ok := obj.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("API returned non-object JSON")
	}
	return payloadObj, nil
}

/**
 * ValidateJSONRequest 在业务流程写入提交状态前校验实际请求头和请求体的 USCC。
 *
 * @param payload 即将发送的 JSON 根对象。
 * @returns Header 与请求体企业一致时返回 nil，否则返回稳定校验错误。
 */
func (c *APIClient) ValidateJSONRequest(payload map[string]any) error {
	return ValidateRequestUSCC(c.requestHeaders(), payload)
}

/**
 * ValidateRequestUSCC 校验带 Authorization 的企业请求是否使用相同 USCC。
 *
 * @param headers 实际发送的 HTTP 请求头。
 * @param payload 实际发送的 JSON 根对象。
 * @returns 不适用或校验通过时返回 nil，否则返回 RequestUSCCValidationError。
 */
func ValidateRequestUSCC(headers map[string]string, payload map[string]any) error {
	bodyUSCC := strings.TrimSpace(StringValue(payload["uscc"]))
	if bodyUSCC == "" {
		return nil
	}
	authorization, present := headerValue(headers, "Authorization")
	if !present {
		return nil
	}
	parts := strings.Split(authorization, ";")
	usccValues := []string{}
	for _, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) != 2 || strings.TrimSpace(keyValue[0]) == "" {
			return &RequestUSCCValidationError{Code: AuthorizationUSCCInvalid, BodyUSCC: bodyUSCC, Detail: "Authorization contains an invalid key-value segment"}
		}
		if strings.EqualFold(strings.TrimSpace(keyValue[0]), "uscc") {
			usccValues = append(usccValues, strings.TrimSpace(keyValue[1]))
		}
	}
	if len(usccValues) == 0 || usccValues[0] == "" {
		return &RequestUSCCValidationError{Code: AuthorizationUSCCMissing, BodyUSCC: bodyUSCC, Detail: "request body contains uscc but Authorization uscc is missing or empty"}
	}
	if len(usccValues) != 1 {
		return &RequestUSCCValidationError{Code: AuthorizationUSCCInvalid, BodyUSCC: bodyUSCC, Detail: "Authorization must contain exactly one uscc"}
	}
	headerUSCC := usccValues[0]
	if headerUSCC != bodyUSCC {
		return &RequestUSCCValidationError{
			Code:       RequestUSCCMismatch,
			BodyUSCC:   bodyUSCC,
			HeaderUSCC: headerUSCC,
			Detail:     fmt.Sprintf("request body uscc=%s does not match Authorization uscc=%s", bodyUSCC, headerUSCC),
		}
	}
	return nil
}

/**
 * headerValue 以不区分大小写的方式读取待发送请求头。
 *
 * @param headers 待发送的请求头集合。
 * @param name 要查找的 Header 名称。
 * @returns Header 值以及是否存在该 Header。
 */
func headerValue(headers map[string]string, name string) (string, bool) {
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return value, true
		}
	}
	return "", false
}

func (c *APIClient) requestHeaders() map[string]string {
	if c.HeaderProvider != nil {
		return c.HeaderProvider()
	}
	return BuildRequestHeaders(nil)
}

func appendSuccessAPIRequestLog(method, url string, headers map[string]string, params map[string]any, statusCode int, responseResult any, startedAt time.Time) {
	AppendAPILog(map[string]any{
		"timestamp":            startedAt.Format("2006-01-02T15:04:05"),
		"method":               strings.ToUpper(method),
		"url":                  url,
		"request_headers":      sanitizeLogHeaders(headers),
		"request_params":       sanitizeLogValue(params, ""),
		"response_status_code": statusCode,
		"response_result":      sanitizeLogValue(responseResult, ""),
		"error":                nil,
	})
}

func appendFailedAPIRequestLog(method, url string, headers map[string]string, params map[string]any, err error, resp *http.Response, responseResult any, startedAt time.Time) {
	errorInfo := map[string]any{
		"type":    fmt.Sprintf("%T", err),
		"message": err.Error(),
	}
	if resp != nil {
		errorInfo["response_status_code"] = resp.StatusCode
		errorInfo["response_result"] = responseResult
	}
	AppendAPILog(map[string]any{
		"timestamp":       startedAt.Format("2006-01-02T15:04:05"),
		"method":          strings.ToUpper(method),
		"url":             url,
		"request_headers": sanitizeLogHeaders(headers),
		"request_params":  sanitizeLogValue(params, ""),
		"error":           errorInfo,
	})
}

func parseAPIResponseLogResult(raw []byte) any {
	var obj any
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj
	}
	return map[string]any{"non_json_response": true, "response_bytes": len(raw)}
}

func sanitizeLogHeaders(headers map[string]string) map[string]any {
	sanitized := make(map[string]any, len(headers))
	for key, value := range headers {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "authorization":
			sanitized[key] = sanitizeAuthorizationLog(value)
		case "accountid":
			sanitized[key] = map[string]any{
				"present":     strings.TrimSpace(value) != "",
				"fingerprint": stableLogFingerprint(value),
			}
		default:
			sanitized[key] = value
		}
	}
	return sanitized
}

func sanitizeAuthorizationLog(value string) map[string]any {
	result := map[string]any{}
	for _, entry := range strings.Split(value, ";") {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		fieldValue := strings.TrimSpace(parts[1])
		switch key {
		case "apikey":
			result["apiKeyFingerprint"] = stableLogFingerprint(fieldValue)
		case "uscc":
			result["uscc"] = fieldValue
		case "capabilitycode":
			result["capabilityCode"] = fieldValue
		}
	}
	return result
}

func sanitizeLogValue(value any, key string) any {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	compactKey := strings.NewReplacer("_", "", "-", "").Replace(normalizedKey)
	switch {
	case strings.Contains(compactKey, "password"),
		strings.Contains(compactKey, "cookie"),
		strings.Contains(compactKey, "smscode"),
		compactKey == "apikey",
		compactKey == "authorization":
		return "[REDACTED]"
	case compactKey == "accountid":
		text := strings.TrimSpace(StringValue(value))
		return map[string]any{
			"present":     text != "",
			"fingerprint": stableLogFingerprint(text),
		}
	case compactKey == "personalaccount",
		compactKey == "phone",
		compactKey == "mobile",
		compactKey == "mobilephone":
		return maskLogPersonalAccount(StringValue(value))
	}

	switch typed := value.(type) {
	case map[string]any:
		sanitized := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			sanitized[childKey] = sanitizeLogValue(childValue, childKey)
		}
		return sanitized
	case []any:
		sanitized := make([]any, len(typed))
		for index, childValue := range typed {
			sanitized[index] = sanitizeLogValue(childValue, "")
		}
		return sanitized
	default:
		return value
	}
}

func stableLogFingerprint(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:8])
}

func maskLogPersonalAccount(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1 {
		return value
	}
	if len(value) <= 7 {
		return value[:1] + "***" + value[len(value)-1:]
	}
	return value[:3] + "****" + value[len(value)-4:]
}

func (c *APIClient) PostSuccessJSON(requestURL string, payload map[string]any, action string, timeout time.Duration) (map[string]any, error) {
	resp, err := c.PostJSON(requestURL, payload, timeout)
	if err != nil {
		return nil, err
	}
	return EnsureSuccessResponse(resp, action)
}

func EnsureSuccessResponse(payload map[string]any, action string) (map[string]any, error) {
	code := payload["code"]
	success := false
	switch v := code.(type) {
	case float64:
		success = v == 0
	case int:
		success = v == 0
	}
	if !success {
		return nil, fmt.Errorf("%s failed: code=%v, message=%v, traceId=%v", action, payload["code"], payload["message"], payload["traceId"])
	}
	return payload, nil
}

func RequiredParam(params map[string]any, key string) (any, error) {
	value, ok := params[key]
	if !ok || strings.TrimSpace(StringValue(value)) == "" {
		return nil, fmt.Errorf("missing required param: %s", key)
	}
	return value, nil
}

/**
 * BuildRequestHeaders 构造不带隐式企业状态的默认请求头。
 *
 * @param extra 调用方需要附加或覆盖的 Header。
 * @returns 仅包含工作区 API Key、当前 capability 和 extra 的请求头。
 */
func BuildRequestHeaders(extra map[string]string) map[string]string {
	config := LoadWorkspaceConfig()
	headers := map[string]string{"Content-Type": "application/json"}
	apikey := strings.TrimSpace(StringValue(config[APIKey]))
	if apikey != "" {
		headers["Authorization"] = fmt.Sprintf("apikey=%s; capabilityCode=%s", apikey, CapabilityCode)
	}
	for k, v := range extra {
		headers[k] = v
	}
	return headers
}

func buildRequestURL(baseURL, requestURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(requestURL, "/") {
		requestURL = "/" + requestURL
	}
	return baseURL + requestURL
}
