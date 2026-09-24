package mailimap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	netmail "net/mail"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
)

var headerDecoder = mime.WordDecoder{CharsetReader: charset.Reader}

const maxBodyKeywordBytes = 256 * 1024

type Message struct {
	UID         string
	Subject     string
	Date        time.Time
	Attachments []Attachment
}

type Attachment struct {
	OriginalFilename   string
	Filename           string
	SavedPath          string
	Size               int64
	Extension          string
	IsInvoiceCandidate bool
	IsTravelForm       bool
	SkippedReason      string
}

type ParsedMessage struct {
	UID         string
	Subject     string
	From        string
	To          string
	BodyText    string
	Date        time.Time
	Attachments []ParsedAttachment
}

type ParsedAttachment struct {
	OriginalFilename   string
	Filename           string
	Content            []byte
	Size               int64
	Extension          string
	IsInvoiceCandidate bool
	IsTravelForm       bool
	SkippedReason      string
}

func NewMessageParseFailure(raw []byte, uid string, fallbackDate time.Time, parseErr error) MessageParseFailure {
	metadata := extractMessageMetadata(raw)
	date := metadata.Date
	if date.IsZero() {
		date = fallbackDate
	}
	message := ""
	if parseErr != nil {
		message = parseErr.Error()
	}
	return MessageParseFailure{
		UID:     strings.TrimSpace(uid),
		Subject: metadata.Subject,
		From:    metadata.From,
		Date:    date,
		Error:   message,
	}
}

type messageMetadata struct {
	Subject string
	From    string
	Date    time.Time
}

func extractMessageMetadata(raw []byte) messageMetadata {
	headers := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	lastKey := ""
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if lastKey != "" {
				headers[lastKey] = strings.TrimSpace(headers[lastKey] + " " + strings.TrimSpace(line))
			}
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		value := strings.TrimSpace(line[colon+1:])
		headers[key] = value
		lastKey = key
	}
	metadata := messageMetadata{
		Subject: decodeHeaderValue(headers["subject"]),
		From:    decodeHeaderValue(headers["from"]),
	}
	if rawDate := strings.TrimSpace(headers["date"]); rawDate != "" {
		if date, err := netmail.ParseDate(rawDate); err == nil {
			metadata.Date = date
		}
	}
	return metadata
}

type SaveOptions struct {
	AttachmentDir string
	TravelDir     string
}

func ParseMessage(raw []byte, uid string) (ParsedMessage, error) {
	reader, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil && reader == nil {
		return ParsedMessage{}, err
	}
	if reader == nil {
		return ParsedMessage{}, errors.New("mail reader was not created")
	}
	defer reader.Close()

	parsed := ParsedMessage{UID: strings.TrimSpace(uid)}
	if parsed.UID == "" {
		if messageID, err := reader.Header.MessageID(); err == nil && strings.TrimSpace(messageID) != "" {
			parsed.UID = strings.TrimSpace(messageID)
		}
	}
	if subject, err := reader.Header.Subject(); err == nil {
		parsed.Subject = strings.TrimSpace(subject)
	} else {
		parsed.Subject = decodeHeaderValue(reader.Header.Get("Subject"))
	}
	parsed.From = decodeHeaderValue(reader.Header.Get("From"))
	parsed.To = decodeHeaderValue(reader.Header.Get("To"))
	if date, err := reader.Header.Date(); err == nil {
		parsed.Date = date
	}

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ParsedMessage{}, err
		}
		header, ok := part.Header.(*mail.AttachmentHeader)
		if !ok {
			payload, readErr := io.ReadAll(io.LimitReader(part.Body, maxBodyKeywordBytes+1))
			if readErr != nil {
				return ParsedMessage{}, readErr
			}
			if len(payload) > maxBodyKeywordBytes {
				payload = payload[:maxBodyKeywordBytes]
			}
			if utf8.Valid(payload) {
				parsed.BodyText = strings.TrimSpace(parsed.BodyText + "\n" + string(payload))
			}
			continue
		}
		filename := attachmentFilename(header)
		content, err := io.ReadAll(part.Body)
		if err != nil {
			return ParsedMessage{}, err
		}
		attachment := classifyAttachment(filename, content)
		parsed.Attachments = append(parsed.Attachments, attachment)
	}
	return parsed, nil
}

func SaveParsedMessage(parsed ParsedMessage, options SaveOptions) (Message, error) {
	message := Message{UID: parsed.UID, Subject: parsed.Subject, Date: parsed.Date}
	for _, parsedAttachment := range parsed.Attachments {
		attachment := Attachment{
			OriginalFilename:   parsedAttachment.OriginalFilename,
			Filename:           parsedAttachment.Filename,
			Size:               parsedAttachment.Size,
			Extension:          parsedAttachment.Extension,
			IsInvoiceCandidate: parsedAttachment.IsInvoiceCandidate,
			IsTravelForm:       parsedAttachment.IsTravelForm,
			SkippedReason:      parsedAttachment.SkippedReason,
		}
		if parsedAttachment.SkippedReason != "" || (!parsedAttachment.IsInvoiceCandidate && !parsedAttachment.IsTravelForm) {
			message.Attachments = append(message.Attachments, attachment)
			continue
		}
		targetDir := options.AttachmentDir
		if parsedAttachment.IsTravelForm && strings.TrimSpace(options.TravelDir) != "" {
			targetDir = options.TravelDir
		}
		if strings.TrimSpace(targetDir) == "" {
			attachment.SkippedReason = "missing_attachment_dir"
			message.Attachments = append(message.Attachments, attachment)
			continue
		}
		savedPath, savedName, err := WriteUniqueFile(targetDir, parsedAttachment.Filename, parsedAttachment.Content)
		if err != nil {
			return Message{}, err
		}
		attachment.Filename = savedName
		attachment.SavedPath = savedPath
		message.Attachments = append(message.Attachments, attachment)
	}
	return message, nil
}

func ParseAndSaveMessage(raw []byte, uid string, options SaveOptions) (Message, error) {
	parsed, err := ParseMessage(raw, uid)
	if err != nil {
		return Message{}, err
	}
	return SaveParsedMessage(parsed, options)
}

func MessageMatches(message ParsedMessage, startDate, endDate time.Time, keyword string) bool {
	if !dateInRange(message.Date, startDate, endDate) {
		return false
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return true
	}
	needle := strings.ToLower(keyword)
	haystack := strings.ToLower(strings.Join([]string{message.Subject, message.From, message.To, message.BodyText}, "\n"))
	return strings.Contains(haystack, needle)
}

func attachmentFilename(header *mail.AttachmentHeader) string {
	if filename, err := header.Filename(); err == nil && strings.TrimSpace(filename) != "" {
		return decodeHeaderValue(filename)
	}
	if _, params, err := header.ContentDisposition(); err == nil {
		for _, key := range []string{"filename", "name"} {
			if value := strings.TrimSpace(params[key]); value != "" {
				return decodeHeaderValue(value)
			}
		}
	}
	if _, params, err := header.ContentType(); err == nil {
		if value := strings.TrimSpace(params["name"]); value != "" {
			return decodeHeaderValue(value)
		}
	}
	return ""
}

func classifyAttachment(originalFilename string, content []byte) ParsedAttachment {
	originalFilename = strings.TrimSpace(originalFilename)
	attachment := ParsedAttachment{OriginalFilename: originalFilename, Size: int64(len(content))}
	safeName, err := SanitizeFilename(originalFilename)
	if err != nil {
		attachment.SkippedReason = "unsafe_filename"
		return attachment
	}
	attachment.Filename = safeName
	attachment.Extension = strings.ToLower(fileExtension(safeName))
	if !IsInvoiceAttachmentExtension(safeName) {
		attachment.SkippedReason = "unsupported_extension"
		return attachment
	}
	attachment.Content = append([]byte(nil), content...)
	attachment.IsTravelForm = IsTravelFormAttachmentFilename(originalFilename) || IsTravelFormAttachmentFilename(safeName)
	attachment.IsInvoiceCandidate = !attachment.IsTravelForm
	return attachment
}

func decodeHeaderValue(value string) string {
	decoded, err := headerDecoder.DecodeHeader(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(decoded) == "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(decoded)
}

func SanitizeFilename(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("empty filename")
	}
	if strings.ContainsRune(name, '\x00') || !utf8.ValidString(name) {
		return "", fmt.Errorf("unsafe filename")
	}
	name = strings.ReplaceAll(name, "\\", "/")
	base := strings.TrimSpace(path.Base(name))
	base = strings.Trim(base, " .")
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("unsafe filename")
	}
	var builder strings.Builder
	for _, r := range base {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("unsafe filename")
		}
		if strings.ContainsRune(`<>:"|?*`, r) {
			builder.WriteRune('_')
			continue
		}
		builder.WriteRune(r)
	}
	base = strings.Trim(builder.String(), " .")
	if base == "" || strings.Contains(base, "/") || strings.Contains(base, "\\") {
		return "", fmt.Errorf("unsafe filename")
	}
	return base, nil
}

func dateInRange(value, startDate, endDate time.Time) bool {
	if value.IsZero() {
		return false
	}
	day := dateOnly(value)
	if !startDate.IsZero() && day.Before(dateOnly(startDate)) {
		return false
	}
	if !endDate.IsZero() && day.After(dateOnly(endDate)) {
		return false
	}
	return true
}

func dateOnly(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	year, month, day := value.In(time.Local).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func fileExtension(filename string) string {
	return filepath.Ext(filename)
}
