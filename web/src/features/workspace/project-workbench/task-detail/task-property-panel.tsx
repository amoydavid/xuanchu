import {
  ChevronDownIcon,
  ChevronRightIcon,
  CircleHelpIcon,
  PlusIcon,
  XIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
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
import {
  extractUDAs,
  formatUDAValue,
} from "@/features/workspace/project-readonly/uda"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import type { ProjectWorkbenchTaskRef } from "../api/project-api"
import type { ProjectTask } from "../api/task-api"
import type { DateBoundary } from "../shared/date-boundary"
import { useModifyTaskMutation } from "../hooks/use-task-mutations"
import { InlineDatePicker } from "../shared/inline-date-picker"
import { InlineSelectEditor } from "../shared/inline-select-editor"
import { InlineTextEditor } from "../shared/inline-text-editor"
import { useEditFeedback } from "../shared/edit-feedback"
import {
  listWorkspaceTaskUDADefinitions,
  type TaskUDADefinition,
} from "../tasks/task-uda-definitions"
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
  const [udaDefinitions, setUDADefinitions] = useState<TaskUDADefinition[]>([])
  const [addedUDANames, setAddedUDANames] = useState<string[]>([])
  useEffect(() => {
    let active = true
    void listWorkspaceTaskUDADefinitions(workspaceSlug)
      .then((definitions) => {
        if (active) setUDADefinitions(definitions)
      })
      .catch(() => {
        if (active) setUDADefinitions([])
      })
    return () => {
      active = false
    }
  }, [workspaceSlug])
  const udaDefinitionByName = useMemo(
    () =>
      new Map(
        udaDefinitions.map((definition) => [definition.name, definition])
      ),
    [udaDefinitions]
  )

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
    (task.blocked_by_info && task.blocked_by_info.length > 0)
  const hasUDA = udas.length > 0 || addedUDANames.length > 0
  // 不可写且计划字段全空时，Schedule 整组隐身（避免空壳噪音）。
  const showSchedule = canWrite || hasSchedule
  const projected = task.recurrence_info?.materialization === "projected"

  return (
    <aside className="rounded-lg space-y-4 border bg-card p-4 text-sm">
      {/* Properties：高频字段，始终展示 */}
      <PropertyGroup title={t("taskDetail.groupProperties")}>
        <PropertyRow label={t("common.status")}>
          <div className="font-medium">
            {projected
              ? t("taskSeries.occurrence.projected")
              : taskStatusLabel(task.status, t)}
          </div>
        </PropertyRow>
        <PropertyRow label={t("taskDetail.urgency")}>
          <TaskUrgencyPanel taskRef={taskRef} workspaceSlug={workspaceSlug} />
        </PropertyRow>
        <PropertyRow label={t("projectReadonly.priority")}>
          <div className="space-y-1">
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
            <InheritanceHint show={projected} />
          </div>
        </PropertyRow>
        <PropertyRow label={t("projectReadonly.assignee")}>
          <div className="space-y-1">
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
            <InheritanceHint show={projected} />
          </div>
        </PropertyRow>
        <PropertyRow label={t("projectReadonly.tags")}>
          <div className="space-y-1">
            <TagPicker
              disabled={!canWrite}
              onSave={async (items) => {
                await modify.mutateAsync({ tags: items })
              }}
              projectSlug={projectSlug}
              value={task.tags ?? []}
              workspaceSlug={workspaceSlug}
            />
            <InheritanceHint show={projected} />
          </div>
        </PropertyRow>
      </PropertyGroup>

      {/* Schedule：日期/周期字段；空组在不可写时隐身 */}
      {showSchedule || task.recurrence_info ? (
        <PropertyGroup
          defaultOpen={hasSchedule || !!task.recurrence_info}
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
          {task.recurrence_info ? (
            <PropertyRow label={t("taskSeries.occurrence.originalDate")}>
              <div className="font-medium">
                {new Intl.DateTimeFormat(undefined, {
                  year: "numeric",
                  month: "2-digit",
                  day: "2-digit",
                }).format(new Date(task.recurrence_info.recurrence_at * 1000))}
              </div>
            </PropertyRow>
          ) : null}
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
        <PropertyGroup title={t("taskDetail.groupRelations")}>
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
              taskUUID={task.uuid ?? ""}
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

      {/* 计划实例尚无持久化时间，不展示会误导为实体记录的系统分组。 */}
      {task.recurrence_info?.materialization !== "projected" ? (
        <PropertyGroup defaultOpen={false} title={t("taskDetail.groupSystem")}>
          <PropertyRow label={t("projectReadonly.entry")}>
            <div className="font-medium">{formatRFCDate(task.entry)}</div>
          </PropertyRow>
          <PropertyRow label={t("projectReadonly.modified")}>
            <div className="font-medium">{formatRFCDate(task.modified)}</div>
          </PropertyRow>
        </PropertyGroup>
      ) : null}

      {/* Custom fields：已有值或可写时提供 Workspace 字段入口。 */}
      {hasUDA || (canWrite && udaDefinitions.length > 0) ? (
        <PropertyGroup title={t("taskDetail.groupCustom")}>
          {[
            ...udas,
            ...addedUDANames
              .filter((name) => !udas.some(([key]) => key === name))
              .map((name) => [name, ""] as [string, unknown]),
          ].map(([key, value]) => {
            const definition = udaDefinitionByName.get(key)
            const persisted = udas.some(([name]) => name === key)
            return (
              <PropertyRow key={key} label={definition?.label || key}>
                <div className="flex items-start gap-1">
                  <UDAFieldEditor
                    definition={definition}
                    name={key}
                    disabled={!canWrite || !definition}
                    onSave={async (nextValue) => {
                      if (nextValue === "") {
                        await modify.mutateAsync({ clear_udas: [key] })
                      } else {
                        await modify.mutateAsync({ udas: { [key]: nextValue } })
                      }
                      setAddedUDANames((current) =>
                        current.filter((name) => name !== key)
                      )
                    }}
                    value={value}
                  />
                  {canWrite && definition ? (
                    <Button
                      aria-label={t("taskCreate.removeCustomField", {
                        name: definition.label,
                      })}
                      onClick={() => {
                        if (persisted) {
                          void modify.mutateAsync({ clear_udas: [key] })
                        } else {
                          setAddedUDANames((current) =>
                            current.filter((name) => name !== key)
                          )
                        }
                      }}
                      size="icon-xs"
                      type="button"
                      variant="ghost"
                    >
                      <XIcon />
                    </Button>
                  ) : null}
                </div>
                {!definition ? (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("taskCreate.customFieldHistoryReadonly")}
                  </p>
                ) : null}
              </PropertyRow>
            )
          })}
          {canWrite ? (
            <UDAFieldAdder
              definitions={udaDefinitions.filter(
                (definition) =>
                  !udas.some(([name]) => name === definition.name) &&
                  !addedUDANames.includes(definition.name)
              )}
              onAdd={(name) =>
                setAddedUDANames((current) => [...current, name])
              }
            />
          ) : null}
        </PropertyGroup>
      ) : null}
    </aside>
  )
}

function InheritanceHint({ show }: { show: boolean }) {
  const { t } = useTranslation()
  return show ? (
    <div className="text-xs text-muted-foreground">
      {t("taskSeries.occurrence.inherited")}
    </div>
  ) : null
}

function UDAFieldAdder({
  definitions,
  onAdd,
}: {
  definitions: TaskUDADefinition[]
  onAdd: (name: string) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const visible = definitions.filter((definition) => {
    const needle = search.trim().toLocaleLowerCase()
    return (
      !needle ||
      `${definition.name} ${definition.label}`
        .toLocaleLowerCase()
        .includes(needle)
    )
  })
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button size="sm" type="button" variant="outline">
          <PlusIcon />
          {t("taskCreate.addCustomField")}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 space-y-2">
        <Input
          aria-label={t("taskCreate.searchCustomFields")}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t("taskCreate.searchCustomFields")}
          value={search}
        />
        <div className="max-h-52 space-y-1 overflow-auto">
          {visible.length === 0 ? (
            <p className="px-2 py-3 text-xs text-muted-foreground">
              {t("taskCreate.noCustomFieldsAvailable")}
            </p>
          ) : (
            visible.map((definition) => (
              <Button
                className="w-full justify-start"
                key={definition.name}
                onClick={() => {
                  onAdd(definition.name)
                  setSearch("")
                  setOpen(false)
                }}
                type="button"
                variant="ghost"
              >
                {definition.label}
              </Button>
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
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
        className="flex w-full items-center gap-1 text-xs font-medium tracking-wide text-muted-foreground uppercase"
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
  definition,
  disabled,
  name,
  onSave,
  value,
}: {
  definition?: TaskUDADefinition
  disabled: boolean
  name: string
  onSave: (value: string) => Promise<void> | void
  value: unknown
}) {
  const normalizedValue = formatUDAValue(value)
  if (!definition) {
    return <span className="font-medium">{normalizedValue || "-"}</span>
  }
  if (definition.values.length > 0) {
    return (
      <InlineSelectEditor
        ariaLabel={`UDA ${name}`}
        disabled={disabled}
        onSave={onSave}
        options={definition.values.map((option) => ({
          label: option,
          value: option,
        }))}
        placeholder={definition.defaultValue ?? "-"}
        triggerSize="sm"
        value={normalizedValue}
      />
    )
  }
  if (definition.type === "numeric" || definition.type === "date") {
    return (
      <UDAInputEditor
        disabled={disabled}
        name={name}
        onSave={onSave}
        type={definition.type === "numeric" ? "number" : "date"}
        value={
          definition.type === "date"
            ? normalizedValue.slice(0, 10)
            : normalizedValue
        }
      />
    )
  }
  return (
    <InlineTextEditor
      ariaLabel={`UDA ${name}`}
      disabled={disabled}
      emptyLabel={definition.defaultValue || "-"}
      onSave={onSave}
      placeholder={definition.defaultValue ?? undefined}
      value={normalizedValue}
    />
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

function formatRFCDate(value?: string | number | null): string {
  const unix = unixLikeToNumber(value)
  if (unix === null) return "-"
  const date = new Date(unix * 1000)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}
