// Package attachments 负责通用附件的配置契约。
//
// 这里只描述附件相关的部署参数：存储后端、配额、保留时长、远程图片
// 抓取策略和可选 S3 配置。具体二进制存储由 internal/blobstore 提供，
// 元数据仓储由 internal/storage 提供。
package attachments

import (
	"fmt"
	"path/filepath"
	"time"
)

// Config 描述附件子系统的部署级配置。
//
// 修改字段时请同步更新 internal/config 解析逻辑与 spec 第 10 节。
type Config struct {
	// Backend 决定新上传写入哪个存储。取值 filesystem|s3。
	Backend string
	// FilesystemDir 是本地后端根目录，为空时由调用方填入 <data-dir>/attachments。
	FilesystemDir string

	// 配额声明。校验顺序必须满足 file <= resource <= workspace。
	MaxFileSizeBytes          int64
	MaxResourceTotalSizeBytes int64
	MaxWorkspaceTotalSizeBytes int64
	MaxAttachmentsPerResource int

	// 保留时长。draft_ttl 默认 24h，deleted_retention 默认 720h；0 合法。
	DraftTTL         time.Duration
	DeletedRetention time.Duration

	// 远程图片抓取。可被运维关闭；非 0/非负的合法范围在 Validate 中固定。
	RemoteFetchEnabled        bool
	RemoteFetchTimeout        time.Duration
	RemoteFetchMaxRedirects   int
	RemoteFetchMaxConcurrency int

	S3 S3Config
}

// S3Config 描述可选的 S3 兼容对象存储后端。
//
// 不携带 AWS access/secret 凭证，凭证继续由 SDK 默认链路提供。
type S3Config struct {
	Bucket                string
	Region                string
	Endpoint              string
	Prefix                string
	ForcePathStyle        bool
	AllowInsecureEndpoint bool
	ServerSideEncryption  string
	KMSKeyID              string
}

// DefaultConfig 返回基于 dataDir 的默认附件配置。
//
// filesystem_dir 默认为 <data-dir>/attachments；其它字段按 spec 第 10 节默认值。
func DefaultConfig(dataDir string) Config {
	return Config{
		Backend:                   "filesystem",
		FilesystemDir:              filepath.Join(dataDir, "attachments"),
		MaxFileSizeBytes:           25 << 20,
		MaxResourceTotalSizeBytes:  200 << 20,
		MaxWorkspaceTotalSizeBytes: 10 << 30,
		MaxAttachmentsPerResource:  100,
		DraftTTL:                   24 * time.Hour,
		DeletedRetention:           720 * time.Hour,
		RemoteFetchEnabled:         true,
		RemoteFetchTimeout:         30 * time.Second,
		RemoteFetchMaxRedirects:    5,
		RemoteFetchMaxConcurrency:  4,
		S3: S3Config{
			Prefix: "xuanchu/attachments",
		},
	}
}

// Validate 检查附件配置的部署级不变量。
//
// 配额关系必须满足 file <= resource <= workspace；remote fetch 的时间、
// redirect 与并发上限按 spec 第 10 节固定；S3 只在 backend=s3 时强校验。
func (c Config) Validate() error {
	if c.Backend != "filesystem" && c.Backend != "s3" {
		return fmt.Errorf("attachments.backend must be filesystem or s3")
	}
	if c.MaxFileSizeBytes <= 0 {
		return fmt.Errorf("attachments.max_file_size_mb must be positive")
	}
	if c.MaxResourceTotalSizeBytes < c.MaxFileSizeBytes {
		return fmt.Errorf("attachments.max_resource_total_size_mb must be >= max_file_size_mb")
	}
	if c.MaxWorkspaceTotalSizeBytes < c.MaxResourceTotalSizeBytes {
		return fmt.Errorf("attachments.max_workspace_total_size_mb must be >= max_resource_total_size_mb")
	}
	if c.MaxAttachmentsPerResource <= 0 {
		return fmt.Errorf("attachments.max_attachments_per_resource must be positive")
	}
	if c.DraftTTL < 0 {
		return fmt.Errorf("attachments.draft_ttl must be non-negative")
	}
	if c.DeletedRetention < 0 {
		return fmt.Errorf("attachments.deleted_retention must be non-negative")
	}
	if c.RemoteFetchTimeout <= 0 || c.RemoteFetchTimeout > 120*time.Second {
		return fmt.Errorf("attachments.remote_fetch_timeout must be within (0,120s]")
	}
	if c.RemoteFetchMaxRedirects < 0 || c.RemoteFetchMaxRedirects > 10 {
		return fmt.Errorf("attachments.remote_fetch_max_redirects must be within [0,10]")
	}
	if c.RemoteFetchMaxConcurrency < 1 || c.RemoteFetchMaxConcurrency > 32 {
		return fmt.Errorf("attachments.remote_fetch_max_concurrency must be within [1,32]")
	}
	if err := c.S3.validate(c.Backend == "s3"); err != nil {
		return err
	}
	return nil
}

func (c S3Config) validate(required bool) error {
	if !required {
		// 非主后端时，仍要保证残留配置自洽，避免 KMS/SSE 字段错配。
		if c.ServerSideEncryption == "aws:kms" && c.KMSKeyID == "" {
			return fmt.Errorf("attachments.s3.kms_key_id is required when server_side_encryption is aws:kms")
		}
		if c.ServerSideEncryption != "" && c.ServerSideEncryption != "AES256" && c.ServerSideEncryption != "aws:kms" {
			return fmt.Errorf("attachments.s3.server_side_encryption must be empty, AES256 or aws:kms")
		}
		return nil
	}
	if c.Bucket == "" {
		return fmt.Errorf("attachments.s3.bucket is required when backend is s3")
	}
	if c.Region == "" {
		return fmt.Errorf("attachments.s3.region is required when backend is s3")
	}
	if c.ServerSideEncryption != "" && c.ServerSideEncryption != "AES256" && c.ServerSideEncryption != "aws:kms" {
		return fmt.Errorf("attachments.s3.server_side_encryption must be empty, AES256 or aws:kms")
	}
	if c.ServerSideEncryption == "aws:kms" && c.KMSKeyID == "" {
		return fmt.Errorf("attachments.s3.kms_key_id is required when server_side_encryption is aws:kms")
	}
	if c.ServerSideEncryption != "aws:kms" && c.KMSKeyID != "" {
		return fmt.Errorf("attachments.s3.kms_key_id must only be set when server_side_encryption is aws:kms")
	}
	return nil
}
