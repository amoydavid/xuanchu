import { useTranslation } from "react-i18next"
import { useParams } from "@tanstack/react-router"

import { AuthFrame } from "@/components/auth/AuthFrame"
import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ProductLogo } from "@/components/ProductLogo"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

export function SsoLoginPage() {
  const { t } = useTranslation()
  const { slug } = useParams({ strict: false })

  // SSO 回调错误（?sso_error=）展示
  const ssoError =
    typeof window !== "undefined"
      ? new URLSearchParams(window.location.search).get("sso_error")
      : null

  const workspaceSlug = (slug as string) || ""

  return (
    <main className="min-h-svh bg-background text-foreground">
      <div className="flex h-12 items-center justify-between border-b px-4">
        <ProductLogo />
        <div className="flex items-center gap-2">
          <LanguageSwitcher />
          <ThemeToggle />
        </div>
      </div>
      <AuthFrame
        title={t("sso.directLogin")}
        description={
          workspaceSlug
            ? t("sso.directLoginDesc", { slug: workspaceSlug })
            : undefined
        }
      >
        {ssoError ? (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{ssoError}</AlertDescription>
          </Alert>
        ) : null}

        <Button
          type="button"
          className="w-full"
          disabled={!workspaceSlug}
          onClick={() => {
            window.location.href = `/sso/oidc/start?workspace=${encodeURIComponent(workspaceSlug)}`
          }}
        >
          {t("sso.directLogin")}
        </Button>
      </AuthFrame>
    </main>
  )
}
