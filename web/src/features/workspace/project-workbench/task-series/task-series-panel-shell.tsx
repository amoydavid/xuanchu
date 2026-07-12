import { useEffect, useState } from "react"

import {
  listTaskSeries,
  getTaskSeries,
  type TaskSeriesView,
  type TaskSeriesListInput,
} from "@/features/workspace/project-workbench/api/task-series-api"

// TaskSeriesPanelShell 是循环任务管理面板的壳层（spec §15.3、§15.12）。
//
// 它：
// - 桌面端作为右栏面板渲染；窄屏/移动端由父级升级为全屏 Sheet（后续 Task 12）
// - 加载 series 列表（默认 active）和可选的 series 详情
// - 加载/错误/空状态分别呈现
// - 面板内的 list/detail 切换不重挂载父级任务页
//
// 注意：本组件是 Task 11 的最小可编译壳层，实际 list/detail/dialog UI 在 Task 12 补齐。
export function TaskSeriesPanelShell({
  workspaceSlug,
  projectSlug,
  seriesRef,
}: {
  workspaceSlug: string
  projectSlug: string
  seriesRef?: string
}) {
  const [statusFilter, setStatusFilter] = useState<string>("active")
  const [list, setList] = useState<TaskSeriesView[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // 详情（若 seriesRef 存在）。
  const [detail, setDetail] = useState<TaskSeriesView | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)

  // 加载 series 列表。
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    const input: TaskSeriesListInput = {
      project: projectSlug,
      status: statusFilter,
      limit: 1,
    }
    void listTaskSeries(workspaceSlug, input)
      .then((page) => {
        if (cancelled) return
        setList(page.items)
        setTotal(page.total)
        setLoading(false)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setError(err instanceof Error ? err.message : "加载循环任务失败")
        setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [workspaceSlug, projectSlug, statusFilter])

  // 加载详情（seriesRef 存在时）。
  useEffect(() => {
    if (!seriesRef) {
      setDetail(null)
      return
    }
    let cancelled = false
    setDetailLoading(true)
    setDetailError(null)
    void getTaskSeries(workspaceSlug, seriesRef)
      .then((view) => {
        if (cancelled) return
        setDetail(view)
        setDetailLoading(false)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setDetailError(err instanceof Error ? err.message : "加载循环任务详情失败")
        setDetailLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [workspaceSlug, seriesRef])

  return (
    <section
      data-testid="task-series-panel"
      data-panel-mode={seriesRef ? "detail" : "list"}
      data-loading={loading || detailLoading}
      data-error={error ?? detailError ?? undefined}
      aria-label="循环任务管理面板"
    >
      <header>
        <h2>循环任务{total > 0 ? ` ${total}` : ""}</h2>
        <p>管理会按计划重复产生实例的任务；每一次仍在左侧完成。</p>
      </header>
      {error && <div role="alert">{error}</div>}
      {loading && <div>加载中…</div>}
      {!loading && !error && list.length === 0 && !seriesRef && (
        <div>还没有循环任务；适合每日巡检、周报等重复执行工作。</div>
      )}
      {list.map((s) => (
        <SeriesRow key={s.id} series={s} />
      ))}
      {seriesRef && detailError && <div role="alert">{detailError}</div>}
      {seriesRef && detailLoading && <div>加载详情中…</div>}
      {seriesRef && detail && (
        <SeriesDetailBlock series={detail} />
      )}
      {/* 状态筛选（占位） */}
      <div className="sr-only" aria-hidden>
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          aria-label="循环任务状态筛选"
        >
          <option value="active">运行中</option>
          <option value="ended">已结束</option>
          <option value="stopped">已停止</option>
          <option value="all">全部</option>
        </select>
      </div>
    </section>
  )
}

function SeriesRow({ series }: { series: TaskSeriesView }) {
  return (
    <article data-testid="task-series-row" data-series-id={series.id}>
      <div>{series.title}</div>
      <div>
        {series.recurrence_rule} · 状态：{series.status} · 未完成 {series.open_occurrence_count}
        {series.next_recurrence_at ? ` · 下次 ${series.next_recurrence_at}` : ""}
      </div>
    </article>
  )
}

function SeriesDetailBlock({ series }: { series: TaskSeriesView }) {
  return (
    <article data-testid="task-series-detail" data-series-id={series.id}>
      <h3>{series.title}</h3>
      <dl>
        <dt>状态</dt>
        <dd>{series.status}</dd>
        <dt>规则</dt>
        <dd>{series.recurrence_rule}</dd>
        <dt>未完成</dt>
        <dd>{series.open_occurrence_count}</dd>
        <dt>已完成</dt>
        <dd>{series.completed_count}</dd>
        <dt>已跳过</dt>
        <dd>{series.skipped_count}</dd>
      </dl>
    </article>
  )
}
