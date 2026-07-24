import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useMe } from "@/features/workspace/session/useMe"

import {
  listHooks,
  listNotificationDeliveries,
  listNotificationRules,
  listNotificationSinks,
  listReminderRules,
} from "./outbound-api"
import {
  canWriteHooks,
  canWriteReminderRules,
  canWriteSinks,
  isOutboundReadonly,
  type OutboundPermissionInput,
} from "./outbound-permissions"
import { HookList } from "./hooks/hook-list"
import { NotificationRuleList } from "./rules/notification-rule-list"
import { ReminderRuleList } from "./rules/reminder-rule-list"
import { SinkList } from "./sinks/sink-list"

export type OutboundTab =
  | "overview"
  | "sinks"
  | "hooks"
  | "notification-rules"
  | "reminder-rules"
  | "deliveries"

export function OutboundConsole({
  initialTab = "hooks",
  workspaceSlug,
}: {
  initialTab?: OutboundTab
  workspaceSlug?: string
}) {
  const { t } = useTranslation()
  const me = useMe()
  const credential = me.data

  const permInput: OutboundPermissionInput = {
    role: credential?.effective_role,
    actorType: credential?.actor_type,
    scopes: credential?.token?.scopes ?? null,
  }

  const canHooks = canWriteHooks(permInput)
  const canSinks = canWriteSinks(permInput)
  const canReminders = canWriteReminderRules(permInput)
  const readonly = isOutboundReadonly(permInput)

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-normal">
          {t("outbound.title")}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("outbound.subtitle")}
        </p>
      </div>

      {readonly && !me.isLoading ? (
        <Alert>
          <AlertDescription>{t("outbound.readonlyHint")}</AlertDescription>
        </Alert>
      ) : null}

      <Tabs defaultValue={initialTab}>
        <TabsList>
          <TabsTrigger value="overview">
            {t("outbound.tab.overview")}
          </TabsTrigger>
          <TabsTrigger value="sinks">{t("outbound.tab.sinks")}</TabsTrigger>
          <TabsTrigger value="hooks">{t("outbound.tab.hooks")}</TabsTrigger>
          <TabsTrigger value="notification-rules">
            {t("outbound.tab.notificationRules")}
          </TabsTrigger>
          <TabsTrigger value="reminder-rules">
            {t("outbound.tab.reminderRules")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <OutboundOverview />
        </TabsContent>
        <TabsContent value="sinks">
          <SinkList canWrite={canSinks} />
        </TabsContent>
        <TabsContent value="hooks">
          <HookList canWrite={canHooks} workspaceSlug={workspaceSlug} />
        </TabsContent>
        <TabsContent value="notification-rules">
          <NotificationRuleList canWrite={canSinks} />
        </TabsContent>
        <TabsContent value="reminder-rules">
          <ReminderRuleList canWrite={canReminders} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// OutboundOverview 是第一版概览：并行查询各资源 list 计数。
// 第一阶段不新增后端 summary endpoint。
function OutboundOverview() {
  const { t } = useTranslation()

  const sinks = useQuery({
    queryKey: ["outbound", "sinks", "all"],
    queryFn: () => listNotificationSinks({ includeDisabled: true }),
  })
  const hooks = useQuery({
    queryKey: ["outbound", "hooks", "all"],
    queryFn: listHooks,
  })
  const notificationRules = useQuery({
    queryKey: ["outbound", "notification-rules"],
    queryFn: listNotificationRules,
  })
  const reminderRules = useQuery({
    queryKey: ["outbound", "reminder-rules"],
    queryFn: listReminderRules,
  })
  const failedDeliveries = useQuery({
    queryKey: ["outbound", "deliveries", "dead_lettered"],
    queryFn: () =>
      listNotificationDeliveries({ status: "dead_lettered", limit: 5 }),
  })

  const sinkEnabled = (sinks.data ?? []).filter((s) => s.enabled).length
  const sinkDisabled = (sinks.data ?? []).length - sinkEnabled
  const hookEnabled = (hooks.data ?? []).filter((h) => h.enabled).length
  const hookDisabled = (hooks.data ?? []).length - hookEnabled
  const failed = failedDeliveries.data ?? []

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <StatCard
          label={t("outbound.stat.sinks")}
          value={`${sinkEnabled} / ${sinkDisabled}`}
        />
        <StatCard
          label={t("outbound.stat.hooks")}
          value={`${hookEnabled} / ${hookDisabled}`}
        />
        <StatCard
          label={t("outbound.stat.notificationRules")}
          value={`${(notificationRules.data ?? []).length}`}
        />
        <StatCard
          label={t("outbound.stat.reminderRules")}
          value={`${(reminderRules.data ?? []).length}`}
        />
        <StatCard
          label={t("outbound.stat.recentDeadLetter")}
          value={`${failed.length}`}
        />
      </div>

      <div className="rounded-lg border bg-card p-4">
        <div className="text-sm font-medium">
          {t("outbound.recentDeadLetter")}
        </div>
        {failed.length === 0 ? (
          <div className="mt-2 text-xs text-muted-foreground">
            {t("common.empty")}
          </div>
        ) : (
          <ul className="mt-2 space-y-1 text-xs">
            {failed.map((d) => (
              <li key={d.id} className="flex flex-wrap items-center gap-2">
                <span className="text-muted-foreground">
                  {new Date((d.created_at ?? 0) * 1000).toLocaleString()}
                </span>
                <span>{d.event_type}</span>
                <span className="text-muted-foreground">{d.sink_id}</span>
                <span className="text-destructive">
                  {d.last_status_code ?? "-"}
                </span>
                {d.last_error ? (
                  <span className="text-muted-foreground">{d.last_error}</span>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border bg-card p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-lg font-semibold">{value}</div>
    </div>
  )
}
