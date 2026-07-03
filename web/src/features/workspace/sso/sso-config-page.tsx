import { useTranslation } from "react-i18next"
import { useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useMe } from "@/features/workspace/session/useMe"
import { PageHeader } from "@/pages/OverviewPage"
import { ApiError } from "@/lib/api"

import {
  useSaveSsoConfigMutation,
  useSsoConfigQuery,
  useTriggerSyncMutation,
  type SsoConfigInput,
} from "./use-sso"

export function SsoConfigPage() {
  const { t } = useTranslation()
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug ?? ""
  const role = me.data?.effective_role ?? ""
  const isOwner = role === "owner"
  const isTenantActor = me.data?.actor_type === "tenant_access_token"
  const canManage = isOwner || isTenantActor

  const configQuery = useSsoConfigQuery(workspaceSlug)
  const saveMutation = useSaveSsoConfigMutation(workspaceSlug)
  const syncMutation = useTriggerSyncMutation(workspaceSlug)

  const enabled = configQuery.data?.enabled ?? false
  const cfg = configQuery.data?.config

  const [form, setForm] = useState<SsoConfigInput>({
    issuer_base_url: "",
    org_id: "",
    client_id: "",
    client_secret: "",
    sync_interval: "1h",
    external_base_url: "",
    session_ttl: "168h",
  })

  // 配置加载成功后预填表单（secret 留空，保存时空值=不修改）
  useEffect(() => {
    if (configQuery.isSuccess && cfg) {
      setForm({
        issuer_base_url: cfg.issuer_base_url,
        org_id: cfg.org_id,
        client_id: cfg.client_id,
        client_secret: "",
        sync_interval: cfg.sync_interval || "1h",
        external_base_url: cfg.external_base_url,
        session_ttl: cfg.session_ttl || "168h",
      })
    }
  }, [configQuery.data])

  if (me.isLoading || configQuery.isLoading) {
    return <Skeleton className="h-96 w-full" />
  }
  if (!canManage) {
    return <p className="text-sm text-muted-foreground">{t("sso.noPermission")}</p>
  }

  const update = (key: keyof SsoConfigInput, value: string) =>
    setForm((prev) => ({ ...prev, [key]: value }))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    saveMutation.mutate(form)
  }

  return (
    <form className="space-y-8" onSubmit={handleSubmit}>
      <div className="flex items-center justify-between">
        <PageHeader title={t("sso.title")} description={t("sso.description")} />
      </div>

      {!enabled && (
        <p className="text-sm text-muted-foreground">{t("sso.notEnabled")}</p>
      )}

      {/* IdP 连接 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionIdp")}</h3>
        <Field label={t("sso.field.issuerBaseUrl")} hint={t("sso.hint.issuerBaseUrl")}>
          <Input
            value={form.issuer_base_url}
            onChange={(e) => update("issuer_base_url", e.target.value)}
            required
          />
        </Field>
        <Field label={t("sso.field.orgId")} hint={t("sso.hint.orgId")}>
          <Input
            value={form.org_id}
            onChange={(e) => update("org_id", e.target.value)}
            required
          />
        </Field>
        <Field label={t("sso.field.clientId")} hint={t("sso.hint.clientId")}>
          <Input
            value={form.client_id}
            onChange={(e) => update("client_id", e.target.value)}
            required
          />
        </Field>
        <Field
          label={t("sso.field.clientSecret")}
          hint={t("sso.hint.clientSecret")}
          badge={
            cfg?.client_secret_masked
              ? `${t("sso.secretSet")} ${cfg.client_secret_masked}`
              : t("sso.secretUnset")
          }
        >
          <Input
            type="password"
            value={form.client_secret}
            onChange={(e) => update("client_secret", e.target.value)}
            placeholder="••••••••"
          />
        </Field>
      </section>

      {/* 通讯录同步 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionDirectory")}</h3>
        <Field label={t("sso.field.syncInterval")} hint={t("sso.hint.syncInterval")}>
          <Select
            value={form.sync_interval}
            onValueChange={(v) => update("sync_interval", v)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="0">{t("sso.syncIntervalDisabled")}</SelectItem>
              <SelectItem value="30m">30m</SelectItem>
              <SelectItem value="1h">1h</SelectItem>
              <SelectItem value="6h">6h</SelectItem>
              <SelectItem value="24h">24h</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </section>

      {/* 高级 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionAdvanced")}</h3>
        <Field
          label={t("sso.field.externalBaseUrl")}
          hint={t("sso.hint.externalBaseUrl")}
        >
          <Input
            value={form.external_base_url}
            onChange={(e) => update("external_base_url", e.target.value)}
          />
        </Field>
        <Field label={t("sso.field.sessionTtl")} hint={t("sso.hint.sessionTtl")}>
          <Select
            value={form.session_ttl}
            onValueChange={(v) => update("session_ttl", v)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="24h">1d</SelectItem>
              <SelectItem value="168h">7d</SelectItem>
              <SelectItem value="720h">30d</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </section>

      {/* 错误提示 */}
      {saveMutation.isError && (
        <p className="text-sm text-destructive">
          {t("sso.errors.save")}: {errorMessage(saveMutation.error)}
        </p>
      )}
      {syncMutation.isError && (
        <p className="text-sm text-destructive">
          {t("sso.errors.sync")}: {errorMessage(syncMutation.error)}
        </p>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <Button type="submit" disabled={saveMutation.isPending}>
          {t("sso.save")}
        </Button>
      </div>

      {/* 成员同步 */}
      <section className="space-y-4 border-t pt-4">
        <h3 className="text-sm font-medium">{t("sso.sectionMembers")}</h3>
        <div className="flex items-center gap-3">
          <Button
            type="button"
            variant="outline"
            onClick={() => syncMutation.mutate()}
            disabled={!enabled || syncMutation.isPending}
          >
            {syncMutation.isPending ? t("sso.syncRunning") : t("sso.syncNow")}
          </Button>
        </div>
      </section>
    </form>
  )
}

function Field({
  label,
  hint,
  badge,
  children,
}: {
  label: string
  hint?: string
  badge?: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <Label className="text-sm">{label}</Label>
        {badge && <span className="text-xs text-muted-foreground">{badge}</span>}
      </div>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return String(err)
}
