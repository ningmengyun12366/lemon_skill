package taxknowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	DefaultCapabilityCode   = "tax_knowledge"
	noResultMessage         = "本次检索未返回与该问题相关的可用内容。"
	apiErrorMessage         = "当前无法正常连接财税知识库接口，暂时不能确认该问题的政策依据。"
	missingAPIKeyMessage    = "缺少 API Key，无法调用财税知识库接口。"
	unknownInvalidTypeLabel = "未知"
)

var (
	ErrInvalidQuery   = errors.New("tax knowledge: invalid query")
	ErrMissingAPIKey  = errors.New("tax knowledge: missing api key")
	ErrInvalidAPIKey  = errors.New("tax knowledge: invalid api key")
	ErrInvalidBaseURL = errors.New("tax knowledge: invalid base url")
	ErrNilContext     = errors.New("tax knowledge: nil context")
)

type Query struct {
	value string
}

func NewQuery(raw string) (Query, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Query{}, fmt.Errorf("query is blank: %w", ErrInvalidQuery)
	}
	return Query{value: trimmed}, nil
}

func (q Query) String() string {
	return q.value
}

type APIKey struct {
	value string
}

func NewAPIKey(raw string) (APIKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return APIKey{}, ErrMissingAPIKey
	}
	if strings.ContainsAny(raw, ";\r\n") {
		return APIKey{}, ErrInvalidAPIKey
	}
	return APIKey{value: trimmed}, nil
}

func (k APIKey) String() string {
	return k.value
}

type Status string

const (
	StatusSuccess    Status = "success"
	StatusNoResult   Status = "no_result"
	StatusAPIError   Status = "api_error"
	StatusMissingKey Status = "missing_key"
)

type Output struct {
	Status              Status          `json:"status"`
	Message             string          `json:"message"`
	Query               string          `json:"query,omitempty"`
	Records             []Record        `json:"records"`
	AnswerContext       string          `json:"answer_context"`
	TraceID             string          `json:"traceId,omitempty"`
	Error               *APIErrorDetail `json:"error,omitempty"`
	AllowExternalSearch bool            `json:"allow_external_search"`
}

type APIErrorDetail struct {
	HTTPStatus int    `json:"http_status,omitempty"`
	Code       int    `json:"code,omitempty"`
	Message    string `json:"message"`
	TraceID    string `json:"traceId,omitempty"`
}

type Record struct {
	Index             int      `json:"index"`
	Title             string   `json:"title"`
	Content           string   `json:"content"`
	Score             float64  `json:"score"`
	DocType           string   `json:"doc_type"`
	DocumentID        string   `json:"document_id"`
	DocumentNo        string   `json:"document_no"`
	SourcePublishDate string   `json:"source_publish_date"`
	SourceURL         string   `json:"source_url"`
	InvalidType       string   `json:"invalid_type"`
	InvalidTypeLabel  string   `json:"invalid_type_label"`
	InvalidPrompt     string   `json:"invalid_prompt"`
	AttachmentRefs    []string `json:"attachment_refs"`
}

type retrievalRequest struct {
	Query string `json:"query"`
}

type retrievalEnvelope struct {
	Records []retrievalRecord `json:"records"`
	Code    int               `json:"code"`
	Message string            `json:"message"`
	TraceID string            `json:"traceId"`
}

type retrievalRecord struct {
	Content  string         `json:"content"`
	Metadata recordMetadata `json:"metadata"`
	Score    float64        `json:"score"`
	Title    string         `json:"title"`
}

type recordMetadata struct {
	AttachmentRefs    attachmentRefs `json:"attachment_refs"`
	DocType           string         `json:"doc_type"`
	DocumentID        string         `json:"document_id"`
	DocumentNo        string         `json:"document_no"`
	InvalidPrompt     string         `json:"invalid_prompt"`
	InvalidType       string         `json:"invalid_type"`
	SourcePublishDate string         `json:"source_publish_date"`
	SourceURL         string         `json:"source_url"`
}

type attachmentRefs []string

func (refs *attachmentRefs) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*refs = nil
		return nil
	}
	var rawRefs []json.RawMessage
	if err := json.Unmarshal(data, &rawRefs); err != nil {
		return fmt.Errorf("decode attachment refs: %w", err)
	}
	parsed := make([]string, 0, len(rawRefs))
	for _, raw := range rawRefs {
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				parsed = append(parsed, trimmed)
			}
			continue
		} else {
			var typeErr *json.UnmarshalTypeError
			if !errors.As(err, &typeErr) {
				return fmt.Errorf("decode attachment ref string: %w", err)
			}
		}

		var ref struct {
			Name              string `json:"name"`
			OSSKey            string `json:"oss_key"`
			RagflowDocumentID string `json:"ragflow_document_id"`
		}
		if err := json.Unmarshal(raw, &ref); err != nil {
			return fmt.Errorf("decode attachment ref object: %w", err)
		}
		for _, value := range []string{ref.Name, ref.OSSKey, ref.RagflowDocumentID} {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				parsed = append(parsed, trimmed)
				break
			}
		}
	}
	*refs = parsed
	return nil
}
