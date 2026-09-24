package mailimap

import (
	"fmt"
	"regexp"
	"strings"
)

const DefaultMailbox = "INBOX"

var providerHosts = map[string]string{
	"qq":    "imap.qq.com",
	"163":   "imap.163.com",
	"gmail": "imap.gmail.com",
}

var travelFormPattern = regexp.MustCompile(`行程.*单`)

type ServerConfig struct {
	Host     string
	Port     int
	Provider string
}

func ResolveServer(mailProvider, imapHost string, port int) (ServerConfig, error) {
	provider := strings.ToLower(strings.TrimSpace(mailProvider))
	host := strings.TrimSpace(imapHost)
	if port <= 0 {
		port = 993
	}
	if provider != "" {
		providerHost, ok := providerHosts[provider]
		if !ok {
			return ServerConfig{}, fmt.Errorf("invalid --mail-provider: %s", mailProvider)
		}
		return ServerConfig{Host: providerHost, Port: port, Provider: provider}, nil
	}
	if host == "" {
		host = providerHosts["qq"]
		provider = "qq"
	}
	return ServerConfig{Host: host, Port: port, Provider: provider}, nil
}

func IsInvoiceAttachmentExtension(filename string) bool {
	switch strings.ToLower(fileExtension(filename)) {
	case ".pdf", ".jpg", ".jpeg", ".png", ".ofd":
		return true
	default:
		return false
	}
}

func IsTravelFormAttachmentFilename(filename string) bool {
	return travelFormPattern.MatchString(strings.TrimSpace(filename))
}
