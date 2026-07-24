import { Button } from "@/components/ui/button"
import { useTranslation } from "react-i18next"

import type { TaskActivityEntry } from "../api/task-api"
import { TaskActivityEntryContent } from "./task-activity-entry"

type TaskActivityTimelineProps = {
  canWrite: boolean
  entries: TaskActivityEntry[]
  hasNextPage: boolean
  isFetchingNextPage: boolean
  onDeleteAnnotation: (annotationID: string) => void
  onEditAnnotation: (annotation: { id: string; description: string }) => void
  onLoadMore: () => void
}

export function TaskActivityTimeline({
  canWrite,
  entries,
  hasNextPage,
  isFetchingNextPage,
  onDeleteAnnotation,
  onEditAnnotation,
  onLoadMore,
}: TaskActivityTimelineProps) {
  const { t } = useTranslation()
  if (entries.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t("taskDetail.activityEmpty")}
      </p>
    )
  }
  return (
    <div className="space-y-1">
      <ol aria-label={t("taskDetail.activity")} className="m-0 list-none p-0">
        {entries.map((entry, index) => {
          const hasPrevious = index > 0
          const hasNext = index < entries.length - 1
          const tail = hasNext ? "entry" : hasNextPage ? "more" : "end"
          return (
            <li
              className="grid grid-cols-[16px_minmax(0,1fr)] gap-3"
              key={entry.id}
            >
              <div aria-hidden="true" className="relative" data-activity-node>
                {hasPrevious ? (
                  <span
                    className="absolute top-0 left-1/2 h-2 -translate-x-1/2 border-l border-border"
                    data-activity-line-before="true"
                  />
                ) : null}
                <span className="absolute top-2 left-1/2 z-10 size-2 -translate-x-1/2 rounded-full border border-foreground/60 bg-background" />
                {tail !== "end" ? (
                  <span
                    className={`absolute top-4 left-1/2 -translate-x-1/2 border-l border-border ${
                      tail === "more" ? "bottom-0 opacity-60" : "bottom-0"
                    }`}
                    data-activity-tail={tail}
                  />
                ) : null}
              </div>
              <TaskActivityEntryContent
                canWrite={canWrite}
                entry={entry}
                onDeleteAnnotation={onDeleteAnnotation}
                onEditAnnotation={onEditAnnotation}
              />
            </li>
          )
        })}
      </ol>
      {hasNextPage ? (
        <div className="pl-7">
          <Button
            disabled={isFetchingNextPage}
            onClick={onLoadMore}
            size="sm"
            type="button"
            variant="ghost"
          >
            {isFetchingNextPage
              ? t("taskDetail.activityLoading")
              : t("taskDetail.activityLoadMore")}
          </Button>
        </div>
      ) : null}
    </div>
  )
}

export function TaskActivityTimelineSkeleton() {
  return (
    <div aria-hidden="true" className="space-y-0">
      {[0, 1, 2].map((index) => (
        <div
          className="grid grid-cols-[16px_minmax(0,1fr)] gap-3"
          data-activity-skeleton
          key={index}
        >
          <div className="relative h-14">
            <span className="absolute top-2 left-1/2 size-2 -translate-x-1/2 rounded-full bg-muted" />
            {index < 2 ? (
              <span className="absolute top-4 bottom-0 left-1/2 -translate-x-1/2 border-l border-border" />
            ) : null}
          </div>
          <div className="space-y-2 pt-1">
            <div className="h-3 w-2/3 animate-pulse rounded bg-muted" />
            <div className="h-3 w-1/3 animate-pulse rounded bg-muted" />
          </div>
        </div>
      ))}
    </div>
  )
}
