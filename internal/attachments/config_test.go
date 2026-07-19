package attachments

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigMatchesSpecDefaults(t *testing.T) {
	cfg := DefaultConfig("/data")
	if cfg.Backend != "filesystem" {
		t.Fatalf("backend = %q", cfg.Backend)
	}
	if cfg.FilesystemDir != "/data/attachments" {
		t.Fatalf("dir = %q", cfg.FilesystemDir)
	}
	if cfg.MaxFileSizeBytes != 25<<20 {
		t.Fatalf("max file = %d", cfg.MaxFileSizeBytes)
	}
	if cfg.MaxResourceTotalSizeBytes != 200<<20 {
		t.Fatalf("max resource = %d", cfg.MaxResourceTotalSizeBytes)
	}
	if cfg.MaxWorkspaceTotalSizeBytes != 10<<30 {
		t.Fatalf("max workspace = %d", cfg.MaxWorkspaceTotalSizeBytes)
	}
	if cfg.MaxAttachmentsPerResource != 100 {
		t.Fatalf("count = %d", cfg.MaxAttachmentsPerResource)
	}
	if cfg.DraftTTL != 24*time.Hour {
		t.Fatalf("draft ttl = %v", cfg.DraftTTL)
	}
	if cfg.DeletedRetention != 720*time.Hour {
		t.Fatalf("deleted retention = %v", cfg.DeletedRetention)
	}
	if !cfg.RemoteFetchEnabled {
		t.Fatalf("remote fetch should default to enabled")
	}
	if cfg.RemoteFetchTimeout != 30*time.Second {
		t.Fatalf("timeout = %v", cfg.RemoteFetchTimeout)
	}
	if cfg.RemoteFetchMaxRedirects != 5 || cfg.RemoteFetchMaxConcurrency != 4 {
		t.Fatalf("redirects/concurrency = %d/%d", cfg.RemoteFetchMaxRedirects, cfg.RemoteFetchMaxConcurrency)
	}
	if cfg.S3.Prefix != "xuanchu/attachments" {
		t.Fatalf("s3 prefix = %q", cfg.S3.Prefix)
	}
}

func TestConfigValidateRejectsInvalidBackend(t *testing.T) {
	cfg := DefaultConfig("/data")
	cfg.Backend = "gcs"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigValidateRejectsInvalidQuotaOrder(t *testing.T) {
	cfg := DefaultConfig("/data")
	cfg.MaxResourceTotalSizeBytes = cfg.MaxFileSizeBytes - 1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_resource_total_size_mb") {
		t.Fatalf("err = %v", err)
	}
	cfg = DefaultConfig("/data")
	cfg.MaxWorkspaceTotalSizeBytes = cfg.MaxResourceTotalSizeBytes - 1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_workspace_total_size_mb") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigValidateRejectsZeroFileCount(t *testing.T) {
	cfg := DefaultConfig("/data")
	cfg.MaxAttachmentsPerResource = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_attachments_per_resource") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigValidateRejectsNegativeRetention(t *testing.T) {
	cfg := DefaultConfig("/data")
	cfg.DraftTTL = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "draft_ttl") {
		t.Fatalf("err = %v", err)
	}
	cfg = DefaultConfig("/data")
	cfg.DeletedRetention = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "deleted_retention") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigValidateRejectsInvalidRemoteFetch(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		message string
	}{
		{"zero_timeout", func(c *Config) { c.RemoteFetchTimeout = 0 }, "remote_fetch_timeout"},
		{"oversize_timeout", func(c *Config) { c.RemoteFetchTimeout = 121 * time.Second }, "remote_fetch_timeout"},
		{"too_many_redirects", func(c *Config) { c.RemoteFetchMaxRedirects = 11 }, "remote_fetch_max_redirects"},
		{"negative_redirects", func(c *Config) { c.RemoteFetchMaxRedirects = -1 }, "remote_fetch_max_redirects"},
		{"zero_concurrency", func(c *Config) { c.RemoteFetchMaxConcurrency = 0 }, "remote_fetch_max_concurrency"},
		{"oversize_concurrency", func(c *Config) { c.RemoteFetchMaxConcurrency = 33 }, "remote_fetch_max_concurrency"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig("/data")
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("err = %v, want %s", err, tc.message)
			}
		})
	}
}

func TestConfigValidateRejectsInvalidS3WhenBackend(t *testing.T) {
	cfg := DefaultConfig("/data")
	cfg.Backend = "s3"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Fatalf("err = %v", err)
	}
	cfg.S3.Bucket = "private"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "region") {
		t.Fatalf("err = %v", err)
	}
	cfg.S3.Region = "us-east-1"
	cfg.S3.ServerSideEncryption = "aws:kms"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "kms_key_id") {
		t.Fatalf("err = %v", err)
	}
	cfg.S3.ServerSideEncryption = "AES256"
	cfg.S3.KMSKeyID = "alias/x"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "kms_key_id") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigValidateAcceptsDefault(t *testing.T) {
	cfg := DefaultConfig("/data")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("err = %v", err)
	}
}
