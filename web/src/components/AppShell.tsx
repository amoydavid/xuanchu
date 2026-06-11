import {
  Activity,
  Bell,
  Boxes,
  ClipboardList,
  FileClock,
  KeyRound,
  RefreshCw,
  Settings,
  Users,
  Webhook,
} from "lucide-react"
import type React from "react"
import { useTranslation } from "react-i18next"

import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { RiskBadge } from "@/components/RiskBadge"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { cn } from "@/lib/utils"

export type PageKey =
  | "overview"
  | "tasks"
  | "projects"
  | "workspaces"
  | "members"
  | "tokens"
  | "hooks"
  | "notifications"
  | "audit"
  | "settings"

const navItems: Array<{ key: PageKey; icon: React.ComponentType<{ className?: string }> }> = [
  { key: "overview", icon: Activity },
  { key: "tasks", icon: ClipboardList },
  { key: "projects", icon: Boxes },
  { key: "workspaces", icon: Boxes },
  { key: "members", icon: Users },
  { key: "tokens", icon: KeyRound },
  { key: "hooks", icon: Webhook },
  { key: "notifications", icon: Bell },
  { key: "audit", icon: FileClock },
  { key: "settings", icon: Settings },
]

export function AppShell({
  activePage,
  actorName,
  children,
  onNavigate,
  onRefresh,
  tokenType,
  workspaceSlug,
}: {
  activePage: PageKey
  actorName?: string
  children: React.ReactNode
  onNavigate: (page: PageKey) => void
  onRefresh: () => void
  tokenType?: string
  workspaceSlug?: string
}) {
  const { t } = useTranslation()

  return (
    <div className="min-h-svh bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-56 border-r bg-background md:block">
        <div className="flex h-12 items-center border-b px-4 text-sm font-medium">
          Xuanchu
        </div>
        <nav className="p-2">
          {navItems.map((item) => {
            const Icon = item.icon
            const active = activePage === item.key
            return (
              <button
                className={cn(
                  "flex h-8 w-full items-center gap-2 border-l-2 px-2 text-left text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                  active
                    ? "border-l-foreground bg-muted font-medium text-foreground"
                    : "border-l-transparent"
                )}
                key={item.key}
                onClick={() => onNavigate(item.key)}
                type="button"
              >
                <Icon className="size-3.5" />
                {t(`nav.${item.key}`)}
              </button>
            )
          })}
        </nav>
        <div className="absolute inset-x-0 bottom-0 border-t p-3">
          <div className="mb-2 text-[11px] uppercase text-muted-foreground">
            {t("shell.tokenRisk")}
          </div>
          <RiskBadge risk="normal" />
        </div>
      </aside>
      <div className="md:pl-56">
        <header className="sticky top-0 z-20 flex h-12 items-center justify-between border-b bg-background/95 px-4 backdrop-blur">
          <div className="min-w-0">
            <div className="truncate text-xs text-muted-foreground">
              {workspaceSlug ? `${t("shell.workspace")}: ${workspaceSlug}` : t("shell.workspace")}
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
          </div>
        </header>
        <main className="mx-auto max-w-7xl px-4 py-5">{children}</main>
      </div>
    </div>
  )
}
