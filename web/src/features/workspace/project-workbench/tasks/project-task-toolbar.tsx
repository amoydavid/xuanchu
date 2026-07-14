import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import {
  CalendarIcon,
  ChevronDownIcon,
  PlusIcon,
  SlidersHorizontalIcon,
  XIcon,
} from "lucide-react"
import { parseISO } from "date-fns"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import type { TaskFilter } from "@/features/workspace/project-readonly/project-filter"
import { activeFilterEntries } from "@/features/workspace/project-readonly/project-filter"
import { cn } from "@/lib/utils"
import { formatLocalDate } from "../shared/date-boundary"
import { i18n } from "@/i18n"

type ProjectTaskToolbarProps = {
  assigneeOptions?: AssigneeFilterOption[]
  canCreateTask: boolean
  filter: TaskFilter
  onCreateTask: () => void
  onCreateRecurringTask?: () => void
  onOpenRecurringTasks?: () => void
  recurringTaskCount?: number
  toParams: { workspaceSlug: string; projectSlug: string }
  // navigateTo 决定筛选 search 写入到哪个项目子页面路由。
  // Tasks 子页面传 /workspaces/$workspaceSlug/projects/$projectSlug/tasks；
  // 默认项目根路由以兼容历史用法。
  navigateTo?: string
}

export type AssigneeFilterOption = {
  email?: string | null
  id: string
  label: string
  name?: string
}

const STATUS_OPTIONS = ["pending", "completed", "waiting", "deleted"]
const PRIORITY_OPTIONS = ["H", "M", "L"]
const SORT_OPTIONS = [
  { label: "创建顺序", value: "entry" },
  { label: "下一步优先", value: "next" },
  { label: "截止日期", value: "due" },
  { label: "暂缓到", value: "wait" },
  { label: "开始时间", value: "start" },
  { label: "完成时间", value: "completed" },
]

const FILTER_CONTROL_CLASS =
  "!h-9 rounded-md border-input bg-background text-sm shadow-none"
const FILTER_BUTTON_CLASS = cn(FILTER_CONTROL_CLASS, "font-normal")

const FILTER_LABELS: Record<keyof TaskFilter, string> = {
  assignee: "负责人",
  assignee_empty: "负责人",
  due_after: "到期不早于",
  due_before: "到期不晚于",
  due_empty: "截止日期",
  priority: "优先级",
  q: "搜索",
  query: "查询",
  scheduled_before: "计划开始早于",
  sort: "排序",
  status: "状态",
  tags: "标签",
  task_type: "任务类型",
  until_before: "有效至早于",
  wait_before: "暂缓到早于",
}

export function ProjectTaskToolbar({
  assigneeOptions = [],
  canCreateTask,
  filter,
  onCreateTask,
  onCreateRecurringTask,
  onOpenRecurringTasks,
  recurringTaskCount = 0,
  toParams,
  navigateTo = "/workspaces/$workspaceSlug/projects/$projectSlug",
}: ProjectTaskToolbarProps) {
  const navigate = useNavigate()
  const advanced = {
    assignee_empty: filter.assignee_empty ?? "",
    due_empty: filter.due_empty ?? "",
    query: filter.query ?? "",
    scheduled_before: filter.scheduled_before ?? "",
    until_before: filter.until_before ?? "",
    wait_before: filter.wait_before ?? "",
  }
  const advancedKey = Object.values(advanced).join("\u0000")

  const setFilter = (key: keyof TaskFilter, value: string) => {
    void navigate({
      to: navigateTo,
      params: toParams,
      search: (prev) => {
        const next = { ...(prev as Record<string, string>) }
        if (value) {
          next[key] = value
        } else {
          delete next[key]
        }
        return next
      },
    })
  }

  const setFilters = (values: Partial<Record<keyof TaskFilter, string>>) => {
    void navigate({
      to: navigateTo,
      params: toParams,
      search: (prev) => {
        const next = { ...(prev as Record<string, string>) }
        for (const [key, value] of Object.entries(values)) {
          if (value) {
            next[key] = value
          } else {
            delete next[key]
          }
        }
        return next
      },
    })
  }

  const clearAll = () => {
    void navigate({
      to: navigateTo,
      params: toParams,
      search: {},
    })
  }

  return (
    <section className="space-y-3">
      <div className="grid grid-cols-2 items-center gap-2 sm:flex sm:flex-wrap">
        <DebouncedInput
          ariaLabel="搜索任务"
          className={cn(FILTER_CONTROL_CLASS, "col-span-2 w-full sm:w-56")}
          key={`q:${filter.q ?? ""}`}
          onCommit={(value) => setFilter("q", value)}
          placeholder="搜索标题、内容"
          value={filter.q ?? ""}
        />
        <Select
          onValueChange={(value) =>
            setFilter("status", value === "__all" ? "" : value)
          }
          value={filter.status ?? ""}
        >
          <SelectTrigger
            aria-label="状态"
            className={cn(FILTER_CONTROL_CLASS, "w-full sm:w-32")}
          >
            <SelectValue placeholder="状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all">全部状态</SelectItem>
            {STATUS_OPTIONS.map((status) => (
              <SelectItem key={status} value={status}>
                {taskStatusLabel(status, i18n.t)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          onValueChange={(value) =>
            setFilter("priority", value === "__all" ? "" : value)
          }
          value={filter.priority ?? ""}
        >
          <SelectTrigger
            aria-label="优先级"
            className={cn(FILTER_CONTROL_CLASS, "w-full sm:w-28")}
          >
            <SelectValue placeholder="优先级" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all">全部优先级</SelectItem>
            {PRIORITY_OPTIONS.map((priority) => (
              <SelectItem key={priority} value={priority}>
                {priority}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          onValueChange={(value) =>
            setFilter("task_type", value === "all" ? "" : value)
          }
          value={filter.task_type ?? "all"}
        >
          <SelectTrigger
            aria-label={i18n.t("taskCreate.typeLabel")}
            className={cn(FILTER_CONTROL_CLASS, "w-full sm:w-32")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{i18n.t("myTasks.allTaskTypes")}</SelectItem>
            <SelectItem value="normal">
              {i18n.t("taskSeries.mode.normal")}
            </SelectItem>
            <SelectItem value="occurrence">
              {i18n.t("taskSeries.mode.recurring")}
            </SelectItem>
          </SelectContent>
        </Select>
        <AssigneeFilterMenu
          options={assigneeOptions}
          selected={assigneeValues(filter.assignee)}
          onChange={(values) =>
            setFilters({
              assignee: values.join(","),
              assignee_empty: "",
            })
          }
        />
        <DebouncedInput
          ariaLabel="标签"
          className={cn(FILTER_CONTROL_CLASS, "w-full sm:w-32")}
          key={`tags:${filter.tags ?? ""}`}
          onCommit={(value) => setFilter("tags", value)}
          placeholder="标签"
          value={filter.tags ?? ""}
        />
        <DateInput
          ariaLabel="到期不早于"
          className="w-full sm:w-40"
          onCommit={(value) => setFilter("due_after", value)}
          value={filter.due_after ?? ""}
        />
        <DateInput
          ariaLabel="到期不晚于"
          className="w-full sm:w-40"
          onCommit={(value) => setFilter("due_before", value)}
          value={filter.due_before ?? ""}
        />
        <Select
          onValueChange={(value) =>
            setFilter("sort", value === "__default" ? "" : value)
          }
          value={filter.sort ?? ""}
        >
          <SelectTrigger
            aria-label="排序"
            className={cn(FILTER_CONTROL_CLASS, "w-full sm:w-36")}
          >
            <SelectValue placeholder="排序" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__default">默认排序</SelectItem>
            {SORT_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Popover>
          <PopoverTrigger asChild>
            <Button
              className={cn(
                FILTER_BUTTON_CLASS,
                "col-span-2 w-full justify-start sm:w-auto"
              )}
              size="lg"
              type="button"
              variant="outline"
            >
              <SlidersHorizontalIcon />
              更多筛选
            </Button>
          </PopoverTrigger>
          <PopoverContent align="start" className="w-80">
            <AdvancedFilterPanel
              initialValue={advanced}
              key={advancedKey}
              onApply={setFilters}
            />
          </PopoverContent>
        </Popover>
        {onOpenRecurringTasks ? (
          <Button
            onClick={onOpenRecurringTasks}
            size="lg"
            type="button"
            variant="outline"
          >
            {recurringTaskCount > 0
              ? i18n.t("taskSeries.count", { count: recurringTaskCount })
              : i18n.t("taskSeries.title")}
          </Button>
        ) : null}
        {canCreateTask ? (
          <div className="col-span-2 flex min-w-0 sm:ml-auto">
            <Button
              className={cn(
                "min-w-0 flex-1 rounded-md sm:flex-none",
                onCreateRecurringTask && "rounded-r-none"
              )}
              onClick={onCreateTask}
              size="lg"
              type="button"
            >
              <PlusIcon />
              {i18n.t("taskSeries.create.title")}
            </Button>
            {onCreateRecurringTask ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    aria-label={i18n.t("taskSeries.create.menuAria")}
                    className="rounded-l-none border-l border-l-primary-foreground/25 px-2"
                    size="lg"
                    type="button"
                  >
                    <ChevronDownIcon />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={onCreateTask}>
                    {i18n.t("taskSeries.create.normalAction")}
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={onCreateRecurringTask}>
                    {i18n.t("taskSeries.create.recurringAction")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </div>
        ) : null}
      </div>
      {activeFilterEntries(filter).length > 0 ? (
        <div className="flex flex-wrap items-center gap-2">
          {activeFilterEntries(filter).map(([key, value]) => (
            <Button
              className="h-7 gap-1 rounded-full px-2 text-xs"
              key={key}
              onClick={() => setFilter(key, "")}
              type="button"
              variant="secondary"
            >
              {`${FILTER_LABELS[key]}=${filterValueLabel(
                key,
                value,
                assigneeOptions
              )}`}
              <XIcon className="size-3" />
            </Button>
          ))}
          <Badge
            className="cursor-pointer"
            onClick={clearAll}
            variant="outline"
          >
            清除全部
          </Badge>
        </div>
      ) : null}
    </section>
  )
}

function AdvancedFilterPanel({
  initialValue,
  onApply,
}: {
  initialValue: Partial<Record<keyof TaskFilter, string>>
  onApply: (values: Partial<Record<keyof TaskFilter, string>>) => void
}) {
  const [draft, setDraft] = useState(initialValue)
  return (
    <div className="space-y-3">
      <DebouncedInput
        ariaLabel="原始查询"
        className={cn(FILTER_CONTROL_CLASS, "w-full")}
        onCommit={(value) =>
          setDraft((current) => ({ ...current, query: value }))
        }
        placeholder="如 annotations:blocked"
        value={draft.query ?? ""}
      />
      <label className="flex items-center gap-2 text-sm">
        <Checkbox
          aria-label="未分配"
          checked={draft.assignee_empty === "true"}
          onCheckedChange={(checked) =>
            setDraft((current) => ({
              ...current,
              assignee_empty: checked ? "true" : "",
            }))
          }
        />
        未分配
      </label>
      <label className="flex items-center gap-2 text-sm">
        <Checkbox
          aria-label="无截止日期"
          checked={draft.due_empty === "true"}
          onCheckedChange={(checked) =>
            setDraft((current) => ({
              ...current,
              due_empty: checked ? "true" : "",
            }))
          }
        />
        无截止日期
      </label>
      <DateInput
        ariaLabel="暂缓到早于"
        className="w-full"
        onCommit={(value) =>
          setDraft((current) => ({ ...current, wait_before: value }))
        }
        value={draft.wait_before ?? ""}
      />
      <DateInput
        ariaLabel="计划开始早于"
        className="w-full"
        onCommit={(value) =>
          setDraft((current) => ({ ...current, scheduled_before: value }))
        }
        value={draft.scheduled_before ?? ""}
      />
      <DateInput
        ariaLabel="有效至早于"
        className="w-full"
        onCommit={(value) =>
          setDraft((current) => ({ ...current, until_before: value }))
        }
        value={draft.until_before ?? ""}
      />
      <Button
        className="w-full"
        onClick={() => onApply(draft)}
        size="sm"
        type="button"
      >
        应用筛选
      </Button>
    </div>
  )
}

function AssigneeFilterMenu({
  onChange,
  options,
  selected,
}: {
  onChange: (values: string[]) => void
  options: AssigneeFilterOption[]
  selected: string[]
}) {
  const [query, setQuery] = useState("")
  const selectedSet = new Set(selected)
  const selectedLabels = assigneeLabelList(selected, options)
  const buttonLabel =
    selectedLabels.length > 0 ? selectedLabels.join("、") : "负责人"
  const filteredOptions = assigneeFilterOptions(options, query)

  const toggle = (id: string) => {
    const next = selectedSet.has(id)
      ? selected.filter((item) => item !== id)
      : [...selected, id]
    onChange(next)
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          aria-label="负责人"
          className={cn(
            FILTER_BUTTON_CLASS,
            "w-full min-w-0 justify-between gap-2 px-3 sm:w-36"
          )}
          type="button"
          variant="outline"
        >
          <span className="max-w-36 truncate">{buttonLabel}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64 p-2">
        <div className="space-y-2">
          <Input
            aria-label="搜索负责人"
            className="h-8 rounded-md bg-background"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索负责人"
            value={query}
          />
          {options.length === 0 ? (
            <div className="px-2 py-6 text-center text-sm text-muted-foreground">
              暂无负责人
            </div>
          ) : filteredOptions.length === 0 ? (
            <div className="px-2 py-6 text-center text-sm text-muted-foreground">
              暂无匹配负责人
            </div>
          ) : (
            <div className="max-h-72 space-y-1 overflow-y-auto">
              {filteredOptions.map((option) => {
                const checked = selectedSet.has(option.id)
                const description = option.email || option.name || option.id
                return (
                  <label
                    className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted"
                    key={option.id}
                  >
                    <Checkbox
                      aria-label={`${option.label} ${description}`}
                      checked={checked}
                      onCheckedChange={() => toggle(option.id)}
                    />
                    <span className="min-w-0">
                      <span className="block truncate font-medium">
                        {option.label}
                      </span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {description}
                      </span>
                    </span>
                  </label>
                )
              })}
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

function filterValueLabel(
  key: keyof TaskFilter,
  value: string,
  assigneeOptions: AssigneeFilterOption[]
): string {
  if (key === "assignee_empty" && value) {
    return "未分配"
  }
  if (key === "assignee") {
    const labels = assigneeLabelList(assigneeValues(value), assigneeOptions)
    return labels.length > 0 ? labels.join("、") : value
  }
  if (key === "due_empty" && value) {
    return "无"
  }
  if (key === "status") {
    return taskStatusLabel(value, i18n.t)
  }
  const sort = SORT_OPTIONS.find((option) => option.value === value)
  if (key === "sort" && sort) {
    return sort.label
  }
  return value
}

function assigneeValues(value: string | undefined): string[] {
  return (value ?? "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
}

function assigneeLabelList(
  values: string[],
  options: AssigneeFilterOption[]
): string[] {
  return values.map((value) => {
    const option = options.find((item) => item.id === value)
    return option?.label || value
  })
}

function assigneeFilterOptions(
  options: AssigneeFilterOption[],
  query: string
): AssigneeFilterOption[] {
  const keyword = query.trim().toLowerCase()
  if (!keyword) {
    return options
  }
  return options.filter((option) =>
    [option.label, option.name, option.email, option.id]
      .filter(Boolean)
      .some((value) => value?.toLowerCase().includes(keyword))
  )
}

function DebouncedInput({
  ariaLabel,
  className,
  onCommit,
  placeholder,
  value,
}: {
  ariaLabel: string
  className?: string
  onCommit: (value: string) => void
  placeholder?: string
  value: string
}) {
  const [local, setLocal] = useState(value)
  const commit = () => {
    if (local !== value) {
      onCommit(local.trim())
    }
  }
  return (
    <Input
      aria-label={ariaLabel}
      className={className}
      onBlur={commit}
      onChange={(event) => setLocal(event.target.value)}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          commit()
        }
      }}
      placeholder={placeholder}
      value={local}
    />
  )
}

function DateInput({
  ariaLabel,
  className,
  onCommit,
  value,
}: {
  ariaLabel: string
  className?: string
  onCommit: (value: string) => void
  value: string
}) {
  // shadcn 风格日期选择器：Popover + Calendar（react-day-picker）。
  // value 是 URL 里的 YYYY-MM-DD 字符串；与 Date 互转用 date-fns。
  // 触发器用 outline Button，高度与 Select/Input 等 h-9 控件一致。
  const [open, setOpen] = useState(false)
  const selected = parseISOOptional(value)
  const commit = (next: Date | undefined) => {
    onCommit(next ? formatLocalDate(next) : "")
    setOpen(false)
  }
  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button
          aria-label={ariaLabel}
          className={cn(FILTER_BUTTON_CLASS, "justify-start", className)}
          data-empty={!selected}
          size="lg"
          type="button"
          variant="outline"
        >
          <CalendarIcon />
          {selected ? (
            formatLocalDate(selected)
          ) : (
            <span className="text-muted-foreground">{ariaLabel}</span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto p-0">
        <Calendar
          captionLayout="dropdown"
          defaultMonth={selected ?? undefined}
          mode="single"
          onSelect={commit}
          selected={selected ?? undefined}
        />
        {selected ? (
          <div className="border-t p-2">
            <Button
              className="w-full justify-start rounded-md"
              onClick={() => commit(undefined)}
              size="xs"
              type="button"
              variant="ghost"
            >
              <XIcon data-icon="inline-start" />
              清除
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}

// parseISOOptional：把 YYYY-MM-DD 解析为本地 Date；空串或非法返回 undefined。
function parseISOOptional(value: string): Date | undefined {
  if (!value) return undefined
  const parsed = parseISO(value)
  return Number.isNaN(parsed.getTime()) ? undefined : parsed
}
