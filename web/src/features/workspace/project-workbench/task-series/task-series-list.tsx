import { ChevronRightIcon, Repeat2Icon, SearchIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import type { TaskSeriesView } from "@/features/workspace/project-workbench/api/task-series-api"
import {
  formatTaskSeriesTimestamp,
  recurrenceRuleLabel,
  taskSeriesStatusLabel,
} from "./recurrence-preview"

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
  const { i18n, t } = useTranslation()
  const hasFilters = Boolean(query || assignee || statusFilter !== "active")

  return (
    <div className="space-y-3" data-testid="task-series-list">
      <div className="relative">
        <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          aria-label={t("taskSeries.aria.search")}
          className="pl-8"
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder={t("taskSeries.list.searchPlaceholder")}
          type="search"
          value={query}
        />
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Select onValueChange={onStatusFilterChange} value={statusFilter}>
          <SelectTrigger
            aria-label={t("taskSeries.aria.statusFilter")}
            className="w-full"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">
              {taskSeriesStatusLabel("active", t)}
            </SelectItem>
            <SelectItem value="ended">
              {taskSeriesStatusLabel("ended", t)}
            </SelectItem>
            <SelectItem value="stopped">
              {taskSeriesStatusLabel("stopped", t)}
            </SelectItem>
            <SelectItem value="all">
              {t("taskSeries.list.allStatuses")}
            </SelectItem>
          </SelectContent>
        </Select>
        <Select onValueChange={onSortChange} value={sort}>
          <SelectTrigger
            aria-label={t("taskSeries.aria.sort")}
            className="w-full"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="next">
              {t("taskSeries.list.sortNext")}
            </SelectItem>
            <SelectItem value="title">
              {t("taskSeries.list.sortTitle")}
            </SelectItem>
            <SelectItem value="modified">
              {t("taskSeries.list.sortModified")}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
      <Input
        aria-label={t("taskSeries.aria.assigneeFilter")}
        onChange={(event) => onAssigneeChange(event.target.value)}
        placeholder={t("taskSeries.list.assigneeFilter")}
        type="search"
        value={assignee}
      />

      <Separator />

      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : loading ? (
        <div className="space-y-2" aria-label={t("taskSeries.list.loading")}>
          {[0, 1, 2].map((item) => (
            <Skeleton className="h-20 w-full" key={item} />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center px-4 py-8 text-center">
          <div className="mb-3 flex size-9 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <Repeat2Icon className="size-4" />
          </div>
          <p className="text-sm font-medium">
            {hasFilters
              ? t("taskSeries.list.noResultsTitle")
              : t("taskSeries.list.emptyTitle")}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {hasFilters
              ? t("taskSeries.list.noResultsDescription")
              : t("taskSeries.list.emptyDescription")}
          </p>
        </div>
      ) : (
        <div className="divide-y overflow-hidden rounded-lg border bg-card">
          {items.map((series) => (
            <Button
              aria-label={t("taskSeries.aria.viewSeries", {
                title: series.title,
              })}
              className="group h-auto w-full items-start justify-start gap-3 rounded-none p-3 text-left whitespace-normal hover:bg-muted/60 focus-visible:ring-inset"
              data-series-id={series.id}
              data-status={series.status}
              data-testid="task-series-row"
              key={series.id}
              onClick={() => onSelect(series)}
              type="button"
              variant="ghost"
            >
              <div className="min-w-0 flex-1 space-y-1.5">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium">
                    {series.title}
                  </span>
                  <Badge
                    data-testid="series-status"
                    variant={
                      series.status === "active" ? "secondary" : "outline"
                    }
                  >
                    {taskSeriesStatusLabel(series.status, t)}
                  </Badge>
                </div>
                <div className="text-xs text-muted-foreground">
                  {recurrenceRuleLabel(series.recurrence_rule, t)}
                  {series.next_recurrence_at != null
                    ? ` · ${t("taskSeries.list.nextAt", { date: formatTaskSeriesTimestamp(series.next_recurrence_at, i18n.language) })}`
                    : ""}
                </div>
                <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                  <span>
                    {t("taskSeries.list.openCount", {
                      count: series.open_occurrence_count,
                    })}
                  </span>
                  {series.overdue_count > 0 ? (
                    <span className="text-destructive">
                      {t("taskSeries.list.overdueCount", {
                        count: series.overdue_count,
                      })}
                    </span>
                  ) : null}
                </div>
              </div>
              <ChevronRightIcon className="mt-1 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
            </Button>
          ))}
        </div>
      )}

      {total > 0 ? (
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
          <span className="tabular-nums">
            {offset + 1}–{Math.min(offset + items.length, total)} / {total}
          </span>
          <div className="flex gap-1">
            <Button
              aria-label={t("taskSeries.list.previousPage")}
              disabled={offset === 0}
              onClick={() => onPageChange(Math.max(0, offset - limit))}
              size="xs"
              type="button"
              variant="outline"
            >
              {t("taskSeries.list.previousPage")}
            </Button>
            <Button
              aria-label={t("taskSeries.list.nextPage")}
              disabled={offset + limit >= total}
              onClick={() => onPageChange(offset + limit)}
              size="xs"
              type="button"
              variant="outline"
            >
              {t("taskSeries.list.nextPage")}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}
