package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
