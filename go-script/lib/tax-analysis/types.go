package taxanalysis

import (
	"encoding/json"
	"fmt"
)

type TaskStatus string
type ModuleStatus string

// APIError is the stable non-zero invoice v1 ErrorCode returned by OpenTax.
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("tax analysis API error %d: %s", e.Code, e.Message)
}

type CreateTaskRequest struct {
	USCC         string `json:"uscc"`
	StartMonth   string `json:"startMonth"`
	EndMonth     string `json:"endMonth"`
	ForceRefresh bool   `json:"forceRefresh"`
}

type QueryTaskRequest struct {
	USCC   string `json:"uscc"`
	TaskID string `json:"taskId"`
}

type FailedItem struct {
	ModuleCode string         `json:"moduleCode,omitempty"`
	ItemID     string         `json:"itemId"`
	Message    string         `json:"message"`
	ErrorCode  int            `json:"errorCode,omitempty"`
	Context    map[string]any `json:"context,omitempty"`
}

type ModuleResult struct {
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Status       ModuleStatus    `json:"status"`
	Data         json.RawMessage `json:"data,omitempty"`
	Error        string          `json:"error,omitempty"`
	Stale        bool            `json:"stale"`
	RefreshError string          `json:"refreshError,omitempty"`
	FailedItems  []FailedItem    `json:"failedItems"`
	UpdatedAt    string          `json:"updatedAt,omitempty"`
}

type TaskResponse struct {
	DataSchemaVersion string                     `json:"dataSchemaVersion"`
	TaxDocuments      map[string]json.RawMessage `json:"taxDocuments"`
	TaskID            string                     `json:"taskId"`
	Generation        int                        `json:"generation"`
	Status            TaskStatus                 `json:"status"`
	USCC              string                     `json:"uscc"`
	StartTime         string                     `json:"startTime"`
	EndTime           string                     `json:"endTime"`
	ModuleResults     []ModuleResult             `json:"moduleResults"`
	FailedItems       []FailedItem               `json:"failedItems"`
	CompletedCount    int                        `json:"completedCount"`
	FailedCount       int                        `json:"failedCount"`
	NotSupportedCount int                        `json:"notSupportedCount"`
	TotalCount        int                        `json:"totalCount"`
	CreatedAt         string                     `json:"createdAt"`
	UpdatedAt         string                     `json:"updatedAt"`
	CompletedAt       string                     `json:"completedAt,omitempty"`
	ExecutionError    *ExecutionError            `json:"executionError,omitempty"`
}

// ExecutionError describes a stopped executor; successful module data remains usable.
type ExecutionError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	OccurredAt string `json:"occurredAt"`
	ModuleCode string `json:"moduleCode,omitempty"`
	Retryable  bool   `json:"retryable"`
}

// Terminal keeps polling recoverable interruptions; stopped tasks retain partial data.
func (r TaskResponse) Terminal() bool {
	if r.Status == StatusInterrupted {
		return r.ExecutionError == nil || !r.ExecutionError.Retryable
	}
	return r.Status == StatusSuccess || r.Status == StatusPartialSuccess || r.Status == StatusFailed
}
