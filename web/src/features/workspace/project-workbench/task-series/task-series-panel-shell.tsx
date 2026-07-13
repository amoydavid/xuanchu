/* eslint-disable react-hooks/set-state-in-effect -- 弹窗/面板打开时需在 effect 内重置状态 */
import { useEffect, useState } from "react"
import { useNavigate } from "@tanstack/react-router"

import {
  listTaskSeries,
  getTaskSeries,
  stopTaskSeries,
  type TaskSeriesView,
  type TaskSeriesListInput,
} from "@/features/workspace/project-workbench/api/task-series-api"
import { TaskSeriesList } from "./task-series-list"
import { TaskSeriesDetail } from "./task-series-detail"
import { TaskSeriesDialog } from "./task-series-dialog"
import { TaskSeriesStopDialog } from "./task-series-stop-dialog"

// TaskSeriesPanelShell 是循环任务管理面板的壳层（spec §15.3、§15.12）。
//
// 桌面端作为右栏面板渲染；seriesRef 存在时显示详情，否则显示列表。
// 接入已实现的 List/Detail/Dialog/StopDialog 组件，导航通过路由 seriesRef 切换。
export function TaskSeriesPanelShell({
  workspaceSlug,
  projectSlug,
  seriesRef,
  canManage = false,
}: {
  workspaceSlug: string
  projectSlug: string
  seriesRef?: string
  canManage?: boolean
}) {
  const navigate = useNavigate()
  const [statusFilter, setStatusFilter] = useState<string>("active")
  const [query, setQuery] = useState<string>("")
  const [assignee, setAssignee] = useState("")
  const [sort, setSort] = useState("next")
  const [offset, setOffset] = useState(0)
  const [listRevision, setListRevision] = useState(0)
  const [list, setList] = useState<TaskSeriesView[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // 详情。
  const [detail, setDetail] = useState<TaskSeriesView | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)

  // 编辑 / 停止弹窗。
  const [editOpen, setEditOpen] = useState(false)
  const [stopOpen, setStopOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)

  const pageLimit = 20

  // 加载 series 列表。
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    const input: TaskSeriesListInput = {
      project: projectSlug,
      status: statusFilter,
      q: query || undefined,
      assignee: assignee || undefined,
      sort,
      limit: pageLimit,
      offset,
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
  }, [workspaceSlug, projectSlug, statusFilter, query, assignee, sort, offset, listRevision])

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

  const selectSeries = (s: TaskSeriesView) => {
    void navigate({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/series/$seriesRef",
      params: { workspaceSlug, projectSlug, seriesRef: s.id },
    })
  }

  const backToList = () => {
    void navigate({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/series",
      params: { workspaceSlug, projectSlug },
    })
  }

  const confirmStop = async (deleteOpen: boolean) => {
    if (!detail) return
    try {
      await stopTaskSeries(workspaceSlug, detail.id, deleteOpen)
      setStopOpen(false)
      setListRevision((current) => current + 1)
      backToList()
    } catch (err: unknown) {
      setDetailError(err instanceof Error ? err.message : "停止循环任务失败")
      setStopOpen(false)
    }
  }

  return (
    <section
      data-testid="task-series-panel"
      data-panel-mode={seriesRef ? "detail" : "list"}
      data-loading={loading || detailLoading}
      data-error={error ?? detailError ?? undefined}
      aria-label="循环任务管理面板"
      className="space-y-3"
    >
      <header>
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-base font-semibold">
            循环任务{total > 0 ? ` ${total}` : ""}
          </h2>
          {canManage ? (
            <button type="button" onClick={() => setCreateOpen(true)}>
              新建循环任务
            </button>
          ) : null}
        </div>
        <p className="text-sm text-muted-foreground">
          管理会按计划重复产生实例的任务；每一次仍在左侧完成。
        </p>
      </header>

      {seriesRef ? (
        // 详情模式。
        <>
          {detailError && <div role="alert">{detailError}</div>}
          {detailLoading && <div>加载详情中…</div>}
          {detail && (
            <TaskSeriesDetail
              series={detail}
              canManage={canManage}
              projectSlug={projectSlug}
              workspaceSlug={workspaceSlug}
              onBack={backToList}
              onEdit={() => setEditOpen(true)}
              onStop={() => setStopOpen(true)}
            />
          )}
        </>
      ) : (
        // 列表模式。
        <TaskSeriesList
          items={list}
          total={total}
          loading={loading}
          error={error}
          onSelect={selectSeries}
          statusFilter={statusFilter}
          onStatusFilterChange={(value) => {
            setStatusFilter(value)
            setOffset(0)
          }}
          query={query}
          onQueryChange={(value) => {
            setQuery(value)
            setOffset(0)
          }}
          assignee={assignee}
          onAssigneeChange={(value) => {
            setAssignee(value)
            setOffset(0)
          }}
          sort={sort}
          onSortChange={(value) => {
            setSort(value)
            setOffset(0)
          }}
          offset={offset}
          limit={pageLimit}
          onPageChange={setOffset}
          canManage={canManage}
        />
      )}

      {/* 编辑弹窗 */}
      {detail && (
        <TaskSeriesDialog
          open={editOpen}
          mode="edit"
          workspaceSlug={workspaceSlug}
          projectSlug={projectSlug}
          series={detail}
          onClose={() => {
            setEditOpen(false)
            setListRevision((current) => current + 1)
            // 重新加载详情。
            void getTaskSeries(workspaceSlug, detail.id).then((v) => setDetail(v))
          }}
        />
      )}

      {/* 停止确认弹窗 */}
      <TaskSeriesStopDialog
        open={stopOpen}
        series={detail ? { title: detail.title } : null}
        openCount={detail?.open_occurrence_count ?? 0}
        onConfirm={confirmStop}
        onCancel={() => setStopOpen(false)}
      />
      <TaskSeriesDialog
        open={createOpen}
        mode="create"
        workspaceSlug={workspaceSlug}
        projectSlug={projectSlug}
        onClose={() => setCreateOpen(false)}
        onCreated={(created) => {
          setCreateOpen(false)
          setListRevision((current) => current + 1)
          selectSeries(created)
        }}
      />
    </section>
  )
}
