package cli

import (
	"bytes"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth"
)

func TestAdminTokenGenerateCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	cmd.SetArgs([]string{"admin", "token", "generate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "token: xuanchu_admin_") || !strings.Contains(out, "hash: sha256:") {
		t.Fatalf("output = %q", out)
	}
}

func TestAdminTokenHashCommandReadsStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	raw := "xuanchu_admin_test_token"
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(raw)})
	cmd.SetArgs([]string{"admin", "token", "hash"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v stderr=%s", err, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != auth.HashAdminToken(raw) {
		t.Fatalf("hash = %q, want %q", got, auth.HashAdminToken(raw))
	}
}
