import type { TaskSeriesView } from "@/features/workspace/project-workbench/api/task-series-api"
import { TaskSeriesDialog } from "./task-series-dialog"

export function TaskSeriesEditorDialog({
  open,
  workspaceSlug,
  projectSlug,
  series,
  onClose,
  onSaved,
}: {
  open: boolean
  workspaceSlug: string
  projectSlug: string
  series: TaskSeriesView | null
  onClose: () => void
  onSaved?: () => void
}) {
  return (
    <TaskSeriesDialog
      mode="edit"
      onClose={onClose}
      onSaved={onSaved}
      open={open}
      projectSlug={projectSlug}
      series={series}
      workspaceSlug={workspaceSlug}
    />
  )
}
