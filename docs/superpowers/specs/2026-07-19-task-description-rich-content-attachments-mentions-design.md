# 任务描述富文本粘贴、通用附件与语义引用设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。本文覆盖多个可独立验收的子系统，实施时按本文第 22 节拆成多份 plan，不要直接把整份规格作为一次提交实现。

- 日期：2026-07-19
- 状态：已确认，待实施
- 目标里程碑：v0.5.11
- 关联文档：[ROADMAP.md](../../../ROADMAP.md)、[README.md](../../../README.md)、`docs/superpowers/specs/2026-07-04-web-console-tiptap-markdown-editor-design.md`、`docs/superpowers/specs/2026-06-10-xuanchu-p1-semantic-events-design.md`、`docs/superpowers/specs/2026-07-10-unify-user-info-and-external-id-user-type-design.md`

## 1. 结论

本期继续把任务 `description` 持久化为 Markdown 字符串，不把 ProseMirror JSON、HTML 或独立字符位置表引入后端契约。在现有 Tiptap Markdown 编辑器上增加三类能力：

1. 从 Word、飞书文档、网页等来源复制粘贴富文本，按白名单转换为现有 Markdown schema。
2. 建立 workspace 内通用附件资源：附件通过 `attached_to_type + attached_to_id` 归属任意业务实体；首期由 task 接入，图片可以嵌入 description，其他文件既可插入为附件卡片，也可只存在于任务附件区。
3. 建立基于稳定 ID 的语义引用：首期支持 `@用户` 和 `#任务`，新增用户 mention 时产生 `task.user_mentioned` 事件，供 Notification Rule、Hook 和项目自动化消费。

附件元数据保存在 SQLite / PostgreSQL，二进制内容默认保存在本地文件系统；S3 兼容对象存储是可选后端。所有附件内容都通过璇础鉴权接口读取，不公开 bucket，不把临时签名 URL 写进 Markdown。

持久化示例：

```markdown
请 [@Alice](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001) 确认
关联 [#agentapi-17 · 补齐接口](ref://task/61f2a51e-0d5d-4f29-b502-cd195dfa1d84)

![当前架构](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)

[需求说明.pdf](ref://attachment/a801f977-c745-4f47-95a4-7893a9317aba)
```

显示文字是保存当时的快照；稳定身份只由 URI 中的 UUID 决定。CLI、MCP、Remote Client、HTTP JSON 和导入导出继续看到可读 Markdown 源码。

## 2. 背景与问题

当前 Web Console 已使用 Tiptap v3 编辑和渲染任务 description、任务注解，并支持标题、列表、引用、代码、表格、待办列表、安全链接和源码模式。编辑器内部使用 ProseMirror JSON，但数据库、App view 和所有对外协议仍保存 Markdown 字符串。

现有边界有四个缺口：

- 从办公软件或网页粘贴内容时，只能依赖浏览器/Tiptap 的默认 HTML 解析，缺少明确的清洗规则和兼容范围。
- Markdown schema 没有图片节点，后端也没有上传、存储、鉴权下载、生命周期和配额能力。
- 任务只有外部 `task_links`，没有任务内部文件资产；图片和普通附件无法统一管理。
- description 中写 `@Alice` 或任务编号只是普通文本，不能稳定定位实体，也不能产生可订阅事件。

如果直接存 HTML，会破坏 CLI、MCP、Task Bundle 和脚本消费契约；如果保存 ProseMirror JSON，会让 Markdown 成为有损投影；如果把 mention 的字符位置独立存表，任何 CLI/MCP 文本修改都可能造成位置漂移。因此本期必须在 Markdown 字符串契约之上增加可验证、可解析的扩展，而不是替换正文模型。

## 3. 目标

### 3.1 编辑与展示

- Web Console 支持粘贴富文本，并稳定转换为当前支持的 Markdown 结构。
- 支持通过剪贴板粘贴、拖拽和文件选择添加图片。
- 粘贴 HTML 中的公网远程图片由服务端受控抓取并转存为任务附件，正文不长期依赖第三方图片 URL。
- 支持 `@` 选择 workspace 成员，支持 `#` 选择当前调用者有权读取的任务。
- 用户、任务、图片和普通附件在富文本模式显示为语义节点，在源码模式显示为明确的 Markdown。
- 复制到外部应用时至少保留可读文本；在璇础编辑器之间复制时保留内部引用身份。

### 3.2 通用附件

- 附件是 workspace 内独立资源，通过稳定类型和资源 ID 关联业务实体，不把表结构锁死在 task。
- 首期只开放 `attached_to_type=task` 的 App/HTTP/CLI/MCP/Web 行为；project、series、workspace 等后续接入不需要迁移附件表或改变 `ref://attachment/{id}`。
- task 附件不要求必须出现在 description 中。
- 图片可以内联显示；其他文件显示为包含文件名、类型、大小和下载动作的附件卡片。
- 任务详情页提供附件列表、上传、重命名、下载和移除能力。
- 附件内容支持本地文件系统和可选 S3 后端，元数据统一保存在现有数据库。
- SQLite 和 PostgreSQL 行为一致，且所有新增 Go 依赖保持纯 Go、零 CGO。

### 3.3 语义引用与事件

- 用户引用保存用户 UUID，展示层遵守 `name` 是稳定查找键、`display_name` 只用于展示的既有边界。
- 任务引用保存任务 UUID，不依赖可变标题、项目 slug 或 task slug 作为稳定身份。
- 首期编辑器只开放 `user` 和 `task`，底层协议可以以后扩展 `project`、`series` 等类型。
- 每次保存 description 时按用户 ID 集合计算新增 mention；新增用户产生 `task.user_mentioned` 事件。
- Notification Rule 新增 `mentioned_users` audience；Hook 和项目自动化复用同一事件，不新增通道专用逻辑。

## 4. 非目标

- 不把 HTML、Tiptap/ProseMirror JSON 或 Delta 保存到数据库。
- 不实现多人实时协同、评论线程、行内批注或修订模式。
- 不在首期开放项目、循环系列、配置定义等实体的编辑器选择器。
- 不在本期提供 project、series、workspace 等非 task 实体的附件 API 和管理界面；本期只保证底层模型、存储、配额和权限解析边界可扩展。
- 不抓取需要登录态、Cookie、私网访问、客户端证书或交互式授权的远程图片；这类图片由用户下载后再上传。
- 不解析 CSS `background-image`、`picture/source` 艺术方向或页面脚本运行后才生成的图片；首期只处理剪贴板 HTML 中静态可解析的 `img` 候选。
- 不支持 SVG、HTML、脚本、可执行文件、带宏 Office 文件或浏览器可主动执行的附件类型。
- 不内置病毒扫描或 DLP；允许的压缩包按不透明下载文件处理，不解包检查。
- 不把 S3 bucket 设为 public，不持久化预签名 URL，不让浏览器直接持有 S3 凭证。
- 不在本期实现跨部署携带二进制附件的 Task Bundle v2；现有 JSON/XLSX/Task Bundle 只保留 Markdown 文本契约。
- 不通过 MCP base64 参数上传或下载二进制文件。
- 不为 `#任务` 引用生成通知事件；任务引用首期只承担稳定导航和语义关联。

## 5. 用户流程

### 5.1 粘贴富文本

1. 用户从 Word、飞书文档、Google Docs 或网页复制内容。
2. Web Console 读取 `text/html`，先按白名单清洗，再交给 Tiptap schema 解析。
3. 标题、段落、粗体、斜体、删除线、列表、待办、引用、代码、链接和表格被保留。
4. 字体、字号、颜色、背景色、class、style、事件属性、脚本、iframe 和未知标签被删除或降级为纯文本。
5. description 保存时仍提交 Markdown 字符串。

只有 `text/plain` 时，沿用 Tiptap 的 Markdown/纯文本粘贴。剪贴板同时包含 HTML 和实际图片文件时，优先使用图片文件，并按 DOM 位置替换对应 `<img>`，避免同一张图片既上传本地副本又抓取远程 `src`。剩余的公网 `http(s)` 图片进入服务端转存流程。

### 5.2 粘贴、上传或转存图片

1. 用户粘贴截图、拖入图片、点击图片按钮，或粘贴带远程 `<img>` 的 HTML。
2. 本地 File/data image 以 `mode=description_draft` 上传；公网远程图片把 source URL 提交给服务端 import endpoint，由服务端受控抓取。
3. 同一次粘贴按规范化后的 source URL 去重，前端最多并发 3 个远程抓取；HTML 中重复出现同一 URL 时复用一个 attachment ID。
4. 服务端只接受最终校验为 PNG、JPEG、GIF 或 WebP 的内容，并使用与普通上传完全相同的大小、像素、配额、BlobStore 和状态机。
5. 成功后把原 `<img>` 替换为 `![alt](ref://attachment/{id})`；上传/抓取过程中显示进度节点，不把远程 URL、data URL 或 blob URL 写进 Markdown。
6. 抓取失败时保留带 alt 和原 URL 的失败占位，提供“重试”“移除”“明确保留为普通外链”三个动作；失败占位本身不进入 Markdown，保存前必须由用户选择移除或保留外链。
7. 用户保存 description 时，App 层解析附件 ID，并在同一业务事务中把本次引用到的 draft attachment 转为 active。
8. 用户取消编辑时，前端尽力删除本次 draft；浏览器中断留下的 draft 由清理器在 24 小时后删除。

### 5.3 管理普通附件

1. 用户在任务详情的附件区点击“添加附件”。
2. 上传使用 `mode=attachment`，成功后附件立即成为 active，不依赖 description 保存。
3. 图片显示缩略图；PDF、文档、表格、演示、文本和压缩包显示文件卡片。
4. 用户可以修改展示名称、下载或移除附件。
5. 当前 description 正在引用的附件不允许移除，必须先删除正文引用并保存。

### 5.4 添加用户 mention

1. 用户输入 `@`，编辑器查询当前 workspace 的 active 成员。
2. 结果按 `display_name`、`name`、email 匹配，界面显示 `display_name` 优先，同时保留稳定 `name` 作为辅助信息。
3. 选择后插入用户引用节点，保存为 `[@展示文字](ref://user/{uuid})`。
4. App 层比较保存前后的用户 UUID 集合，只对新增集合生成一次 `task.user_mentioned`。
5. Notification Rule 可用 `audience=mentioned_users` 为每个被提及者生成 delivery；默认排除事件操作者本人。Hook 和项目自动化仍收到完整事件，包括自我 mention。

### 5.5 添加任务引用

1. 用户输入 `#`，编辑器查询当前 token scope、workspace/project allowlist 和行级权限内可读的实际任务。
2. 当前项目的任务优先展示，其他可读项目随后展示；projected occurrence 不进入首期结果，物化后才可引用。
3. 选择后保存为 `[#task_slug · title](ref://task/{uuid})`。
4. 渲染时使用当前标题和 canonical URL；目标不可读或已不可用时退回保存时的文字快照，并显示为不可点击节点。

## 6. Markdown 扩展契约

### 6.1 URI 语法

首期保留三类 URI：

```text
ref://user/{canonical-uuid}
ref://task/{canonical-uuid}
ref://attachment/{canonical-uuid}
```

约束：

- scheme 固定小写 `ref`。
- `user`、`task`、`attachment` 是 URI host；以后增加实体类型时增加 host，不改变 path 结构。
- UUID 必须是带连字符的 canonical lowercase 形式。
- 不允许 userinfo、port、query、fragment、额外 path segment 或 percent-encoded UUID。
- `ref://` 是 Markdown 内容内部引用的保留协议；格式非法、类型未开放或目标越权时，写操作返回 422，不把它降级为普通外链。
- 仓库既有 `xuanchu://workspace/...`、`xuanchu://project/...` 等 URI 继续只表示 MCP Resource；description 不复用该 scheme，避免资源协议和正文实体引用混淆。
- 普通 `http`、`https`、`mailto` 链接继续使用现有白名单。

### 6.2 Markdown 解析

Go 侧新增纯 Go `github.com/yuin/goldmark`，通过 AST 读取 link/image destination，不使用正则表达式解释 Markdown。解析结果至少包含：

```go
type ContentReference struct {
    Kind  string // user|task|attachment
    ID    string
    Label string
    Image bool
}
```

解析器放在 `internal/task`，不依赖 Cobra、GORM、HTTP 或前端。App 层负责根据 workspace、request scope 和 repository 解析目标、校验权限、批量加载 `task.UserInfo`。

### 6.3 兼容规则

- 历史 Markdown 没有保留协议，无需数据迁移。
- 普通文本 `@Alice`、`#agentapi-17` 不自动变成引用，避免改写历史语义。
- 用户在源码模式手写合法内部 URI，与富文本模式插入产生完全相同的服务端校验和事件。
- label 变化不改变引用身份；事件差异只比较 `(kind, id)`。
- 同一用户在 description 中出现多次，集合层只算一个 mention。
- description 单次保存最多包含 200 个内部引用；超过返回 `description_reference_limit_exceeded`。
- 修改后的 description 最大为 512 KiB UTF-8；超过返回 `description_too_large`。历史超限内容可以读取，只有再次保存时需要缩减。

## 7. 代码边界

### 7.1 后端

| 位置 | 职责 |
|---|---|
| `internal/task/content_reference.go` | Markdown AST 解析、内部 URI 语法、引用集合差异 |
| `internal/blobstore/` | 二进制存储接口、本地文件系统实现、S3 实现、流式读写与删除 |
| `internal/safefetch/` | 无 Cookie 的公网 HTTP 图片抓取、DNS/IP/redirect SSRF 防护和响应限流 |
| `internal/storage/attachment_repo.go` | 通用附件元数据 CRUD、按 attached resource/workspace 配额统计、状态迁移和清理查询 |
| `internal/app/attachment.go` | attachment target 解析、资源读写权限、上传/远程转存生命周期、引用绑定、附件 view |
| `internal/app/content_reference.go` | 用户/任务批量解析、suggest/resolve、mention 事件数据 |
| `internal/httpapi/attachments.go` | multipart 上传、metadata/content/list/modify/delete HTTP handler |
| `internal/httpapi/content_references.go` | suggestion 和 batch resolve HTTP handler |
| `internal/app/event_notification.go` | `mentioned_users` audience 解析 |
| `internal/app/project_automation_*` | `mentioned_users` 模板变量和事件上下文 |
| `internal/config/` | attachments backend、配额和 S3 配置解析 |

`internal/httpapi` 不解释附件业务状态，不直接访问 GORM、S3 或远程图片 URL；所有写操作必须进入 `internal/app`。`internal/storage` 不解析 Markdown，不解释 actor 权限或 attached resource 类型。`internal/blobstore` 不知道 task、workspace、用户或权限。`internal/safefetch` 只负责安全取得受限字节流，不创建 attachment row，不决定资源权限。

App 层定义统一 target：

```go
type AttachmentTarget struct {
    Type        string // task；后续可增加 project|series|workspace
    ID          string // 目标资源稳定 UUID
    WorkspaceID string
}

type AttachmentTargetView struct {
    Type string
    ID   string
}
```

`resolveAttachmentTarget(type, ref)` 负责解析稳定 ID、确认 workspace 归属并执行资源类型对应的 read/write/closed-state 判断。首期只注册 task handler；未知 type 返回 `attachment_target_type_unsupported`。删除前的引用检查也按 target type 分派：首期 task handler 检查当前 task description，后续实体接入时增加自己的 checker，不在通用 repository 中解析正文。

### 7.2 Web

| 位置 | 职责 |
|---|---|
| `web/src/components/markdown/paste-sanitizer.ts` | DOMPurify 白名单与粘贴 HTML 归一化 |
| `web/src/components/markdown/reference-extension.ts` | 用户/任务原子节点、Markdown parse/serialize、Suggestion 触发器 |
| `web/src/components/markdown/attachment-extension.ts` | 图片/文件节点、Markdown parse/serialize、authenticated blob 加载 |
| `web/src/components/markdown/attachment-node-view.tsx` | 图片、上传进度、文件卡片、失败与重试状态 |
| `web/src/features/workspace/attachments/` | 附件 API、附件面板、上传队列、重命名和删除交互 |
| `web/src/features/workspace/content-references/` | suggest/resolve API、缓存和不可用引用降级 |

现有 `markdownExtensions` 仍是编辑和只读渲染的单一 schema 来源。不得为只读展示另建 remark/react-markdown 解析链。

## 8. 附件数据模型

新增通用 `attachments`：

```go
type Attachment struct {
    ID                   string  `gorm:"primaryKey"`
    WorkspaceID          string  `gorm:"not null;index;index:idx_attachments_target,priority:1"`
    AttachedToType       string  `gorm:"not null;index:idx_attachments_target,priority:2"`
    AttachedToID         string  `gorm:"not null;index:idx_attachments_target,priority:3"`
    State                string  `gorm:"not null;index"` // uploading|draft|active|deleted
    OriginalName         string  `gorm:"not null"`
    DisplayName          string  `gorm:"not null"`
    MediaType            string  `gorm:"not null"`
    Extension            string  `gorm:"not null"`
    SizeBytes            int64   `gorm:"not null"`
    SHA256               string  `gorm:"not null;index"`
    InlineCapable        bool    `gorm:"not null;default:false"`
    EverEmbedded         bool    `gorm:"not null;default:false"`
    SourceType           string  `gorm:"not null;default:'upload'"` // upload|remote_url
    SourceHost           string  `gorm:"not null;default:''"`
    SourceURLHash        string  `gorm:"not null;default:'';index"`
    StorageBackend       string  `gorm:"not null"` // filesystem|s3
    StorageKey           string  `gorm:"not null;uniqueIndex"`
    CreatedBy            string  `gorm:"not null;index"`
    CreatedByActorType   string  `gorm:"not null;default:'user';index"`
    CreatedByUserID      *string `gorm:"index"`
    CreatedByTokenID     *string `gorm:"index"`
    CreatedByTokenName   *string
    CreatedByTokenPrefix *string
    CreatedAt            int64   `gorm:"not null;index"`
    ModifiedAt           int64   `gorm:"not null"`
    ExpiresAt            *int64  `gorm:"index"`
    DeletedAt            *int64  `gorm:"index"`
    PurgeAfter           *int64  `gorm:"index"`
}
```

约束：

- 数据库只保存元数据和存储 key，不保存文件内容。
- `StorageKey` 由服务端生成，不含原始文件名。
- 远程转存只记录规范化 host 和 source URL 的 SHA-256，不保存可能含签名参数、访问 token 或个人信息的完整 URL。
- `CreatedBy` 对外必须转换为 `task.ActorInfo`；如果是用户，内层必须使用完整 `task.UserInfo`，不输出裸 UUID。
- `active` 附件默认出现在 attached resource 的附件列表；首期对应任务附件列表。`draft` 只对创建它的 actor 可见；`deleted` 默认不出现在列表。
- `EverEmbedded` 一旦为 true 不再回退，用于删除提示和审计保留判断。
- GORM migration 同时覆盖 SQLite 和 PostgreSQL；SQLite 迁移保持纯 Go driver。
- `(WorkspaceID, AttachedToType, AttachedToID)` 是所有列表、配额和生命周期查询的归属键；首期写入的 type 固定为 `task`。
- 每个 attachment 只有一个 attached target，创建后不可修改或“移动”；本期不引入多对多共享附件。某个实体正文只能嵌入归属于同一实体的附件，避免借内部 URI 跨资源复用绕过生命周期和权限。
- attached resource 与附件 metadata 不使用数据库级多态外键或 `ON DELETE CASCADE`：数据库无法安全表达多态关联，删除 blob 又是外部副作用。资源硬清理必须先由 App 层清理附件对象，再删除 metadata/resource，不能让数据库先删 metadata 留下不可追踪对象。

对外 view：

```go
type AttachmentView struct {
    ID            string
    AttachedTo    AttachmentTargetView
    State         string
    OriginalName  string
    DisplayName   string
    MediaType     string
    SizeBytes     int64
    SHA256        string
    InlineCapable bool
    SourceType    string
    ContentURL    string
    CreatedBy     task.ActorInfo
    CreatedAt     int64
    ModifiedAt    int64
}
```

`StorageBackend`、`StorageKey` 和内部 target resolver 信息永不进入普通 HTTP、CLI、MCP 或 Hook 输出。`AttachedTo` 只返回 `type/id`，不重复嵌入完整 task/project/workspace 对象。

## 9. BlobStore 与存储后端

### 9.1 接口

新增不依赖 GORM 的流式接口：

```go
type Store interface {
    Put(ctx context.Context, key string, src io.Reader, size int64, mediaType string) error
    Open(ctx context.Context, key string) (io.ReadCloser, BlobInfo, error)
    Delete(ctx context.Context, key string) error
    Health(ctx context.Context) error
}
```

上传 handler 不能 `io.ReadAll` 整个 multipart 文件；使用 `io.LimitedReader` 流式计算 SHA-256、写临时文件并执行大小检查。下载也必须流式写入 response。

### 9.2 本地文件系统

默认后端为 `filesystem`，默认根目录：

```text
<data-dir>/attachments
```

对象路径：

```text
<root>/<workspace-id>/<attachment-id[0:2]>/<attachment-id[2:4]>/<attachment-id>
```

规则：

- 根目录和 workspace 目录使用 `0700`，文件使用 `0600`。
- 上传先写同文件系统的 `.tmp/{attachment-id}`，完成校验和 `fsync` 后原子 rename。
- 不把用户文件名拼入路径，不接受调用方传入的 storage key。
- 启动时验证目录可创建、可写；配置后端不可用时 server 启动失败并给出明确错误。

### 9.3 S3 可选后端

S3 使用纯 Go `github.com/aws/aws-sdk-go-v2`，兼容 AWS S3、MinIO 和实现必要 S3 API 的对象存储。

对象 key：

```text
{prefix}/workspaces/{workspace-id}/attachments/{attachment-id}
```

规则：

- bucket 必须是 private，璇础不自动创建 bucket、不修改 bucket policy。
- 服务端代理所有上传和下载；不把 S3 URL 或预签名 URL 写入 Markdown、数据库 view 或事件。
- 上传设置服务端已确认的 `Content-Type` 和 SHA-256 metadata。
- 支持 `AES256` 和 `aws:kms` server-side encryption；选择 KMS 时必须配置 key ID。
- 自定义 endpoint 默认要求 HTTPS；只有显式 `allow_insecure_endpoint=true` 才允许 HTTP，供本机 MinIO 使用。
- 凭证只使用 AWS SDK 默认 credential chain，包括环境变量、shared credentials、ECS/EC2 role 和 Web Identity；璇础 TOML 不增加 access key/secret key 明文字段。
- 启动时执行 `HeadBucket`/最小 health 检查；配置错误时 fail fast。

### 9.4 后端切换

每条附件记录保存其写入时的 `StorageBackend`。`attachments.backend` 只决定新上传写到哪里；读取和删除按记录中的 backend 分派。

- filesystem store 始终按配置目录注册。
- 配置 S3 后同时注册 S3 store。
- 从 filesystem 切换到 S3 不自动搬迁历史对象；只要旧目录仍挂载，历史附件可继续读取。
- 本期不提供批量存储迁移命令。迁移服务器前必须同时迁移本地附件目录，不能只迁移数据库。

## 10. 配置

TOML：

```toml
[attachments]
backend = "filesystem"                 # filesystem | s3
filesystem_dir = ""                   # 空值 = <data-dir>/attachments
max_file_size_mb = 25
max_resource_total_size_mb = 200
max_workspace_total_size_mb = 10240
max_attachments_per_resource = 100
draft_ttl = "24h"
deleted_retention = "720h"             # 30 天；0 表示删除后尽快清理
remote_fetch_enabled = true
remote_fetch_timeout = "30s"
remote_fetch_max_redirects = 5
remote_fetch_max_concurrency = 4         # 单进程、单 workspace

[attachments.s3]
bucket = ""
region = ""
endpoint = ""
prefix = "xuanchu/attachments"
force_path_style = false
allow_insecure_endpoint = false
server_side_encryption = ""            # "" | AES256 | aws:kms
kms_key_id = ""
```

环境变量覆盖：

```text
XUANCHU_ATTACHMENTS_BACKEND
XUANCHU_ATTACHMENTS_FILESYSTEM_DIR
XUANCHU_ATTACHMENTS_MAX_FILE_SIZE_MB
XUANCHU_ATTACHMENTS_MAX_RESOURCE_TOTAL_SIZE_MB
XUANCHU_ATTACHMENTS_MAX_WORKSPACE_TOTAL_SIZE_MB
XUANCHU_ATTACHMENTS_MAX_ATTACHMENTS_PER_RESOURCE
XUANCHU_ATTACHMENTS_DRAFT_TTL
XUANCHU_ATTACHMENTS_DELETED_RETENTION
XUANCHU_ATTACHMENTS_REMOTE_FETCH_ENABLED
XUANCHU_ATTACHMENTS_REMOTE_FETCH_TIMEOUT
XUANCHU_ATTACHMENTS_REMOTE_FETCH_MAX_REDIRECTS
XUANCHU_ATTACHMENTS_REMOTE_FETCH_MAX_CONCURRENCY
XUANCHU_ATTACHMENTS_S3_BUCKET
XUANCHU_ATTACHMENTS_S3_REGION
XUANCHU_ATTACHMENTS_S3_ENDPOINT
XUANCHU_ATTACHMENTS_S3_PREFIX
XUANCHU_ATTACHMENTS_S3_FORCE_PATH_STYLE
XUANCHU_ATTACHMENTS_S3_ALLOW_INSECURE_ENDPOINT
XUANCHU_ATTACHMENTS_S3_SERVER_SIDE_ENCRYPTION
XUANCHU_ATTACHMENTS_S3_KMS_KEY_ID
```

AWS 凭证继续使用 `AWS_ACCESS_KEY_ID`、`AWS_SECRET_ACCESS_KEY`、`AWS_SESSION_TOKEN`、`AWS_PROFILE`、Web Identity 等 SDK 标准来源。

配置校验：

- backend 未设置时为 `filesystem`。
- S3 backend 必须有 bucket 和 region；自定义 endpoint 仍必须给 region。
- `aws:kms` 必须有 `kms_key_id`，非 KMS 模式禁止残留 KMS key。
- 文件大小、attached resource/workspace 容量和每资源附件数必须为正；workspace 总量不得小于 resource 总量，resource 总量不得小于单文件上限。
- duration 必须能被 `time.ParseDuration` 解析且非负。
- remote fetch timeout 必须大于 0 且不超过 120 秒；redirect 上限为 0-10；并发数必须为 1-32。

## 11. 上传、绑定、删除与清理

### 11.1 上传状态机

```text
uploading -> draft -> active -> deleted -> purged
          \-> active
```

- 创建 metadata row 时先写 `uploading`。
- blob 写入、MIME/扩展名/尺寸/哈希校验完成后：`mode=description_draft` 进入 `draft`，`mode=attachment` 进入 `active`。
- description 保存时，合法引用到的同任务 draft 转为 `active`，并设置 `EverEmbedded=true`。
- draft 只允许创建它的 actor 绑定；用户 actor 按 user ID 匹配，非用户 token actor 按 token ID 匹配。
- `uploading` 超过 1 小时或 `draft` 超过配置 TTL 后可清理。

### 11.2 一致性

数据库和文件/S3 不能共享事务，因此使用状态机和补偿：

1. 事务创建 `uploading` row。
2. 流式写 blob。
3. 事务更新最终 metadata 和 state。
4. 第 2 步失败时删除 row；删除失败交给清理器。
5. 进程在第 2、3 步之间崩溃时，清理器根据 stale `uploading` row 删除 blob 和 row。
6. description 修改与 draft 激活、audit、semantic event 生成在同一数据库事务中完成。

### 11.3 删除语义

- draft 删除立即尝试删除 blob 和 row。
- active 删除先检查当前 description；仍被引用时返回 409 `attachment_in_use`。
- 可删除的 active 变为 `deleted`，从普通附件列表隐藏，并设置 `PurgeAfter`。
- retention 期间，拥有该 task `task:read` 的调用者仍可读取内容，使 description audit 历史可以继续渲染。
- 到期后清理器删除 blob；成功后删除 metadata row。删除 blob 失败时保留 row 并重试，不能先删 metadata 制造不可追踪孤儿。
- deleted 附件在实际 purge 前仍计入 workspace/task 存储配额。

### 11.4 清理运行时

`xuanchu server` 启动 `AttachmentJanitor`，默认每小时运行一次，按小批次处理 stale uploading、expired draft 和到期 deleted。janitor 复用现有 shutdown coordinator，支持优雅停止，不阻塞 HTTP 请求。

本地 CLI 模式不启动常驻循环；执行附件写命令时最多 opportunistic 清理一批过期记录。

## 12. 文件类型、安全与配额

### 12.1 允许类型

可内联图片：

```text
.png  image/png
.jpg/.jpeg  image/jpeg
.gif  image/gif
.webp  image/webp
```

仅下载附件：

```text
.pdf
.txt .md .csv .json
.docx .xlsx .pptx
.zip .7z .tar .gz .tgz
```

服务端同时检查扩展名、`http.DetectContentType`/格式 magic 和必要的格式解析。图片必须能解码 header，宽高均不得超过 20,000 像素，总像素不得超过 40,000,000。

OOXML 文件必须是有效 ZIP，包含 `[Content_Types].xml` 以及与扩展名匹配的 `word/`、`xl/` 或 `ppt/` 目录，并且不能包含 `vbaProject.bin`。普通 ZIP/7z/tar/gzip 只校验容器 magic/header，不遍历或解压其中内容。

固定拒绝：

```text
.html .htm .xhtml .svg .xml
.js .mjs .cjs .wasm
.sh .bash .zsh .fish .bat .cmd .ps1 .vbs
.exe .dll .msi .com .scr .apk .app .dmg .pkg .jar
.docm .xlsm .pptm
```

MIME 与扩展名冲突、无法识别或不在允许表内时返回 `attachment_type_not_allowed`。压缩包不解包检查，只以 `Content-Disposition: attachment` 下载；界面明确显示“压缩包未经过内容扫描”。

### 12.2 配额

默认限制：

- 单文件 25 MiB。
- 单一 attached resource 尚未 purge 的附件总量 200 MiB。
- 单 workspace 尚未 purge 的附件总量 10 GiB。
- 单一 attached resource 的 `uploading + draft + active` 最多 100 个附件。

配额检查在开始上传前做一次声明值检查，在流式接收结束后按实际大小再次检查。并发上传最终提交时必须在事务内重新统计，避免竞争越过配额。

### 12.3 HTTP 安全响应

- metadata/list/content 都要求鉴权和 workspace 行级隔离。
- content response 设置 `X-Content-Type-Options: nosniff`。
- 图片返回已验证的图片 MIME；其他类型统一 `Content-Disposition: attachment`。
- 文件名使用 RFC 5987 编码，剥离控制字符、路径分隔符和换行，防止 header 注入。
- content 设置基于 SHA-256 的 ETag 和 `Cache-Control: private, no-store`。
- CSP 增加 `img-src 'self' blob: data:`；`data:` 只用于编辑器本地预览，持久化内容不得保留 data URL。
- S3 错误、storage key、bucket、endpoint 和本地绝对路径不得返回给普通调用者。

### 12.4 远程图片安全抓取

远程图片只能由服务端 `internal/safefetch` 抓取，浏览器不直接加载粘贴来源的 URL。实现复用并抽取现有 Hook/Notification 出站防护中的 DNS/IP 判断，不能另写一套弱化版 URL 校验。

请求规则：

- 只允许 `http` 和 `https`，拒绝 `file`、`ftp`、`data`、`blob`、userinfo、空 host 和畸形端口。
- HTTP 只允许有效端口 80，HTTPS 只允许有效端口 443；非标准端口不进入首期远程转存范围。
- 不携带浏览器 Cookie、Authorization、Referer 或 workspace/token 凭证；使用固定、无身份信息的 User-Agent。
- HTTP client 禁用环境代理，避免通过内部 proxy 绕过目标地址校验。
- DNS 解析得到的所有地址都必须是公网地址；任一结果属于 loopback、RFC1918、link-local、multicast、unspecified、RFC6598 或其它保留网段时整次请求失败。
- 自定义 `DialContext` 只连接本次已经校验过的公网 IP，Host/TLS SNI 仍使用原域名，阻止校验后 DNS rebinding。
- 最多跟随配置数量的 301/302/303/307/308 redirect；每一跳重新执行 scheme、host、DNS 和 IP 校验。HTTPS 跳转到 HTTP 被拒绝。
- 只接受 2xx 响应。`Content-Length` 已知且超过单文件限制时立即中止；未知长度使用 `io.LimitedReader(max+1)`。
- 不信任远端 `Content-Type` 和扩展名，最终以内存/临时文件中的 magic、图片 decode config、像素限制和第 12.1 节 allowlist 为准。
- 总请求时间使用 `remote_fetch_timeout`；连接、TLS handshake、响应 header 和 body 读取都受 context 取消。
- 单 workspace 受进程内 semaphore 限制；超过并发上限时排队等待当前请求 context，不创建无界后台任务。

远程抓取不会访问经过认证的内部知识库、企业内网或 localhost。抓取不到不代表粘贴整体失败：文本继续进入编辑器，图片以失败占位等待用户重试、移除或明确保留为普通 HTTPS 链接。

## 13. HTTP API

### 13.1 附件

```text
POST   /api/v1/tasks/{taskRef}/attachments
POST   /api/v1/tasks/{taskRef}/attachments/import-url
GET    /api/v1/tasks/{taskRef}/attachments
GET    /api/v1/attachments/{attachmentID}
GET    /api/v1/attachments/{attachmentID}/content
PATCH  /api/v1/attachments/{attachmentID}
DELETE /api/v1/attachments/{attachmentID}
```

上传使用 `multipart/form-data`：

```text
file=<binary>                         必填
mode=attachment|description_draft    必填
display_name=<text>                  可选
```

远程图片转存使用 JSON，不与 multipart 混用：

```json
{
  "source_url": "https://cdn.example.com/architecture.png",
  "mode": "description_draft",
  "display_name": "架构图.png"
}
```

- `mode` 首期固定为 `description_draft`；普通附件区不提供任意 URL 下载器。
- endpoint 只接受最终内容为允许内联的图片，不把 HTML、PDF 或未知文件作为远程附件导入。
- `display_name` 可省略；服务端依次使用安全的 `Content-Disposition` filename、URL path basename、`remote-image.{detected-ext}`。
- 同一请求不按 URL 做全局去重；前端只在单次粘贴内复用重复 URL，避免不同任务意外共享附件权限或生命周期。

`PATCH` 首期只接受：

```json
{
  "display_name": "最终需求说明.pdf"
}
```

列表默认只返回 active；`include_drafts=true` 只返回当前 actor 自己的 draft，供编辑器恢复/清理，不允许查看其他 actor 的 draft。

上传、远程转存、PATCH、DELETE 要求 `task:write`，并遵守 project allowlist、workspace allowlist、membership role 和 closed project 不可写规则。远程转存被配置关闭时返回稳定错误，不退化为浏览器直连。列表、metadata 和 content 要求 `task:read`。

task-scoped 创建 endpoint 只接收 `taskRef`，服务端解析后写入 `attached_to_type=task` 和稳定 task UUID；请求体不允许传入或覆盖 `attached_to_*`。通用 metadata `PATCH` 也不能改变 attached target。后续增加 project/series/workspace endpoint 时必须先注册对应 App target handler，不能仅靠客户端提交 type/id 开启新类型。

### 13.2 引用建议

```text
GET /api/v1/content-references/suggestions?type=user|task&q=<query>&project=<projectRef>&limit=20
```

- `type=user` 要求 `member:read`，只返回当前 workspace active member，响应中的用户必须是完整 `task.JSONUserInfo`。
- `type=task` 要求 `task:read`，只返回 request scope 可读的实际任务；返回 `id/title/task_slug/project/url/status`，不返回 description。
- `q` 允许为空：空 query 时 user 返回当前 workspace 全部 active member、task 返回可读任务（均按 limit 截断），让用户敲 `@` / `#` 就看到候选列表；非空 query 则按 display_name/name/email（user）或 title/slug（task）过滤。limit 默认 20、最大 50。
- task 结果当前项目优先，然后按最近修改时间和 title 排序。

### 13.3 批量解析

```text
POST /api/v1/content-references/resolve
```

请求最多 200 个引用，支持 `user`、`task` 和 `attachment`；attachment 用于 description 渲染时批量取得附件 metadata，避免每个图片/文件各发一次 metadata 请求：

```json
{
  "references": [
    {"type": "user", "id": "8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001"},
    {"type": "task", "id": "61f2a51e-0d5d-4f29-b502-cd195dfa1d84"},
    {"type": "attachment", "id": "40af0185-316f-42bb-b52b-545d21f6f012"}
  ]
}
```

响应保持输入顺序。可读目标返回 `status=resolved` 和 typed object；attachment 返回完整 `AttachmentView`，并通过 attached target handler 校验调用者对目标资源的读取权限。不存在或不可读统一返回 `status=unavailable`，不区分 403/404，避免枚举资源。混合批次按每个引用独立做权限判断：缺少某一资源的 read capability 只让对应项 unavailable，不泄漏目标是否存在。

## 14. CLI、Remote Client 与 MCP

### 14.1 CLI / Remote

新增：

```bash
xuanchu attachment add <task-ref> <file> [--display-name <name>]
xuanchu attachment list <task-ref>
xuanchu attachment info <attachment-id>
xuanchu attachment download <attachment-id> [--output <path>]
xuanchu attachment rename <attachment-id> <display-name>
xuanchu attachment remove <attachment-id>
```

- 本地 CLI 复用 App service 和 BlobStore。
- Remote CLI 使用同一 HTTP API；上传/download 必须流式处理，不把文件整体 base64 放进 JSON。
- stdout 只放结果或文件内容；默认 download 写文件，`--output -` 才写 stdout；进度写 stderr。
- `--json` 只用于 metadata/list/info/write result，不与二进制 stdout 混用。

description 中的内部 URI 原样显示和导出；CLI 不把显示 label 当稳定 ID。

### 14.2 MCP

新增只处理 metadata 的工具：

```text
task_attachment_list
task_attachment_get
task_attachment_rename
task_attachment_remove
```

MCP 不新增 base64 upload/download tool。`task_attachment_get` 返回 metadata、`content_url` 和说明：需要使用同一 Bearer token 通过 HTTP 下载内容。所有用户字段继续输出完整 `task.UserInfo`。

## 15. Web Console 编辑器

### 15.1 富文本清洗

新增 `dompurify`，粘贴时使用显式 allowlist。允许的 HTML 元素仅覆盖现有 Markdown schema：

```text
p br h1 h2 h3 strong b em i s del
ul ol li blockquote pre code
table thead tbody tr th td
a
```

只允许链接的 `href/title`、表格必要的 `colspan/rowspan` 和内部节点的受控 `data-xuanchu-*` 属性。删除 `style`、`class`、`id`、所有 `on*` 属性、iframe/object/embed/form/input/video/audio 和未知元素。

清洗前先按 DOM 顺序提取 `<img>`：实际 clipboard File 与 data image 进入普通上传，公网 `http(s)` URL 进入服务端 `import-url`。候选 URL 优先级为 `src`、`data-src`/`data-original`、`srcset` 中最高分辨率项；每个图片位置先替换为本地异步占位，成功后变成 `XuanchuAttachment`，失败后保留重试/移除/转普通外链动作。随后 DOMPurify allowlist 不再保留任何原始 `img`，确保 data URL、blob URL 和远程 URL 都不能绕过附件流程进入持久化内容。

远程 URL 规范化只用于单次粘贴去重：scheme/host 小写、移除 fragment、保留 path/query，不能删除 query 后合并不同签名图片。前端远程转存并发固定为 3，组件卸载或取消编辑时使用 AbortController 取消未完成请求。

### 15.2 Tiptap 节点

新增两个 extension：

- `XuanchuReference`：inline atom，attrs 为 `kind/id/label`；解析/序列化为标准 Markdown link。
- `XuanchuAttachment`：图片为 block atom，普通文件为 inline/block card；attrs 为 `id/label/image`；解析/序列化为 Markdown image/link。

编辑和静态渲染共用同一 extension 定义。源码模式切回富文本时重新解析并校验节点；非法保留 URI 显示源码错误并禁止保存，不静默丢失内容。

### 15.3 鉴权图片加载

普通 PAT/Agent token Console 把 token 放在 `sessionStorage`，浏览器原生 `<img src>` 无法附带 Authorization。因此不得把受保护 content URL 直接交给 `<img>`。

`AuthenticatedAttachmentImage` 必须：

1. 使用现有 workspace session 选择 acting token、普通 token 或 OIDC cookie。
2. `fetch(content_url)` 获取 Blob。
3. 创建 `URL.createObjectURL(blob)` 给实际 `<img>`。
4. 组件卸载、引用变化或 token 变化时 revoke object URL。
5. 按 attachment ID + SHA-256 做当前页面内缓存和请求去重。

OIDC cookie 模式也走同一 fetch 路径，避免两套安全语义。

### 15.4 Mention/任务选择器

- `@` 和 `#` 只在普通文本位置触发，不在 code/codeBlock/link 内触发。
- 支持上下键、Enter、Esc、鼠标和 IME；中文输入法 composing 阶段不提前提交选择。
- 空查询不请求服务端；输入防抖 150ms，旧请求使用 AbortController 取消。
- 用户结果展示 `display_name`、稳定 `name`、可选 email；不得用 `display_name` 回传或查找用户。
- 任务结果展示 task slug、title、project 和状态。
- 已插入节点是原子节点，Backspace 一次删除整个节点；复制时提供 HTML、纯文本和 `application/x-xuanchu-markdown` 三种 clipboard 表达。

### 15.5 附件面板

任务详情主叙事区新增“附件”区：

- 默认显示 active 附件数量和最近附件；展开后显示完整列表。
- 图片显示缩略图；其他文件使用类型图标。
- 每行显示展示名称、大小、创建者和创建时间。
- 可写用户看到上传、重命名、移除；只读用户只看到下载。
- closed project 隐藏写入口，但继续允许读取。
- 上传队列显示逐文件进度、错误和重试，不因一个文件失败取消其他文件。

## 16. 语义事件与通知

### 16.1 事件生成

新增事件类型：

```text
task.user_mentioned
```

计算：

```text
added = unique(next user reference IDs) - unique(previous user reference IDs)
```

只有 `added` 非空时生成事件。移动节点、修改 label、删除再在同一次保存中重新插入、重复出现同一用户都不产生重复事件。先删除保存、之后再次添加并保存视为新的 mention，会再次产生事件。

事件 payload：

```json
{
  "event_type": "task.user_mentioned",
  "source_field": "description",
  "task": {"uuid": "...", "title": "...", "url": "..."},
  "mentioned_users": [
    {
      "id": "...",
      "name": "alice",
      "display_name": "Alice",
      "email": "alice@example.com",
      "external_ids": []
    }
  ],
  "current_mentioned_users": []
}
```

`mentioned_users` 和 `current_mentioned_users` 必须使用 `task.UserInfo` / `task.JSONUserInfo`，不允许裸 UUID。

### 16.2 Notification Rule

event notification audience 新增：

```text
mentioned_users
```

- 只允许用于 `task.user_mentioned`，其他 event type 创建/修改规则时返回 `audience_unsupported_for_event`。
- 每个新增 mentioned user 生成一条 notification delivery。
- 默认排除与 actor user ID 相同的 recipient；如果 actor 是 tenant/agent token，没有 user actor 则不做排除。
- 同一 event/rule/recipient 继续使用现有 dedupe key，不能重复生成 delivery。
- audience 为空不影响 task 修改，不生成 delivery，只记录 debug 级结构化日志。

模板上下文继续提供 `recipient.*`、`actor.*`、`task.*`；无需模板作者自行遍历 `mentioned_users`。

### 16.3 Hook 与项目自动化

- Hook 白名单加入 `task.user_mentioned`，事件 envelope 自然包含完整 payload。
- 项目自动化 event trigger 白名单加入该事件。
- 自动化上下文 include 新增 `mentioned_users`，模板变量 `{{mentioned_users}}` 是完整 JSON 数组。
- 不新增 mention 专用 dispatcher，不直接调用飞书、企业微信、钉钉或邮件。

## 17. 权限、租户与状态边界

- 附件 metadata row 必须有 `WorkspaceID + AttachedToType + AttachedToID`；所有读取先解析 attachment，再由对应 target handler 做 workspace/project/资源状态授权。首期 `task` handler 复用现有 task 行级授权。
- 不能仅凭不可猜 UUID 绕过权限。
- 用户 suggestion 只包含当前 workspace 成员；用户引用不能跨 workspace。
- 任务 suggestion/reference 必须在当前 workspace 且位于 request scope 可读项目内。
- 保存 description 时重新校验所有新增内部引用，不能信任前端 suggestion 结果。
- 历史已有引用若目标后来不可读，不阻止编辑无关文本；只要调用者没有新增或改变该引用，保留原 URI 和 label。调用者尝试新建/替换为不可读目标时拒绝。
- archived/cancelled project 的任务 description、task 附件和 mention 都保持只读；后续 target type 必须在接入时定义自己的 closed-state 规则。
- admin acting、tenant access、PAT、Agent token 和 OIDC session 继续经过现有 authz decision；附件不增加旁路鉴权。
- draft 只有创建 actor 可读取/绑定；active/deleted-retention 内容按 task read 权限读取。

## 18. API 错误

| code | HTTP | 含义 |
|---|---:|---|
| `description_too_large` | 422 | 修改后的 Markdown 超过 512 KiB |
| `description_reference_invalid` | 422 | 保留 URI 格式、类型或目标非法 |
| `description_reference_limit_exceeded` | 422 | 内部引用超过 200 个 |
| `content_reference_query_invalid` | 400 | suggest/resolve 参数非法 |
| `attachment_not_found` | 404 | 附件不存在或不在当前授权范围 |
| `attachment_upload_incomplete` | 400 | multipart 缺文件、提前中断或声明大小不一致 |
| `attachment_remote_fetch_disabled` | 403 | 运维配置关闭远程图片转存 |
| `attachment_remote_url_invalid` | 422 | URL scheme/host/redirect 或 SSRF 校验失败 |
| `attachment_remote_fetch_failed` | 502 | 公网目标超时、返回非 2xx 或响应读取失败 |
| `attachment_too_large` | 413 | 单文件超过限制 |
| `attachment_type_not_allowed` | 415 | 文件类型、扩展名或 magic 不允许 |
| `attachment_image_invalid` | 422 | 图片无法解码或像素尺寸超限 |
| `attachment_target_type_unsupported` | 422 | 当前版本尚未注册该 attached resource 类型 |
| `attachment_quota_exceeded` | 409 | attached resource/workspace 数量或容量超限 |
| `attachment_in_use` | 409 | 当前 description 仍引用该附件 |
| `attachment_draft_creator_mismatch` | 403 | 非创建 actor 尝试绑定 draft |
| `attachment_state_invalid` | 409 | 当前状态不支持操作 |
| `attachment_content_gone` | 410 | metadata/history 存在但 blob 已 purge |
| `attachment_storage_unavailable` | 503 | filesystem/S3 临时不可用 |

客户端只展示稳定 code 对应的本地化文案；S3 SDK 原始错误、本地路径和内部 key 只写日志。

## 19. 审计、历史与可观测性

审计 action：

```text
attachment.add
attachment.rename
attachment.remove
```

audit target 是 attachment；payload 记录 attachment ID、`attached_to.type/id`、display name、media type、size、SHA-256、source type、远程 source host 和 actor，不记录完整 source URL、storage key、S3 endpoint、凭证或文件内容。首期 task Activity 如需显示附件操作，通过 payload 中的 attached target 归并，不把 action 名重新写死为 `task.*`。

description 的内部 URI 继续进入现有 `task.modify` before/after。任务历史渲染器识别附件/引用节点：

- retention 内的旧图片仍可鉴权加载。
- purge 后显示“附件内容已清理”，不能让整个历史项渲染失败。
- 不可用用户/任务引用显示保存时 label。

结构化指标/日志：

- 上传成功/失败次数、字节数、耗时、backend、media type。
- 远程抓取成功/失败次数、响应耗时、redirect 次数、拒绝原因和 source host；不记录 query、fragment 或完整 URL。
- 下载成功/失败次数、字节数、耗时、backend。
- janitor 扫描、删除、重试和孤儿数量。
- mention 事件数量、每事件 recipient 数量、空 audience 数量。
- 日志只记录 attachment ID 和哈希前缀，不记录原始文件内容；文件名按现有日志脱敏规则处理。

`/healthz` 继续表达进程存活；附件 backend 健康进入 readiness/启动检查，不因一次运行期 S3 短暂失败把整个进程判死。上传/下载在后端不可用时返回 503。

## 20. 导入、导出与兼容性

- 现有 `task.description` 字段仍是字符串，HTTP/MCP/CLI/Remote JSON 字段不改名。
- Taskwarrior 风格 JSON、XLSX task import 和 `xuanchu.task-bundle/v1` 不承载二进制。
- 同 workspace 内导入包含现有 `ref://attachment/{id}` 的 description 时，App 层按普通修改校验附件归属和权限。
- 跨 workspace 或跨部署导入遇到 attachment URI 时返回明确的 `description_reference_invalid`，不能静默生成坏图。
- 普通 user/task 内部引用在 import 时必须解析到目标 workspace 中的同一 UUID；本期不按 name/title 猜测重映射。
- 导出保留 Markdown URI，并在 export envelope 增加非阻断 `warnings`，列出二进制未包含的 attachment IDs。
- Web Console 从旧版本升级后，历史纯 Markdown 无迁移；新 extension 对普通链接保持原有行为。
- API 新增字段和 endpoint 均为向后兼容扩展；附件不写入持久 `task.JSONTask.Links`，不与既有 `task_links` 混用。

## 21. 依赖

Go：

```text
github.com/yuin/goldmark
github.com/aws/aws-sdk-go-v2
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/service/s3
golang.org/x/image/webp
```

Web：

```text
dompurify
```

继续使用当前锁定的 Tiptap v3.27.1 系列；不能只升级某个 Tiptap package。所有 Go 依赖必须在 `CGO_ENABLED=0` 下构建。

## 22. 实施分段

本文必须拆成四份 implementation plan，每份都产生可独立测试和审阅的交付物：

1. **通用附件基础与存储后端**：`attachments + attached target` metadata migration、target handler、BlobStore、filesystem、S3、safe remote fetch、配置、上传/下载/远程转存 API、janitor、权限和配额；首期注册 task target。
2. **task 附件跨入口**：在通用 attachment foundation 上接入任务附件面板、CLI/Remote、MCP metadata tools、审计、历史渲染和文档。
3. **富文本粘贴与图片节点**：DOMPurify、远程图片识别/转存/失败占位、paste pipeline、Tiptap attachment extension、draft binding、authenticated blob rendering、smoke coverage。
4. **用户/任务引用与 mention 事件**：Goldmark parser、suggest/resolve、Tiptap reference extension、`task.user_mentioned`、Notification audience、Hook/automation。

依赖顺序为 1 → 2/3，4 可在 1 完成后与 2/3 独立实施。每份 plan 内继续按 TDD 小步提交，最终由一轮全量回归统一验收。

## 23. 测试策略

### 23.1 Domain/App

- Goldmark 正确解析普通链接、user/task 引用、图片和附件链接。
- 非 canonical UUID、额外 query/fragment、未知类型、伪造 scheme 被拒绝。
- user mention 集合 diff 去重，label 修改不触发，删除后再次添加触发。
- 用户引用解析返回完整 `UserInfo`；未找到 fallback 不得用于新引用校验。
- target task 跨 workspace/project allowlist 被拒绝。
- draft 创建者、绑定、过期、active、deleted、purge 状态机。
- 当前 description 引用阻止附件删除。
- closed project、viewer、scope 缺失、tenant/admin acting 权限矩阵。
- 并发上传的 attached resource/workspace quota 最终提交不越限。
- 未注册 attached type 被拒绝；同一通用 repository 可按 task target 列表，未来增加 project handler 不需要迁移表结构。
- 远程抓取与普通上传进入同一 state/quota/validation 路径，不产生旁路 active attachment。

### 23.2 Storage/BlobStore

- SQLite 和 PostgreSQL migration、`attached_to_type/id` 组合索引、按资源/workspace 状态查询和配额统计。
- filesystem 原子写、权限、路径分片、失败清理、stream read/delete。
- S3 使用 fake HTTP S3 或 MinIO opt-in 覆盖 Put/Get/Delete、path-style、SSE header 和错误映射；测试不访问真实公网。
- safe fetch 覆盖公网成功、所有私网/loopback/link-local/CGNAT/保留地址、混合 DNS 结果、DNS rebinding、redirect 每跳重验、HTTPS 降级、超时、超长 body、错误 MIME 和图片像素炸弹。
- backend 切换后按 row backend 读取历史对象。
- janitor 只删除过期记录，blob 删除失败会重试且 metadata 不丢失。

### 23.3 HTTP/CLI/MCP

- multipart 上传、远程 URL 转存、提前断流、超限、MIME 冲突、图片炸弹、配额和错误码。
- content 鉴权、nosniff、Content-Disposition、ETag、不可枚举 403/404 边界。
- suggestion 的 user/task 权限与排序、resolve 的 user/task/attachment 批量权限、limit 和 unavailable 降级。
- CLI local/remote 上传下载字节一致，stdout/stderr 分离，`--json` 不混入进度。
- MCP 四个 tool 名使用下划线，schema golden 和 structuredContent 完整。
- `task.user_mentioned` Hook、Notification Rule、automation 三条链路各有集成测试。

### 23.4 Web

- 富文本粘贴保留白名单结构，删除 style/script/iframe；公网 remote image 成功转存，私网/认证/失败图片进入明确失败占位。
- data image 和 clipboard File 走上传，不持久化 data/blob URL。
- 同次粘贴重复 URL 只抓取一次；redirect、取消、重试、移除和明确保留普通外链行为可验证。
- 图片上传成功/失败/重试/取消和 draft 清理。
- PAT、acting token、OIDC cookie 三种模式均通过 authenticated fetch 加载图片。
- object URL 正确 revoke，请求按 attachment ID/SHA 去重。
- `@`/`#` 键盘、鼠标、IME、AbortController、不可用引用和源码往返。
- description 保存激活 draft；取消不把 draft 显示为 active。
- 附件面板读写权限、closed project、删除 in-use 错误和下载。
- task history 中旧图片 purge 后降级，不造成页面崩溃。

## 24. 验收标准

功能验收：

- 从 Word/飞书文档粘贴包含标题、列表、表格和链接的内容，保存后数据库仍是 Markdown，刷新后结构一致。
- 粘贴截图、拖入图片和文件选择都能上传；图片刷新后仍可显示，普通文件可下载且字节一致。
- 粘贴带公网 `<img src="https://...">` 的 HTML 会把图片抓取并转存为 draft attachment，保存后 Markdown 只含 `ref://attachment/{id}`；远程源失效后已保存图片仍可显示。
- localhost、私网、link-local、认证图片和超限响应不会被服务端抓取；失败占位不会静默丢图，也不会把远程图片 URL 当成持久 `<img>`。
- PAT、OIDC browser session、admin acting 三种 Console 身份都能按既有权限显示图片。
- task 作为首个 attached resource，可独立上传、重命名、下载、移除附件；description 正在引用时不能移除。
- filesystem 默认配置零额外基础设施可用；S3 配置可在 private bucket/MinIO 上完成同样操作。
- 输入 `@` 能选择成员，输入 `#` 能选择可读任务；保存后的 Markdown 使用稳定 UUID。
- 新增用户 mention 只生成一次 `task.user_mentioned`，Notification Rule 可用 `mentioned_users` 投递，Hook/automation 收到完整 `UserInfo`。
- CLI、HTTP、MCP、Remote 中的 description 仍是 Markdown 字符串，不出现 ProseMirror JSON、HTML、blob URL 或预签名 URL。

验证命令：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
git diff --check
```

S3 opt-in 验收另使用本地 MinIO 或 CI service container，不依赖共享公网账号。filesystem 和 S3 两种 backend 都必须跑相同的 App/HTTP contract suite。

## 25. 文档同步

实施完成时同步：

- `README.md`：description 富文本粘贴、内部引用、附件用法、文件限制。
- `ROADMAP.md`：v0.5.11 状态、通用 attachment foundation 与首期 task 接入边界。
- OpenAPI：multipart、附件、suggest/resolve、事件 audience。
- `docs/manual/notifications.md`：`task.user_mentioned` 和 `mentioned_users`。
- CLI manual：attachment 命令、remote upload/download。
- MCP skills：四个 `task_attachment_*` tool 和 mention 事件说明。
- 部署文档：本地附件目录备份、容器 volume、S3/MinIO 配置和 AWS credential chain。
- 安全文档：远程图片 egress 开关、SSRF 禁止网段、redirect/timeout/并发限制和失败降级。

## 26. 已确认决策

- description 的唯一持久化格式继续是 Markdown。
- mention 保存后产生语义事件，由 Notification Rule、Hook 和项目自动化消费，不直接绑定某个外部通道。
- 首期可选择的语义实体是用户和任务，协议预留扩展能力。
- 附件底层是 workspace 级通用资源，以 `attached_to_type + attached_to_id` 归属业务实体；首期只接入 task，不局限于 description 图片。
- 图片可以嵌入，其他允许类型以附件卡片和下载为主。
- 高风险主动内容、脚本、可执行文件和带宏 Office 文件不允许上传。
- 默认使用本地文件系统，S3 是可选配置项。
- 所有附件内容通过璇础鉴权代理读取，不使用公开 bucket 或持久预签名 URL。
