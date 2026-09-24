package taxknowledge

import (
	"fmt"
	"strings"
)

func successOutput(query Query, envelope retrievalEnvelope) Output {
	records := normalizeRecords(envelope.Records)
	return Output{
		Status:              StatusSuccess,
		Message:             envelope.Message,
		Query:               query.String(),
		Records:             records,
		AnswerContext:       BuildAnswerContext(records),
		TraceID:             envelope.TraceID,
		AllowExternalSearch: false,
	}
}

func noResultOutput(query Query, traceID string) Output {
	return Output{
		Status:              StatusNoResult,
		Message:             noResultMessage,
		Query:               query.String(),
		Records:             []Record{},
		AnswerContext:       noResultMessage,
		TraceID:             traceID,
		AllowExternalSearch: false,
	}
}

func apiErrorOutput(query Query, detail APIErrorDetail) Output {
	if strings.TrimSpace(detail.Message) == "" {
		detail.Message = apiErrorMessage
	}
	return Output{
		Status:              StatusAPIError,
		Message:             apiErrorMessage,
		Query:               query.String(),
		Records:             []Record{},
		AnswerContext:       apiErrorMessage,
		TraceID:             detail.TraceID,
		Error:               &detail,
		AllowExternalSearch: false,
	}
}

func MissingAPIKeyOutput() Output {
	return Output{
		Status:              StatusMissingKey,
		Message:             missingAPIKeyMessage,
		Records:             []Record{},
		AnswerContext:       missingAPIKeyMessage,
		Error:               &APIErrorDetail{Message: missingAPIKeyMessage},
		AllowExternalSearch: false,
	}
}

func normalizeRecords(records []retrievalRecord) []Record {
	normalized := make([]Record, 0, len(records))
	for index, record := range records {
		metadata := record.Metadata
		normalized = append(normalized, Record{
			Index:             index + 1,
			Title:             strings.TrimSpace(record.Title),
			Content:           strings.TrimSpace(record.Content),
			Score:             record.Score,
			DocType:           strings.TrimSpace(metadata.DocType),
			DocumentID:        strings.TrimSpace(metadata.DocumentID),
			DocumentNo:        strings.TrimSpace(metadata.DocumentNo),
			SourcePublishDate: strings.TrimSpace(metadata.SourcePublishDate),
			SourceURL:         strings.TrimSpace(metadata.SourceURL),
			InvalidType:       strings.TrimSpace(metadata.InvalidType),
			InvalidTypeLabel:  invalidTypeLabel(metadata.InvalidType),
			InvalidPrompt:     strings.TrimSpace(metadata.InvalidPrompt),
			AttachmentRefs:    append([]string{}, metadata.AttachmentRefs...),
		})
	}
	return normalized
}

func BuildAnswerContext(records []Record) string {
	if len(records) == 0 {
		return noResultMessage
	}
	lines := make([]string, 0, len(records)*9)
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("[%d] %s", record.Index, record.Title))
		appendIfPresent(&lines, "文号", record.DocumentNo)
		appendIfPresent(&lines, "发布日期", record.SourcePublishDate)
		lines = append(lines, "失效标记："+valueOrUnknown(record.InvalidTypeLabel))
		appendIfPresent(&lines, "失效/变更提示", record.InvalidPrompt)
		appendIfPresent(&lines, "官方来源", record.SourceURL)
		lines = append(lines, fmt.Sprintf("召回分数：%g", record.Score))
		lines = append(lines, "命中内容："+collapseWhitespace(record.Content))
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func appendIfPresent(lines *[]string, label string, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		*lines = append(*lines, label+"："+trimmed)
	}
}

func valueOrUnknown(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return unknownInvalidTypeLabel
	}
	return trimmed
}

func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func invalidTypeLabel(invalidType string) string {
	switch strings.TrimSpace(invalidType) {
	case "not_invalid":
		return "未标记失效（不代表当前有效）"
	case "partially_invalid":
		return "部分失效"
	case "invalid", "fully_invalid":
		return "已失效"
	case "":
		return unknownInvalidTypeLabel
	default:
		return strings.TrimSpace(invalidType)
	}
}
