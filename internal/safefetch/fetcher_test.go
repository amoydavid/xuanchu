package safefetch

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustNewTestFetcher(t *testing.T, cfg Config) *Fetcher {
	t.Helper()
	cfg.Timeout = 5 * time.Second
	cfg.MaxBytes = 1024
	f, err := WithAllowLoopbackForTest(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

func TestFetchRejectsInvalidSchemes(t *testing.T) {
	// URL 结构校验必须在 allowLoopback 关闭时生效；用一个严格 fetcher。
	strict, err := New(Config{Timeout: 2 * time.Second, MaxRedirects: 3, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"file:///etc/passwd",
		"ftp://example.com/a.png",
		"data:image/png;base64,xx",
		"blob:https://example.com/abc",
		"https://user:pass@example.com/a.png",
		"https://example.com:8443/a.png",
		"http://example.com:8080/a.png",
		"https://example.com/a.png#frag",
		"",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := strict.Fetch(context.Background(), raw)
			var fe FetchError
			if !errors.As(err, &fe) || fe.Code != ErrCodeRemoteURLInvalid {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestFetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("payload"))
	}))
	defer srv.Close()
	f := mustNewTestFetcher(t, Config{MaxRedirects: 3})
	res, err := f.Fetch(context.Background(), srv.URL+"/dir/a.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer res.Body.Close()
	got, _ := io.ReadAll(res.Body)
	if string(got) != "payload" {
		t.Fatalf("body = %q", got)
	}
	if res.FileName != "a.png" {
		t.Fatalf("filename = %q", res.FileName)
	}
}

func TestFetchFollowsRedirectAndRevalidatesEachHop(t *testing.T) {
	hops := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops = append(hops, r.URL.Path)
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/middle", http.StatusFound)
		case "/middle":
			http.Redirect(w, r, "/final.png", http.StatusFound)
		case "/final.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := mustNewTestFetcher(t, Config{MaxRedirects: 5})
	res, err := f.Fetch(context.Background(), srv.URL+"/start")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer res.Body.Close()
	if res.RedirectCount != 2 {
		t.Fatalf("redirects = %d", res.RedirectCount)
	}
	if res.FileName != "final.png" {
		t.Fatalf("filename = %q", res.FileName)
	}
}

func TestFetchRejectsTooManyRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()
	f := mustNewTestFetcher(t, Config{MaxRedirects: 2})
	_, err := f.Fetch(context.Background(), srv.URL+"/loop")
	var fe FetchError
	if !errors.As(err, &fe) || fe.Code != ErrCodeRemoteURLInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchRejectsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	f := mustNewTestFetcher(t, Config{MaxRedirects: 3})
	_, err := f.Fetch(context.Background(), srv.URL+"/a.png")
	var fe FetchError
	if !errors.As(err, &fe) || fe.Code != ErrCodeRemoteFetchFailed {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchRejectsOversizeContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999999")
		w.Write(make([]byte, 100))
	}))
	defer srv.Close()
	f := mustNewTestFetcher(t, Config{MaxRedirects: 3, MaxBytes: 50})
	_, err := f.Fetch(context.Background(), srv.URL+"/a.png")
	var fe FetchError
	if !errors.As(err, &fe) || fe.Code != ErrCodeRemoteTooLarge {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchRejectsBlockedDNSResolution(t *testing.T) {
	// 不开 allowLoopback 的 Fetcher：解析到 loopback 应被拒绝。
	// 我们指向一个本机 httptest server，但 resolver 返回 127.0.0.1。
	stub := &stubResolver{addrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}
	f, err := New(Config{
		Timeout:      2 * time.Second,
		MaxRedirects: 3,
		MaxBytes:     1024,
		Resolver:     stub,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Fetch(context.Background(), "https://example.com/a.png")
	if err == nil {
		t.Fatal("expected blocked DNS rejection")
	}
}

func TestFetchPinsValidatedIPAndRejectsRebinding(t *testing.T) {
	// 模拟 DNS rebinding：第一次解析返回公网，DialContext 使用 IP；但本测试只验证
	// 当 resolver 返回私网地址时 fetch 失败。
	stub := &stubResolver{addrs: []net.IPAddr{
		{IP: net.ParseIP("93.184.216.34")},
		{IP: net.ParseIP("10.0.0.1")}, // 混合结果必须整次失败
	}}
	f, err := New(Config{
		Timeout:      2 * time.Second,
		MaxRedirects: 3,
		MaxBytes:     1024,
		Resolver:     stub,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Fetch(context.Background(), "https://example.com/a.png")
	if err == nil {
		t.Fatal("expected mixed DNS rejection")
	}
}

func TestFetchStripsCookiesAndAuth(t *testing.T) {
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	f := mustNewTestFetcher(t, Config{MaxRedirects: 3})
	res, err := f.Fetch(context.Background(), srv.URL+"/a.png")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if seen.Get("Cookie") != "" || seen.Get("Authorization") != "" || seen.Get("Referer") != "" {
		t.Fatalf("unexpected headers: %#v", seen)
	}
	if !strings.HasPrefix(seen.Get("User-Agent"), "xuanchu-attachments") {
		t.Fatalf("UA = %q", seen.Get("User-Agent"))
	}
}

func TestSourceURLHashNormalizes(t *testing.T) {
	a := SourceURLHash("https://Example.com/a.png?token=1#frag")
	b := SourceURLHash("HTTPS://example.com/a.png?token=1")
	if a != b {
		t.Fatalf("hash mismatch: %s vs %s", a, b)
	}
}

type stubResolver struct {
	addrs []net.IPAddr
	err   error
}

func (s *stubResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return s.addrs, s.err
}
