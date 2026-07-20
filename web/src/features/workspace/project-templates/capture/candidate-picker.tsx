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
  type CandidateKind,
  type ConfigCandidate,
  type Page,
  type ResolveCandidateSelectionInput,
  type SeriesCandidate,
  type TaskCandidate,
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
  onChange,
  onLimitError,
  onSummariesChange,
  refreshKey,
  selected,
  sourceProjectRef,
  workspaceSlug,
}: CandidatePickerProps) {
  const [q, setQ] = useState("")
  const [status, setStatus] = useState("")
  const [offset, setOffset] = useState(0)
  const [resolving, setResolving] = useState(false)
  const refreshedKeyRef = useRef<number | undefined>(undefined)
  const filter = useMemo(
    () => buildResolveInput(kind, q, status),
    [kind, q, status]
  )
  const query = useQuery({
    queryKey: [
      "project-template-candidates",
      workspaceSlug,
      sourceProjectRef,
      kind,
      q,
      status,
      offset,
      refreshKey,
    ],
    queryFn: () =>
      listCandidates(kind, workspaceSlug, sourceProjectRef, {
        q: q || undefined,
        status: status || undefined,
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
    refreshedKeyRef.current = refreshKey
    const refs = [...selected.keys()]
    if (refs.length === 0) return
    let cancelled = false
    void Promise.all(
      chunk(refs, 100).map((selectedRefs) =>
        listCandidates(kind, workspaceSlug, sourceProjectRef, {
          limit: selectedRefs.length,
          offset: 0,
          refs: selectedRefs,
        })
      )
    ).then((pages) => {
      if (cancelled) return
      const next = new Map(selected)
      for (const candidate of pages.flatMap((page) => page.items)) {
        next.set(candidate.ref, summarizeCandidate(candidate, kind))
      }
      if (!sameSelectionMap(next, selected)) onSummariesChange(next)
    })
    return () => {
      cancelled = true
    }
  }, [kind, onSummariesChange, refreshKey, selected, sourceProjectRef, workspaceSlug])

  function toggle(candidate: Candidate, checked: boolean) {
    const next = new Map(selected)
    if (checked) next.set(candidate.ref, summarizeCandidate(candidate, kind))
    else next.delete(candidate.ref)
    onChange(next)
  }

  function togglePage(checked: boolean) {
    const next = new Map(selected)
    for (const candidate of page.items) {
      if (checked) next.set(candidate.ref, summarizeCandidate(candidate, kind))
      else next.delete(candidate.ref)
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
          next.delete(ref)
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
      <div className="grid gap-2 border bg-muted/20 p-2 sm:grid-cols-[minmax(12rem,1fr)_10rem_auto]">
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
        {kind === "task" || kind === "series" || kind === "automation" ? (
          <select
            aria-label="状态筛选"
            className="h-8 border bg-background px-2 text-xs"
            onChange={(event) => {
              setStatus(event.target.value)
              setOffset(0)
            }}
            value={status}
          >
            <option value="">全部状态</option>
            {kind === "task" ? (
              <>
                <option value="pending">待处理</option>
                <option value="waiting">等待中</option>
                <option value="completed">已完成</option>
              </>
            ) : kind === "series" ? (
              <>
                <option value="active">进行中</option>
                <option value="ended">已结束</option>
                <option value="stopped">已停止</option>
              </>
            ) : (
              <>
                <option value="enabled">已启用</option>
                <option value="disabled">已停用</option>
              </>
            )}
          </select>
        ) : (
          <span />
        )}
        <span className="self-center text-right text-xs text-muted-foreground tabular-nums">
          {page.total} 条
        </span>
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

function listCandidates(
  kind: CandidateKind,
  workspaceSlug: string,
  projectRef: string,
  options: {
    q?: string
    status?: string
    limit: number
    offset: number
    refs?: string[]
  }
): Promise<Page<Candidate>> {
  switch (kind) {
    case "task":
      return listProjectTemplateTaskCandidates(
        workspaceSlug,
        projectRef,
        options
      )
    case "series":
      return listProjectTemplateSeriesCandidates(
        workspaceSlug,
        projectRef,
        options
      )
    case "config":
      return listProjectTemplateConfigCandidates(workspaceSlug, projectRef, {
        q: options.q,
        limit: options.limit,
        offset: options.offset,
        refs: options.refs,
      })
    case "automation":
      return listProjectTemplateAutomationCandidates(
        workspaceSlug,
        projectRef,
        options
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

function buildResolveInput(
  kind: CandidateKind,
  q: string,
  status: string
): ResolveCandidateSelectionInput {
  const values = { q: q || undefined, status: status || undefined }
  switch (kind) {
    case "task":
      return { kind, task: values }
    case "series":
      return { kind, series: values }
    case "config":
      return { kind, config: { q: values.q } }
    case "automation":
      return { kind, automation: values }
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
