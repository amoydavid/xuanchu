import { useEffect, useState } from "react"
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

type LoginPageProps = {
  onSignedIn: () => void
  redirectPath?: string
}

interface SsoWorkspaceInfo {
  slug: string
  name: string
}

export function LoginPage({ onSignedIn, redirectPath }: LoginPageProps) {
  const { t } = useTranslation()
  const [token, setTokenValue] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [ssoWorkspace, setSsoWorkspace] = useState<SsoWorkspaceInfo | null>(null)

  // SSO 回调错误（?sso_error=）展示
  const ssoError =
    typeof window !== "undefined"
      ? new URLSearchParams(window.location.search).get("sso_error")
      : null

  // 探测唯一一个启用了 OIDC 的 workspace
  useEffect(() => {
    fetch("/api/v1/sso/workspace")
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (data?.data?.slug) {
          setSsoWorkspace({ slug: data.data.slug, name: data.data.name || data.data.slug })
        }
      })
      .catch(() => {
        // 404 或网络错误 → 不显示 OIDC 入口
      })
  }, [])

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
            <label htmlFor="token" className="text-sm font-medium">
              {t("auth.tokenLabel")}
            </label>
            <input
              autoComplete="off"
              id="token"
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
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

        {/* OIDC 一键登录（仅有唯一 OIDC workspace 时显示） */}
        {ssoWorkspace ? (
          <div className="border-t pt-4">
            {ssoError ? (
              <Alert variant="destructive" className="mb-3">
                <AlertDescription>{ssoError}</AlertDescription>
              </Alert>
            ) : null}
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={() => {
                window.location.href = `/sso/oidc/start?workspace=${encodeURIComponent(ssoWorkspace.slug)}`
              }}
            >
              {t("sso.quickLogin", { name: ssoWorkspace.name })}
            </Button>
          </div>
        ) : null}
      </div>
    </main>
  )
}
