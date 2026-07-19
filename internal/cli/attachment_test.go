package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachmentDownloadRejectsJSONAndBinaryStdout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("payload"))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr, Server: srv.URL, Token: "token"})
	cmd.SetArgs([]string{"--json", "attachment", "download", "att-1", "--output", "-"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "attachment_binary_json_conflict") {
		t.Fatalf("err = %v", err)
	}
}

func TestAttachmentDownloadStdoutDoesNotMixProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''test.png`)
		w.Write([]byte("payload"))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr, Server: srv.URL, Token: "token"})
	cmd.SetArgs([]string{"attachment", "download", "att-1", "--output", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stdout.String() != "payload" {
		t.Fatalf("stdout = %q, want %q", stdout.String(), "payload")
	}
	if strings.Contains(stdout.String(), "Downloading") {
		t.Fatalf("progress leaked to stdout: %q", stdout.String())
	}
}

func TestAttachmentDownloadOutputExists(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/attachments/att-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":{"id":"att-1","display_name":"test.png"}}`)
	})
	mux.HandleFunc("/api/v1/attachments/att-1/content", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("payload"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	existingPath := filepath.Join(dir, "test.png")
	if err := os.WriteFile(existingPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr, Server: srv.URL, Token: "token"})
	_ = os.Chdir(dir)
	cmd.SetArgs([]string{"attachment", "download", "att-1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "attachment_output_exists") {
		t.Fatalf("err = %v", err)
	}
	got, _ := os.ReadFile(existingPath)
	if string(got) != "old" {
		t.Fatalf("existing file was modified: %q", got)
	}
}

func TestAttachmentListRemote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"id":"att-1","state":"active","display_name":"a.png","media_type":"image/png","size_bytes":3}]}`)
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr, Server: srv.URL, Token: "token"})
	cmd.SetArgs([]string{"attachment", "list", "task-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(stdout.String(), "att-1") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

// 避免 unused import
var _ = context.Background
