import { useEffect, useState } from "react"
import type { FormEvent } from "react"
import { ArrowLeft, ShieldAlert, ShieldCheck } from "lucide-react"
import { useTranslation } from "react-i18next"

import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { AdminRiskBadge } from "@/features/admin/components/AdminRiskBadge"
import { adminApiGet } from "@/features/admin/session/admin-api"
import { setAdminToken } from "@/features/admin/session/admin-token"
import {
  getAdminStatus,
  type AdminStatus,
} from "@/features/admin/setup/admin-setup-api"
import { ApiError } from "@/lib/api"

type AdminLoginPageProps = {
  onSignedIn: () => void
}

export function AdminLoginPage({ onSignedIn }: AdminLoginPageProps) {
  const { t } = useTranslation()
  const [token, setTokenValue] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [status, setStatus] = useState<AdminStatus | null>(null)

  useEffect(() => {
    let mounted = true
    void getAdminStatus()
      .then((nextStatus) => {
        if (mounted) {
          setStatus(nextStatus)
        }
      })
      .catch(() => {
        if (mounted) {
          setStatus({ enabled: true, setup_required: false, status: "login_required" })
        }
      })
    return () => {
      mounted = false
    }
  }, [])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      setAdminToken(token.trim())
      await adminApiGet("/api/v1/admin/session")
      onSignedIn()
    } catch (err) {
      if (err instanceof ApiError && err.code === "admin_setup_required") {
        setStatus({ enabled: true, setup_required: true, status: "setup_required" })
      }
      setError(t("admin.authFailed"))
    } finally {
      setSubmitting(false)
    }
  }

  const setupRequired = status?.status === "setup_required"
  const disabled = status?.status === "disabled"

  return (
    <main className="min-h-svh bg-background text-foreground">
      <div className="flex h-12 items-center justify-between border-b px-4">
        <div className="flex items-center gap-3">
          <ProductLogo />
          <AdminRiskBadge />
        </div>
        <div className="flex items-center gap-2">
          <LanguageSwitcher />
          <ThemeToggle />
        </div>
      </div>
      <div className="mx-auto grid min-h-[calc(100svh-3rem)] max-w-5xl content-center gap-8 px-6 py-10 md:grid-cols-[1fr_420px]">
        <section className="max-w-xl space-y-5">
          <div className="inline-flex size-11 items-center justify-center border border-destructive/40 bg-destructive/10 text-destructive">
            <ShieldAlert className="size-5" />
          </div>
          <div>
            <h1 className="text-3xl font-semibold tracking-normal">
              {t("admin.loginTitle")}
            </h1>
            <p className="mt-3 text-sm leading-6 text-muted-foreground">
              {t("admin.loginDescription")}
            </p>
          </div>
        </section>
        <section className="border bg-card p-5 shadow-sm">
          {setupRequired ? (
            <div className="space-y-4">
              <Alert>
                <ShieldCheck className="size-4" />
                <AlertTitle>{t("admin.setupRequiredTitle")}</AlertTitle>
                <AlertDescription>
                  {t("admin.setupRequiredDescription")}
                </AlertDescription>
              </Alert>
              <Button asChild className="w-full">
                <a href="/admin/setup">{t("admin.openAdminSetup")}</a>
              </Button>
              <Button
                asChild
                className="w-full justify-start px-0"
                variant="link"
              >
                <a href="/">
                  <ArrowLeft className="size-4" />
                  {t("admin.backToWorkspaceConsole")}
                </a>
              </Button>
            </div>
          ) : disabled ? (
            <div className="space-y-4">
              <Alert variant="destructive">
                <AlertTitle>{t("admin.disabledTitle")}</AlertTitle>
                <AlertDescription>{t("admin.disabledDescription")}</AlertDescription>
              </Alert>
              <Button
                asChild
                className="w-full justify-start px-0"
                variant="link"
              >
                <a href="/">
                  <ArrowLeft className="size-4" />
                  {t("admin.backToWorkspaceConsole")}
                </a>
              </Button>
            </div>
          ) : (
            <form className="space-y-4" onSubmit={submit}>
            <div className="space-y-2">
              <Label htmlFor="admin-token">{t("admin.tokenLabel")}</Label>
              <Input
                autoComplete="off"
                id="admin-token"
                onChange={(event) => setTokenValue(event.target.value)}
                placeholder="xuanchu_admin_..."
                type="password"
                value={token}
              />
            </div>
            {error ? (
              <Alert variant="destructive">
                <AlertTitle>{t("common.error")}</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <Button
              className="w-full"
              disabled={submitting || token.trim() === ""}
              type="submit"
            >
              {t("admin.authenticate")}
            </Button>
            <Button
              asChild
              className="w-full justify-start px-0"
              variant="link"
            >
              <a href="/">
                <ArrowLeft className="size-4" />
                {t("admin.backToWorkspaceConsole")}
              </a>
            </Button>
            </form>
          )}
        </section>
      </div>
    </main>
  )
}
