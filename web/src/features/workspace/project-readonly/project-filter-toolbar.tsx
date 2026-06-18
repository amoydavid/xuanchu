import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import {
  activeFilterEntries,
  emptyFilter,
  type TaskFilter,
} from "./project-filter"

type ProjectFilterToolbarProps = {
  filter: TaskFilter
  // toParams 是当前项目详情路由的参数（workspaceSlug/projectSlug），用于 navigate。
  toParams: { workspaceSlug: string; projectSlug: string }
}

const STATUS_OPTIONS = ["pending", "completed", "waiting", "recurring"]
const PRIORITY_OPTIONS = ["H", "M", "L"]

// FILTER_LABELS 把过滤 key 映射为展示用的简短标签（活跃 chips 用）。
const FILTER_LABELS: Record<keyof TaskFilter, string> = {
  status: "status",
  priority: "priority",
  assignee: "assignee",
  due_after: "due after",
  due_before: "due before",
  tags: "tags",
  q: "q",
}

export function ProjectFilterToolbar({
  filter,
  toParams,
}: ProjectFilterToolbarProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const to = "/workspaces/$workspaceSlug/projects/$projectSlug" as const

  // setFilter 更新单个过滤字段到 URL（其余字段保留）。
  const setFilter = (key: keyof TaskFilter, value: string) => {
    void navigate({
      to,
      params: toParams,
      search: (prev) => {
        const next: Record<string, string> = { ...(prev as Record<string, string>) }
        if (value) {
          next[key] = value
        } else {
          delete next[key]
        }
        return next
      },
    })
  }

  const clearAll = () => {
    void navigate({ to, params: toParams, search: {} })
  }

  const active = activeFilterEntries(filter)

  return (
    <section className="space-y-2 border bg-card p-3">
      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={filter.status ?? ""}
          onValueChange={(v) => setFilter("status", v === "__all" ? "" : v)}
        >
          <SelectTrigger className="h-8 w-32">
            <SelectValue placeholder={t("common.status")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all">{t("common.status")}</SelectItem>
            {STATUS_OPTIONS.map((s) => (
              <SelectItem key={s} value={s}>
                {s}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filter.priority ?? ""}
          onValueChange={(v) => setFilter("priority", v === "__all" ? "" : v)}
        >
          <SelectTrigger className="h-8 w-28">
            <SelectValue placeholder={t("projectReadonly.priority")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all">{t("projectReadonly.priority")}</SelectItem>
            {PRIORITY_OPTIONS.map((p) => (
              <SelectItem key={p} value={p}>
                {p}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <DebouncedInput
          className="h-8 w-32"
          key={`assignee:${filter.assignee ?? ""}`}
          onCommit={(v) => setFilter("assignee", v)}
          placeholder={t("projectReadonly.assignee")}
          value={filter.assignee ?? ""}
        />

        <DebouncedInput
          className="h-8 w-36"
          key={`q:${filter.q ?? ""}`}
          onCommit={(v) => setFilter("q", v)}
          placeholder={t("common.search")}
          value={filter.q ?? ""}
        />
      </div>

      {!emptyFilter(filter) ? (
        <div className="flex flex-wrap items-center gap-2">
          {active.map(([key, value]) => (
            <button
              className="rounded-full bg-primary/10 px-2 py-0.5 text-xs text-primary"
              key={key}
              onClick={() => setFilter(key, "")}
              type="button"
            >
              {FILTER_LABELS[key]}={value} ✕
            </button>
          ))}
          <button
            className="text-xs text-muted-foreground underline"
            onClick={clearAll}
            type="button"
          >
            {t("common.clear")}
          </button>
        </div>
      ) : null}
    </section>
  )
}

// DebouncedInput 是文本输入框，本地维护输入态，在失焦或回车时才把值提交到 URL，
// 避免用户打字时每个按键都触发一次 navigate + 请求。
// 外部 value 变化（如 URL 同步、清除全部）由调用方 key 变化重建输入框。
function DebouncedInput({
  className,
  onCommit,
  placeholder,
  value,
}: {
  className?: string
  onCommit: (value: string) => void
  placeholder?: string
  value: string
}) {
  const [local, setLocal] = useState(value)
  const commit = () => {
    if (local !== value) {
      onCommit(local)
    }
  }
  return (
    <Input
      className={className}
      onBlur={commit}
      onChange={(e) => setLocal(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          commit()
        }
      }}
      placeholder={placeholder}
      value={local}
    />
  )
}
