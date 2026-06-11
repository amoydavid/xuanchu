package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPRequiresBearerTokenForPostAndGet(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})

	post := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Accept", "application/json, text/event-stream")
	srv.Router().ServeHTTP(post, postReq)
	assertHTTPErrorCode(t, post, http.StatusUnauthorized, "auth_missing_token")

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	getReq.Header.Set("Accept", "text/event-stream")
	srv.Router().ServeHTTP(get, getReq)
	assertHTTPErrorCode(t, get, http.StatusUnauthorized, "auth_missing_token")
}

func TestMCPBodyLimitUsesHTTPMiddleware(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	fixture.server.bodyLimitBytes = 4

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"too":"large"}`))
	req.Header.Set("Authorization", "Bearer "+fixture.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	fixture.server.Router().ServeHTTP(rr, req)

	assertHTTPErrorCode(t, rr, http.StatusRequestEntityTooLarge, "api_payload_too_large")
}

func TestMCPRejectsExternalHostOnLoopbackByDefault(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})

	req := mcpLoopbackRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Host = "xuanchu.example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestMCPTrustedProxyHostReachesAuthMiddleware(t *testing.T) {
	srv := NewServer(Options{
		Store: openHTTPTestStore(t),
		MCPTrustedProxyHosts: []string{
			"xuanchu.example.com",
		},
	})

	req := mcpLoopbackRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Host = "xuanchu.example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")
}

func TestMCPRejectsUntrustedProxyHost(t *testing.T) {
	srv := NewServer(Options{
		Store: openHTTPTestStore(t),
		MCPTrustedProxyHosts: []string{
			"trusted.example.com",
		},
	})

	req := mcpLoopbackRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Host = "evil.example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func mcpLoopbackRequest(method, target string, body *strings.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 8080,
	})
	return req.WithContext(ctx)
}
