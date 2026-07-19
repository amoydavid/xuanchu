package app

import (
	"context"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/attachments"
	"git.dajee.net/dajee/xuanchu/internal/blobstore"
)

// AttachmentRuntime 是附件存储后端和远程抓取的运行时集合。
//
// Stores 按 backend 名注册；每条附件记录保存自己的写入 backend，
// 读取/删除按记录中的 backend 分派。Fetcher 由 Task 7 的 safefetch 填充。
type AttachmentRuntime struct {
	Config  attachments.Config
	Stores  map[string]blobstore.Store
	Fetcher any // *safefetch.Fetcher，运行时注入，避免本文件依赖 safefetch 形成强约束。
}

// StoreFor 按记录中的 backend 返回对应 store。
func (r *AttachmentRuntime) StoreFor(name string) (blobstore.Store, error) {
	if r == nil || r.Stores == nil {
		return nil, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachment storage backend is unavailable"}
	}
	store, ok := r.Stores[name]
	if !ok {
		return nil, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachment storage backend is unavailable"}
	}
	return store, nil
}

// NewAttachmentRuntime 根据配置构造附件运行时。
//
// filesystem 永远注册；backend=s3 时同时注册 s3，并对所有已配置 store
// 执行 Health 检查（fail fast）。
func NewAttachmentRuntime(ctx context.Context, cfg attachments.Config) (*AttachmentRuntime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	rt := &AttachmentRuntime{Config: cfg, Stores: map[string]blobstore.Store{}}
	fs := blobstore.NewFilesystem(cfg.FilesystemDir)
	if err := fs.Health(ctx); err != nil {
		return nil, fmt.Errorf("attachments filesystem health: %w", err)
	}
	rt.Stores["filesystem"] = fs
	if cfg.Backend == "s3" {
		s3Store, err := blobstore.NewS3(ctx, cfg.S3)
		if err != nil {
			return nil, err
		}
		if err := s3Store.Health(ctx); err != nil {
			return nil, fmt.Errorf("attachments s3 health: %w", err)
		}
		rt.Stores["s3"] = s3Store
	}
	return rt, nil
}
