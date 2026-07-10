# 任务重新打开（reopen completed task）

日期：2026-07-10
状态：已实施

## 背景

当前任务一旦被标记完成（completed），无法再回到 pending，全链路是一条单向终态：

- **domain**（`internal/task/model.go`）：只有 `Complete` / `Delete` / `StartTask` / `StopTask`，没有任何 completed→pending 的逆操作。
- **app**（`internal/app/service.go`）：`deleteLocked` / `startLocked` / `stopLocked` 都显式拒绝 completed 任务；`doneLocked` 不拒绝（幂等再完成），但永远只能更 completed。
- **HTTP**（`internal/httpapi`）：只有 done/start/stop/delete 端点，`PATCH /tasks/{ref}` 的 `modifyTaskRequest` 没有 status 字段，无法通过 modify 改状态。
- **Web console**（`web/`）：详情页 `isWritableTaskStatus`（`task-detail-page.tsx:432`）对 completed 返回 false，`TaskActionBar` 直接 `return null`（`task-action-bar.tsx:32`）—— 操作栏整个不渲染；列表行 `task-row-actions.tsx:62` 用 `!isCompleted` 整组隐藏动作按钮。

这个设计在仓库里有一个明显的**自相矛盾**：`service.go:1892` 给"给 completed 父任务加子任务"报的错是：

> parent task is completed; please **reopen the parent** before adding sub-tasks

错误文案主动引导用户去 reopen，但系统根本没有 reopen 能力。

另外 AGENTS.md 第 1 节要求"兼容 Taskwarrior 的核心命令名、JSON 数据格式与 urgency 公式"。Taskwarrior 里 completed 任务可以 `modify status:pending` 回到 pending，当前实现与 taskwarrior 语义不一致（本项目的 modify 不处理 status，且无 status:pending 写路径）。

## 目标

- 提供 completed → pending 的 reopen 能力，打通"误完成 → 可撤销"的最小闭环。
- 后端、HTTP、CLI、Web console 全链路一致。
- 消解 `service.go:1892` 的矛盾文案。

## 决策

- **范围限定：仅 completed → pending**。deleted 不纳入本轮（deleted 是软删除，恢复涉及归档/权限/可见性等独立问题，单独立项更安全）。
- **不通过 modify status 实现**：保持与现有 done/start/stop 一致的"独立动作"模型（独立 service 方法 + 独立端点 + 独立 audit/hook），而非把 status 塞进 modify。理由：modify 在本项目是字段级编辑语义，done/start/stop 都是状态机动作，reopen 属于后者。
- **reopen 后的字段重置**：`Status=StatusPending`、`End=nil`（清完成时间）、`Start=nil`（回到非 active，用户可重新 start）。理由：Complete 会设 `Start=nil`，reopen 是其逆操作；pending 任务默认 Start 为 nil。
- **命名**：domain 方法 `Reopen(now)`；service `Reopen(target)` / `reopenLocked`；audit action `task.reopen`；hook event `task.reopened`（遵循 completed/deleted/started/stopped 的过去式约定）；HTTP `POST /api/v1/tasks/{taskRef}/reopen`；CLI action `reopen`（target+action dispatch）；远程 client `ReopenTask`。
- **状态守卫**：`reopenLocked` 仅允许 `StatusCompleted` 通过；pending/recurring 报错 `cannot reopen %s task`，deleted 报错同样。recurring 是模板态，不应被 reopen。
- **依赖反向联动**：`Reopen` 作为 `Done` 的逆操作，处理依赖事件对称性。`Done` 在 blocker 完成后对其 dependents 发 `task.unblocked`；`Reopen` 在 blocker 回到 pending 后，对从 unblocked 变回 blocked 的 dependents 发 `task.blocked`。依赖状态本身在查询时实时计算（`buildDependencyState`），数据始终正确，事件投递保证 webhook/通知订阅者感知到阻塞状态变化。
- **Web console UX**：把"completed = 操作栏整体隐藏"改为"completed = 隐藏 done/start/stop，但显示 reopen"；列表行 completed 时显示一个 reopen 图标按钮。

## 影响

- **行为**：completed 任务重新可操作（pending 状态恢复编辑/计时）。
- **兼容**：新增 audit action 和 hook event（`task.reopen` / `task.reopened`）。已订阅 task 事件的 webhook 会收到新事件类型，需关注但不破坏现有事件。
- **文档**：README 需补 reopen 命令说明；ROADMAP 无需调整（属 M1 任务生命周期的完善，非新 milestone）。

## 已知边界（不在本轮处理）

- **modify 缺少 completed 状态守卫**：`modifyLocked`（`service.go:678`）不像 `deleteLocked`/`startLocked`/`stopLocked` 那样拒绝 completed 任务，completed 任务可通过 modify 改 title/description 等非状态字段。web console 通过 `isWritableTaskStatus` 在前端禁用 completed 的字段编辑（比后端更严格），所以 Web UX 不受影响；但 HTTP/MCP/CLI 直接调 modify 仍可改 completed 任务字段。这是既有行为，非本轮引入，后续如需对齐应单独调整 `modifyLocked`。
- **recurring parent 子任务的 reopen 不撤销已生成的下一周期实例**：完成 recurring parent 的子任务会生成下一周期实例；reopen 该子任务不会删除已生成的新实例（它们是独立实体，符合直觉）。

