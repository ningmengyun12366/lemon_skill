package excelx

import (
	"fmt"
	"math"
	"reflect"
)

type WorkbookSummary struct {
	Sheets        []string            `json:"sheets"`
	Headers       []string            `json:"headers"`
	Rows          []map[string]string `json:"rows"`
	NumberFormats []CellFormat        `json:"number_formats"`
	Hyperlinks    []CellHyperlink     `json:"hyperlinks"`
	Widths        []ColumnWidth       `json:"widths"`
}

type CellFormat struct {
	Cell   string `json:"cell"`
	Header string `json:"header"`
	Format string `json:"format"`
}

type CellHyperlink struct {
	Cell    string `json:"cell"`
	Header  string `json:"header"`
	Target  string `json:"target"`
	Display string `json:"display"`
}

type ColumnWidth struct {
	Header string  `json:"header"`
	Width  float64 `json:"width"`
}

func CompareWorkbookSummary(got, want WorkbookSummary) error {
	if !reflect.DeepEqual(got.Sheets, want.Sheets) {
		return fmt.Errorf("sheets mismatch: got %#v want %#v", got.Sheets, want.Sheets)
	}
	if !reflect.DeepEqual(got.Headers, want.Headers) {
		return fmt.Errorf("headers mismatch: got %#v want %#v", got.Headers, want.Headers)
	}
	if len(got.Rows) != len(want.Rows) {
		return fmt.Errorf("row count mismatch: got %d want %d", len(got.Rows), len(want.Rows))
	}
	for idx := range want.Rows {
		if !reflect.DeepEqual(got.Rows[idx], want.Rows[idx]) {
			return fmt.Errorf("row %d mismatch: got %#v want %#v", idx+1, got.Rows[idx], want.Rows[idx])
		}
	}
	if len(got.NumberFormats) != len(want.NumberFormats) || (len(want.NumberFormats) > 0 && !reflect.DeepEqual(got.NumberFormats, want.NumberFormats)) {
		return fmt.Errorf("number formats mismatch: got %#v want %#v", got.NumberFormats, want.NumberFormats)
	}
	if len(got.Hyperlinks) != len(want.Hyperlinks) || (len(want.Hyperlinks) > 0 && !reflect.DeepEqual(got.Hyperlinks, want.Hyperlinks)) {
		return fmt.Errorf("hyperlinks mismatch: got %#v want %#v", got.Hyperlinks, want.Hyperlinks)
	}
	if len(got.Widths) != len(want.Widths) {
		return fmt.Errorf("width count mismatch: got %d want %d", len(got.Widths), len(want.Widths))
	}
	for idx := range want.Widths {
		if got.Widths[idx].Header != want.Widths[idx].Header || math.Abs(got.Widths[idx].Width-want.Widths[idx].Width) > 0.01 {
			return fmt.Errorf("width %d mismatch: got %#v want %#v", idx, got.Widths[idx], want.Widths[idx])
		}
	}
	return nil
}
