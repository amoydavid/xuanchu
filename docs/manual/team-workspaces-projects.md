---
title: "团队、Workspace 与 Project"
weight: 60
---

# 团队、Workspace 与 Project

Xuanchu 的企业边界由 workspace、project、user、membership 和 audit 组成。

如果你还不确定“当前 CLI/MCP 到底以哪个 user 身份运行”，先读 [身份与初始化](identity-and-initialization.md)。本页更偏团队对象和管理命令。

## 概念

- `workspace`：企业、团队或租户级隔离边界。
- `project`：workspace 内的真实项目。
- `user`：本地或服务端 actor。
- `member`：user 在 workspace 内的角色。
- `audit`：写操作审计记录。

同名 project 可以存在于不同 workspace。`dajee/agentapi` 和 `partner/agentapi` 是两个不同项目。

## User

```bash
xuanchu user list
xuanchu user add alice email:alice@example.test
xuanchu user use alice
xuanchu user info
```

本地模式会自动创建 `local` user。

本地 CLI 的 actor 来自 `user use` 设置的 active user。远程 CLI 和 HTTP MCP 的 actor 来自 token 绑定的 user，不受本机 `user use` 影响。

## Workspace

```bash
xuanchu workspace list
xuanchu workspace add dajee name:Dajee visibility:team
xuanchu workspace use dajee
xuanchu workspace info dajee
xuanchu workspace modify dajee description:"Dajee enterprise workspace"
xuanchu workspace archive old
```

一次性指定 workspace：

```bash
xuanchu --workspace dajee list
```

`--workspace` 只选择本次命令的 effective workspace，不会改变 actor，也不会突破 token 的 workspace scope。

## Member 与角色

```bash
xuanchu member list
xuanchu member add bob role:viewer
xuanchu member role bob member
```

角色：

| 角色 | 说明 |
|---|---|
| `viewer` | 读任务、报表、helper、成员列表，切换自己的 active context |
| `member` | 额外可写任务、import、管理 context |
| `admin` | 额外可管理 workspace metadata、成员、audit、UDA schema、Hook |
| `owner` | 额外可授予/降级 owner、归档 workspace |

当前没有 `member delete`。如需撤销写权限，可以把成员降为 `viewer`。

## 授权决策

本地 CLI 的 actor 来自 active user；远程 HTTP/MCP 的 actor 来自 token，或在 Agent token impersonation 下来自 `X-Xuanchu-As`。最终权限仍然是 membership role、token capability、workspace allowlist、project allowlist 的交集，服务端把这四项统一表达为 Authorization Decision，HTTP API、HTTP MCP 和远程 CLI 共用同一个决策入口。

- PAT / Agent token 只提供 capability 和 allowlist，不能放大 membership role。
- `tenant_access_token` 是 workspace 级机器 actor，不绑定用户、不读取 membership role；最终权限来自 tenant scope、workspace/project allowlist 和 tenant 工具/接口禁止清单。它可按 scope 管理 user、member、tenant token、workspace 设置，以及 hook、notification、reminder、task link、project annotation 等带 `created_by` / `actor` 的资源；这些字段会记录并输出为系统 actor。
- Agent token impersonation 不会提权：以目标用户的 membership role 为准。
- impersonation 必须显式 workspace：token 可见多个 workspace 且请求未指定 workspace 时返回 `workspace_required`。
- 目标用户不存在或非成员时统一返回 `membership_not_found`，避免身份枚举。

## Project

```bash
xuanchu --workspace dajee project add agentapi name:"AI Agent Platform"
xuanchu --workspace dajee project list
xuanchu --workspace dajee project info agentapi
xuanchu --workspace dajee project modify agentapi description:"Owns MCP work"
xuanchu --workspace dajee project archive agentapi
```

project 归档后不能被新任务引用，但已有任务仍可读取、完成和删除。

## Project 生命周期

project 有显式状态机：`planning` → `active` → `archived` / `cancelled`。

```bash
xuanchu project transition agentapi active
xuanchu project transition agentapi archived
```

- `planning`：新项目（含模板实例化）的初始状态，可正常读写任务。
- `active`：正式进行中。
- `archived`：等价于 `project archive`，不能被新任务引用。
- `cancelled`：项目取消，语义上与 archived 类似但保留取消原因。

状态流转记录到 audit 和项目 timeline。

## 项目模板

项目模板是 workspace 内的不可变项目初始化快照，用于把成熟项目的任务结构、循环任务、project config 和自动化规则复制到新项目。

- **Capture**：在 Web Console 项目 Header 的「更多操作 → 另存为模板」完成，服务端筛选候选任务 / Series / config / automation，支持分页与跨页保留已选。
- **config 策略**：每个 config 可选 `fixed`（固定当前值进快照）、`inherit`（不写 project config，沿用实例化时的 workspace/default）或 `prompt`（创建时必填/选填）。选择 automation 时其显式 config 依赖自动闭包加入。
- **版本化**：同标识重复保存会在同一 Template 下追加不可变 Snapshot 并把 current 指向最新；已归档模板不会静默复活。
- **实例化**：`/projects` 的「从模板创建」按 current Snapshot 的 `config_inputs` 生成 typed form，必填 prompt 必须显式填写，在单事务中创建 `planning` Project。

CLI / 远程 CLI 只提供列表与实例化：

```bash
xuanchu project template list
xuanchu project template instantiate <template-ref> <new-slug> name:<name> \
  --snapshot <uuid> --snapshot-hash <hash> --start-date 2026-09-01 \
  --input inputs.json
```

`--snapshot` / `--snapshot-hash` 必须来自最近一次 `project template list` 的 current Snapshot；若期间有新 Snapshot 追加，实例化返回 `project_template_snapshot_hash_mismatch`，需重新 list 确认。`--input` 提供 `config_inputs`（JSON 文件或 `-` 读 stdin，最大 1 MiB），只能包含 Snapshot 声明的 prompt key。新任务和 Series 使用全新身份，自动化规则创建后保持停用，delivery 不复制。

## Project 配置

project/shared config 现在分成 schema 定义和值写入两层：

```bash
xuanchu --workspace dajee config schema set agent.background type:string scopes:project
xuanchu --workspace dajee config schema set ads.roi_threshold type:number scopes:workspace,project default:1.8

xuanchu --workspace dajee config set ads.roi_threshold 2.0
xuanchu project config set agentapi agent.background "Owns Xuanchu MCP integration."
xuanchu project config get agentapi agent.background
xuanchu project config list agentapi
xuanchu project config unset agentapi agent.background
```

补充说明：

- `config schema` 只在当前 workspace 内生效，不会跨 workspace 共享。
- `project config get` 的读取顺序是：project 显式值 -> workspace 显式值 -> schema default。
- `config list` / `project config list` 只显示显式写入的值，不展开继承结果。
- 如果 schema 仍被 workspace/project 值引用，`config schema delete <key>` 会拒绝；要连值一起删除，需要显式加 `--purge`。
- 系统会自动预置 `agent.background`、`agent.constraints`、`agent.default_context`、`agent.handoff`、`context.default` 这些常用 project key 的 schema。

## Audit

```bash
xuanchu audit list
xuanchu audit list --limit 20 --json
xuanchu audit list --project agentapi
```

audit 记录写操作，例如 task、context、workspace、member、project、config、token、hook、manual replay 等。

远程 API 中读取 audit 需要 `audit:read` capability，并且 actor 角色需要 admin 或 owner。
