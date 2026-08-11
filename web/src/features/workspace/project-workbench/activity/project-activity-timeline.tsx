import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import type { TaskFieldChange } from "../api/task-api"
import type { ProjectTimelineEntry } from "../api/project-api"
import type { AuditRow } from "@/features/workspace/audit/audit-api"

// ActivityItem 统一 project annotation、task（注释与生命周期事件）和 audit 三类记录。
// 去重 key 为 `${kind}:${id}:${entry}`；不能把 audit 与 annotation 合并成无来源的摘要。
//
// task 分支有两种形态：
//   - 注释（action 为空）：渲染 source_label（任务标题）+ content（注释正文）
//   - 生命周期事件（action 非空）：渲染动作文案 + 可点击的任务标题，可带 changes/link
export type ActivityItem =
  | {
      kind: "project"
      id: string
      entry: number
      content: string
      actorLabel: string
    }
  | {
      kind: "task"
      id: string
      entry: number
      content: string
      sourceLabel: string
      actorLabel: string
      taskHref?: string
      // 生命周期事件专用字段（注释条目留空）。
      action?: string
      changes?: TaskFieldChange[]
      link?: { id: string; type: string; url: string; title: string } | null
    }
  | {
      kind: "audit"
      id: string
      entry: number
      action: string
      actorLabel: string
      summary: string
    }

type ProjectActivityTimelineProps = {
  items: ActivityItem[]
  emptyLabel?: string
  // workspaceSlug/projectSlug 用于生成任务跳转链接；活动页传入，概览/侧栏可不传。
  workspaceSlug?: string
  projectSlug?: string
}

// annotationAuditActions 列出同时写 annotations 表和 audit_logs 表的注释类动作。
// 这些动作在 timeline 里已由 project/task 注释条目覆盖，audit 分支跳过避免重复。
const annotationAuditActions = new Set([
  "project.annotate",
  "project.denotate",
  "task.annotate",
  "task.denotate",
])

// mergeActivity 合并 timeline 与 audit，按 entry 倒序，按 kind:id:entry 去重。
// taskHref 仅在 workspaceSlug/projectSlug 同时提供时生成（指向任务详情）。
export function mergeActivity(
  timeline: ProjectTimelineEntry[],
  audit: AuditRow[] | undefined,
  filter: ActivityFilter,
  route?: { workspaceSlug: string; projectSlug: string }
): ActivityItem[] {
  const items: ActivityItem[] = []
  const taskHref = (sourceId: string) =>
    route
      ? `/workspaces/${route.workspaceSlug}/projects/${route.projectSlug}/tasks/${sourceId}`
      : undefined
  if (filter.project || filter.all) {
    for (const entry of timeline) {
      if (entry.source_type === "project") {
        items.push({
          kind: "project",
          id: entry.source_id ?? "",
          entry: entry.entry ?? 0,
          content: entry.content ?? "",
          actorLabel: actorLabelFromTimeline(entry),
        })
      }
    }
  }
  if (filter.task || filter.all) {
    for (const entry of timeline) {
      if (entry.source_type === "task") {
        items.push({
          kind: "task",
          id: entry.source_id ?? "",
          entry: entry.entry ?? 0,
          content: entry.content ?? "",
          sourceLabel: entry.source_label ?? "",
          actorLabel: actorLabelFromTimeline(entry),
          taskHref: taskHref(entry.source_id ?? ""),
          action: entry.action,
          changes: entry.changes,
          link: entry.link ?? undefined,
        })
      }
    }
  }
  if ((filter.audit || filter.all) && audit) {
    for (const row of audit) {
      // 跳过已被 timeline 覆盖的审计行，避免同一条动作重复显示：
      //   - target_type === "task"：任务生命周期/字段/链接事件已由 timeline
      //     的任务活动条目覆盖（渲染更完整，带任务标题/动作文案/可点击跳转）。
      //   - 注释类 action（project.annotate / project.denotate /
      //     task.annotate / task.denotate）：注释本身已由 timeline 的
      //     project/task 注释条目覆盖，audit 不再重复。
      if (row.target_type === "task") continue
      if (annotationAuditActions.has(row.action)) continue
      items.push({
        kind: "audit",
        id: `${row.action}:${row.id}`,
        entry: row.created_at,
        action: row.action,
        actorLabel: row.actor?.name ?? row.actor_type ?? "",
        summary: summarizeAuditPayload(row),
      })
    }
  }
  // 去重：kind + id + entry
  const seen = new Set<string>()
  const deduped: ActivityItem[] = []
  for (const item of items) {
    const key = `${item.kind}:${item.id}:${item.entry}`
    if (seen.has(key)) continue
    seen.add(key)
    deduped.push(item)
  }
  deduped.sort((a, b) => b.entry - a.entry)
  return deduped
}

export type ActivityFilter = {
  all?: boolean
  project?: boolean
  task?: boolean
  audit?: boolean
}

function actorLabelFromTimeline(entry: ProjectTimelineEntry): string {
  const by = entry.created_by
  if (by?.user?.name) return by.user.name
  if (by?.token?.name) return by.token.name
  if (by?.name) return by.name
  if (entry.actor?.name) return entry.actor.name
  return ""
}

// summarizeAuditPayload 仅展示 payload 里已有的字段，不解释成无来源的自然语言。
function summarizeAuditPayload(row: AuditRow): string {
  const payload = row.payload
  if (payload && typeof payload === "object") {
    const obj = payload as Record<string, unknown>
    const from = obj["from"]
    const to = obj["to"]
    if (from !== undefined && to !== undefined) {
      return `${formatValue(from)} → ${formatValue(to)}`
    }
  }
  return ""
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) {
    return "—"
  }
  return String(value)
}

// ProjectActivityTimeline 按 entry 倒序渲染合并后的 ActivityItem。
export function ProjectActivityTimeline({
  items,
  emptyLabel,
  workspaceSlug,
  projectSlug,
}: ProjectActivityTimelineProps) {
  const { t } = useTranslation()
  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {emptyLabel ?? t("projectSubpages.activityEmpty")}
      </p>
    )
  }
  return (
    <ol className="space-y-3">
      {items.map((item, index) => (
        <li className="rounded-lg border bg-card p-3 text-sm" key={`${item.kind}:${item.id}:${item.entry}:${index}`}>
          <ActivityItemView
            item={item}
            projectSlug={projectSlug}
            workspaceSlug={workspaceSlug}
          />
        </li>
      ))}
    </ol>
  )
}

function ActivityItemView({
  item,
  workspaceSlug,
  projectSlug,
}: {
  item: ActivityItem
  workspaceSlug?: string
  projectSlug?: string
}) {
  const { t } = useTranslation()
  const entryTime = formatEntryTime(item.entry)
  if (item.kind === "project") {
    return (
      <div className="space-y-1">
        <div className="text-xs text-muted-foreground">
          {t("projectSubpages.activityFilterProject")} · {item.actorLabel} · {entryTime}
        </div>
        <div>{item.content}</div>
      </div>
    )
  }
  if (item.kind === "task") {
    // 生命周期事件（action 非空）：渲染「{actor} {动作} 《{任务标题}》」。
    if (item.action) {
      return (
        <div className="space-y-1">
          <div className="text-xs text-muted-foreground">
            {t("projectSubpages.activityFilterTask")} · {entryTime}
          </div>
          <div className="leading-6">
            {item.actorLabel ? (
              <span className="font-medium text-foreground">{item.actorLabel} </span>
            ) : null}
            <span className="text-foreground">
              {t(`projectSubpages.activityTaskAction.${item.action}`, {
                title: item.sourceLabel,
              })}
            </span>
            {item.action === "fields_changed" && item.sourceLabel && workspaceSlug && projectSlug ? (
              <span>
                {" · "}
                <Link
                  className="text-primary underline-offset-4 hover:underline"
                  params={{
                    workspaceSlug,
                    projectSlug,
                    taskRef: item.id,
                  }}
                  to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
                >
                  {item.sourceLabel}
                </Link>
              </span>
            ) : null}
          </div>
          {item.changes && item.changes.length > 0 ? (
            <ActivityChanges changes={item.changes} />
          ) : null}
          {item.link ? <ActivityLink link={item.link} /> : null}
        </div>
      )
    }
    // 任务注释（action 为空）：渲染 source_label（任务标题）+ content。
    const titleElement =
      item.sourceLabel && workspaceSlug && projectSlug ? (
        <Link
          className="text-primary underline-offset-4 hover:underline"
          params={{
            workspaceSlug,
            projectSlug,
            taskRef: item.id,
          }}
          to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
        >
          {item.sourceLabel}
        </Link>
      ) : (
        item.sourceLabel
      )
    return (
      <div className="space-y-1">
        <div className="text-xs text-muted-foreground">
          {t("projectSubpages.activityFilterTask")} · {titleElement} · {item.actorLabel} · {entryTime}
        </div>
        <div>{item.content}</div>
      </div>
    )
  }
  return (
    <div className="space-y-1">
      <div className="text-xs text-muted-foreground">
        {t("projectSubpages.activityFilterAudit")} · {item.action} · {item.actorLabel} · {entryTime}
      </div>
      {item.summary ? <div>{item.summary}</div> : null}
    </div>
  )
}

// ActivityChanges 渲染任务字段变更列表（与 task 详情页保持一致的语义）。
function ActivityChanges({ changes }: { changes: TaskFieldChange[] }) {
  const { i18n, t } = useTranslation()
  return (
    <div className="space-y-1 text-xs text-muted-foreground">
      {changes.map((change, index) =>
        change.kind === "scalar" && change.field === "description" ? (
          <div key={`${change.field}-${index}`}>
            <span className="text-foreground">
              {t(`projectWorkbench.taskHistory.field.${change.field}`)}
            </span>
          </div>
        ) : (
          <div key={`${change.field}-${index}`}>
            <span className="text-foreground">
              {t(`projectWorkbench.taskHistory.field.${change.field}`)}
            </span>
            {change.kind === "scalar" ? (
              <span>
                {" "}
                {activityChangeValue(change.previous, change.field, i18n.language, t)}
                {" → "}
                {activityChangeValue(change.current, change.field, i18n.language, t)}
              </span>
            ) : change.kind === "set" ? (
              <span>
                {" · "}
                {t("projectWorkbench.taskHistory.added")}:{" "}
                {formatSet(change.added, change.field, t)};{" "}
                {t("projectWorkbench.taskHistory.removed")}:{" "}
                {formatSet(change.removed, change.field, t)}
              </span>
            ) : (
              <ul className="ml-3 list-disc">
                {change.entries.map((entry) => (
                  <li key={entry.name}>
                    {entry.name}:{" "}
                    {entry.before?.text ?? t("projectWorkbench.taskHistory.unset")} →{" "}
                    {entry.after?.text ?? t("projectWorkbench.taskHistory.unset")}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )
      )}
    </div>
  )
}

function activityChangeValue(
  value: { raw: unknown; text: string } | null | undefined,
  field: string,
  locale: string,
  t: ReturnType<typeof useTranslation>["t"]
) {
  if (!value) return t("projectWorkbench.taskHistory.unset")
  if (value.raw == null) return t("projectWorkbench.taskHistory.unset")
  if (["due", "wait", "scheduled", "until"].includes(field) && typeof value.raw === "number") {
    return new Date(value.raw * 1000).toLocaleDateString(locale)
  }
  if (typeof value.raw === "string" || typeof value.raw === "number") return String(value.raw)
  return value.text
}

function formatSet(
  values: Array<{ raw: unknown; text: string }>,
  field: string,
  t: ReturnType<typeof useTranslation>["t"]
) {
  if (values.length === 0) return t("projectWorkbench.taskHistory.none")
  return values
    .map((value) => {
      if (field === "assignees" && value.raw && typeof value.raw === "object") {
        const actor = value.raw as { display_name?: string; name?: string; id?: string }
        return actor.display_name || actor.name || actor.id || value.text
      }
      return value.raw == null ? t("projectWorkbench.taskHistory.unset") : value.text
    })
    .join(", ")
}

function ActivityLink({
  link,
}: {
  link: { id: string; type: string; url: string; title: string }
}) {
  const label = link.title || link.url || link.id
  return link.url ? (
    <a
      className="inline-flex max-w-full truncate text-xs text-primary underline-offset-4 hover:underline"
      href={link.url}
      rel="noreferrer"
      target="_blank"
    >
      {label}
    </a>
  ) : (
    <span className="text-xs text-muted-foreground">{label}</span>
  )
}

function formatEntryTime(unix: number): string {
  if (!unix) return ""
  const date = new Date(unix * 1000)
  if (Number.isNaN(date.getTime())) return ""
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  const hours = String(date.getHours()).padStart(2, "0")
  const minutes = String(date.getMinutes()).padStart(2, "0")
  return `${year}-${month}-${day} ${hours}:${minutes}`
}
