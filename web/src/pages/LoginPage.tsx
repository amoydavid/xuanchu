import { useState } from "react"
import type { FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"

import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type LoginPageProps = {
  onSignedIn: () => void
  redirectPath?: string
}

export function LoginPage({ onSignedIn, redirectPath }: LoginPageProps) {
  const { t } = useTranslation()
  const [token, setTokenValue] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [ssoWorkspace, setSsoWorkspace] = useState("")

  // SSO 回调错误（?sso_error=）展示
  const ssoError =
    typeof window !== "undefined"
      ? new URLSearchParams(window.location.search).get("sso_error")
      : null

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      setWorkspaceToken(token.trim())
      await workspaceApiGet("/api/v1/credentials/current")
      onSignedIn()
    } catch {
      setError(t("auth.failed"))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="min-h-svh bg-background text-foreground">
      <div className="flex h-12 items-center justify-between border-b px-4">
        <ProductLogo />
        <div className="flex items-center gap-2">
          <LanguageSwitcher />
          <ThemeToggle />
        </div>
      </div>
      <div className="mx-auto flex min-h-[calc(100svh-3rem)] max-w-md flex-col justify-center px-6">
        <div className="mb-6">
          <ProductLogo
            className="mb-5"
            markClassName="size-14"
            showWordmark={false}
          />
          <h1 className="text-xl font-semibold tracking-normal">
            {t("auth.signInTitle")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("auth.sessionOnly")}
          </p>
          {redirectPath ? (
            <div className="mt-4 border bg-card p-3 text-xs">
              <div className="text-muted-foreground">{t("auth.continueTo")}</div>
              <code className="mt-1 block break-all text-foreground">
                {redirectPath}
              </code>
            </div>
          ) : null}
        </div>
        <form className="space-y-4" onSubmit={submit}>
          <div className="space-y-2">
            <Label htmlFor="token">{t("auth.tokenLabel")}</Label>
            <Input
              autoComplete="off"
              id="token"
              onChange={(event) => setTokenValue(event.target.value)}
              placeholder={t("auth.tokenPlaceholder")}
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
          <Button disabled={submitting || token.trim() === ""} type="submit">
            {t("auth.signIn")}
          </Button>
          <div>
            <Link
              className="text-xs text-muted-foreground transition-colors hover:text-foreground"
              to="/admin/login"
            >
              {t("auth.adminLoginLink")}
            </Link>
          </div>
        </form>

        {/* OIDC 单点登录 */}
        <div className="border-t pt-4">
          {ssoError ? (
            <Alert variant="destructive" className="mb-3">
              <AlertDescription>{ssoError}</AlertDescription>
            </Alert>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="sso-workspace">{t("sso.title")}</Label>
            <Input
              id="sso-workspace"
              placeholder="workspace-slug"
              value={ssoWorkspace}
              onChange={(e) => setSsoWorkspace(e.target.value)}
            />
            <Button
              type="button"
              variant="outline"
              className="w-full"
              disabled={ssoWorkspace.trim() === ""}
              onClick={() => {
                window.location.href = `/sso/oidc/start?workspace=${encodeURIComponent(ssoWorkspace.trim())}`
              }}
            >
              {t("sso.oidcLogin")}
            </Button>
          </div>
        </div>
      </div>
    </main>
  )
}
