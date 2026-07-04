import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { MarkdownView } from "@/components/markdown"
import { useTaskAuditQuery } from "../hooks/use-task-detail-data"
import type {
  TaskAuditEntry,
  TaskChangeDisplayValue,
  TaskFieldChange,
} from "../api/task-api"

type TaskChangeHistoryProps = {
  workspaceSlug: string
  taskRef: string
}

// TaskChangeHistory 渲染任务的字段级变更历史。
// 数据来自 GET /tasks/{ref}/audit，每条按时间倒序展示。
// 前端用 field + raw + i18n locale 把变更渲染成自然语言句子，
// 不直接展示 raw JSON / unix 秒 / null。
export function TaskChangeHistory({
  workspaceSlug,
  taskRef,
}: TaskChangeHistoryProps) {
  const { t } = useTranslation()
  const audit = useTaskAuditQuery(workspaceSlug, taskRef)

  if (audit.isError) {
    return (
      <section className="space-y-2 border bg-card p-4">
        <h2 className="text-sm font-medium">
          {t("projectWorkbench.taskHistory.title")}
        </h2>
        <p className="text-xs text-muted-foreground">
          {t("projectWorkbench.taskHistory.unavailable")}
        </p>
      </section>
    )
  }

  // 只展示带字段级 changes 的行；旧 audit 行无 changes 时不显示。
  const rows = (audit.data ?? []).filter(
    (entry) => (entry.changes ?? []).length > 0
  )

  return (
    <section className="space-y-3 border bg-card p-4">
      <h2 className="text-sm font-medium">
        {t("projectWorkbench.taskHistory.title")}
      </h2>
      {audit.isPending ? (
        <p className="text-xs text-muted-foreground">…</p>
      ) : rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("projectWorkbench.taskHistory.empty")}
        </p>
      ) : (
        <ul className="space-y-3">
          {rows.map((entry) =>
            (entry.changes ?? []).map((change, index) => (
              <li
                className="text-sm leading-6 text-muted-foreground"
                key={`${entry.id}-${index}`}
              >
                <ChangeLine entry={entry} change={change} />
              </li>
            ))
          )}
        </ul>
      )}
    </section>
  )
}

function ChangeLine({
  entry,
  change,
}: {
  entry: TaskAuditEntry
  change: TaskFieldChange
}) {
  const { t } = useTranslation()
  const actor = actorLabel(entry, t)
  const createdAt = formatTimestamp(entry.created_at)
  const field = t(`projectWorkbench.taskHistory.field.${change.field}`)

  if (change.kind === "set") {
    return (
      <span>
        <span className="text-foreground">{createdAt}</span> ·{" "}
        {t("projectWorkbench.taskHistory.setChange", {
          actor,
          field,
          added: formatSet(change.added, change.field, t, false),
          removed: formatSet(change.removed, change.field, t, true),
        })}
      </span>
    )
  }

  // description 体积可能很大，走专门的可展开渲染。
  if (change.field === "description") {
    return (
      <DescriptionChangeLine
        actor={actor}
        createdAt={createdAt}
        change={change}
      />
    )
  }

  return (
    <span>
      <span className="text-foreground">{createdAt}</span> ·{" "}
      {t("projectWorkbench.taskHistory.scalarChange", {
        actor,
        field,
        previous: formatScalar(change.previous, change.field, t),
        current: formatScalar(change.current, change.field, t),
      })}
    </span>
  )
}

function DescriptionChangeLine({
  actor,
  createdAt,
  change,
}: {
  actor: string
  createdAt: string
  change: Extract<TaskFieldChange, { kind: "scalar" }>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const previous = formatScalar(change.previous, "description", t)
  const current = formatScalar(change.current, "description", t)

  return (
    <div className="space-y-1">
      <div>
        <span className="text-foreground">{createdAt}</span> ·{" "}
        {t("projectWorkbench.taskHistory.changedDescription", { actor })}
      </div>
      <div className="flex flex-wrap items-center gap-1 text-xs">
        <Badge variant="outline">
          {t("projectWorkbench.taskHistory.changedDescription", {
            actor: "",
          }).trim()}
          : {current}
        </Badge>
        <Button
          onClick={() => setOpen(true)}
          size="sm"
          type="button"
          variant="ghost"
        >
          {t("projectWorkbench.taskHistory.expandValue")}
        </Button>
      </div>
      <Dialog onOpenChange={setOpen} open={open}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t("projectWorkbench.taskHistory.changedDescription", {
                actor,
              })}
            </DialogTitle>
            <DialogDescription>{createdAt}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 text-sm">
            <div>
              <div className="mb-1 text-xs uppercase text-muted-foreground">
                {t("projectWorkbench.taskHistory.changedDescription", {
                  actor: "",
                }).trim()}
              </div>
              <DescriptionValue value={change.current} />
            </div>
            <div>
              <div className="mb-1 text-xs uppercase text-muted-foreground">
                {t("projectWorkbench.taskHistory.removed")}
              </div>
              <DescriptionValue value={change.previous} />
            </div>
          </div>
        </DialogContent>
      </Dialog>
      {/* previous 留作可访问性兜底，避免直接丢失 */}
      <span className="sr-only">{previous}</span>
    </div>
  )
}

function DescriptionValue({ value }: { value: TaskChangeDisplayValue }) {
  const { t } = useTranslation()
  if (value.raw == null) {
    return (
      <span className="text-muted-foreground">
        {t("projectWorkbench.taskHistory.unset")}
      </span>
    )
  }
  if (typeof value.raw === "string") {
    return (
      <MarkdownView className="max-w-none text-sm leading-6 text-muted-foreground">
        {value.raw}
      </MarkdownView>
    )
  }
  return <span className="text-muted-foreground">{value.text}</span>
}

function actorLabel(
  entry: TaskAuditEntry,
  t: ReturnType<typeof useTranslation>["t"]
): string {
  if (entry.actor) {
    return (
      entry.actor.display_name ||
      entry.actor.name ||
      entry.actor.id ||
      t("projectWorkbench.taskHistory.unknownActor")
    )
  }
  if (entry.actor_token) {
    return entry.actor_token.name || entry.actor_token.id
  }
  return t("projectWorkbench.taskHistory.unknownActor")
}

// formatScalar 把标量 raw 渲染成人类可读文本。
// null -> 未设置；due -> 本地日期；其它直接文本化。
function formatScalar(
  value: TaskChangeDisplayValue | undefined,
  field: string,
  t: ReturnType<typeof useTranslation>["t"]
): string {
  if (!value || value.raw == null) {
    return t("projectWorkbench.taskHistory.unset")
  }
  if (field === "due" && typeof value.raw === "number") {
    return new Date(value.raw * 1000).toLocaleDateString()
  }
  if (typeof value.raw === "string" || typeof value.raw === "number") {
    return String(value.raw)
  }
  // 复杂对象等未知类型回退到后端兜底文案。
  return value.text
}

// formatSet 把集合元素渲染成逗号分隔的文本。
// assignees 用 display_name -> name -> id；tags 直接字符串。
function formatSet(
  values: TaskChangeDisplayValue[] | undefined,
  field: string,
  t: ReturnType<typeof useTranslation>["t"],
  removed: boolean
): string {
  const items = values ?? []
  if (items.length === 0) {
    return t("projectWorkbench.taskHistory.unset")
  }
  const parts = items.map((item) => formatSetItem(item, field, t))
  const join = parts.join(", ")
  return `${t(
    removed
      ? "projectWorkbench.taskHistory.removed"
      : "projectWorkbench.taskHistory.added"
  )} ${join}`
}

function formatSetItem(
  item: TaskChangeDisplayValue,
  field: string,
  t: ReturnType<typeof useTranslation>["t"]
): string {
  if (field === "assignees" && item.raw && typeof item.raw === "object") {
    const obj = item.raw as {
      display_name?: string
      name?: string
      id?: string
    }
    return (
      obj.display_name || obj.name || obj.id || item.text || ""
    )
  }
  if (item.raw == null) {
    return t("projectWorkbench.taskHistory.unset")
  }
  if (typeof item.raw === "string") {
    return item.raw
  }
  return item.text
}

function formatTimestamp(unixSeconds: number): string {
  if (!unixSeconds) {
    return ""
  }
  return new Date(unixSeconds * 1000).toLocaleString()
}
