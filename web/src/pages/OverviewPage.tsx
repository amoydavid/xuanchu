import type React from "react"
import { useTranslation } from "react-i18next"

import { DataTable } from "@/components/DataTable"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

const deliveryRows = [
  { id: "hd_local", kind: "hook", sink: "audit-stream", status: "dead_lettered" },
  { id: "nd_local", kind: "notification", sink: "default", status: "retry_wait" },
]

const auditRows = [
  { time: "10:22", actor: "local", action: "token.create", target: "token" },
  { time: "10:20", actor: "agent", action: "task.done", target: "task" },
]

type OverviewPageProps = {
  me?: {
    actor: { name: string }
    token: { type: string; scopes: string[] }
    effective_workspace: { slug: string }
  }
}

export function OverviewPage({ me }: OverviewPageProps) {
  const { t } = useTranslation()

  return (
    <div className="space-y-5">
      <PageHeader title={t("page.overview")} description={t("app.description")} />
      <section className="grid gap-3 md:grid-cols-3">
        {[
          [t("overview.currentActor"), me?.actor.name ?? "-"],
          [t("overview.tokenType"), me?.token.type ?? "-"],
          [t("overview.scope"), me?.token.scopes.join(", ") || "-"],
        ].map(([label, value]) => (
          <div className="border bg-card p-3" key={label}>
            <div className="text-xs text-muted-foreground">{label}</div>
            <div className="mt-1 text-sm font-medium">{value}</div>
          </div>
        ))}
      </section>
      <section className="grid gap-3 md:grid-cols-5">
        {[
          t("overview.tasks"),
          t("overview.overdue"),
          t("overview.dueSoon"),
          t("overview.projects"),
          t("overview.failedDelivery"),
        ].map((label, index) => (
          <div className="border bg-card p-3" key={label}>
            <div className="text-xs text-muted-foreground">{label}</div>
            <div className="mt-1 text-lg font-semibold">{index}</div>
          </div>
        ))}
      </section>
      <section className="space-y-2">
        <SectionTitle
          action={<Button variant="outline">{t("common.refresh")}</Button>}
          title={t("overview.recentFailedDeliveries")}
        />
        <DataTable
          columns={[
            { key: "id", header: t("overview.delivery"), render: (row) => <code>{row.id}</code> },
            { key: "kind", header: t("overview.kind"), render: (row) => row.kind },
            { key: "sink", header: t("overview.sink"), render: (row) => row.sink },
            { key: "status", header: t("common.status"), render: (row) => <Badge variant="outline">{row.status}</Badge> },
          ]}
          empty={t("common.empty")}
          rows={deliveryRows}
        />
      </section>
      <section className="space-y-2">
        <SectionTitle title={t("overview.recentAudit")} />
        <DataTable
          columns={[
            { key: "time", header: t("overview.time"), render: (row) => row.time },
            { key: "actor", header: t("common.actor"), render: (row) => row.actor },
            { key: "action", header: t("overview.action"), render: (row) => row.action },
            { key: "target", header: t("overview.target"), render: (row) => row.target },
          ]}
          empty={t("common.empty")}
          rows={auditRows}
        />
      </section>
    </div>
  )
}

export function PageHeader({
  description,
  title,
}: {
  description?: string
  title: string
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
        {description ? (
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
    </div>
  )
}

function SectionTitle({
  action,
  title,
}: {
  action?: React.ReactNode
  title: string
}) {
  return (
    <div className="flex h-8 items-center justify-between">
      <h2 className="text-sm font-medium">{title}</h2>
      {action}
    </div>
  )
}
