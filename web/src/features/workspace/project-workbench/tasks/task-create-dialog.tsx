import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  replaceDeferredAttachmentMarkers,
  type DeferredAttachment,
} from "@/components/markdown"
import {
  importTaskDraftAttachmentURL,
  removeAttachment,
  uploadTaskDraftAttachment,
} from "@/features/workspace/attachments"
import { dataURLToFile } from "@/components/markdown/paste-sanitizer"
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
import type {
  ProjectTaskFilterParams,
  ProjectWorkbenchTask,
} from "../api/project-api"
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
  onCreated?: (task: ProjectWorkbenchTask) => void
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

function hasDeferredAttachmentMarker(markdown: string, marker: string): boolean {
  return markdown.includes(marker) || markdown.includes(marker.replace(/[\\[\]]/g, "\\$&"))
}

export function TaskCreateDialog({
  filters,
  onCreated,
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
  const [deferredAttachments, setDeferredAttachments] = useState<DeferredAttachment[]>([])
  const [isSubmitting, setIsSubmitting] = useState(false)
  const submissionControllerRef = useRef<AbortController | null>(null)

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
    setDeferredAttachments([])
  }

  const cancelPendingSubmission = () => {
    const controller = submissionControllerRef.current
    if (!controller || createTask.isPending) {
      return false
    }
    controller.abort()
    submissionControllerRef.current = null
    setIsSubmitting(false)
    reset()
    onOpenChange(false)
    return true
  }

  const submit = async () => {
    if (submissionControllerRef.current) return
    const normalizedTitle = common.title.trim()
    if (!normalizedTitle) {
      setError(t("taskCreate.titleRequired"))
      return
    }
    setError(null)
    const controller = new AbortController()
    submissionControllerRef.current = controller
    setIsSubmitting(true)
    const throwIfCancelled = () => {
      if (controller.signal.aborted) {
        throw new DOMException("submission cancelled", "AbortError")
      }
    }
    try {
      const sourceDescription = common.description.trim()
      const activeDeferredAttachments = deferredAttachments.filter(({ marker }) =>
        hasDeferredAttachmentMarker(sourceDescription, marker)
      )
      const draftTarget =
        activeDeferredAttachments.length > 0 ? crypto.randomUUID() : undefined
      const resolved: Array<{ marker: string; id: string; alt: string }> = []
      if (draftTarget) {
        try {
          for (const { candidate, marker } of activeDeferredAttachments) {
            if (candidate.kind === "remote" && candidate.sourceURL) {
              const attachment = await importTaskDraftAttachmentURL(
                workspaceSlug,
                draftTarget,
                { sourceURL: candidate.sourceURL, displayName: candidate.alt },
                { signal: controller.signal }
              )
              resolved.push({ marker, id: attachment.id, alt: candidate.alt })
              throwIfCancelled()
              continue
            }
            const file = candidate.kind === "data" && candidate.sourceURL
              ? dataURLToFile(candidate.sourceURL, candidate.alt)
              : candidate.file
            if (!file) throw new Error("图片数据无效")
            const attachment = await uploadTaskDraftAttachment(
              workspaceSlug,
              draftTarget,
              { file, mode: "description_draft", displayName: candidate.alt },
              { signal: controller.signal }
            )
            resolved.push({ marker, id: attachment.id, alt: candidate.alt })
            throwIfCancelled()
          }
        } catch (uploadErr) {
          await Promise.all(
            resolved.map(({ id }) =>
              removeAttachment(workspaceSlug, id).catch(() => undefined)
            )
          )
          throw uploadErr
        }
      }
      const description = replaceDeferredAttachmentMarkers(
        sourceDescription,
        resolved
      ).trim()
      try {
        throwIfCancelled()
        const created = await createTask.mutateAsync({
          ...(description ? { description } : {}),
          ...(draftTarget ? { attachment_draft_target: draftTarget } : {}),
          ...(due !== null ? { due } : {}),
          ...(common.priority ? { priority: common.priority } : {}),
          ...(scheduled !== null ? { scheduled } : {}),
          ...(common.assignees.length > 0
            ? { assignees: common.assignees }
            : {}),
          ...(splitCSV(common.tags).length > 0
            ? { tags: splitCSV(common.tags) }
            : {}),
          ...(Object.keys(common.udas).length > 0 ? { udas: common.udas } : {}),
          ...(until !== null ? { until } : {}),
          ...(wait !== null ? { wait } : {}),
          project: projectSlug,
          title: normalizedTitle,
        })
        onCreated?.(created)
        reset()
        onOpenChange(false)
      } catch (createErr) {
        await Promise.all(
          resolved.map(({ id }) =>
            removeAttachment(workspaceSlug, id).catch(() => undefined)
          )
        )
        throw createErr
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        setError(err instanceof Error ? err.message : String(err))
      }
    } finally {
      if (submissionControllerRef.current === controller) {
        submissionControllerRef.current = null
        setIsSubmitting(false)
      }
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && isSubmitting && cancelPendingSubmission()) {
          return
        }
        if (!nextOpen && !createTask.isPending && !seriesSubmitting && !isSubmitting) {
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
              disabled={createTask.isPending || seriesSubmitting || isSubmitting}
              value="normal"
            >
              {t("taskSeries.mode.normal")}
            </TabsTrigger>
            <TabsTrigger
              disabled={createTask.isPending || seriesSubmitting || isSubmitting}
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
                disabled={createTask.isPending || isSubmitting}
                onChange={(next) => {
                  setCommon(next)
                  setError(null)
                }}
                onDeferredAttachment={(attachment) => {
                  setDeferredAttachments((current) =>
                    current.some((item) => item.marker === attachment.marker)
                      ? current
                      : [...current, attachment]
                  )
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
                onClick={() => {
                  if (!cancelPendingSubmission()) onOpenChange(false)
                }}
                type="button"
                variant="outline"
              >
                {t("common.cancel")}
              </Button>
              <Button
                disabled={createTask.isPending || isSubmitting}
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
