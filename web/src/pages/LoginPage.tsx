import { useState } from "react"
import type { FormEvent } from "react"
import { useTranslation } from "react-i18next"

import { LanguageSwitcher } from "@/components/LanguageSwitcher"
import { ThemeToggle } from "@/components/ThemeToggle"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { apiGet } from "@/lib/api"
import { setToken } from "@/lib/token"

type LoginPageProps = {
  onSignedIn: () => void
}

export function LoginPage({ onSignedIn }: LoginPageProps) {
  const { t } = useTranslation()
  const [token, setTokenValue] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      setToken(token.trim())
      await apiGet("/api/v1/me")
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
        <div className="text-sm font-medium">{t("app.title")}</div>
        <div className="flex items-center gap-2">
          <LanguageSwitcher />
          <ThemeToggle />
        </div>
      </div>
      <div className="mx-auto flex min-h-[calc(100svh-3rem)] max-w-md flex-col justify-center px-6">
        <div className="mb-6">
          <h1 className="text-xl font-semibold tracking-normal">
            {t("auth.signInTitle")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("auth.sessionOnly")}
          </p>
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
        </form>
      </div>
    </main>
  )
}
