# Workspace 配置入口与自动化 Provider 展开逻辑修正 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修正项目自动化页 Provider 展开逻辑（按 effective 完整性判断 + 声明写入 project 级 + 引导去配 workspace 级），并在 `/workspaces` 列表当前 effective 行新增「配置」入口，跳转到新页 `/workspaces/{slug}/config` 编辑该 workspace 全部 config 值。

**Architecture:** 纯前端改动，后端零改动（`?workspace=` query 已被 `requestWorkspaceRef` 原生支持）。两条独立改动线：A 改项目自动化页展开逻辑，B 加 workspace 配置入口与新页。B 内部先抽离 `ProjectConfigTab` 的共享编辑组件再复用。

**Tech Stack:** React + TanStack Router + TanStack Query + Tailwind + shadcn/ui + Vitest/RTL。i18n 在 `web/src/locales/zh-CN.ts` 与 `en-US.ts`。

**关联 spec:** `docs/superpowers/specs/2026-07-29-workspace-config-entry-design.md`

---

## 文件结构

**改动线 A（项目自动化页展开逻辑）：**
- 修改 `web/src/features/workspace/project-workbench/automations/project-automations-api.ts`：新增 `useProjectAutomationProviderConfig` hook（GET facade）。
- 修改 `web/src/features/workspace/project-workbench/automations/project-automations-page.tsx`：用 facade `complete` 替换 `isProviderConfigComplete` 判断。
- 修改 `web/src/features/workspace/project-workbench/automations/automation-provider-config.tsx`：卡片顶部加「写入 project 级」声明 + workspace 配置引导链接。
- 修改 `web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx`：更新 mock。
- 修改 `web/src/features/workspace/project-workbench/automations/automation-provider-config.test.tsx`：新增文案断言。

**改动线 B（workspace 配置入口与新页）：**
- 新建 `web/src/features/workspace/config/effective-config-editor.tsx`：从 `ProjectConfigTab` 抽离的共享组件（`EffectiveConfigList` + `AddConfigValueDialog` + `sourceLabel`）。
- 修改 `web/src/pages/project-config-tab.tsx`：改为复用抽离后的共享组件（行为不变）。
- 新建 `web/src/features/workspace/config/workspace-config-api.ts`：workspace config 按 slug 寻址的 api 函数。
- 新建 `web/src/pages/workspace-config-page.tsx`：新页主体。
- 新建 `web/src/routes/workspace/WorkspaceConfigRoute.tsx`：路由组件。
- 修改 `web/src/routes/router.tsx`：注册新路由。
- 修改 `web/src/features/workspace/workspaces/workspace-console.tsx`：行新增「配置」按钮。
- 修改 `web/src/features/workspace/workspaces/workspace-console.test.tsx`：新增按钮断言。
- 新建 `web/src/pages/workspace-config-page.test.tsx`：新页测试。
- 修改 `web/src/locales/zh-CN.ts` 与 `en-US.ts`：新增 i18n key。

---

## 改动线 A：项目自动化页展开逻辑修正

### Task A1: 新增项目 provider facade hook

**Files:**
- Modify: `web/src/features/workspace/project-workbench/automations/project-automations-api.ts`

- [ ] **Step 1: 在 `project-automations-api.ts` 末尾新增 GET facade hook**

先读文件确认末尾位置与现有 import（`useQuery` / `workspaceApiGet` 是否已导入）。在文件末尾追加：

```ts
// AutomationProviderFacade 对应后端 app.AutomationProviderConfigView（safe facade，不回传 api_key 原值）。
export type AutomationProviderFacade = {
  base_url: string
  model: string
  allowed_hosts: string[]
  api_key_set: boolean
  complete: boolean
  missing_fields: string[]
}

// 项目级 provider 配置的 safe facade（按 effective 值计算 complete）。后端路由：
// GET /api/v1/projects/{projectRef}/automations/provider-config
export function useProjectAutomationProviderConfig(projectSlug: string) {
  return useQuery({
    queryKey: ["project", projectSlug, "automation-provider-config"],
    queryFn: () =>
      workspaceApiGet<AutomationProviderFacade>(
        `/api/v1/projects/${encodeURIComponent(projectSlug)}/automations/provider-config`
      ),
  })
}
```

若 `useQuery` / `workspaceApiGet` 尚未在该文件顶部 import，补上：

```ts
import { useQuery } from "@tanstack/react-query"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
```

（先 grep 确认；该文件已有别的 query/mutation hook，大概率已导入。）

- [ ] **Step 2: typecheck**

Run: `pnpm --dir web typecheck`
Expected: PASS，无新错误。

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/project-workbench/automations/project-automations-api.ts
git commit -m "feat: 新增项目 provider safe facade hook"
```

---

### Task A2: 用 facade complete 替换展开判断

**Files:**
- Modify: `web/src/features/workspace/project-workbench/automations/project-automations-page.tsx:64-72,203-209`

- [ ] **Step 1: 替换 providerComplete 计算**

在 `project-automations-page.tsx` 中，删除旧的 project config 判断（第 64-72 行）：

```tsx
// 删除这段：
// 读取 project config 判断 provider 是否已配齐；与 AutomationProviderConfigSection 共享同一 query key，React Query 自动去重。
const configQueryKey = ["project", projectSlug, "config"]
const projectConfig = useQuery({
  queryKey: configQueryKey,
  queryFn: () => listProjectConfig(workspaceSlug, projectSlug),
})
const providerComplete = projectConfig.data
  ? isProviderConfigComplete(providerConfigFromEntries(projectConfig.data))
  : false
```

替换为：

```tsx
// provider 是否配齐按 effective 值判断（project → workspace → default）。
// 数据源是后端 safe facade，complete=true 表示 effective 链已配齐 base_url/api_key/model。
const providerConfig = useProjectAutomationProviderConfig(projectSlug)
const providerComplete = providerConfig.data?.complete ?? false
```

并在顶部 import 中：移除 `listProjectConfig`、`isProviderConfigComplete`、`providerConfigFromEntries` 的导入，新增 `useProjectAutomationProviderConfig`：

```tsx
// 原 import 段（第 15、30 行附近）里删掉：
//   import { listProjectConfig } from "@/features/workspace/project-workbench/api/project-api"
// 以及从 "./automation-provider-config" 删掉 isProviderConfigComplete, providerConfigFromEntries
// 新增：
import { useProjectAutomationProviderConfig } from "./project-automations-api"
```

> 注：`AutomationProviderConfigSection`（卡片组件）内部仍需读 project config 做编辑回显，它自己有独立的 `useQuery(["project", projectSlug, "config"])`（`automation-provider-config.tsx:54-58`），**不受影响**，保留。本任务只改「是否展开」的判断，不动卡片内部。

- [ ] **Step 2: 确认展开处使用的是 `providerComplete` 变量名（第 203 行无需改）**

`{providerComplete ? null : <AutomationProviderConfigSection ... />}` 变量名一致，无需改动该行。

- [ ] **Step 3: 更新 `project-automations-page.test.tsx` 的 mock**

该测试当前 mock 了 `listProjectConfig`（见文件约第 72-80 行 `vi.mock("./project-api", ...)` 或对 `project-api` 的 mock）。由于页面不再调用 `listProjectConfig`，需改为 mock `useProjectAutomationProviderConfig`。

读 `project-automations-page.test.tsx` 找到 `vi.mock("./project-automations-api", ...)` 块，在其中加入对 `useProjectAutomationProviderConfig` 的 mock：

```tsx
// 在 vi.mock("./project-automations-api", () => ({...})) 的返回对象里追加：
useProjectAutomationProviderConfig: vi.fn(() => ({
  data: { complete: false, base_url: "", model: "", allowed_hosts: [], api_key_set: false, missing_fields: ["base_url", "api_key_set", "model"] },
  isLoading: false,
})),
```

如果该测试有「provider 已配齐时不展开」的断言用例，新增/调整一个用例验证：mock 返回 `complete: true` 时卡片不渲染，`complete: false` 时渲染。参照现有用例（如 "renders provider config section when incomplete" 之类）。

- [ ] **Step 4: 运行测试**

Run: `pnpm --dir web test -- --run project-automations-page`
Expected: PASS。

- [ ] **Step 5: typecheck + lint**

Run: `pnpm --dir web typecheck && pnpm --dir web lint`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/automations/project-automations-page.tsx web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx
git commit -m "feat: 项目自动化 provider 展开改按 effective 完整性判断"
```

---

### Task A3: 卡片声明「写入 project 级」+ workspace 配置引导链接

**Files:**
- Modify: `web/src/features/workspace/project-workbench/automations/automation-provider-config.tsx:101-116`
- Modify: `web/src/features/workspace/project-workbench/automations/automation-provider-config.test.tsx`

- [ ] **Step 1: 在卡片顶部（`<section>` 内，标题下方）加声明与引导**

在 `automation-provider-config.tsx` 第 108 行（`!complete ?` 的 Alert 之前）插入声明段落。同时为引导链接引入 `useMe` 和 `Link`。

文件顶部加 import：

```tsx
import { Link } from "@tanstack/react-router"
import { useMe } from "@/features/workspace/session/useMe"
```

在组件函数体开头（`const feedback = ...` 附近）取 effective slug：

```tsx
const me = useMe()
const effectiveSlug = me.data?.effective_workspace.slug ?? ""
```

把第 101-116 行的 `<section>` 开头改为（在标题行 `<div className="flex items-center justify-between">...</div>` 之后、`{!complete ? <Alert>...` 之前，插入两个段落）：

```tsx
<section className="space-y-3 rounded-md border p-4">
  <div className="flex items-center justify-between">
    <h3 className="text-sm font-medium">Agent Provider 配置</h3>
    <span className={complete ? "text-xs text-primary" : "text-xs text-warn"}>
      {complete ? "已配置" : `缺少 ${missing.join("、")}`}
    </span>
  </div>

  {/* 新增：声明写入范围 */}
  <p className="text-xs text-muted-foreground">
    此处的 base_url / API Key / model / allowed_hosts 将写入<strong>当前项目（project 级）配置</strong>。
  </p>

  {/* 新增：引导去配 workspace 级（仅当能拿到 effective slug 时显示链接） */}
  {effectiveSlug ? (
    <p className="text-xs text-muted-foreground">
      建议优先配置 workspace 级 provider，同 workspace 下所有项目可共享，无需逐项目配置。前往配置：
      <Link
        className="ml-1 font-medium text-primary hover:underline"
        to="/workspaces/$workspaceSlug/config"
        params={{ workspaceSlug: effectiveSlug }}
      >
        {effectiveSlug}
      </Link>
    </p>
  ) : null}

  {!complete ? (
    <Alert variant="default">
      <AlertTitle>预览和测试需要先配置 Agent Provider</AlertTitle>
      <AlertDescription>
        填写下方 base_url、API Key 和 model 后保存。allowed_hosts 是可选的 SSRF 白名单，未配置时不限制目标 host。
      </AlertDescription>
    </Alert>
  ) : null}
  {/* ... 后续输入框不变 ... */}
```

- [ ] **Step 2: 更新 `automation-provider-config.test.tsx` 断言**

读现有测试文件，确认其渲染方式（大概率用 `renderWithRouter` 或 fetch spy）。新增/调整用例断言：
- 卡片渲染时包含文案「当前项目（project 级）配置」。
- 当 mock 的 `useMe` effective slug = `"acme"` 时，存在一个指向 `/workspaces/acme/config` 的链接（`expect(screen.getByRole("link", { name: "acme" })).toHaveAttribute("href", "/workspaces/acme/config")` 或用 `screen.getByText("acme").closest("a")`）。

如果该测试当前不 mock router（直接 `render`），引入 `<Link>` 后需改用 `renderWithRouter`（`@/test/router-wrapper`），否则 `<Link>` 会报错。参照 `project-config-tab.test.tsx` 的 `renderWithRouter` 用法。

> 提示：`useMe` 走 `workspaceApiGet("/api/v1/credentials/current")`，若用 fetch spy 风格，让 fetch 在 url 含 `/credentials/current` 时返回 `{ data: { effective_workspace: { slug: "acme" }, effective_role: "owner", token: { scopes: ["config:write"] }, actor: { id: "u1", name: "U" } } }`。

- [ ] **Step 3: 运行测试**

Run: `pnpm --dir web test -- --run automation-provider-config`
Expected: PASS。

- [ ] **Step 4: typecheck + lint**

Run: `pnpm --dir web typecheck && pnpm --dir web lint`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/project-workbench/automations/automation-provider-config.tsx web/src/features/workspace/project-workbench/automations/automation-provider-config.test.tsx
git commit -m "feat: provider 卡片声明写入 project 级并引导 workspace 配置"
```

---

## 改动线 B：workspace 配置入口与新页

### Task B1: 抽离共享编辑组件 `EffectiveConfigList` / `AddConfigValueDialog`

**Files:**
- Create: `web/src/features/workspace/config/effective-config-editor.tsx`
- Modify: `web/src/pages/project-config-tab.tsx`

目标：把 `project-config-tab.tsx` 里的 `EffectiveRow` / `AddValueDialog` / `sourceLabel` / `canEditProjectScope` 抽到共享文件，参数化 scope 判定，使 project 和 workspace 两边复用。project 侧行为必须不变。

- [ ] **Step 1: 先写共享组件文件 `effective-config-editor.tsx`**

从 `project-config-tab.tsx` 搬运 `sourceLabel`（第 141-152 行）、`EffectiveRow`（第 154-313 行）、`AddValueDialog`（第 315-429 行），重命名为 `EffectiveConfigList`（渲染列表）和 `AddConfigValueDialog`，并参数化 scope 判定。

新文件 `web/src/features/workspace/config/effective-config-editor.tsx`：

```tsx
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import { formatConfigDisplayValue } from "@/features/workspace/config/config-display"
import { ConfigValueControl } from "@/features/workspace/config/config-value-control"
import {
  type ConfigAllowedScope,
  type ConfigEffectiveValue,
} from "@/features/workspace/config/config-definition-api"

// sourceLabel 把后端来源字符串翻译成展示文案。
export function sourceLabel(source: string, t: (k: string) => string): string {
  switch (source) {
    case "project":
      return t("configDefinitions.sourceProject")
    case "workspace":
      return t("configDefinitions.sourceWorkspace")
    case "default":
      return t("configDefinitions.sourceDefault")
    default:
      return t("configDefinitions.sourceMissing")
  }
}

// EffectiveConfigList 渲染 effective 配置行列表。scope 决定可写判定：
// project 侧传 "project"，workspace 侧传 "workspace"；definition.allowed_scopes 不含该 scope 的行只读。
export function EffectiveConfigList({
  rows,
  scope,
  canManage,
  onSave,
  onRestore,
}: {
  rows: ConfigEffectiveValue[]
  scope: ConfigAllowedScope
  canManage: boolean
  onSave: (key: string, value: string) => Promise<void>
  onRestore: (key: string) => Promise<void>
}) {
  const { t } = useTranslation()
  const canEditRow = (row: ConfigEffectiveValue): boolean =>
    canManage && (row.definition.allowed_scopes as string[]).includes(scope)

  return (
    <div className="divide-y">
      {rows.map((row) => (
        <EffectiveRow
          canEdit={canEditRow(row)}
          isOverriddenSource={row.source === scope}
          key={row.key}
          row={row}
          onSave={(value) => onSave(row.key, value)}
          onRestore={() => onRestore(row.key)}
        />
      ))}
      {!rows.length ? (
        <p className="py-2 text-sm text-muted-foreground">
          {t("configDefinitions.noValues")}
        </p>
      ) : null}
    </div>
  )
}

function EffectiveRow({
  row,
  canEdit,
  isOverriddenSource,
  onSave,
  onRestore,
}: {
  row: ConfigEffectiveValue
  canEdit: boolean
  isOverriddenSource: boolean
  onSave: (value: string) => Promise<void>
  onRestore: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(row.value ?? "")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const isSecret = row.definition.secret
  const [revealed, setRevealed] = useState(false)

  const startEdit = () => {
    setDraft(row.value ?? "")
    setError(null)
    setEditing(true)
  }

  const save = async () => {
    setBusy(true)
    setError(null)
    try {
      await onSave(draft)
      setEditing(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setBusy(false)
    }
  }

  const restore = async () => {
    setBusy(true)
    setError(null)
    try {
      await onRestore()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setBusy(false)
    }
  }

  const displayValue =
    row.value === null
      ? "—"
      : isSecret && !revealed
        ? "••••••"
        : formatConfigDisplayValue(row.definition.value_type, row.value)

  return (
    <div className="py-2">
      <div className="flex flex-wrap items-center gap-2">
        <code className="text-xs">{row.key}</code>
        {row.definition.label ? (
          <span className="text-xs text-muted-foreground">
            {row.definition.label}
          </span>
        ) : null}
        <Badge variant="outline">{sourceLabel(row.source, t)}</Badge>
        {row.missing_required ? (
          <Badge variant="destructive">
            {t("configDefinitions.statusMissingRequired")}
          </Badge>
        ) : null}
        {!canEdit ? (
          <Badge variant="secondary">
            {t("configDefinitions.statusReadonly")}
          </Badge>
        ) : null}
        {isOverriddenSource ? (
          <Badge variant="secondary">
            {t("configDefinitions.statusOverridden")}
          </Badge>
        ) : row.source === "workspace" || row.source === "default" ? (
          <Badge variant="secondary">
            {t("configDefinitions.statusInherited")}
          </Badge>
        ) : null}
      </div>

      {editing ? (
        <div className="mt-2 space-y-2">
          <ConfigValueControl
            definition={row.definition}
            onChange={setDraft}
            value={draft}
          />
          {error ? <p className="text-xs text-destructive">{error}</p> : null}
          <div className="flex gap-2">
            <Button disabled={busy} onClick={save} size="sm" type="button">
              {t("configDefinitions.save")}
            </Button>
            <Button
              onClick={() => setEditing(false)}
              size="sm"
              type="button"
              variant="outline"
            >
              {t("configDefinitions.cancel")}
            </Button>
          </div>
        </div>
      ) : (
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <span className="truncate text-sm text-muted-foreground">
            {displayValue}
          </span>
          {isSecret && row.value !== null ? (
            <Button
              aria-label={
                revealed
                  ? t("configDefinitions.hideSecret")
                  : t("configDefinitions.revealSecret")
              }
              onClick={() => setRevealed((v) => !v)}
              size="icon-sm"
              type="button"
              variant="ghost"
            >
              {revealed ? "隐藏" : "显示"}
            </Button>
          ) : null}
          {canEdit ? (
            <div className="flex gap-1">
              <Button
                onClick={startEdit}
                size="sm"
                type="button"
                variant="outline"
              >
                {t("configDefinitions.edit")}
              </Button>
              {isOverriddenSource ? (
                <Button
                  disabled={busy}
                  onClick={restore}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  {t("configDefinitions.restoreInherited")}
                </Button>
              ) : null}
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}

// AddConfigValueDialog 新增配置值。只列出 allowed_scopes 含当前 scope、且尚未在该 scope 被覆盖的 key。
export function AddConfigValueDialog({
  rows,
  scope,
  canManage,
  onSave,
}: {
  rows: ConfigEffectiveValue[]
  scope: ConfigAllowedScope
  canManage: boolean
  onSave: (key: string, value: string) => Promise<void>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [value, setValue] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (!canManage) return null

  // 可写：定义允许当前 scope 且当前 scope 尚未覆盖（用 source 判断：source===scope 表示已被本 scope 覆盖）。
  const scopeValueKey = `${scope}_value` as "project_value" | "workspace_value"
  const writable = rows.filter(
    (r) =>
      (r.definition.allowed_scopes as string[]).includes(scope) &&
      r[scopeValueKey] == null
  )
  const scopeOnly = rows.filter(
    (r) => !(r.definition.allowed_scopes as string[]).includes(scope)
  )
  const selectedDef = selectedKey
    ? rows.find((r) => r.key === selectedKey)?.definition
    : undefined

  const submit = async () => {
    if (!selectedKey) return
    setBusy(true)
    setError(null)
    try {
      await onSave(selectedKey, value)
      setOpen(false)
      setSelectedKey(null)
      setValue("")
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Button onClick={() => setOpen(true)} size="sm" type="button">
        {t("configDefinitions.addValue")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("configDefinitions.addValueTitle")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <Select onValueChange={(k) => setSelectedKey(k)} value={selectedKey ?? ""}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t("configDefinitions.addValueKey")} />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectLabel>
                    {t("configDefinitions.addValueProjectWritable")}
                  </SelectLabel>
                  {writable.map((r) => (
                    <SelectItem key={r.key} value={r.key}>
                      {r.key} ({r.definition.value_type})
                    </SelectItem>
                  ))}
                  {scopeOnly.length > 0 ? (
                    <>
                      <SelectLabel>
                        {t("configDefinitions.addValueWorkspaceOnly")}
                      </SelectLabel>
                      {scopeOnly.map((r) => (
                        <SelectItem disabled key={r.key} value={r.key}>
                          {r.key} ({r.definition.value_type})
                        </SelectItem>
                      ))}
                    </>
                  ) : null}
                </SelectGroup>
              </SelectContent>
            </Select>
            {selectedDef ? (
              <ConfigValueControl
                definition={selectedDef}
                onChange={setValue}
                value={value}
              />
            ) : null}
            {error ? <p className="text-xs text-destructive">{error}</p> : null}
          </div>
          <DialogFooter>
            <Button
              disabled={busy || !selectedKey}
              onClick={submit}
              size="sm"
              type="button"
            >
              {t("configDefinitions.save")}
            </Button>
            <Button
              onClick={() => setOpen(false)}
              size="sm"
              type="button"
              variant="outline"
            >
              {t("configDefinitions.cancel")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
```

> 行为对齐说明（与原 `ProjectConfigTab` 逐项核对）：
> - 原 `canEditProjectScope(row)` = `allowed_scopes.includes("project")` → 这里参数化为 `scope` 传入，project 侧传 `"project"` 等价。
> - 原「已覆盖」判断用 `row.source === "project"` → 这里 `isOverriddenSource = row.source === scope`，project 侧等价。
> - 原 `AddValueDialog` 用 `r.project_value == null` 判断未覆盖 → 这里 `r[scope_value_key]`，project 侧取 `project_value`，等价。workspace 侧取 `workspace_value`（后端 effective view 含该字段，见 `ConfigEffectiveValue` 类型 `workspace_value?: string | null`）。

- [ ] **Step 2: 改 `project-config-tab.tsx` 复用共享组件**

把 `project-config-tab.tsx` 改为：删除本地 `sourceLabel` / `EffectiveRow` / `AddValueDialog` / `canEditProjectScope`，import 共享组件，调用 `<EffectiveConfigList scope="project" .../>` 和 `<AddConfigValueDialog scope="project" .../>`。

修改后的 `project-config-tab.tsx`（保留页面外壳，替换内部）：

```tsx
import { useMemo } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  EffectiveConfigList,
  AddConfigValueDialog,
} from "@/features/workspace/config/effective-config-editor"
import { listProjectEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import {
  deleteProjectConfig,
  setProjectConfig,
} from "@/features/workspace/project-workbench/api/project-api"

type ProjectConfigTabProps = {
  canManage: boolean
  closed: boolean
  projectSlug: string
  workspaceSlug: string
}

export function ProjectConfigTab({
  canManage,
  closed,
  projectSlug,
  workspaceSlug,
}: ProjectConfigTabProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const queryKey = useMemo(
    () => ["project", workspaceSlug, projectSlug, "config", "effective"] as const,
    [workspaceSlug, projectSlug]
  )

  const effectiveQuery = useQuery({
    queryKey,
    queryFn: () => listProjectEffectiveConfig(projectSlug),
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey })

  const saveMut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setProjectConfig(workspaceSlug, projectSlug, key, value),
    onSuccess: invalidate,
  })

  const deleteMut = useMutation({
    mutationFn: (key: string) => deleteProjectConfig(workspaceSlug, projectSlug, key),
    onSuccess: invalidate,
  })

  const rows = effectiveQuery.data ?? []
  const writable = canManage && !closed

  return (
    <section className="rounded-lg space-y-3 border bg-card p-4">
      <div>
        <h2 className="text-sm font-medium">
          {t("projectSettings.configTitle")}
        </h2>
        <p className="text-xs text-muted-foreground">
          {t("configDefinitions.effectiveDescription")}
        </p>
      </div>

      {closed ? (
        <Alert>
          <AlertDescription>
            {t("projectSettings.configClosedReadonly")}
          </AlertDescription>
        </Alert>
      ) : null}

      {effectiveQuery.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : null}

      {effectiveQuery.isError ? (
        <p className="text-sm text-destructive">{t("common.error")}</p>
      ) : null}

      <EffectiveConfigList
        rows={rows}
        scope="project"
        canManage={writable}
        onSave={(key, value) => saveMut.mutateAsync({ key, value })}
        onRestore={(key) => deleteMut.mutateAsync(key)}
      />

      <AddConfigValueDialog
        rows={rows}
        scope="project"
        canManage={writable}
        onSave={async (key, value) => {
          await saveMut.mutateAsync({ key, value })
        }}
      />
    </section>
  )
}
```

- [ ] **Step 3: 运行现有 project-config-tab 测试，确认行为不回归**

Run: `pnpm --dir web test -- --run project-config-tab`
Expected: PASS（所有原有用例通过，证明抽离无行为变化）。

- [ ] **Step 4: typecheck + lint**

Run: `pnpm --dir web typecheck && pnpm --dir web lint`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/config/effective-config-editor.tsx web/src/pages/project-config-tab.tsx
git commit -m "refactor: 抽离 effective config 编辑组件供 project/workspace 复用"
```

---

### Task B2: 新增 workspace config api 函数

**Files:**
- Create: `web/src/features/workspace/config/workspace-config-api.ts`

- [ ] **Step 1: 写 api 文件**

```tsx
import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"
import { type ConfigEffectiveValue } from "@/features/workspace/config/config-definition-api"

// 按 slug 寻址的 workspace config。后端 /api/v1/config* 原生支持 ?workspace= query（requestWorkspaceRef）。
// 与现有 listWorkspaceEffectiveConfig（无 slug，依赖 token）区分，用 ForSlug 后缀。
function workspaceQuery(workspaceSlug: string): string {
  return `workspace=${encodeURIComponent(workspaceSlug)}`
}

export function workspaceConfigEffectivePath(workspaceSlug: string): string {
  return `/api/v1/config/effective?${workspaceQuery(workspaceSlug)}`
}

export function workspaceConfigKeyPath(
  workspaceSlug: string,
  key: string
): string {
  return `/api/v1/config/${encodeURIComponent(key)}?${workspaceQuery(workspaceSlug)}`
}

export function listWorkspaceEffectiveConfigForSlug(
  workspaceSlug: string
): Promise<ConfigEffectiveValue[]> {
  return workspaceApiGet<ConfigEffectiveValue[]>(
    workspaceConfigEffectivePath(workspaceSlug)
  )
}

export function setWorkspaceConfig(
  workspaceSlug: string,
  key: string,
  value: string
): Promise<void> {
  return workspaceApiPut<void>(workspaceConfigKeyPath(workspaceSlug, key), {
    value,
  })
}

export function deleteWorkspaceConfig(
  workspaceSlug: string,
  key: string
): Promise<void> {
  return workspaceApiDelete<void>(workspaceConfigKeyPath(workspaceSlug, key))
}
```

- [ ] **Step 2: typecheck**

Run: `pnpm --dir web typecheck`
Expected: PASS。

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/config/workspace-config-api.ts
git commit -m "feat: 新增按 slug 寻址的 workspace config api"
```

---

### Task B3: 新增 i18n key

**Files:**
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 在 `zh-CN.ts` 的 `workspacesConsole` 命名空间（约第 887-893 行）追加 key**

```ts
workspacesConsole: {
  subtitle: "管理可见 workspace。归档后只读；恢复能力待后端支持。",
  archive: "归档",
  archiveConfirm: "确认归档该 workspace？归档后变为只读。",
  restore: "恢复",
  restoreUnavailable: "后端暂不支持 workspace unarchive。",
  config: "配置",                              // 新增
},
```

并在 `zh-CN.ts` 新增一个命名空间 `workspaceConfig`（放在合适位置，如 `configDefinitions` 附近）：

```ts
workspaceConfig: {
  title: "工作空间配置",
  description: "编辑当前工作空间的配置值。schema 定义请前往 /settings。",
  readonlyNotCurrent: "仅可在当前工作空间上下文编辑配置。如需修改其它工作空间，请联系管理员或切换上下文。",
},
```

- [ ] **Step 2: 在 `en-US.ts` 对应位置追加英文镜像**

```ts
workspacesConsole: {
  // ...existing...
  config: "Config",
},
// ...
workspaceConfig: {
  title: "Workspace Config",
  description: "Edit config values for the current workspace. Manage schema definitions in /settings.",
  readonlyNotCurrent: "Config can only be edited within the current workspace context. To modify another workspace, contact an admin or switch context.",
},
```

- [ ] **Step 3: typecheck**

Run: `pnpm --dir web typecheck`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 新增 workspace 配置入口与页面的 i18n key"
```

---

### Task B4: 新增 `WorkspaceConfigPage` 页面组件

**Files:**
- Create: `web/src/pages/workspace-config-page.tsx`

- [ ] **Step 1: 写页面组件**

```tsx
import { useMemo } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AddConfigValueDialog,
  EffectiveConfigList,
} from "@/features/workspace/config/effective-config-editor"
import {
  deleteWorkspaceConfig,
  listWorkspaceEffectiveConfigForSlug,
  setWorkspaceConfig,
} from "@/features/workspace/config/workspace-config-api"
import { useMe } from "@/features/workspace/session/useMe"
import { canConfigManage } from "@/features/workspace/project-workbench/permissions/permissions"

type WorkspaceConfigPageProps = {
  workspaceSlug: string
}

export function WorkspaceConfigPage({ workspaceSlug }: WorkspaceConfigPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const me = useMe()
  const effectiveSlug = me.data?.effective_workspace.slug ?? ""
  const isCurrent = workspaceSlug === effectiveSlug && workspaceSlug !== ""
  const canManage =
    isCurrent &&
    canConfigManage({
      role: me.data?.effective_role,
      scopes: me.data?.token.scopes,
    })

  const queryKey = useMemo(
    () => ["workspace", workspaceSlug, "config", "effective"] as const,
    [workspaceSlug]
  )

  const effectiveQuery = useQuery({
    queryKey,
    queryFn: () => listWorkspaceEffectiveConfigForSlug(workspaceSlug),
    enabled: isCurrent,
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey })

  const saveMut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setWorkspaceConfig(workspaceSlug, key, value),
    onSuccess: invalidate,
  })

  const deleteMut = useMutation({
    mutationFn: (key: string) => deleteWorkspaceConfig(workspaceSlug, key),
    onSuccess: invalidate,
  })

  if (!isCurrent) {
    return (
      <Alert>
        <AlertDescription>
          {t("workspaceConfig.readonlyNotCurrent")}（
          <Link
            className="font-medium text-primary hover:underline"
            to="/workspaces"
          >
            {t("page.workspaces")}
          </Link>
          ）
        </AlertDescription>
      </Alert>
    )
  }

  const rows = effectiveQuery.data ?? []

  return (
    <section className="space-y-3 rounded-lg border bg-card p-4">
      <div>
        <h2 className="text-sm font-medium">{t("workspaceConfig.title")}</h2>
        <p className="text-xs text-muted-foreground">
          {t("workspaceConfig.description")}
        </p>
      </div>

      {effectiveQuery.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : null}

      {effectiveQuery.isError ? (
        <p className="text-sm text-destructive">{t("common.error")}</p>
      ) : null}

      <EffectiveConfigList
        rows={rows}
        scope="workspace"
        canManage={canManage}
        onSave={(key, value) => saveMut.mutateAsync({ key, value })}
        onRestore={(key) => deleteMut.mutateAsync(key)}
      />

      <AddConfigValueDialog
        rows={rows}
        scope="workspace"
        canManage={canManage}
        onSave={async (key, value) => {
          await saveMut.mutateAsync({ key, value })
        }}
      />
    </section>
  )
}
```

- [ ] **Step 2: typecheck**

Run: `pnpm --dir web typecheck`
Expected: PASS。

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/workspace-config-page.tsx
git commit -m "feat: 新增 WorkspaceConfigPage 页面"
```

---

### Task B5: 新增路由组件并注册路由

**Files:**
- Create: `web/src/routes/workspace/WorkspaceConfigRoute.tsx`
- Modify: `web/src/routes/router.tsx`（import 段 + route 定义 + routeTree 注册）

- [ ] **Step 1: 写路由组件 `WorkspaceConfigRoute.tsx`**

```tsx
import { useTranslation } from "react-i18next"
import { useParams } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { WorkspaceConfigPage } from "@/pages/workspace-config-page"

export function WorkspaceConfigRoute() {
  const params = useParams({ strict: false }) as { workspaceSlug: string }
  const { t } = useTranslation()
  return (
    <div className="space-y-4">
      <div>
        <Button
          onClick={() => window.history.back()}
          size="sm"
          type="button"
          variant="ghost"
        >
          ← {t("page.workspaces")}
        </Button>
        <h1 className="mt-2 text-xl font-semibold tracking-normal">
          {params.workspaceSlug}
        </h1>
      </div>
      <WorkspaceConfigPage workspaceSlug={params.workspaceSlug} />
    </div>
  )
}
```

> 说明：这里用轻量面包屑（返回按钮 + slug 标题）而非复用 `WorkspaceSettingsNav`，因为该 nav 的 `active` 只支持 `configDefinitions/customFields/projectTemplates`，与「从 /workspaces 列表进入」的语义不符，强行复用会污染 active 状态。

- [ ] **Step 2: 在 `router.tsx` 注册路由**

(a) 在 import 段（lazy import 区，参照现有 `lazyRoute(WorkspaceCustomFieldsRoute)` 的写法）新增：

```tsx
const WorkspaceConfigRoute = lazyRoute(WorkspaceConfigRouteComponent)
```

（确认 `lazyRoute` 的用法与现有写法一致——读 router.tsx 顶部 import 段，看 `WorkspaceCustomFieldsRoute` 是怎么 import + lazy 的，照抄。如果现有模式是 `import { WorkspaceCustomFieldsRoute } from "./workspace/WorkspaceCustomFieldsRoute"` 然后 `lazyRoute(WorkspaceCustomFieldsRoute)`，照此办理。）

(b) 在路由定义区（`workspaceCustomFieldsRoute` 定义之后，约第 493 行后）新增：

```tsx
const workspaceConfigRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/workspaces/$workspaceSlug/config",
  component: WorkspaceConfigRoute,
})
```

(c) 在 `routeTree` 的 `workspaceRootRoute.addChildren([...])`（约第 502 行）里加入 `workspaceConfigRoute,`。

- [ ] **Step 3: typecheck**

Run: `pnpm --dir web typecheck`
Expected: PASS。

- [ ] **Step 4: build 验证路由能编译**

Run: `pnpm --dir web build`
Expected: PASS（确认新路由被正确打进 bundle）。

- [ ] **Step 5: Commit**

```bash
git add web/src/routes/workspace/WorkspaceConfigRoute.tsx web/src/routes/router.tsx
git commit -m "feat: 注册 /workspaces/$workspaceSlug/config 路由"
```

---

### Task B6: `/workspaces` 列表行新增「配置」按钮

**Files:**
- Modify: `web/src/features/workspace/workspaces/workspace-console.tsx`
- Modify: `web/src/features/workspace/workspaces/workspace-console.test.tsx`

- [ ] **Step 1: 改 `workspace-console.tsx`**

顶部加 import：

```tsx
import { Link } from "@tanstack/react-router"
import { useMe } from "@/features/workspace/session/useMe"
```

在 `WorkspaceConsole` 函数体内（`const queryClient = useQueryClient()` 附近）取 effective slug：

```tsx
const me = useMe()
const effectiveSlug = me.data?.effective_workspace.slug
```

在 Actions 单元格（第 96-123 行）的 Archive/Restore 逻辑**之前**插入「配置」按钮（仅当前 effective 行显示）：

```tsx
<TableCell className="text-right">
  {ws.slug === effectiveSlug ? (
    <Link
      params={{ workspaceSlug: ws.slug }}
      to="/workspaces/$workspaceSlug/config"
    >
      <Button size="sm" variant="outline">
        {t("workspacesConsole.config")}
      </Button>
    </Link>
  ) : null}
  {ws.archived ? (
    /* ...原有 Restore 逻辑不变... */
```

注意：`<Link>` 内只能有一个根元素；`<Button>` 是唯一子元素即可。

- [ ] **Step 2: 更新测试 `workspace-console.test.tsx`**

现有 fetch spy 只返回 `/api/v1/workspaces`。需新增对 `/credentials/current` 的响应分支，让 `useMe` 拿到 `effective_workspace.slug = "acme"`，从而 `acme` 行显示「配置」按钮、`sandbox` 行不显示。

改 `beforeEach` 的 fetch mock 为按 url 分支：

```tsx
beforeEach(async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.includes("/credentials/current")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            data: {
              actor_type: "user",
              actor: { id: "u1", name: "U" },
              token: { type: "xuanchu_pat", scopes: ["config:write"] },
              effective_workspace: { slug: "acme", name: "ACME" },
              effective_role: "owner",
            },
          }),
          { status: 200 }
        )
      )
    }
    return Promise.resolve(
      new Response(
        JSON.stringify({
          data: [
            { id: "w1", slug: "acme", name: "ACME", archived: false },
            { id: "w2", slug: "sandbox", name: "Sandbox", archived: true },
          ],
        }),
        { status: 200 }
      )
    )
  })
  await i18n.changeLanguage("zh-CN")
})
```

由于引入了 `<Link>`，渲染需用 `renderWithRouter`。把 `renderConsole` 改为：

```tsx
import { renderWithRouter } from "@/test/router-wrapper"

function renderConsole(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderWithRouter(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <WorkspaceConsole canWrite={canWrite} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
```

新增用例：

```tsx
it("shows config link only on effective workspace row", async () => {
  renderConsole()
  await waitFor(() => {
    expect(screen.getByText("acme")).toBeTruthy()
  })
  // effective 行（acme）有「配置」链接
  const configLink = screen.getByRole("link", { name: /配置/ })
  expect(configLink.getAttribute("href")).toContain("/workspaces/acme/config")
  // 非当前行（sandbox）无「配置」链接（只有一个链接）
  const allConfigLinks = screen.getAllByRole("link", { name: /配置/ })
  expect(allConfigLinks.length).toBe(1)
})
```

- [ ] **Step 3: 运行测试**

Run: `pnpm --dir web test -- --run workspace-console`
Expected: PASS（含新增用例 + 原有归档用例不回归）。

- [ ] **Step 4: typecheck + lint**

Run: `pnpm --dir web typecheck && pnpm --dir web lint`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/workspaces/workspace-console.tsx web/src/features/workspace/workspaces/workspace-console.test.tsx
git commit -m "feat: /workspaces 行新增配置入口（仅当前 effective 行）"
```

---

### Task B7: 新页 `WorkspaceConfigPage` 测试

**Files:**
- Create: `web/src/pages/workspace-config-page.test.tsx`

- [ ] **Step 1: 写测试（fetch spy 风格 + renderWithRouter）**

参照 `project-config-tab.test.tsx` 的模式（fetch spy 按 url 分支 + `renderWithRouter` + `setWorkspaceToken`）。覆盖：
- 正常渲染 effective 行（含 workspace/default/missing 三种 source）。
- workspace scope 不可写的 key 显示只读（无编辑按钮）。
- URL slug ≠ effective slug 时渲染只读降级提示。

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { renderWithRouter } from "@/test/router-wrapper"

import { WorkspaceConfigPage } from "./workspace-config-page"

// 构造一个 effective view，source 可配置。
function effectiveRow(
  key: string,
  source: "workspace" | "default" | "missing",
  opts: { value?: string; allowedScopes?: string[]; workspaceValue?: string | null } = {}
) {
  return {
    key,
    value: opts.value ?? null,
    source,
    workspace_value: opts.workspaceValue ?? null,
    default_value: null,
    definition: {
      key,
      value_type: "string",
      allowed_scopes: opts.allowedScopes ?? ["workspace"],
      label: "",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: false,
      created_at: 0,
      modified_at: 0,
    },
    show_on_console_home: false,
    missing_required: false,
  }
}

describe("WorkspaceConfigPage", () => {
  beforeEach(async () => {
    setWorkspaceToken("xuanchu_pat_test")
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = typeof input === "string" ? input : (input as Request).url
      if (url.includes("/credentials/current")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              data: {
                actor_type: "user",
                actor: { id: "u1", name: "U" },
                token: { type: "xuanchu_pat", scopes: ["config:write"] },
                effective_workspace: { slug: "acme", name: "ACME" },
                effective_role: "owner",
              },
            }),
            { status: 200 }
          )
        )
      }
      if (url.includes("/config/effective")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              data: [
                effectiveRow("agent.provider.base_url", "workspace", { value: "https://a.example.com", allowedScopes: ["workspace"] }),
                effectiveRow("date.format", "default", { value: "YYYY-MM-DD", allowedScopes: ["workspace"] }),
                effectiveRow("uda.cost", "missing", { allowedScopes: ["project"] }),
              ],
            }),
            { status: 200 }
          )
        )
      }
      return Promise.resolve(new Response(JSON.stringify({ data: {} }), { status: 200 }))
    })
    await i18n.changeLanguage("zh-CN")
  })

  function renderPage(slug = "acme") {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return renderWithRouter(
      <QueryClientProvider client={client}>
        <ThemeProvider>
          <TooltipProvider>
            <WorkspaceConfigPage workspaceSlug={slug} />
          </TooltipProvider>
        </ThemeProvider>
      </QueryClientProvider>
    )
  }

  it("renders effective rows for the current workspace", async () => {
    renderPage("acme")
    await waitFor(() => {
      expect(screen.getByText("agent.provider.base_url")).toBeTruthy()
    })
    expect(screen.getByText("date.format")).toBeTruthy()
  })

  it("marks project-only keys as readonly", async () => {
    renderPage("acme")
    await waitFor(() => {
      expect(screen.getByText("uda.cost")).toBeTruthy()
    })
    // uda.cost 的 allowed_scopes 不含 workspace → 只读，无「编辑」按钮
    // 找到 uda.cost 所在行区域，断言其没有「编辑」按钮（通过 role + name 在该行内查找）
    expect(screen.queryByRole("button", { name: /编辑/ })).toBeNull()
    // 同时应显示只读 badge（i18n key configDefinitions.statusReadonly 的中文）
  })

  it("shows readonly notice when slug is not the effective workspace", async () => {
    renderPage("other")
    await waitFor(() => {
      expect(screen.getByText(/仅可在当前工作空间上下文编辑配置/)).toBeTruthy()
    })
  })
})
```

> 注意：第二个用例「project-only keys as readonly」的断言（`queryByRole("button", { name: /编辑/ })` 为 null）只有在**所有行**都不可写时才严格成立。上面数据里 `agent.provider.base_url` 是 workspace 可写的、会有「编辑」按钮。请把该用例的数据调整为只含 `uda.cost`（project-only）这一行，或把断言改为：在 `uda.cost` 行的容器内没有「编辑」按钮。实现时按实际 DOM 结构调整，核心意图是「project-only 的 key 在 workspace 页显示只读」。

- [ ] **Step 2: 运行测试**

Run: `pnpm --dir web test -- --run workspace-config-page`
Expected: PASS。

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/workspace-config-page.test.tsx
git commit -m "test: WorkspaceConfigPage effective 行与只读降级测试"
```

---

## 收尾：全量验证与文档

### Task C1: 全量验证

- [ ] **Step 1: 前端全量验证**

Run:
```bash
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web test
```
Expected: 全部 PASS。

- [ ] **Step 2: 后端全量验证（确认零回归）**

Run:
```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全部 PASS（本次为纯前端改动，后端应无影响）。

- [ ] **Step 3: 手动集成验证（启动 dev 环境按 spec §4.4 / §5.2 / §5.6 原型核对）**

- `/workspaces`：当前 effective 行有「配置」按钮，点击跳转 `/workspaces/{slug}/config`；非当前行无按钮。
- `/workspaces/{slug}/config`：列出 effective 配置行，可编辑 workspace scope 的 key，project-only 的 key 只读。
- 项目自动化页：当 workspace 已配 provider 时不再展开卡片；未配时展开且含「写入 project 级」声明 + workspace 配置链接，链接跳转正确。

---

### Task C2: 文档同步

**Files:**
- Modify: `README.md`（如「配置」入口属用户可见行为变化）

- [ ] **Step 1: 评估并在 README 补充 workspace config 编辑入口说明**

若 README 有「Web Console 功能」或「配置」相关章节，补充一句：workspace 级配置值可在 `/workspaces` 列表对应行的「配置」入口编辑。如 README 无相关章节则跳过（避免为改而改）。

- [ ] **Step 2: Commit（如有改动）**

```bash
git add README.md
git commit -m "docs: 补充 workspace 配置入口说明"
```

---

## 自检清单（实现者完成后逐项确认）

- [ ] 改动线 A：展开逻辑改用 `useProjectAutomationProviderConfig` 的 `complete`，`isProviderConfigComplete`/`providerConfigFromEntries` 如不再被引用已删除。
- [ ] 改动线 A：卡片含「写入 project 级」声明 + workspace 配置链接。
- [ ] 改动线 B：`EffectiveConfigList`/`AddConfigValueDialog` 已抽离，`ProjectConfigTab` 复用且原有测试不回归。
- [ ] 改动线 B：新路由 `/workspaces/$workspaceSlug/config` 已注册，页面可访问。
- [ ] 改动线 B：「配置」按钮仅当前 effective 行显示，非当前行不渲染。
- [ ] 改动线 B：URL slug ≠ effective slug 时页面降级只读。
- [ ] 全量 `typecheck && lint && build && test` 通过。
- [ ] 后端 `go test ./...` 与 `CGO_ENABLED=0` 构建通过。
