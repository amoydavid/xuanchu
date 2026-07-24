import {
  ArrowLeft,
  Bell,
  Boxes,
  CheckSquare,
  ChevronDown,
  FileClock,
  House,
  KeyRound,
  LogOut,
  Menu,
  RefreshCw,
  Settings,
  ShieldAlert,
  ShieldCheck,
  Users,
  Webhook,
} from "lucide-react"
import { Link, useLocation } from "@tanstack/react-router"
import * as React from "react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { useBrandName } from "@/brand/BrandContext"
import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Separator } from "@/components/ui/separator"
import { Sheet, SheetContent, SheetHeader, SheetTrigger } from "@/components/ui/sheet"
import {
  clearAdminActingSession,
  clearTenantSwitchSession,
  getAdminActingContext,
  getTenantSwitchContext,
  type ActingContext,
  type TenantSwitchContext,
} from "@/features/workspace/session/workspace-token"
import { useMe } from "@/features/workspace/session/useMe"
import { memberRoleLabel } from "@/features/workspace/members/role-label"
import { navigateToDocument } from "@/lib/browser-navigation"
import { cn } from "@/lib/utils"

export type PageKey =
  | "overview"
  | "myTasks"
  | "projects"
  | "workspaces"
  | "members"
  | "tokens"
  | "sso"
  | "hooks"
  | "notifications"
  | "integrations"
  | "audit"
  | "settings"

type NavItem = {
  key: PageKey
  icon: React.ComponentType<{ className?: string }>
  to: string
  ssoOnly?: boolean
}

// 按角色任务流分组：个人 / 管理 / 系统
const navGroups: Array<{ labelKey: string; items: NavItem[] }> = [
  {
    labelKey: "nav.group.personal",
    items: [
      { key: "overview", icon: House, to: "/" },
      { key: "myTasks", icon: CheckSquare, to: "/my-tasks" },
      { key: "projects", icon: Boxes, to: "/projects" },
    ],
  },
  {
    labelKey: "nav.group.management",
    items: [
      { key: "members", icon: Users, to: "/members" },
      { key: "tokens", icon: KeyRound, to: "/tokens" },
      { key: "hooks", icon: Webhook, to: "/hooks" },
      { key: "notifications", icon: Bell, to: "/notifications" },
      { key: "sso", icon: ShieldCheck, to: "/sso", ssoOnly: true },
    ],
  },
  {
    labelKey: "nav.group.system",
    items: [
      { key: "workspaces", icon: Boxes, to: "/workspaces" },
      { key: "audit", icon: FileClock, to: "/audit" },
      { key: "settings", icon: Settings, to: "/settings" },
    ],
  },
]

export function AppShell({
  breadcrumbs,
  children,
  headerActions,
  headerTitle,
  onLogout,
  onRefresh,
}: {
  breadcrumbs?: React.ReactNode
  children: React.ReactNode
  headerActions?: React.ReactNode
  headerTitle?: React.ReactNode
  onLogout: () => void
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const brandName = useBrandName()
  const location = useLocation()
  const me = useMe()
  const role = me.data?.effective_role ?? ""
  const isOwner = role === "owner"
  const actingContext = getAdminActingContext()
  const tenantContext = getTenantSwitchContext()
  const acting = actingContext !== null
  const tenantSwitch = !acting && tenantContext !== null
  const systemActor = me.data?.actor_type === "tenant_access_token"
  const showSso = isOwner || systemActor || tenantSwitch
  const showRisk = acting || systemActor || tenantSwitch
  // 移动端导航抽屉开关。路由变化后由 NavItem 的 onClick 关闭。
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  const identityBlock = (
    <IdentityBlock
      me={me.data}
      onLogout={onLogout}
      showRisk={showRisk}
      acting={acting}
      actingContext={actingContext}
      tenantSwitch={tenantSwitch}
      tenantContext={tenantContext}
    />
  )

  return (
    <div className="min-h-svh bg-background text-foreground">
      {/* 桌面端固定侧栏（>=md 显示）。深色骨架，恒深色。 */}
      <aside className="fixed inset-y-0 left-0 hidden w-[248px] flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground md:flex">
        <SidebarNav
          showSso={showSso}
          pathname={location.pathname}
          logoHeader
        />
        {identityBlock}
      </aside>
      <div className="md:pl-[248px]">
        <header
          className={cn(
            "sticky top-0 z-20 flex h-14 items-center justify-between border-b bg-background/95 px-4 backdrop-blur",
            (acting || tenantSwitch) && "bg-warn/10"
          )}
        >
          <div className="flex min-w-0 items-center gap-2">
            {/* 移动端汉堡按钮（<md 显示），打开导航抽屉 */}
            <Sheet
              onOpenChange={setMobileNavOpen}
              open={mobileNavOpen}
            >
              <SheetTrigger asChild>
                <Button
                  aria-label={t("shell.openMenu")}
                  className="-ml-2 md:hidden"
                  size="icon-sm"
                  variant="ghost"
                >
                  <Menu className="size-4" />
                </Button>
              </SheetTrigger>
              <SheetContent
                aria-label={t("shell.navLabel")}
                className="border-sidebar-border bg-sidebar text-sidebar-foreground"
                role="dialog"
              >
                <SheetHeader className="border-sidebar-border">
                  <ProductLogo />
                </SheetHeader>
                <SidebarNav
                  showSso={showSso}
                  pathname={location.pathname}
                  onNavigate={() => setMobileNavOpen(false)}
                />
                {identityBlock}
              </SheetContent>
            </Sheet>
            {(acting || systemActor) && !tenantSwitch ? (
              <ShieldAlert className="size-4 shrink-0 text-warn" />
            ) : null}
            <div className="min-w-0 truncate text-sm font-medium">
              {breadcrumbs ?? headerTitle ?? t("app.title", { brand: brandName })}
            </div>
          </div>
          <div className="flex items-center gap-2">
            {headerActions}
            <Button
              aria-label={t("common.refresh")}
              onClick={onRefresh}
              size="icon-sm"
              variant="ghost"
            >
              <RefreshCw className="size-4" />
            </Button>
            <Separator className="hidden h-5 sm:inline-flex" orientation="vertical" />
            <LanguageSwitcher />
            <ThemeToggle />
          </div>
        </header>
        <main className="px-4 py-5 md:px-6">{children}</main>
      </div>
    </div>
  )
}

// SidebarNav：导航主体，桌面 aside 与移动抽屉共用同一份渲染逻辑。
// logoHeader=true 时渲染顶部 logo 行（桌面端用），移动端在 SheetHeader 单独渲染。
function SidebarNav({
  showSso,
  pathname,
  logoHeader = false,
  onNavigate,
}: {
  showSso: boolean
  pathname: string
  logoHeader?: boolean
  onNavigate?: () => void
}) {
  const { t } = useTranslation()
  return (
    <>
      {logoHeader ? (
        <div className="flex h-14 items-center border-b border-sidebar-border px-4 text-sm font-medium">
          <ProductLogo />
        </div>
      ) : null}
      <nav
        aria-label={t("shell.navLabel")}
        className="flex-1 overflow-y-auto p-2"
      >
        {navGroups.map((group) => {
          const items = group.items.filter(
            (item) => !item.ssoOnly || showSso
          )
          if (items.length === 0) return null
          return (
            <div className="mb-3" key={group.labelKey}>
              <div className="px-2 pb-1 text-[10px] font-medium uppercase tracking-wide text-sidebar-muted-foreground">
                {t(group.labelKey)}
              </div>
              {items.map((item) => {
                const Icon = item.icon
                const active = isNavItemActive(item.key, item.to, pathname)
                return (
                  <Link
                    className={cn(
                      "relative flex h-9 w-full items-center gap-2 rounded-md px-2.5 text-left text-[13px] text-sidebar-foreground/80 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-foreground",
                      active
                        ? "is-nav-active bg-sidebar-accent font-semibold text-sidebar-accent-foreground"
                        : "text-sidebar-muted-foreground"
                    )}
                    key={item.key}
                    onClick={onNavigate}
                    to={item.to}
                  >
                    <Icon className="size-4 shrink-0" />
                    {t(`nav.${item.key}`)}
                  </Link>
                )
              })}
            </div>
          )
        })}
      </nav>
    </>
  )
}

function IdentityBlock({
  acting,
  actingContext,
  me,
  onLogout,
  showRisk,
  tenantContext,
  tenantSwitch,
}: {
  acting: boolean
  actingContext: ActingContext | null
  me:
    | {
        actor: {
          id: string
          name: string
          display_name?: string
          email?: string | null
        }
        actor_type: string
        token: { type: string }
        effective_workspace: { slug: string; name?: string }
        effective_role: string
      }
    | undefined
  onLogout: () => void
  showRisk: boolean
  tenantContext: TenantSwitchContext | null
  tenantSwitch: boolean
}) {
  const { t } = useTranslation()
  const displayName =
    me?.actor.display_name ||
    me?.actor.name ||
    (tenantSwitch && tenantContext
      ? t("shell.systemIdentity")
      : t("shell.identity"))
  const workspaceSlug = me?.effective_workspace.slug ?? ""
  const workspaceName =
    me?.effective_workspace.name || workspaceSlug || t("shell.workspace")
  const tokenType = me?.token.type ?? ""
  const roleLabel = me?.effective_role
    ? memberRoleLabel(t, me.effective_role)
    : ""

  return (
    <div className="border-t border-sidebar-border p-2.5">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            className="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-sidebar-accent/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring data-[state=open]:bg-sidebar-accent/50"
            type="button"
          >
            <WorkspaceTile name={workspaceName} />
            <span className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="truncate text-[13px] font-semibold text-sidebar-foreground">
                {workspaceName}
              </span>
              <span className="flex items-center gap-1 truncate text-[11px] text-sidebar-muted-foreground">
                {showRisk ? (
                  <ShieldAlert className="size-3 shrink-0 text-warn" />
                ) : null}
                <span className="truncate">{roleLabel}</span>
              </span>
            </span>
            <UserAvatar displayName={displayName} />
            <ChevronDown className="size-3.5 shrink-0 text-sidebar-muted-foreground transition-transform duration-200 [[data-state=open]_&]:rotate-180" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          className="w-[248px] border-sidebar-border bg-sidebar-surface p-1.5 text-sidebar-foreground ring-0"
          side="top"
          sideOffset={8}
        >
          {/* 当前用户信息块 */}
          <div className="flex items-center gap-2.5 px-2 py-1.5">
            <UserAvatar displayName={displayName} size="lg" />
            <span className="flex min-w-0 flex-col gap-0">
              <span className="truncate text-[13px] font-semibold text-sidebar-foreground">
                {displayName}
              </span>
              {me?.actor.email ? (
                <span className="truncate font-mono text-[10.5px] text-sidebar-muted-foreground">
                  {me.actor.email}
                </span>
              ) : null}
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 px-2 pb-1.5 text-[10.5px] text-sidebar-muted-foreground">
            {tokenType ? <span>{tokenType}</span> : null}
            {workspaceSlug ? (
              <span className="truncate font-mono">{workspaceSlug}</span>
            ) : null}
          </div>
          <DropdownMenuSeparator className="bg-sidebar-border" />
          {/* acting / tenantSwitch：返回超管（危险态） */}
          {tenantSwitch && tenantContext ? (
            <ReturnItem
              label={t("shell.returnToAdmin")}
              onClick={() => {
                clearTenantSwitchSession()
                navigateToDocument(tenantContext.returnTo)
              }}
            />
          ) : acting && actingContext ? (
            <ReturnItem
              label={t("shell.returnToAdmin")}
              onClick={() => {
                clearAdminActingSession()
                navigateToDocument(
                  `/admin/workspaces/${encodeURIComponent(
                    actingContext.workspaceSlug
                  )}`
                )
              }}
            />
          ) : null}
          {/* 登出：危险操作，用 destructive 变体（文字/图标红、hover 红调暗底，对齐参考 ws-action.danger） */}
          <DropdownMenuItem
            className="gap-2 rounded-md px-2 py-1.5 text-[13px]"
            onClick={onLogout}
            variant="destructive"
          >
            <LogOut className="size-4 shrink-0" />
            {t("auth.logout")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

// 工作空间首字母色块（深色侧栏上的中性标识 tile，参考 ws-tile 的中性深灰渐变）。
function WorkspaceTile({ name }: { name: string }) {
  const glyph = (name || "?").trim().charAt(0).toUpperCase()
  return (
    <span className="grid size-7 shrink-0 place-items-center rounded-md bg-sidebar-foreground/15 font-heading text-[13px] font-bold text-sidebar-foreground">
      {glyph}
    </span>
  )
}

// 用户头像：取 displayName 首字，绿渐变底（品牌绿强调，深色侧栏上的唯一绿色点缀之一）。
function UserAvatar({
  displayName,
  size = "md",
}: {
  displayName: string
  size?: "md" | "lg"
}) {
  const glyph = (displayName || "?").trim().charAt(0).toUpperCase()
  return (
    <span
      className={cn(
        "grid shrink-0 place-items-center rounded-full border-2 border-sidebar bg-gradient-to-br from-primary to-primary/80 font-heading font-bold text-primary-foreground",
        size === "lg" ? "size-8 text-xs" : "size-6 text-[10px]"
      )}
    >
      {glyph}
    </span>
  )
}

// 浮层内的「返回超管」项（acting / tenantSwitch 态用，克制红字）。
function ReturnItem({
  label,
  onClick,
}: {
  label: string
  onClick: () => void
}) {
    return (
      <DropdownMenuItem
        className="gap-2 rounded-md px-2 py-1.5 text-[13px]"
        onClick={onClick}
        variant="destructive"
      >
        <ArrowLeft className="size-4 shrink-0" />
      {label}
    </DropdownMenuItem>
  )
}

function isNavItemActive(key: PageKey, to: string, pathname: string): boolean {
  if (key === "projects") {
    return (
      pathname === "/projects" ||
      pathname.startsWith("/projects/") ||
      /^\/workspaces\/[^/]+\/projects(?:\/|$)/.test(pathname)
    )
  }
  if (key === "myTasks") {
    return pathname === "/my-tasks" || pathname.startsWith("/tasks/")
  }
  if (key === "workspaces") {
    return pathname === "/workspaces"
  }
  if (to === "/") {
    return pathname === "/"
  }
  return pathname === to || pathname.startsWith(`${to}/`)
}
