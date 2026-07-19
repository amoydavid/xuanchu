package app

import (
	"context"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/attachments"
	"git.dajee.net/dajee/xuanchu/internal/blobstore"
)

func TestAttachmentRuntimeFilesystemOnly(t *testing.T) {
	cfg := attachments.DefaultConfig(t.TempDir())
	rt, err := NewAttachmentRuntime(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewAttachmentRuntime: %v", err)
	}
	if _, err := rt.StoreFor("filesystem"); err != nil {
		t.Fatalf("StoreFor filesystem: %v", err)
	}
	if _, err := rt.StoreFor("s3"); err == nil {
		t.Fatal("expected s3 unavailable")
	}
	if _, err := rt.StoreFor("missing"); err == nil {
		t.Fatal("expected missing backend unavailable")
	}
}

func TestAttachmentRuntimeInvalidConfig(t *testing.T) {
	cfg := attachments.DefaultConfig(t.TempDir())
	cfg.Backend = "gcs"
	if _, err := NewAttachmentRuntime(context.Background(), cfg); err == nil {
		t.Fatal("expected config validation error")
	}
}

func TestAttachmentRuntimeReadsByRowBackend(t *testing.T) {
	fs := blobstore.NewFilesystem(t.TempDir())
	rt := &AttachmentRuntime{Stores: map[string]blobstore.Store{"filesystem": fs}}
	if store, err := rt.StoreFor("filesystem"); err != nil || store != fs {
		t.Fatalf("StoreFor filesystem = %v %v", store, err)
	}
}
