import { useState } from "react"
import { useTranslation } from "react-i18next"

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { TaskSeriesView } from "@/features/workspace/project-workbench/api/task-series-api"
import { TaskSeriesForm } from "./task-series-form"

/** 独立创建/编辑入口的兼容容器；嵌入其它弹窗时直接使用 TaskSeriesForm。 */
export function TaskSeriesDialog({
  open,
  workspaceSlug,
  projectSlug,
  mode,
  series,
  onClose,
  onCreated,
  onSaved,
}: {
  open: boolean
  workspaceSlug: string
  projectSlug: string
  mode: "create" | "edit"
  series?: TaskSeriesView | null
  onClose: () => void
  onCreated?: (series: TaskSeriesView) => void
  onSaved?: () => void
}) {
  const { t } = useTranslation()
  const isCreate = mode === "create"
  const [submitting, setSubmitting] = useState(false)

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => !next && !submitting && onClose()}
    >
      <DialogContent
        className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
        data-testid="task-series-dialog"
        showCloseButton={!submitting}
      >
        <DialogHeader>
          <DialogTitle>
            {isCreate
              ? t("taskSeries.create.recurringTitle")
              : t("taskSeries.edit.title")}
          </DialogTitle>
          <DialogDescription>
            {isCreate
              ? t("taskSeries.create.description")
              : t("taskSeries.edit.description")}
          </DialogDescription>
        </DialogHeader>
        <TaskSeriesForm
          key={`${mode}:${series?.id ?? "new"}`}
          mode={mode}
          onCancel={onClose}
          onSubmittingChange={setSubmitting}
          onCreated={(created) => {
            onCreated?.(created)
            onClose()
          }}
          onSaved={() => {
            onSaved?.()
            onClose()
          }}
          projectSlug={projectSlug}
          series={series}
          workspaceSlug={workspaceSlug}
        />
      </DialogContent>
    </Dialog>
  )
}
