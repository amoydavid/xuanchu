// Package blobstore 提供附件二进制内容的流式存储抽象。
//
// 它不感知 task、workspace、用户或权限；只负责把 io.Reader 中的字节
// 写入底层后端，并返回可校验的元信息。具体实现见 filesystem.go 和 s3.go。
package blobstore

import (
	"context"
	"io"
)

// BlobInfo 描述已写入或即将读取的 blob 元数据。
type BlobInfo struct {
	Size      int64
	MediaType string
	// ETag 是基于 SHA-256 的稳定校验值，不假设来自外部存储。
	ETag string
}

// Store 是 BlobStore 的统一接口。
//
// 实现必须流式处理，不得把整个 blob 缓存到内存；调用方传入的 key 由
// 服务端生成，不得包含用户文件名。
type Store interface {
	// Put 把 src 中的字节写入 key。size 是声明长度，用于提前校验；
	// mediaType 是已服务端校验的 MIME，存储后端按需持久化。
	Put(ctx context.Context, key string, src io.Reader, size int64, mediaType string) error
	// Open 打开 key 对应的对象，返回流式 reader 和元数据。调用方必须 Close。
	Open(ctx context.Context, key string) (io.ReadCloser, BlobInfo, error)
	// Delete 删除 key。对象不存在不返回错误。
	Delete(ctx context.Context, key string) error
	// Health 检查后端是否可用。配置错误应使进程启动失败，运行期失败
	// 用于 readiness 判断，不应直接 panic。
	Health(ctx context.Context) error
}
