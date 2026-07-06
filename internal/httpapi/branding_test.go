package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/config"
)

func TestBrandingIsAnonymous(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/branding", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestBrandingReturnsNameFields(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/branding", nil)
	srv.Router().ServeHTTP(rr, req)

	var got struct {
		NameZh string `json:"name_zh"`
		NameEn string `json:"name_en"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if got.NameZh == "" {
		t.Fatalf("name_zh empty body=%s", rr.Body.String())
	}
	if got.NameEn == "" {
		t.Fatalf("name_en empty body=%s", rr.Body.String())
	}
}

func TestBrandingHasCacheControlHeader(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/branding", nil)
	srv.Router().ServeHTTP(rr, req)
	if cc := rr.Header().Get("Cache-Control"); cc == "" {
		t.Fatalf("missing Cache-Control header")
	}
}

// 验证 console 启用（生产路径布局）时 /api/branding 仍可达，
// 且未被 console SPA fallback 吞掉。
func TestBrandingReachableWhenConsoleEnabled(t *testing.T) {
	srv := NewServer(Options{
		Store:             openHTTPTestStore(t),
		Console:           config.ConsoleConfig{Enabled: true},
		TestConsoleHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "console-fallback", http.StatusOK)
		}),
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/branding", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		NameZh string `json:"name_zh"`
		NameEn string `json:"name_en"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v body=%s (console fallback 吞掉了 branding?)", err, rr.Body.String())
	}
}
