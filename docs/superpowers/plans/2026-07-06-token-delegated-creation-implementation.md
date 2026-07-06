# Owner/Admin 代为创建 PAT 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Web Console `/tokens` 页面，让 owner/admin 能为当前 workspace 其它成员创建 PAT，并在列表里筛选查看不同成员的 token。

**Architecture:** 后端零改动（`POST /api/v1/tokens` 的 `user` 字段、`resolveTokenTargetUser` 的 owner/admin 校验、`resolveTokenListUser` 的 owner/admin 查询均已就绪）。前端新增 cmdk 驱动的 `UserPicker` Combobox，嵌入创建表单（仅 PAT + admin/owner 可见）和列表筛选器；扩展 `TokenFormValues`/`TokenCreateInput` 增加 `user` 字段。

**Tech Stack:** React 19、TanStack Query、radix-ui Popover、cmdk（新增）、shadcn/ui 风格、Vitest、i18next。

---

## Source Spec

- `docs/superpowers/specs/2026-07-06-token-delegated-creation-design.md`

## File Map

新建：
- `web/src/components/ui/command.tsx` — shadcn 风格 Command（基于 cmdk）。
- `web/src/features/workspace/tokens/user-picker.tsx` — UserPicker Combobox（创建表单 + 列表筛选复用）。
- `web/src/features/workspace/tokens/user-picker.test.tsx` — UserPicker 单测。

修改：
- `web/package.json` — 新增 `cmdk` 依赖。
- `web/src/features/workspace/tokens/token-api.ts` — `TokenFormValues`/`TokenCreateInput` 加 `user?` 字段。
- `web/src/features/workspace/tokens/token-form.tsx` — 加归属用户 Field + `valuesToCreateInput` 扩展 + type 切换时重置 user。
- `web/src/features/workspace/tokens/token-create-dialog.tsx` — 透传 `canManageUsers`。
- `web/src/features/workspace/tokens/tokens-page.tsx` — 列表筛选器 + query key 参数化。
- `web/src/features/workspace/tokens/tokens-page.test.tsx` — admin 代创建 + 筛选测试。
- `web/src/locales/zh-CN.ts` — 中文文案。
- `web/src/locales/en-US.ts` — 英文文案。
- `README.md` — token 创建段落补充代为创建说明。

---

## Chunk 1: 基础设施 — cmdk 依赖与 Command 组件

### Task 1: 新增 cmdk 依赖并创建 Command 组件

**Files:**
- Modify: `web/package.json`
- Create: `web/src/components/ui/command.tsx`

- [ ] **Step 1: 安装 cmdk 依赖**

Run:

```bash
cd web && pnpm add cmdk
```

Expected: `package.json` 的 `dependencies` 出现 `cmdk`，`pnpm-lock.yaml` 更新。

- [ ] **Step 2: 创建 command.tsx**

Create `web/src/components/ui/command.tsx`，参照 shadcn 官方 Command 实现（基于 `cmdk` 的 `Command` primitive），适配项目既有样式约定（`rounded-none`、`cn` 工具）。导出：`Command`、`CommandDialog`（暂不使用，预留）、`CommandInput`、`CommandList`、`CommandEmpty`、`CommandGroup`、`CommandItem`、`CommandSeparator`。

```tsx
import * as React from "react"
import { Command as CommandPrimitive } from "cmdk"
import { SearchIcon } from "lucide-react"

import { cn } from "@/lib/utils"
import { Dialog, DialogContent } from "@/components/ui/dialog"

function Command({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive>) {
  return (
    <CommandPrimitive
      data-slot="command"
      className={cn(
        "flex h-full w-full flex-col overflow-hidden rounded-none bg-popover text-popover-foreground",
        className
      )}
      {...props}
    />
  )
}

function CommandDialog({
  title = "Command Palette",
  description,
  children,
  className,
  showCloseButton = true,
  ...props
}: React.ComponentProps<typeof Dialog> & {
  title?: string
  description?: string
  className?: string
  showCloseButton?: boolean
}) {
  return (
    <Dialog {...props}>
      <DialogContent
        className="overflow-hidden p-0 shadow-lg"
        showCloseButton={showCloseButton}
      >
        <Command
          className={cn(
            "[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group]:not([hidden])_~[cmdk-group]]:pt-0 [&_[cmdk-group]]:px-2 [&_[cmdk-input-wrapper]_svg]:h-4 [&_[cmdk-input-wrapper]_svg]:w-4 [&_[cmdk-input]]:h-12 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-3 [&_[cmdk-item]_svg]:h-4 [&_[cmdk-item]_svg]:w-4",
            className
          )}
        >
          {children}
        </Command>
      </DialogContent>
    </Dialog>
  )
}

function CommandInput({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Input>) {
  return (
    <div
      data-slot="command-input-wrapper"
      className="flex h-9 items-center gap-2 border-b px-3"
      cmdk-input-wrapper=""
    >
      <SearchIcon className="size-4 shrink-0 opacity-50" />
      <CommandPrimitive.Input
        data-slot="command-input"
        className={cn(
          "flex h-10 w-full rounded-none bg-transparent py-3 text-xs outline-hidden placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50",
          className
        )}
        {...props}
      />
    </div>
  )
}

function CommandList({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.List>) {
  return (
    <CommandPrimitive.List
      data-slot="command-list"
      className={cn(
        "max-h-[240px] scroll-py-1 overflow-x-hidden overflow-y-auto",
        className
      )}
      {...props}
    />
  )
}

function CommandEmpty({
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Empty>) {
  return (
    <CommandPrimitive.Empty
      data-slot="command-empty"
      className="py-6 text-center text-xs"
      {...props}
    />
  )
}

function CommandGroup({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Group>) {
  return (
    <CommandPrimitive.Group
      data-slot="command-group"
      className={cn(
        "overflow-hidden p-1 text-popover-foreground [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground",
        className
      )}
      {...props}
    />
  )
}

function CommandSeparator({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Separator>) {
  return (
    <CommandPrimitive.Separator
      data-slot="command-separator"
      className={cn("-mx-1 h-px bg-border", className)}
      {...props}
    />
  )
}

function CommandItem({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Item>) {
  return (
    <CommandPrimitive.Item
      data-slot="command-item"
      className={cn(
        "relative flex cursor-default select-none items-center gap-2 rounded-none px-2 py-1.5 text-xs outline-hidden data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 data-[selected=true]:bg-muted data-[selected=true]:text-foreground",
        className
      )}
      {...props}
    />
  )
}

export {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
}
```

- [ ] **Step 3: 验证 typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS（command.tsx 编译通过）。

---

## Chunk 2: 类型扩展

### Task 2: 扩展 token-api.ts 类型

**Files:**
- Modify: `web/src/features/workspace/tokens/token-api.ts`

- [ ] **Step 1: TokenFormValues 加 user 字段**

在 `TokenFormValues`（`token-api.ts:47-55`）的 `type` 字段后新增：

```ts
export type TokenFormValues = {
  name: string
  type: string // 'pat' | 'agent'
  user?: string // ★ 新增: 归属用户 user_id, 空/undefined = 我自己（仅 admin/owner 代为创建时使用）
  workspaces: string[] // workspace slug/id refs
  scopes: string[]
  projects: string[] // project slug/id refs
  expiresPreset: ExpiresPreset
  expiresAt: string // ISO datetime，仅 preset=custom 时使用
}
```

- [ ] **Step 2: TokenCreateInput 加 user 字段**

在 `TokenCreateInput`（`token-api.ts:60-67`）的 `type` 字段后新增：

```ts
export type TokenCreateInput = {
  name: string
  type?: string
  user?: string // ★ 新增: 归属用户 user_id，后端 resolveTokenTargetUser 校验 admin/owner
  scopes?: string[]
  workspaces?: string[]
  projects?: string[]
  expires_in_seconds?: number | null
}
```

- [ ] **Step 3: 验证 typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS（新字段可选，不破坏现有代码）。

---

## Chunk 3: UserPicker 组件

### Task 3: 创建 UserPicker 组件

**Files:**
- Create: `web/src/features/workspace/tokens/user-picker.tsx`
- Create: `web/src/features/workspace/tokens/user-picker.test.tsx`

- [ ] **Step 1: 写失败测试**

Create `web/src/features/workspace/tokens/user-picker.test.tsx`：

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { UserPicker } from "./user-picker"

function renderPicker(props: React.ComponentProps<typeof UserPicker>) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <UserPicker {...props} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function okResponse(data: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify({ data }), { status: 200 })
  )
}

const meResponse = {
  actor_type: "user",
  actor: { id: "u-admin", name: "admin", display_name: "管理员" },
  token: { type: "pat", scopes: ["token:read", "token:write"] },
  effective_workspace: { slug: "local" },
  effective_role: "owner",
}

const members = [
  {
    user_id: "u-admin",
    name: "admin",
    display_name: "管理员",
    email: "admin@example.com",
    role: "owner",
    joined_at: 1,
    modified_at: 1,
  },
  {
    user_id: "u-zhang",
    name: "zhangsan",
    display_name: "张三",
    email: "zhangsan@example.com",
    role: "member",
    joined_at: 1,
    modified_at: 1,
  },
]

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = String(input)
    if (url.includes("/api/v1/credentials/current")) return okResponse(meResponse)
    if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
      return okResponse(members)
    }
    return okResponse([])
  })
}

describe("UserPicker", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("shows 'myself' as default when value is empty", async () => {
    mockFetch()
    renderPicker({ value: "", onChange: () => {} })

    await waitFor(() => {
      expect(screen.getByText(/我自己/)).toBeTruthy()
    })
  })

  it("opens dropdown and lists members", async () => {
    mockFetch()
    const user = userEvent.setup()
    renderPicker({ value: "", onChange: () => {} })

    await waitFor(() => {
      expect(screen.getByText(/我自己/)).toBeTruthy()
    })

    await user.click(screen.getByRole("combobox"))
    await waitFor(() => {
      expect(screen.getByText("张三")).toBeTruthy()
    })
  })

  it("filters members by search input", async () => {
    mockFetch()
    const user = userEvent.setup()
    renderPicker({ value: "", onChange: () => {} })

    await waitFor(() => {
      expect(screen.getByText(/我自己/)).toBeTruthy()
    })
    await user.click(screen.getByRole("combobox"))

    await user.type(
      screen.getByPlaceholderText(/搜索成员/),
      "zhangsan"
    )
    await waitFor(() => {
      expect(screen.getByText("张三")).toBeTruthy()
      expect(screen.queryByText("管理员")).toBeNull()
    })
  })

  it("calls onChange with member user_id when selected", async () => {
    const fetchMock = mockFetch()
    const user = userEvent.setup()
    const onChange = vi.fn()
    renderPicker({ value: "", onChange })

    await waitFor(() => {
      expect(screen.getByText(/我自己/)).toBeTruthy()
    })
    await user.click(screen.getByRole("combobox"))

    await waitFor(() => {
      expect(screen.getByText("张三")).toBeTruthy()
    })
    await user.click(screen.getByText("张三"))

    expect(onChange).toHaveBeenCalledWith("u-zhang")
    fetchMock.mockRestore()
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
pnpm --dir web test -- user-picker
```

Expected: FAIL（`UserPicker` 未定义）。

- [ ] **Step 3: 实现 UserPicker**

Create `web/src/features/workspace/tokens/user-picker.tsx`：

```tsx
import { useQuery } from "@tanstack/react-query"
import { CheckIcon, ChevronsUpDownIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useMe } from "@/features/workspace/session/useMe"
import {
  listWorkspaceMembers,
  type WorkspaceMemberRow,
} from "@/features/workspace/members/members-api"
import { cn } from "@/lib/utils"

export type UserPickerProps = {
  /** 选中的 user_id；"" 表示「我自己」语义。 */
  value: string
  onChange: (userId: string) => void
  disabled?: boolean
  className?: string
}

export function UserPicker({
  value,
  onChange,
  disabled = false,
  className,
}: UserPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const me = useMe()
  const slug = me.data?.effective_workspace.slug ?? ""
  const membersQuery = useQuery({
    enabled: slug !== "",
    queryKey: ["workspace", "members", slug],
    queryFn: () => listWorkspaceMembers(slug),
  })
  const members: WorkspaceMemberRow[] = membersQuery.data ?? []
  const selfId = me.data?.actor.id ?? ""

  // value === "" → 我自己；否则匹配 members 中的具体成员
  const selectedName =
    value === ""
      ? t("token.user.self")
      : members.find((m) => m.user_id === value)?.display_name ??
        members.find((m) => m.user_id === value)?.name ??
        value

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button
          aria-expanded={open}
          aria-label={t("token.field.user")}
          className={cn("w-full justify-between font-normal", className)}
          disabled={disabled}
          role="combobox"
          type="button"
          variant="outline"
        >
          <span className="truncate">{selectedName}</span>
          <ChevronsUpDownIcon className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-(--radix-popover-trigger-width) p-0">
        <Command>
          <CommandInput
            placeholder={t("token.user.searchPlaceholder")}
          />
          <CommandList>
            <CommandEmpty>{t("common.empty")}</CommandEmpty>
            <CommandGroup>
              {/* 「我自己」始终置顶，value="" */}
              <CommandItem
                onSelect={() => {
                  onChange("")
                  setOpen(false)
                }}
                value={`self-${selfId}`}
              >
                <CheckIcon
                  className={cn(
                    "mr-2 size-4",
                    value === "" ? "opacity-100" : "opacity-0"
                  )}
                />
                <span>{t("token.user.self")}</span>
                {me.data?.effective_role ? (
                  <span className="ml-auto text-xs text-muted-foreground">
                    {me.data.effective_role}
                  </span>
                ) : null}
              </CommandItem>
              {members
                .filter((m) => m.user_id !== selfId)
                .map((m) => (
                  <CommandItem
                    key={m.user_id}
                    onSelect={() => {
                      onChange(m.user_id)
                      setOpen(false)
                    }}
                    value={`${m.display_name || m.name} ${m.email ?? ""} ${m.name}`}
                  >
                    <CheckIcon
                      className={cn(
                        "mr-2 size-4",
                        value === m.user_id ? "opacity-100" : "opacity-0"
                      )}
                    />
                    <span>{m.display_name || m.name}</span>
                    {m.email ? (
                      <span className="ml-2 truncate text-xs text-muted-foreground">
                        {m.email}
                      </span>
                    ) : null}
                    <span className="ml-auto text-xs text-muted-foreground">
                      {m.role}
                    </span>
                  </CommandItem>
                ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
pnpm --dir web test -- user-picker
```

Expected: PASS（4 个用例）。

---

## Chunk 4: TokenForm 集成

### Task 4: TokenForm 增加归属用户 Field

**Files:**
- Modify: `web/src/features/workspace/tokens/token-form.tsx`

- [ ] **Step 1: 增加 canManageUsers prop**

在 `TokenFormProps`（`token-form.tsx:32-38`）新增 `canManageUsers`：

```ts
type TokenFormProps = {
  mode: "create" | "edit"
  initial?: TokenRow
  onSubmit: (values: TokenFormValues) => void
  submitting?: boolean
  canImpersonate?: boolean
  canManageUsers?: boolean // ★ 新增: 仅 owner/admin 为 true 时显示「归属用户」字段
}
```

函数签名解构同步加上 `canManageUsers = false`。

- [ ] **Step 2: defaultValues 含 user 字段**

在 `defaultValues` 的 create 分支（`token-form.tsx:56-64`）加 `user: ""`：

```ts
return {
  name: "",
  type: "pat",
  user: "",
  workspaces: [],
  scopes: [],
  projects: [],
  expiresPreset: "never",
  expiresAt: "",
}
```

edit 分支无需 user（编辑不改归属，字段不显示）。

- [ ] **Step 3: type 切换时重置 user**

在 `update` 调用处，type 字段的 `onValueChange` 改为：

```tsx
<Select
  disabled={mode === "edit"}
  onValueChange={(v) =>
    setValues((prev) => ({
      ...prev,
      type: v,
      // 切换到 agent 时清空归属用户（仅 PAT 支持代为创建）
      user: v === "pat" ? prev.user : "",
    }))
  }
  value={values.type}
>
```

- [ ] **Step 4: 在 type Field 后插入归属用户 Field**

在 type 的 `</Field>` 之后、workspaces 的 `<Field>` 之前插入（仅 create 模式 + PAT + admin/owner 可见）：

```tsx
{mode === "create" && canManageUsers && values.type === "pat" ? (
  <Field
    hint={t("token.field.userHint")}
    label={t("token.field.user")}
  >
    <UserPicker
      onChange={(userId) => update("user", userId)}
      value={values.user ?? ""}
    />
  </Field>
) : null}
```

并在文件顶部 import：

```ts
import { UserPicker } from "./user-picker"
```

- [ ] **Step 5: valuesToCreateInput 透传 user**

修改 `valuesToCreateInput`（`token-form.tsx:306-316`），仅当 `values.user` 非空时带上：

```ts
export function valuesToCreateInput(values: TokenFormValues) {
  const expires = presetToExpiresSeconds(values.expiresPreset, values.expiresAt)
  return {
    name: values.name.trim(),
    type: values.type,
    ...(values.user ? { user: values.user } : {}),
    scopes: values.scopes,
    workspaces: values.workspaces,
    projects: values.projects,
    expires_in_seconds: expires,
  }
}
```

- [ ] **Step 6: 验证 typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS。

---

## Chunk 5: Dialog 透传

### Task 5: TokenCreateDialog 透传 canManageUsers

**Files:**
- Modify: `web/src/features/workspace/tokens/token-create-dialog.tsx`

- [ ] **Step 1: Props 新增 canManageUsers**

```ts
type TokenCreateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  canImpersonate?: boolean
  canManageUsers?: boolean // ★ 新增
}
```

函数签名解构加上 `canManageUsers = false`。

- [ ] **Step 2: 透传到 TokenForm**

在 `<TokenForm ...>` 上新增：

```tsx
<TokenForm
  canImpersonate={canImpersonate}
  canManageUsers={canManageUsers}
  mode="create"
  ...
/>
```

- [ ] **Step 3: 验证 typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS。

---

## Chunk 6: TokensPage 列表筛选器

### Task 6: TokensPage 增加列表筛选器与 query key 参数化

**Files:**
- Modify: `web/src/features/workspace/tokens/tokens-page.tsx`

- [ ] **Step 1: 新增 viewUser state 与 canManageUsers**

在 `tokens-page.tsx` 的 state 区（约第 52-83 行）新增：

```tsx
const [viewUser, setViewUser] = useState<string>("") // "" = 我自己
```

在 `canImpersonate` 下方新增（语义独立，不复用 prop）：

```tsx
const canManageUsers = role === "admin" || role === "owner"
```

- [ ] **Step 2: 参数化 tokenQuery 的 queryKey 与 queryFn**

把原 `tokenQuery`（`tokens-page.tsx:56-60`）改为：

```tsx
const tokenQuery = useQuery({
  queryKey: [
    "resource",
    "/api/v1/tokens",
    ...(viewUser ? [{ user: viewUser }] : []),
  ],
  queryFn: () =>
    workspaceApiGet<TokenRow[]>(
      viewUser
        ? `/api/v1/tokens?user=${encodeURIComponent(viewUser)}`
        : "/api/v1/tokens"
    ),
  enabled: me.isSuccess && !isTenantActor && activeTab === "api",
})
```

- [ ] **Step 3: 在 Tabs 下方、Table 上方插入筛选器（仅 admin/owner + api tab）**

在 `<Tabs>...</Tabs>` 之后、`{loading ? ...}` 之前插入：

```tsx
{activeTab === "api" && canManageUsers && !isTenantActor ? (
  <div className="flex items-center gap-2">
    <span className="text-xs text-muted-foreground">
      {t("token.user.viewFilter")}
    </span>
    <UserPicker
      className="max-w-xs"
      onChange={setViewUser}
      value={viewUser}
    />
  </div>
) : null}
```

并在文件顶部 import：

```ts
import { UserPicker } from "./user-picker"
```

- [ ] **Step 4: TokenCreateDialog 透传 canManageUsers**

把 `<TokenCreateDialog ...>`（`tokens-page.tsx:258-262`）改为：

```tsx
<TokenCreateDialog
  canImpersonate={canImpersonate}
  canManageUsers={canManageUsers}
  onOpenChange={setCreateOpen}
  open={createOpen && activeTab === "api"}
/>
```

- [ ] **Step 5: 验证 typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS。

---

## Chunk 7: i18n 文案

### Task 7: 补充中文与英文文案

**Files:**
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: zh-CN.ts 新增文案**

在 zh-CN.ts 的 `token.field` 块（约 `:834-843`）的 `lastUsed` 后新增 `user`：

```ts
field: {
  name: "名称",
  type: "类型",
  scopes: "权限范围 (Scope)",
  workspaces: "工作空间",
  projects: "项目",
  expires: "过期时间",
  prefix: "前缀",
  lastUsed: "最后使用",
  user: "归属用户", // ★ 新增
  userHint: "为当前 workspace 成员创建，token 将归属所选用户", // ★ 新增
},
```

在 `token.errors` 块（约 `:896-913`）的 `unknown` 前新增：

```ts
permission_denied: "你没有为其它用户创建 token 的权限",
user_not_found: "找不到所选用户，请重新选择",
```

在 `token` 块顶层（建议 `typeLocked` 之后）新增 `user` 子对象：

```ts
user: {
  self: "我自己",
  searchPlaceholder: "搜索成员姓名或邮箱",
  viewFilter: "查看用户",
},
```

- [ ] **Step 2: en-US.ts 新增对应英文文案**

在 en-US.ts 的 `token.field` 块（约 `:853-862`）同步新增：

```ts
field: {
  name: "Name",
  type: "Type",
  scopes: "Scopes",
  workspaces: "Workspaces",
  projects: "Projects",
  expires: "Expiration",
  prefix: "Prefix",
  lastUsed: "Last used",
  user: "Owner", // ★ 新增
  userHint: "Create for a workspace member; the token belongs to the selected user", // ★ 新增
},
```

在 `token.errors` 块的 `unknown` 前新增：

```ts
permission_denied: "You do not have permission to create tokens for other users",
user_not_found: "Selected user not found, please choose again",
```

在 `token` 块顶层新增：

```ts
user: {
  self: "Myself",
  searchPlaceholder: "Search by name or email",
  viewFilter: "View user",
},
```

- [ ] **Step 3: 验证 typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS。

---

## Chunk 8: 集成测试

### Task 8: 补充 tokens-page 集成测试

**Files:**
- Modify: `web/src/features/workspace/tokens/tokens-page.test.tsx`

- [ ] **Step 1: 新增成员 mock 数据辅助**

在 `tokens-page.test.tsx` 的 mock 区新增成员响应。在 `credentialCurrentResponse` 中补 `actor.id`：

```ts
function credentialCurrentResponse(
  overrides: Record<string, unknown> = {}
) {
  return {
    actor_type: "user",
    actor: { id: "u-admin", name: "local" },
    token: { type: "pat", scopes: ["token:read", "token:write"] },
    effective_workspace: { slug: "local" },
    effective_role: "owner",
    ...overrides,
  }
}

function membersResponse() {
  return [
    {
      user_id: "u-admin",
      name: "admin",
      display_name: "管理员",
      email: "admin@example.com",
      role: "owner",
      joined_at: 1,
      modified_at: 1,
    },
    {
      user_id: "u-zhang",
      name: "zhangsan",
      display_name: "张三",
      email: "zhangsan@example.com",
      role: "member",
      joined_at: 1,
      modified_at: 1,
    },
  ]
}
```

- [ ] **Step 2: 测试 admin 创建弹窗显示归属用户字段**

```ts
it("shows owner user field in create dialog for admin", async () => {
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/api/v1/credentials/current")) {
        return okResponse(credentialCurrentResponse())
      }
      if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
        return okResponse(membersResponse())
      }
      if (url.includes("/api/v1/tokens")) {
        return okResponse([makeToken()])
      }
      return okResponse([])
    })

  renderPage()

  await waitFor(() => {
    expect(screen.getByText("ci-deploy")).toBeTruthy()
  })
  await userEvent.click(screen.getByText("创建 Token"))

  await waitFor(() => {
    expect(screen.getByText("归属用户")).toBeTruthy()
  })
  // 默认「我自己」
  expect(screen.getByText(/我自己/)).toBeTruthy()

  fetchMock.mockRestore()
})
```

- [ ] **Step 3: 测试 member 不显示归属用户字段**

```ts
it("hides owner user field for member role", async () => {
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/api/v1/credentials/current")) {
        return okResponse(
          credentialCurrentResponse({
            effective_role: "member",
            actor: { id: "u-member", name: "member" },
          })
        )
      }
      if (url.includes("/api/v1/tokens")) {
        return okResponse([makeToken()])
      }
      return okResponse([])
    })

  renderPage()

  await waitFor(() => {
    expect(screen.getByText("ci-deploy")).toBeTruthy()
  })
  await userEvent.click(screen.getByText("创建 Token"))

  await waitFor(() => {
    expect(screen.queryByText("归属用户")).toBeNull()
  })

  fetchMock.mockRestore()
})
```

- [ ] **Step 4: 测试列表筛选器存在且 admin 可见**

```ts
it("shows view user filter for admin", async () => {
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/api/v1/credentials/current")) {
        return okResponse(credentialCurrentResponse())
      }
      if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
        return okResponse(membersResponse())
      }
      if (url.includes("/api/v1/tokens")) {
        return okResponse([makeToken()])
      }
      return okResponse([])
    })

  renderPage()

  await waitFor(() => {
    expect(screen.getByText("ci-deploy")).toBeTruthy()
  })
  expect(screen.getByText("查看用户")).toBeTruthy()

  fetchMock.mockRestore()
})
```

- [ ] **Step 5: 测试切换筛选器后请求带 ?user=**

```ts
it("requests tokens filtered by user when filter changes", async () => {
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/api/v1/credentials/current")) {
        return okResponse(credentialCurrentResponse())
      }
      if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
        return okResponse(membersResponse())
      }
      if (url.includes("/api/v1/tokens")) {
        return okResponse([makeToken({ name: "zhang-token", id: "tok-zhang" })])
      }
      return okResponse([])
    })

  renderPage()

  await waitFor(() => {
    expect(screen.getByText("查看用户")).toBeTruthy()
  })
  // 打开筛选器
  await userEvent.click(screen.getAllByRole("combobox")[0])
  await waitFor(() => {
    expect(screen.getByText("张三")).toBeTruthy()
  })
  await userEvent.click(screen.getByText("张三"))

  await waitFor(() => {
    expect(
      fetchMock.mock.calls.some((call) =>
        String(call[0]).includes("/api/v1/tokens?user=u-zhang")
      )
    ).toBe(true)
  })

  fetchMock.mockRestore()
})
```

- [ ] **Step 6: 运行测试确认通过**

Run:

```bash
pnpm --dir web test -- tokens-page user-picker
```

Expected: PASS（全部既有 + 新增用例）。

---

## Chunk 9: 文档更新

### Task 9: 更新 README 与 web-console 文档

**Files:**
- Modify: `README.md`
- Modify: `docs/manual/web-console.md`（若已有 /tokens 说明）

- [ ] **Step 1: 更新 README token 创建段落**

在 README 中找到 token 创建相关说明（"明文只展示一次"附近），补充：

```text
owner/admin 可在 Web Console `/tokens` 页面为当前 workspace 的其它成员创建 PAT：创建表单的「归属用户」字段仅对 owner/admin 可见，默认「我自己」。代为创建的 token 仍受调用凭证的 scope/workspace 上限约束。admin 还可在列表顶部用「查看用户」筛选器切换查看不同成员的 token。
```

- [ ] **Step 2: 更新 web-console 手册（若存在相关段落）**

Run:

```bash
rg -n "tokens|Token|MCP" docs/manual/web-console.md | head -20
```

若有 `/tokens` 段落，追加代为创建与筛选说明；若不存在，跳过。

---

## Chunk 10: 全量验证

### Task 10: 跑完整验证命令

- [ ] **Step 1: typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS。

- [ ] **Step 2: 全量测试**

Run:

```bash
pnpm --dir web test
```

Expected: PASS（含既有用例不回归）。

- [ ] **Step 3: lint**

Run:

```bash
pnpm --dir web lint
```

Expected: PASS。

- [ ] **Step 4: build**

Run:

```bash
pnpm --dir web build
```

Expected: PASS。

- [ ] **Step 5: 后端回归（确认零改动无影响）**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS（本功能不改后端，应无回归）。

---

## 验收标准

- owner/admin 打开创建弹窗，type=pat 时可见「归属用户」Combobox，默认「我自己」。
- admin 选择张三并提交，网络请求体含 `user: "u-zhang"`。
- member/viewer 看不到「归属用户」字段，请求体不含 `user`。
- type 切换为 agent 时，归属用户字段隐藏且 user 值清空。
- admin 在列表顶部切换「查看用户」为张三，列表请求带 `?user=u-zhang`。
- 普通成员看不到列表筛选器。
- `pnpm --dir web typecheck / test / lint / build` 通过。
- 后端无回归。
