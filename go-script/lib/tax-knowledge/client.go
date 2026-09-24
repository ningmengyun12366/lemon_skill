package taxknowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/skillupdate"
)

const defaultTimeout = 30 * time.Second

type ClientConfig struct {
	BaseURL        string
	APIKey         APIKey
	HTTPClient     *http.Client
	VersionChecker skillupdate.Checker
}

type Client struct {
	baseURL        string
	apiKey         APIKey
	httpClient     *http.Client
	versionChecker skillupdate.Checker
}

func NewClient(config ClientConfig) (*Client, error) {
	baseURL, err := normalizeBaseURL(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("normalize tax knowledge base url: %w", err)
	}
	if config.APIKey.String() == "" {
		return nil, ErrMissingAPIKey
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: baseURL, apiKey: config.APIKey, httpClient: httpClient, versionChecker: config.VersionChecker}, nil
}

func (client *Client) Retrieve(ctx context.Context, query Query) (Output, error) {
	if ctx == nil {
		return Output{}, ErrNilContext
	}
	payload := retrievalRequest{Query: query.String()}
	body, err := json.Marshal(payload)
	if err != nil {
		return Output{}, fmt.Errorf("marshal tax knowledge retrieval request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.retrievalURL(), bytes.NewReader(body))
	if err != nil {
		return Output{}, fmt.Errorf("build tax knowledge retrieval request: %w", err)
	}
	request.Header.Set("Authorization", BuildAuthorizationHeader(client.apiKey))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if client.versionChecker != nil {
		if err := client.versionChecker.Check(ctx); err != nil {
			return Output{}, err
		}
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return apiErrorOutput(query, APIErrorDetail{Message: err.Error()}), nil
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return Output{}, fmt.Errorf("read tax knowledge retrieval response: %w", err)
	}
	envelope, parsed := decodeEnvelope(raw)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if parsed {
			return apiErrorOutput(query, APIErrorDetail{
				HTTPStatus: response.StatusCode,
				Code:       envelope.Code,
				Message:    envelope.Message,
				TraceID:    envelope.TraceID,
			}), nil
		}
		return apiErrorOutput(query, APIErrorDetail{
			HTTPStatus: response.StatusCode,
			Message:    fmt.Sprintf("HTTP %d", response.StatusCode),
		}), nil
	}
	if !parsed {
		return apiErrorOutput(query, APIErrorDetail{HTTPStatus: response.StatusCode, Message: "response is not JSON"}), nil
	}
	if envelope.Code != 0 {
		return apiErrorOutput(query, APIErrorDetail{
			HTTPStatus: response.StatusCode,
			Code:       envelope.Code,
			Message:    envelope.Message,
			TraceID:    envelope.TraceID,
		}), nil
	}
	if len(envelope.Records) == 0 {
		return noResultOutput(query, envelope.TraceID), nil
	}
	return successOutput(query, envelope), nil
}

func BuildAuthorizationHeader(apiKey APIKey) string {
	return "apikey=" + apiKey.String() + "; capabilityCode=" + DefaultCapabilityCode
}

func (client *Client) retrievalURL() string {
	if strings.HasSuffix(client.baseURL, "/retrieval") {
		return client.baseURL
	}
	return client.baseURL + "/retrieval"
}

func decodeEnvelope(raw []byte) (retrievalEnvelope, bool) {
	var envelope retrievalEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return retrievalEnvelope{}, false
	}
	return envelope, true
}

func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", ErrInvalidBaseURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidBaseURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidBaseURL
	}
	return trimmed, nil
}
