import * as React from "react"
import { useState } from "react"
import { Building2, KeyRound, LogOut, Menu, RefreshCw, ShieldAlert } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"

import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { Sheet, SheetContent, SheetHeader, SheetTrigger } from "@/components/ui/sheet"

import { AdminRiskBadge } from "./AdminRiskBadge"

type AdminShellProps = {
  children: React.ReactNode
  onLogout: () => void
  onRefresh: () => void
  tokenName?: string
}

const navLinkBase =
  "relative flex h-9 w-full items-center gap-2 rounded-md px-2.5 text-left text-[13px] font-medium transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-foreground text-sidebar-muted-foreground"
const navLinkActive =
  "bg-sidebar-accent font-semibold text-sidebar-accent-foreground"

export function AdminShell({
  children,
  onLogout,
  onRefresh,
  tokenName,
}: AdminShellProps) {
  const { t } = useTranslation()
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  const sidebarInner = (
    <>
      <nav aria-label={t("admin.navLabel")} className="space-y-1 p-2">
        <Link
          activeOptions={{ exact: true }}
          activeProps={{ className: navLinkActive, "data-active": "true" }}
          className={navLinkBase}
          onClick={() => setMobileNavOpen(false)}
          to="/admin"
        >
          <ShieldAlert className="size-4 shrink-0" />
          {t("admin.nav.bootstrap")}
        </Link>
        <Link
          activeProps={{ className: navLinkActive, "data-active": "true" }}
          className={navLinkBase}
          onClick={() => setMobileNavOpen(false)}
          to="/admin/workspaces"
        >
          <Building2 className="size-4 shrink-0" />
          {t("admin.nav.workspaces")}
        </Link>
        <Link
          activeProps={{ className: navLinkActive, "data-active": "true" }}
          className={navLinkBase}
          onClick={() => setMobileNavOpen(false)}
          to="/admin/tokens"
        >
          <KeyRound className="size-4 shrink-0" />
          {t("admin.nav.tokens")}
        </Link>
      </nav>
      <div className="mt-auto border-t border-sidebar-border p-3">
        <div className="mb-2 text-[11px] text-sidebar-muted-foreground uppercase">
          {t("admin.riskLabel")}
        </div>
        <AdminRiskBadge />
      </div>
    </>
  )

  return (
    <div className="min-h-svh bg-background text-foreground">
      {/* 桌面端固定侧栏（>=md 显示）。深色骨架，恒深色。 */}
      <aside className="fixed inset-y-0 left-0 hidden w-[248px] flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground md:flex">
        <div className="flex h-14 items-center border-b border-sidebar-border px-4 text-sm font-medium">
          <ProductLogo />
        </div>
        <div className="flex flex-1 flex-col">{sidebarInner}</div>
      </aside>
      <div className="md:pl-[248px]">
        <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b bg-background/95 px-4 backdrop-blur">
          <div className="flex min-w-0 items-center gap-2">
            {/* 移动端汉堡按钮（<md 显示） */}
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
              <SheetContent aria-label={t("admin.navLabel")} role="dialog">
                <SheetHeader>
                  <ProductLogo />
                </SheetHeader>
                <div className="flex flex-1 flex-col">{sidebarInner}</div>
              </SheetContent>
            </Sheet>
            <div className="min-w-0">
              <div className="truncate text-xs text-muted-foreground">
                {t("admin.shellSubtitle")}
              </div>
              <div className="truncate text-sm font-medium">
                {t("admin.shellTitle", { tokenName: tokenName || "-" })}
              </div>
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
            <Separator className="hidden h-5 sm:inline-flex" orientation="vertical" />
            <LanguageSwitcher />
            <ThemeToggle />
            <Button onClick={onLogout} size="sm" variant="outline">
              <LogOut className="size-4" />
              {t("auth.logout")}
            </Button>
          </div>
        </header>
        <main className="mx-auto max-w-7xl px-4 py-5 md:px-6">{children}</main>
      </div>
    </div>
  )
}
