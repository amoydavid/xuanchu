package httpapi

import (
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
