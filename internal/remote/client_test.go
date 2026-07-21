package remote

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func TestRemoteClientSetsAsHeader(t *testing.T) {
	var receivedAs string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAs = r.Header.Get("X-Xuanchu-As")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	client, err := NewClient(Options{
		BaseURL: srv.URL,
		Token:   "test-token",
		AsUser:  "alice",
	})
	if err != nil {
		t.Fatal(err)
	}

	var result apiEnvelope[[]any]
	if err := client.get(context.Background(), "/api/v1/tasks", nil, &result); err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if receivedAs != "alice" {
		t.Fatalf("X-Xuanchu-As = %q, want %q", receivedAs, "alice")
	}
}

func TestRemoteClientOmitsAsHeaderWhenEmpty(t *testing.T) {
	var receivedAs string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAs = r.Header.Get("X-Xuanchu-As")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	client, err := NewClient(Options{
		BaseURL: srv.URL,
		Token:   "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}

	var result apiEnvelope[[]any]
	if err := client.get(context.Background(), "/api/v1/tasks", nil, &result); err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if receivedAs != "" {
		t.Fatalf("X-Xuanchu-As = %q, want empty", receivedAs)
	}
}

func TestRemoteClientRejectsActingTokenPrefix(t *testing.T) {
	_, err := NewClient(Options{
		BaseURL: "http://127.0.0.1:8080",
		Token:   "xuanchu_act_test",
	})
	if err == nil {
		t.Fatal("expected error for acting token")
	}
	if !strings.Contains(err.Error(), "acting") {
		t.Fatalf("error = %v, want acting token rejection", err)
	}
	// 错误应携带稳定的 code，便于上层识别。
	if apiErr, ok := err.(APIError); !ok || apiErr.Code != "remote_acting_token_not_allowed" {
		t.Fatalf("error = %v, want APIError with remote_acting_token_not_allowed", err)
	}
}

func TestRemoteClientAcceptsPATAndAgentTokens(t *testing.T) {
	for _, token := range []string{"xuanchu_pat_abc", "xuanchu_agent_def"} {
		if _, err := NewClient(Options{
			BaseURL: "http://127.0.0.1:8080",
			Token:   token,
		}); err != nil {
			t.Fatalf("NewClient(%s) error = %v", token, err)
		}
	}
}

func TestAPIErrorNormalizesToRuntimeAndPermissionErrorsWithoutLosingStatus(t *testing.T) {
	err := APIError{Status: http.StatusForbidden, Code: "permission_denied", Message: "permission denied"}
	var apiErr APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden || apiErr.Code != "permission_denied" || apiErr.Message != "permission denied" {
		t.Fatalf("APIError status/code/message changed: %#v", apiErr)
	}
	var runtimeErr app.RuntimeError
	if !errors.As(err, &runtimeErr) || runtimeErr.Code != "permission_denied" || runtimeErr.Message != "permission denied" {
		t.Fatalf("RuntimeError = %#v", runtimeErr)
	}
	var permissionErr app.PermissionError
	if !errors.As(err, &permissionErr) || permissionErr.Code != "permission_denied" || permissionErr.Message != "permission denied" {
		t.Fatalf("PermissionError = %#v", permissionErr)
	}
}
