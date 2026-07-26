import { useState } from "react"
import { useTranslation } from "react-i18next"

import {
  useReplayWorkspaceAutomationDelivery,
  useWorkspaceAutomationDeliveries,
  useWorkspaceAutomationProviderConfig,
  useWorkspaceAutomationRules,
  useToggleWorkspaceAutomationRule,
} from "@/features/workspace/automations/workspace-automations-api"
import { cn } from "@/lib/utils"
import { relativeTime } from "@/lib/time"

type Tab = "rules" | "deliveries"

// WorkspaceAutomationsConsole 是「管理 → 自动化」入口的控制台。
// 顶层只有两个 tab：规则（Workspace scope）和运行记录（跨 Project）。
// 详细规则编辑、Provider 配置、sample Project、preview 等复杂表单在后续迭代中
// 通过 Dialog 承载；当前实现先把列表/启停/删除/replay 等高频闭环跑通。
export function WorkspaceAutomationsConsole(props?: {
  workspaceSlug?: string
}) {
  // workspaceSlug 当前未直接使用，但保留 prop 以便未来根据 slug 路由到子页面。
  void props?.workspaceSlug
  const { t } = useTranslation()
  const [tab, setTab] = useState<Tab>("rules")

  return (
    <div className="space-y-4">
      <header className="space-y-1">
        <h1 className="text-xl font-semibold">{t("page.workspaceAutomations")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("automation.workspace.intro")}
        </p>
      </header>
      <ProviderMissingWarning />
      <div className="flex items-center gap-2 border-b">
        <TabButton active={tab === "rules"} onClick={() => setTab("rules")}>
          {t("automation.tab.rules")}
        </TabButton>
        <TabButton active={tab === "deliveries"} onClick={() => setTab("deliveries")}>
          {t("automation.tab.deliveries")}
        </TabButton>
      </div>
      {tab === "rules" ? <RulesTab /> : <DeliveriesTab />}
    </div>
  )
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "-mb-px border-b-2 px-3 py-2 text-sm transition-colors",
        active
          ? "border-primary text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground"
      )}
    >
      {children}
    </button>
  )
}

function ProviderMissingWarning() {
  const { t } = useTranslation()
  const provider = useWorkspaceAutomationProviderConfig()
  if (!provider.data || provider.data.complete) return null
  return (
    <div
      role="status"
      className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/40 dark:text-amber-100"
    >
      {t("automation.provider.incomplete")}
    </div>
  )
}

function RulesTab() {
  const { t } = useTranslation()
  const rules = useWorkspaceAutomationRules(false)
  const toggle = useToggleWorkspaceAutomationRule()

  if (rules.isLoading) {
    return <div className="text-sm text-muted-foreground">{t("common.loading")}</div>
  }
  if (rules.isError) {
    return (
      <div className="text-sm text-destructive">
        {t("common.error")}: {String(rules.error)}
      </div>
    )
  }
  if (!rules.data || rules.data.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
        {t("automation.empty.rules")}
      </div>
    )
  }
  return (
    <ul className="divide-y rounded-md border">
      {rules.data.map((rule) => (
        <li key={rule.id} className="flex items-center gap-3 px-3 py-2.5">
          <StatusDot enabled={rule.enabled} />
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{rule.name}</div>
            <div className="truncate text-xs text-muted-foreground">
              {summarizeTrigger(rule.trigger_type, rule.trigger_config)}
            </div>
          </div>
          <div className="text-xs text-muted-foreground">
            {rule.last_delivery
              ? relativeTime(rule.last_delivery.created_at)
              : t("automation.lastRun.never")}
          </div>
          <button
            type="button"
            className="rounded border px-2 py-1 text-xs hover:bg-accent"
            onClick={() => {
              toggle.mutate({ ruleId: rule.id, enable: !rule.enabled })
            }}
          >
            {rule.enabled ? t("automation.action.disable") : t("automation.action.enable")}
          </button>
        </li>
      ))}
    </ul>
  )
}

function DeliveriesTab() {
  const { t } = useTranslation()
  const deliveries = useWorkspaceAutomationDeliveries({ limit: 50 })
  const replay = useReplayWorkspaceAutomationDelivery()

  if (deliveries.isLoading) {
    return <div className="text-sm text-muted-foreground">{t("common.loading")}</div>
  }
  if (deliveries.isError) {
    return (
      <div className="text-sm text-destructive">
        {t("common.error")}: {String(deliveries.error)}
      </div>
    )
  }
  if (!deliveries.data || deliveries.data.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
        {t("automation.empty.deliveries")}
      </div>
    )
  }
  return (
    <ul className="divide-y rounded-md border">
      {deliveries.data.map((delivery) => (
        <li key={delivery.id} className="flex items-center gap-3 px-3 py-2.5">
          <DeliveryStatusDot status={delivery.status} />
          <div className="min-w-0 flex-1">
            <div className="truncate font-mono text-xs">{delivery.id}</div>
            <div className="truncate text-xs text-muted-foreground">
              {delivery.trigger_type === "event"
                ? delivery.event_type || delivery.trigger_type
                : summarizeTrigger(delivery.trigger_type, {})}
              {delivery.project ? ` · ${delivery.project.slug}` : ""}
            </div>
          </div>
          <div className="font-mono text-xs text-muted-foreground">
            {delivery.response_status_code ?? "—"}
          </div>
          <div className="text-xs text-muted-foreground">
            {relativeTime(delivery.created_at)}
          </div>
          <button
            type="button"
            className="rounded border px-2 py-1 text-xs hover:bg-accent"
            onClick={() => replay.mutate(delivery.id)}
          >
            {t("automation.action.replay")}
          </button>
        </li>
      ))}
    </ul>
  )
}

function StatusDot({ enabled }: { enabled: boolean }) {
  return (
    <span
      aria-label={enabled ? "enabled" : "disabled"}
      className={cn(
        "inline-block size-[7px] shrink-0 rounded-full",
        enabled ? "bg-emerald-500" : "bg-muted-foreground/40"
      )}
    />
  )
}

function DeliveryStatusDot({ status }: { status: string }) {
  const color =
    status === "succeeded"
      ? "bg-emerald-500"
      : status === "dead_lettered"
        ? "bg-red-500"
        : status === "retry_wait"
          ? "bg-amber-500"
          : "bg-muted-foreground/40"
  return (
    <span
      aria-label={status}
      className={cn("inline-block size-[7px] shrink-0 rounded-full", color)}
    />
  )
}

// summarizeTrigger 把 trigger_config 渲染成单行摘要（不暴露完整 prompt）。
function summarizeTrigger(
  triggerType: string,
  config: { schedule_type?: string; schedule_value?: string; event_type?: string }
): string {
  if (triggerType === "event") {
    return config.event_type ?? "event"
  }
  if (triggerType === "schedule") {
    if (config.schedule_type === "cron") {
      return `cron ${config.schedule_value ?? ""}`
    }
    return `daily_at ${config.schedule_value ?? ""}`
  }
  return triggerType
}
