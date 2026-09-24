package issue

import (
	"encoding/base64"
	"net/url"
	"regexp"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	invoiceOssProxyPath   = "/api/v1/skill/share/oss-file"
	invoiceOssProxySource = "invoice-sync"
)

var (
	invoiceOssExpiresPattern   = regexp.MustCompile(`^[0-9]{1,20}$`)
	invoiceOssSignaturePattern = regexp.MustCompile(`^[A-Za-z0-9+/=]{1,512}$`)
)

func issuePDFDownloadLink(fileInfo map[string]any) string {
	rawURL := issueFileURL(fileInfo, "pdf")
	return transformInvoiceOssDownloadURL(rawURL, common.DefaultBaseURL)
}

func transformInvoiceOssDownloadURL(rawURL, openAPISkillBaseURL string) string {
	if rawURL == "" {
		return ""
	}

	signedURL, err := url.Parse(rawURL)
	if err != nil || !isSignedInvoiceSourceURL(signedURL) {
		return rawURL
	}

	query, err := url.ParseQuery(signedURL.RawQuery)
	if err != nil || len(query) != 4 {
		return rawURL
	}
	accessKeyID, ok := singleQueryValue(query, "OSSAccessKeyId")
	if !ok || len(accessKeyID) > 128 {
		return rawURL
	}
	expires, ok := singleQueryValue(query, "Expires")
	if !ok || !invoiceOssExpiresPattern.MatchString(expires) {
		return rawURL
	}
	signature, ok := singleQueryValue(query, "Signature")
	if !ok || !invoiceOssSignaturePattern.MatchString(signature) {
		return rawURL
	}
	securityToken, ok := singleQueryValue(query, "security-token")
	if !ok || len(securityToken) > 4096 {
		return rawURL
	}

	objectKey := strings.TrimPrefix(signedURL.Path, "/")
	if objectKey == "" || strings.HasPrefix(objectKey, "/") {
		return rawURL
	}

	proxyBaseURL, err := url.Parse(openAPISkillBaseURL)
	if err != nil || proxyBaseURL.Host == "" || proxyBaseURL.User != nil || (proxyBaseURL.Scheme != "http" && proxyBaseURL.Scheme != "https") {
		return rawURL
	}

	proxyURL := url.URL{
		Scheme: proxyBaseURL.Scheme,
		Host:   proxyBaseURL.Host,
		Path:   invoiceOssProxyPath,
	}
	proxyURL.RawQuery = url.Values{
		"source":    {invoiceOssProxySource},
		"objectKey": {objectKey},
		"nmyk":      {base64.RawURLEncoding.EncodeToString([]byte(accessKeyID))},
		"Expires":   {expires},
		"nmys":      {signature},
		"st":        {securityToken},
	}.Encode()
	return proxyURL.String()
}

// isSignedInvoiceSourceURL validates only the input shape needed to extract signed fields.
// The OSS destination is fixed server-side by source=invoice-sync; this input host is never forwarded.
func isSignedInvoiceSourceURL(candidate *url.URL) bool {
	return candidate != nil &&
		candidate.Scheme == "https" &&
		candidate.User == nil &&
		candidate.Hostname() != "" &&
		candidate.Port() == "" &&
		candidate.Fragment == ""
}

func singleQueryValue(query url.Values, key string) (string, bool) {
	values, ok := query[key]
	if !ok || len(values) != 1 || values[0] == "" {
		return "", false
	}
	return values[0], true
}
