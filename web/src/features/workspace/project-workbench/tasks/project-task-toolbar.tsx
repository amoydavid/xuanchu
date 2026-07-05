import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { PlusIcon, SlidersHorizontalIcon, XIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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
import { i18n } from "@/i18n"

type ProjectTaskToolbarProps = {
  canCreateTask: boolean
  filter: TaskFilter
  onCreateTask: () => void
  toParams: { workspaceSlug: string; projectSlug: string }
}

const STATUS_OPTIONS = ["pending", "completed", "waiting", "recurring"]
const PRIORITY_OPTIONS = ["H", "M", "L"]
const SORT_OPTIONS = [
  { label: "创建顺序", value: "entry" },
  { label: "下一步优先", value: "next" },
  { label: "截止日期", value: "due" },
  { label: "暂缓到", value: "wait" },
  { label: "开始时间", value: "start" },
  { label: "完成时间", value: "completed" },
]

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
  until_before: "有效至早于",
  wait_before: "暂缓到早于",
}

export function ProjectTaskToolbar({
  canCreateTask,
  filter,
  onCreateTask,
  toParams,
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
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
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
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
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
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: toParams,
      search: {},
    })
  }

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <DebouncedInput
          ariaLabel="搜索任务"
          className="h-9 w-56"
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
          <SelectTrigger aria-label="状态" className="h-9 w-32">
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
          <SelectTrigger aria-label="优先级" className="h-9 w-28">
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
        <DebouncedInput
          ariaLabel="负责人"
          className="h-9 w-32"
          key={`assignee:${filter.assignee ?? ""}`}
          onCommit={(value) => setFilter("assignee", value)}
          placeholder="负责人"
          value={filter.assignee ?? ""}
        />
        <DebouncedInput
          ariaLabel="标签"
          className="h-9 w-32"
          key={`tags:${filter.tags ?? ""}`}
          onCommit={(value) => setFilter("tags", value)}
          placeholder="标签"
          value={filter.tags ?? ""}
        />
        <DateInput
          ariaLabel="到期不早于"
          onCommit={(value) => setFilter("due_after", value)}
          value={filter.due_after ?? ""}
        />
        <DateInput
          ariaLabel="到期不晚于"
          onCommit={(value) => setFilter("due_before", value)}
          value={filter.due_before ?? ""}
        />
        <Select
          onValueChange={(value) =>
            setFilter("sort", value === "__default" ? "" : value)
          }
          value={filter.sort ?? ""}
        >
          <SelectTrigger aria-label="排序" className="h-9 w-36">
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
            <Button size="sm" type="button" variant="outline">
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
        {canCreateTask ? (
          <Button
            className="ml-auto"
            onClick={onCreateTask}
            size="sm"
            type="button"
          >
            <PlusIcon />
            新建任务
          </Button>
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
              {FILTER_LABELS[key]}={filterValueLabel(key, value)}
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
        className="h-9"
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
        onCommit={(value) =>
          setDraft((current) => ({ ...current, wait_before: value }))
        }
        value={draft.wait_before ?? ""}
      />
      <DateInput
        ariaLabel="计划开始早于"
        onCommit={(value) =>
          setDraft((current) => ({ ...current, scheduled_before: value }))
        }
        value={draft.scheduled_before ?? ""}
      />
      <DateInput
        ariaLabel="有效至早于"
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

function filterValueLabel(key: keyof TaskFilter, value: string): string {
  if (key === "assignee_empty" && value) {
    return "未分配"
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
  onCommit,
  value,
}: {
  ariaLabel: string
  onCommit: (value: string) => void
  value: string
}) {
  return (
    <label className="flex h-9 items-center gap-2 rounded-md border bg-background px-2 text-sm shadow-xs">
      <span className="text-xs whitespace-nowrap text-muted-foreground">
        {ariaLabel}
      </span>
      <Input
        aria-label={ariaLabel}
        className="h-7 w-32 border-0 bg-transparent p-0 shadow-none focus-visible:ring-0"
        onChange={(event) => onCommit(event.target.value)}
        type="date"
        value={value}
      />
    </label>
  )
}
