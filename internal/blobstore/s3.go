package blobstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"git.dajee.net/dajee/xuanchu/internal/attachments"
)

// S3 是基于 AWS SDK 的私有 S3 兼容 BlobStore。
//
// 所有 Put/Open/Delete 都通过服务端 SDK 完成签名，不向调用方暴露
// 预签名 URL。Health 使用 HeadBucket 验证 bucket 可访问。
type S3 struct {
	client  S3Client
	bucket  string
	prefix  string
	sse     string
	kmsKey  string
	hashKey string
}

// S3Client 是 S3 客户端的最小接口，便于在测试中注入 fake。
type S3Client interface {
	PutObject(ctx context.Context, in *awss3.PutObjectInput, opts ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	GetObject(ctx context.Context, in *awss3.GetObjectInput, opts ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, in *awss3.DeleteObjectInput, opts ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error)
	HeadBucket(ctx context.Context, in *awss3.HeadBucketInput, opts ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error)
}

// S3Options 描述构造 S3 store 的非凭证参数。
type S3Options struct {
	Bucket               string
	Prefix               string
	ServerSideEncryption string
	KMSKeyID             string
	HashMetadataKey      string
}

// NewS3WithClient 用于测试和装配：传入已构造的 client。
func NewS3WithClient(client S3Client, opts S3Options) (*S3, error) {
	if client == nil {
		return nil, errors.New("blobstore: s3 client is required")
	}
	if opts.Bucket == "" {
		return nil, errors.New("blobstore: s3 bucket is required")
	}
	prefix := strings.Trim(opts.Prefix, "/")
	hashKey := opts.HashMetadataKey
	if hashKey == "" {
		hashKey = "sha256"
	}
	return &S3{
		client:  client,
		bucket:  opts.Bucket,
		prefix:  prefix,
		sse:     opts.ServerSideEncryption,
		kmsKey:  opts.KMSKeyID,
		hashKey: hashKey,
	}, nil
}

// NewS3 使用 attachments.S3Config 构造生产 S3 store。
//
// 凭证只使用 AWS SDK 默认链路，璇础不读取 access/secret 字段。
func NewS3(ctx context.Context, cfg attachments.S3Config) (*S3, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("blobstore: s3 bucket is required")
	}
	if cfg.Region == "" {
		return nil, errors.New("blobstore: s3 region is required")
	}
	if cfg.Endpoint != "" && strings.HasPrefix(cfg.Endpoint, "http://") && !cfg.AllowInsecureEndpoint {
		return nil, errors.New("blobstore: insecure s3 endpoint requires allow_insecure_endpoint")
	}
	client, err := newS3SDKClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return NewS3WithClient(client, S3Options{
		Bucket:               cfg.Bucket,
		Prefix:               cfg.Prefix,
		ServerSideEncryption: cfg.ServerSideEncryption,
		KMSKeyID:             cfg.KMSKeyID,
	})
}

func (s *S3) objectKey(key string) string {
	if s.prefix == "" {
		return strings.TrimPrefix(key, "/")
	}
	return s.prefix + "/" + strings.TrimPrefix(key, "/")
}

// Put 流式写入对象。
//
// spec 第 9.1 节要求流式，但单个附件 ≤ 25 MiB，且需要把 SHA-256 作为
// 对象 metadata 写入。这里在内存里完成一次哈希计算与 size 验证，整体
// 峰值占用 = 单文件大小，不会把多个上传聚合到内存。
func (s *S3) Put(ctx context.Context, key string, src io.Reader, size int64, mediaType string) error {
	h := sha256.New()
	buf, err := io.ReadAll(io.TeeReader(src, h))
	if err != nil {
		return err
	}
	if size > 0 && int64(len(buf)) != size {
		return errors.New("blobstore: declared size does not match actual bytes")
	}
	input := &awss3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(s.objectKey(key)),
		Body:          bytes.NewReader(buf),
		ContentLength: aws.Int64(int64(len(buf))),
		ContentType:   aws.String(mediaType),
		Metadata:      map[string]string{s.hashKey: hex.EncodeToString(h.Sum(nil))},
	}
	if s.sse != "" {
		input.ServerSideEncryption = types.ServerSideEncryption(s.sse)
	}
	if s.sse == "aws:kms" && s.kmsKey != "" {
		input.SSEKMSKeyId = aws.String(s.kmsKey)
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return err
	}
	return nil
}

// Open 流式读取对象。
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, BlobInfo, error) {
	out, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil {
		return nil, BlobInfo{}, err
	}
	info := BlobInfo{}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		info.MediaType = *out.ContentType
	}
	if out.Metadata != nil {
		if v, ok := out.Metadata[s.hashKey]; ok {
			info.ETag = v
		}
	}
	return out.Body, info, nil
}

// Delete 删除对象。
func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil
		}
		return err
	}
	return nil
}

// Health 用 HeadBucket 验证 bucket 可访问。
func (s *S3) Health(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &awss3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	return err
}

// newS3SDKClient 构造 AWS SDK S3 客户端。
func newS3SDKClient(ctx context.Context, cfg attachments.S3Config) (*awss3.Client, error) {
	loadCtx := ctx
	if loadCtx == nil {
		loadCtx = context.Background()
	}
	opts := []func(*config.LoadOptions) error{config.WithRegion(cfg.Region)}
	if cfg.Endpoint != "" && strings.HasPrefix(cfg.Endpoint, "http://") {
		// 仅本机 MinIO 调试使用，HTTPS endpoint 走 SDK 默认 HTTP client。
		opts = append(opts, config.WithHTTPClient(&http.Client{Transport: &http.Transport{Proxy: nil}}))
	}
	awsCfg, err := config.LoadDefaultConfig(loadCtx, opts...)
	if err != nil {
		return nil, err
	}
	clientOpts := []func(*awss3.Options){}
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *awss3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			if cfg.ForcePathStyle {
				o.UsePathStyle = true
			}
		})
	} else if cfg.ForcePathStyle {
		clientOpts = append(clientOpts, func(o *awss3.Options) {
			o.UsePathStyle = true
		})
	}
	return awss3.NewFromConfig(awsCfg, clientOpts...), nil
}
