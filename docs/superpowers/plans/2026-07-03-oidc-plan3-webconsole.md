# OIDC 接入 Plan 3：Web Console 前端（SSO 配置页 + 登录入口）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 在 Web Console 新增「单点登录」配置页（仅 workspace owner + admin acting 可见），管理员可填写 yaoguang OIDC 配置、触发通讯录同步；并在登录页加 OIDC 登录入口。

**对应 spec:** `docs/superpowers/specs/2026-07-03-workspace-oidc-design.md`（第 4.4 节）

**依赖：** Plan 1、Plan 2 的后端 API 已就绪（`GET/PUT /api/v1/workspaces/{slug}/sso/config`、`POST .../sso/sync`、`/sso/oidc/start`）。

**技术栈：** React 19、TanStack Router、TanStack Query、shadcn/ui、i18next。

---

## 关键现有代码参考

- `navItems` 静态数组：`web/src/components/AppShell.tsx:47-61`（无角色过滤先例）
- `PageKey` 联合类型：`web/src/components/AppShell.tsx:36-46`
- nav 渲染 map：`web/src/components/AppShell.tsx:92-112`（无条件渲染）
- 角色判断：`web/src/features/workspace/tokens/tokens-page.tsx:71-72`（`me.data?.effective_role`）
- `useMe` hook：`web/src/features/workspace/session/useMe.ts`（返回 `actor_type` / `effective_role`）
- acting 判断：`web/src/components/AppShell.tsx:79`（`getAdminActingContext() !== null`）
- 表单模式：`web/src/features/workspace/tokens/token-form.tsx`（受控 `useState` + `Field` 包装）
- 数据层模式：`web/src/features/workspace/tokens/use-token-mutations.ts`（`useMutation` + `workspaceApiGet/Post/Patch` + invalidate）
- 路由模式：`web/src/routes/router.tsx:201-205`（`tokensRoute` 写法）+ `web/src/routes/workspace/TokensRoute.tsx`
- API client：`web/src/features/workspace/session/workspace-api.ts`（`workspaceApiGet/Post/Patch`，`assertWorkspacePath` 要求 `/api/v1/` 前缀）
- i18n：`web/src/locales/zh-CN.ts`（顶层分组 + `nav.*`）

---

### Task 1: i18n 文案

**Files:**
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 在 zh-CN.ts 加 nav.sso 和 sso.* 文案组**

在 `nav` 组加一项 `"sso": "单点登录"`。

在顶层加新的 `sso` 文案组：

```ts
  sso: {
    title: "单点登录（OIDC）",
    description: "通过 yaoguang IdP 让 workspace 成员使用浏览器 SSO 登录。",
    sectionIdp: "IdP 连接",
    sectionDirectory: "通讯录同步",
    sectionAdvanced: "高级（可选）",
    sectionMembers: "成员同步",
    field: {
      issuerBaseUrl: "Issuer 根地址",
      orgId: "组织 ID",
      clientId: "Client ID",
      clientSecret: "Client Secret",
      directoryAccessToken: "通讯录访问令牌",
      syncInterval: "同步周期",
      externalBaseUrl: "外部可达地址",
      sessionTtl: "Session 有效期",
    },
    hint: {
      issuerBaseUrl: "yaoguang 服务根 URL；OIDC discovery 与通讯录接口都基于此地址。",
      orgId: "yaoguang 的 organization id，用于通讯录接口路径。",
      clientId: "xuanchu 在 yaoguang 注册的 internal app client_id。",
      clientSecret: "OIDC client_secret，加密存储。留空保存表示不修改。",
      directoryAccessToken: "调用 yaoguang 通讯录接口的 tenant_access_token，需覆盖 org.members.read scope 且 directory_access=org_read。",
      syncInterval: "定时拉取通讯录的间隔；选「禁用」则仅手动触发。",
      externalBaseUrl: "用于拼接 OIDC redirect_uri，留空则用请求 Host。",
      sessionTtl: "Browser session 有效期。",
    },
    secretSet: "已设置",
    secretUnset: "未设置",
    save: "保存配置",
    cancel: "取消",
    syncNow: "立即同步成员",
    lastSync: "上次同步",
    syncRunning: "同步中…",
    notEnabled: "尚未配置 SSO，填写以下信息后保存即可启用。",
    noPermission: "您没有权限查看此页面。",
    errors: {
      save: "保存失败",
      sync: "同步触发失败",
      syncInProgress: "已有同步任务进行中",
    },
  },
```

- [ ] **Step 2: 在 en-US.ts 加对应英文文案**（key 结构相同，值翻译）

- [ ] **Step 3: 验证 i18n 编译**

```bash
cd web && pnpm tsc --noEmit
```
Expected: 无类型错误。

- [ ] **Step 4: Commit**

```bash
git add web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): SSO 配置页 i18n 文案"
```

---

### Task 2: nav 项 + owner/acting 显隐过滤

**Files:**
- Modify: `web/src/components/AppShell.tsx`

- [ ] **Step 1: 在 PageKey 加 "sso"**

```ts
export type PageKey =
  | "overview"
  | "projects"
  | "workspaces"
  | "members"
  | "tokens"
  | "sso"
  | "hooks"
  | "notifications"
  | "audit"
  | "settings"
```

- [ ] **Step 2: 在 navItems 加 sso 项（用 KeyRound 图标，放在 tokens 之后）**

```ts
const navItems: Array<{
  key: PageKey
  icon: React.ComponentType<{ className?: string }>
  to: string
  ssoOnly?: boolean
}> = [
  { key: "overview", icon: Activity, to: "/" },
  { key: "projects", icon: Boxes, to: "/projects" },
  { key: "workspaces", icon: Boxes, to: "/workspaces" },
  { key: "members", icon: Users, to: "/members" },
  { key: "tokens", icon: KeyRound, to: "/tokens" },
  { key: "sso", icon: ShieldCheck, to: "/sso", ssoOnly: true },
  { key: "hooks", icon: Webhook, to: "/hooks" },
  { key: "notifications", icon: Bell, to: "/notifications" },
  { key: "audit", icon: FileClock, to: "/audit" },
  { key: "settings", icon: Settings, to: "/settings" },
]
```

在 import 里加 `ShieldCheck`：

```ts
import {
  Activity, ArrowLeft, Bell, Boxes, FileClock, KeyRound, LogOut,
  RefreshCw, Settings, ShieldAlert, ShieldCheck, Users, Webhook,
} from "lucide-react"
```

- [ ] **Step 3: 在 AppShell 组件内加显隐过滤**

AppShell 组件需要 `useMe` 判断 owner/acting。当前 AppShell props 没有 me 数据，需在组件内调用 `useMe()`（参照 tokens-page）。在 `const { t } = useTranslation()` 后加：

```ts
import { useMe } from "@/features/workspace/session/useMe"
// ...
  const me = useMe()
  const role = me.data?.effective_role ?? ""
  const isOwner = role === "owner"
  const showSso = isOwner || acting
```

把 nav 渲染 map（L92）加过滤：

```tsx
          {navItems
            .filter((item) => !item.ssoOnly || showSso)
            .map((item) => {
```

> 注意：`acting` 变量在 L81 已定义（`getAdminActingContext() !== null`）。`useMe` 需确认 AppShell 已在 QueryClientProvider 内（是的，router 层已保证）。

- [ ] **Step 4: 验证类型检查**

```bash
cd web && pnpm tsc --noEmit
```
Expected: 无错误。

- [ ] **Step 5: Commit**

```bash
git add web/src/components/AppShell.tsx
git commit -m "feat(web): nav 加 SSO 项（仅 owner/acting 可见）"
```

---

### Task 3: 路由注册

**Files:**
- Create: `web/src/routes/workspace/SsoRoute.tsx`
- Modify: `web/src/routes/router.tsx`

- [ ] **Step 1: 创建 SsoRoute 包装（仿 TokensRoute）**

`web/src/routes/workspace/TokensRoute.tsx` 的结构（先读它），照搬到 `web/src/routes/workspace/SsoRoute.tsx`：

```tsx
import { SsoConfigPage } from "@/features/workspace/sso/sso-config-page"

export function SsoRoute() {
  return <SsoConfigPage />
}
```

> 若 TokensRoute 还有 lazy/suspense 包装，照搬同样的模式。

- [ ] **Step 2: 在 router.tsx 注册路由（仿 tokensRoute，L196-205）**

在 `tokensRoute` 定义后加：

```ts
const SsoRoute = lazy(() =>
  import("@/routes/workspace/SsoRoute").then((module) => ({
    default: module.SsoRoute,
  }))
)
const ssoRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/sso",
  component: lazyRoute(SsoRoute),
})
```

在 `workspaceRootRoute.addChildren`（L208-221）的 `tokensRoute` 后加 `ssoRoute`：

```ts
    membersRoute,
    tokensRoute,
    ssoRoute,
    createResourceRoute("hooks", "/hooks"),
```

- [ ] **Step 3: 验证类型检查**

```bash
cd web && pnpm tsc --noEmit
```
Expected: 无错误（SsoConfigPage 尚未实现会报错，下一步实现）。

- [ ] **Step 4: Commit（先不提交，等 Task 4 页面实现后一起）**

---

### Task 4: SSO 数据层（query + mutation）

**Files:**
- Create: `web/src/features/workspace/sso/use-sso.ts`

- [ ] **Step 1: 实现数据层（仿 use-token-mutations.ts）**

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  workspaceApiGet,
  workspaceApiPost,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"
import { ApiError } from "@/lib/api"

export interface SsoConfigResponse {
  enabled: boolean
  config?: SsoConfig
}

export interface SsoConfig {
  provider: string
  issuer_base_url: string
  org_id: string
  client_id: string
  client_secret_masked: string
  directory_access_token_masked: string
  scopes: string
  redirect_path: string
  external_base_url: string
  session_ttl: string
  sync_interval: string
  insecure_cookie: boolean
}

export interface SsoConfigInput {
  issuer_base_url: string
  org_id: string
  client_id: string
  client_secret: string
  directory_access_token: string
  sync_interval: string
  external_base_url: string
  session_ttl: string
}

export interface SyncJobResponse {
  ok: boolean
  data: { job_id: string; status: string }
}

const SSO_CONFIG_KEY = ["workspace", "sso", "config"] as const

export function useSsoConfigQuery(workspaceSlug: string) {
  return useQuery({
    queryKey: SSO_CONFIG_KEY,
    queryFn: () =>
      workspaceApiGet<SsoConfigResponse>(`/api/v1/workspaces/${workspaceSlug}/sso/config`),
  })
}

export function useSaveSsoConfigMutation(workspaceSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SsoConfigInput) =>
      workspaceApiPut<SsoConfigResponse>(
        `/api/v1/workspaces/${workspaceSlug}/sso/config`,
        input,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: SSO_CONFIG_KEY })
    },
  })
}

export function useTriggerSyncMutation(workspaceSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      workspaceApiPost<SyncJobResponse>(
        `/api/v1/workspaces/${workspaceSlug}/sso/sync`,
        {},
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workspace", "sso", "jobs"] })
    },
  })
}
```

> `workspaceApiPut` 需确认在 `workspace-api.ts` 已导出；若没有则补一个（仿 Post/Patch）。

- [ ] **Step 2: 确认 workspaceApiPut 存在**

读 `web/src/features/workspace/session/workspace-api.ts`，若无 `workspaceApiPut`，仿 `workspaceApiPost` 加：

```ts
export async function workspaceApiPut<T>(path: string, body: unknown): Promise<T> {
  assertWorkspacePath(path)
  const res = await fetch(path, {
    method: "PUT",
    headers: jsonHeaders(),
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}
```

（`jsonHeaders` / `handleResponse` / `assertWorkspacePath` 复用既有 helper）

- [ ] **Step 3: Commit（与 Task 5 页面一起提交）**

---

### Task 5: SSO 配置页组件

**Files:**
- Create: `web/src/features/workspace/sso/sso-config-page.tsx`

- [ ] **Step 1: 实现配置页（仿 token-form.tsx 受控表单）**

```tsx
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useMe } from "@/features/workspace/session/useMe"
import { PageHeader } from "@/pages/OverviewPage"
import { ApiError } from "@/lib/api"

import {
  useSaveSsoConfigMutation,
  useSsoConfigQuery,
  useTriggerSyncMutation,
  type SsoConfigInput,
} from "./use-sso"

export function SsoConfigPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { t } = useTranslation()
  const me = useMe()
  const role = me.data?.effective_role ?? ""
  const isOwner = role === "owner"
  const canManage = isOwner // acting 模式由 nav 显隐保证进入此页

  const configQuery = useSsoConfigQuery(workspaceSlug)
  const saveMutation = useSaveSsoConfigMutation(workspaceSlug)
  const syncMutation = useTriggerSyncMutation(workspaceSlug)

  const enabled = configQuery.data?.enabled ?? false
  const cfg = configQuery.data?.config

  const [form, setForm] = useState<SsoConfigInput>({
    issuer_base_url: "",
    org_id: "",
    client_id: "",
    client_secret: "",
    directory_access_token: "",
    sync_interval: "1h",
    external_base_url: "",
    session_ttl: "168h",
  })
  const [loaded, setLoaded] = useState(false)

  // 首次加载配置后预填表单（secret 留空，保存时空值=不修改）
  if (configQuery.isSuccess && cfg && !loaded) {
    setForm({
      issuer_base_url: cfg.issuer_base_url,
      org_id: cfg.org_id,
      client_id: cfg.client_id,
      client_secret: "",
      directory_access_token: "",
      sync_interval: cfg.sync_interval || "1h",
      external_base_url: cfg.external_base_url,
      session_ttl: cfg.session_ttl || "168h",
    })
    setLoaded(true)
  }

  if (configQuery.isLoading) {
    return <Skeleton className="h-96 w-full" />
  }
  if (!canManage) {
    return <p className="text-sm text-muted-foreground">{t("sso.noPermission")}</p>
  }

  const update = (key: keyof SsoConfigInput, value: string) =>
    setForm((prev) => ({ ...prev, [key]: value }))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    saveMutation.mutate(form)
  }

  return (
    <form className="space-y-8" onSubmit={handleSubmit}>
      <div className="flex items-center justify-between">
        <PageHeader title={t("sso.title")} description={t("sso.description")} />
      </div>

      {!enabled && (
        <p className="text-sm text-muted-foreground">{t("sso.notEnabled")}</p>
      )}

      {/* IdP 连接 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionIdp")}</h3>
        <Field label={t("sso.field.issuerBaseUrl")} hint={t("sso.hint.issuerBaseUrl")}>
          <Input value={form.issuer_base_url} onChange={(e) => update("issuer_base_url", e.target.value)} required />
        </Field>
        <Field label={t("sso.field.orgId")} hint={t("sso.hint.orgId")}>
          <Input value={form.org_id} onChange={(e) => update("org_id", e.target.value)} required />
        </Field>
        <Field label={t("sso.field.clientId")} hint={t("sso.hint.clientId")}>
          <Input value={form.client_id} onChange={(e) => update("client_id", e.target.value)} required />
        </Field>
        <Field
          label={t("sso.field.clientSecret")}
          hint={t("sso.hint.clientSecret")}
          badge={cfg?.client_secret_masked ? `${t("sso.secretSet")} ${cfg.client_secret_masked}` : t("sso.secretUnset")}
        >
          <Input type="password" value={form.client_secret} onChange={(e) => update("client_secret", e.target.value)} placeholder="••••••••" />
        </Field>
      </section>

      {/* 通讯录同步 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionDirectory")}</h3>
        <Field
          label={t("sso.field.directoryAccessToken")}
          hint={t("sso.hint.directoryAccessToken")}
          badge={cfg?.directory_access_token_masked ? `${t("sso.secretSet")} ${cfg.directory_access_token_masked}` : t("sso.secretUnset")}
        >
          <Input type="password" value={form.directory_access_token} onChange={(e) => update("directory_access_token", e.target.value)} placeholder="••••••••" />
        </Field>
        <Field label={t("sso.field.syncInterval")} hint={t("sso.hint.syncInterval")}>
          <Select value={form.sync_interval} onValueChange={(v) => update("sync_interval", v)}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="0">禁用</SelectItem>
              <SelectItem value="30m">30 分钟</SelectItem>
              <SelectItem value="1h">1 小时</SelectItem>
              <SelectItem value="6h">6 小时</SelectItem>
              <SelectItem value="24h">24 小时</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </section>

      {/* 高级 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionAdvanced")}</h3>
        <Field label={t("sso.field.externalBaseUrl")} hint={t("sso.hint.externalBaseUrl")}>
          <Input value={form.external_base_url} onChange={(e) => update("external_base_url", e.target.value)} />
        </Field>
        <Field label={t("sso.field.sessionTtl")} hint={t("sso.hint.sessionTtl")}>
          <Select value={form.session_ttl} onValueChange={(v) => update("session_ttl", v)}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="24h">1 天</SelectItem>
              <SelectItem value="168h">7 天</SelectItem>
              <SelectItem value="720h">30 天</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </section>

      {/* 错误提示 */}
      {saveMutation.isError && (
        <p className="text-sm text-destructive">
          {t("sso.errors.save")}: {errorMessage(saveMutation.error)}
        </p>
      )}
      {syncMutation.isError && (
        <p className="text-sm text-destructive">
          {t("sso.errors.sync")}: {errorMessage(syncMutation.error)}
        </p>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <Button type="submit" disabled={saveMutation.isPending}>
          {t("sso.save")}
        </Button>
      </div>

      {/* 成员同步 */}
      <section className="space-y-4 border-t pt-4">
        <h3 className="text-sm font-medium">{t("sso.sectionMembers")}</h3>
        <div className="flex items-center gap-3">
          <Button
            type="button"
            variant="outline"
            onClick={() => syncMutation.mutate()}
            disabled={!enabled || syncMutation.isPending}
          >
            {syncMutation.isPending ? t("sso.syncRunning") : t("sso.syncNow")}
          </Button>
        </div>
      </section>
    </form>
  )
}

function Field({
  label, hint, badge, children,
}: {
  label: string
  hint?: string
  badge?: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <Label className="text-sm">{label}</Label>
        {badge && <span className="text-xs text-muted-foreground">{badge}</span>}
      </div>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return String(err)
}
```

> 注意：`SsoConfigPage` 接收 `workspaceSlug` prop。需确认 workspace root route 如何向下传 slug（参考 TokensPage 如何拿到 workspaceSlug——通常从 route params 或 useMe 的 effective_workspace.slug）。若现有页面通过 `useMe().data?.effective_workspace.slug` 获取，照搬。

- [ ] **Step 2: 确认 workspaceSlug 来源**

读 `web/src/routes/workspace/TokensRoute.tsx` 和 tokens-page 如何获取 workspaceSlug。若是 route loader / params，照搬到 SsoRoute。若是组件内 `useMe`，在 SsoConfigPage 内同样获取。

- [ ] **Step 3: 类型检查 + 构建**

```bash
cd web && pnpm tsc --noEmit && pnpm build
```
Expected: 无错误。

- [ ] **Step 4: Commit（Task 3+4+5 一起）**

```bash
git add web/src/routes/workspace/SsoRoute.tsx web/src/routes/router.tsx web/src/features/workspace/sso/ web/src/features/workspace/session/workspace-api.ts
git commit -m "feat(web): SSO 配置页（表单+同步触发）"
```

---

### Task 6: 登录页 OIDC 登录入口

**Files:**
- 找到 Web Console 登录页组件（token 输入页），加「OIDC 登录」按钮

- [ ] **Step 1: 定位登录页**

搜索登录页组件（`signInTitle` 在 `web/src/locales/zh-CN.ts:20` 附近被引用）。找到渲染 token 输入框的组件文件。

- [ ] **Step 2: 在登录表单下加 OIDC 登录按钮**

```tsx
<Button
  type="button"
  variant="outline"
  onClick={() => {
    const slug = workspaceSlug // 当前输入或选中的 workspace
    window.location.href = `/sso/oidc/start?workspace=${encodeURIComponent(slug)}`
  }}
>
  OIDC 单点登录
</Button>
```

> 按钮跳转到 `/sso/oidc/start`，由后端 302 到 yaoguang。callback 后回到 console 根并带 cookie。若 SSO 未启用某 workspace，后端返回错误页（带 `?sso_error=`）。

- [ ] **Step 3: 处理 SSO 错误回调**

在登录页读取 `?sso_error=` query param，显示对应中文提示（identity_not_found / membership_inactive / invalid_state 等）。

- [ ] **Step 4: 类型检查 + 构建**

```bash
cd web && pnpm tsc --noEmit && pnpm build
```

- [ ] **Step 5: Commit**

```bash
git add web/src/...（登录页文件）
git commit -m "feat(web): 登录页加 OIDC 登录入口"
```

---

### Task 7: 构建前端产物 + embed

**Files:**
- 由构建产出 `internal/webconsole/dist`

- [ ] **Step 1: 构建前端并嵌入**

```bash
make web-console-build
```

> 该命令重新生成 `internal/webconsole/dist`（被 Go embed），参照 `docs/manual/web-console.md`。

- [ ] **Step 2: 验证后端构建包含新前端**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 3: Commit**

```bash
git add internal/webconsole/dist
git commit -m "build: 重新构建 web console（含 SSO 页面）"
```

---

### Task 8: 阶段三全量验证

- [ ] **Step 1: 全量后端测试**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全 PASS。

- [ ] **Step 2: 端到端验证**

1. owner 登录 → 侧边栏看到「单点登录」→ 配置 yaoguang 信息 → 保存
2. 点「立即同步成员」→ 成员同步
3. 退出 → 登录页点「OIDC 单点登录」→ 跳转 yaoguang → 回到 console 带 cookie
4. member/viewer 登录 → 看不到「单点登录」菜单
5. cookie 只读：GET 成功、POST 返回 403

- [ ] **Step 3: 更新文档**

更新 `docs/manual/web-console.md`：新增 OIDC 登录入口、通讯录同步操作说明、cookie 只读约束。
更新 `README.md`：新增 SSO 登录说明。
更新 `ROADMAP.md`：标记 v0.5.0 OIDC 完成。

- [ ] **Step 4: Commit**

```bash
git add docs/manual/web-console.md README.md ROADMAP.md
git commit -m "docs: OIDC 接入完成，更新文档"
```

---

## 阶段三完成标准

- [ ] 「单点登录」菜单仅 owner + admin acting 可见
- [ ] SSO 配置页可读写 yaoguang 配置（secret 脱敏、留空不改）
- [ ] 可触发通讯录同步
- [ ] 登录页有 OIDC 登录入口
- [ ] 前端构建产物已嵌入
- [ ] 全量 `go test` + `CGO_ENABLED=0` 通过
- [ ] 文档已同步
