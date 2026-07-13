/* eslint-disable react-hooks/set-state-in-effect -- 弹窗/面板打开时需在 effect 内重置状态 */
import { useEffect, useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { PlusIcon, Repeat2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"

import {
  listTaskSeries,
  getTaskSeries,
  stopTaskSeries,
  type TaskSeriesView,
  type TaskSeriesListInput,
} from "@/features/workspace/project-workbench/api/task-series-api"
import { TaskSeriesList } from "./task-series-list"
import { TaskSeriesDetail } from "./task-series-detail"
import { TaskSeriesEditorDialog } from "./task-series-editor-dialog"
import { TaskSeriesStopDialog } from "./task-series-stop-dialog"
import { TaskCreateDialog } from "../tasks/task-create-dialog"

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
  const { t } = useTranslation()
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
      status: statusFilter === "all" ? undefined : statusFilter,
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
        setError(
          err instanceof Error ? err.message : t("taskSeries.errors.loadList")
        )
        setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [
    workspaceSlug,
    projectSlug,
    statusFilter,
    query,
    assignee,
    sort,
    offset,
    listRevision,
    t,
  ])

  // 加载详情（seriesRef 存在时）。
  useEffect(() => {
    if (!seriesRef) {
      setDetail(null)
      return
    }
    let cancelled = false
    setDetail(null)
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
        setDetailError(
          err instanceof Error ? err.message : t("taskSeries.errors.loadDetail")
        )
        setDetailLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [workspaceSlug, seriesRef, t])

  const selectSeries = (s: TaskSeriesView) => {
    void navigate({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/series/$seriesRef",
      params: { workspaceSlug, projectSlug, seriesRef: s.id },
      search: true,
    })
  }

  const backToList = () => {
    void navigate({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/series",
      params: { workspaceSlug, projectSlug },
      search: true,
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
      setDetailError(
        err instanceof Error ? err.message : t("taskSeries.errors.stop")
      )
      setStopOpen(false)
    }
  }

  return (
    <section
      data-testid="task-series-panel"
      data-panel-mode={seriesRef ? "detail" : "list"}
      data-loading={loading || detailLoading}
      data-error={error ?? detailError ?? undefined}
      aria-label={t("taskSeries.aria.panel")}
      className="w-full space-y-4 lg:w-80 lg:shrink-0"
    >
      <header className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <h2 className="flex items-center gap-2 text-base font-semibold">
            <Repeat2Icon className="size-4 text-muted-foreground" />
            {total > 0
              ? t("taskSeries.count", { count: total })
              : t("taskSeries.title")}
          </h2>
          {canManage ? (
            <Button
              aria-label={t("taskSeries.actions.create")}
              onClick={() => setCreateOpen(true)}
              size="icon-sm"
              type="button"
            >
              <PlusIcon />
            </Button>
          ) : null}
        </div>
        <p className="text-xs leading-5 text-muted-foreground">
          {t("taskSeries.description")}
        </p>
      </header>

      <Separator />

      {seriesRef ? (
        // 详情模式。
        <>
          {detailError ? (
            <Alert variant="destructive">
              <AlertDescription>{detailError}</AlertDescription>
            </Alert>
          ) : null}
          {detailLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-8 w-2/3" />
              <Skeleton className="h-28 w-full" />
              <Skeleton className="h-40 w-full" />
            </div>
          ) : null}
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

      {detail && editOpen ? (
        <TaskSeriesEditorDialog
          open={editOpen}
          workspaceSlug={workspaceSlug}
          projectSlug={projectSlug}
          series={detail}
          onClose={() => setEditOpen(false)}
          onSaved={() => {
            setListRevision((current) => current + 1)
            void getTaskSeries(workspaceSlug, detail.id).then((v) =>
              setDetail(v)
            )
          }}
        />
      ) : null}

      {/* 停止确认弹窗 */}
      <TaskSeriesStopDialog
        open={stopOpen}
        series={detail ? { title: detail.title } : null}
        openCount={detail?.open_occurrence_count ?? 0}
        onConfirm={confirmStop}
        onCancel={() => setStopOpen(false)}
      />
      {createOpen ? (
        <TaskCreateDialog
          initialMode="recurring"
          onOpenChange={(next) => {
            setCreateOpen(next)
            if (!next) setListRevision((current) => current + 1)
          }}
          onRecurringCreated={selectSeries}
          open={createOpen}
          projectSlug={projectSlug}
          workspaceSlug={workspaceSlug}
        />
      ) : null}
    </section>
  )
}
