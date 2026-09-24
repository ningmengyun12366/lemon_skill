package main

import (
	"encoding/json"
	"fmt"
	"io"

	taxknowledge "invoice-assistant-skill/go-script/lib/tax-knowledge"
)

type cliOutput struct {
	OK                  bool                         `json:"ok"`
	Status              taxknowledge.Status          `json:"status"`
	Message             string                       `json:"message"`
	Query               string                       `json:"query,omitempty"`
	RecordCount         int                          `json:"record_count"`
	CapabilityCode      string                       `json:"capability_code"`
	Records             []taxknowledge.Record        `json:"records"`
	AnswerContext       string                       `json:"answer_context"`
	TraceID             string                       `json:"traceId,omitempty"`
	Error               *taxknowledge.APIErrorDetail `json:"error,omitempty"`
	AllowExternalSearch bool                         `json:"allow_external_search"`
}

func writeFailure(stdout io.Writer, stderr io.Writer, status taxknowledge.Status, message string, query string) int {
	_, _ = fmt.Fprintf(stderr, "diagnostic: %s\n", message)
	output := taxknowledge.Output{
		Status:              status,
		Message:             message,
		Query:               query,
		Records:             []taxknowledge.Record{},
		AnswerContext:       message,
		Error:               &taxknowledge.APIErrorDetail{Message: message},
		AllowExternalSearch: false,
	}
	if err := encodeCLIOutput(stdout, output, false); err != nil {
		_, _ = fmt.Fprintf(stderr, "diagnostic: write JSON failure: %s\n", err)
	}
	return 1
}

func writeOutput(stdout io.Writer, output taxknowledge.Output, pretty bool) int {
	if err := encodeCLIOutput(stdout, output, pretty); err != nil {
		return 1
	}
	return 1
}

func encodeCLIOutput(writer io.Writer, output taxknowledge.Output, pretty bool) error {
	records := output.Records
	if records == nil {
		records = []taxknowledge.Record{}
	}
	value := cliOutput{
		OK:                  output.Status == taxknowledge.StatusSuccess,
		Status:              output.Status,
		Message:             output.Message,
		Query:               output.Query,
		RecordCount:         len(records),
		CapabilityCode:      taxknowledge.DefaultCapabilityCode,
		Records:             records,
		AnswerContext:       output.AnswerContext,
		TraceID:             output.TraceID,
		Error:               output.Error,
		AllowExternalSearch: output.AllowExternalSearch,
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}
