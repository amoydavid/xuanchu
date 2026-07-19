// Package safefetch 提供无 Cookie、无凭证、逐跳重验的公网图片抓取。
//
// 只允许 http:80/https:443，DNS 解析必须全部为公网地址，自定义 DialContext
// 只连接本次已校验的 IP，避免 DNS rebinding。HTTPS→HTTP redirect 拒绝。
// 不携带 Cookie/Authorization/Referer，禁用环境代理。
package safefetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/netguard"
)

// 错误码与 spec §18 一致。
const (
	ErrCodeRemoteURLInvalid  = "attachment_remote_url_invalid"
	ErrCodeRemoteFetchFailed = "attachment_remote_fetch_failed"
	ErrCodeRemoteTooLarge    = "attachment_too_large"
)

// FetchError 携带稳定 code。
type FetchError struct {
	Code    string
	Message string
	Cause   error
}

func (e FetchError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is/As。
func (e FetchError) Unwrap() error { return e.Cause }

func urlInvalid(reason string) FetchError { return FetchError{Code: ErrCodeRemoteURLInvalid, Message: reason} }
func fetchFailed(reason string) FetchError { return FetchError{Code: ErrCodeRemoteFetchFailed, Message: reason} }
func tooLarge() FetchError                 { return FetchError{Code: ErrCodeRemoteTooLarge, Message: "remote response exceeds size limit"} }

// Config 描述 safefetch 行为。
type Config struct {
	Timeout       time.Duration
	MaxRedirects  int
	MaxBytes      int64
	Resolver      netguard.Resolver
	// UserAgent 用于固定无身份的 UA。
	UserAgent string
	// allowLoopback 仅供单元测试使用：跳过 netguard 私网/loopback 拦截和
	// 端口 80/443 限制，让 fetcher 能连接本机 httptest server。
	// 生产路径绝对不得开启。
	allowLoopback bool
}

// WithAllowLoopbackForTest 返回一个开启 loopback 测试豁免的 Fetcher 构造器。
//
// 该 helper 只在单元测试中使用；任何生产装配都不得调用。
func WithAllowLoopbackForTest(cfg Config) (*Fetcher, error) {
	cfg.allowLoopback = true
	return New(cfg)
}

// Result 是 Fetch 返回的受限读取结果。
type Result struct {
	Body          io.ReadCloser
	ContentLength int64
	SourceHost    string
	FileName      string
	FinalURL      string
	RedirectCount int
}

// Fetcher 封装一个可复用的 HTTP client 与配置。
type Fetcher struct {
	cfg        Config
	httpClient *http.Client
}

// New 构造 Fetcher。cfg.Timeout 必须为正，MaxBytes 为正。
func New(cfg Config) (*Fetcher, error) {
	if cfg.Timeout <= 0 {
		return nil, errors.New("safefetch: timeout must be positive")
	}
	if cfg.MaxBytes <= 0 {
		return nil, errors.New("safefetch: max bytes must be positive")
	}
	if cfg.MaxRedirects < 0 {
		return nil, errors.New("safefetch: max redirects must be non-negative")
	}
	if cfg.Resolver == nil {
		cfg.Resolver = netguard.DefaultResolver()
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "xuanchu-attachments/1.0"
	}
	transport := &http.Transport{
		Proxy:                 nil, // 禁用环境代理
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext:           dialPublicIP(cfg.Resolver, cfg.allowLoopback),
	}
	f := &Fetcher{
		cfg: cfg,
		httpClient: &http.Client{
			Transport:     transport,
			Timeout:       cfg.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	f.httpClient.CheckRedirect = f.checkRedirect
	return f, nil
}

// checkRedirect 由 http.Client 在每跳 redirect 时调用。
//
// 我们自己手动跟随，这里直接返回 ErrUseLastResponse；每一跳的校验在 Fetch 循环中完成。
func (f *Fetcher) checkRedirect(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

// Fetch 抓取 rawURL，返回受限 reader。
//
// 失败返回 FetchError；调用方必须 Close Body。
func (f *Fetcher) Fetch(ctx context.Context, raw string) (Result, error) {
	if f == nil {
		return Result{}, errors.New("safefetch: fetcher is nil")
	}
	current, err := validateInitialURL(raw, f.cfg.allowLoopback)
	if err != nil {
		return Result{}, err
	}

	redirectCount := 0
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
		if err != nil {
			return Result{}, urlInvalid("cannot build request: " + err.Error())
		}
		req.Header.Set("User-Agent", f.cfg.UserAgent)
		// 不带 Cookie/Authorization/Referer；Go 默认不会跨域带这些，显式清空兜底。
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("Referer")

		resp, err := f.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			return Result{}, fetchFailed("http request failed: " + err.Error())
		}

		// 处理 redirect：3xx。
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			resp.Body.Close()
			if redirectCount >= f.cfg.MaxRedirects {
				return Result{}, urlInvalid("too many redirects")
			}
			if location == "" {
				return Result{}, urlInvalid("redirect without Location")
			}
			next, err := resolveRedirect(current, location, f.cfg.allowLoopback)
			if err != nil {
				return Result{}, urlInvalid("redirect target invalid: " + err.Error())
			}
			// HTTPS→HTTP 拒绝。
			if current.Scheme == "https" && next.Scheme == "http" {
				return Result{}, urlInvalid("https→http downgrade rejected")
			}
			redirectCount++
			current = next
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return Result{}, fetchFailed(fmt.Sprintf("remote returned status %d", resp.StatusCode))
		}

		// Content-Length 已知时立即校验大小。
		if resp.ContentLength > f.cfg.MaxBytes {
			resp.Body.Close()
			return Result{}, tooLarge()
		}

		// 用 LimitReader(max+1) 包装 body：实际读取时如果发现 >max，调用方会在 inspect 阶段拒绝。
		limited := &countingReader{r: io.LimitReader(resp.Body, f.cfg.MaxBytes+1)}
		fileName := deriveFileName(current, resp.Header.Get("Content-Disposition"))
		return Result{
			Body:          &bodyWithClose{reader: limited, closer: resp.Body},
			ContentLength: resp.ContentLength,
			SourceHost:    strings.ToLower(current.Hostname()),
			FileName:      fileName,
			FinalURL:      current.String(),
			RedirectCount: redirectCount,
		}, nil
	}
}

// dialPublicIP 构造自定义 DialContext：先解析域名、对每个 IP 做 netguard.IsBlockedIP，
// 只 dial 第一个公网 IP。Host/SNI 仍用原 hostname。
//
// allowLoopback 仅供单元测试跳过 netguard 拦截，生产必须为 false。
func dialPublicIP(resolver netguard.Resolver, allowLoopback bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		dialer := &net.Dialer{Timeout: 30 * time.Second}
		if allowLoopback {
			return dialer.DialContext(ctx, network, addr)
		}
		// 字面 IP 直接校验；不再 DNS 解析。
		if literal := net.ParseIP(host); literal != nil {
			if netguard.IsBlockedIP(literal) {
				return nil, errors.New("blocked address")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		}
		addrs, err := netguard.ResolvePublic(ctx, resolver, host)
		if err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, errors.New("no public addresses")
		}
		// 只连第一个公网 IP，避免 DNS rebinding。
		return dialer.DialContext(ctx, network, net.JoinHostPort(addrs[0].IP.String(), port))
	}
}

// validateInitialURL 校验用户传入 URL：scheme/port/userinfo。
func validateInitialURL(raw string, allowLoopback bool) (*url.URL, error) {
	if raw == "" {
		return nil, urlInvalid("empty url")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, urlInvalid("invalid url: " + err.Error())
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, urlInvalid("only http/https allowed")
	}
	if parsed.User != nil {
		return nil, urlInvalid("userinfo not allowed")
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, urlInvalid("host required")
	}
	if !allowLoopback {
		port := parsed.Port()
		if port != "" {
			n, err := strconv.Atoi(port)
			if err != nil {
				return nil, urlInvalid("invalid port")
			}
			if scheme == "http" && n != 80 {
				return nil, urlInvalid("http must use port 80")
			}
			if scheme == "https" && n != 443 {
				return nil, urlInvalid("https must use port 443")
			}
		}
	}
	// 拒绝 file/ftp/data/blob 已经被 scheme 过滤覆盖；显式拒绝残留 fragment。
	if parsed.Fragment != "" {
		return nil, urlInvalid("fragment not allowed")
	}
	return parsed, nil
}

// resolveRedirect 计算下一跳 URL，保持 scheme 校验。
func resolveRedirect(base *url.URL, location string, allowLoopback bool) (*url.URL, error) {
	ref, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	next := base.ResolveReference(ref)
	scheme := strings.ToLower(next.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("redirect scheme not allowed")
	}
	if next.User != nil {
		return nil, errors.New("redirect userinfo not allowed")
	}
	if !allowLoopback {
		port := next.Port()
		if port != "" {
			n, err := strconv.Atoi(port)
			if err != nil {
				return nil, errors.New("redirect invalid port")
			}
			if scheme == "http" && n != 80 {
				return nil, errors.New("redirect http must use port 80")
			}
			if scheme == "https" && n != 443 {
				return nil, errors.New("redirect https must use port 443")
			}
		}
	}
	return next, nil
}

// deriveFileName 从 Content-Disposition 或 URL path 推断文件名。
func deriveFileName(u *url.URL, contentDisposition string) string {
	if contentDisposition != "" {
		if name := parseContentDispositionFilename(contentDisposition); name != "" {
			return sanitizeFileName(name)
		}
	}
	base := path.Base(u.Path)
	if base == "" || base == "/" || base == "." {
		return "remote-image"
	}
	return sanitizeFileName(base)
}

// parseContentDispositionFilename 解析 filename*=UTF-8''... 或 filename="..."。
func parseContentDispositionFilename(value string) string {
	parts := strings.Split(value, ";")
	for _, part := range parts {
		trim := strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(trim), "filename*=") {
			// RFC 5987：filename*=UTF-8''name.png
			_, after, ok := strings.Cut(trim, "''")
			if !ok {
				continue
			}
			return after
		}
		if strings.HasPrefix(strings.ToLower(trim), "filename=") {
			_, after, ok := strings.Cut(trim, "=")
			if !ok {
				continue
			}
			after = strings.Trim(after, `"`)
			return after
		}
	}
	return ""
}

func sanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	if name == "" {
		return "remote-image"
	}
	return name
}

// countingReader 计数读取字节数，供调用方判断是否超限。
type countingReader struct {
	r     io.Reader
	read  int64
	max   int64
}

// bodyWithClose 让 io.LimitReader 的 EOF 关闭底层 Body。
type bodyWithClose struct {
	reader *countingReader
	closer io.Closer
}

func (b *bodyWithClose) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	return n, err
}

func (b *bodyWithClose) Close() error { return b.closer.Close() }

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += int64(n)
	return n, err
}

// Read 返回到目前为止读取的字节数。
func (c *countingReader) BytesRead() int64 { return c.read }

// SourceURLHash 返回规范化 source URL 的 SHA-256 十六进制。
//
// 规范化：scheme/host 小写、移除 fragment、保留 path/query。
func SourceURLHash(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	h := sha256.Sum256([]byte(parsed.String()))
	return hex.EncodeToString(h[:])
}
