package cli

import (
	"bytes"
	"testing"
)

func TestRootCommandVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{
		Stdout:  &stdout,
		Stderr:  &stderr,
		Version: "test",
	})
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stdout.String(); got != "taskg test\n" {
		t.Fatalf("stdout = %q, want %q", got, "taskg test\n")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
