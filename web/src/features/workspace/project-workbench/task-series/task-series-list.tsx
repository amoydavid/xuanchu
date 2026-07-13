import { useState } from "react"

import type { TaskSeriesView } from "@/features/workspace/project-workbench/api/task-series-api"

// TaskSeriesList 是面板内的 series 列表（spec §15.3）。
// 行点击进入 detail；ended/stopped 行只读。
export function TaskSeriesList({
  items,
  total,
  loading,
  error,
  onSelect,
  statusFilter,
  onStatusFilterChange,
  query,
  onQueryChange,
  canManage,
  assignee = "",
  onAssigneeChange = () => {},
  sort = "next",
  onSortChange = () => {},
  offset = 0,
  limit = 20,
  onPageChange = () => {},
}: {
  items: TaskSeriesView[]
  total: number
  loading: boolean
  error: string | null
  onSelect: (series: TaskSeriesView) => void
  statusFilter: string
  onStatusFilterChange: (status: string) => void
  query: string
  onQueryChange: (q: string) => void
  canManage: boolean
  assignee?: string
  onAssigneeChange?: (assignee: string) => void
  sort?: string
  onSortChange?: (sort: string) => void
  offset?: number
  limit?: number
  onPageChange?: (offset: number) => void
}) {
  return (
    <div data-testid="task-series-list">
      <div className="flex gap-2">
        <input
          type="search"
          placeholder="搜索规则…"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          aria-label="搜索循环任务规则"
          className="flex-1"
        />
        <select
          value={statusFilter}
          onChange={(e) => onStatusFilterChange(e.target.value)}
          aria-label="循环任务状态筛选"
        >
          <option value="active">运行中</option>
          <option value="ended">已结束</option>
          <option value="stopped">已停止</option>
          <option value="all">全部</option>
        </select>
      </div>
      <div className="flex gap-2">
        <input
          aria-label="循环任务负责人筛选"
          onChange={(event) => onAssigneeChange(event.target.value)}
          placeholder="负责人"
          type="search"
          value={assignee}
        />
        <select
          aria-label="循环任务排序"
          onChange={(event) => onSortChange(event.target.value)}
          value={sort}
        >
          <option value="next">下一次执行</option>
          <option value="title">标题</option>
          <option value="modified">最近修改</option>
        </select>
      </div>
      {error && <div role="alert">{error}</div>}
      {loading && <div>加载中…</div>}
      {!loading && !error && items.length === 0 && (
        <div>还没有循环任务；适合每日巡检、周报等重复执行工作。</div>
      )}
      {items.map((series) => (
        <TaskSeriesListItem
          key={series.id}
          series={series}
          onSelect={onSelect}
          canManage={canManage}
        />
      ))}
      {total > 0 ? (
        <div>
          <span>
            {offset + 1}–{Math.min(offset + items.length, total)} / {total}
          </span>
          <button
            aria-label="上一页"
            disabled={offset === 0}
            onClick={() => onPageChange(Math.max(0, offset - limit))}
            type="button"
          >
            上一页
          </button>
          <button
            aria-label="下一页"
            disabled={offset + limit >= total}
            onClick={() => onPageChange(offset + limit)}
            type="button"
          >
            下一页
          </button>
        </div>
      ) : null}
    </div>
  )
}

function TaskSeriesListItem({
  series,
  onSelect,
  canManage,
}: {
  series: TaskSeriesView
  onSelect: (series: TaskSeriesView) => void
  canManage: boolean
}) {
  const readonly = series.status !== "active" || !canManage
  const [expanded, setExpanded] = useState(false)
  return (
    <article
      data-testid="task-series-row"
      data-series-id={series.id}
      data-status={series.status}
    >
      <button
        type="button"
        onClick={() => onSelect(series)}
        aria-label={`查看循环任务 ${series.title}`}
      >
        <span>{series.title}</span>
      </button>
      <span data-testid="series-status">
        {series.status === "active" ? "● 运行中" : series.status === "ended" ? "已结束" : "已停止"}
      </span>
      <span>
        {series.recurrence_rule}
        {series.next_recurrence_at ? ` · 下次 ${series.next_recurrence_at}` : ""}
        {" · 未完成 "}{series.open_occurrence_count}
        {series.overdue_count > 0 ? `（逾期 ${series.overdue_count}）` : ""}
      </span>
      {readonly && <span title="只读">只读</span>}
      {canManage && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          aria-label={expanded ? "收起" : "展开"}
          aria-expanded={expanded}
        >
          {expanded ? "›" : "›"}
        </button>
      )}
    </article>
  )
}
