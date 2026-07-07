---
name: xuanchu-capture-and-track-work
description: 把群里冒出来的工作变成结构化任务、设依赖、关联 PR/ticket、记录进度、认领并完成任务、导出导入。用户提到记一下这件事、建任务、加备注、设依赖、关联 PR、开始/完成、导出导入任务时使用。
---

# 捕获与跟踪任务

把 IM 群里冒出来的工作变成结构化任务，并跟踪到完成。这是把"对话"转成"可追踪任务"的核心动作。

## 何时使用

当你要建任务、加备注、设依赖、关联 PR/ticket、开始或完成任务、导出导入任务时，用本 skill。它覆盖 task 的完整生命周期。报表级汇总（今天做什么、逾期）用 xuanchu-report-and-review skill。

## 核心原则

- **先遵循 `xuanchu-mcp-base` 的工具名解析规则。** 本文中的 `task_add` 等是 canonical tool name，真实运行时可能带 MCP server 前缀。
- **每次调用都显式传 `workspace`**，任务归 project 时带 `project_id` / `project`。
- **任务引用只用 UUID 或 `task_slug`**（如 `api-1`），不用本地 working-set 数字 ID——MCP 接口不接受数字 ID。
- 不确定 workspace/project 时先 `project_list` 发现，不要依赖隐式状态。
- 自己接手的任务，建完后用 `task_start` 开始、`task_done` 完成。

## 标准工作流

### 场景 A：群聊转任务

群里说的事变成可追踪任务，记上下文，关联外部材料。

```
1. task_add({"workspace":"dajee","project_id":"...","title":"修复白屏","priority":"H"})
2. task_annotate({"workspace":"dajee","project_id":"...","id":"新任务uuid","annotation":"客户反馈 Chrome 121 必现"})
3. task_link_add({"workspace":"dajee","project_id":"...","task":"...","type":"pr","url":"https://github.com/.../pull/42"})
4. // 自己认领就 task_start，做完 task_done
```

### 场景 B：查询并修改

```
1. task_query({"workspace":"dajee","query":"assignee:me status:pending"})
2. task_modify({"workspace":"dajee","project_id":"...","id":"...","priority":"M"})
3. // 清空字段用 clear:["priority","assignees"]
```

### 场景 C：设依赖

```
1. task_depends({"workspace":"dajee","id":"任务B-uuid","depends":["任务A-uuid"]})  // B 依赖 A
```

## 易错点

- **MCP 接口不能用 working-set 数字 ID**，只能 UUID 或 `task_slug`（形如 `api-1`）。
- `task_denotate` 用 `annotation_id`（从 `task_get`/`task_query` 读取的稳定 ID），不要用显示顺序删除注释。
- `task_modify` 清空字段用 `clear:["priority","assignees"]`，不能传空值清空。
- `task_depends` 引用依赖任务也用 UUID；`clear_depends:true` 清空所有依赖。
- `task_link_add` 的 `type` 常用值：`document`、`pr`、`ticket`、`design`、`issue`。

## 参考文档

| 文件 | 何时读 |
|---|---|
| references/task-tools.md | task 全部操作完整 JSON |
| references/query-syntax.md | task query 表达式语法 |
| references/task-workflows.md | 建任务→记录→完成的端到端示例 |
