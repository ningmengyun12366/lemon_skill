package mailimap

import (
	"context"
	"strings"
	"time"
)

type ScanOptions struct {
	Email         string
	Password      string
	Host          string
	Port          int
	Provider      string
	Mailbox       string
	Limit         int
	StartDate     time.Time
	EndDate       time.Time
	Keyword       string
	AttachmentDir string
	TravelDir     string
}

type ScanResult struct {
	Mailbox       string
	Host          string
	Port          int
	Provider      string
	Messages      []Message
	ParseFailures []MessageParseFailure
}

type MessageParseFailure struct {
	UID     string
	Subject string
	From    string
	Date    time.Time
	Error   string
}

type Fetcher interface {
	Fetch(context.Context, ScanOptions) (ScanResult, error)
}

func ParseFailureMatches(failure MessageParseFailure, startDate, endDate time.Time, keyword string) bool {
	if (!startDate.IsZero() || !endDate.IsZero()) && !dateInRange(failure.Date, startDate, endDate) {
		return false
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return true
	}
	needle := strings.ToLower(keyword)
	haystack := strings.ToLower(strings.Join([]string{failure.Subject, failure.From}, "\n"))
	return strings.Contains(haystack, needle)
}

func (result ScanResult) SavedAttachmentCount() int {
	count := 0
	for _, message := range result.Messages {
		for _, attachment := range message.Attachments {
			if strings.TrimSpace(attachment.SavedPath) != "" {
				count++
			}
		}
	}
	return count
}

func (result ScanResult) TravelFormAttachmentCount() int {
	count := 0
	for _, message := range result.Messages {
		for _, attachment := range message.Attachments {
			if attachment.IsTravelForm && strings.TrimSpace(attachment.SavedPath) != "" {
				count++
			}
		}
	}
	return count
}

func (result ScanResult) MessageIDBySavedPath() map[string]string {
	lookup := map[string]string{}
	for _, message := range result.Messages {
		for _, attachment := range message.Attachments {
			if strings.TrimSpace(attachment.SavedPath) != "" {
				lookup[attachment.SavedPath] = message.UID
			}
		}
	}
	return lookup
}
