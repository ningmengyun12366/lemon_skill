package skillupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxDocBytes    = 1 << 20
	defaultRetries = 2
	defaultTimeout = 5 * time.Second
)

var (
	versionLinePattern = regexp.MustCompile(`(?m)^- version:\s*(\S+)\s*$`)
	semverPattern      = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
)

// Checker blocks a remote business operation until the installed skill version is current.
type Checker interface {
	Check(context.Context) error
}

// CheckFunc adapts a function to Checker for custom transport boundaries.
type CheckFunc func(context.Context) error

func (check CheckFunc) Check(ctx context.Context) error {
	return check(ctx)
}

// OutdatedError is the exact user-facing failure returned for a version mismatch.
type OutdatedError struct {
	RemoteVersion string
	InstallDocURL string
}

func (err *OutdatedError) Error() string {
	return fmt.Sprintf("当前 skill 版本过低，无法执行，最新版是 %s 版本，请从 %s 链接获取最新安装教程并重新安装本 skill。您可告诉agent，\"更新并重装skill\" agent就会自动拉取最新skill并重装。", err.RemoteVersion, err.InstallDocURL)
}

// UserFacingError unwraps an outdated-version failure so CLI output stays exact.
func UserFacingError(err error) error {
	if outdated, ok := FindOutdatedError(err); ok {
		return outdated
	}
	return err
}

// FindOutdatedError locates a version mismatch through ordinary Go error wrapping.
func FindOutdatedError(err error) (*OutdatedError, bool) {
	if err == nil {
		return nil, false
	}
	var outdated *OutdatedError
	if errors.As(err, &outdated) {
		return outdated, true
	}
	return nil, false
}

// Gate fetches one installation document and caches its result for this process.
type Gate struct {
	skillName    string
	localVersion string
	installURL   string
	httpClient   *http.Client
	retries      int
	retryDelay   func(int) time.Duration
	once         sync.Once
	err          error
}

// Option customizes a Gate. Production callers should use the defaults.
type Option func(*Gate)

// WithHTTPClient replaces the installation-document HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(gate *Gate) {
		if client != nil {
			gate.httpClient = client
		}
	}
}

// WithRetries changes how many retries follow the first attempt.
func WithRetries(retries int) Option {
	return func(gate *Gate) {
		if retries < 0 {
			retries = 0
		}
		gate.retries = retries
	}
}

// WithRetryDelay changes the retry delay used after a failed document request.
func WithRetryDelay(delay func(int) time.Duration) Option {
	return func(gate *Gate) {
		if delay != nil {
			gate.retryDelay = delay
		}
	}
}

// New constructs an independent version gate for a published skill.
func New(skillName, installDocURL string, options ...Option) *Gate {
	localVersion, _ := Version(strings.TrimSpace(skillName))
	gate := &Gate{
		skillName:    strings.TrimSpace(skillName),
		localVersion: localVersion,
		installURL:   strings.TrimSpace(installDocURL),
		httpClient:   secureHTTPClient(),
		retries:      defaultRetries,
		retryDelay: func(attempt int) time.Duration {
			return time.Duration(attempt) * 500 * time.Millisecond
		},
	}
	for _, option := range options {
		option(gate)
	}
	return gate
}

var sharedGates = struct {
	sync.Mutex
	items map[string]*Gate
}{items: map[string]*Gate{}}

// Shared returns one process-wide gate for a skill and installation-document URL.
func Shared(skillName, installDocURL string) Checker {
	key := strings.TrimSpace(skillName) + "\x00" + strings.TrimSpace(installDocURL)
	sharedGates.Lock()
	defer sharedGates.Unlock()
	if gate, ok := sharedGates.items[key]; ok {
		return gate
	}
	gate := New(skillName, installDocURL)
	sharedGates.items[key] = gate
	return gate
}

// Check fetches and compares the remote version once for the lifetime of this process.
func (gate *Gate) Check(ctx context.Context) error {
	gate.once.Do(func() {
		gate.err = gate.check(ctx)
	})
	return gate.err
}

func (gate *Gate) check(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if gate.localVersion == "" {
		return fmt.Errorf("skill 版本检查失败：未知 skill %q", gate.skillName)
	}
	if err := validateInstallDocURL(gate.installURL); err != nil {
		return fmt.Errorf("skill 版本检查失败：%v；安装文档 URL：%s", err, gate.installURL)
	}

	var lastErr error
	for attempt := 0; attempt <= gate.retries; attempt++ {
		remoteVersion, err := gate.fetchVersion(ctx)
		if err == nil {
			if remoteVersion != gate.localVersion {
				return &OutdatedError{RemoteVersion: remoteVersion, InstallDocURL: gate.installURL}
			}
			return nil
		}
		lastErr = err
		if attempt == gate.retries || ctx.Err() != nil {
			break
		}
		delay := gate.retryDelay(attempt + 1)
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				lastErr = ctx.Err()
				attempt = gate.retries
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("skill 版本检查失败：%v；无法执行远程操作；安装文档 URL：%s", lastErr, gate.installURL)
}

func (gate *Gate) fetchVersion(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, gate.installURL, nil)
	if err != nil {
		return "", errors.New("无法创建安装文档请求")
	}
	request.Header.Set("Accept", "text/markdown, text/plain;q=0.9")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := gate.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("获取安装文档失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("安装文档返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDocBytes+1))
	if err != nil {
		return "", errors.New("读取安装文档失败")
	}
	if len(body) > maxDocBytes {
		return "", errors.New("安装文档超过 1 MiB 限制")
	}
	matches := versionLinePattern.FindAllSubmatch(body, -1)
	if len(matches) != 1 {
		return "", errors.New("安装文档必须且只能包含一行 '- version: <SemVer>'")
	}
	version := string(matches[0][1])
	if !semverPattern.MatchString(version) {
		return "", errors.New("安装文档 version 不是有效 SemVer")
	}
	return version, nil
}

func secureHTTPClient() *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("安装文档重定向次数超过限制")
			}
			return validateInstallDocURL(request.URL.String())
		},
	}
}

func validateInstallDocURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("安装文档 URL 无效")
	}
	if parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" || parsed.Opaque != "" {
		return errors.New("安装文档 URL 必须是 HTTPS 地址，且不得包含用户信息、查询参数或片段")
	}
	return nil
}
