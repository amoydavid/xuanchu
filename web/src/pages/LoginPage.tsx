import { useEffect, useState } from "react"
import type { FormEvent } from "react"
import { useTranslation } from "react-i18next"

import { useBrandName } from "@/brand/BrandContext"
import { AuthFrame } from "@/components/auth/AuthFrame"
import { OrSeparator } from "@/components/auth/OrSeparator"
import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { adminApiGet } from "@/features/admin/session/admin-api"
import { setAdminToken } from "@/features/admin/session/admin-token"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { ApiError } from "@/lib/api"

type LoginPageProps = {
  onSignedIn: () => void
  redirectPath?: string
}

interface SsoWorkspaceInfo {
  slug: string
  name: string
}

// 超管 token 前缀。前端只按前缀做 UX 分流，后端会再次校验 token 类型。
const ADMIN_TOKEN_PREFIX = "xuanchu_admin_"

function isAdminCredential(value: string): boolean {
  return value.startsWith(ADMIN_TOKEN_PREFIX)
}

export function LoginPage({ onSignedIn, redirectPath }: LoginPageProps) {
  const { t } = useTranslation()
  const brandName = useBrandName()
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

  // 按凭证前缀分流：超管走 admin 存储 + admin 校验 + 整页跳转 /admin；
  // 其余（PAT / tenant）走 workspace 存储 + credentials/current 校验。
  // 整页跳转是因为 admin token 不在 WorkspaceRootRoute 的登录态判定里，
  // 必须让 AdminGuardRoute 重新初始化才能识别。
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const trimmed = token.trim()
    setSubmitting(true)
    setError(null)
    try {
      if (isAdminCredential(trimmed)) {
        setAdminToken(trimmed)
        await adminApiGet("/api/v1/admin/session")
        // 整页跳转：admin token 不在 WorkspaceRootRoute 的登录态判定里，
        // 必须让 AdminGuardRoute 重新初始化才能识别。
        window.location.href = "/admin"
        return
      }
      setWorkspaceToken(trimmed)
      await workspaceApiGet("/api/v1/credentials/current")
      onSignedIn()
    } catch (err) {
      // admin 分支保留 setup_required 语义：该错误码映射到独立 /admin/login 页，
      // 由它兜底 setup 视图切换。
      if (err instanceof ApiError && err.code === "admin_setup_required") {
        window.location.href = "/admin/login"
        return
      }
      // 按错误码映射 i18n 文案；未命中回退通用失败文案。
      if (err instanceof ApiError && err.code) {
        const codeKey = `auth.errors.${err.code}`
        const translated = t(codeKey)
        setError(translated === codeKey ? t("auth.failed") : translated)
      } else {
        setError(t("auth.failed"))
      }
    } finally {
      setSubmitting(false)
    }
  }

  const showSso = !!ssoWorkspace
  // 有 SSO 时凭证登录折叠为二级入口；无 SSO 时直接展开，避免落地空页。
  const tokenExpanded = !showSso

  return (
    <main className="min-h-svh bg-background text-foreground">
      <div className="flex h-12 items-center justify-between border-b px-4">
        <ProductLogo />
        <div className="flex items-center gap-2">
          <LanguageSwitcher />
          <ThemeToggle />
        </div>
      </div>
      <AuthFrame title={t("auth.signInTitle", { brand: brandName })} description={t("auth.sessionOnly")}>
        {redirectPath ? (
          <div className="mb-4 border bg-card p-3 text-xs">
            <div className="text-muted-foreground">{t("auth.continueTo")}</div>
            <code className="mt-1 block break-all text-foreground">
              {redirectPath}
            </code>
          </div>
        ) : null}

        {showSso ? (
          <div className="space-y-1">
            {ssoError ? (
              <Alert variant="destructive" className="mb-3">
                <AlertDescription>{ssoError}</AlertDescription>
              </Alert>
            ) : null}
            <Button
              type="button"
              className="w-full"
              onClick={() => {
                if (ssoWorkspace) {
                  window.location.href = `/sso/oidc/start?workspace=${encodeURIComponent(ssoWorkspace.slug)}`
                }
              }}
            >
              {t("sso.quickLogin", { name: ssoWorkspace?.name })}
            </Button>
            <OrSeparator label={t("auth.or")} />
          </div>
        ) : null}

        {tokenExpanded ? (
          <form className="space-y-4" onSubmit={submit}>
            <div className="space-y-2">
              <Label htmlFor="token">{t("auth.tokenLabel")}</Label>
              <Input
                autoComplete="off"
                id="token"
                onChange={(event) => setTokenValue(event.target.value)}
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
              {t("auth.signIn")}
            </Button>
          </form>
        ) : (
          <details>
            <summary className="cursor-pointer text-sm font-medium text-muted-foreground">
              {t("auth.tokenLoginToggle")}
            </summary>
            <form className="mt-3 space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                <Label htmlFor="token">{t("auth.tokenLabel")}</Label>
                <Input
                  autoComplete="off"
                  id="token"
                  onChange={(event) => setTokenValue(event.target.value)}
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
                {t("auth.signIn")}
              </Button>
            </form>
          </details>
        )}
      </AuthFrame>
    </main>
  )
}
