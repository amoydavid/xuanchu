import { useTranslation } from "react-i18next"
import { useParams } from "@tanstack/react-router"

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
      <div className="mx-auto flex min-h-[calc(100svh-3rem)] max-w-md flex-col justify-center px-6">
        <div className="mb-6">
          <ProductLogo
            className="mb-5"
            markClassName="size-14"
            showWordmark={false}
          />
          <h1 className="text-xl font-semibold tracking-normal">
            {t("sso.directLogin")}
          </h1>
          {workspaceSlug ? (
            <p className="mt-2 text-sm text-muted-foreground">
              {t("sso.directLoginDesc", { slug: workspaceSlug })}
            </p>
          ) : null}
        </div>

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
      </div>
    </main>
  )
}
