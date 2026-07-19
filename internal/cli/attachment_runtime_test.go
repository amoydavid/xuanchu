package cli

import (
	"io"
	"path/filepath"
	"testing"
)

// TestBuildServiceFromOptsInjectsAttachmentRuntime 验证本地 CLI 构造的 service 自带附件运行时。
func TestBuildServiceFromOptsInjectsAttachmentRuntime(t *testing.T) {
	opts := Options{
		DataDir: filepath.Join(t.TempDir(), "share", "xuanchu"),
		Stdout:  io.Discard,
		Stderr:  io.Discard,
	}
	svc, closeFn, err := buildServiceFromOpts(opts)
	if err != nil {
		t.Fatalf("buildServiceFromOpts: %v", err)
	}
	defer closeFn()
	if svc.AttachmentRuntime() == nil {
		t.Fatal("attachment runtime missing on local CLI service")
	}
}
