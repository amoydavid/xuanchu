import { Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { listProjectEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import { formatConfigDisplayValue } from "@/features/workspace/config/config-display"
import {
  useProjectTaskSummaryQuery,
  useProjectTimelineQuery,
} from "../hooks/use-project-data"
import { useProjectLayout } from "./project-layout"

type ProjectOverviewPageProps = {
  projectSlug: string
  workspaceSlug: string
}

// ProjectOverviewPage 是项目判断页：最新更新、当前重点、项目附属信息、负载、最近活动。
// 「当前重点」全部来自 ProjectTaskSummary，不用当前任务列表派生全量数字。
export function ProjectOverviewPage({
  projectSlug,
  workspaceSlug,
}: ProjectOverviewPageProps) {
  const { t } = useTranslation()
  const { project, canReadTasks, canManage, closed } = useProjectLayout()
  const summary = useProjectTaskSummaryQuery(
    workspaceSlug,
    projectSlug,
    canReadTasks
  )
  const timeline = useProjectTimelineQuery(workspaceSlug, projectSlug)
  const homeConfig = useQuery({
    queryKey: [
      "project-workbench",
      workspaceSlug,
      projectSlug,
      "config-effective",
      "console-home",
    ],
    queryFn: () => listProjectEffectiveConfig(projectSlug, { consoleHome: true }),
  })

  const latestAnnotation = project.recent_annotations?.[0]
  const timelinePreview = (timeline.data ?? []).slice(0, 5)
  const seriesMetrics = summary.data?.series_metrics
  const hasNormalTasks = project.task_count > 0
  const hasRecurringRuntime =
    (seriesMetrics?.active_recurring_series_count ?? 0) > 0 ||
    (seriesMetrics?.open_recurring_occurrence_count ?? 0) > 0
  const completedRatio = hasNormalTasks
    ? Math.round((project.completed_count / project.task_count) * 100)
    : 0

  return (
    <div className="space-y-4">
      {/* 最新项目更新 */}
      <section className="rounded-lg border bg-card p-4">
        <div className="mb-2 flex items-center justify-between">
          <h2 className="text-sm font-medium">
            {t("projectSubpages.latestUpdateTitle")}
          </h2>
          {canManage && !closed ? (
            <Link
              className="text-sm text-primary hover:underline"
              params={{ projectSlug, workspaceSlug }}
              to="/workspaces/$workspaceSlug/projects/$projectSlug/activity"
            >
              {t("projectSubpages.latestUpdateWriteAction")}
            </Link>
          ) : null}
        </div>
        {latestAnnotation ? (
          <div className="space-y-1">
            <div className="text-sm text-muted-foreground">
              {latestAnnotation.created_by?.name ?? ""}
            </div>
            <p className="text-sm">{latestAnnotation.content}</p>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t("projectSubpages.noProjectUpdate")}
          </p>
        )}
      </section>

      {/* 执行概览只展示已有的普通任务或循环运行数据，避免空项目出现零值摘要。 */}
      {summary.data && (hasNormalTasks || hasRecurringRuntime) ? (
        <section className="rounded-lg border bg-card p-4">
          <h2 className="mb-2 text-sm font-medium">
            {t("projectSubpages.executionOverviewTitle")}
          </h2>
          <div className="space-y-1 text-sm text-muted-foreground">
            {hasNormalTasks ? (
              <p>
                {t("projectSubpages.executionNormalTasks", {
                  completed: project.completed_count,
                  total: project.task_count,
                  percent: completedRatio,
                })}
              </p>
            ) : null}
            {hasRecurringRuntime ? (
              <p>
                {t("projectSubpages.executionRecurringTasks", {
                  active: seriesMetrics?.active_recurring_series_count ?? 0,
                  open: seriesMetrics?.open_recurring_occurrence_count ?? 0,
                })}
              </p>
            ) : null}
          </div>
        </section>
      ) : null}

      {/* 当前重点：全部来自 ProjectTaskSummary。无 task:read 时不展示。 */}
      {canReadTasks ? (
        summary.isError ? (
          <section className="rounded-lg border border-destructive/30 bg-card p-4 text-sm text-destructive">
            {t("projectSubpages.railSummaryError")}
          </section>
        ) : summary.data ? (
          <section className="rounded-lg border bg-card p-4">
            <h2 className="mb-3 text-sm font-medium">
              {t("projectSubpages.focusTitle")}
            </h2>
            <div className="space-y-2">
              <FocusRow
                href={`/workspaces/${workspaceSlug}/projects/${projectSlug}/tasks?status=open`}
                label={t("projectSubpages.riskOverdue")}
                refs={summary.data.overdue_refs}
                total={summary.data.overdue_count}
              />
              <FocusRow
                href={`/workspaces/${workspaceSlug}/projects/${projectSlug}/tasks?priority=H&status=open`}
                label={t("projectSubpages.riskHighPriority")}
                refs={summary.data.high_priority_open_refs}
                total={summary.data.high_priority_open_count}
              />
              <FocusRow
                href={`/workspaces/${workspaceSlug}/projects/${projectSlug}/tasks?status=open`}
                label={t("projectSubpages.riskWaitReady")}
                refs={summary.data.wait_ready_refs}
                total={summary.data.wait_ready_count}
              />
              <FocusRow
                href={`/workspaces/${workspaceSlug}/projects/${projectSlug}/tasks?assignee_empty=true&status=open`}
                label={t("projectSubpages.riskUnassigned")}
                refs={summary.data.unassigned_open_refs}
                total={summary.data.unassigned_open_count}
              />
            </div>
          </section>
        ) : null
      ) : null}

      {/* 项目附属信息 */}
      {homeConfig.isError ? null : (
        <section className="rounded-lg border bg-card p-4">
          <div className="mb-2 flex items-center justify-between">
            <h2 className="text-sm font-medium">
              {t("projectSubpages.factsTitle")}
            </h2>
            <Link
              className="text-sm text-primary hover:underline"
              params={{ projectSlug }}
              to="/projects/$projectSlug/settings/config"
            >
              {t("projectSubpages.factsManageAction")}
            </Link>
          </div>
          <div className="space-y-2">
            {(homeConfig.data ?? []).map((row) => (
              <FactRow key={row.key} row={row} />
            ))}
          </div>
        </section>
      )}

      {/* 成员待办 */}
      {canReadTasks && summary.data && summary.data.workload.length > 0 ? (
        <section className="rounded-lg border bg-card p-4">
          <h2 className="mb-3 text-sm font-medium">
            {t("projectSubpages.workloadTitle")}
          </h2>
          <div className="space-y-2">
            {summary.data.workload
              .filter(
                // 未分配行（user 为 null）在 open_count 为 0 时不显示，避免噪音。
                (row) => row.user != null || row.open_count > 0
              )
              .slice(0, 5)
              .map((row, index) => (
                <div className="text-sm" key={row.label || `wl-${index}`}>
                  <div className="font-medium">{row.label}</div>
                  <div className="text-muted-foreground">
                    {t("projectSubpages.workloadPending", { count: row.open_count })}
                    {row.overdue_count > 0
                      ? " · " +
                        t("projectSubpages.workloadOverdue", {
                          count: row.overdue_count,
                        })
                      : ""}
                    {row.high_priority_count > 0
                      ? " · " +
                        t("projectSubpages.workloadHighPriority", {
                          count: row.high_priority_count,
                        })
                      : ""}
                  </div>
                </div>
              ))}
          </div>
        </section>
      ) : null}

      {/* 最近活动 */}
      <section className="rounded-lg border bg-card p-4">
        <div className="mb-2 flex items-center justify-between">
          <h2 className="text-sm font-medium">
            {t("projectSubpages.recentActivityTitle")}
          </h2>
          <Link
            className="text-sm text-primary hover:underline"
            params={{ projectSlug, workspaceSlug }}
            to="/workspaces/$workspaceSlug/projects/$projectSlug/activity"
          >
            {t("projectSubpages.recentActivityViewAll")}
          </Link>
        </div>
        {timeline.isError ? (
          <p className="text-sm text-destructive">
            {t("projectSubpages.railTimelineError")}
          </p>
        ) : timelinePreview.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t("projectSubpages.activityEmpty")}
          </p>
        ) : (
          <ol className="space-y-2">
            {timelinePreview.map((entry, index) => (
              <li className="text-sm" key={`overview-act-${index}`}>
                <div className="font-medium">
                  {entry.source_label ?? entry.source_type ?? ""}
                </div>
                {entry.content ? (
                  <div className="text-muted-foreground">{entry.content}</div>
                ) : null}
              </li>
            ))}
          </ol>
        )}
      </section>
    </div>
  )
}

function FocusRow({
  href,
  label,
  refs,
  total,
}: {
  href: string
  label: string
  refs: Array<{ label?: string; task_slug?: string; title: string }>
  total: number
}) {
  const { t } = useTranslation()
  const refLabels = refs
    .slice(0, 3)
    .map((ref) => ref.label || ref.task_slug || "")
    .filter(Boolean)
  return (
    <div className="flex items-center justify-between gap-3 text-sm">
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-semibold">{total}</span>
        {refLabels.length > 0 ? (
          <span className="text-muted-foreground">
            {refLabels.join("、")}
          </span>
        ) : null}
      </div>
      <a className="text-primary hover:underline" href={href}>
        {t("projectSubpages.focusViewAction")}
      </a>
    </div>
  )
}

function FactRow({
  row,
}: {
  row: Awaited<ReturnType<typeof listProjectEffectiveConfig>>[number]
}) {
  const { t } = useTranslation()
  const label = row.definition.label || row.key
  let value: React.ReactNode = "—"
  if (row.missing_required) {
    value = t("projectSubpages.factsMissingRequired")
  } else if (row.definition.secret) {
    value = row.value ? t("projectSubpages.secretConfigured") : "—"
  } else if (row.value !== null && row.value !== "") {
    value = formatConfigDisplayValue(row.definition.value_type, row.value)
  }
  return (
    <div className="flex items-center justify-between gap-3 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate font-medium">{value}</span>
    </div>
  )
}
