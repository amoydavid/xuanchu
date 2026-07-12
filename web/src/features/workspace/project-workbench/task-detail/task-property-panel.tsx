import { ChevronDownIcon, ChevronRightIcon, CircleHelpIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Switch } from "@/components/ui/switch"
import {
  extractUDAs,
  formatUDAValue,
} from "@/features/workspace/project-readonly/uda"
import {
  taskStatusLabel,
} from "@/features/workspace/shared/task-labels"
import type { ProjectWorkbenchTaskRef } from "../api/project-api"
import type { ProjectTask } from "../api/task-api"
import type { DateBoundary } from "../shared/date-boundary"
import { useModifyTaskMutation } from "../hooks/use-task-mutations"
import { InlineDatePicker } from "../shared/inline-date-picker"
import { InlineSelectEditor } from "../shared/inline-select-editor"
import { InlineTextEditor } from "../shared/inline-text-editor"
import { useEditFeedback } from "../shared/edit-feedback"
import { AssigneePicker } from "./assignee-picker"
import { TagPicker } from "./tag-picker"
import { TaskDependencyPicker } from "./task-dependency-picker"
import { TaskUrgencyPanel } from "./task-urgency-panel"

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

  // 分组是否「有内容」：用于空组隐身（spec §9.5）。
  // Schedule 全空时仍保留（可写用户需要入口新增计划字段），但不可写时全空则隐身。
  const hasSchedule =
    unixLikeToNumber(task.due) !== null ||
    unixLikeToNumber(task.wait) !== null ||
    unixLikeToNumber(task.scheduled) !== null ||
    unixLikeToNumber(task.until) !== null
  const hasRelations =
    !!task.parent ||
    (task.depends && task.depends.length > 0) ||
    (task.blocked_by_info && task.blocked_by_info.length > 0) ||
    (task.links && task.links.length > 0)
  const hasUDA = udas.length > 0
  // 不可写且计划字段全空时，Schedule 整组隐身（避免空壳噪音）。
  const showSchedule = canWrite || hasSchedule

  return (
    <aside className="space-y-4 border bg-card p-4 text-sm">
      {/* Properties：高频字段，始终展示 */}
      <PropertyGroup
        title={t("taskDetail.groupProperties")}
      >
        <PropertyRow label={t("common.status")}>
          <div className="font-medium">{taskStatusLabel(task.status, t)}</div>
        </PropertyRow>
        <PropertyRow label={t("taskDetail.urgency")}>
          <TaskUrgencyPanel taskRef={taskRef} workspaceSlug={workspaceSlug} />
        </PropertyRow>
        <PropertyRow label={t("projectReadonly.priority")}>
          <InlineSelectEditor
            ariaLabel={t("projectReadonly.priority")}
            className="w-full"
            disabled={!canWrite}
            onSave={async (priority) => {
              await modify.mutateAsync(
                priority === "none" ? { clear_priority: true } : { priority }
              )
            }}
            options={priorityOptions}
            placeholder="-"
            triggerSize="sm"
            value={task.priority ?? "none"}
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
      </PropertyGroup>

      {/* Schedule：日期/周期字段；空组在不可写时隐身 */}
      {showSchedule ? (
        <PropertyGroup
          title={t("taskDetail.groupSchedule")}
        >
          <PropertyRow label={t("projectReadonly.dueDate")}>
            <InlineDatePicker
              ariaLabel={t("projectReadonly.dueDate")}
              boundary="end"
              className="w-full"
              disabled={!canWrite}
              onSave={async (due) => {
                await modify.mutateAsync(
                  due === null ? { clear_due: true } : { due }
                )
              }}
              value={unixLikeToNumber(task.due)}
            />
          </PropertyRow>
          <DateProperty
            disabled={!canWrite}
            helpText={t("projectReadonly.waitUntilHelp")}
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
            helpText={t("projectReadonly.scheduledStartHelp")}
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
            boundary="end"
            helpText={t("projectReadonly.untilHelp")}
            label={t("projectReadonly.until")}
            onSave={async (until) => {
              await modify.mutateAsync(
                until === null ? { clear_until: true } : { until }
              )
            }}
            value={task.until}
          />
        </PropertyGroup>
      ) : null}

      {/* Relations：parent/depends/blocking；空组隐身 */}
      {hasRelations ? (
        <PropertyGroup
          title={t("taskDetail.groupRelations")}
        >
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
        </PropertyGroup>
      ) : null}

      {/* System：默认折叠 */}
      <PropertyGroup
        defaultOpen={false}
        title={t("taskDetail.groupSystem")}
      >
        <PropertyRow label={t("projectReadonly.entry")}>
          <div className="font-medium">{formatRFCDate(task.entry)}</div>
        </PropertyRow>
        <PropertyRow label={t("projectReadonly.modified")}>
          <div className="font-medium">{formatRFCDate(task.modified)}</div>
        </PropertyRow>
      </PropertyGroup>

      {/* Custom fields：仅有 UDA 时展示 */}
      {hasUDA ? (
        <PropertyGroup title={t("taskDetail.groupCustom")}>
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
        </PropertyGroup>
      ) : null}
    </aside>
  )
}

// PropertyGroup 可折叠分组容器；hasContent=false 时整组不渲染（spec §9.5）。
function PropertyGroup({
  children,
  defaultOpen = true,
  title,
}: {
  children: React.ReactNode
  defaultOpen?: boolean
  title: string
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="space-y-2">
      <button
        className="flex w-full items-center gap-1 text-xs font-medium uppercase tracking-wide text-muted-foreground"
        onClick={() => setOpen((v) => !v)}
        type="button"
      >
        {open ? (
          <ChevronDownIcon className="size-3" />
        ) : (
          <ChevronRightIcon className="size-3" />
        )}
        {title}
      </button>
      {open ? <div className="space-y-3">{children}</div> : null}
    </div>
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
  helpText,
  label,
}: {
  children: React.ReactNode
  helpText?: string
  label: string
}) {
  return (
    <div>
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <span>{label}</span>
        {helpText ? <FieldHelp label={label} text={helpText} /> : null}
      </div>
      <div className="mt-1">{children}</div>
    </div>
  )
}

function DateProperty({
  boundary = "start",
  disabled,
  helpText,
  label,
  onSave,
  value,
}: {
  disabled: boolean
  boundary?: DateBoundary
  helpText?: string
  label: string
  onSave: (value: number | null) => Promise<void> | void
  value?: string | number | null
}) {
  return (
    <PropertyRow helpText={helpText} label={label}>
      <InlineDatePicker
        ariaLabel={label}
        boundary={boundary}
        className="w-full"
        disabled={disabled}
        onSave={onSave}
        value={unixLikeToNumber(value)}
      />
    </PropertyRow>
  )
}

function FieldHelp({ label, text }: { label: string; text: string }) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          aria-label={`说明：${label}`}
          className="size-5 text-muted-foreground hover:text-foreground"
          size="icon-xs"
          type="button"
          variant="ghost"
        >
          <CircleHelpIcon />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64">
        <PopoverTitle>{label}</PopoverTitle>
        <PopoverDescription>{text}</PopoverDescription>
      </PopoverContent>
    </Popover>
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
