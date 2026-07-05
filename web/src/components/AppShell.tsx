import {
  Activity,
  ArrowLeft,
  Bell,
  Boxes,
  CheckSquare,
  FileClock,
  KeyRound,
  LogOut,
  RefreshCw,
  Settings,
  ShieldAlert,
  ShieldCheck,
  Users,
  Webhook,
} from "lucide-react"
import { Link, useLocation } from "@tanstack/react-router"
import type React from "react"
import { useTranslation } from "react-i18next"

import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { RiskBadge } from "@/components/RiskBadge"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import {
  clearAdminActingSession,
  clearTenantSwitchSession,
  getAdminActingContext,
  getTenantSwitchContext,
  type ActingContext,
  type TenantSwitchContext,
} from "@/features/workspace/session/workspace-token"
import { useMe } from "@/features/workspace/session/useMe"
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
      { key: "overview", icon: Activity, to: "/" },
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

  return (
    <div className="min-h-svh bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-56 flex-col border-r bg-background md:flex">
        <div className="flex h-12 items-center border-b px-4 text-sm font-medium">
          <ProductLogo />
        </div>
        <nav className="flex-1 overflow-y-auto p-2">
          {navGroups.map((group) => {
            const items = group.items.filter(
              (item) => !item.ssoOnly || showSso
            )
            if (items.length === 0) return null
            return (
              <div className="mb-3" key={group.labelKey}>
                <div className="px-2 pb-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                  {t(group.labelKey)}
                </div>
                {items.map((item) => {
                  const Icon = item.icon
                  const active = isNavItemActive(
                    item.key,
                    item.to,
                    location.pathname
                  )
                  return (
                    <Link
                      className={cn(
                        "flex h-8 w-full items-center gap-2 border-l-2 px-2 text-left text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                        active
                          ? "border-l-foreground bg-muted font-medium text-foreground"
                          : "border-l-transparent"
                      )}
                      key={item.key}
                      to={item.to}
                    >
                      <Icon className="size-3.5" />
                      {t(`nav.${item.key}`)}
                    </Link>
                  )
                })}
              </div>
            )
          })}
        </nav>
        <IdentityBlock
          me={me.data}
          onLogout={onLogout}
          showRisk={showRisk}
          acting={acting}
          actingContext={actingContext}
          tenantSwitch={tenantSwitch}
          tenantContext={tenantContext}
        />
      </aside>
      <div className="md:pl-56">
        <header
          className={cn(
            "sticky top-0 z-20 flex h-12 items-center justify-between border-b bg-background/95 px-4 backdrop-blur",
            (acting || tenantSwitch) && "bg-amber-50/95 dark:bg-amber-950/40"
          )}
        >
          <div className="flex min-w-0 items-center gap-2">
            {(acting || systemActor) && !tenantSwitch ? (
              <ShieldAlert className="size-4 shrink-0 text-amber-600 dark:text-amber-400" />
            ) : null}
            <div className="min-w-0 truncate text-sm font-medium">
              {breadcrumbs ?? headerTitle ?? t("app.title")}
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
            <Separator className="h-5" orientation="vertical" />
            <LanguageSwitcher />
            <ThemeToggle />
          </div>
        </header>
        <main className="px-4 py-5">{children}</main>
      </div>
    </div>
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
  const handle =
    me?.actor.name && me?.actor.display_name ? me.actor.name : undefined
  const workspaceSlug = me?.effective_workspace.slug ?? ""
  const tokenType = me?.token.type ?? ""

  return (
    <div className="border-t p-3">
      <div className="mb-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
        {t("shell.identity")}
      </div>
      <div className="truncate text-sm font-medium">{displayName}</div>
      <div className="mt-0.5 truncate text-[11px] text-muted-foreground">
        {[handle, me?.effective_role, tokenType, workspaceSlug]
          .filter(Boolean)
          .join(" · ")}
      </div>
      {showRisk ? (
        <div className="mt-2">
          <RiskBadge risk="high" />
        </div>
      ) : null}
      <div className="mt-2 flex items-center gap-1">
        {tenantSwitch && tenantContext ? (
          <ReturnButton
            label={t("shell.returnToAdmin")}
            onClick={() => {
              clearTenantSwitchSession()
              navigateToDocument(tenantContext.returnTo)
            }}
          />
        ) : acting && actingContext ? (
          <ReturnButton
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
        ) : (
          <Button onClick={onLogout} size="sm" variant="outline">
            <LogOut className="size-4" />
            {t("auth.logout")}
          </Button>
        )}
      </div>
    </div>
  )
}

function ReturnButton({
  label,
  onClick,
}: {
  label: string
  onClick: () => void
}) {
  return (
    <Button
      className="border-amber-300 bg-amber-100 text-amber-900 hover:bg-amber-200 hover:text-amber-900 dark:border-amber-700 dark:bg-amber-900 dark:text-amber-100 dark:hover:bg-amber-800"
      onClick={onClick}
      size="sm"
      variant="outline"
    >
      <ArrowLeft className="size-4" />
      {label}
    </Button>
  )
}

function isNavItemActive(key: PageKey, to: string, pathname: string): boolean {
  if (key === "projects") {
    return (
      pathname === "/projects" ||
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
