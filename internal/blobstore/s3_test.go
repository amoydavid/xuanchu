package blobstore

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"git.dajee.net/dajee/xuanchu/internal/attachments"
)

// fakeS3Client 在内存中模拟 S3 Put/Get/Delete/HeadBucket。
type fakeS3Client struct {
	mu      sync.Mutex
	objects map[string]fakeObject
	// 记录最后一次 PutObject 的请求输入，便于断言 SSE/metadata。
	lastPut *awss3.PutObjectInput
	headErr error
}

type fakeObject struct {
	body        []byte
	contentType string
	metadata    map[string]string
}

func newFakeS3Client() *fakeS3Client {
	return &fakeS3Client{objects: map[string]fakeObject{}}
}

func (f *fakeS3Client) PutObject(ctx context.Context, in *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	ct := ""
	if in.ContentType != nil {
		ct = *in.ContentType
	}
	meta := map[string]string{}
	for k, v := range in.Metadata {
		meta[k] = v
	}
	f.objects[*in.Key] = fakeObject{body: body, contentType: ct, metadata: meta}
	cp := *in
	f.lastPut = &cp
	return &awss3.PutObjectOutput{}, nil
}

func (f *fakeS3Client) GetObject(ctx context.Context, in *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	obj, ok := f.objects[*in.Key]
	if !ok {
		return nil, &types.NoSuchKey{Message: aws.String("not found")}
	}
	return &awss3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(obj.body)),
		ContentLength: aws.Int64(int64(len(obj.body))),
		ContentType:   aws.String(obj.contentType),
		Metadata:      obj.metadata,
	}, nil
}

func (f *fakeS3Client) DeleteObject(ctx context.Context, in *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, *in.Key)
	return &awss3.DeleteObjectOutput{}, nil
}

func (f *fakeS3Client) HeadBucket(ctx context.Context, in *awss3.HeadBucketInput, _ ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error) {
	if f.headErr != nil {
		return nil, f.headErr
	}
	return &awss3.HeadBucketOutput{}, nil
}

func TestS3PutOpenDeleteRoundTripAndMetadata(t *testing.T) {
	client := newFakeS3Client()
	store, err := NewS3WithClient(client, S3Options{Bucket: "private", Prefix: "xuanchu/attachments", ServerSideEncryption: "AES256"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	payload := []byte("payload")
	if err := store.Put(ctx, "workspaces/ws/attachments/id", bytes.NewReader(payload), int64(len(payload)), "image/png"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	lastPut := client.lastPut
	if lastPut == nil || lastPut.ServerSideEncryption != types.ServerSideEncryptionAes256 {
		t.Fatalf("SSE = %v", lastPut.ServerSideEncryption)
	}
	if lastPut.Metadata["sha256"] == "" {
		t.Fatal("missing sha256 metadata")
	}
	if lastPut.ContentType == nil || *lastPut.ContentType != "image/png" {
		t.Fatalf("content type = %v", lastPut.ContentType)
	}
	if lastPut.Key == nil || *lastPut.Key != "xuanchu/attachments/workspaces/ws/attachments/id" {
		t.Fatalf("key = %v", lastPut.Key)
	}

	r, info, err := store.Open(ctx, "workspaces/ws/attachments/id")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q", got)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("size = %d", info.Size)
	}
	if info.ETag == "" {
		t.Fatal("ETag missing")
	}
	if err := store.Delete(ctx, "workspaces/ws/attachments/id"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := store.Open(ctx, "workspaces/ws/attachments/id"); err == nil {
		t.Fatal("expected NoSuchKey after delete")
	}
	if err := store.Delete(ctx, "workspaces/ws/attachments/id"); err != nil {
		t.Fatalf("Delete idempotent: %v", err)
	}
}

func TestS3Health(t *testing.T) {
	client := newFakeS3Client()
	store, _ := NewS3WithClient(client, S3Options{Bucket: "b"})
	if err := store.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	client.headErr = &types.NotFound{Message: aws.String("bucket missing")}
	if err := store.Health(context.Background()); err == nil {
		t.Fatal("expected health error")
	}
}

func TestS3SizeMismatch(t *testing.T) {
	store, _ := NewS3WithClient(newFakeS3Client(), S3Options{Bucket: "b"})
	err := store.Put(context.Background(), "k", strings.NewReader("abc"), 5, "image/png")
	if err == nil || !strings.Contains(err.Error(), "declared size") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewS3WithClientRejectsBadConfig(t *testing.T) {
	if _, err := NewS3WithClient(nil, S3Options{Bucket: "b"}); err == nil {
		t.Fatal("expected nil client error")
	}
	if _, err := NewS3WithClient(newFakeS3Client(), S3Options{}); err == nil {
		t.Fatal("expected missing bucket error")
	}
}

func TestNewS3RejectsInsecureEndpoint(t *testing.T) {
	_, err := NewS3(context.Background(), attachmentsS3Config("http://localhost:9000", false))
	if err == nil || !strings.Contains(err.Error(), "insecure") {
		t.Fatalf("err = %v", err)
	}
	// 显式 allow_insecure_endpoint 才通过构造阶段（client 构造在无凭证环境可能失败，这里只断言没卡在 insecure 检查）。
	_, err = NewS3(context.Background(), attachmentsS3Config("http://localhost:9000", true))
	if err != nil && strings.Contains(err.Error(), "insecure") {
		t.Fatalf("err = %v", err)
	}
}

// attachmentsS3Config 构造仅用于 NewS3 校验的 S3Config；region/bucket 必填。
func attachmentsS3Config(endpoint string, allowInsecure bool) attachments.S3Config {
	return attachments.S3Config{
		Bucket:                "xuanchu",
		Region:                "us-east-1",
		Endpoint:              endpoint,
		AllowInsecureEndpoint: allowInsecure,
	}
}
