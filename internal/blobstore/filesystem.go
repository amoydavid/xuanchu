package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Filesystem 是基于本地文件系统的 BlobStore 实现。
//
// 根目录与 workspace 目录使用 0700，blob 文件使用 0600；上传通过同
// 文件系统下的临时文件 + 原子 rename 完成，临时目录固定为 <root>/.tmp。
type Filesystem struct {
	root string
}

// NewFilesystem 构造一个基于 root 的本地存储。
func NewFilesystem(root string) *Filesystem {
	return &Filesystem{root: filepath.Clean(root)}
}

// Put 流式写入 key。
func (s *Filesystem) Put(ctx context.Context, key string, src io.Reader, size int64, mediaType string) error {
	target, err := s.safePath(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.root, ".tmp"), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Join(s.root, ".tmp"), "put-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	hasher := sha256.New()
	written, err := io.Copy(tmp, io.TeeReader(src, hasher))
	if err != nil {
		return err
	}
	if size > 0 && written != size {
		return errors.New("blobstore: declared size does not match actual bytes")
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	cleanup = false
	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		return err
	}
	_ = mediaType // filesystem 不单独存元数据；ETag 由 Open 重算
	return nil
}

// Open 流式读取 key。
func (s *Filesystem) Open(ctx context.Context, key string) (io.ReadCloser, BlobInfo, error) {
	target, err := s.safePath(key)
	if err != nil {
		return nil, BlobInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, BlobInfo{}, err
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, BlobInfo{}, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, BlobInfo{}, err
	}
	return f, BlobInfo{Size: stat.Size()}, nil
}

// Delete 删除 key。
func (s *Filesystem) Delete(ctx context.Context, key string) error {
	target, err := s.safePath(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// Health 通过创建/删除临时文件验证目录可写。
func (s *Filesystem) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.root, ".health-*")
	if err != nil {
		return err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return os.Remove(path)
}

// HashETag 计算 SHA-256 并返回十六进制字符串。
//
// 仅供内部使用，filesystem 不在磁盘上持久化 ETag，由调用方按需缓存。
func HashETag(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// safePath 拒绝路径穿越并把 key 映射到 root 下的绝对路径。
func (s *Filesystem) safePath(key string) (string, error) {
	if key == "" {
		return "", errors.New("blobstore: storage key is required")
	}
	// 先按原始 segment 拒绝 .. 与隐藏文件名，避免依赖后续 Clean 归一化
	// 抹掉这些片段后再判断。
	for _, part := range strings.Split(key, "/") {
		if part == ".." {
			return "", errors.New("blobstore: storage key must not contain parent segment")
		}
		if part == "" || part == "." {
			continue
		}
		if strings.HasPrefix(part, ".") {
			return "", errors.New("blobstore: storage key must not target hidden file")
		}
	}
	cleaned := filepath.Clean("/" + key)
	target := filepath.Join(s.root, cleaned)
	if target == s.root || !strings.HasPrefix(target, s.root+string(filepath.Separator)) {
		return "", errors.New("blobstore: storage key escapes root")
	}
	return target, nil
}
