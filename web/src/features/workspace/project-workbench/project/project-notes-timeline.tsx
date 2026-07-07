import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { MarkdownView } from "@/components/markdown"
import type { ProjectAnnotationInfo } from "../api/project-api"

type ProjectNotesTimelineProps = {
  entries: ProjectAnnotationInfo[]
  canManage: boolean
  onDelete: (id: string) => void
  emptyLabel: string
  deleteLabel?: string
  deleteConfirm?: string
}

export function ProjectNotesTimeline({
  entries,
  canManage,
  onDelete,
  emptyLabel,
  deleteLabel,
  deleteConfirm,
}: ProjectNotesTimelineProps) {
  const { t } = useTranslation()
  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground">{emptyLabel}</p>
  }
  const ordered = [...entries].sort((a, b) => b.entry - a.entry)
  return (
    <ol className="space-y-2">
      {ordered.map((ann) => (
        <li
          className="relative border-l border-border pb-3 pl-4 last:pb-0"
          key={ann.id}
          role="listitem"
        >
          <span className="absolute -left-[5px] top-1 size-2 rounded-full bg-foreground ring-2 ring-card" />
          <div className="mb-1 text-xs text-muted-foreground">
            {ann.created_by?.name ?? "-"} · {formatTime(ann.entry)}
          </div>
          <div className="rounded border bg-background p-3 text-sm">
            <MarkdownView>{ann.content}</MarkdownView>
          </div>
          {canManage ? (
            <div className="mt-1 text-right">
              <Button
                onClick={() => {
                  if (
                    window.confirm(
                      deleteConfirm ?? t("projectSettings.noteDeleteConfirm")
                    )
                  ) {
                    onDelete(ann.id)
                  }
                }}
                size="sm"
                variant="outline"
              >
                {deleteLabel ?? t("common.delete")}
              </Button>
            </div>
          ) : null}
        </li>
      ))}
    </ol>
  )
}

function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  const hh = String(d.getHours()).padStart(2, "0")
  const mm = String(d.getMinutes()).padStart(2, "0")
  return `${y}-${m}-${day} ${hh}:${mm}`
}
