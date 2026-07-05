# Web Console 项目工作台首页重构设计

**日期：** 2026-07-05
**状态：** 已实施
**范围：** `/workspaces/:workspaceSlug/projects/:projectSlug` 项目工作台首页

## 背景

当前项目工作台已经具备项目编辑、任务过滤、快速创建、任务列表、负责人摘要和最近动态能力，但首页信息层级偏散：

- 搜索区、新建区、负责人摘要均使用独立边框块，视觉噪音高。
- 过滤条件只覆盖状态、优先级、负责人和关键词，不能满足常见任务检索。
- 任务列表没有显式排序入口。
- 新建任务使用行内输入，无法一次性填写描述、负责人、日期、优先级等字段。
- 负责人摘要只是文本统计，难以扫描，也不能直接驱动筛选。

本次重构不改变后端权限模型和业务边界，只把已有 project-first 工作台整理成更接近 Linear 的操作体验。

## 目标

1. 去掉搜索区、新建区、负责人摘要的大块边框，改为更轻量、清晰的工作台布局。
2. 将任务过滤升级为可组合工具栏：常用条件直接展示，低频条件收纳到更多筛选。
3. 为任务列表增加排序控制，并把排序同步到 URL 和 `/api/v1/tasks?sort=`。
4. 将新建任务改为按钮打开弹窗，弹窗内填写标题、描述、负责人、日期、优先级、标签等字段。
5. 优化负责人摘要为可扫描的 workload 区域，并支持点击负责人快速筛选。
6. 继续复用 shadcn/ui 现有组件和当前 project-workbench API/mutation 边界。

## 非目标

- 不做看板、拖拽排序或批量编辑。
- 不新增全局任务 inbox。
- 不新增后端专用筛选协议；优先复用现有 RESTful 参数和 `query` 表达式。
- 不改变 task.UserInfo / assignee 的身份语义。
- 不改变权限判定和 closed project 禁写规则。

## 设计

### 页面结构

项目工作台首页调整为：

```text
项目 Header
统计指标条
TaskToolbar：筛选 chips + 排序 + 新建任务
TaskTable：可排序表头 + inline 高频字段
AssigneeWorkloadSummary：负责人工作量摘要
RecentActivity
```

过滤、新建和负责人摘要不再是独立边框卡片。任务表格保留边界，因为它是主要数据容器。

### 过滤工具栏

新增 `ProjectTaskToolbar`，负责 URL search 的读写和展示：

- 常用条件：关键词、状态、优先级、负责人、标签、到期起止。
- 更多筛选：原始查询表达式、未分配、无截止日期、等待到、计划开始、有效至。
- 活跃条件使用 `Badge` / 小按钮展示，可单项清除或全部清除。
- 日期使用 shadcn `Input` 的日期输入；新建弹窗内复用现有 `InlineDatePicker`。

URL search 保留现有字段并新增：

- `sort`
- `query`
- `due_empty`
- `assignee_empty`
- `wait_before`
- `scheduled_before`
- `until_before`

这些字段在前端转换为当前后端已支持的 `query` 表达式或 RESTful 参数。

### 排序

排序使用 `sort` 参数透传到后端。第一版只提供后端已支持的排序：

- `entry`：创建顺序
- `next`：下一步优先
- `due`：截止日期
- `wait`：暂缓到
- `start`：开始时间
- `completed`：完成时间

任务表头的“标识 / 到期”等可作为排序快捷入口，但不声称支持任意列升降序。

### 新建任务弹窗

新增 `TaskCreateDialog`，替代行内快速创建：

- 必填：标题。
- 可选：描述、负责人、优先级、截止日期、计划开始、暂缓到、有效至、标签。
- 描述使用现有 `@/components/markdown` 中封装的 `MarkdownEditor`，保持与任务详情页一致的 Markdown/Tiptap 编辑体验。
- 负责人选择复用 workspace members 查询，展示 name / display_name / email。
- 保存失败不关闭弹窗，保留输入。
- 创建成功后关闭弹窗，刷新当前筛选下的任务列表和项目摘要。

### 负责人摘要

新增 `AssigneeWorkloadSummary`：

- 按负责人聚合未完成、逾期、高优先级、即将到期、总数。
- 未分配作为独立项展示。
- 每个负责人一行，使用紧凑指标和轻量进度条表达负载。
- 点击负责人设置当前 `assignee` 过滤；点击未分配设置 `assignee_empty`。

### 组件边界

- `project-workbench-page.tsx` 只做数据获取、权限判断和页面编排。
- `project-task-toolbar.tsx` 负责过滤和排序 UI。
- `task-create-dialog.tsx` 负责创建任务表单。
- `assignee-workload-summary.tsx` 负责负责人摘要展示和点击筛选。
- `task-table.tsx` 继续负责任务列表和 inline 高频编辑。

## 测试

前端测试重点：

- toolbar 能渲染更多筛选、写入 URL，并把 `sort` 传给任务查询。
- 新建任务弹窗提交完整字段。
- 任务表格排序控制展示和交互正确。
- 负责人摘要显示未完成/逾期/高优先级，并可点击触发筛选。
- closed project 时不显示新建任务入口。

验证命令：

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web build
pnpm --dir web lint
git diff --check
```

如本次没有修改 Go 后端，可不跑完整 Go 验证；若调整后端查询语义，则补跑：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
