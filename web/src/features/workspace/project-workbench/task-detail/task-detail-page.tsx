import { useEffect, useState } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { MarkdownEditor, MarkdownView } from "@/components/markdown"
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
import { ActivitySection } from "./activity-section"
import { TaskLinksEditor } from "./task-links-editor"
import {
  embeddedImageAttachmentIDs,
  TaskAttachmentPanel,
} from "@/features/workspace/attachments/task-attachment-panel"
import { useDescriptionDraftCleanup } from "@/features/workspace/attachments/use-description-draft-cleanup"
import { suggestContentReferences } from "@/features/workspace/content-references"
import { resolutionToMenuItem } from "@/components/markdown/reference-suggestion-menu"
import { TaskPropertyPanel } from "./task-property-panel"
import { SubTaskList } from "./sub-task-list"
import { recurrenceRuleLabel } from "../task-series/recurrence-preview"
import {
  canonicalTaskRouteRef,
  isCanonicalTaskSlug,
  taskDisplayRef,
} from "../tasks/task-reference"
import { RecurrenceContextAlert } from "./recurrence-context-alert"

type TaskDetailPageProps = {
  myTasksReturnSearch?: string
  projectClosed?: boolean
  projectSlug?: string
  returnToHome?: boolean
  taskRef: string
  workspaceSlug: string
}

type MobileDetailTab = "description" | "subtasks" | "properties" | "activity"

export function TaskDetailPage({
  myTasksReturnSearch,
  projectClosed = false,
  projectSlug,
  returnToHome = false,
  taskRef,
  workspaceSlug,
}: TaskDetailPageProps) {
  return (
    <EditFeedbackProvider>
      <TaskDetailPageContent
        myTasksReturnSearch={myTasksReturnSearch}
        projectClosed={projectClosed}
        projectSlug={projectSlug}
        returnToHome={returnToHome}
        taskRef={taskRef}
        workspaceSlug={workspaceSlug}
      />
    </EditFeedbackProvider>
  )
}

function TaskDetailPageContent({
  myTasksReturnSearch,
  projectClosed = false,
  projectSlug,
  returnToHome = false,
  taskRef,
  workspaceSlug,
}: TaskDetailPageProps) {
  const { i18n, t } = useTranslation()
  const me = useMe()
  const navigate = useNavigate()
  const canWrite = canTaskWrite({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const task = useTaskDetailQuery(workspaceSlug, taskRef)
  const [activeMobileTab, setActiveMobileTab] =
    useState<MobileDetailTab>("description")
  const [linksOpen, setLinksOpen] = useState(false)
  const [subTasksOpen, setSubTasksOpen] = useState(false)
  const [hasVisibleSubTaskContent, setHasVisibleSubTaskContent] =
    useState(false)
  const handleSubTaskContentVisibilityChange = (visible: boolean) => {
    setHasVisibleSubTaskContent(visible)
    if (!visible && !subTasksOpen) {
      setActiveMobileTab((current) =>
        current === "subtasks" ? "description" : current
      )
    }
  }

  // projectSlug 优先取路由参数（项目内进入），兜底取 task 自身 project 字段（/my-tasks / /tasks/:ref 进入）。
  // 注意：这里只读 task.data 的 project 字段用于派生 effectiveProjectSlug，
  // 不在 render 后续直接消费 taskData；render 主体的 taskData 在 isPending/isError 之后重新取，
  // 让 TS 能正确 narrow 为非 undefined。
  const taskDataForProject = task.data
  const effectiveProjectSlug =
    projectSlug ??
    (taskDataForProject && typeof taskDataForProject.project === "string"
      ? taskDataForProject.project
      : undefined)

  // useModifyTaskMutation 必须在顶层调用，保证 hooks 顺序稳定。
  const modifyTask = useModifyTaskMutation(
    workspaceSlug,
    effectiveProjectSlug ?? "",
    taskRef
  )
  const projectHref = effectiveProjectSlug
    ? `/workspaces/${workspaceSlug}/projects/${effectiveProjectSlug}`
    : undefined
  const projectTasksHref: string | undefined = projectHref
    ? `${projectHref}/tasks`
    : undefined
  const myTasksHref =
    myTasksReturnSearch === undefined
      ? undefined
      : normalizedMyTasksHref(myTasksReturnSearch)
  const homeHref = returnToHome ? "/" : undefined
  const returnHref = homeHref ?? myTasksHref ?? projectHref

  useEffect(() => {
    const loaded = task.data
    if (!loaded?.recurrence_info || !loaded.task_slug) return
    const canonicalRef = canonicalTaskRouteRef(loaded)
    if (
      !canonicalRef ||
      canonicalRef === taskRef ||
      !isCanonicalTaskSlug(canonicalRef) ||
      !effectiveProjectSlug ||
      loaded.project !== effectiveProjectSlug
    ) {
      return
    }
    void navigate({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef",
      params: {
        workspaceSlug,
        projectSlug: effectiveProjectSlug,
        taskRef: canonicalRef,
      },
      ...(returnToHome
        ? { search: { from: "home" } }
        : myTasksReturnSearch === undefined
          ? {}
          : {
              search: {
                from: "my-tasks",
                my_tasks_search: myTasksReturnSearch,
              },
            }),
      replace: true,
    })
  }, [
    effectiveProjectSlug,
    myTasksReturnSearch,
    navigate,
    returnToHome,
    task.data,
    taskRef,
    workspaceSlug,
  ])

  useEffect(() => {
    const loaded = task.data
    if (!loaded) return
    const previousTitle = document.title
    const reference = taskDisplayRef(loaded, i18n.language) || taskRef
    const nextTitle = `${reference} · ${loaded.title}`
    document.title = nextTitle
    return () => {
      if (document.title === nextTitle) document.title = previousTitle
    }
  }, [i18n.language, task.data, taskRef])

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
        {returnHref ? (
          <Button asChild className="mt-5" variant="outline">
            <a href={returnHref}>
              {homeHref
                ? t("taskDetail.backToHome")
                : myTasksHref
                  ? t("taskDetail.backToMyTasks")
                  : t("projectReadonly.backToProject")}
            </a>
          </Button>
        ) : null}
      </section>
    )
  }

  const taskData = task.data
  if (!taskData) {
    // 已经过 isPending / isError，data 必然存在；防御性兜底。
    return <TaskDetailSkeleton />
  }
  const pageCanWrite = canWrite && !projectClosed
  const taskWritable = pageCanWrite && isWritableTaskStatus(taskData.status)
  // 关联资料和子任务是普通任务的补充内容：必须同时满足可写、未完成/未删除、非循环实例。
  // 该规则一次派生后下发至顶部入口和各区块，避免入口与局部操作权限漂移。
  const canCreateRelatedContent = taskWritable && !taskData.recurrence_info
  // 仅在显式 projectSlug（项目内进入）时校验归属；从全局入口进入不做该严格校验。
  if (projectSlug && !taskBelongsToProject(taskData, projectSlug)) {
    return (
      <section className="max-w-2xl border bg-card p-6">
        <h1 className="text-xl font-semibold tracking-normal">
          {t("projectReadonly.taskNotFoundTitle")}
        </h1>
        {projectHref ? (
          <Button asChild className="mt-5" variant="outline">
            <a href={projectHref}>{t("projectReadonly.backToProject")}</a>
          </Button>
        ) : null}
      </section>
    )
  }

  return (
    <div className="space-y-5">
      <section className="border-b pb-4">
        <nav className="text-xs text-muted-foreground">
          <Link className="hover:text-foreground" to="/projects">
            {workspaceSlug}
          </Link>
          {effectiveProjectSlug ? (
            <>
              {" / "}
              <Link className="hover:text-foreground" to={projectHref}>
                {effectiveProjectSlug}
              </Link>
              {" / "}
              <Link className="hover:text-foreground" to={projectTasksHref}>
                {t("projectSubpages.tasks")}
              </Link>
            </>
          ) : null}
          {" / "}
          <span>{taskDisplayRef(taskData, i18n.language) || taskRef}</span>
          {homeHref || myTasksHref ? (
            <>
              {" · "}
              <a className="hover:text-foreground" href={returnHref}>
                {homeHref
                  ? t("taskDetail.backToHome")
                  : t("taskDetail.backToMyTasks")}
              </a>
            </>
          ) : null}
        </nav>
        <div className="mt-3 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div className="min-w-0 flex-1">
            <h1 aria-label={taskData.title}>
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
            </h1>
            <div className="mt-3 flex flex-wrap gap-2">
              <Badge variant="outline">
                {taskData.recurrence_info?.materialization === "projected"
                  ? t("taskSeries.occurrence.projected")
                  : taskData.recurrence_info && taskData.status === "deleted"
                    ? t("taskSeries.occurrence.skipped")
                    : taskStatusLabel(taskData.status, t)}
              </Badge>
              {taskData.priority ? (
                <Badge variant="outline">{taskData.priority}</Badge>
              ) : null}
              {taskData.task_slug ? (
                <Badge variant="outline">{taskData.task_slug}</Badge>
              ) : null}
              {taskData.recurrence_info ? (
                <Badge variant="outline" data-testid="recurrence-info-badge">
                  {t("taskSeries.occurrence.badge", {
                    rule: recurrenceRuleLabel(taskData.recurrence_info.rule, t),
                  })}
                </Badge>
              ) : null}
            </div>
          </div>
          <div className="flex shrink-0 items-start md:items-end">
            <TaskActionBar
              canCreateRelatedContent={canCreateRelatedContent}
              onAddLink={() => setLinksOpen(true)}
              onAddSubTask={() => {
                setSubTasksOpen(true)
                setActiveMobileTab("subtasks")
              }}
              permissionCanWrite={pageCanWrite}
              projectSlug={effectiveProjectSlug ?? ""}
              myTasksReturnSearch={myTasksReturnSearch}
              task={taskData}
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
          </div>
        </div>
        <div className="mt-4 w-full">
          <RecurrenceContextAlert
            projectSlug={effectiveProjectSlug}
            task={taskData}
            workspaceSlug={workspaceSlug}
          />
        </div>
      </section>

      <MobileDetailTabs
        active={activeMobileTab}
        onChange={setActiveMobileTab}
        showSubTasks={hasVisibleSubTaskContent}
      />

      <div className="grid gap-5 md:grid-cols-[minmax(0,1fr)_280px]">
        <div className="contents md:block md:min-w-0 md:space-y-5">
          {/* 正文 + 关联资源：spec §7.1 主叙事区顶部，links 作为正文附近的关联资源 */}
          <div
            className={mobilePanelClass(activeMobileTab, "description")}
            data-testid="mobile-description-panel"
          >
            <TaskDescriptionBlock
              canWrite={taskWritable}
              inherited={
                taskData.recurrence_info?.materialization === "projected"
              }
              onSave={async (description) => {
                await modifyTask.mutateAsync(
                  description ? { description } : { clear_description: true }
                )
              }}
              value={taskData.description ?? ""}
              taskRef={taskRef}
              projectRef={effectiveProjectSlug}
              workspaceSlug={workspaceSlug}
            />
            <TaskLinksEditor
              canWrite={canCreateRelatedContent}
              links={taskData.links}
              onOpenChange={setLinksOpen}
              open={linksOpen}
              projectSlug={effectiveProjectSlug ?? ""}
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
            <TaskAttachmentPanel
              workspaceSlug={workspaceSlug}
              taskRef={taskRef}
              canWrite={taskWritable}
              embeddedImageAttachmentIDs={embeddedImageAttachmentIDs(
                taskData.description ?? ""
              )}
            />
          </div>
          {/* 子任务 */}
          <div className={mobilePanelClass(activeMobileTab, "subtasks")}>
            <SubTaskList
              canCreate={canCreateRelatedContent}
              onContentVisibilityChange={handleSubTaskContentVisibilityChange}
              onOpenChange={setSubTasksOpen}
              open={subTasksOpen}
              parentRef={taskRef}
              parentUUID={taskData.uuid ?? taskRef}
              projectSlug={effectiveProjectSlug ?? ""}
              workspaceSlug={workspaceSlug}
            />
          </div>
          {/* 活动：注解 + 变更历史统一时间轴（过渡态，spec §9.4） */}
          <div className={mobilePanelClass(activeMobileTab, "activity")}>
            <ActivitySection
              annotations={taskData.annotations}
              canWrite={taskWritable}
              projectSlug={effectiveProjectSlug ?? ""}
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
          </div>
        </div>
        <div className={mobilePanelClass(activeMobileTab, "properties")}>
          <TaskPropertyPanel
            canWrite={taskWritable}
            projectSlug={effectiveProjectSlug ?? ""}
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
  showSubTasks,
}: {
  active: MobileDetailTab
  onChange: (tab: MobileDetailTab) => void
  showSubTasks: boolean
}) {
  const { t } = useTranslation()
  const tabs: Array<{ label: string; value: MobileDetailTab }> = [
    { label: t("taskDetail.description"), value: "description" },
    { label: t("projectReadonly.attributes"), value: "properties" },
    { label: t("taskDetail.activity"), value: "activity" },
  ]
  if (showSubTasks) {
    tabs.splice(1, 0, { label: t("taskDetail.subTasks"), value: "subtasks" })
  }
  return (
    <Tabs
      className="md:hidden"
      onValueChange={(value) => onChange(value as MobileDetailTab)}
      value={active}
    >
      <TabsList
        aria-label={t("projectReadonly.detailTabs")}
        className={cn(
          "grid h-auto w-full gap-1 border bg-card p-1",
          showSubTasks ? "grid-cols-4" : "grid-cols-3"
        )}
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
  inherited,
  onSave,
  value,
  taskRef,
  projectRef,
  workspaceSlug,
}: {
  canWrite: boolean
  inherited: boolean
  onSave: (value: string) => Promise<void> | void
  value: string
  taskRef: string
  projectRef?: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pendingUploads, setPendingUploads] = useState(0)
  const [failedUploads, setFailedUploads] = useState(0)
  const draftCleanup = useDescriptionDraftCleanup(workspaceSlug)

  function openEditor() {
    setDraft(value)
    setError(null)
    setPendingUploads(0)
    setFailedUploads(0)
    draftCleanup.reset()
    setEditing(true)
  }

  async function save() {
    if (saving || pendingUploads > 0 || failedUploads > 0) {
      return
    }
    setSaving(true)
    setError(null)
    try {
      await onSave(draft)
      draftCleanup.reset()
      setEditing(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  async function cancel() {
    // best-effort 清理本次会话创建的 draft；失败不阻塞关闭。
    await draftCleanup.cleanup()
    setEditing(false)
  }

  return (
    <>
      <section className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-medium">
              {t("projectReadonly.description")}
            </h2>
            {inherited ? (
              <div className="text-xs text-muted-foreground">
                {t("taskSeries.occurrence.inherited")}
              </div>
            ) : null}
          </div>
          <Button
            disabled={!canWrite}
            onClick={openEditor}
            size="sm"
            type="button"
            variant="ghost"
          >
            {t("projectReadonly.editDescription")}
          </Button>
        </div>
        {value ? (
          <MarkdownView
            attachmentContext={{ workspaceSlug, taskRef }}
            className="max-w-3xl text-sm leading-6 text-muted-foreground"
            headingOffset={1}
          >
            {value}
          </MarkdownView>
        ) : (
          <div className="text-sm text-muted-foreground">
            {t("projectReadonly.addDescription")}
          </div>
        )}
      </section>
      <Dialog open={editing} onOpenChange={setEditing}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t("projectReadonly.editDescriptionTitle")}
            </DialogTitle>
            <DialogDescription>
              {t("projectReadonly.editDescriptionDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="task-description-editor">
              {t("projectReadonly.taskDescription")}
            </Label>
            <MarkdownEditor
              attachmentContext={{
                workspaceSlug,
                taskRef,
                projectRef,
                fetchSuggestions: async ({ kind, query, signal }) => {
                  const resolved = await suggestContentReferences({
                    type: kind,
                    query,
                    project: projectRef,
                    limit: 20,
                  }, { signal })
                  return resolved.map(resolutionToMenuItem).filter((item) => item !== null)
                },
              }}
              ariaLabel={t("projectReadonly.taskDescription")}
              minHeight={260}
              onAttachmentPendingChange={setPendingUploads}
              onAttachmentFailureChange={setFailedUploads}
              onDraftAttachmentCreated={draftCleanup.track}
              onChange={(markdown) => setDraft(markdown)}
              onModEnter={() => {
                void save()
              }}
              value={draft}
            />
          </div>
          {error ? (
            <div className="text-sm text-destructive">{error}</div>
          ) : null}
          {failedUploads > 0 ? (
            <div className="text-sm text-destructive">图片上传失败，请删除占位内容后重试粘贴。</div>
          ) : null}
          <DialogFooter>
            <Button
              onClick={() => {
                void cancel()
              }}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button
              disabled={saving || pendingUploads > 0 || failedUploads > 0}
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

function mobilePanelClass(
  active: MobileDetailTab,
  tab: MobileDetailTab
): string {
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

function normalizedMyTasksHref(raw: string): string {
  const input = new URLSearchParams(raw)
  const output = new URLSearchParams()
  for (const key of ["priority", "project", "q", "sort", "tab", "task_type"]) {
    const value = input.get(key)
    if (value) output.set(key, value)
  }
  const query = output.toString()
  return query ? `/my-tasks?${query}` : "/my-tasks"
}
