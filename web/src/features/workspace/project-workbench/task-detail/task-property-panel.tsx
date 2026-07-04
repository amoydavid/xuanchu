import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import {
  extractUDAs,
  formatUDAValue,
} from "@/features/workspace/project-readonly/uda"
import {
  recurrenceLabel,
  recurrenceOptions,
  taskStatusLabel,
} from "@/features/workspace/shared/task-labels"
import type { ProjectWorkbenchTaskRef } from "../api/project-api"
import type { ProjectTask } from "../api/task-api"
import { useModifyTaskMutation } from "../hooks/use-task-mutations"
import { InlineDatePicker } from "../shared/inline-date-picker"
import { InlineSelectEditor } from "../shared/inline-select-editor"
import { InlineTextEditor } from "../shared/inline-text-editor"
import { useEditFeedback } from "../shared/edit-feedback"
import { AssigneePicker } from "./assignee-picker"
import { TagPicker } from "./tag-picker"
import { TaskDependencyPicker } from "./task-dependency-picker"

type TaskPropertyPanelProps = {
  canWrite: boolean
  projectSlug: string
  task: ProjectTask
  taskRef: string
  workspaceSlug: string
}

const priorityOptions = [
  { label: "-", value: "none" },
  { label: "H", value: "H" },
  { label: "M", value: "M" },
  { label: "L", value: "L" },
]

export function TaskPropertyPanel({
  canWrite,
  projectSlug,
  task,
  taskRef,
  workspaceSlug,
}: TaskPropertyPanelProps) {
  const { t } = useTranslation()
  const modify = useModifyTaskMutation(workspaceSlug, projectSlug, taskRef)
  const udas = useMemo(() => extractUDAs(task), [task])
  const recurrenceSelectOptions = recurrenceOptions.map((option) => ({
    label: t(option.labelKey),
    value: option.value,
  }))

  return (
    <aside className="space-y-3 border bg-card p-4 text-sm">
      <h2 className="text-xs font-medium text-muted-foreground uppercase">
        {t("projectReadonly.attributes")}
      </h2>
      <PropertyRow label={t("common.status")}>
        <div className="font-medium">{taskStatusLabel(task.status, t)}</div>
      </PropertyRow>
      <PropertyRow label={t("projectReadonly.priority")}>
        <InlineSelectEditor
          ariaLabel={t("projectReadonly.priority")}
          className="h-7 w-full"
          disabled={!canWrite}
          onSave={async (priority) => {
            await modify.mutateAsync(
              priority === "none" ? { clear_priority: true } : { priority }
            )
          }}
          options={priorityOptions}
          placeholder="-"
          value={task.priority ?? "none"}
        />
      </PropertyRow>
      <PropertyRow label={t("projectReadonly.dueDate")}>
        <InlineDatePicker
          ariaLabel={t("projectReadonly.dueDate")}
          className="h-7"
          disabled={!canWrite}
          onSave={async (due) => {
            await modify.mutateAsync(
              due === null ? { clear_due: true } : { due }
            )
          }}
          value={unixLikeToNumber(task.due)}
        />
      </PropertyRow>
      <PropertyRow label={t("projectReadonly.assignee")}>
        <AssigneePicker
          disabled={!canWrite}
          onSave={async (items) => {
            // 后端 assignees 字段是「增量追加」语义（add），不是替换。
            // 这里用 clear + assignees 表达「整体替换为 items」，
            // 避免 a→b 时因未移除 a 导致结果变成 a+b。
            await modify.mutateAsync(
              items.length === 0
                ? { clear_assignees: true }
                : { clear_assignees: true, assignees: items }
            )
          }}
          value={task.assignees ?? []}
          workspaceSlug={workspaceSlug}
        />
      </PropertyRow>
      <PropertyRow label={t("projectReadonly.tags")}>
        <TagPicker
          disabled={!canWrite}
          onSave={async (items) => {
            await modify.mutateAsync({ tags: items })
          }}
          projectSlug={projectSlug}
          value={task.tags ?? []}
          workspaceSlug={workspaceSlug}
        />
      </PropertyRow>
      <DateProperty
        disabled={!canWrite}
        label={t("projectReadonly.waitUntil")}
        onSave={async (wait) => {
          await modify.mutateAsync(
            wait === null ? { clear_wait: true } : { wait }
          )
        }}
        value={task.wait}
      />
      <DateProperty
        disabled={!canWrite}
        label={t("projectReadonly.scheduledStart")}
        onSave={async (scheduled) => {
          await modify.mutateAsync(
            scheduled === null ? { clear_scheduled: true } : { scheduled }
          )
        }}
        value={task.scheduled}
      />
      <DateProperty
        disabled={!canWrite}
        label={t("projectReadonly.hideUntil")}
        onSave={async (until) => {
          await modify.mutateAsync(
            until === null ? { clear_until: true } : { until }
          )
        }}
        value={task.until}
      />
      <PropertyRow label={t("projectReadonly.recur")}>
        <InlineSelectEditor
          ariaLabel={t("projectReadonly.recur")}
          className="h-7 w-full"
          disabled={!canWrite}
          onSave={async (recur) => {
            await modify.mutateAsync(
              recur === "none" ? { clear_recur: true } : { recur }
            )
          }}
          options={recurrenceSelectOptions}
          placeholder="-"
          value={task.recur ?? "none"}
        />
        <div className="mt-1 text-xs text-muted-foreground">
          {recurrenceLabel(task.recur, t)}
        </div>
      </PropertyRow>
      <PropertyRow label={t("projectReadonly.dependsOn")}>
        <TaskDependencyPicker
          disabled={!canWrite}
          onSave={async (depends) => {
            await modify.mutateAsync(
              depends.length === 0 ? { clear_depends: true } : { depends }
            )
          }}
          projectSlug={projectSlug}
          refs={task.depends_info}
          taskUUID={task.uuid}
          value={task.depends ?? []}
          workspaceSlug={workspaceSlug}
        />
      </PropertyRow>
      {task.parent ? (
        <PropertyRow label={t("projectReadonly.parent")}>
          <TaskRefLinks
            projectSlug={projectSlug}
            refs={task.parent_info ? [task.parent_info] : undefined}
            uuids={[task.parent]}
            workspaceSlug={workspaceSlug}
          />
        </PropertyRow>
      ) : null}
      {task.blocked_by_info && task.blocked_by_info.length > 0 ? (
        <PropertyRow label={t("projectReadonly.blocking")}>
          <TaskRefLinks
            projectSlug={projectSlug}
            refs={task.blocked_by_info}
            uuids={task.blocked_by_info.map((ref) => ref.uuid)}
            workspaceSlug={workspaceSlug}
          />
        </PropertyRow>
      ) : null}
      <PropertyRow label={t("projectReadonly.entry")}>
        <div className="font-medium">{formatRFCDate(task.entry)}</div>
      </PropertyRow>
      <PropertyRow label={t("projectReadonly.modified")}>
        <div className="font-medium">{formatRFCDate(task.modified)}</div>
      </PropertyRow>
      {udas.length > 0 ? (
        <>
          <Separator />
          <h2 className="text-xs font-medium text-muted-foreground uppercase">
            {t("projectReadonly.customFields")}
          </h2>
          {udas.map(([key, value]) => (
            <PropertyRow key={key} label={key}>
              <UDAFieldEditor
                name={key}
                disabled={!canWrite}
                onSave={async (nextValue) => {
                  await modify.mutateAsync({ udas: { [key]: nextValue } })
                }}
                value={value}
              />
            </PropertyRow>
          ))}
        </>
      ) : null}
    </aside>
  )
}

function UDAFieldEditor({
  disabled,
  name,
  onSave,
  value,
}: {
  disabled: boolean
  name: string
  onSave: (value: string) => Promise<void> | void
  value: unknown
}) {
  const kind = inferUDAKind(name, value)
  const normalizedValue = formatUDAValue(value)
  if (kind === "boolean") {
    return (
      <UDABooleanEditor
        checked={toBoolean(value)}
        disabled={disabled}
        name={name}
        onSave={onSave}
      />
    )
  }
  if (kind === "number" || kind === "date") {
    return (
      <UDAInputEditor
        disabled={disabled}
        name={name}
        onSave={onSave}
        type={kind === "number" ? "number" : "date"}
        value={normalizedValue}
      />
    )
  }
  return (
    <InlineTextEditor
      ariaLabel={`UDA ${name}`}
      disabled={disabled}
      emptyLabel="-"
      onSave={onSave}
      value={normalizedValue}
    />
  )
}

function UDABooleanEditor({
  checked,
  disabled,
  name,
  onSave,
}: {
  checked: boolean
  disabled: boolean
  name: string
  onSave: (value: string) => Promise<void> | void
}) {
  const feedback = useEditFeedback()
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  return (
    <span className="inline-flex flex-col gap-1">
      <Switch
        aria-label={`UDA ${name}`}
        checked={checked}
        disabled={disabled || saving}
        onCheckedChange={async (next) => {
          setSaving(true)
          setError(null)
          try {
            await onSave(String(Boolean(next)))
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err)
            setError(message)
            feedback.failure(`UDA ${name}`, message)
          } finally {
            setSaving(false)
          }
        }}
      />
      {error ? <span className="text-xs text-destructive">{error}</span> : null}
    </span>
  )
}

function UDAInputEditor({
  disabled,
  name,
  onSave,
  type,
  value,
}: {
  disabled: boolean
  name: string
  onSave: (value: string) => Promise<void> | void
  type: "date" | "number"
  value: string
}) {
  const feedback = useEditFeedback()
  const [draftState, setDraftState] = useState(() => ({
    source: value,
    value,
  }))
  const draft = draftState.source === value ? draftState.value : value
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const setDraft = (next: string) =>
    setDraftState({ source: value, value: next })

  const save = async () => {
    if (draft === value) {
      return
    }
    setSaving(true)
    setError(null)
    try {
      await onSave(draft)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure(`UDA ${name}`, message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <span className="inline-flex flex-col gap-1">
      <Input
        aria-label={`UDA ${name}`}
        className="h-7"
        disabled={disabled || saving}
        onBlur={() => {
          void save()
        }}
        onChange={(event) => {
          setDraft(event.target.value)
          setError(null)
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault()
            void save()
          }
          if (event.key === "Escape") {
            event.preventDefault()
            setDraft(value)
            setError(null)
          }
        }}
        type={type}
        value={draft}
      />
      {error ? <span className="text-xs text-destructive">{error}</span> : null}
    </span>
  )
}

function inferUDAKind(
  name: string,
  value: unknown
): "boolean" | "date" | "number" | "text" {
  if (typeof value === "boolean") {
    return "boolean"
  }
  if (typeof value === "number") {
    return "number"
  }
  const raw = formatUDAValue(value)
  if (/^(true|false)$/i.test(raw)) {
    return "boolean"
  }
  if (/^\d{4}-\d{2}-\d{2}$/.test(raw) || /(^|_)(date|day)$/.test(name)) {
    return "date"
  }
  if (/^-?\d+(\.\d+)?$/.test(raw)) {
    return "number"
  }
  return "text"
}

function toBoolean(value: unknown): boolean {
  if (typeof value === "boolean") {
    return value
  }
  return /^true$/i.test(formatUDAValue(value))
}

function PropertyRow({
  children,
  label,
}: {
  children: React.ReactNode
  label: string
}) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1">{children}</div>
    </div>
  )
}

function DateProperty({
  disabled,
  label,
  onSave,
  value,
}: {
  disabled: boolean
  label: string
  onSave: (value: number | null) => Promise<void> | void
  value?: string | number | null
}) {
  return (
    <PropertyRow label={label}>
      <InlineDatePicker
        ariaLabel={label}
        className="h-7"
        disabled={disabled}
        onSave={onSave}
        value={unixLikeToNumber(value)}
      />
    </PropertyRow>
  )
}

function TaskRefLinks({
  projectSlug,
  refs,
  uuids,
  workspaceSlug,
}: {
  projectSlug: string
  refs?: ProjectWorkbenchTaskRef[]
  uuids: string[]
  workspaceSlug: string
}) {
  return (
    <div className="flex flex-col gap-2">
      {uuids.map((uuid) => {
        const info = refs?.find((ref) => ref.uuid === uuid)
        const ref = info?.task_slug || uuid
        const title = info?.title || info?.task_slug || uuid.slice(0, 8)
        return (
          <a
            className="block"
            href={`/workspaces/${workspaceSlug}/projects/${projectSlug}/tasks/${ref}`}
            key={uuid}
          >
            {info?.task_slug ? (
              <code className="text-[11px] text-muted-foreground">
                {info.task_slug}
              </code>
            ) : null}
            <span className="block truncate font-medium text-primary underline-offset-4 hover:underline">
              {title}
            </span>
          </a>
        )
      })}
    </div>
  )
}

function unixLikeToNumber(value: string | number | null | undefined) {
  if (typeof value === "number") {
    return value
  }
  if (typeof value === "string") {
    const parsed = Date.parse(value)
    if (!Number.isNaN(parsed)) {
      return Math.floor(parsed / 1000)
    }
  }
  return null
}

function formatRFCDate(value?: string): string {
  return value ? value.slice(0, 10) : "-"
}
