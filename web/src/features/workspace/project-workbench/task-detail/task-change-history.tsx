import { useState } from "react"
import { useTranslation } from "react-i18next"

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
// 不直接展示 raw JSON / unix 秒 / null / UUID。
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
  const { t, i18n } = useTranslation()
  const locale = i18n.language
  const actor = actorLabel(entry, t)
  const createdAt = formatTimestamp(entry.created_at, locale)
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

  // description 体积可能很大，走专门的可展开渲染：列表行展示截断的
  // previous -> current，点击查看完整 Markdown before/after。
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
  const previous = truncateDescription(
    formatScalar(change.previous, "description", t)
  )
  const current = truncateDescription(
    formatScalar(change.current, "description", t)
  )

  return (
    <div className="space-y-1">
      <span>
        <span className="text-foreground">{createdAt}</span> ·{" "}
        {t("projectWorkbench.taskHistory.scalarChange", {
          actor,
          field: t("projectWorkbench.taskHistory.field.description"),
          previous,
          current,
        })}
      </span>
      <div>
        <Button
          onClick={() => setOpen(true)}
          size="sm"
          type="button"
          variant="link"
          className="h-auto p-0 text-xs"
        >
          {t("projectWorkbench.taskHistory.expandValue")}
        </Button>
      </div>
      <Dialog onOpenChange={setOpen} open={open}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t("projectWorkbench.taskHistory.changedDescription", { actor })}
            </DialogTitle>
            <DialogDescription>{createdAt}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 text-sm">
            <div>
              <div className="mb-1 text-xs uppercase text-muted-foreground">
                {t("projectWorkbench.taskHistory.currentValue")}
              </div>
              <DescriptionValue value={change.current} />
            </div>
            <div>
              <div className="mb-1 text-xs uppercase text-muted-foreground">
                {t("projectWorkbench.taskHistory.previousValue")}
              </div>
              <DescriptionValue value={change.previous} />
            </div>
          </div>
        </DialogContent>
      </Dialog>
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
    return (
      entry.actor_token.name ||
      t("projectWorkbench.taskHistory.unknownActor")
    )
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
// 空集合渲染为「无」（不是「未设置」，后者专指标量被清空）。
function formatSet(
  values: TaskChangeDisplayValue[] | undefined,
  field: string,
  t: ReturnType<typeof useTranslation>["t"],
  removed: boolean
): string {
  const items = values ?? []
  const verb = removed
    ? t("projectWorkbench.taskHistory.removed")
    : t("projectWorkbench.taskHistory.added")
  if (items.length === 0) {
    return t("projectWorkbench.taskHistory.none")
  }
  const parts = items.map((item) => formatSetItem(item, field, t))
  return `${verb} ${parts.join(", ")}`
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
    return obj.display_name || obj.name || obj.id || item.text || ""
  }
  if (item.raw == null) {
    return t("projectWorkbench.taskHistory.unset")
  }
  if (typeof item.raw === "string") {
    return item.raw
  }
  return item.text
}

// truncateDescription 压缩 description 用于 timeline 列表行展示，
// 避免整段 Markdown 撑高单行；完整 before/after 在展开 Dialog 里查看。
function truncateDescription(text: string): string {
  const max = 80
  // 去掉 markdown 标记符号的粗略处理，列表行只作摘要。
  const flat = text
    .replace(/[#*_`>~-]/g, " ")
    .replace(/\s+/g, " ")
    .trim()
  if (flat.length <= max) {
    return flat
  }
  return `${flat.slice(0, max)}…`
}

function formatTimestamp(unixSeconds: number, locale: string): string {
  if (unixSeconds === 0 || !Number.isFinite(unixSeconds)) {
    return ""
  }
  return new Date(unixSeconds * 1000).toLocaleString(locale)
}
