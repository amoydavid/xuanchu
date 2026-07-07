# 项目设置页重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `/projects/<slug>/settings` 重构为 tanstack 父子子路由驱动的两个 tab（配置项 / 项目备注），配置项按 schema 增强、备注 timeline 化，删除与首页重复的 section。

**Architecture:** 设置页拆成 layout 父路由 + config/notes 两个子路由，tab 激活态由 URL pathname 推导；配置项子页并行拉取 project config 值与 workspace 级 `ConfigDefinition` schema，按 schema 类型渲染 shadcn 输入控件并支持 inline 编辑；备注子页用圆点轴 timeline 渲染 annotation，支持新增/删除。后端不动。

**Tech Stack:** React + TypeScript + @tanstack/react-router + @tanstack/react-query + shadcn/ui (Tabs / Select / Input / Button / Dialog / Badge) + react-i18next + Vitest + jsdom。

---

## 文件结构

```
web/src/
  routes/
    router.tsx                                    # 修改：替换单条 projectSettingsRoute 为父子三条 + index 重定向
    workspace/
      ProjectSettingsLayoutRoute.tsx              # 新建：父 route，取 params/me，渲染 layout 组件
      ProjectSettingsConfigRoute.tsx              # 新建：配置项子 route
      ProjectSettingsNotesRoute.tsx               # 新建：备注子 route
  pages/
    project-settings-layout.tsx                   # 新建：面包屑 + Tabs(Outlet) + closed banner
    project-config-tab.tsx                        # 新建：配置项 tab 内容（schema 增强 + inline 编辑 + 新增）
    project-notes-tab.tsx                         # 新建：备注 tab 内容（timeline + 新增/删除）
    project-settings-page.tsx                     # 删除（职责已拆分）
  routes/workspace/
    ProjectSettingsRoute.tsx                      # 删除（被 LayoutRoute 取代）
  features/workspace/project-workbench/
    api/
      project-api.ts                              # 修改：导出 normalizeProjectConfigEntries
      config-schema-api.ts                        # 新建：listConfigSchema + 类型
      config-schema-api.test.ts                   # 新建
      project-api.test.ts                         # 新建：归一化回归测试
    project/
      project-config-row.tsx                      # 新建：单行配置项（展示态 + inline 编辑态）
      project-notes-timeline.tsx                  # 新建：备注 timeline 组件
      project-notes-timeline.test.tsx             # 新建
  locales/
    zh-CN.ts                                      # 修改：清理/补充 projectSettings 键
    en-US.ts                                      # 修改：同上
```

---

### Task 1: config-schema 前端 API

**Files:**
- Create: `web/src/features/workspace/project-workbench/api/config-schema-api.ts`
- Test: `web/src/features/workspace/project-workbench/api/config-schema-api.test.ts`

- [ ] **Step 1: 写失败测试**

创建 `web/src/features/workspace/project-workbench/api/config-schema-api.test.ts`：

```ts
import { describe, expect, it } from "vitest"

import { configSchemaPath, type ConfigSchemaDefinition } from "./config-schema-api"

describe("config schema api", () => {
  it("builds workspace-scoped config schema list path", () => {
    expect(configSchemaPath()).toBe("/api/v1/config-schema")
  })

  it("exposes ConfigSchemaDefinition shape", () => {
    const def: ConfigSchemaDefinition = {
      key: "agent.background",
      value_type: "string",
      allowed_scopes: ["project"],
      label: "",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      created_at: 0,
      modified_at: 0,
    }
    expect(def.key).toBe("agent.background")
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && pnpm vitest run src/features/workspace/project-workbench/api/config-schema-api.test.ts`
Expected: FAIL（模块不存在）

- [ ] **Step 3: 写实现**

创建 `web/src/features/workspace/project-workbench/api/config-schema-api.ts`：

```ts
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

// 对应后端 app.ConfigDefinitionView。
export type ConfigSchemaDefinition = {
  key: string
  value_type: string
  allowed_scopes: string[]
  label: string
  description: string
  enum_values: string[]
  default_value: string | null
  required: boolean
  secret: boolean
  created_at: number
  modified_at: number
}

export function configSchemaPath(): string {
  return "/api/v1/config-schema"
}

export function listConfigSchema(): Promise<ConfigSchemaDefinition[]> {
  return workspaceApiGet<ConfigSchemaDefinition[]>(configSchemaPath())
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && pnpm vitest run src/features/workspace/project-workbench/api/config-schema-api.test.ts`
Expected: PASS

- [ ] **Step 5: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误

- [ ] **Step 6: 提交**

```bash
git add web/src/features/workspace/project-workbench/api/config-schema-api.ts web/src/features/workspace/project-workbench/api/config-schema-api.test.ts
git commit -m "feat: 新增 config-schema 前端 API"
```

---

### Task 2: project config 归一化回归测试

**Files:**
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts`（导出 `normalizeProjectConfigEntries`）
- Test: `web/src/features/workspace/project-workbench/api/project-api.test.ts`（新建）

- [ ] **Step 1: 写失败测试**

创建 `web/src/features/workspace/project-workbench/api/project-api.test.ts`：

```ts
import { describe, expect, it } from "vitest"

import { normalizeProjectConfigEntries } from "./project-api"

describe("normalizeProjectConfigEntries", () => {
  it("归一化后端返回的对象形态为按 key 排序的数组", () => {
    const entries = normalizeProjectConfigEntries({
      "agent.background": "Owns MCP",
      "ads.roi": "1.8",
    })
    expect(entries).toEqual([
      { key: "ads.roi", value: "1.8" },
      { key: "agent.background", value: "Owns MCP" },
    ])
  })

  it("已是数组时原样返回", () => {
    const input = [
      { key: "agent.background", value: "Owns MCP" },
    ]
    expect(normalizeProjectConfigEntries(input)).toBe(input)
  })

  it("null/undefined 返回空数组", () => {
    expect(normalizeProjectConfigEntries(null)).toEqual([])
    expect(normalizeProjectConfigEntries(undefined)).toEqual([])
  })

  it("非对象原始值返回空数组", () => {
    expect(normalizeProjectConfigEntries("hello")).toEqual([])
    expect(normalizeProjectConfigEntries(42)).toEqual([])
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && pnpm vitest run src/features/workspace/project-workbench/api/project-api.test.ts`
Expected: FAIL（`normalizeProjectConfigEntries` 未导出）

- [ ] **Step 3: 导出函数**

在 `web/src/features/workspace/project-workbench/api/project-api.ts` 中，把 `function normalizeProjectConfigEntries` 改为 `export function normalizeProjectConfigEntries`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && pnpm vitest run src/features/workspace/project-workbench/api/project-api.test.ts`
Expected: PASS（4 tests）

- [ ] **Step 5: 提交**

```bash
git add web/src/features/workspace/project-workbench/api/project-api.ts web/src/features/workspace/project-workbench/api/project-api.test.ts
git commit -m "test: 覆盖 project config 归一化（回归 entries.map 崩溃）"
```

---

### Task 3: i18n 文案调整

**Files:**
- Modify: `web/src/locales/zh-CN.ts`（`projectSettings` 块）
- Modify: `web/src/locales/en-US.ts`（`projectSettings` 块）

- [ ] **Step 1: 更新 zh-CN**

在 `web/src/locales/zh-CN.ts` 中，找到 `projectSettings: {` 块（约 479 行），替换为：

```ts
  projectSettings: {
    title: "项目设置",
    tabConfig: "配置项",
    tabNotes: "项目备注",
    configTitle: "配置项",
    configDescription: "管理项目级配置。key 合法性由 workspace schema 决定。",
    configEmpty: "暂无配置项",
    configKey: "key",
    configValue: "value",
    configEdit: "编辑",
    configSave: "保存",
    configCancel: "取消",
    configDelete: "删除",
    configDeleteConfirm: "确认删除该配置项？",
    configAdd: "新增配置项",
    configKeyExists: "该 key 已存在，请直接编辑对应行",
    configNoSchema: "该 key 未在 workspace schema 注册",
    configSecret: "敏感配置",
    configReveal: "显示",
    configHide: "隐藏",
    notesTitle: "项目备注",
    notesEmpty: "暂无备注",
    noteDelete: "删除",
    noteDeleteConfirm: "确认删除该备注？",
    noteNew: "新增备注",
    noteAdd: "添加备注",
    noteClosedReadonly: "项目已关闭，备注只读",
    configClosedReadonly: "项目已关闭，配置项只读",
  },
```

- [ ] **Step 2: 更新 en-US**

在 `web/src/locales/en-US.ts` 中，找到 `projectSettings: {` 块（约 493 行），替换为对应的英文键：

```ts
  projectSettings: {
    title: "Project Settings",
    tabConfig: "Config",
    tabNotes: "Notes",
    configTitle: "Config",
    configDescription: "Manage project-scoped config. Keys are validated against workspace schema.",
    configEmpty: "No config entries",
    configKey: "key",
    configValue: "value",
    configEdit: "Edit",
    configSave: "Save",
    configCancel: "Cancel",
    configDelete: "Delete",
    configDeleteConfirm: "Delete this config entry?",
    configAdd: "Add config",
    configKeyExists: "This key already exists; edit the existing row instead",
    configNoSchema: "This key is not registered in workspace schema",
    configSecret: "Secret",
    configReveal: "Show",
    configHide: "Hide",
    notesTitle: "Notes",
    notesEmpty: "No notes",
    noteDelete: "Delete",
    noteDeleteConfirm: "Delete this note?",
    noteNew: "Add a note",
    noteAdd: "Add note",
    noteClosedReadonly: "Project is closed; notes are read-only",
    configClosedReadonly: "Project is closed; config is read-only",
  },
```

- [ ] **Step 3: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误（i18n 键被引用前先存在，避免后续组件用到时报错）

- [ ] **Step 4: 提交**

```bash
git add web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "i18n: 调整 projectSettings 文案以适配 tab 重构"
```

---

### Task 4: 备注 timeline 组件

**Files:**
- Create: `web/src/features/workspace/project-workbench/project/project-notes-timeline.tsx`
- Test: `web/src/features/workspace/project-workbench/project/project-notes-timeline.test.tsx`

- [ ] **Step 1: 写失败测试**

创建 `web/src/features/workspace/project-workbench/project/project-notes-timeline.test.tsx`：

```tsx
import { describe, expect, it, vi } from "vitest"
import { render, screen, fireEvent } from "@testing-library/react"

import { ProjectNotesTimeline } from "./project-notes-timeline"

describe("ProjectNotesTimeline", () => {
  it("空数组渲染空状态", () => {
    render(
      <ProjectNotesTimeline
        entries={[]}
        canManage={false}
        onDelete={() => {}}
        emptyLabel="暂无备注"
      />
    )
    expect(screen.getByText("暂无备注")).toBeInTheDocument()
  })

  it("按时间倒序渲染节点", () => {
    const entries = [
      {
        id: "a",
        project_id: "p",
        entry: 1000,
        content: "第一条",
        created_by: { id: "u1", name: "alice" },
        created_at: 1000,
      },
      {
        id: "b",
        project_id: "p",
        entry: 2000,
        content: "第二条",
        created_by: { id: "u2", name: "bob" },
        created_at: 2000,
      },
    ]
    render(
      <ProjectNotesTimeline
        entries={entries}
        canManage={false}
        onDelete={() => {}}
        emptyLabel="暂无备注"
      />
    )
    const nodes = screen.getAllByRole("listitem")
    expect(nodes).toHaveLength(2)
    // 倒序：第二条在前
    expect(nodes[0]).toHaveTextContent("第二条")
    expect(nodes[1]).toHaveTextContent("第一条")
  })

  it("canManage 时展示删除按钮并触发回调", () => {
    const onDelete = vi.fn()
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true)
    render(
      <ProjectNotesTimeline
        entries={[
          {
            id: "a",
            project_id: "p",
            entry: 1000,
            content: "hello",
            created_by: { id: "u1", name: "alice" },
            created_at: 1000,
          },
        ]}
        canManage={true}
        onDelete={onDelete}
        emptyLabel="暂无备注"
        deleteLabel="删除"
        deleteConfirm="确认删除该备注？"
      />
    )
    fireEvent.click(screen.getByText("删除"))
    expect(confirmSpy).toHaveBeenCalled()
    expect(onDelete).toHaveBeenCalledWith("a")
    confirmSpy.mockRestore()
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && pnpm vitest run src/features/workspace/project-workbench/project/project-notes-timeline.test.tsx`
Expected: FAIL（组件不存在）

- [ ] **Step 3: 写实现**

创建 `web/src/features/workspace/project-workbench/project/project-notes-timeline.tsx`：

```tsx
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { MarkdownView } from "@/components/markdown"
import type { ProjectAnnotationInfo } from "../api/project-api"

type ProjectNotesTimelineProps = {
  entries: ProjectAnnotationInfo[]
  canManage: boolean
  onDelete: (id: string) => void
  emptyLabel: string
  deleteLabel?: string
  deleteConfirm?: string
}

export function ProjectNotesTimeline({
  entries,
  canManage,
  onDelete,
  emptyLabel,
  deleteLabel,
  deleteConfirm,
}: ProjectNotesTimelineProps) {
  const { t } = useTranslation()
  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground">{emptyLabel}</p>
  }
  const ordered = [...entries].sort((a, b) => b.entry - a.entry)
  return (
    <ol className="space-y-2">
      {ordered.map((ann) => (
        <li
          className="relative border-l border-border pl-4 pb-3 last:pb-0"
          key={ann.id}
          role="listitem"
        >
          <span className="absolute -left-[5px] top-1 size-2 rounded-full bg-foreground ring-2 ring-card" />
          <div className="mb-1 text-xs text-muted-foreground">
            {ann.created_by?.name ?? "-"} · {formatTime(ann.entry)}
          </div>
          <div className="rounded border bg-background p-3 text-sm">
            <MarkdownView>{ann.content}</MarkdownView>
          </div>
          {canManage ? (
            <div className="mt-1 text-right">
              <Button
                onClick={() => {
                  if (window.confirm(deleteConfirm ?? t("projectSettings.noteDeleteConfirm"))) {
                    onDelete(ann.id)
                  }
                }}
                size="sm"
                variant="outline"
              >
                {deleteLabel ?? t("common.delete")}
              </Button>
            </div>
          ) : null}
        </li>
      ))}
    </ol>
  )
}

function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  const hh = String(d.getHours()).padStart(2, "0")
  const mm = String(d.getMinutes()).padStart(2, "0")
  return `${y}-${m}-${day} ${hh}:${mm}`
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && pnpm vitest run src/features/workspace/project-workbench/project/project-notes-timeline.test.tsx`
Expected: PASS（3 tests）

- [ ] **Step 5: 提交**

```bash
git add web/src/features/workspace/project-workbench/project/project-notes-timeline.tsx web/src/features/workspace/project-workbench/project/project-notes-timeline.test.tsx
git commit -m "feat: 新增项目备注 timeline 组件"
```

---

### Task 5: 配置项单行组件（展示 + inline 编辑）

**Files:**
- Create: `web/src/features/workspace/project-workbench/project/project-config-row.tsx`

- [ ] **Step 1: 写实现**

创建 `web/src/features/workspace/project-workbench/project/project-config-row.tsx`：

```tsx
import { useState } from "react"
import { EyeIcon, EyeOffIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import type { ProjectConfigEntry } from "../api/project-api"
import type { ConfigSchemaDefinition } from "../api/config-schema-api"

type ProjectConfigRowProps = {
  entry: ProjectConfigEntry
  schema?: ConfigSchemaDefinition
  canManage: boolean
  onSave: (value: string) => Promise<void>
  onDelete: () => void
}

export function ProjectConfigRow({
  entry,
  schema,
  canManage,
  onSave,
  onDelete,
}: ProjectConfigRowProps) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(entry.value)
  const [revealed, setRevealed] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isSecret = schema?.secret === true
  const enumValues = schema?.enum_values ?? []

  const startEdit = () => {
    setDraft(entry.value)
    setError(null)
    setEditing(true)
  }

  const cancel = () => {
    setEditing(false)
    setError(null)
  }

  const save = async () => {
    setSaving(true)
    setError(null)
    try {
      await onSave(draft)
      setEditing(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setSaving(false)
    }
  }

  if (editing) {
    return (
      <div className="space-y-2 border-b py-2">
        <div className="flex items-center justify-between gap-2">
          <code className="text-xs">{entry.key}</code>
          {schema ? null : (
            <span className="text-xs text-muted-foreground">
              {t("projectSettings.configNoSchema")}
            </span>
          )}
        </div>
        {enumValues.length > 0 ? (
          <Select onValueChange={setDraft} value={draft}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {enumValues.map((v) => (
                <SelectItem key={v} value={v}>
                  {v}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <Input
            onChange={(e) => setDraft(e.target.value)}
            type={isSecret ? "password" : "text"}
            value={draft}
          />
        )}
        {schema?.description ? (
          <p className="text-xs text-muted-foreground">{schema.description}</p>
        ) : null}
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
        <div className="flex gap-2">
          <Button disabled={saving} onClick={save} size="sm" type="button">
            {t("projectSettings.configSave")}
          </Button>
          <Button onClick={cancel} size="sm" type="button" variant="outline">
            {t("projectSettings.configCancel")}
          </Button>
        </div>
      </div>
    )
  }

  const displayValue =
    isSecret && !revealed ? "••••••" : entry.value

  return (
    <div className="flex items-center justify-between gap-2 border-b py-1 text-sm last:border-b-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <code className="text-xs">{entry.key}</code>
          {isSecret ? (
            <span className="text-xs text-muted-foreground">
              {t("projectSettings.configSecret")}
            </span>
          ) : null}
          {schema ? null : (
            <span className="text-xs text-muted-foreground">
              {t("projectSettings.configNoSchema")}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          <span className="truncate text-muted-foreground">{displayValue}</span>
          {isSecret ? (
            <Button
              aria-label={revealed ? t("projectSettings.configHide") : t("projectSettings.configReveal")}
              onClick={() => setRevealed((v) => !v)}
              size="icon-sm"
              type="button"
              variant="ghost"
            >
              {revealed ? <EyeOffIcon /> : <EyeIcon />}
            </Button>
          ) : null}
        </div>
      </div>
      {canManage ? (
        <div className="flex shrink-0 gap-1">
          <Button onClick={startEdit} size="sm" type="button" variant="outline">
            {t("projectSettings.configEdit")}
          </Button>
          <Button
            onClick={() => {
              if (window.confirm(t("projectSettings.configDeleteConfirm"))) {
                onDelete()
              }
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            {t("common.delete")}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
```

- [ ] **Step 2: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误

- [ ] **Step 3: 提交**

```bash
git add web/src/features/workspace/project-workbench/project/project-config-row.tsx
git commit -m "feat: 新增配置项单行组件（schema 感知 + inline 编辑）"
```

---

### Task 6: 配置项 tab 页

**Files:**
- Create: `web/src/pages/project-config-tab.tsx`

- [ ] **Step 1: 写实现**

创建 `web/src/pages/project-config-tab.tsx`：

```tsx
import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import { ProjectConfigRow } from "@/features/workspace/project-workbench/project/project-config-row"
import {
  deleteProjectConfig,
  listProjectConfig,
  setProjectConfig,
  type ProjectConfigEntry,
} from "@/features/workspace/project-workbench/api/project-api"
import { listConfigSchema } from "@/features/workspace/project-workbench/api/config-schema-api"

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
  const configQuery = useQuery<ProjectConfigEntry[]>({
    queryKey: ["project", workspaceSlug, projectSlug, "config"],
    queryFn: () => listProjectConfig(workspaceSlug, projectSlug),
  })
  const schemaQuery = useQuery({
    queryKey: ["config-schema", workspaceSlug],
    queryFn: () => listConfigSchema(),
  })

  const schemaMap = useMemo(() => {
    const m = new Map<string, ReturnType<typeof Object.fromEntries>>()
    for (const def of schemaQuery.data ?? []) {
      m.set(def.key, def)
    }
    return m
  }, [schemaQuery.data])

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["project", workspaceSlug, projectSlug, "config"],
    })

  const saveMut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setProjectConfig(workspaceSlug, projectSlug, key, value),
    onSuccess: invalidate,
  })

  const delMut = useMutation({
    mutationFn: (key: string) =>
      deleteProjectConfig(workspaceSlug, projectSlug, key),
    onSuccess: invalidate,
  })

  // 新增表单状态
  const [newKey, setNewKey] = useState("")
  const [newValue, setNewValue] = useState("")
  const [addError, setAddError] = useState<string | null>(null)

  const existingKeys = useMemo(
    () => new Set((configQuery.data ?? []).map((e) => e.key)),
    [configQuery.data]
  )
  const newSchema = newKey.trim() ? schemaMap.get(newKey.trim()) : undefined
  const newEnumValues = newSchema?.enum_values ?? []
  const newIsSecret = newSchema?.secret === true
  const keyDuplicate = existingKeys.has(newKey.trim())

  const submitAdd = async () => {
    const key = newKey.trim()
    if (!key) return
    setAddError(null)
    try {
      await setProjectConfig(workspaceSlug, projectSlug, key, newValue)
      setNewKey("")
      setNewValue("")
      await invalidate()
    } catch (err) {
      setAddError(err instanceof ApiError ? err.message : t("common.error"))
    }
  }

  return (
    <section className="space-y-3 border bg-card p-4">
      <div>
        <h2 className="text-sm font-medium">{t("projectSettings.configTitle")}</h2>
        <p className="text-xs text-muted-foreground">
          {t("projectSettings.configDescription")}
        </p>
      </div>

      {closed ? (
        <Alert>
          <AlertDescription>
            {t("projectSettings.configClosedReadonly")}
          </AlertDescription>
        </Alert>
      ) : null}

      {configQuery.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : (configQuery.data ?? []).length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("projectSettings.configEmpty")}</p>
      ) : (
        <div>
          {(configQuery.data ?? []).map((entry) => (
            <ProjectConfigRow
              canManage={canManage && !closed}
              entry={entry}
              key={entry.key}
              onDelete={() => delMut.mutate(entry.key)}
              onSave={async (value) => {
                await saveMut.mutateAsync({ key: entry.key, value })
              }}
              schema={schemaMap.get(entry.key)}
            />
          ))}
        </div>
      )}

      {canManage && !closed ? (
        <form
          className="grid gap-2 border-t pt-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (keyDuplicate) return
            void submitAdd()
          }}
        >
          <h3 className="text-sm font-medium">{t("projectSettings.configAdd")}</h3>
          <Input
          aria-label={t("projectSettings.configKey")}
          onChange={(e) => setNewKey(e.target.value)}
          placeholder={t("projectSettings.configKey")}
          value={newKey}
        />
          {newSchema?.description ? (
            <p className="text-xs text-muted-foreground">{newSchema.description}</p>
          ) : null}
          {newEnumValues.length > 0 ? (
            <Select onValueChange={setNewValue} value={newValue}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t("projectSettings.configValue")} />
              </SelectTrigger>
              <SelectContent>
                {newEnumValues.map((v) => (
                  <SelectItem key={v} value={v}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              aria-label={t("projectSettings.configValue")}
              onChange={(e) => setNewValue(e.target.value)}
              placeholder={t("projectSettings.configValue")}
              type={newIsSecret ? "password" : "text"}
              value={newValue}
            />
          )}
          {keyDuplicate ? (
            <p className="text-xs text-destructive">
              {t("projectSettings.configKeyExists")}
            </p>
          ) : null}
          {!newSchema && newKey.trim() ? (
            <p className="text-xs text-muted-foreground">
              {t("projectSettings.configNoSchema")}
            </p>
          ) : null}
          {addError ? (
            <Alert variant="destructive">
              <AlertDescription>{addError}</AlertDescription>
            </Alert>
          ) : null}
          <Button disabled={savingDisabled(keyDuplicate, newKey)} size="sm" type="submit">
            {t("common.save")}
          </Button>
        </form>
      ) : null}
    </section>
  )
}

function savingDisabled(keyDuplicate: boolean, newKey: string): boolean {
  return keyDuplicate || !newKey.trim()
}
```

- [ ] **Step 2: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误

- [ ] **Step 3: 提交**

```bash
git add web/src/pages/project-config-tab.tsx
git commit -m "feat: 新增配置项 tab 页（schema 增强 + inline 编辑 + 新增）"
```

---

### Task 7: 项目备注 tab 页

**Files:**
- Create: `web/src/pages/project-notes-tab.tsx`

- [ ] **Step 1: 写实现**

创建 `web/src/pages/project-notes-tab.tsx`：

```tsx
import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api"
import { ProjectNotesTimeline } from "@/features/workspace/project-workbench/project/project-notes-timeline"
import {
  addProjectAnnotation,
  deleteProjectAnnotation,
  listProjectAnnotations,
} from "@/features/workspace/project-workbench/api/project-api"

type ProjectNotesTabProps = {
  canManage: boolean
  closed: boolean
  projectSlug: string
  workspaceSlug: string
}

export function ProjectNotesTab({
  canManage,
  closed,
  projectSlug,
  workspaceSlug,
}: ProjectNotesTabProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ["project", workspaceSlug, projectSlug, "annotations"],
    queryFn: () => listProjectAnnotations(workspaceSlug, projectSlug),
  })
  const [content, setContent] = useState("")
  const [error, setError] = useState<string | null>(null)

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["project", workspaceSlug, projectSlug, "annotations"],
    })

  const addMut = useMutation({
    mutationFn: (text: string) =>
      addProjectAnnotation(workspaceSlug, projectSlug, text),
    onSuccess: () => {
      setContent("")
      setError(null)
      void invalidate()
    },
    onError: (err) => {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    },
  })

  const delMut = useMutation({
    mutationFn: (id: string) =>
      deleteProjectAnnotation(workspaceSlug, projectSlug, id),
    onSuccess: invalidate,
  })

  return (
    <section className="space-y-3 border bg-card p-4">
      <h2 className="text-sm font-medium">{t("projectSettings.notesTitle")}</h2>

      {closed ? (
        <Alert>
          <AlertDescription>
            {t("projectSettings.noteClosedReadonly")}
          </AlertDescription>
        </Alert>
      ) : null}

      {query.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : (
        <ProjectNotesTimeline
          canManage={canManage && !closed}
          onDelete={(id) => delMut.mutate(id)}
          emptyLabel={t("projectSettings.notesEmpty")}
          entries={query.data ?? []}
        />
      )}

      {canManage && !closed ? (
        <form
          className="grid gap-2 border-t pt-3"
          onSubmit={(e) => {
            e.preventDefault()
            const text = content.trim()
            if (!text) return
            addMut.mutate(text)
          }}
        >
          <Label htmlFor="project-note-content">
            {t("projectSettings.noteNew")}
          </Label>
          <Textarea
            id="project-note-content"
            onChange={(e) => setContent(e.target.value)}
            rows={3}
            value={content}
          />
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <Button disabled={addMut.isPending} size="sm" type="submit">
            {t("projectSettings.noteAdd")}
          </Button>
        </form>
      ) : null}
    </section>
  )
}
```

- [ ] **Step 2: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误

- [ ] **Step 3: 提交**

```bash
git add web/src/pages/project-notes-tab.tsx
git commit -m "feat: 新增项目备注 tab 页（timeline + 新增/删除）"
```

---

### Task 8: 设置页 layout（Tabs + Outlet）

**Files:**
- Create: `web/src/pages/project-settings-layout.tsx`

- [ ] **Step 1: 写实现**

创建 `web/src/pages/project-settings-layout.tsx`：

```tsx
import { useState } from "react"
import { Outlet, useNavigate, useRouterState } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"

type ProjectSettingsLayoutProps = {
  canManage: boolean
  closed: boolean
  projectSlug: string
  workspaceSlug: string
}

export function ProjectSettingsLayout({
  canManage,
  closed,
  projectSlug,
  workspaceSlug,
}: ProjectSettingsLayoutProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const activeTab = pathname.endsWith("/notes") ? "notes" : "config"

  const onValueChange = (value: string) => {
    const suffix = value === "notes" ? "notes" : "config"
    void navigate({
      to: "/projects/$projectSlug/settings/$tab",
      params: { projectSlug, tab: suffix },
    })
  }

  return (
    <div className="max-w-3xl space-y-4">
      <div>
        <nav className="text-xs text-muted-foreground">
          <a
            className="hover:text-foreground"
            href={`/workspaces/${workspaceSlug}/projects/${projectSlug}`}
          >
            {projectSlug}
          </a>
          {" / "}
          <span>{t("projectSettings.title")}</span>
        </nav>
        <h1 className="mt-2 text-xl font-semibold tracking-normal">
          {t("projectSettings.title")}
        </h1>
      </div>

      <Tabs onValueChange={onValueChange} value={activeTab}>
        <TabsList>
          <TabsTrigger value="config">{t("projectSettings.tabConfig")}</TabsTrigger>
          <TabsTrigger value="notes">{t("projectSettings.tabNotes")}</TabsTrigger>
        </TabsList>
      </Tabs>

      <Outlet />
    </div>
  )
}
```

> 注：子页面内容通过 `<Outlet />` 渲染（由子路由提供），Tabs 这里只用 `TabsList` 做导航触发器，不放 `TabsContent`——content 由路由驱动。

- [ ] **Step 2: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误

- [ ] **Step 3: 提交**

```bash
git add web/src/pages/project-settings-layout.tsx
git commit -m "feat: 新增项目设置 layout（Tabs 导航 + Outlet）"
```

---

### Task 9: 路由文件（layout + config + notes + index 重定向）

**Files:**
- Create: `web/src/routes/workspace/ProjectSettingsLayoutRoute.tsx`
- Create: `web/src/routes/workspace/ProjectSettingsConfigRoute.tsx`
- Create: `web/src/routes/workspace/ProjectSettingsNotesRoute.tsx`
- Delete: `web/src/routes/workspace/ProjectSettingsRoute.tsx`

- [ ] **Step 1: 创建 layout route**

创建 `web/src/routes/workspace/ProjectSettingsLayoutRoute.tsx`：

```tsx
import { useParams } from "@tanstack/react-router"

import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { ProjectSettingsLayout } from "@/pages/project-settings-layout"
import { isClosedProjectStatus } from "@/features/workspace/project-workbench/project/project-status-menu"
import { canProjectManage } from "@/features/workspace/project-workbench/permissions/permissions"

export function ProjectSettingsLayoutRoute() {
  const params = useParams({ strict: false }) as { projectSlug: string }
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug

  if (me.isError) {
    return (
      <section className="border bg-card p-6 text-sm text-destructive">
        {me.error instanceof ApiError ? me.error.code : "unknown"}
      </section>
    )
  }
  if (!workspaceSlug || !me.data) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }

  // closed 状态需要 project 数据；layout 无法在拿到 project 前确定 closed。
  // 这里把 closed 判断下沉到各子页（子页自己拉 project 数据），layout 只传 canManage。
  return (
    <ProjectSettingsLayout
      canManage={canProjectManage({
        role: me.data.effective_role,
        scopes: me.data.token.scopes,
      })}
      closed={false}
      projectSlug={params.projectSlug}
      workspaceSlug={workspaceSlug}
    />
  )
}
```

> 说明：layout 把 `closed={false}` 占位，真正的 closed 判断在 config/notes 子页内拉取 project 数据后决定（见 Task 10/11）。layout 的 `closed` prop 仅保留接口一致性，子页各自覆盖。

- [ ] **Step 2: 创建 config route**

创建 `web/src/routes/workspace/ProjectSettingsConfigRoute.tsx`：

```tsx
import { useParams } from "@tanstack/react-router"

import { useProjectQuery } from "@/features/workspace/project-workbench/hooks/use-project-data"
import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { ProjectConfigTab } from "@/pages/project-config-tab"
import { isClosedProjectStatus } from "@/features/workspace/project-workbench/project/project-status-menu"
import { canProjectManage } from "@/features/workspace/project-workbench/permissions/permissions"

export function ProjectSettingsConfigRoute() {
  const params = useParams({ strict: false }) as { projectSlug: string }
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug
  const project = useProjectQuery(workspaceSlug ?? "", params.projectSlug)

  if (!workspaceSlug || !me.data) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }
  if (project.isError) {
    return (
      <section className="border bg-card p-6 text-sm text-destructive">
        {project.error instanceof ApiError ? project.error.message : "error"}
      </section>
    )
  }
  if (project.isPending) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }

  return (
    <ProjectConfigTab
      canManage={canProjectManage({
        role: me.data.effective_role,
        scopes: me.data.token.scopes,
      })}
      closed={isClosedProjectStatus(project.data.status)}
      projectSlug={params.projectSlug}
      workspaceSlug={workspaceSlug}
    />
  )
}
```

- [ ] **Step 3: 创建 notes route**

创建 `web/src/routes/workspace/ProjectSettingsNotesRoute.tsx`：

```tsx
import { useParams } from "@tanstack/react-router"

import { useProjectQuery } from "@/features/workspace/project-workbench/hooks/use-project-data"
import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { ProjectNotesTab } from "@/pages/project-notes-tab"
import { isClosedProjectStatus } from "@/features/workspace/project-workbench/project/project-status-menu"
import { canProjectManage } from "@/features/workspace/project-workbench/permissions/permissions"

export function ProjectSettingsNotesRoute() {
  const params = useParams({ strict: false }) as { projectSlug: string }
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug
  const project = useProjectQuery(workspaceSlug ?? "", params.projectSlug)

  if (!workspaceSlug || !me.data) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }
  if (project.isError) {
    return (
      <section className="border bg-card p-6 text-sm text-destructive">
        {project.error instanceof ApiError ? project.error.message : "error"}
      </section>
    )
  }
  if (project.isPending) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }

  return (
    <ProjectNotesTab
      canManage={canProjectManage({
        role: me.data.effective_role,
        scopes: me.data.token.scopes,
      })}
      closed={isClosedProjectStatus(project.data.status)}
      projectSlug={params.projectSlug}
      workspaceSlug={workspaceSlug}
    />
  )
}
```

- [ ] **Step 4: 确认 useProjectQuery 签名**

Run: `cd web && grep -n "export function useProjectQuery\|export const useProjectQuery" src/features/workspace/project-workbench/hooks/use-project-data.ts`
Expected: 输出函数签名，确认参数为 `(workspaceSlug, projectSlug)`。若签名不同，调整 Task 9 的调用以匹配实际签名。

- [ ] **Step 5: 删除旧 route 文件**

```bash
rm web/src/routes/workspace/ProjectSettingsRoute.tsx
```

- [ ] **Step 6: 提交**

```bash
git add web/src/routes/workspace/ProjectSettingsLayoutRoute.tsx web/src/routes/workspace/ProjectSettingsConfigRoute.tsx web/src/routes/workspace/ProjectSettingsNotesRoute.tsx
git rm web/src/routes/workspace/ProjectSettingsRoute.tsx
git commit -m "feat: 新增项目设置 layout/config/notes 三个 route 文件"
```

---

### Task 10: router.tsx 接入父子子路由

**Files:**
- Modify: `web/src/routes/router.tsx`

- [ ] **Step 1: 替换 lazy import**

在 `web/src/routes/router.tsx` 中，把现有的 `ProjectSettingsRoute` lazy import（约 62-65 行）：

```tsx
const ProjectSettingsRoute = lazy(() =>
  import("@/routes/workspace/ProjectSettingsRoute").then((module) => ({
    default: module.ProjectSettingsRoute,
  }))
)
```

替换为三个 lazy import：

```tsx
const ProjectSettingsLayoutRoute = lazy(() =>
  import("@/routes/workspace/ProjectSettingsLayoutRoute").then((module) => ({
    default: module.ProjectSettingsLayoutRoute,
  }))
)
const ProjectSettingsConfigRoute = lazy(() =>
  import("@/routes/workspace/ProjectSettingsConfigRoute").then((module) => ({
    default: module.ProjectSettingsConfigRoute,
  }))
)
const ProjectSettingsNotesRoute = lazy(() =>
  import("@/routes/workspace/ProjectSettingsNotesRoute").then((module) => ({
    default: module.ProjectSettingsNotesRoute,
  }))
)
```

- [ ] **Step 2: 替换路由定义**

把现有的单条路由定义（约 253-257 行）：

```tsx
const projectSettingsRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/projects/$projectSlug/settings",
  component: lazyRoute(ProjectSettingsRoute),
})
```

替换为：

```tsx
const projectSettingsRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/projects/$projectSlug/settings",
  component: lazyRoute(ProjectSettingsLayoutRoute),
})

const projectSettingsIndexRoute = createRoute({
  getParentRoute: () => projectSettingsRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/projects/$projectSlug/settings/config" })
  },
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

> 注意：`redirect` 的 `to` 在 tanstack router 中通常需要带完整 params。若运行时报错缺少 params，改为：
> `throw redirect({ to: "/projects/$projectSlug/settings/config", params: { projectSlug: ... } })`
> 但 `beforeLoad` 里取 params 需通过 `ctx`。如果遇到类型问题，把 index route 改成 component 形式用 `<Navigate>`，见 Step 6 备选方案。

- [ ] **Step 3: 注册子路由**

在 `routeTree` 的 `projectSettingsRoute` 项下注册子路由。找到：

```tsx
    projectSettingsRoute,
```

替换为：

```tsx
    projectSettingsRoute.addChildren([
      projectSettingsIndexRoute,
      projectSettingsConfigRoute,
      projectSettingsNotesRoute,
    ]),
```

- [ ] **Step 4: 修复 layout 内的 navigate 目标路径**

Task 8 的 layout 用了 `to: "/workspaces/$workspaceSlug/projects/$projectSlug/settings/$tab"`。确认 workspaceRootRoute 的 path 前缀：检查 `workspaceRootRoute` 的 `path`。

Run: `cd web && grep -n "workspaceRootRoute = createRoute" -A 5 src/routes/router.tsx`
Expected: 看到 workspaceRootRoute 的 path（可能是 `/workspaces/$workspaceSlug` 或空）。若 workspaceRootRoute 已含 `/workspaces/$workspaceSlug` 前缀，则 layout navigate 的 `to` 应改为相对 `settings/$tab` 或去掉前缀匹配实际结构。

若 workspaceRootRoute path 是 `/workspaces/$workspaceSlug`，把 Task 8 中 `onValueChange` 的 navigate `to` 改为：

```tsx
void navigate({
  to: "/workspaces/$workspaceSlug/projects/$projectSlug/settings/$tab",
  params: { workspaceSlug, projectSlug, tab: suffix },
})
```

（保持原样，因为 router 配置里 workspaceRootRoute 会拼接前缀。）若运行报 route not found，根据 router 实际路由树调整 `to`。

- [ ] **Step 5: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误

- [ ] **Step 6: 备选方案（若 beforeLoad redirect 失败）**

若 `beforeLoad` + `redirect` 方案在 tanstack router 当前版本下报错，把 `projectSettingsIndexRoute` 改为 component 形式：

```tsx
import { Navigate } from "@tanstack/react-router"

const projectSettingsIndexRoute = createRoute({
  getParentRoute: () => projectSettingsRoute,
  path: "/",
  component: () => (
    <Navigate
      to="/workspaces/$workspaceSlug/projects/$projectSlug/settings/config"
      params={(u) => ({ workspaceSlug: u.workspaceSlug, projectSlug: u.projectSlug })}
    />
  ),
})
```

具体 params 传递方式以 tanstack router 文档和实际类型检查为准。

- [ ] **Step 7: 删除旧 settings 页文件**

```bash
rm web/src/pages/project-settings-page.tsx
```

- [ ] **Step 8: 提交**

```bash
git add web/src/routes/router.tsx
git rm web/src/pages/project-settings-page.tsx
git commit -m "feat: router 接入项目设置父子子路由 + tab 导航"
```

---

### Task 11: 清理项目列表行的设置入口路径

**Files:**
- Modify: `web/src/features/workspace/project-workbench/projects/project-row-actions.tsx`

- [ ] **Step 1: 确认当前链接**

Run: `cd web && grep -n "settings" src/features/workspace/project-workbench/projects/project-row-actions.tsx`
Expected: 看到 `to: "/projects/$projectSlug/settings"`（约第 44 行）。

- [ ] **Step 2: 更新链接到默认 tab**

把：

```tsx
to: "/projects/$projectSlug/settings",
```

改为：

```tsx
to: "/projects/$projectSlug/settings/config",
```

（若该行还带了 workspace 前缀，保持前缀一致，仅把 `settings` 改为 `settings/config`。）

- [ ] **Step 3: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误

- [ ] **Step 4: 提交**

```bash
git add web/src/features/workspace/project-workbench/projects/project-row-actions.tsx
git commit -m "fix: 项目列表行设置入口指向 config tab"
```

---

### Task 12: 全量验证

**Files:** 无（验证任务）

- [ ] **Step 1: 类型检查**

Run: `cd web && pnpm tsc --noEmit`
Expected: 无错误

- [ ] **Step 2: 单元测试**

Run: `cd web && pnpm vitest run`
Expected: 全部通过（原 378 + 新增测试）

- [ ] **Step 3: 检查未引用的旧代码残留**

Run: `cd web && grep -rn "project-settings-page\|ProjectSettingsPage\b" src/ --include="*.ts" --include="*.tsx"`
Expected: 无结果（旧文件和组件已完全移除）

- [ ] **Step 4: 检查 i18n 残留键**

Run: `cd web && grep -rn "projectSettings.basic\|projectSettings.status\b" src/`
Expected: 无结果（已清理的键不应再被引用）

- [ ] **Step 5: 构建**

Run: `cd web && pnpm build`
Expected: 构建成功，刷新 `internal/webconsole/dist`

- [ ] **Step 6: Go 构建冒烟**

Run: `cd /Users/mac/code/projects/dajee/task && CGO_ENABLED=0 go build ./cmd/xuanchu`
Expected: 成功（embed dist 不破坏 Go 构建）

- [ ] **Step 7: 最终提交（如有改动）**

```bash
git add -A
git commit -m "chore: 项目设置页重构全量验证"
```

---

## Self-Review 记录

**Spec 覆盖：**
- §4 路由设计（父子三条 + index 重定向 + tab URL 双向绑定）→ Task 8, 9, 10 ✅
- §5 配置项 schema 增强（并行拉 schema、按类型渲染、inline 编辑、新增、secret 遮掩、重复 key 拦截）→ Task 5, 6 ✅
- §6 备注 timeline 化（圆点轴、Markdown、actor、新增/删除、closed 只读）→ Task 4, 7 ✅
- §7 删除范围（基本信息/状态 section、旧单体组件、旧 route）→ Task 9 Step 5, Task 10 Step 7 ✅
- §10 验收（tsc、vitest、构建）→ Task 12 ✅
- §1.3 entries.map 崩溃修复回归 → Task 2 ✅

**已知风险点（执行时留意）：**
1. Task 10 的 `beforeLoad` redirect 在不同 tanstack router 版本下语法可能不同，给了 Step 6 备选方案。
2. Task 9 layout 的 `closed` 判断下沉到子页，layout 传 `closed={false}` 占位——这是刻意的，因为 layout 不拉 project 数据，避免重复请求。
3. `useProjectQuery` 签名需 Task 9 Step 4 确认。
