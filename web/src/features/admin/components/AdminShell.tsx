import type React from "react"
import { KeyRound, LogOut, RefreshCw, ShieldAlert } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"

import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"

import { AdminRiskBadge } from "./AdminRiskBadge"

type AdminShellProps = {
  children: React.ReactNode
  onLogout: () => void
  onRefresh: () => void
  tokenName?: string
}

const navLinkBase =
  "flex h-8 w-full items-center gap-2 border-l-2 px-2 text-left text-xs font-medium transition-colors hover:bg-muted hover:text-foreground border-l-transparent text-muted-foreground"
const navLinkActive =
  "border-l-foreground bg-muted text-foreground"

export function AdminShell({
  children,
  onLogout,
  onRefresh,
  tokenName,
}: AdminShellProps) {
  const { t } = useTranslation()

  return (
    <div className="min-h-svh bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-56 border-r bg-background md:block">
        <div className="flex h-12 items-center border-b px-4 text-sm font-medium">
          <ProductLogo />
        </div>
        <nav aria-label={t("admin.navLabel")} className="space-y-1 p-2">
          <Link
            activeOptions={{ exact: true }}
            activeProps={{ className: navLinkActive }}
            className={navLinkBase}
            to="/admin"
          >
            <ShieldAlert className="size-3.5" />
            {t("admin.nav.bootstrap")}
          </Link>
          <Link
            activeProps={{ className: navLinkActive }}
            className={navLinkBase}
            to="/admin/tokens"
          >
            <KeyRound className="size-3.5" />
            {t("admin.nav.tokens")}
          </Link>
        </nav>
        <div className="absolute inset-x-0 bottom-0 border-t p-3">
          <div className="mb-2 text-[11px] text-muted-foreground uppercase">
            {t("admin.riskLabel")}
          </div>
          <AdminRiskBadge />
        </div>
      </aside>
      <div className="md:pl-56">
        <header className="sticky top-0 z-20 flex h-12 items-center justify-between border-b bg-background/95 px-4 backdrop-blur">
          <div className="min-w-0">
            <div className="truncate text-xs text-muted-foreground">
              {t("admin.shellSubtitle")}
            </div>
            <div className="truncate text-sm font-medium">
              {t("admin.shellTitle", { tokenName: tokenName || "-" })}
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
        <main className="mx-auto max-w-7xl px-4 py-5">{children}</main>
      </div>
    </div>
  )
}
