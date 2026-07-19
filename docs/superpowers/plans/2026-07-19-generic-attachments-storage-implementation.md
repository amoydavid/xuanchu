# 通用附件基础与存储后端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 workspace 级通用附件元数据、流式 BlobStore、filesystem/S3 后端、安全远程图片转存、task 首个 target handler、HTTP API 与清理运行时。

**Architecture:** `attachments` 保存通用归属和状态，`internal/blobstore` 只处理二进制对象，`internal/safefetch` 只处理受限公网读取，`internal/app` 统一处理 target 权限、配额、状态机和补偿。HTTP 只负责流式协议适配；每条记录按自己的 `storage_backend` 选择 store，因此切换新写入后端不迁移历史对象。

**Tech Stack:** Go 1.25、GORM、`github.com/glebarez/sqlite`、PostgreSQL/pgx、AWS SDK for Go v2、`golang.org/x/image/webp`、`github.com/yuin/goldmark`、Huma/OpenAPI、React 19、TypeScript 6。

## Global Constraints

- SQLite 必须继续使用 `github.com/glebarez/sqlite`，不得引入 `gorm.io/driver/sqlite` 或 `github.com/mattn/go-sqlite3`。
- 所有新增 Go 依赖必须是纯 Go，并通过 `CGO_ENABLED=0 go test ./...` 与 `CGO_ENABLED=0 go build ./cmd/xuanchu`。
- 默认 backend 是 `filesystem`，默认目录是 `<data-dir>/attachments`；S3/MinIO 是可选配置，不公开 bucket，不持久化预签名 URL或凭证。
- 附件归属键固定为 `(workspace_id, attached_to_type, attached_to_id)`；首期只注册 `task`，未知类型返回 `attachment_target_type_unsupported`。
- 二进制不得写入数据库；multipart 上传、远程抓取和下载不得 `io.ReadAll` 整个文件。
- 单文件默认 25 MiB、单资源 200 MiB、单 workspace 10 GiB、每资源最多 100 个未删除附件；图片边长不超过 20,000，总像素不超过 40,000,000。
- `draft_ttl` 默认 `24h`，`deleted_retention` 默认 `720h`；stale `uploading` 的清理阈值固定为 1 小时。
- `draft` 仅创建 actor 可见和绑定；`active` 删除前检查正文引用；`deleted` 在 retention 内仍可鉴权读取且继续计入配额。
- 所有外部用户身份必须使用 `task.UserInfo` / `task.ActorInfo`，不得输出裸用户 UUID。
- HTTP、App、Storage、BlobStore、SafeFetch 分层遵守 spec 第 7 节，不得由 HTTP 直接访问 GORM/S3/远程 URL。

---

## Dependency Contract

本计划产出的公共接口是后三份计划的唯一依赖，后续不得另起同义类型：

```go
// internal/attachments/config.go
type Config struct {
    Backend string
    FilesystemDir string
    MaxFileSizeBytes int64
    MaxResourceTotalSizeBytes int64
    MaxWorkspaceTotalSizeBytes int64
    MaxAttachmentsPerResource int
    DraftTTL time.Duration
    DeletedRetention time.Duration
    RemoteFetchEnabled bool
    RemoteFetchTimeout time.Duration
    RemoteFetchMaxRedirects int
    RemoteFetchMaxConcurrency int
    S3 S3Config
}

// internal/blobstore/store.go
type Store interface {
    Put(context.Context, string, io.Reader, int64, string) error
    Open(context.Context, string) (io.ReadCloser, BlobInfo, error)
    Delete(context.Context, string) error
    Health(context.Context) error
}

// internal/app/attachment.go
type AttachmentTarget struct { Type, ID, WorkspaceID string }
type AttachmentTargetView struct { Type string `json:"type"`; ID string `json:"id"` }
type AttachmentUploadInput struct {
    Reader io.Reader
    DeclaredSize int64
    OriginalName string
    DisplayName string
    Mode string
}
type AttachmentImportURLInput struct { SourceURL, DisplayName, Mode string }
type AttachmentView struct {
    ID string
    AttachedTo AttachmentTargetView
    State, OriginalName, DisplayName, MediaType string
    SizeBytes int64
    SHA256 string
    InlineCapable bool
    SourceType, ContentURL string
    CreatedBy task.ActorInfo
    CreatedAt, ModifiedAt int64
}
type AttachmentContent struct {
    View AttachmentView
    Reader io.ReadCloser
    Blob blobstore.BlobInfo
}
type AttachmentRuntime struct {
    Config attachments.Config
    Stores map[string]blobstore.Store
    Fetcher *safefetch.Fetcher
}
```

---

### Task 1: 固定配置、依赖和运行时装配契约

**Files:**
- Create: `internal/attachments/config.go`
- Create: `internal/attachments/config_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `config.Options.DataDir`、现有扁平 TOML map 和环境变量覆盖规则。
- Produces: `attachments.Config`、`attachments.S3Config`、`config.Config.Attachments`、`config.Config.DataDir`。

- [ ] **Step 1: 写失败测试，锁定默认值、TOML、环境变量和非法组合**

```go
func TestResolveAttachmentDefaults(t *testing.T) {
    cfg, err := Resolve(Options{HomeDir: "/home/alice", Env: map[string]string{}})
    if err != nil { t.Fatal(err) }
    if cfg.DataDir != "/home/alice/.local/share/xuanchu" { t.Fatalf("data dir = %q", cfg.DataDir) }
    if cfg.Attachments.Backend != "filesystem" { t.Fatalf("backend = %q", cfg.Attachments.Backend) }
    if cfg.Attachments.FilesystemDir != "/home/alice/.local/share/xuanchu/attachments" { t.Fatalf("dir = %q", cfg.Attachments.FilesystemDir) }
    if cfg.Attachments.MaxFileSizeBytes != 25<<20 || cfg.Attachments.DraftTTL != 24*time.Hour { t.Fatalf("defaults = %#v", cfg.Attachments) }
}

func TestAttachmentConfigRejectsInvalidS3AndQuota(t *testing.T) {
    cfg := attachments.DefaultConfig("/data")
    cfg.Backend = "s3"
    if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "bucket") { t.Fatalf("err = %v", err) }
    cfg = attachments.DefaultConfig("/data")
    cfg.MaxResourceTotalSizeBytes = cfg.MaxFileSizeBytes - 1
    if err := cfg.Validate(); err == nil { t.Fatal("want quota validation error") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config ./internal/attachments -run 'TestResolveAttachment|TestAttachmentConfig' -count=1`

Expected: FAIL，`internal/attachments` 与 `Config.Attachments` 尚不存在。

- [ ] **Step 3: 实现配置结构和严格校验**

```go
type S3Config struct {
    Bucket, Region, Endpoint, Prefix string
    ForcePathStyle, AllowInsecureEndpoint bool
    ServerSideEncryption, KMSKeyID string
}

func DefaultConfig(dataDir string) Config {
    return Config{
        Backend: "filesystem", FilesystemDir: filepath.Join(dataDir, "attachments"),
        MaxFileSizeBytes: 25 << 20, MaxResourceTotalSizeBytes: 200 << 20,
        MaxWorkspaceTotalSizeBytes: 10 << 30, MaxAttachmentsPerResource: 100,
        DraftTTL: 24 * time.Hour, DeletedRetention: 720 * time.Hour,
        RemoteFetchEnabled: true, RemoteFetchTimeout: 30 * time.Second,
        RemoteFetchMaxRedirects: 5, RemoteFetchMaxConcurrency: 4,
        S3: S3Config{Prefix: "xuanchu/attachments"},
    }
}

func (c Config) Validate() error {
    if c.Backend != "filesystem" && c.Backend != "s3" { return fmt.Errorf("attachments.backend must be filesystem or s3") }
    if c.MaxFileSizeBytes <= 0 || c.MaxResourceTotalSizeBytes < c.MaxFileSizeBytes || c.MaxWorkspaceTotalSizeBytes < c.MaxResourceTotalSizeBytes { return fmt.Errorf("attachments quota order must be file <= resource <= workspace") }
    if c.MaxAttachmentsPerResource <= 0 { return fmt.Errorf("attachments.max_attachments_per_resource must be positive") }
    if c.DraftTTL < 0 || c.DeletedRetention < 0 { return fmt.Errorf("attachments retention durations must be non-negative") }
    if c.RemoteFetchTimeout <= 0 || c.RemoteFetchTimeout > 120*time.Second { return fmt.Errorf("attachments.remote_fetch_timeout must be within (0,120s]") }
    if c.RemoteFetchMaxRedirects < 0 || c.RemoteFetchMaxRedirects > 10 || c.RemoteFetchMaxConcurrency < 1 || c.RemoteFetchMaxConcurrency > 32 { return fmt.Errorf("attachments remote fetch limits are invalid") }
    return c.S3.validate(c.Backend == "s3")
}
```

`Resolve` 必须依次应用以下 TOML/env 映射，MiB 值用 `int64(n) << 20` 转换，并调用 `Validate()`；AWS access key/secret 不加入结构体：

```text
attachments.backend                         XUANCHU_ATTACHMENTS_BACKEND
attachments.filesystem_dir                  XUANCHU_ATTACHMENTS_FILESYSTEM_DIR
attachments.max_file_size_mb                XUANCHU_ATTACHMENTS_MAX_FILE_SIZE_MB
attachments.max_resource_total_size_mb      XUANCHU_ATTACHMENTS_MAX_RESOURCE_TOTAL_SIZE_MB
attachments.max_workspace_total_size_mb     XUANCHU_ATTACHMENTS_MAX_WORKSPACE_TOTAL_SIZE_MB
attachments.max_attachments_per_resource    XUANCHU_ATTACHMENTS_MAX_ATTACHMENTS_PER_RESOURCE
attachments.draft_ttl                       XUANCHU_ATTACHMENTS_DRAFT_TTL
attachments.deleted_retention               XUANCHU_ATTACHMENTS_DELETED_RETENTION
attachments.remote_fetch_enabled            XUANCHU_ATTACHMENTS_REMOTE_FETCH_ENABLED
attachments.remote_fetch_timeout            XUANCHU_ATTACHMENTS_REMOTE_FETCH_TIMEOUT
attachments.remote_fetch_max_redirects      XUANCHU_ATTACHMENTS_REMOTE_FETCH_MAX_REDIRECTS
attachments.remote_fetch_max_concurrency    XUANCHU_ATTACHMENTS_REMOTE_FETCH_MAX_CONCURRENCY
attachments.s3.bucket                       XUANCHU_ATTACHMENTS_S3_BUCKET
attachments.s3.region                       XUANCHU_ATTACHMENTS_S3_REGION
attachments.s3.endpoint                     XUANCHU_ATTACHMENTS_S3_ENDPOINT
attachments.s3.prefix                       XUANCHU_ATTACHMENTS_S3_PREFIX
attachments.s3.force_path_style             XUANCHU_ATTACHMENTS_S3_FORCE_PATH_STYLE
attachments.s3.allow_insecure_endpoint      XUANCHU_ATTACHMENTS_S3_ALLOW_INSECURE_ENDPOINT
attachments.s3.server_side_encryption       XUANCHU_ATTACHMENTS_S3_SERVER_SIDE_ENCRYPTION
attachments.s3.kms_key_id                   XUANCHU_ATTACHMENTS_S3_KMS_KEY_ID
```

- [ ] **Step 4: 安装纯 Go 依赖并运行测试**

Run:

```bash
go get github.com/aws/aws-sdk-go-v2/config github.com/aws/aws-sdk-go-v2/service/s3 github.com/yuin/goldmark golang.org/x/image/webp
go mod tidy
go test ./internal/config ./internal/attachments -count=1
```

Expected: PASS，`go.mod` 不出现 CGO SQLite driver。

- [ ] **Step 5: 提交**

```bash
git add go.mod go.sum internal/attachments/config.go internal/attachments/config_test.go internal/config/config.go internal/config/config_test.go
git commit -m "feat: 增加附件配置契约"
```

### Task 2: 抽取共享公网地址防护

**Files:**
- Create: `internal/netguard/public_target.go`
- Create: `internal/netguard/public_target_test.go`
- Modify: `internal/app/hook_endpoint.go`
- Modify: `internal/app/hook_test.go`
- Modify: `internal/hookruntime/dispatcher.go`
- Modify: `internal/notificationruntime/dispatcher.go`

**Interfaces:**
- Consumes: 现有 `app.HookHostResolver` 和 `app.IsBlockedWebhookIP` 行为。
- Produces: `netguard.Resolver`、`netguard.ResolvePublic(ctx, resolver, host) ([]net.IPAddr, error)`、`netguard.IsBlockedIP(net.IP) bool`。

- [ ] **Step 1: 写失败测试覆盖全部禁止地址和混合 DNS**

```go
func TestResolvePublicRejectsAnyBlockedAddress(t *testing.T) {
    resolver := stubResolver{addrs: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}}
    if _, err := ResolvePublic(context.Background(), resolver, "example.com"); err == nil { t.Fatal("mixed public/private DNS must fail") }
}

func TestIsBlockedIP(t *testing.T) {
    for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "100.64.0.1", "224.0.0.1", "::1", "fc00::1", "fe80::1"} {
        if !IsBlockedIP(net.ParseIP(raw)) { t.Fatalf("%s must be blocked", raw) }
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/netguard ./internal/app ./internal/hookruntime ./internal/notificationruntime -run 'Public|SSRF|BlockedIP' -count=1`

Expected: FAIL，`internal/netguard` 尚不存在。

- [ ] **Step 3: 抽取实现并让既有 runtime 复用**

```go
type Resolver interface { LookupIPAddr(context.Context, string) ([]net.IPAddr, error) }

func ResolvePublic(ctx context.Context, resolver Resolver, host string) ([]net.IPAddr, error) {
    addrs, err := resolver.LookupIPAddr(ctx, host)
    if err != nil { return nil, err }
    if len(addrs) == 0 { return nil, errors.New("host resolved to no addresses") }
    for _, addr := range addrs {
        if IsBlockedIP(addr.IP) { return nil, errors.New("host resolves to blocked address") }
    }
    return addrs, nil
}
```

`app.HookHostResolver` 改为 type alias：`type HookHostResolver = netguard.Resolver`；Hook 和 Notification dispatcher 的 `DialContext` 必须调用 `ResolvePublic` 后只连接本次返回的 IP，保留原 hostname 作为 HTTP Host/TLS SNI，并禁止环境代理。

- [ ] **Step 4: 运行回归**

Run: `go test ./internal/netguard ./internal/app ./internal/hookruntime ./internal/notificationruntime -count=1`

Expected: PASS，既有 webhook/notification SSRF 测试不变。

- [ ] **Step 5: 提交**

```bash
git add internal/netguard internal/app/hook_endpoint.go internal/app/hook_test.go internal/hookruntime/dispatcher.go internal/notificationruntime/dispatcher.go
git commit -m "refactor: 统一出站公网地址防护"
```

### Task 3: 实现 BlobStore 与 filesystem 后端

**Files:**
- Create: `internal/blobstore/store.go`
- Create: `internal/blobstore/filesystem.go`
- Create: `internal/blobstore/filesystem_test.go`

**Interfaces:**
- Consumes: server 生成的相对 storage key。
- Produces: `blobstore.Store`、`blobstore.BlobInfo{Size int64; MediaType, ETag string}`、`blobstore.NewFilesystem(root string)`。

- [ ] **Step 1: 写失败测试锁定权限、原子写、流式读取和 key 防穿越**

```go
func TestFilesystemRoundTripAndPermissions(t *testing.T) {
    root := t.TempDir()
    store := NewFilesystem(root)
    key := "workspaces/ws/attachments/id"
    if err := store.Put(context.Background(), key, strings.NewReader("abc"), 3, "text/plain"); err != nil { t.Fatal(err) }
    r, info, err := store.Open(context.Background(), key)
    if err != nil { t.Fatal(err) }
    defer r.Close()
    got, _ := io.ReadAll(r)
    if string(got) != "abc" || info.Size != 3 { t.Fatalf("got %q %#v", got, info) }
    stat, _ := os.Stat(filepath.Join(root, filepath.FromSlash(key)))
    if stat.Mode().Perm() != 0o600 { t.Fatalf("mode = %o", stat.Mode().Perm()) }
}

func TestFilesystemRejectsTraversal(t *testing.T) {
    if err := NewFilesystem(t.TempDir()).Put(context.Background(), "../escape", strings.NewReader("x"), 1, "text/plain"); err == nil { t.Fatal("want traversal error") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/blobstore -run Filesystem -count=1`

Expected: FAIL，package 尚不存在。

- [ ] **Step 3: 实现接口和原子文件写入**

```go
type BlobInfo struct { Size int64; MediaType, ETag string }
type Store interface {
    Put(context.Context, string, io.Reader, int64, string) error
    Open(context.Context, string) (io.ReadCloser, BlobInfo, error)
    Delete(context.Context, string) error
    Health(context.Context) error
}
```

`Put` 必须在 `<root>/.tmp/<uuid>` 以 `0600` 创建，`io.CopyN`/EOF 验证实际长度，`Sync` 后 `Rename`；目录 `0700`。`safePath` 对 `filepath.Clean`、绝对路径、`..` segment 做拒绝。`Health` 创建并删除 `.health-*` 文件。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/blobstore -run Filesystem -count=1`

Expected: PASS，并确认失败写入不留下目标文件。

- [ ] **Step 5: 提交**

```bash
git add internal/blobstore/store.go internal/blobstore/filesystem.go internal/blobstore/filesystem_test.go
git commit -m "feat: 实现本地附件对象存储"
```

### Task 4: 实现 private S3 后端与 backend registry

**Files:**
- Create: `internal/blobstore/s3.go`
- Create: `internal/blobstore/s3_test.go`
- Create: `internal/app/attachment_runtime.go`
- Create: `internal/app/attachment_runtime_test.go`

**Interfaces:**
- Consumes: `attachments.Config`、AWS 默认 credential chain。
- Produces: `blobstore.NewS3(ctx, attachments.S3Config)`、`app.NewAttachmentRuntime(ctx, cfg, logger)`。

- [ ] **Step 1: 写 fake S3 失败测试**

```go
func TestS3PutOpenDeleteUsesPrivateObjectAndSSE(t *testing.T) {
    fake := newFakeS3Server(t)
    store, err := newS3WithClient(fake.client(), S3Options{Bucket: "private", Prefix: "xuanchu/attachments", SSE: "AES256"})
    if err != nil { t.Fatal(err) }
    if err := store.Put(context.Background(), "workspaces/ws/attachments/id", strings.NewReader("abc"), 3, "image/png"); err != nil { t.Fatal(err) }
    req := fake.lastPut()
    if req.Header.Get("x-amz-server-side-encryption") != "AES256" { t.Fatalf("SSE = %q", req.Header.Get("x-amz-server-side-encryption")) }
    if req.URL.Query().Get("X-Amz-Signature") != "" { t.Fatal("presigned URL must not be produced") }
}

func TestAttachmentRuntimeReadsByRowBackend(t *testing.T) {
    rt := &AttachmentRuntime{Stores: map[string]blobstore.Store{"filesystem": fs, "s3": s3}}
    if rt.StoreFor("s3") != s3 || rt.StoreFor("filesystem") != fs { t.Fatal("backend registry mismatch") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/blobstore ./internal/app -run 'S3|AttachmentRuntime' -count=1`

Expected: FAIL，S3 store/runtime 尚不存在。

- [ ] **Step 3: 实现 SDK client 和 fail-fast health**

```go
type AttachmentRuntime struct {
    Config attachments.Config
    Stores map[string]blobstore.Store
}

func (r *AttachmentRuntime) StoreFor(name string) (blobstore.Store, error) {
    store, ok := r.Stores[name]
    if !ok { return nil, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachment storage backend is unavailable"} }
    return store, nil
}
```

S3 `PutObject` 设置已验证 Content-Type、`x-amz-meta-sha256` 和 SSE；`Open` 使用 `GetObject`，`Delete` 使用 `DeleteObject`，`Health` 使用 `HeadBucket`。自定义 HTTP endpoint 只有 `allow_insecure_endpoint=true` 时接受 `http`，KMS 模式必须传 key ID。`NewAttachmentRuntime` 总是注册 filesystem；S3 配置存在时同时注册 S3，对所有已配置 store 执行 `Health`。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/blobstore ./internal/app -run 'S3|AttachmentRuntime' -count=1`

Expected: PASS；测试仅访问 fake server，不访问公网 AWS。

- [ ] **Step 5: 提交**

```bash
git add internal/blobstore/s3.go internal/blobstore/s3_test.go internal/app/attachment_runtime.go internal/app/attachment_runtime_test.go
git commit -m "feat: 增加可选 S3 附件后端"
```

### Task 5: 建立附件模型、迁移与 repository

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/migrate_sqlite.go`
- Modify: `internal/storage/migrate_postgres.go`
- Modify: `internal/storage/db_test.go`
- Create: `internal/storage/attachment_repo.go`
- Create: `internal/storage/attachment_repo_test.go`

**Interfaces:**
- Consumes: GORM transaction 与两种 dialect。
- Produces: `storage.Attachment`、`AttachmentRepository` 的 Create/Get/List/Update/Quota/Janitor 查询。

- [ ] **Step 1: 写迁移和 repository 失败测试**

```go
func TestAttachmentMigrationCreatesGenericTargetIndex(t *testing.T) {
    store := openTestStore(t)
    if !store.DB().Migrator().HasTable(&Attachment{}) { t.Fatal("attachments table missing") }
    if !store.DB().Migrator().HasIndex(&Attachment{}, "idx_attachments_target") { t.Fatal("target index missing") }
}

func TestAttachmentRepositoryScopesByWorkspaceAndTarget(t *testing.T) {
    repo := NewAttachmentRepository(store.DB())
    mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws-a", AttachedToType: "task", AttachedToID: "t-1", State: AttachmentStateActive})
    mustCreateAttachment(t, repo, Attachment{ID: "b", WorkspaceID: "ws-b", AttachedToType: "task", AttachedToID: "t-1", State: AttachmentStateActive})
    got, err := repo.List(AttachmentListOptions{WorkspaceID: "ws-a", AttachedToType: "task", AttachedToID: "t-1", States: []string{AttachmentStateActive}})
    if err != nil || len(got) != 1 || got[0].ID != "a" { t.Fatalf("got %#v err=%v", got, err) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/storage -run Attachment -count=1`

Expected: FAIL，model/repository 尚不存在。

- [ ] **Step 3: 实现 spec 第 8 节完整 model 和原子配额提交方法**

```go
type AttachmentRepository struct { db *gorm.DB }
func (r *AttachmentRepository) Create(row Attachment) error
func (r *AttachmentRepository) GetByID(id string) (Attachment, error)
func (r *AttachmentRepository) List(opts AttachmentListOptions) ([]Attachment, error)
func (r *AttachmentRepository) FinalizeWithQuota(id string, final AttachmentFinalize, limits AttachmentQuotaLimits) error
func (r *AttachmentRepository) MarkDeleted(id string, now, purgeAfter int64) error
func (r *AttachmentRepository) ListJanitorCandidates(now int64, limit int) ([]Attachment, error)
func (r *AttachmentRepository) DeleteRow(id string) error
```

`FinalizeWithQuota` 在同一 DB transaction 内重查资源数量/字节和 workspace 字节，SQLite 通过立即写锁/GORM transaction 串行化最终提交，PostgreSQL 对 workspace 相关行执行 `FOR UPDATE`；所有统计包含 uploading/draft/active/deleted-retention，数量上限只算 uploading/draft/active。

model 必须完整包含 spec 第 8 节字段，尤其 `EverEmbedded`、`SourceType`、`SourceHost`、`SourceURLHash`、`StorageBackend`、`StorageKey`、actor columns、`ExpiresAt/DeletedAt/PurgeAfter`；远程 URL 只保存规范化 host 和 URL SHA-256，不保存完整 URL。`EverEmbedded` 激活后只允许 false→true。

- [ ] **Step 4: 运行 SQLite 测试与 PostgreSQL opt-in contract**

Run:

```bash
go test ./internal/storage -run Attachment -count=1
XUANCHU_TEST_POSTGRES_URL="$XUANCHU_TEST_POSTGRES_URL" go test ./internal/storage -run 'Attachment.*Postgres' -count=1
```

Expected: 第一条 PASS；未配置 PostgreSQL 时第二条 SKIP，配置时 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/storage/models.go internal/storage/migrate_sqlite.go internal/storage/migrate_postgres.go internal/storage/db_test.go internal/storage/attachment_repo.go internal/storage/attachment_repo_test.go
git commit -m "feat: 增加通用附件元数据仓储"
```

### Task 6: 实现流式暂存、类型识别和文件安全策略

**Files:**
- Create: `internal/attachments/inspect.go`
- Create: `internal/attachments/inspect_test.go`
- Create: `internal/attachments/ooxml.go`
- Create: `internal/attachments/ooxml_test.go`

**Interfaces:**
- Consumes: 文件 reader、原始文件名、声明大小和 `Config.MaxFileSizeBytes`。
- Produces: `attachments.InspectToTemp(...) (InspectedFile, cleanup, error)` 与稳定 `attachments.Error{Code}`。

- [ ] **Step 1: 写失败测试覆盖 allowlist、magic、像素和 OOXML 宏**

```go
func TestInspectRejectsExecutableAndMIMEConflict(t *testing.T) {
    _, _, err := InspectToTemp(context.Background(), strings.NewReader("MZ..."), "evil.exe", 5, 25<<20)
    assertAttachmentCode(t, err, "attachment_type_not_allowed")
}

func TestInspectRejectsMacroEnabledOOXML(t *testing.T) {
    data := makeOOXML(t, "word/document.xml", "word/vbaProject.bin")
    _, _, err := InspectToTemp(context.Background(), bytes.NewReader(data), "doc.docx", int64(len(data)), 25<<20)
    assertAttachmentCode(t, err, "attachment_type_not_allowed")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/attachments -run Inspect -count=1`

Expected: FAIL，inspect API 尚不存在。

- [ ] **Step 3: 实现 0600 temp spool 和完整 allowlist**

```go
type InspectedFile struct {
    Path, Extension, MediaType, SHA256 string
    SizeBytes int64
    InlineCapable bool
}

func InspectToTemp(ctx context.Context, src io.Reader, originalName string, declaredSize, max int64) (InspectedFile, func(), error)
```

实现必须用 `io.LimitedReader{N:max+1}`、流式 SHA-256、`http.DetectContentType`、`image.DecodeConfig`，并固定以下 allow/deny 表：

```text
inline: .png image/png; .jpg/.jpeg image/jpeg; .gif image/gif; .webp image/webp
download: .pdf .txt .md .csv .json .docx .xlsx .pptx .zip .7z .tar .gz .tgz
deny: .html .htm .xhtml .svg .xml .js .mjs .cjs .wasm .sh .bash .zsh .fish
      .bat .cmd .ps1 .vbs .exe .dll .msi .com .scr .apk .app .dmg .pkg .jar
      .docm .xlsm .pptm
```

OOXML 只打开 ZIP central directory，检查 `[Content_Types].xml`、对应 `word/|xl/|ppt/` 根目录和任何 `vbaProject.bin`，不解压普通压缩包。ZIP/7z/tar/gzip 只校验容器 header。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/attachments -run 'Inspect|OOXML' -count=1`

Expected: PASS，超限返回 `attachment_too_large`，图片炸弹返回 `attachment_image_invalid`。

- [ ] **Step 5: 提交**

```bash
git add internal/attachments/inspect.go internal/attachments/inspect_test.go internal/attachments/ooxml.go internal/attachments/ooxml_test.go
git commit -m "feat: 校验附件文件类型和大小"
```

### Task 7: 实现安全远程图片抓取

**Files:**
- Create: `internal/safefetch/fetcher.go`
- Create: `internal/safefetch/fetcher_test.go`
- Modify: `internal/app/attachment_runtime.go`
- Modify: `internal/app/attachment_runtime_test.go`

**Interfaces:**
- Consumes: `netguard.Resolver`、`attachments.Config`。
- Produces: `safefetch.New(Config)`、`Fetcher.Fetch(ctx, rawURL) (Result, error)`，Result 暴露受限 reader、最终 URL、source host 和安全候选文件名。

- [ ] **Step 1: 写失败测试覆盖 redirect、DNS rebinding、超长响应和 HTTPS 降级**

```go
func TestFetcherPinsValidatedIPAndRevalidatesRedirect(t *testing.T) {
    resolver := &changingResolver{answers: [][]net.IPAddr{{{IP: publicIP}}, {{IP: net.ParseIP("127.0.0.1")}}}}
    fetcher := newTestFetcher(resolver, transportToFixture)
    _, err := fetcher.Fetch(context.Background(), "https://img.example/a.png")
    assertCode(t, err, "attachment_remote_url_invalid")
}

func TestFetcherRejectsHTTPSDowngradeAndOversize(t *testing.T) {
    // fixture 返回 https -> http redirect，以及 Content-Length=max+1 两个 case。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/safefetch -count=1`

Expected: FAIL，package 尚不存在。

- [ ] **Step 3: 实现无代理、无凭证、逐跳校验的 client**

```go
type Config struct { Timeout time.Duration; MaxRedirects int; MaxBytes int64; Resolver netguard.Resolver }
type Result struct { Body io.ReadCloser; ContentLength int64; SourceHost, FileName string }
func (f *Fetcher) Fetch(ctx context.Context, raw string) (Result, error)
```

只允许 http:80/https:443；拒绝 userinfo、Cookie、Authorization、Referer；`Transport.Proxy=nil`；每跳 `ResolvePublic`，自定义 `DialContext` 只连已验证 IP；HTTPS→HTTP 拒绝；仅接受 2xx；body 包装为 max+1 的 reader 并在调用方 spool 时二次判断。

同时把 `Fetcher *safefetch.Fetcher` 加入 `AttachmentRuntime`，由 `NewAttachmentRuntime` 按 attachment config 构造；remote fetch disabled 时仍保留 nil/disabled 明确状态，App 返回 `attachment_remote_fetch_disabled`，不能回退浏览器直连。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/safefetch -count=1`

Expected: PASS，测试覆盖 loopback、RFC1918、link-local、CGNAT、保留/混合地址、redirect 每跳重验、timeout 和 cancellation。

- [ ] **Step 5: 提交**

```bash
git add internal/safefetch/fetcher.go internal/safefetch/fetcher_test.go internal/app/attachment_runtime.go internal/app/attachment_runtime_test.go
git commit -m "feat: 安全抓取公网附件图片"
```

### Task 8: 实现 task target handler 与附件 App 状态机

**Files:**
- Create: `internal/task/content_reference.go`
- Create: `internal/task/content_reference_test.go`
- Create: `internal/app/attachment.go`
- Create: `internal/app/attachment_test.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/audit.go`

**Interfaces:**
- Consumes: Tasks 3–7 的 runtime/store/repository/inspection/fetcher。
- Produces: Dependency Contract 中全部 App 类型；`ServiceOptions.Attachments *AttachmentRuntime` 和附件 CRUD/open/cleanup 方法。

- [ ] **Step 1: 写失败测试覆盖 target、actor、状态、配额与补偿**

```go
func TestAttachmentUploadDraftAndBindRules(t *testing.T) {
    svc := newAttachmentService(t, actor("alice"))
    draft, err := svc.UploadAttachment(ctx, "task", taskID, AttachmentUploadInput{Reader: pngReader(), DeclaredSize: pngSize, OriginalName: "a.png", Mode: "description_draft"})
    if err != nil || draft.State != "draft" { t.Fatalf("draft=%#v err=%v", draft, err) }
    other := newAttachmentServiceFromFixture(t, actor("bob"))
    err = other.ActivateDescriptionDrafts(taskID, []string{draft.ID})
    assertRuntimeCode(t, err, "attachment_draft_creator_mismatch")
}

func TestUnknownAttachmentTargetAndInUseDelete(t *testing.T) {
    _, err := svc.ListAttachments("project", projectID, false)
    assertRuntimeCode(t, err, "attachment_target_type_unsupported")
    err = svc.RemoveAttachment(ctx, embeddedAttachmentID)
    assertRuntimeCode(t, err, "attachment_in_use")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/task ./internal/app -run 'ContentReference|Attachment' -count=1`

Expected: FAIL，App API 尚不存在。

- [ ] **Step 3: 用 Goldmark 建立 attachment-only AST parser scaffold**

```go
type ContentReferenceKind string
const (
    ContentReferenceUser ContentReferenceKind = "user"
    ContentReferenceTask ContentReferenceKind = "task"
    ContentReferenceAttachment ContentReferenceKind = "attachment"
)
type ContentReference struct { Kind ContentReferenceKind; ID, Label string; Image bool }
func ParseContentReferences(markdown string) ([]ContentReference, error)
func AttachmentReferenceIDs(markdown string) ([]string, error)
```

本任务只接受并返回 `ref://attachment/{canonical-lowercase-uuid}`；普通链接忽略，非法保留 URI返回 `description_reference_invalid`。第四份计划在同一文件扩展 user/task，不能新建第二套 parser。

- [ ] **Step 4: 实现 App 方法和 task handler registry**

```go
func (s *Service) UploadAttachment(ctx context.Context, targetType, targetRef string, in AttachmentUploadInput) (AttachmentView, error)
func (s *Service) ImportAttachmentURL(ctx context.Context, targetType, targetRef string, in AttachmentImportURLInput) (AttachmentView, error)
func (s *Service) ListAttachments(targetType, targetRef string, includeDrafts bool) ([]AttachmentView, error)
func (s *Service) GetAttachment(id string) (AttachmentView, error)
func (s *Service) OpenAttachmentContent(ctx context.Context, id string) (AttachmentContent, error)
func (s *Service) RenameAttachment(id, displayName string) (AttachmentView, error)
func (s *Service) RemoveAttachment(ctx context.Context, id string) error
func (s *Service) ActivateDescriptionDrafts(taskID string, ids []string) error
func (s *Service) CleanupAttachments(ctx context.Context, limit int) (AttachmentCleanupResult, error)
```

`resolveAttachmentTarget("task", ref, read|write)` 必须调用 `resolveTargetForRead/Write`、验证 workspace/scope/closed project；storage key 固定为 `workspaces/{workspaceID}/attachments/{attachmentID}`；上传使用 uploading→draft/active，任何失败删除 blob/row或留给 janitor；远程导入必须复用同一 `completeUpload`，且只接受 inline image。

- [ ] **Step 5: 运行 App 测试**

Run: `go test ./internal/task ./internal/app -run 'ContentReference|Attachment' -count=1`

Expected: PASS，包括 viewer/closed project/cross-workspace/project allowlist、并发配额、backend 切换、删除 in-use、draft actor 和补偿路径。

- [ ] **Step 6: 提交**

```bash
git add internal/task/content_reference.go internal/task/content_reference_test.go internal/app/attachment.go internal/app/attachment_test.go internal/app/runtime.go internal/app/service.go internal/app/audit.go
git commit -m "feat: 实现通用附件生命周期"
```

### Task 9: 暴露附件 HTTP/OpenAPI 协议

**Files:**
- Create: `internal/httpapi/attachments.go`
- Create: `internal/httpapi/attachments_test.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/app_service.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/middleware.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/server_test.go`

**Interfaces:**
- Consumes: `Options.Attachments *app.AttachmentRuntime` 和 Task 8 App API。
- Produces: spec 第 13.1 节七个 endpoint、multipart/OpenAPI contract 和安全 content response。

**Stable error mapping:**

```text
attachment_not_found                 404
attachment_upload_incomplete         400
attachment_remote_fetch_disabled     403
attachment_remote_url_invalid        422
attachment_remote_fetch_failed       502
attachment_too_large                 413
attachment_type_not_allowed          415
attachment_image_invalid             422
attachment_target_type_unsupported   422
attachment_quota_exceeded            409
attachment_in_use                    409
attachment_draft_creator_mismatch    403
attachment_state_invalid             409
attachment_content_gone              410
attachment_storage_unavailable       503
```

**Routes:**

```text
POST   /api/v1/tasks/{taskRef}/attachments
POST   /api/v1/tasks/{taskRef}/attachments/import-url
GET    /api/v1/tasks/{taskRef}/attachments
GET    /api/v1/attachments/{attachmentID}
GET    /api/v1/attachments/{attachmentID}/content
PATCH  /api/v1/attachments/{attachmentID}
DELETE /api/v1/attachments/{attachmentID}
```

multipart 只接受 `file`、`mode=attachment|description_draft`、可选 `display_name`；import-url JSON 只接受 `source_url`、`mode=description_draft`、可选 `display_name`；PATCH 只接受 `display_name`，任何 `attached_to_*` 字段都拒绝。

- [ ] **Step 1: 写失败 HTTP contract 测试**

```go
func TestAttachmentMultipartAndContentRoundTrip(t *testing.T) {
    body, contentType := multipartFixture(t, "file", "diagram.png", pngBytes, map[string]string{"mode": "attachment"})
    upload := requestHTTPBodyBytes(t, fixture.server, http.MethodPost, "/api/v1/tasks/"+taskID+"/attachments?workspace=local", body, headers(fixture.token, contentType))
    if upload.Code != http.StatusCreated { t.Fatalf("upload=%d %s", upload.Code, upload.Body.String()) }
    content := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/attachments/"+id+"/content?workspace=local", auth(fixture.token))
    if content.Header().Get("X-Content-Type-Options") != "nosniff" || content.Header().Get("Cache-Control") != "private, no-store" { t.Fatalf("headers=%v", content.Header()) }
    if !bytes.Equal(content.Body.Bytes(), pngBytes) { t.Fatal("content mismatch") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/httpapi -run Attachment -count=1`

Expected: FAIL，routes 尚未注册。

- [ ] **Step 3: 实现 handler、错误映射和 route-specific body limit**

上传路由必须跳过现有 10 MiB 全局 `bodyLimitMiddleware`，handler 自己使用：

```go
r.Body = http.MaxBytesReader(w, r.Body, s.attachments.Config.MaxFileSizeBytes+(1<<20))
file, header, err := r.FormFile("file")
defer file.Close()
view, err := scoped.UploadAttachment(r.Context(), "task", taskRef, app.AttachmentUploadInput{
    Reader: file, DeclaredSize: header.Size, OriginalName: header.Filename,
    DisplayName: r.FormValue("display_name"), Mode: r.FormValue("mode"),
})
```

content 用 `io.Copy`，图片返回验证 MIME，其他文件强制 `Content-Disposition: attachment; filename*=UTF-8''...`，文件名按 RFC 5987 编码并剥离控制字符/路径分隔符/换行；设置 ETag、nosniff、private/no-store。404 对越权和不存在统一，503 不暴露 backend/key/path。

- [ ] **Step 4: 更新 Huma multipart/request/response schema**

`registerHumaBridge` 对 upload path 使用 `multipart/form-data`，import-url/PATCH 使用 JSON；OpenAPI 必须声明 409/410/413/415/422/502/503 响应和 `AttachmentView` 字段，不能把 `storage_backend`/`storage_key` 暴露进 schema。

- [ ] **Step 5: 运行 HTTP 测试**

Run: `go test ./internal/httpapi -run 'Attachment|OpenAPI' -count=1`

Expected: PASS，包括 multipart 断流、超限、非法 MIME、远程关闭、SSRF、quota、draft visibility、headers 和权限矩阵。

- [ ] **Step 6: 提交**

```bash
git add internal/httpapi/attachments.go internal/httpapi/attachments_test.go internal/httpapi/server.go internal/httpapi/app_service.go internal/httpapi/router.go internal/httpapi/middleware.go internal/httpapi/huma_routes.go internal/httpapi/server_test.go
git commit -m "feat: 暴露附件 HTTP 接口"
```

### Task 10: 建立 Web 附件传输和鉴权图片基础

**Files:**
- Modify: `web/src/features/workspace/session/workspace-api.ts`
- Modify: `web/src/features/workspace/session/workspace-api.test.ts`
- Create: `web/src/features/workspace/attachments/attachment-api.ts`
- Create: `web/src/features/workspace/attachments/attachment-api.test.ts`
- Create: `web/src/features/workspace/attachments/attachment-blob-cache.ts`
- Create: `web/src/features/workspace/attachments/attachment-blob-cache.test.ts`
- Create: `web/src/features/workspace/attachments/authenticated-attachment-image.tsx`
- Create: `web/src/features/workspace/attachments/authenticated-attachment-image.test.tsx`
- Create: `web/src/features/workspace/attachments/index.ts`

**Interfaces:**
- Consumes: Task 9 HTTP API 与现有 PAT/acting/OIDC credential 选择。
- Produces: `workspaceApiBlob`、`workspaceApiMultipart`、Web `Attachment` DTO/API 和 `AuthenticatedAttachmentImage`，使计划 2/3 可独立并行消费。

- [ ] **Step 1: 写失败 credential/blob/multipart 测试**

```ts
it.each([["workspace", "pat-token"], ["acting", "act-token"], ["oidc", null]])(
  "loads attachment with %s credentials",
  async (_mode, token) => {
    const blob = await workspaceApiBlob("/api/v1/attachments/a/content")
    expect(await blob.text()).toBe("payload")
    expect(lastRequest.headers.get("Authorization")).toBe(token ? `Bearer ${token}` : null)
    expect(lastRequest.credentials).toBe("same-origin")
  }
)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- workspace-api attachment-api authenticated-attachment-image`

Expected: FAIL，二进制/multipart/鉴权图片 helper 尚不存在。

- [ ] **Step 3: 抽取统一 credential request**

```ts
export function workspaceApiBlob(path: string, signal?: AbortSignal): Promise<Blob>
export function workspaceApiMultipart<T>(path: string, body: FormData, options?: { signal?: AbortSignal; onProgress?: (sent: number, total: number) => void }): Promise<T>
```

JSON/blob/multipart 共用现有 acting-first、PAT fallback、OIDC cookie 和 401 cleanup；FormData 不手写 boundary。上传进度使用 XMLHttpRequest 时也必须复用 Authorization、`credentials/withCredentials`、AbortSignal 和 error envelope 解析。

- [ ] **Step 4: 定义唯一 Web Attachment DTO/API**

```ts
export type Attachment = {
  id: string
  attached_to: { type: "task"; id: string }
  state: "draft" | "active" | "deleted"
  original_name: string
  display_name: string
  media_type: string
  size_bytes: number
  sha256: string
  inline_capable: boolean
  source_type: "upload" | "remote_url"
  content_url: string
  created_by: ActorInfo
  created_at: number
  modified_at: number
}
```

实现 `listTaskAttachments/uploadTaskAttachment/importTaskAttachmentURL/getAttachment/getAttachmentBlob/renameAttachment/removeAttachment`；后续计划不得复制 DTO 或 endpoint path。

- [ ] **Step 5: 实现 page-scope blob cache 和鉴权图片**

```ts
type CacheEntry = { promise: Promise<string>; refs: number; objectURL?: string }
export function acquireAttachmentBlob(attachment: Attachment, signal?: AbortSignal): Promise<{ url: string; release: () => void }>
```

key 固定为 `${attachment.id}:${attachment.sha256}`，最后 consumer release 时 revoke；失败移除 cache；workspace/token/acting/OIDC session 变化清空。`AuthenticatedAttachmentImage` 的真实 `<img src>` 只接受 object URL，不直接使用 content_url。

- [ ] **Step 6: 运行测试并提交**

Run:

```bash
pnpm --dir web test -- workspace-api attachment-api attachment-blob-cache authenticated-attachment-image
pnpm --dir web typecheck
```

Expected: PASS，PAT/acting/OIDC 三种模式一致、重复图片只请求一次、最后 unmount revoke。

```bash
git add web/src/features/workspace/session/workspace-api.ts web/src/features/workspace/session/workspace-api.test.ts web/src/features/workspace/attachments
git commit -m "feat: 建立 Web 附件访问基础"
```

### Task 11: 装配 server、janitor 和优雅停止

**Files:**
- Create: `internal/app/attachment_janitor.go`
- Create: `internal/app/attachment_janitor_test.go`
- Modify: `internal/app/attachment.go`
- Modify: `internal/app/attachment_test.go`
- Modify: `internal/httpapi/attachments.go`
- Modify: `internal/httpapi/attachments_test.go`
- Modify: `internal/cli/server.go`
- Modify: `internal/cli/server_test.go`

**Interfaces:**
- Consumes: `app.NewAttachmentRuntime`、`Service.CleanupAttachments`、`runtimeutil.ShutdownCoordinator`。
- Produces: `AttachmentJanitor.Run(context.Context)`，server 启动 fail-fast 和每小时小批清理。

- [ ] **Step 1: 写失败测试锁定删除顺序和重试**

```go
func TestAttachmentJanitorKeepsMetadataWhenBlobDeleteFails(t *testing.T) {
    janitor := newJanitorWithStore(t, failingDeleteStore{})
    result, err := janitor.RunOnce(context.Background(), 100)
    if err != nil || result.Retry != 1 { t.Fatalf("result=%#v err=%v", result, err) }
    if _, err := repo.GetByID(expiredID); err != nil { t.Fatal("metadata must remain for retry") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app ./internal/cli -run 'AttachmentJanitor|AttachmentRuntimeStartup' -count=1`

Expected: FAIL，janitor/server wiring 尚不存在。

- [ ] **Step 3: 实现 janitor 与 server wiring**

```go
type AttachmentJanitor struct { Store *storage.Store; Runtime *AttachmentRuntime; Clock Clock; BatchSize int }
func (j *AttachmentJanitor) RunOnce(ctx context.Context, limit int) (AttachmentCleanupResult, error)
func (j *AttachmentJanitor) Run(ctx context.Context)
```

server 在 `net.Listen` 前构造 runtime 并 health/readiness check；将 runtime 注入 `httpapi.Options`；`runtimeWG` 增加一个 janitor goroutine，默认每 1 小时运行、每批 100 条。收到 signal 后先 `StopAccepting`，取消 `runCtx`，等待 janitor 结束；一次 S3 运行期失败只影响附件操作，不把 `/healthz` 判死。

- [ ] **Step 4: 增加结构化可观测日志**

上传/远程抓取/下载记录 outcome、bytes、duration_ms、backend、media_type 和 attachment_id；远程抓取额外记录 redirect_count、reject_reason、source_host，但不记录 query/fragment/完整 URL。janitor 记录 scanned/purged/retry/orphan_count。仓库当前没有通用 metrics registry，因此本里程碑以这些结构化 counter-like 日志作为可采集信号；不要为附件单独引入 Prometheus/OTel 依赖。

- [ ] **Step 5: 运行测试**

Run: `go test ./internal/app ./internal/cli -run 'AttachmentJanitor|AttachmentRuntimeStartup|Shutdown' -count=1`

Expected: PASS；filesystem 不可写、S3 health 失败在监听端口前返回明确配置错误。

- [ ] **Step 6: 提交**

```bash
git add internal/app/attachment_janitor.go internal/app/attachment_janitor_test.go internal/app/attachment.go internal/app/attachment_test.go internal/httpapi/attachments.go internal/httpapi/attachments_test.go internal/cli/server.go internal/cli/server_test.go
git commit -m "feat: 运行附件清理任务"
```

### Task 12: 基础计划回归与交付门禁

**Files:**
- Modify: `docs/superpowers/specs/2026-07-19-task-description-rich-content-attachments-mentions-design.md`（仅在实现事实与 spec 发生必要偏差时同步，不能降低安全边界）

**Interfaces:**
- Consumes: Tasks 1–11。
- Produces: 可供后三份计划使用的稳定 foundation。

- [ ] **Step 1: 运行格式、静态检查和全量 Go 验证**

Run:

```bash
git diff --name-only --diff-filter=ACM -- '*.go' | xargs gofmt -w
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
pnpm --dir web test -- workspace-api attachment-api attachment-blob-cache authenticated-attachment-image
pnpm --dir web typecheck
git diff --check
```

Expected: 全部退出 0；不得并行再启动第二套全量 E2E。

- [ ] **Step 2: 运行 race-sensitive 目标测试**

Run:

```bash
go test ./internal/app -run 'Attachment.*Quota' -count=20
go test ./internal/safefetch -count=10
go test ./internal/httpapi -run Attachment -count=10
```

Expected: 全部 PASS，无偶发 quota 越限、临时文件泄漏或 HTTP 500。

- [ ] **Step 3: 检查对外字段和存储泄漏**

Run: `rg -n 'storage_backend|storage_key|filesystem_dir|s3.*endpoint' internal/httpapi internal/mcpserver internal/remote web/src`

Expected: 普通对外 DTO/JSON 不包含这些内部字段；仅配置或内部实现命中。

- [ ] **Step 4: 提交必要的验证修正**

```bash
git diff --name-only --diff-filter=ACM -z | xargs -0 git add --
git commit -m "test: 完善附件基础回归覆盖"
```

如果 `git status --short` 为空，不创建空提交。
