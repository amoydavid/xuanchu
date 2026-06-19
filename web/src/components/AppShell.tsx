import {
  Activity,
  ArrowLeft,
  Bell,
  Boxes,
  FileClock,
  KeyRound,
  LogOut,
  RefreshCw,
  Settings,
  Users,
  Webhook,
} from "lucide-react"
import { Link } from "@tanstack/react-router"
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
  getAdminActingContext,
} from "@/features/workspace/session/workspace-token"
import { cn } from "@/lib/utils"

export type PageKey =
  | "overview"
  | "projects"
  | "workspaces"
  | "members"
  | "tokens"
  | "hooks"
  | "notifications"
  | "audit"
  | "settings"

const navItems: Array<{
  key: PageKey
  icon: React.ComponentType<{ className?: string }>
  to: string
}> = [
  { key: "overview", icon: Activity, to: "/" },
  { key: "projects", icon: Boxes, to: "/projects" },
  { key: "workspaces", icon: Boxes, to: "/workspaces" },
  { key: "members", icon: Users, to: "/members" },
  { key: "tokens", icon: KeyRound, to: "/tokens" },
  { key: "hooks", icon: Webhook, to: "/hooks" },
  { key: "notifications", icon: Bell, to: "/notifications" },
  { key: "audit", icon: FileClock, to: "/audit" },
  { key: "settings", icon: Settings, to: "/settings" },
]

export function AppShell({
  actorName,
  children,
  onLogout,
  onRefresh,
  tokenType,
  workspaceSlug,
}: {
  actorName?: string
  children: React.ReactNode
  onLogout: () => void
  onRefresh: () => void
  tokenType?: string
  workspaceSlug?: string
}) {
  const { t } = useTranslation()

  return (
    <div className="min-h-svh bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-56 border-r bg-background md:block">
        <div className="flex h-12 items-center border-b px-4 text-sm font-medium">
          <ProductLogo />
        </div>
        <nav className="p-2">
          {navItems.map((item) => {
            const Icon = item.icon
            return (
              <Link
                activeOptions={{ exact: item.to === "/" }}
                activeProps={{
                  className:
                    "border-l-foreground bg-muted font-medium text-foreground",
                }}
                className={cn(
                  "flex h-8 w-full items-center gap-2 border-l-2 px-2 text-left text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                  "border-l-transparent"
                )}
                key={item.key}
                to={item.to}
              >
                <Icon className="size-3.5" />
                {t(`nav.${item.key}`)}
              </Link>
            )
          })}
        </nav>
        <div className="absolute inset-x-0 bottom-0 border-t p-3">
          <div className="mb-2 text-[11px] text-muted-foreground uppercase">
            {t("shell.tokenRisk")}
          </div>
          <RiskBadge risk="normal" />
        </div>
      </aside>
      <div className="md:pl-56">
        <ActingBanner />
        <header className="sticky top-0 z-20 flex h-12 items-center justify-between border-b bg-background/95 px-4 backdrop-blur">
          <div className="min-w-0">
            <div className="truncate text-xs text-muted-foreground">
              {workspaceSlug
                ? `${t("shell.workspace")}: ${workspaceSlug}`
                : t("shell.workspace")}
            </div>
            <div className="truncate text-sm font-medium">
              {actorName ? `${actorName} · ${tokenType ?? ""}` : t("app.title")}
            </div>
          </div>
          <div className="flex items-center gap-2">
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
            <Button onClick={onLogout} size="sm" variant="outline">
              <LogOut className="size-4" />
              {t("auth.logout")}
            </Button>
          </div>
        </header>
        <main className="px-4 py-5">{children}</main>
      </div>
    </div>
  )
}

// ActingBanner 在 acting mode 下持续显示，提醒用户当前以 workspace
// 管理员身份操作。点击「返回超管界面」清理 acting token/context，跳回 workspace 详情。
// 普通 workspace console（无 acting context）不渲染任何东西。
function ActingBanner() {
  const { t } = useTranslation()
  const context = getAdminActingContext()
  if (!context) {
    return null
  }
  return (
    <div className="flex items-center justify-between gap-3 border-b bg-amber-100 px-4 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-100">
      <div className="truncate">
        {t("admin.acting.banner", {
          workspace: context.workspaceName,
          actor: context.actorName,
          role: context.role,
          adminTokenName: context.adminTokenName,
        })}
      </div>
      <Button
        onClick={() => {
          clearAdminActingSession()
          // 返回到发起 acting 的 workspace 详情页。
          window.location.assign(
            `/admin/workspaces/${encodeURIComponent(context.workspaceSlug)}`
          )
        }}
        size="sm"
        variant="outline"
      >
        <ArrowLeft className="size-4" />
        {t("admin.acting.returnToAdmin")}
      </Button>
    </div>
  )
}
