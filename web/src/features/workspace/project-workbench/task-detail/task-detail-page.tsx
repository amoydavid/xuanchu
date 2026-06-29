import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { useMe } from "@/features/workspace/session/useMe"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import { ApiError } from "@/lib/api"
import { cn } from "@/lib/utils"
import type { ProjectTask } from "../api/task-api"
import { useModifyTaskMutation } from "../hooks/use-task-mutations"
import { useTaskDetailQuery } from "../hooks/use-task-detail-data"
import { canTaskWrite } from "../permissions/permissions"
import { EditFeedbackProvider } from "../shared/edit-feedback"
import { InlineTextEditor } from "../shared/inline-text-editor"
import { TaskActionBar } from "./task-action-bar"
import { TaskAnnotationsEditor } from "./task-annotations-editor"
import { TaskLinksEditor } from "./task-links-editor"
import { TaskPropertyPanel } from "./task-property-panel"

type TaskDetailPageProps = {
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

type MobileDetailTab = "annotations" | "links" | "properties"

export function TaskDetailPage({
  projectSlug,
  taskRef,
  workspaceSlug,
}: TaskDetailPageProps) {
  return (
    <EditFeedbackProvider>
      <TaskDetailPageContent
        projectSlug={projectSlug}
        taskRef={taskRef}
        workspaceSlug={workspaceSlug}
      />
    </EditFeedbackProvider>
  )
}

function TaskDetailPageContent({
  projectSlug,
  taskRef,
  workspaceSlug,
}: TaskDetailPageProps) {
  const { t } = useTranslation()
  const me = useMe()
  const canWrite = canTaskWrite({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const task = useTaskDetailQuery(workspaceSlug, taskRef)
  const modifyTask = useModifyTaskMutation(workspaceSlug, projectSlug, taskRef)
  const projectHref = `/workspaces/${workspaceSlug}/projects/${projectSlug}`
  const [activeMobileTab, setActiveMobileTab] =
    useState<MobileDetailTab>("properties")

  if (task.isPending) {
    return <TaskDetailSkeleton />
  }

  if (task.isError) {
    const title =
      task.error instanceof ApiError && task.error.status === 404
        ? t("projectReadonly.taskNotFoundTitle")
        : t("common.error")
    return (
      <section className="max-w-2xl border bg-card p-6">
        <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
        <p className="mt-3 text-sm text-muted-foreground">
          {task.error instanceof ApiError ? task.error.code : "unknown"}
        </p>
        <Button asChild className="mt-5" variant="outline">
          <a href={projectHref}>{t("projectReadonly.backToProject")}</a>
        </Button>
      </section>
    )
  }

  const taskData = task.data
  const taskWritable = canWrite && isWritableTaskStatus(taskData.status)
  if (!taskBelongsToProject(taskData, projectSlug)) {
    return (
      <section className="max-w-2xl border bg-card p-6">
        <h1 className="text-xl font-semibold tracking-normal">
          {t("projectReadonly.taskNotFoundTitle")}
        </h1>
        <Button asChild className="mt-5" variant="outline">
          <a href={projectHref}>{t("projectReadonly.backToProject")}</a>
        </Button>
      </section>
    )
  }

  return (
    <div className="space-y-5">
      <section className="border-b pb-4">
        <div className="text-xs text-muted-foreground">
          {workspaceSlug} / {projectSlug} /{" "}
          {taskData.task_slug || taskData.uuid.slice(0, 8)}
        </div>
        <div className="mt-3 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div className="min-w-0 flex-1">
            <InlineTextEditor
              ariaLabel={t("projectReadonly.taskTitle")}
              disabled={!taskWritable}
              displayClassName="text-2xl font-semibold tracking-normal"
              onSave={async (title) => {
                await modifyTask.mutateAsync({ title })
              }}
              validate={(value) =>
                value.trim() ? null : t("projectReadonly.taskTitleRequired")
              }
              value={taskData.title}
            />
            <div className="mt-3 flex flex-wrap gap-2">
              <Badge variant="outline">{taskStatusLabel(taskData.status, t)}</Badge>
              {taskData.priority ? (
                <Badge variant="outline">{taskData.priority}</Badge>
              ) : null}
              {taskData.task_slug ? (
                <Badge variant="outline">{taskData.task_slug}</Badge>
              ) : null}
            </div>
          </div>
          <div className="flex shrink-0 flex-col items-start gap-2 md:items-end">
            <Button asChild variant="outline">
              <a href={projectHref}>{t("projectReadonly.backToProject")}</a>
            </Button>
            <TaskActionBar
              canWrite={taskWritable}
              projectSlug={projectSlug}
              task={taskData}
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
          </div>
        </div>
      </section>

      <MobileDetailTabs
        active={activeMobileTab}
        onChange={setActiveMobileTab}
      />

      <div className="grid gap-5 md:grid-cols-[1fr_240px]">
        <div className="contents md:block md:space-y-5">
          <div className={mobilePanelClass(activeMobileTab, "annotations")}>
            <TaskDescriptionBlock
              canWrite={taskWritable}
              onSave={async (description) => {
                await modifyTask.mutateAsync(
                  description ? { description } : { clear_description: true }
                )
              }}
              value={taskData.description ?? ""}
            />
            <TaskAnnotationsEditor
              annotations={taskData.annotations}
              canWrite={taskWritable}
              projectSlug={projectSlug}
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
          </div>
          <div className={mobilePanelClass(activeMobileTab, "links")}>
            <TaskLinksEditor
              canWrite={taskWritable}
              links={taskData.links}
              projectSlug={projectSlug}
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
          </div>
        </div>
        <div className={mobilePanelClass(activeMobileTab, "properties")}>
          <TaskPropertyPanel
            canWrite={taskWritable}
            projectSlug={projectSlug}
            task={taskData}
            taskRef={taskRef}
            workspaceSlug={workspaceSlug}
          />
        </div>
      </div>
    </div>
  )
}

function MobileDetailTabs({
  active,
  onChange,
}: {
  active: MobileDetailTab
  onChange: (tab: MobileDetailTab) => void
}) {
  const { t } = useTranslation()
  const tabs: Array<{ label: string; value: MobileDetailTab }> = [
    { label: t("projectReadonly.attributes"), value: "properties" },
    { label: t("projectReadonly.annotations"), value: "annotations" },
    { label: t("projectReadonly.links"), value: "links" },
  ]
  return (
    <Tabs
      className="md:hidden"
      onValueChange={(value) => onChange(value as MobileDetailTab)}
      value={active}
    >
      <TabsList
        aria-label={t("projectReadonly.detailTabs")}
        className="grid h-auto w-full grid-cols-3 gap-1 border bg-card p-1"
      >
        {tabs.map((tab) => (
          <TabsTrigger className="h-8" key={tab.value} value={tab.value}>
            {tab.label}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}

function TaskDescriptionBlock({
  canWrite,
  onSave,
  value,
}: {
  canWrite: boolean
  onSave: (value: string) => Promise<void> | void
  value: string
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function openEditor() {
    setDraft(value)
    setError(null)
    setEditing(true)
  }

  async function save() {
    if (saving) {
      return
    }
    setSaving(true)
    setError(null)
    try {
      await onSave(draft)
      setEditing(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <section className="space-y-3 border bg-card p-4">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-medium">
            {t("projectReadonly.description")}
          </h2>
          <Button
            disabled={!canWrite}
            onClick={openEditor}
            size="sm"
            type="button"
            variant="outline"
          >
            {t("projectReadonly.editDescription")}
          </Button>
        </div>
        {value ? (
          <div className="max-w-3xl whitespace-pre-wrap text-sm leading-6 text-muted-foreground">
            {value}
          </div>
        ) : (
          <div className="text-sm text-muted-foreground">
            {t("projectReadonly.addDescription")}
          </div>
        )}
      </section>
      <Dialog open={editing} onOpenChange={setEditing}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("projectReadonly.editDescriptionTitle")}</DialogTitle>
            <DialogDescription>
              {t("projectReadonly.editDescriptionDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="task-description-editor">
              {t("projectReadonly.taskDescription")}
            </Label>
            <Textarea
              className="min-h-64 resize-y"
              id="task-description-editor"
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
                  event.preventDefault()
                  void save()
                }
              }}
              value={draft}
            />
          </div>
          {error ? <div className="text-sm text-destructive">{error}</div> : null}
          <DialogFooter>
            <Button
              onClick={() => setEditing(false)}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button
              disabled={saving}
              onClick={() => {
                void save()
              }}
              type="button"
            >
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function mobilePanelClass(active: MobileDetailTab, tab: MobileDetailTab): string {
  return cn(active === tab ? "block" : "hidden", "md:block")
}

function TaskDetailSkeleton() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-10 w-96 max-w-full" />
      <div className="grid gap-2 md:grid-cols-3">
        {Array.from({ length: 6 }).map((_, index) => (
          <Skeleton className="h-20" key={index} />
        ))}
      </div>
    </div>
  )
}

function taskBelongsToProject(task: ProjectTask, projectSlug: string): boolean {
  return !task.project || task.project === projectSlug
}

function isWritableTaskStatus(status: string): boolean {
  return status !== "completed" && status !== "deleted"
}
