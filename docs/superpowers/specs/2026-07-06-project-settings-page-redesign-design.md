# 项目设置页重构：Tab 子路由 + 配置项 schema 增强 + 备注 timeline 化

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-06
**状态：** 草案
**背景：** `/projects/<slug>/settings` 当前是一个长页面，含「基本信息 / 状态 / 配置项 / 项目备注」四个 section。其中前两个与项目首页（工作台）完全重复，配置项编辑器体验薄弱且存在 `entries.map is not a function` 崩溃，项目备注缺少 timeline 视觉。本次重构收敛职责、增强配置项体验、把备注 timeline 化，并用 tanstack router 的父子子路由实现 tab 切换。

## 1. 背景与现状

### 1.1 设置页现状

`web/src/pages/project-settings-page.tsx` 当前渲染四个 section：

1. **基本信息**（`basic`）：只读展示 name / slug / description。
2. **状态**（`status`）：四个按钮在 `planning/active/archived/cancelled` 间转移。
3. **配置项**（`ProjectConfigEditor`）：列出 `project config` key/value，支持新增/删除。
4. **项目备注**（`ProjectAnnotationsEditor`）：列出 `ProjectAnnotation`，支持新增/删除。

### 1.2 首页（工作台）已覆盖的能力

`web/src/features/workspace/project-workbench/project/project-workbench-page.tsx` 配合 `ProjectHeaderEditor` 已经具备：

- name / description 的 **inline 编辑**（`InlineTextEditor`，比设置页只读展示更强）。
- **状态流转下拉菜单** `ProjectStatusMenu`（带当前态勾选、关闭态二次确认 `DestructiveConfirmDialog`，体验优于设置页的平铺按钮组）。
- **slug 修改弹窗** `ProjectSettingsDialog`（齿轮按钮触发，改完后自动 navigate 到新 slug）。
- **timeline 展示** `ProjectActivity`（圆点轴、actor 名、相对时间、混合 task/project 事件）。

结论：设置页的「基本信息」和「状态」section 是 100% 重复且更弱的版本，应删除。

### 1.3 配置项编辑器的问题

`ProjectConfigEditor`（`project-settings-page.tsx:175`）当前问题：

- **运行时崩溃**：`listProjectConfig` 把后端返回的对象 `{"agent.background":"..."}` 当数组用，`entries.map is not a function`。（已临时修复归一化，但体验问题仍在。）
- **无 schema 感知**：只有两个裸文本框（key / value），无法体现 workspace 级 `ConfigDefinition` 的类型、枚举、默认值、secret 标记。
- **无 inline 编辑**：现有值只能删除后重建，不能就地改。
- **无保存反馈、无重复 key 校验**。

### 1.4 项目备注展示的问题

`ProjectAnnotationsEditor` 当前是平铺卡片，缺少首页 timeline 那种时间轴视觉，与项目活动的呈现风格不统一。

### 1.5 后端能力（已具备，本次不改）

| 能力 | API | 说明 |
|---|---|---|
| project config list | `GET /api/v1/projects/{ref}/config` | 返回 `map[string]string`，按 project 隔离 |
| project config get/set/unset | `GET/PUT/DELETE /api/v1/projects/{ref}/config/{key}` | 值为字符串 |
| config schema list | `GET /api/v1/config-schema` | 返回 `ConfigDefinitionView[]` |
| config schema get | `GET /api/v1/config-schema/{key}` | 返回单个 `ConfigDefinitionView` |
| project annotations | `GET/POST/DELETE /api/v1/projects/{ref}/annotations[/{id}]` | 含 actor 信息 |
| project timeline | `GET /api/v1/projects/{ref}/timeline` | 混合事件 timeline（备注页不直接用） |

`ConfigDefinitionView` 字段：`Key / ValueType / AllowedScopes / Label / Description / EnumValues / DefaultValue / Required / Secret`。

## 2. 目标

1. 设置页只保留**配置项**和**项目备注**两块职责，删除与首页重复的基本信息和状态 section。
2. 用 **tanstack router 父子子路由**实现两个 tab（不是纯组件 state 切换），每个 tab 是独立子页面，可单独被 URL 定位、前进后退、刷新。
3. **配置项 tab 读 schema 增强**：按 `ConfigDefinition` 的 `ValueType / EnumValues / Secret` 渲染对应输入控件，支持 inline 编辑现有值。
4. **项目备注 tab timeline 化**：复用首页 timeline 视觉语言（圆点轴、actor、时间），保留 Markdown 渲染和新增/删除交互。
5. 修掉 `entries.map is not a function` 崩溃（已在 API 层临时归一化，重构中保留并补测试）。
6. 不改后端，不改权限模型，不改 CLI/MCP。

## 3. 非目标

- 不在配置项 tab 内管理 schema 定义本身（schema CRUD 仍走 CLI 或后续单独的 schema 管理页）。
- 不引入 project 备注的编辑能力（后端 `ProjectAnnotation` 暂无 PATCH，spec 维持只新增/删除）。
- 不把首页的混合 timeline 搬进备注 tab；备注 tab 数据源是 `ProjectAnnotation`，不是 project timeline endpoint。
- 不改配置项的值存储模型（仍是单字符串，结构化数据自行编码）。
- 不做配置项值的导入导出。

## 4. 路由设计

### 4.1 URL 结构

```
/workspaces/$workspaceSlug/projects/$projectSlug/settings          → 父布局页（tab 骨架 + Outlet）
/workspaces/$workspaceSlug/projects/$projectSlug/settings/config   → 配置项子页
/workspaces/$workspaceSlug/projects/$projectSlug/settings/notes    → 项目备注子页
```

`settings` 父路由默认重定向到 `settings/config`（首选 tab）。

### 4.2 tanstack router 实现

把现在的单条 `projectSettingsRoute`（`router.tsx:253`）改成父子三条路由：

```tsx
const projectSettingsRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/projects/$projectSlug/settings",
  component: lazyRoute(ProjectSettingsLayoutRoute),
})

const projectSettingsConfigRoute = createRoute({
  getParentRoute: () => projectSettingsRoute,
  path: "config",
  component: lazyRoute(ProjectSettingsConfigRoute),
})

const projectSettingsNotesRoute = createRoute({
  getParentRoute: () => projectSettingsRoute,
  path: "notes",
  component: lazyRoute(ProjectSettingsNotesRoute),
})
```

并在路由树中建立父子关系：

```tsx
projectSettingsRoute.addChildren([
  projectSettingsConfigRoute,   // index 顺序决定默认 tab
  projectSettingsNotesRoute,
])
```

父路由 `settings` 自身（path 无子段匹配时）重定向到 `settings/config`：

```tsx
const projectSettingsIndexRoute = createRoute({
  getParentRoute: () => projectSettingsRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/workspaces/$workspaceSlug/projects/$projectSlug/settings/config" })
  },
})
```

### 4.3 Tab 与 URL 双向绑定

tab 的激活态完全由当前 URL 决定（不引入额外 state）。点击 tab 调 `navigate({ to: ...settings/config })` 切换子路由；浏览器前进后退、刷新、分享链接都能正确定位 tab。

tab 顺序：**配置项**（默认）在前，**项目备注**在后。理由：配置项是设置页的主要价值入口（承载 project 扩展信息，见 README 定论），备注是辅助。

### 4.4 父布局页职责

`ProjectSettingsLayoutRoute` 渲染：

1. 面包屑：`<projectSlug> / 设置`（链接回工作台首页）。
2. 页面标题。
3. `<Tabs>` 组件，`TabsList` 含两个 `TabsTrigger`，`TabsContent` 区域渲染 `<Outlet />`。
4. `useParams` 取 `projectSlug`，`useMe` 取 `workspaceSlug` 和 `canManage`，下传给子页。

`Tabs` 的受控 value 由当前 location pathname 推导：`pathname.endsWith("/notes") ? "notes" : "config"`。`onValueChange` 触发 `navigate`。

注意：用现有 `web/src/components/ui/tabs.tsx`（shadcn Tabs），但 content 区不放具体内容，而是放 `<Outlet />`，让子路由填充。这样既享受 Tabs 视觉，又保留路由驱动。

## 5. 配置项 tab 设计（schema 增强）

### 5.1 数据获取

子页 `ProjectSettingsConfigRoute` 并行两个 query：

- `useQuery(["project", ws, slug, "config"])` → `listProjectConfig` → `ProjectConfigEntry[]`
- `useQuery(["config-schema", ws])` → `listConfigSchema` → `ConfigDefinitionView[]`（workspace 级 schema 全量）

前端把 schema 数组转成 `Map<key, ConfigDefinitionView>` 便于按 key 查找。

### 5.2 配置项列表渲染

每个现有 `ProjectConfigEntry` 一行，按 key 升序。一行展示：

- **key**（等宽字体）+ schema label（若有，作为可读标题）。
- **值**：根据该 key 的 schema 渲染。
  - `Secret=true`：默认遮掩（`••••••`），点击眼睛图标 reveal。
  - `ValueType=string 且 EnumValues 非空`：显示当前枚举值的 label。
  - 其它：显示原始字符串值（长值截断 + tooltip）。
- **操作**：inline 编辑（canManage）、删除（canManage）。

### 5.3 inline 编辑

点击一行进入编辑态（行内展开，不开新弹窗）：

- 若 key 有 schema：
  - `EnumValues` 非空 → `<Select>` 下拉。
  - `ValueType=number` → `<Input type="number">`。
  - `Secret=true` → `<Input type="password">` + reveal toggle。
  - 其它 → `<Input type="text">`。
  - 若 schema 有 `Description`，编辑态下方展示说明文案。
- 若 key 无 schema（orphan 值）：`<Input type="text">`，并标注「未注册 schema」提示。
- 保存：`setProjectConfig(ws, slug, key, value)`，成功后 invalidate query、退出编辑态。失败展示行内错误。
- 取消：还原原值，退出编辑态。

### 5.4 新增配置项

底部「新增」区（canManage 可见）：

- key 输入框 + 值输入区。
- key 输入时，若匹配到已注册 schema，值输入区自动切换为对应控件（同 5.3 规则），并展示该 schema 的 label/description。
- 若 key 未匹配 schema，提示「该 key 未在 workspace schema 注册，写入需 schema 允许 project scope」，仍允许提交（最终由后端 `config_scope_not_allowed` 裁决）。
- 重复 key（已存在于当前列表）：前端拦截，提示「key 已存在，请直接编辑该行」。
- 提交：`setProjectConfig`，成功后清空表单、invalidate。

### 5.5 删除

删除按钮带 `window.confirm`（沿用现有文案 key `projectSettings.configDeleteConfirm`），确认后 `deleteProjectConfig`。

### 5.6 空状态

无配置项时展示空状态文案 + （canManage）引导到新增区。

### 5.7 secret 值的列表展示边界

列表接口 `GET /projects/{ref}/config` 会返回 secret 值的明文（后端当前不过滤）。前端在**列表展示**层默认遮掩 secret 值，避免设置页一眼看到敏感配置；reveal 是用户主动点击行为。这不改变后端契约，只是前端展示策略。

> 备注：若后续希望列表接口本身不回 secret 明文，应作为单独的后端 spec 处理，本次不做。

## 6. 项目备注 tab 设计（timeline 化）

### 6.1 数据获取

`useQuery(["project", ws, slug, "annotations"])` → `listProjectAnnotations` → `ProjectAnnotationInfo[]`。

按 `entry`（或 `created_at`）倒序排列。

### 6.2 timeline 渲染

复用首页 `ProjectActivity` 的视觉语言（圆点轴 + 左侧竖线），但**不复用组件本身**——因为 `ProjectActivity` 接收的是混合事件 `ProjectReadonlyTimelineEntry`，而备注是 `ProjectAnnotationInfo`，字段结构不同（备注有完整 `content` Markdown、`created_by` 是 `UserInfo`）。

新建 `ProjectNotesTimeline` 组件（放 `project-workbench/project/` 下），渲染：

- 每条备注一个 timeline 节点：圆点 + 竖线。
- 节点头部：actor 名（`created_by.name`，复用 `UserInfo` 展示规则）+ 相对/绝对时间。
- 节点正文：`<MarkdownView>{content}</MarkdownView>`。
- 节点尾部（canManage）：删除按钮（带 confirm）。

时间格式化复用现有的 `formatTime`（`project-settings-page.tsx:406`）或提取为 shared util。

### 6.3 新增备注

timeline 顶部（或底部）放新增表单：`<Textarea>` + 提交按钮，`addProjectAnnotation`。沿用现有文案 `projectSettings.annotationNew` 等。

提交成功后清空、invalidate。失败展示错误。

### 6.4 空状态

无备注时展示引导文案。

### 6.5 关闭态处理

project 处于 `archived/cancelled` 时，备注 tab 设为只读（隐藏新增和删除入口），并在顶部展示 closed banner（复用 `ProjectClosedBanner` 或简化提示）。这与配置项写入的 `ensureProjectConfigWritable` 后端约束一致（归档项目禁止写 config，备注同理受 closed project 规则约束）。

## 7. 删除范围

重构后删除/弃用的代码：

- `project-settings-page.tsx` 中的 `ProjectConfigEditor` 和 `ProjectAnnotationsEditor` 旧实现（被新子页替代）。
- 「基本信息」section（`basic`）和「状态」section（`status`）整段删除——首页已全覆盖且更强。
- 旧的 `ProjectSettingsPage` 单体组件（拆成 layout + 两个子页后，原文件可整体重写或拆分）。
- i18n 中 `projectSettings.basic` 和 `projectSettings.status` 两个不再使用的键（保留 `configTitle / annotationsTitle / transitionConfirm` 等仍在用的键；`transitionConfirm` 移交首页 `ProjectStatusMenu` 路径，若首页已用自有文案则一并清理）。

`ProjectSettingsRoute.tsx` 改为 `ProjectSettingsLayoutRoute`，仅负责 layout 和上下文注入，不再渲染具体 section。

## 8. 文件结构规划

```
web/src/routes/workspace/
  ProjectSettingsLayoutRoute.tsx   # 父布局：面包屑 + Tabs + Outlet
  ProjectSettingsConfigRoute.tsx   # 配置项子页 route 包装
  ProjectSettingsNotesRoute.tsx    # 备注子页 route 包装

web/src/pages/
  project-settings-layout.tsx      # 父布局页面组件（与 Route 解耦）
  project-config-tab.tsx           # 配置项 tab 内容（schema 增强 + inline 编辑）
  project-notes-tab.tsx            # 备注 tab 内容（timeline + 新增/删除）

web/src/features/workspace/project-workbench/
  project/
    project-notes-timeline.tsx     # 备注 timeline 组件（圆点轴）
    project-config-row.tsx         # 单行配置项（展示 + inline 编辑态）
  api/
    project-api.ts                 # 已有 config/annotation API；新增 listConfigSchema
    config-schema-api.ts           # （新增或并入 project-api）config schema 读取
```

具体文件拆分在 implementation plan 阶段可调整，但 route 与 page 分离、feature 组件下沉到 feature 目录的原则不变。

## 9. 权限与边界

- `canManage`（owner/admin）控制配置项和备注的写入口可见性，沿用现有 `canProjectManage`。
- 读权限：viewer/member 可见配置项和备注列表（需 `project:read` + `project config:read`）。
- closed project（archived/cancelled）：两个 tab 都只读，后端 `ensureProjectConfigWritable` 是最终裁决。
- secret 配置项：列表默认遮掩，reveal 是前端展示行为，不改变后端返回。

## 10. 测试与验收

### 10.1 单元测试

- `listProjectConfig` 归一化：对象/数组两种输入都返回 `ProjectConfigEntry[]`（补上之前崩溃路径的回归测试）。
- `ProjectConfigRow`：schema 存在/不存在两种渲染分支；secret 遮掩与 reveal；inline 编辑保存/取消。
- `ProjectNotesTimeline`：空状态、多节点渲染、删除交互。
- tab value 与 pathname 的映射推导。

### 10.2 验收清单

- [ ] 访问 `/projects/<slug>/settings` 自动跳到 `/settings/config`。
- [ ] 两个 tab 可通过 URL 直接定位，前进后退正常。
- [ ] 配置项 tab：有 schema 的 key 按类型渲染输入控件（枚举下拉、密码遮掩等）。
- [ ] 配置项 tab：inline 编辑现有值成功后列表刷新。
- [ ] 配置项 tab：新增重复 key 被前端拦截。
- [ ] 配置项 tab：secret 值默认遮掩，点击 reveal。
- [ ] 配置项 tab：删除带确认。
- [ ] 备注 tab：timeline 视觉（圆点轴），Markdown 渲染，actor + 时间。
- [ ] 备注 tab：新增/删除正常，权限边界生效。
- [ ] archived/cancelled 项目两个 tab 只读。
- [ ] 不再有 `entries.map is not a function`。
- [ ] `pnpm tsc --noEmit` 通过，`pnpm vitest run` 全绿。

### 10.3 回归

- 首页（工作台）不受影响：状态流转、inline 编辑、slug 弹窗照常。
- 首页齿轮按钮（`ProjectSettingsDialog`，改 slug）不指向设置页，无需改动。
- 项目列表行的「设置」入口（`project-row-actions.tsx:44` 跳 `/projects/$projectSlug/settings`）仍可到达，落到默认 config tab。

## 11. 风险

- **tanstack router 父子子路由 + Tabs 受控**：需确认 `Tabs` 的 value 由 pathname 推导时，子路由 lazy 加载不会出现 content 区短暂空白。缓解：父布局提供骨架，子路由 `Suspense` fallback。
- **schema list 性能**：workspace 级 schema 全量拉取，单 workspace schema 数量预期很小，不构成问题；若未来 schema 量级增长，再按 key 精确查询。
- **secret 列表展示**：前端遮掩是展示层策略，若用户用 DevTools 看网络响应仍能看到明文。这是当前后端契约决定的，真正隔离需要后端改动（非目标）。

## 12. 后续

- 配置项值的结构化编辑（若 schema 声明复合类型）。
- 配置项/备注的批量操作。
- schema 管理独立页（配置项 tab 只读 schema，不在本次管理 schema 定义）。
- 列表接口对 secret 值的服务端过滤（独立后端 spec）。
