# AGENTS.md

本文件面向在本仓库内工作的编码代理、自动化助手和协作者，定义项目背景、工程约束、开发流程和交付标准。目标不是限制发挥，而是帮助大家在同一套假设下工作，减少返工和风格漂移。

## 0. 开发原则

以第一性原理从原始需求和问题本质出发，不从惯例或模板出发。

- 不要假设我清楚自己想要什么。动机或目标不清晰时，停下来讨论。
- 目标清晰但路径不是最短的，直接告诉我并建议更好的办法。
- 遇到问题追根因，不打补丁。每个决策都要能回答“为什么”。
- 输出说重点，砍掉一切不改变决策的信息。

偷懒是第一生产力，解决一些问题时，有成熟的第三方库就用，不要重复造轮子。

重要，永远使用中文为主要语言撰写文档和注释。

## 1. 项目目标

`taskg` 的最终目标见 [README.md](/Users/mac/code/projects/dajee/task/README.md) 和 [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)：

- 单一二进制，同时承担本地 CLI、远程 CLI 客户端、HTTP/JSON API 服务端、MCP Server。
- 使用纯 Go SQLite 方案，保持零 CGO。
- 支持多用户、多 workspace、行级隔离。
- 兼容 Taskwarrior 的核心命令名、JSON 数据格式与 urgency 公式。

开发要点

- 在开始任何新的 milestone 或跨越当前范围的特性前，先更新 spec，再写 implementation plan。

## 2. 当前技术栈

- Go: `1.25`
- CLI: `github.com/spf13/cobra`
- ORM: `gorm.io/gorm`
- SQLite driver: `github.com/glebarez/sqlite`
- UUID: `github.com/google/uuid`

重要约束：

- 本仓库当前使用的是 `GORM + github.com/glebarez/sqlite`。
- 这是为了保持纯 Go、零 CGO。
- 不要引入 `gorm.io/driver/sqlite`。
- 不要直接引入 `github.com/mattn/go-sqlite3`。
- 每次改动后都必须继续满足 `CGO_ENABLED=0` 的测试和构建要求。

## 3. 目录与分层约束

当前代码结构：

- `cmd/taskg`
  - 二进制入口。
- `internal/cli`
  - Cobra 命令、参数路由、CLI 输出。
- `internal/app`
  - 用例编排、service、目标解析、时钟注入。
- `internal/task`
  - 任务领域模型、JSON DTO、生命周期规则。
- `internal/query`
  - 查询参数、日期解析、后续 AST 和表达式引擎入口。
- `internal/config`
  - 本地配置和路径解析。
- `internal/storage/sqlite`
  - GORM model、SQLite 打开、仓储。
- `internal/render`
  - human/JSON 输出渲染。
- `tests/integration`
  - CLI 黑盒集成测试。

修改时请遵守这些边界：

- `internal/cli` 不要直接实现业务规则。
- `internal/app` 负责拼装 repo 和 domain 行为，不要把 SQL/GORM 细节拉进来。
- `internal/task` 不依赖 Cobra、GORM 或 CLI 输出。
- `internal/storage/sqlite` 不负责参数解释和 CLI 行为。
- 新增 HTTP/MCP 时必须复用 `internal/app`，不要复制业务逻辑。

## 4. 开发原则

- 优先延续现有结构，不要为“优雅”重写仓库。
- 小步修改，尽量让每个提交都可测试、可解释、可回退。
- 先补测试，再补实现，至少保持红绿验证思路。
- 优先写可组合的 app/service 和 domain 逻辑，再挂到 CLI/API/MCP。
- 所有命令都要尽量脚本友好：
  - stdout 只放结果。
  - stderr 放错误。
  - `--json` 输出必须稳定。

## 5. 规格与计划流程

这个仓库已经采用 spec/plan 驱动流程，后续请继续沿用：

1. 产品方向或 milestone 拆解先落到 [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)。
2. 新 milestone 开始前，先写中文 spec：
   - 路径：`docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`
3. spec 通过后，再写 implementation plan：
   - 路径：`docs/superpowers/plans/YYYY-MM-DD-<topic>-implementation.md`
4. plan 应拆成可执行的小任务，包含测试命令与验收标准。
5. milestone 完成后更新：
   - `README.md`
   - `ROADMAP.md`
   - 相关 spec / plan / docs

如果工作内容只是当前 milestone 内的小修复，不一定要新写 spec；但如果你在扩大范围、调整边界、改动里程碑定义，就应该先更新 spec 或 roadmap。

## 6. 测试与验证

任何声称“完成”“通过”“可合并”的改动前，都要先跑验证。当前项目至少需要：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
```

如果改动影响 CLI 行为，重点关注：

- `tests/integration/cli_test.go`
- stdout/stderr 分离
- `--json` 输出
- 数字 working-set ID 与 UUID 的行为一致性

如果改动影响 SQLite 或持久化层，额外关注：

- `internal/storage/sqlite/db_test.go`
- `internal/storage/sqlite/task_repo_test.go`
- 不要破坏 `CGO_ENABLED=0`

如果没有运行验证，不要在结论里说“已完成”或“测试通过”。

## 7. Git 与提交规范

- 除非用户明确要求，否则不要改写历史。
- 不要 `git reset --hard`。
- 不要 `git checkout -- <file>` 去覆盖用户改动。
- 不要随意删除未跟踪文件，除非确认它是你刚生成的产物。
- 提交信息尽量简短清晰，延续当前风格：
  - `feat: ...`
  - `fix: ...`
  - `docs: ...`
  - `chore: ...`
- 使用中文提交信息

提交前先确认：

- 只包含和当前任务相关的文件。
- 文档变更和代码变更逻辑一致。
- 路线图、README、spec 是否需要同步。

## 8. SQLite 与数据层硬约束

- 使用 `github.com/glebarez/sqlite` 作为 GORM dialector。
- 所有数据库访问都应通过 `internal/storage/sqlite` 聚合。
- M0/M1 阶段可以继续用 GORM，但需要保持查询逻辑清晰，不要把复杂 filter 直接堆成字符串拼接。
- 面向 M1 的查询能力，应优先设计成：
  - AST 或结构化查询表示
  - 安全的参数绑定
  - 可复用到 CLI、后续 HTTP、后续 MCP

后续如果确实需要在某些热路径上绕开 GORM，也必须先有明确理由，并保持对现有测试和 app service 的兼容。

## 9. 文档同步规则

以下情况必须同步更新文档：

- 产品边界变化：更新 `ROADMAP.md`
- 里程碑进入新阶段：更新 `ROADMAP.md`
- 用户可见命令、默认值、数据库路径变化：更新 `README.md`
- 设计决策或范围变化：更新对应 spec
- 实现步骤变化：更新对应 plan

不要让文档和代码长期分叉。

## 10. 当前已知项目习惯

- 本地数据库默认路径：`~/.local/share/taskg/taskg.db`
- 全局 flag：`--db`、`--data-dir`、`--json`、`--no-color`
- 当前根命令支持两种入口模式：
  - `taskg <subcommand> ...`
  - `taskg <target> <action> ...`
- 集成测试会临时构建 `./cmd/taskg` 二进制运行。
- 仓库中可能存在本地构建产物 `taskg`，处理前先确认是否是临时文件。

### 用户信息输出规范

所有对外输出（HTTP API、CLI `--json`、MCP tool 响应、Remote Client）中涉及用户身份的字段，必须使用 `task.UserInfo` 统一结构体，包含完整的用户信息：

```go
type UserInfo struct {
    ID          string
    Name        string
    Email       *string
    ExternalIDs []ExternalIDInfo
}
```

**规则：**

- 凡是 JSON 输出中出现用户引用的地方（`created_by`、`actor`、`user` 等），必须是 `UserInfo` 对象（`{"id":"...", "name":"...", "email":"...", "external_ids":[...]}`），不允许只输出裸 UUID。
- App 层 view struct 中引用用户时使用 `task.UserInfo`（或 `*task.UserInfo` 表示可选）。
- Storage 层返回原始 UUID 字符串，App 层通过 `resolveUserInfos(ids []string) (map[string]UserInfo, error)` 批量解析为完整 `UserInfo`。未找到的用户 fallback 为 `{ID: id, Name: id}`。
- HTTP/CLI/MCP/Remote 输出层使用 `task.UserInfoToJSON()` 或 `userInfoToJSONMap()` 进行序列化，统一 JSON 格式。
- 新增任何涉及用户身份的输出字段时，必须遵循此规范，不要退化为裸 UUID 字符串。

**当前已覆盖的字段：**

| View Struct | 字段 | 旧格式 | 新格式 |
|---|---|---|---|
| `TaskLinkInfo.CreatedBy` | `UserInfo` | `string` | `{"id","name","email","external_ids"}` |
| `ProjectAnnotationInfo.CreatedBy` | `UserInfo` | `string` | 同上 |
| `TimelineEntry.CreatedBy` | `UserInfo` | `string` | 同上 |
| `WorkspaceView.CreatedBy` | `*UserInfo` | `*string` | 同上 |
| `HookDeliveryView.Actor` | `UserInfo` | `string` | 同上 |
| `TokenView.User` | `UserInfo` | `string` | 同上 |
| `AuditLogView.Actor` | `*UserInfo` | `*string`+`string` | 同上 |
| `AuditLogView.DelegatorUser` | `*UserInfo` | `*string` | 同上 |

## 11. 对后续代理的建议

- 从 [README.md](/Users/mac/code/projects/dajee/task/README.md)、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、以及当前 milestone 的 spec 开始读上下文。
- 改动前先看对应层的测试文件，理解当前行为边界。
- 当你发现"现在能改，但会把后续 HTTP/MCP/多 workspace 做死"的实现方式时，优先选择对未来更稳的边界。
- 如果你需要新增一套跨层能力，先问自己：
  - 这是不是应该在 `app` 层？
  - 这会不会未来被 CLI、HTTP、MCP 共用？
  - 这会不会破坏 `CGO_ENABLED=0`？

### MCP Tool 命名规范

所有 MCP tool name 必须使用下划线 `_` 分隔，不使用点号 `.`。

- 正确：`task_add`、`project_annotate`、`workspace_list`、`config_get`、`user_get`、`project_get_current`
- 错误：`task.add`、`project.annotate`、`workspace.list`、`config.get`、`user_info`、`project_current`

命名格式：`{资源}_{动作}`，如 `task_query`、`user_get`、`member_add`。对于资源下的子资源，使用 `task_link_add`、`task_link_remove`、`project_list_annotations`、`project_list_timeline` 等格式。读操作（列出子资源）统一用 `list` 前缀，写操作（添加/删除）统一用动词。

## 12. 禁止事项

- 不要引入需要 CGO 的 SQLite 实现。
- 不要在 `internal/cli` 里写大段业务逻辑。
- 不要跳过测试却声称改动已完成。
- 不要在没有更新文档的情况下悄悄扩 milestone 范围。
- 不要把“临时兼容”变成默认长期方案，除非文档明确记录。
