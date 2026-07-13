import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { TaskSeriesView } from "../api/task-series-api"
import type { ProjectTaskFilterParams } from "../api/project-api"
import { useCreateTaskMutation } from "../hooks/use-task-mutations"
import { InlineDatePicker } from "../shared/inline-date-picker"
import { TaskSeriesForm } from "../task-series/task-series-form"
import {
  TaskCommonFields,
  type TaskCommonFieldValue,
} from "./task-common-fields"

type TaskCreateDialogProps = {
  filters?: ProjectTaskFilterParams | string
  onOpenChange: (open: boolean) => void
  open: boolean
  projectSlug: string
  workspaceSlug: string
  /**
   * 初始模式（spec §15.15）：
   * - normal（默认）：创建普通任务，提交 POST /tasks
   * - recurring：创建循环任务，提交 POST /task-series 并关闭弹窗
   *
   * 两种模式共享同一个 Dialog 容器，表单状态分别保留。
   */
  initialMode?: "normal" | "recurring"
  onRecurringCreated?: (series: TaskSeriesView) => void
}

const emptyCommonFields = (): TaskCommonFieldValue => ({
  title: "",
  description: "",
  priority: "",
  assignees: [],
  tags: "",
  udas: {},
})

export function TaskCreateDialog({
  filters,
  onOpenChange,
  open,
  onRecurringCreated,
  projectSlug,
  workspaceSlug,
  initialMode = "normal",
}: TaskCreateDialogProps) {
  const { t } = useTranslation()
  // mode selector：支持在弹窗内切换普通/循环（spec §15.15）。
  const [mode, setMode] = useState<"normal" | "recurring">(initialMode)
  const [seriesSubmitting, setSeriesSubmitting] = useState(false)
  const createTask = useCreateTaskMutation(workspaceSlug, projectSlug, filters)
  const [common, setCommon] = useState<TaskCommonFieldValue>(emptyCommonFields)
  const [due, setDue] = useState<number | null>(null)
  const [recurringFirstDue, setRecurringFirstDue] = useState<number | null>(
    null
  )
  const copiedToRecurring = useRef(false)
  const copiedToNormal = useRef(false)
  const [scheduled, setScheduled] = useState<number | null>(null)
  const [wait, setWait] = useState<number | null>(null)
  const [until, setUntil] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)

  const reset = () => {
    setCommon(emptyCommonFields())
    setDue(null)
    setRecurringFirstDue(null)
    copiedToRecurring.current = false
    copiedToNormal.current = false
    setScheduled(null)
    setWait(null)
    setUntil(null)
    setError(null)
  }

  const submit = async () => {
    const normalizedTitle = common.title.trim()
    if (!normalizedTitle) {
      setError(t("taskCreate.titleRequired"))
      return
    }
    setError(null)
    try {
      await createTask.mutateAsync({
        ...(common.description.trim()
          ? { description: common.description.trim() }
          : {}),
        ...(due !== null ? { due } : {}),
        ...(common.priority ? { priority: common.priority } : {}),
        ...(scheduled !== null ? { scheduled } : {}),
        ...(common.assignees.length > 0 ? { assignees: common.assignees } : {}),
        ...(splitCSV(common.tags).length > 0
          ? { tags: splitCSV(common.tags) }
          : {}),
        ...(Object.keys(common.udas).length > 0 ? { udas: common.udas } : {}),
        ...(until !== null ? { until } : {}),
        ...(wait !== null ? { wait } : {}),
        project: projectSlug,
        title: normalizedTitle,
      })
      reset()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && !createTask.isPending && !seriesSubmitting) {
          reset()
          onOpenChange(false)
        }
      }}
    >
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("taskSeries.create.title")}</DialogTitle>
          <DialogDescription>
            {mode === "recurring"
              ? t("taskSeries.create.description")
              : t("taskCreate.description")}
          </DialogDescription>
        </DialogHeader>
        <Tabs
          onValueChange={(value) => {
            const next = value as typeof mode
            if (
              next === "recurring" &&
              recurringFirstDue == null &&
              due != null &&
              !copiedToRecurring.current
            ) {
              setRecurringFirstDue(due)
              copiedToRecurring.current = true
            }
            if (
              next === "normal" &&
              due == null &&
              recurringFirstDue != null &&
              !copiedToNormal.current
            ) {
              setDue(recurringFirstDue)
              copiedToNormal.current = true
            }
            setMode(next)
          }}
          value={mode}
        >
          <TabsList
            aria-label={t("taskCreate.typeLabel")}
            className="grid w-full grid-cols-2"
          >
            <TabsTrigger
              disabled={createTask.isPending || seriesSubmitting}
              value="normal"
            >
              {t("taskSeries.mode.normal")}
            </TabsTrigger>
            <TabsTrigger
              disabled={createTask.isPending || seriesSubmitting}
              value="recurring"
            >
              {t("taskSeries.mode.recurring")}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        {mode === "normal" ? (
          <div>
            <div className="grid gap-4">
              <TaskCommonFields
                autoFocus
                disabled={createTask.isPending}
                onChange={(next) => {
                  setCommon(next)
                  setError(null)
                }}
                onSubmit={() => void submit()}
                value={common}
                workspaceSlug={workspaceSlug}
              />
              <div className="grid gap-3 md:grid-cols-2">
                <InlineDatePicker
                  ariaLabel={t("taskCreate.due")}
                  boundary="end"
                  emptyLabel={t("taskCreate.due")}
                  onSave={setDue}
                  value={due}
                />
                <InlineDatePicker
                  ariaLabel={t("taskCreate.scheduled")}
                  emptyLabel={t("taskCreate.scheduled")}
                  onSave={setScheduled}
                  value={scheduled}
                />
                <InlineDatePicker
                  ariaLabel={t("taskCreate.wait")}
                  emptyLabel={t("taskCreate.wait")}
                  onSave={setWait}
                  value={wait}
                />
                <InlineDatePicker
                  ariaLabel={t("taskCreate.until")}
                  boundary="end"
                  emptyLabel={t("taskCreate.until")}
                  onSave={setUntil}
                  value={until}
                />
              </div>
              {error ? (
                <p className="text-sm text-destructive">{error}</p>
              ) : null}
            </div>
            <DialogFooter>
              <Button
                disabled={createTask.isPending}
                onClick={() => onOpenChange(false)}
                type="button"
                variant="outline"
              >
                {t("common.cancel")}
              </Button>
              <Button
                disabled={createTask.isPending}
                onClick={() => void submit()}
                type="button"
              >
                {t("taskCreate.submit")}
              </Button>
            </DialogFooter>
          </div>
        ) : null}
        {mode === "recurring" ? (
          <div>
            <TaskSeriesForm
              commonValue={common}
              firstDueValue={recurringFirstDue}
              mode="create"
              onCancel={() => onOpenChange(false)}
              onCreated={(created) => {
                onRecurringCreated?.(created)
                reset()
                onOpenChange(false)
              }}
              onSubmittingChange={setSeriesSubmitting}
              onCommonChange={setCommon}
              onFirstDueChange={setRecurringFirstDue}
              projectSlug={projectSlug}
              workspaceSlug={workspaceSlug}
            />
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
}
