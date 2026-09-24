package mailimap

import (
	"context"
	"fmt"
	"mime"
	"net"
	"strconv"
	"strings"

	"invoice-assistant-skill/go-script/lib/skillupdate"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
)

type IMAPFetcher struct {
	DialTLS        func(address string, options *imapclient.Options) (*imapclient.Client, error)
	VersionChecker skillupdate.Checker
}

func (fetcher IMAPFetcher) Fetch(ctx context.Context, options ScanOptions) (ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	host := strings.TrimSpace(options.Host)
	if host == "" {
		return ScanResult{}, fmt.Errorf("IMAP host is required")
	}
	port := options.Port
	if port <= 0 {
		port = 993
	}
	if fetcher.VersionChecker != nil {
		if err := fetcher.VersionChecker.Check(ctx); err != nil {
			return ScanResult{}, err
		}
	}
	dial := fetcher.DialTLS
	if dial == nil {
		dial = imapclient.DialTLS
	}
	client, err := dial(net.JoinHostPort(host, strconv.Itoa(port)), &imapclient.Options{
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	})
	if err != nil {
		return ScanResult{}, fmt.Errorf("IMAP connection failed: %w", err)
	}
	defer client.Close()
	if err := client.Login(options.Email, options.Password).Wait(); err != nil {
		return ScanResult{}, fmt.Errorf("IMAP login failed")
	}
	selectData, err := client.Select(DefaultMailbox, nil).Wait()
	if err != nil {
		return ScanResult{}, fmt.Errorf("IMAP select INBOX failed: %w", err)
	}
	result := ScanResult{Mailbox: DefaultMailbox, Host: host, Port: port, Provider: options.Provider}
	if selectData == nil || selectData.NumMessages == 0 {
		_ = client.Logout().Wait()
		return result, nil
	}

	start := uint32(1)
	filtersBeforeLimit := !options.StartDate.IsZero() || !options.EndDate.IsZero() || strings.TrimSpace(options.Keyword) != ""
	if !filtersBeforeLimit && options.Limit > 0 && uint32(options.Limit) < selectData.NumMessages {
		start = selectData.NumMessages - uint32(options.Limit) + 1
	}
	seqSet := imap.SeqSet{}
	seqSet.AddRange(start, selectData.NumMessages)
	section := &imap.FetchItemBodySection{Peek: true}
	fetchOptions := &imap.FetchOptions{UID: true, InternalDate: true, BodySection: []*imap.FetchItemBodySection{section}}
	messages, err := client.Fetch(seqSet, fetchOptions).Collect()
	if err != nil {
		return ScanResult{}, fmt.Errorf("IMAP fetch INBOX messages failed: %w", err)
	}
	for _, item := range messages {
		if err := ctx.Err(); err != nil {
			return ScanResult{}, err
		}
		body := item.FindBodySection(section)
		if len(body) == 0 {
			continue
		}
		uid := strconv.FormatUint(uint64(item.UID), 10)
		if strings.TrimSpace(uid) == "0" {
			uid = fmt.Sprintf("seq-%d", item.SeqNum)
		}
		parsed, err := ParseMessage(body, uid)
		if err != nil {
			failure := NewMessageParseFailure(body, uid, item.InternalDate, err)
			if ParseFailureMatches(failure, options.StartDate, options.EndDate, options.Keyword) {
				result.ParseFailures = append(result.ParseFailures, failure)
			}
			continue
		}
		if parsed.Date.IsZero() && !item.InternalDate.IsZero() {
			parsed.Date = item.InternalDate
		}
		if !MessageMatches(parsed, options.StartDate, options.EndDate, options.Keyword) {
			continue
		}
		message, err := SaveParsedMessage(parsed, SaveOptions{AttachmentDir: options.AttachmentDir, TravelDir: options.TravelDir})
		if err != nil {
			return ScanResult{}, err
		}
		result.Messages = append(result.Messages, message)
		if options.Limit > 0 && len(result.Messages) >= options.Limit {
			break
		}
	}
	_ = client.Logout().Wait()
	return result, nil
}
