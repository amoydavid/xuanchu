# Task 端到端工作流

CIO 在群里处理工作的典型链路。

## 场景 A：群聊转任务

用户在群里说"登录页 Chrome 121 白屏了，要修"。CIO 把它变成可追踪任务：

```json
// Step 1: 创建任务，归到对应 project
task_add({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "description": "修复登录页面白屏问题",
  "priority": "H",
  "tags": ["bug"]
})
// → 拿到新任务 uuid

// Step 2: 记录上下文备注
task_annotate({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "新任务 uuid",
  "annotation": "客户反馈: Chrome 121 必现"
})

// Step 3: 关联 PR/ticket
task_link_add({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "task": "新任务 uuid",
  "type": "pr",
  "url": "https://github.com/org/repo/pull/42"
})

// Step 4: CIO 自己认领并开始
task_start({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "新任务 uuid"
})

// Step 5: 做完后完成
task_done({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "新任务 uuid"
})
```

## 场景 B：查询并修改

```json
// Step 1: 查自己的高优先级待办
task_query({
  "workspace": "dajee",
  "query": "assignee:me status:pending priority:H"
})

// Step 2: 调整某任务优先级
task_modify({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "任务 uuid",
  "priority": "M"
})

// Step 3: 清空某任务的优先级和执行者
task_modify({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "任务 uuid",
  "clear": ["priority", "assignees"]
})
```

## 场景 C：设置依赖

任务 B 必须等任务 A 完成才能开始：

```json
// B 依赖 A
task_depends({
  "workspace": "dajee",
  "id": "任务B-uuid",
  "depends": ["任务A-uuid"]
})

// 查看依赖是否解除阻塞
task_get({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "任务B-uuid"
})
```

## 场景 D：导出导入

```json
// 导出某项目的全部任务
task_export({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx"
})

// 导入到另一个项目
task_import({
  "workspace": "dajee",
  "project_id": "proj-uuid-yyy",
  "tasks": [
    {"uuid": "imported-001", "description": "导入的任务", "status": "pending", "entry": "2025-06-01T00:00:00Z", "modified": "2025-06-01T00:00:00Z"}
  ]
})
```
