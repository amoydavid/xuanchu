---
title: "命令速查"
weight: 200
---

# 命令速查

## 全局参数

```bash
xuanchu --db ./xuanchu.db list
xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu" list
xuanchu --data-dir ./data list
xuanchu --json list
xuanchu --no-color list
xuanchu --no-context list
xuanchu --workspace dajee list
xuanchu --project agentapi list
xuanchu --project-id <uuid> list
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" list
```

## 任务

```bash
xuanchu add "Description" project:<project> +tag due:tomorrow
xuanchu list [filters...]
xuanchu next [filters...]
xuanchu all [filters...]
xuanchu completed [filters...]
xuanchu deleted [filters...]
xuanchu waiting [filters...]
xuanchu active [filters...]
xuanchu ready [filters...]
xuanchu overdue [filters...]
xuanchu blocked [filters...]
xuanchu blocking [filters...]
xuanchu info <target>
xuanchu <target> modify [modifications...]
xuanchu <target> done
xuanchu <target> delete
xuanchu start <target>
xuanchu stop <target>
xuanchu annotate <target> <description...>
xuanchu denotate <target> <annotation-id>
xuanchu append <target> <text...>
xuanchu prepend <target> <text...>
xuanchu edit <target>

xuanchu link add <task-ref> --type <type> --url <url> [--title <title>]
xuanchu link list <task-ref>
xuanchu link remove <task-ref> --link-id <link-id>
```

本地 CLI 的 `<target>` / `<task-ref>` 可以是 working-set ID、UUID、UUID 前缀或 `task_slug`。HTTP API 和 MCP tool 只接受 UUID 或 `task_slug`。

## 查询、报表、helper

```bash
xuanchu urgency <target>
xuanchu _urgency <target>
xuanchu _get [expr...]
xuanchu _ids [filters...]
xuanchu _uuids [filters...]
xuanchu _projects [--all]
xuanchu _tags
xuanchu _udas
xuanchu _unique <attr> [filters...]
xuanchu _show [key...]
xuanchu _version
xuanchu calc <expression>
```

## 导入导出

```bash
xuanchu export
xuanchu import [file]
```

## 配置与 context

```bash
xuanchu show
xuanchu config get <key>
xuanchu config set <key> <value>
xuanchu config unset <key>
xuanchu config list
xuanchu config import-taskrc <path> [--dry-run] [--json]

xuanchu context define <name> <filter...>
xuanchu context use <name>
xuanchu context none
xuanchu context show
xuanchu context list
xuanchu context delete <name>
```

## User / Workspace / Member / Audit

```bash
xuanchu user list
xuanchu user add <name> [email:<email>]
xuanchu user use <name|email|uuid>
xuanchu user info [name|email|uuid]

xuanchu workspace list [--all]
xuanchu workspace add <slug> [name:<name>] [description:<text>] [visibility:private|team|public]
xuanchu workspace use <slug|uuid>
xuanchu workspace info [slug|uuid]
xuanchu workspace modify <slug|uuid> [name:<name>] [description:<text>] [visibility:private|team|public]
xuanchu workspace archive <slug|uuid>

xuanchu member list
xuanchu member add <user> [role:<role>]
xuanchu member role <user> <owner|admin|member|viewer>

xuanchu audit list [--project <project>] [--limit <n>]
```

## Project

```bash
xuanchu project list [--all]
xuanchu project add <slug> name:<name> [description:<text>]
xuanchu project info <slug|uuid>
xuanchu project modify <slug|uuid> [name:<name>] [description:<text>]
xuanchu project archive <slug|uuid>

xuanchu project config get <project> <key>
xuanchu project config set <project> <key> <value>
xuanchu project config unset <project> <key>
xuanchu project config list <project>
```

## Config Schema

```bash
xuanchu config schema list
xuanchu config schema get <key>
xuanchu config schema set <key> type:<type> scopes:<workspace|project|workspace,project> [label:<text>] [description:<text>] [values:<csv>] [default:<value>] [required:true|false] [secret:true|false]
xuanchu config schema delete <key> [--purge]
```

说明：

- `config schema set` 定义 shared config key 的类型、允许作用域和默认值。
- `config set/get/unset/list` 操作 workspace scope 的显式值。
- `project config set/get/unset/list` 操作 project scope 的显式值。
- `config schema delete <key>` 默认会在 key 仍被引用时拒绝；`--purge` 会同时删除该 workspace 下的所有对应值。

## Server / Token / MCP

```bash
xuanchu server --listen :8080

xuanchu token create <name> [--type pat|agent] [--scope <scope>] [--user <name|email|uuid>] [--workspace-id <uuid>] [--project <slug>] [--project-id <uuid>] [--expires-in 720h]
xuanchu token list [--all]
xuanchu token modify <id|prefix> [--name NAME] [--scope SCOPE...] [--expires-in SECONDS]
xuanchu token revoke <id|prefix>

xuanchu scope list
xuanchu scope ls

xuanchu mcp stdio
```

## Hook

```bash
xuanchu hook list [--project <slug>]
xuanchu hook add <name> --event <event> --url <url> [--scope workspace|project] [--project <slug>] [--secret <secret>] [--secret-stdin] [--secret-file <path>] [--timeout <seconds>] [--max-attempts <n>]
xuanchu hook info <hook-id>
xuanchu hook modify <hook-id> [--name <name>] [--event <event>] [--url <url>] [--secret-stdin] [--secret-file <path>] [--timeout <seconds>] [--max-attempts <n>]
xuanchu hook enable <hook-id>
xuanchu hook disable <hook-id>
xuanchu hook delete <hook-id>
xuanchu hook deliveries <hook-id> [--status queued|delivering|retry_wait|succeeded|dead_lettered|disabled_skipped] [--limit <n>]
xuanchu hook replay <delivery-id>
```

## Notification / Reminder

```bash
xuanchu notification sink list [--all]
xuanchu notification sink add <name> --type webhook|http_template --url <url> [--endpoint-mode static_url|template|config_value] [--url-template <template>] [--config-key <key>] [--allowed-host <host>] [--header Name=Value] [--body-template <json>] [--body-template-file <path>] [--body-content-type <type>] [--secret-ref alias=config.key] [--secret <secret>] [--timeout <seconds>] [--max-attempts <n>]
xuanchu notification sink info <sink-id>
xuanchu notification sink modify <sink-id> [--name <name>] [--type webhook|http_template] [--url <url>] [--url-template <template>] [--config-key <key>] [--allowed-host <host>] [--header Name=Value] [--body-template <json>] [--body-template-file <path>] [--body-content-type <type>] [--secret-ref alias=config.key] [--secret <secret>] [--timeout <seconds>] [--max-attempts <n>]
xuanchu notification sink enable <sink-id>
xuanchu notification sink disable <sink-id>
xuanchu notification sink delete <sink-id>

xuanchu reminder rule list [--project <slug>] [--all]
xuanchu reminder rule add <name> --schedule daily@HH:MM --filter <task-filter> [--repeat once|every:<duration>] --audience assignees|explicit_users|assignees_and_explicit_users --sink <sink-id-or-name> [--recipient <user-ref>] [--project <slug>]
xuanchu reminder rule add <name> --trigger due_before|overdue [--offset <duration>] [--after <duration>] [--repeat once|every:<duration>] --audience assignees|explicit_users|assignees_and_explicit_users --sink <sink-id-or-name> [--recipient <user-ref>] [--project <slug>]
xuanchu reminder rule info <rule-id>
xuanchu reminder rule modify <rule-id> [--name <name>] [--schedule daily@HH:MM] [--filter <task-filter>] [--trigger due_before|overdue] [--offset <duration>] [--after <duration>] [--repeat once|every:<duration>] [--audience assignees|explicit_users|assignees_and_explicit_users] [--sink <sink-id-or-name>] [--recipient <user-ref>] [--project <slug>]
xuanchu reminder rule enable <rule-id>
xuanchu reminder rule disable <rule-id>
xuanchu reminder rule delete <rule-id>

xuanchu notification delivery list [--status queued|retry_wait|delivering|succeeded|dead_lettered|disabled_skipped] [--sink <sink-id>] [--limit <n>]
xuanchu notification delivery info <delivery-id>
xuanchu notification delivery replay <delivery-id>
```
