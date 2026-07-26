# 项目模板配置输入 Implementation Plan

> **执行要求：** 严格按 Red → Green → Refactor 推进。每个任务先运行新增测试并记录预期失败，再写最小实现；不要先铺完整实现后补测试。

**目标：** 实现 `xuanchu.project-template-snapshot/v2`，让模板作者把 project-scoped config 声明为 fixed、inherit、必填 prompt 或选填 prompt，并让 HTTP/Web/CLI/Remote/MCP 在同一 Snapshot ID/hash 下发现、填写、校验并原子写入这些配置；模板标识留空时默认来源 Project slug，同标识保存追加版本。

**设计依据：** `docs/superpowers/specs/2026-07-26-project-template-config-inputs-design.md`

**架构：** `internal/projecttemplate` 负责 v1/v2 持久协议与升级；`internal/app` 负责候选、Capture policy、表单 descriptor、输入白名单、当前 ConfigDefinition 校验和事务计划；Storage 继续只保存 `snapshot_json TEXT`。HTTP/Web 提供完整治理，CLI/Remote/MCP 继续只提供 list/current instantiate。

## 全局约束

- 新 Capture 默认写 v2；v1 Snapshot 原始 JSON/hash 不改。
- v1 `secret_input` 保持“输入优先、否则继承”语义；v2 prompt 必须走 `config_inputs`。
- prompt required 只能由本次显式输入满足，workspace/default 不算填写。
- prompt optional 留空不写 project config row。
- 无源 project row 的 project-scoped definition 默认 inherit，也可选 prompt，但不能 fixed。
- automation 依赖的 prompt 后端强制 required。
- Snapshot、响应、Preview、审计、日志、错误不得包含 prompt secret 值。
- 只要 Snapshot 含 config，实例化权限仍要求 config write，不能按本次是否留空降级。
- 不新增模板子表或数据库列；SQLite 继续使用纯 Go driver，必须通过零 CGO 验证。
- Web 复用/扩展 `ConfigValueControl`，遵守 `DESIGN.md`，不复制 typed config 控件。
- 所有对外新增 JSON 字段必须同步 OpenAPI、Remote、CLI、MCP 与文档。

## Task 1：Snapshot v2 严格 codec 与 v1 升级

**文件：**

- 修改 `internal/projecttemplate/model.go`
- 修改 `internal/projecttemplate/codec.go`
- 修改 `internal/projecttemplate/validate.go`
- 修改 `internal/projecttemplate/model_test.go`
- 修改 `internal/projecttemplate/codec_test.go`

### Red

新增测试覆盖：

- v2 literal、secret_copy、prompt required、prompt optional round-trip。
- v2 config key 重复拒绝。
- mode/value/ciphertext/prompt 非法组合拒绝。
- v2 unknown field、trailing JSON、未知 schema 拒绝。
- v1 literal/secret_copy/secret_input 解码到 current model 后语义保持。
- required true/false 改变 canonical hash。
- `EncodeV1` 产物与现有 golden/hash 不变。

运行：

```bash
go test ./internal/projecttemplate -run 'SnapshotV2|DecodeV1|ConfigBlueprint' -count=1
```

确认因 `SnapshotSchemaV2`、`EncodeV2` 或 prompt 字段不存在而失败。

### Green

- 定义独立 `SnapshotV2` / `ConfigBlueprintV2` / `ConfigPromptV2`。
- 定义 current internal `Snapshot`，让 App 不直接依赖某个持久版本。
- `Decode` 按 header 分派 v1/v2，严格解码后显式升级。
- 新增 `EncodeV2`；保留 `EncodeV1` 完全不变。
- current config 能区分 fixed literal、fixed secret、legacy secret resolution、prompt required/optional。

### Refactor / Gate

```bash
gofmt -w internal/projecttemplate
go test ./internal/projecttemplate -count=1
git diff --check
```

## Task 2：Config 候选扩展到 project-scoped definitions

**文件：**

- 修改 `internal/storage/config_repo.go`
- 修改 `internal/storage/config_repo_test.go`
- 修改 `internal/app/project_template_candidates.go`
- 修改 `internal/app/project_template_candidates_test.go`

### Red

先补测试：

- 有 project row 的 definition 返回 `has_project_value=true/can_fixed=true`。
- 只有 workspace/default/missing 的 project-scoped definition 也进入候选，返回 `can_fixed=false`，不返回值。
- 不允许 project scope 的 definition 不进入候选。
- secret 候选不泄漏值。
- `all|fixed_available|prompt_available|secret|non_secret` 稳定分页与 refs 精确重取。
- 旧 `literal|secret` filter 兼容。
- workspace 隔离和 `ORDER BY key ASC`。

运行聚焦测试，确认现有 repo 只查显式 row 导致失败。

### Green

- 增加 definition 驱动的 candidate query，LEFT JOIN project config / workspace config，只返回 source 状态。
- App DTO 增加 secret、has_project_value、effective_source、can_fixed。
- 不在 candidate DTO 放任何 value/default。

### Gate

```bash
go test ./internal/storage ./internal/app -run 'ConfigCandidate' -count=1
```

## Task 3：Capture policy 与 v2 Snapshot 生成

**文件：**

- 修改 `internal/app/project_template_capture.go`
- 修改 `internal/app/project_template_capture_test.go`
- 修改 `internal/app/project_template.go`

### Red

新增测试：

- policy 省略时，显式 project row 默认 fixed。
- 无 project row definition 可 prompt required/optional。
- 无 project row fixed 阻断。
- 未选 key、重复 policy、未知 strategy、fixed+required 阻断。
- prompt Snapshot 不包含源普通值或 secret。
- fixed 普通/secret 保持现状。
- automation 依赖 key 使用 prompt 时强制 required，伪造 optional 请求被拒绝。
- definition fingerprint 进入 source hash；policy 进入 Snapshot hash。
- Capture 默认写 schema v2。

运行：

```bash
go test ./internal/app -run 'ProjectTemplateCapture.*(Policy|Prompt|V2|Automation)' -count=1
```

### Green

- `CaptureInput` 增加 `ConfigPolicies`。
- source load 支持没有 project row 的 definition。
- source hash 覆盖所有选中 definition 当前 schema。
- 映射 fixed/inherit/prompt v2 blueprint 并调用 `EncodeV2`。
- Capture preview/view 对 prompt 只返回安全元数据。

### Gate

```bash
go test ./internal/projecttemplate ./internal/storage ./internal/app -count=1
```

## Task 4：实例化 descriptor、config_inputs 与事务

**文件：**

- 修改 `internal/app/project_template.go`
- 修改 `internal/app/project_template_instantiate.go`
- 修改 `internal/app/project_template_instantiate_test.go`

### Red

新增测试：

- Summary/Preview 生成 key/label/description/type/enum/required/secret/status descriptor，绝不返回值/default。
- required prompt 缏失、空、纯空白阻断，且 workspace/default 不能满足。
- optional prompt 缺失/空白不写 project row；填写后写 row。
- string/number/boolean/json/date/datetime/enum 复用当前 schema normalization。
- secret prompt 贯通且所有 JSON/audit/error 不泄漏。
- fixed/未知 key 输入拒绝；`config_inputs` 与 v1 `secret_inputs` 冲突拒绝。
- definition 删除、scope/type/enum/secret 漂移产生稳定 issue。
- optional 留空仍要求 config write。
- Preview 后 hash drift 阻断。
- 任一后续阶段失败，Project/config 一起回滚。
- v1 三种 mode 兼容回归。

### Green

- `InstantiateInput` 增加 `ConfigInputs`。
- 增加 descriptor/resolution DTO 与安全 MarshalJSON。
- 拆分 definition resolve、fixed/inherit/prompt planning、descriptor build。
- 只把成功 fixed/显式 prompt 放进 `plan.ConfigValues`。
- 复用现有 `validateProspectiveProjectConfigValue`，不在入口重复校验。

### Gate

```bash
go test ./internal/app -run 'ProjectTemplateInstantiate' -count=1
go test ./internal/app -count=1
```

## Task 5：HTTP 与 OpenAPI

**文件：**

- 修改 `internal/httpapi/project_templates.go`
- 修改 `internal/httpapi/project_templates_test.go`
- 修改 `internal/httpapi/server_test.go`
- 修改相关 OpenAPI schema 定义文件

### Red

- Capture body 接受 `config_policies`。
- Candidate 响应包含安全字段。
- List/detail/preview 包含 descriptor。
- Instantiate 接受 `config_inputs`。
- required/enum/openapi examples 正确。
- HTTP 响应与 error body 不泄漏输入值。

### Green / Gate

```bash
go test ./internal/httpapi -run 'ProjectTemplate|OpenAPI' -count=1
go test ./internal/httpapi -count=1
```

## Task 6：CLI、Remote、MCP

**文件：**

- 修改 `internal/cli/project_template.go`
- 修改 `internal/cli/project_template_test.go`
- 修改 `internal/remote/project_template.go`
- 修改 `internal/remote/project_template_test.go`
- 修改 `internal/mcpserver/tools_project_template.go`
- 修改 `internal/mcpserver/tools_project_template_test.go`
- 修改 MCP schema/list-tools golden 与 `docs/skills` 中对应模板文档

### Red

- CLI/Remote input JSON 接收 `config_inputs`。
- list JSON/human 能发现 required/optional prompt。
- stdin secret 不出现在 stdout/stderr。
- Remote typed body 精确传递 descriptor/input。
- MCP list result/schema 暴露 descriptor，instantiate schema 接收 config_inputs。
- MCP text/structuredContent/error 不回显 secret。
- MCP tool 数量与命名不变。

### Green / Gate

```bash
go test ./internal/remote ./internal/cli ./internal/mcpserver -run 'ProjectTemplate' -count=1
```

## Task 7：Web API 类型与 Capture 策略 UI

**文件：**

- 修改 `web/src/features/workspace/project-templates/api/project-template-api.ts`
- 修改对应 API test/type-test
- 修改 `capture/project-template-capture-wizard.tsx`
- 修改 `capture/candidate-picker.tsx`
- 修改 `capture/selected-items-sheet.tsx`
- 修改 Capture wizard tests

### Red

- API body 序列化 config_policies。
- prompt-only definition 默认 prompt，fixed 不可选。
- 有 project value 默认 fixed。
- prompt required 开关正确。
- automation 依赖 prompt 强制 required且不可取消。
- Preview 摘要区分 fixed/required/optional。
- secret/普通源值均不进入 DOM 或请求外字段。

### Green / Gate

```bash
pnpm --dir web test -- --run src/features/workspace/project-templates/capture
pnpm --dir web typecheck
```

## Task 8：Web typed config form 与 secret 生命周期

**文件：**

- 修改 `web/src/features/workspace/config/config-value-control.tsx`
- 修改其测试
- 修改 `instantiate/project-template-instantiate-wizard.tsx`
- 修改实例化 wizard tests

### Red

- `ConfigValueControl` 支持 allowEmpty、boolean 未选择态、secret 禁止 reveal。
- 项目与配置步骤按 descriptor 渲染 enum/boolean/number/json/date/datetime/secret/string。
- optional 空值不进 config_inputs；required 空值前端提示，服务端仍最终裁决。
- Preview 请求含 config_inputs；响应不回填值。
- 第 3 步不重复显示 v2 prompt secret resolution。
- 确认页只显示数量/已填写状态。
- close/success/hash drift 清空 secret；同一会话返回保留。
- 390px 无横向滚动，label/error ARIA 完整。

### Green / Gate

```bash
pnpm --dir web test -- --run src/features/workspace/config/config-value-control.test.tsx
pnpm --dir web test -- --run src/features/workspace/project-templates/instantiate
pnpm --dir web typecheck
pnpm --dir web lint
```

## Task 9：文档、迁移回归与完整收尾

**文件：**

- 更新 `README.md`
- 更新 `ROADMAP.md`
- 更新本 spec 状态与本 plan checkbox/实施记录
- 更新相关 `docs/skills/*/SKILL.md`
- 必要时新增旧 SQLite fixture migration test

### 文档要求

- README 示例从仅 `secret_inputs` 扩展到 v2 `config_inputs`，保留 v1 兼容说明。
- ROADMAP 增加 v0.6.3 并锁定产品边界。
- 说明旧 binary 不支持 v2 的 downgrade 行为。
- Agent Skill 明确 list descriptor → 收集输入 → 固定 ID/hash instantiate。

### 完整验证

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web test
pnpm --dir web run smoke:project-template
```

完成前逐条对照 spec §19 十二项验收标准，记录每项的代码、测试或命令证据；任何缺失不得标记完成。

## 实施记录（2026-07-26）

- Task 1 已完成：新增独立 Snapshot v2 持久结构、strict codec、v1 显式升级和 canonical hash 回归；新 Capture 默认写 v2，v1 原始 JSON/hash 不变。
- Task 2-3 已完成：候选从 project-scoped ConfigDefinition 出发，返回 fixed/inherit/prompt 安全能力字段；Capture 支持 `config_policies`，prompt/inherit 不保存来源值，automation 依赖 prompt 强制 required，definition/policy 进入 source hash。
- Task 4 已完成：Summary/Preview 返回 typed descriptor，Instantiate 统一接收 `config_inputs`；required 仅接受本次显式输入，optional 空白不写 row，所有类型复用现有 config normalization，事务与 secret 边界保持不变。
- Task 5 已完成：HTTP body、response 和运行时 OpenAPI 已同步 policy、candidate、descriptor、resolution 与 `config_inputs`，未新增 route。
- Task 6 已完成：CLI、Remote、MCP 同步 descriptor 和 `config_inputs`；MCP 仍只注册两个 project template tool，schema golden 已更新。
- Task 7-8 已完成：Web Capture 支持 fixed/inherit/prompt/required 与 automation 锁定；Instantiate 在“项目与配置”步骤复用 typed ConfigValueControl，optional 空值不提交，secret 禁止 reveal 并在失败/hash drift/关闭时按规则清理。
- Task 9 已完成：文档已同步 README、ROADMAP 和 `xuanchu-govern-projects` Skill；SQLite、PostgreSQL、零 CGO、Go/Web 全量 gate 均通过。

新增边界回归覆盖：required 缺失/空/纯空白与 workspace/default 不可替代、全 typed config/enum 归一化、definition/scope 漂移 descriptor、v2 prompt 事务回滚、optional 空白仍要求 config write，以及 Web automation 依赖 prompt 的 required 锁定。

最终验证：

```text
go test ./...                                                        PASS
CGO_ENABLED=0 go test ./...                                          PASS
CGO_ENABLED=0 go build ./cmd/xuanchu                                 PASS
go vet ./...                                                         PASS
git diff --check                                                     PASS
pnpm --dir web typecheck                                             PASS
pnpm --dir web lint                                                  PASS
pnpm --dir web build                                                 PASS
pnpm --dir web test                                                  PASS (144 files / 828 tests)
pnpm --dir web run smoke:project-template                            PASS
XUANCHU_TEST_DB_URL=... XUANCHU_E2E_POSTGRES_ADMIN_URL=... \
  go test ./internal/storage ./tests/integration -run Postgres -count=1
                                                                      PASS
```

PostgreSQL gate 使用临时 PostgreSQL 15 实例，覆盖 storage 全套 Postgres 测试和 HTTP/MCP project-template current workflow。该 workflow 明确验证 v2 普通/secret prompt descriptor、`config_inputs`、Snapshot 不保存 prompt 来源值、typed project config 写入和 stale hash 阻断；验证后临时实例已移除。

Playwright smoke 使用生产 Web 构建和真实 `xuanchu server`，新增覆盖：无源 row 的 enum required prompt 与 boolean optional prompt、secret prompt、Capture 策略与 automation required 锁定、创建页必填阻断、Radix typed 控件交互、`config_inputs` 写入、optional 留空不落 row、secret 覆盖来源值且创建后不残留在页面。脚本完成后服务退出且端口释放。

## 变更 Task 10：inherit 与统一“创建时无需填写”

### Red

- Snapshot v2 codec 增加 `inherit` 合法组合及非法 value/ciphertext/prompt 组合测试。
- App Capture 测试证明无显式 project row 时默认/显式 inherit，且 fixed 继续拒绝。
- Instantiate 测试证明 inherit 不写 project row、能读取当前 workspace/default、missing 不阻断普通项目、Automation 缺有效值时阻断。
- Web 测试证明 `can_fixed=false` 时仍显示“创建时无需填写”，默认发送 inherit。

### Green / Gate

- 扩展 typed Snapshot v2、Capture policy、HTTP OpenAPI enum 和安全 view。
- Web 统一显示两个产品选择，内部按来源 row 使用 fixed/inherit。
- 更新摘要、详情、README/ROADMAP/Skill 文档。

## 变更 Task 11：模板标识默认与同 key 追加版本

### Red

- App/HTTP 测试证明 key 空白时使用来源 Project slug。
- App/HTTP 并发与行为测试证明同 active key 复用 Template、更新元数据并追加 current Snapshot，不返回 key conflict。
- Web 测试证明 key 可留空，保存后按响应最终 key 失效缓存。
- Playwright 真实 E2E 覆盖默认 slug、第二次保存、唯一列表项和 current v2。

### Green / Gate

- 将 Create 语义收敛为原子 save-or-version；继续复用唯一约束、Template lock 和 AppendSnapshotLocked。
- archived 同 key 继续明确阻断，不静默复活。
- 完整运行原 Task 9 全量 gate，并提交中文 Git commit。

### 调整实施记录（2026-07-26）

- Task 10 已完成：Snapshot v2、Capture、Instantiate、HTTP/OpenAPI 与 Web 已统一支持 inherit；没有来源 project row 的 config 默认仍提供“创建时无需填写”，实例化不创建 project config row。
- Task 11 已完成：模板标识留空时由 App 使用来源 Project slug；同 active key 保存复用 Template、更新元数据并追加 current Snapshot，完全相同内容幂等成功，archived key 明确阻断。
- SQLite 同 key 并发保存测试连续运行 10 次通过；真实 Playwright smoke 覆盖默认 slug、同标识 v1→v2、唯一列表项、inherit 实例化和无 project config row。
- 最终全量 gate 通过：Go 普通/零 CGO 测试、零 CGO 构建、vet、Web typecheck/lint/build、144 个文件共 828 个 Web 测试，以及项目模板 Playwright smoke。
