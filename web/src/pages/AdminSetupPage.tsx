import { useState } from "react"
import type { FormEvent } from "react"
import { ArrowLeft, Check, Copy, KeyRound, ShieldCheck } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"

import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { AdminRiskBadge } from "@/features/admin/components/AdminRiskBadge"
import {
  completeAdminSetup,
  type AdminSetupResult,
} from "@/features/admin/setup/admin-setup-api"
import { ApiError } from "@/lib/api"

export function AdminSetupPage() {
  const { t } = useTranslation()
  const [setupCode, setSetupCode] = useState("")
  const [name, setName] = useState("primary")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<AdminSetupResult | null>(null)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    setResult(null)
    try {
      const created = await completeAdminSetup({
        setup_code: setupCode.trim(),
        name: name.trim() || undefined,
      })
      setResult(created)
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.code)
      } else {
        setError("unknown")
      }
    } finally {
      setSubmitting(false)
    }
  }

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
          <div className="inline-flex size-11 items-center justify-center border border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400">
            <ShieldCheck className="size-5" />
          </div>
          <div>
            <h1 className="text-3xl font-semibold tracking-normal">
              {t("admin.setupTitle")}
            </h1>
            <p className="mt-3 text-sm leading-6 text-muted-foreground">
              {t("admin.setupDescription")}
            </p>
          </div>
        </section>

        <section className="border bg-card p-5 shadow-sm">
          {result ? (
            <div className="space-y-4">
              <div className="flex items-center gap-2">
                <Check className="size-4 text-emerald-600" />
                <h2 className="text-sm font-medium">
                  {t("admin.setupResultTitle")}
                </h2>
              </div>
              <div className="space-y-2">
                <div className="border bg-muted p-3 font-mono text-xs break-all">
                  {result.token}
                </div>
                <p className="text-xs text-muted-foreground">
                  {t("admin.setupTokenWarning")}
                </p>
              </div>
              <div className="grid gap-2 text-sm">
                <div className="flex justify-between gap-3">
                  <span className="text-muted-foreground">
                    {t("admin.result.tokenPrefix")}
                  </span>
                  <span className="font-mono text-xs">
                    {result.token_prefix}
                  </span>
                </div>
                <div className="flex justify-between gap-3">
                  <span className="text-muted-foreground">
                    {t("admin.setupTokenName")}
                  </span>
                  <span>{result.token_name}</span>
                </div>
              </div>
              <div className="flex flex-col gap-2 sm:flex-row">
                <Button
                  onClick={() => void navigator.clipboard?.writeText(result.token)}
                  variant="outline"
                >
                  <Copy className="size-4" />
                  {t("admin.copy")}
                </Button>
                <Button asChild>
                  <Link to="/admin/login">{t("admin.goToAdminLogin")}</Link>
                </Button>
              </div>
            </div>
          ) : (
            <form className="space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                <Label htmlFor="admin-setup-code">
                  {t("admin.setupCodeLabel")}
                </Label>
                <Input
                  autoComplete="off"
                  id="admin-setup-code"
                  onChange={(event) => setSetupCode(event.target.value)}
                  placeholder="setup-code"
                  value={setupCode}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="admin-token-name">
                  {t("admin.setupTokenName")}
                </Label>
                <Input
                  id="admin-token-name"
                  onChange={(event) => setName(event.target.value)}
                  value={name}
                />
              </div>
              {error ? (
                <Alert variant="destructive">
                  <AlertTitle>{t("common.error")}</AlertTitle>
                  <AlertDescription>
                    {t(`admin.errors.${error}`, { defaultValue: error })}
                  </AlertDescription>
                </Alert>
              ) : null}
              <Button
                className="w-full"
                disabled={submitting || setupCode.trim() === ""}
                type="submit"
              >
                <KeyRound className="size-4" />
                {t("admin.createAdminToken")}
              </Button>
              <Button
                asChild
                className="w-full justify-start px-0"
                variant="link"
              >
                <Link to="/admin/login">
                  <ArrowLeft className="size-4" />
                  {t("admin.backToAdminLogin")}
                </Link>
              </Button>
            </form>
          )}
        </section>
      </div>
    </main>
  )
}
