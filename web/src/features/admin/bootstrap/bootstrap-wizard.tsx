import { useMemo, useState } from "react"
import { Check, Copy, KeyRound, ShieldAlert } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { adminApiPost } from "@/features/admin/session/admin-api"
import { ApiError } from "@/lib/api"

import { AgentTokenForm, type AgentTokenFormValue } from "./agent-token-form"
import {
  WorkspaceAdminForm,
  type WorkspaceAdminFormValue,
} from "./workspace-admin-form"
import { WorkspaceForm, type WorkspaceFormValue } from "./workspace-form"

type CreatedTokenResponse = {
  token?: string
  prefix?: string
}

type BootstrapResult = {
  adminLabel: string
  token?: string
  tokenPrefix?: string
  workspaceSlug: string
}

export function BootstrapWizard() {
  const { t } = useTranslation()
  const [workspace, setWorkspace] = useState<WorkspaceFormValue>({
    description: "",
    name: "",
    slug: "",
    visibility: "team",
  })
  const [admin, setAdmin] = useState<WorkspaceAdminFormValue>({
    email: "",
    name: "",
    role: "owner",
  })
  const [token, setToken] = useState<AgentTokenFormValue>({
    expiresIn: "720h",
    name: "",
    scopes: "*",
  })
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<BootstrapResult | null>(null)

  const tokenName = useMemo(() => {
    if (token.name.trim()) {
      return token.name.trim()
    }
    const base = admin.email.trim() || admin.name.trim() || "workspace"
    return `${base.replace(/[^a-zA-Z0-9_-]+/g, "-")}-admin-agent`
  }, [admin.email, admin.name, token.name])

  const canSubmit = workspace.slug.trim() !== "" && admin.name.trim() !== ""

  async function submit() {
    if (!canSubmit) {
      return
    }
    setSubmitting(true)
    setError(null)
    setResult(null)
    const slug = workspace.slug.trim()
    try {
      await createWorkspaceAndOwner(slug)
      const createdToken = await createAgentToken(slug, tokenName)
      setResult({
        adminLabel: admin.email.trim() || admin.name.trim(),
        token: createdToken.token,
        tokenPrefix: createdToken.prefix,
        workspaceSlug: slug,
      })
    } catch (err) {
      setError(errorCode(err))
    } finally {
      setSubmitting(false)
    }
  }

  async function createWorkspaceAndOwner(slug: string) {
    try {
      await adminApiPost("/api/v1/admin/workspaces", {
        slug,
        name: workspace.name.trim(),
        description: workspace.description.trim(),
        visibility: workspace.visibility,
        owner: {
          name: admin.name.trim(),
          email: admin.email.trim(),
        },
      })
    } catch (err) {
      if (err instanceof ApiError && err.code === "admin_workspace_exists") {
        await adminApiPost(
          `/api/v1/admin/workspaces/${encodeURIComponent(slug)}/admins`,
          {
            name: admin.name.trim(),
            email: admin.email.trim(),
            role: admin.role,
          }
        )
        return
      }
      throw err
    }
  }

  async function createAgentToken(slug: string, name: string) {
    return adminApiPost<CreatedTokenResponse>(
      `/api/v1/admin/workspaces/${encodeURIComponent(slug)}/agent-tokens`,
      {
        name,
        user: admin.email.trim() || admin.name.trim(),
        scopes: parseScopes(token.scopes),
        expires_in: token.expiresIn.trim(),
      }
    )
  }

  function done() {
    setResult(result ? { ...result, token: undefined } : null)
  }

  return (
    <div className="space-y-5">
      <section className="rounded-lg flex flex-col gap-3 border bg-card p-4 md:flex-row md:items-center md:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <ShieldAlert className="size-4 text-destructive" />
            <h1 className="text-xl font-semibold tracking-normal">
              {t("admin.bootstrapTitle")}
            </h1>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("admin.bootstrapDescription")}
          </p>
        </div>
        <Badge variant="outline">{t("admin.highRisk")}</Badge>
      </section>

      <section className="rounded-lg space-y-5 border bg-card p-4">
        <Step index={1} title={t("admin.workspaceStep")} />
        <WorkspaceForm onChange={setWorkspace} value={workspace} />
        <Step index={2} title={t("admin.adminStep")} />
        <WorkspaceAdminForm onChange={setAdmin} value={admin} />
        <Step index={3} title={t("admin.tokenStep")} />
        <AgentTokenForm
          onChange={setToken}
          value={{ ...token, name: token.name || tokenName }}
        />

        {error ? (
          <Alert variant="destructive">
            <AlertTitle>{t("common.error")}</AlertTitle>
            <AlertDescription>
              {t(`admin.errors.${error}`, { defaultValue: error })}
            </AlertDescription>
          </Alert>
        ) : null}

        <Button disabled={!canSubmit || submitting} onClick={submit}>
          <KeyRound className="size-4" />
          {t("admin.createBootstrap")}
        </Button>
      </section>

      {result ? (
        <section className="rounded-lg space-y-4 border bg-card p-4">
          <div className="flex items-center gap-2">
            <Check className="size-4 text-primary" />
            <h2 className="text-sm font-medium">{t("admin.resultTitle")}</h2>
          </div>
          <dl className="grid gap-3 text-sm md:grid-cols-3">
            <ResultItem
              label={t("admin.result.workspace")}
              value={result.workspaceSlug}
            />
            <ResultItem
              label={t("admin.result.admin")}
              value={result.adminLabel}
            />
            <ResultItem
              label={t("admin.result.tokenPrefix")}
              value={result.tokenPrefix || "-"}
            />
          </dl>
          {result.token ? (
            <div className="space-y-2">
              <div className="rounded-lg border bg-muted p-3 font-mono text-xs break-all">
                {result.token}
              </div>
              <p className="text-xs text-muted-foreground">
                {t("admin.shownOnce")}
              </p>
              <div className="flex gap-2">
                <Button
                  onClick={() =>
                    void navigator.clipboard?.writeText(result.token ?? "")
                  }
                  variant="outline"
                >
                  <Copy className="size-4" />
                  {t("admin.copy")}
                </Button>
                <Button onClick={done} variant="secondary">
                  {t("admin.done")}
                </Button>
              </div>
            </div>
          ) : null}
        </section>
      ) : null}
    </div>
  )
}

function Step({ index, title }: { index: number; title: string }) {
  return (
    <div className="flex items-center gap-2 border-t pt-4 first:border-t-0 first:pt-0">
      <span className="rounded-lg flex size-6 items-center justify-center border bg-muted text-xs font-medium">
        {index}
      </span>
      <h2 className="text-sm font-medium">{title}</h2>
    </div>
  )
}

function ResultItem({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 font-medium">{value}</dd>
    </div>
  )
}

function parseScopes(value: string): string[] {
  const trimmed = value.trim()
  if (trimmed === "" || trimmed === "*") {
    return ["*"]
  }
  return trimmed
    .split(",")
    .map((scope) => scope.trim())
    .filter(Boolean)
}

function errorCode(err: unknown): string {
  if (err instanceof ApiError) {
    return err.code
  }
  return "unknown"
}
