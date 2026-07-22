import { keepPreviousData, useQuery } from "@tanstack/react-query"
import {
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  KeyRound,
  Search,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api"

import {
  listProjectTemplateAutomationCandidates,
  listProjectTemplateConfigCandidates,
  listProjectTemplateSeriesCandidates,
  listProjectTemplateTaskCandidates,
  resolveProjectTemplateCandidateSelection,
  type AutomationCandidate,
  type AutomationCandidateFilter,
  type CandidateKind,
  type ConfigCandidate,
  type ConfigCandidateFilter,
  type Page,
  type ResolveCandidateSelectionInput,
  type SeriesCandidate,
  type SeriesCandidateFilter,
  type TaskCandidate,
  type TaskCandidateFilter,
} from "../api/project-template-api"

export type CandidateSummary = {
  label: string
  ref: string
  secondary?: string
  secret?: boolean
}

export type CandidateSelectionStore = Record<
  CandidateKind,
  Map<string, CandidateSummary>
>

type Candidate =
  | TaskCandidate
  | SeriesCandidate
  | ConfigCandidate
  | AutomationCandidate

type CandidatePickerProps = {
  kind: CandidateKind
  lockedRefs?: Set<string>
  onChange: (next: Map<string, CandidateSummary>) => void
  onLimitError: (message?: string) => void
  onSummariesChange: (next: Map<string, CandidateSummary>) => void
  refreshKey: number
  selected: Map<string, CandidateSummary>
  sourceProjectRef: string
  workspaceSlug: string
}

const PAGE_SIZE = 50

const labels: Record<CandidateKind, { search: string; empty: string }> = {
  task: { search: "搜索任务", empty: "没有匹配的任务" },
  series: { search: "搜索循环任务", empty: "没有匹配的循环任务" },
  config: { search: "搜索配置", empty: "没有匹配的配置" },
  automation: { search: "搜索自动化", empty: "没有匹配的自动化" },
}

export function CandidatePicker({
  kind,
  lockedRefs = new Set(),
  onChange,
  onLimitError,
  onSummariesChange,
  refreshKey,
  selected,
  sourceProjectRef,
  workspaceSlug,
}: CandidatePickerProps) {
  const [q, setQ] = useState("")
  const [status, setStatus] = useState("all")
  const [priority, setPriority] = useState("all")
  const [assignees, setAssignees] = useState("")
  const [tags, setTags] = useState("")
  const [dueAfter, setDueAfter] = useState("")
  const [dueBefore, setDueBefore] = useState("")
  const [taskQuery, setTaskQuery] = useState("")
  const [sort, setSort] = useState(kind === "task" ? "urgency" : "next")
  const [mode, setMode] = useState("all")
  const [triggerType, setTriggerType] = useState("all")
  const [offset, setOffset] = useState(0)
  const [resolving, setResolving] = useState(false)
  const refreshedKeyRef = useRef<number | undefined>(undefined)
  const filter = useMemo(() => {
    const common = { q: q || undefined, status }
    switch (kind) {
      case "task":
        return {
          kind,
          task: {
            ...common,
            priority,
            assignees: splitFilterList(assignees),
            tags: splitFilterList(tags),
            due_after: dueAfter || undefined,
            due_before: dueBefore || undefined,
            query: taskQuery || undefined,
            sort,
          },
        } satisfies ResolveCandidateSelectionInput
      case "series":
        return {
          kind,
          series: {
            ...common,
            assignee: assignees.trim() || undefined,
            sort,
          },
        } satisfies ResolveCandidateSelectionInput
      case "config":
        return {
          kind,
          config: { q: common.q, mode },
        } satisfies ResolveCandidateSelectionInput
      case "automation":
        return {
          kind,
          automation: { ...common, trigger_type: triggerType },
        } satisfies ResolveCandidateSelectionInput
    }
  }, [
    assignees,
    dueAfter,
    dueBefore,
    kind,
    mode,
    priority,
    q,
    sort,
    status,
    tags,
    taskQuery,
    triggerType,
  ])
  const query = useQuery({
    queryKey: [
      "project-template-candidates",
      workspaceSlug,
      sourceProjectRef,
      kind,
      filter,
      offset,
      refreshKey,
    ],
    queryFn: () =>
      listCandidates(kind, workspaceSlug, sourceProjectRef, filter, {
        limit: PAGE_SIZE,
        offset,
      }),
    placeholderData: keepPreviousData,
  })

  const page = query.data ?? { items: [], total: 0, limit: PAGE_SIZE, offset }
  const pageRefs = page.items.map((item) => item.ref)
  const selectedOnPage = pageRefs.filter((ref) => selected.has(ref)).length

  useEffect(() => {
    const next = new Map(selected)
    let changed = false
    for (const candidate of page.items) {
      const current = next.get(candidate.ref)
      if (!current) continue
      const summary = summarizeCandidate(candidate, kind)
      if (!sameSummary(current, summary)) {
        next.set(candidate.ref, summary)
        changed = true
      }
    }
    if (changed) onSummariesChange(next)
  }, [kind, onSummariesChange, page.items, selected])

  useEffect(() => {
    if (refreshKey === 0 || refreshedKeyRef.current === refreshKey) return
    const refs = [...selected.keys()]
    if (refs.length === 0) {
      refreshedKeyRef.current = refreshKey
      return
    }
    let cancelled = false
    void Promise.all(
      chunk(refs, 100).map((selectedRefs) =>
        listCandidates(
          kind,
          workspaceSlug,
          sourceProjectRef,
          emptyResolveInput(kind),
          {
            limit: selectedRefs.length,
            offset: 0,
            refs: selectedRefs,
          }
        )
      )
    ).then((pages) => {
      if (cancelled) return
      const next = new Map(selected)
      for (const candidate of pages.flatMap((page) => page.items)) {
        next.set(candidate.ref, summarizeCandidate(candidate, kind))
      }
      refreshedKeyRef.current = refreshKey
      if (!sameSelectionMap(next, selected)) onSummariesChange(next)
    })
    return () => {
      cancelled = true
    }
  }, [
    kind,
    onSummariesChange,
    refreshKey,
    selected,
    sourceProjectRef,
    workspaceSlug,
  ])

  function toggle(candidate: Candidate, checked: boolean) {
	if (!checked && lockedRefs.has(candidate.ref)) return
    const next = new Map(selected)
    if (checked) next.set(candidate.ref, summarizeCandidate(candidate, kind))
    else next.delete(candidate.ref)
    onChange(next)
  }

  function togglePage(checked: boolean) {
    const next = new Map(selected)
    for (const candidate of page.items) {
      if (checked) next.set(candidate.ref, summarizeCandidate(candidate, kind))
      else if (!lockedRefs.has(candidate.ref)) next.delete(candidate.ref)
    }
    onChange(next)
  }

  async function resolveSelection(action: "add" | "remove") {
    setResolving(true)
    onLimitError(undefined)
    try {
      const result = await resolveProjectTemplateCandidateSelection(
        workspaceSlug,
        sourceProjectRef,
        filter
      )
      const next = new Map(selected)
      for (const ref of result.refs) {
        if (action === "remove") {
		  if (!lockedRefs.has(ref)) next.delete(ref)
          continue
        }
        const current = page.items.find((item) => item.ref === ref)
        next.set(
          ref,
          current
            ? summarizeCandidate(current, kind)
            : { ref, label: ref, secondary: "已通过服务端解析选择" }
        )
      }
      onChange(next)
    } catch (error) {
      onLimitError(
        error instanceof ApiError &&
          error.code === "project_template_candidate_limit_exceeded"
          ? "匹配结果超过模板快照上限，请先筛选再选择。"
          : "无法解析当前筛选结果，请重试。"
      )
    } finally {
      setResolving(false)
    }
  }

  if (query.isPending) {
    return (
      <div className="space-y-2" role="status" aria-label="正在加载候选内容">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    )
  }

  if (query.isError) {
    return (
      <Alert variant="destructive">
        <AlertCircle />
        <AlertDescription>加载候选内容失败，请重试。</AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="space-y-3">
      <div className="space-y-2 border bg-muted/20 p-2">
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative">
            <Search className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground" />
            <Input
              aria-label={labels[kind].search}
              className="pl-8"
              onChange={(event) => {
                setQ(event.target.value)
                setOffset(0)
              }}
              placeholder={labels[kind].search}
              value={q}
            />
          </div>
          <span className="self-center text-right text-xs text-muted-foreground tabular-nums sm:ml-auto">
            {page.total} 条
          </span>
        </div>
        {kind === "task" ? (
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-4">
            <CandidateSelect
              label="任务状态筛选"
              onChange={(value) => updateFilter(value, setStatus, setOffset)}
              value={status}
              options={[
                ["all", "全部状态"],
                ["pending", "待处理"],
                ["waiting", "等待中"],
                ["completed", "已完成"],
              ]}
            />
            <CandidateSelect
              label="任务优先级筛选"
              onChange={(value) => updateFilter(value, setPriority, setOffset)}
              value={priority}
              options={[
                ["all", "全部优先级"],
                ["H", "高优先级"],
                ["M", "中优先级"],
                ["L", "低优先级"],
              ]}
            />
            <CandidateInput
              label="任务负责人筛选"
              onChange={(value) => updateFilter(value, setAssignees, setOffset)}
              placeholder="名称或 ID，逗号分隔"
              value={assignees}
            />
            <CandidateInput
              label="任务标签筛选"
              onChange={(value) => updateFilter(value, setTags, setOffset)}
              placeholder="标签，逗号分隔"
              value={tags}
            />
            <CandidateInput
              label="任务截止日期从"
              onChange={(value) => updateFilter(value, setDueAfter, setOffset)}
              type="date"
              value={dueAfter}
            />
            <CandidateInput
              label="任务截止日期至"
              onChange={(value) => updateFilter(value, setDueBefore, setOffset)}
              type="date"
              value={dueBefore}
            />
            <CandidateInput
              label="任务查询表达式"
              onChange={(value) => updateFilter(value, setTaskQuery, setOffset)}
              placeholder="现有 task query 语法"
              value={taskQuery}
            />
            <CandidateSelect
              label="任务排序"
              onChange={(value) => updateFilter(value, setSort, setOffset)}
              value={sort}
              options={[
                ["urgency", "紧急度"],
                ["entry", "创建时间"],
                ["due", "截止时间"],
                ["wait", "等待时间"],
                ["completed", "完成时间"],
              ]}
            />
          </div>
        ) : kind === "series" ? (
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <CandidateSelect
              label="循环任务状态筛选"
              onChange={(value) => updateFilter(value, setStatus, setOffset)}
              value={status}
              options={[
                ["all", "全部状态"],
                ["active", "进行中"],
                ["ended", "已结束"],
                ["stopped", "已停止"],
              ]}
            />
            <CandidateInput
              label="循环任务负责人筛选"
              onChange={(value) => updateFilter(value, setAssignees, setOffset)}
              placeholder="名称或 ID"
              value={assignees}
            />
            <CandidateSelect
              label="循环任务排序"
              onChange={(value) => updateFilter(value, setSort, setOffset)}
              value={sort}
              options={[
                ["next", "下次发生时间"],
                ["title", "标题"],
                ["modified", "修改时间"],
              ]}
            />
          </div>
        ) : kind === "config" ? (
          <div className="grid grid-cols-1 gap-2 sm:max-w-xs">
            <CandidateSelect
              label="配置类型筛选"
              onChange={(value) => updateFilter(value, setMode, setOffset)}
              value={mode}
              options={[
                ["all", "全部配置"],
                ["literal", "普通值"],
                ["secret", "机密值"],
              ]}
            />
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <CandidateSelect
              label="自动化状态筛选"
              onChange={(value) => updateFilter(value, setStatus, setOffset)}
              value={status}
              options={[
                ["all", "全部状态"],
                ["enabled", "已启用"],
                ["disabled", "已停用"],
              ]}
            />
            <CandidateSelect
              label="自动化触发类型筛选"
              onChange={(value) =>
                updateFilter(value, setTriggerType, setOffset)
              }
              value={triggerType}
              options={[
                ["all", "全部触发类型"],
                ["schedule", "定时触发"],
                ["event", "事件触发"],
              ]}
            />
          </div>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-2 border-b pb-2">
        <label className="flex items-center gap-2 text-xs font-medium">
          <Checkbox
            aria-label={`选择本页 ${page.items.length} 项`}
            checked={
              page.items.length > 0 && selectedOnPage === page.items.length
                ? true
                : selectedOnPage > 0
                  ? "indeterminate"
                  : false
            }
            onCheckedChange={(checked) => togglePage(checked === true)}
          />
          选择本页 {page.items.length} 项
        </label>
        <Button
          disabled={resolving || page.total === 0}
          onClick={() => void resolveSelection("add")}
          size="sm"
          variant="outline"
        >
          选择全部 {page.total} 条匹配结果
        </Button>
        <Button
          disabled={resolving || selected.size === 0}
          onClick={() => void resolveSelection("remove")}
          size="sm"
          variant="ghost"
        >
          清除当前筛选结果的选择
        </Button>
      </div>

      {page.items.length === 0 ? (
        <div className="border border-dashed p-8 text-center text-xs text-muted-foreground">
          {labels[kind].empty}
        </div>
      ) : (
        <ul className="divide-y border">
          {page.items.map((candidate) => {
            const summary = summarizeCandidate(candidate, kind)
            return (
              <li className="flex items-start gap-3 p-3" key={candidate.ref}>
                <Checkbox
                  aria-label={summary.label}
                  checked={selected.has(candidate.ref)}
				  disabled={lockedRefs.has(candidate.ref)}
                  onCheckedChange={(checked) =>
                    toggle(candidate, checked === true)
                  }
                />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {summary.label}
                    </span>
                    {summary.secret ? (
                      <Badge variant="outline">
                        <KeyRound /> 机密
                      </Badge>
                    ) : null}
                    {candidate.warning_count > 0 ? (
                      <Badge variant="outline">
                        {candidate.warning_count} 个提醒
                      </Badge>
                    ) : null}
                  </div>
                  <div className="mt-1 text-xs text-muted-foreground">
                    {summary.secondary}
                  </div>
                  {summary.secret ? (
                    <div className="mt-1 text-[11px] text-muted-foreground">
                      机密值不会显示或复制
                    </div>
                  ) : null}
				  {lockedRefs.has(candidate.ref) ? (
					  <Badge variant="outline">由自动化依赖</Badge>
				  ) : null}
                </div>
              </li>
            )
          })}
        </ul>
      )}

      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground tabular-nums">
          {page.total === 0
            ? "0 / 0"
            : `${page.offset + 1}–${Math.min(page.offset + page.items.length, page.total)} / ${page.total}`}
        </span>
        <div className="flex gap-1">
          <Button
            aria-label="上一页"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            size="icon-sm"
            variant="outline"
          >
            <ChevronLeft />
          </Button>
          <Button
            aria-label="下一页"
            disabled={offset + PAGE_SIZE >= page.total}
            onClick={() => setOffset(offset + PAGE_SIZE)}
            size="icon-sm"
            variant="outline"
          >
            <ChevronRight />
          </Button>
        </div>
      </div>
    </div>
  )
}

function CandidateInput({
  label,
  onChange,
  placeholder,
  type = "text",
  value,
}: {
  label: string
  onChange: (value: string) => void
  placeholder?: string
  type?: string
  value: string
}) {
  return (
    <Input
      aria-label={label}
      onChange={(event) => onChange(event.target.value)}
      placeholder={placeholder}
      type={type}
      value={value}
    />
  )
}

function CandidateSelect({
  label,
  onChange,
  options,
  value,
}: {
  label: string
  onChange: (value: string) => void
  options: Array<[string, string]>
  value: string
}) {
  return (
    <select
      aria-label={label}
      className="h-8 min-w-0 border bg-background px-2 text-xs"
      onChange={(event) => onChange(event.target.value)}
      value={value}
    >
      {options.map(([optionValue, optionLabel]) => (
        <option key={optionValue} value={optionValue}>
          {optionLabel}
        </option>
      ))}
    </select>
  )
}

function updateFilter(
  value: string,
  setValue: (value: string) => void,
  setOffset: (value: number) => void
) {
  setValue(value)
  setOffset(0)
}

function splitFilterList(value: string): string[] | undefined {
  const items = value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
  return items.length ? items : undefined
}

function listCandidates(
  kind: CandidateKind,
  workspaceSlug: string,
  projectRef: string,
  filter: ResolveCandidateSelectionInput,
  pagination: {
    limit: number
    offset: number
    refs?: string[]
  }
): Promise<Page<Candidate>> {
  switch (kind) {
    case "task":
      return listProjectTemplateTaskCandidates(workspaceSlug, projectRef, {
        ...(filter.task ?? ({} as TaskCandidateFilter)),
        ...pagination,
      })
    case "series":
      return listProjectTemplateSeriesCandidates(workspaceSlug, projectRef, {
        ...(filter.series ?? ({} as SeriesCandidateFilter)),
        ...pagination,
      })
    case "config":
      return listProjectTemplateConfigCandidates(workspaceSlug, projectRef, {
        ...(filter.config ?? ({} as ConfigCandidateFilter)),
        ...pagination,
      })
    case "automation":
      return listProjectTemplateAutomationCandidates(
        workspaceSlug,
        projectRef,
        {
          ...(filter.automation ?? ({} as AutomationCandidateFilter)),
          ...pagination,
        }
      )
  }
}

function chunk<T>(items: T[], size: number): T[][] {
  const chunks: T[][] = []
  for (let index = 0; index < items.length; index += size) {
    chunks.push(items.slice(index, index + size))
  }
  return chunks
}

function emptyResolveInput(
  kind: CandidateKind
): ResolveCandidateSelectionInput {
  switch (kind) {
    case "task":
      return { kind, task: {} }
    case "series":
      return { kind, series: {} }
    case "config":
      return { kind, config: {} }
    case "automation":
      return { kind, automation: {} }
  }
}

export function summarizeCandidate(
  candidate: Candidate,
  kind: CandidateKind
): CandidateSummary {
  switch (kind) {
    case "task": {
      const item = candidate as TaskCandidate
      return {
        ref: item.ref,
        label: item.title,
        secondary: `#${item.project_seq ?? "-"} · ${item.status}`,
      }
    }
    case "series": {
      const item = candidate as SeriesCandidate
      return {
        ref: item.ref,
        label: item.title,
        secondary: `${item.status} · ${item.recurrence_rule}`,
      }
    }
    case "config": {
      const item = candidate as ConfigCandidate
      return {
        ref: item.ref,
        label: item.label || item.key,
        secondary: `${item.key} · ${item.value_type}`,
        secret: item.mode === "secret",
      }
    }
    case "automation": {
      const item = candidate as AutomationCandidate
      return {
        ref: item.ref,
        label: item.name,
        secondary: `${item.enabled ? "已启用" : "已停用"} · ${item.trigger_type}`,
      }
    }
  }
}

function sameSummary(left: CandidateSummary, right: CandidateSummary) {
  return (
    left.ref === right.ref &&
    left.label === right.label &&
    left.secondary === right.secondary &&
    left.secret === right.secret
  )
}

function sameSelectionMap(
  left: Map<string, CandidateSummary>,
  right: Map<string, CandidateSummary>
) {
  if (left.size !== right.size) return false
  for (const [ref, summary] of left) {
    const other = right.get(ref)
    if (!other || !sameSummary(summary, other)) return false
  }
  return true
}
