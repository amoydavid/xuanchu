---
title: "命令速查"
weight: 200
---

# 命令速查

## 全局参数

```bash
taskg --db ./taskg.db list
taskg --db-url "postgres://user:pass@localhost:5432/taskg" list
taskg --data-dir ./data list
taskg --json list
taskg --no-color list
taskg --no-context list
taskg --workspace dajee list
taskg --project ai-agent-platform list
taskg --project-id <uuid> list
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" list
```

## 任务

```bash
taskg add "Description" project:<project> +tag due:tomorrow
taskg list [filters...]
taskg next [filters...]
taskg all [filters...]
taskg completed [filters...]
taskg deleted [filters...]
taskg waiting [filters...]
taskg active [filters...]
taskg ready [filters...]
taskg overdue [filters...]
taskg blocked [filters...]
taskg blocking [filters...]
taskg info <target>
taskg <target> modify [modifications...]
taskg <target> done
taskg <target> delete
taskg start <target>
taskg stop <target>
taskg annotate <target> <description...>
taskg denotate <target> <index>
taskg append <target> <text...>
taskg prepend <target> <text...>
taskg edit <target>
```

## 查询、报表、helper

```bash
taskg urgency <target>
taskg _urgency <target>
taskg _get [expr...]
taskg _ids [filters...]
taskg _uuids [filters...]
taskg _projects [--all]
taskg _tags
taskg _udas
taskg _unique <attr> [filters...]
taskg _show [key...]
taskg _version
taskg calc <expression>
```

## 导入导出

```bash
taskg export
taskg import [file]
```

## 配置与 context

```bash
taskg show
taskg config get <key>
taskg config set <key> <value>
taskg config unset <key>
taskg config list
taskg config import-taskrc <path> [--dry-run] [--json]

taskg context define <name> <filter...>
taskg context use <name>
taskg context none
taskg context show
taskg context list
taskg context delete <name>
```

## User / Workspace / Member / Audit

```bash
taskg user list
taskg user add <name> [email:<email>]
taskg user use <name|email|uuid>
taskg user info [name|email|uuid]

taskg workspace list [--all]
taskg workspace add <slug> [name:<name>] [description:<text>] [visibility:private|team|public]
taskg workspace use <slug|uuid>
taskg workspace info [slug|uuid]
taskg workspace modify <slug|uuid> [name:<name>] [description:<text>] [visibility:private|team|public]
taskg workspace archive <slug|uuid>

taskg member list
taskg member add <user> [role:<role>]
taskg member role <user> <owner|admin|member|viewer>

taskg audit list [--project <project>] [--limit <n>]
```

## Project

```bash
taskg project list [--all]
taskg project add <slug> name:<name> [description:<text>]
taskg project info <slug|uuid>
taskg project modify <slug|uuid> [name:<name>] [description:<text>]
taskg project archive <slug|uuid>

taskg project config get <project> <key>
taskg project config set <project> <key> <value>
taskg project config unset <project> <key>
taskg project config list <project>
```

## Server / Token / MCP

```bash
taskg server --listen :8080

taskg token create <name> [--type pat|agent] [--scope <scope>] [--workspace-id <uuid>] [--project <slug>] [--project-id <uuid>] [--expires-in 720h]
taskg token list [--all]
taskg token modify <id|prefix> [--name NAME] [--scope SCOPE...] [--expires-in SECONDS]
taskg token revoke <id|prefix>

taskg scope list
taskg scope ls

taskg mcp stdio
```

## Hook

```bash
taskg hook list [--project <slug>]
taskg hook add <name> --event <event> --url <url> [--scope workspace|project] [--project <slug>] [--secret <secret>] [--secret-stdin] [--secret-file <path>] [--timeout <seconds>] [--max-attempts <n>]
taskg hook info <hook-id>
taskg hook modify <hook-id> [--name <name>] [--event <event>] [--url <url>] [--secret-stdin] [--secret-file <path>] [--timeout <seconds>] [--max-attempts <n>]
taskg hook enable <hook-id>
taskg hook disable <hook-id>
taskg hook delete <hook-id>
taskg hook deliveries <hook-id> [--status queued|delivering|retry_wait|succeeded|dead_lettered|disabled_skipped] [--limit <n>]
taskg hook replay <delivery-id>
```
