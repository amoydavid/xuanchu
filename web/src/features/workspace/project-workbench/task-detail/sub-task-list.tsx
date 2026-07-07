import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { MarkdownEditor } from "@/components/markdown"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import type { ProjectWorkbenchAssignee } from "../api/project-api"
import type { ProjectTask } from "../api/task-api"
import { useCreateSubTaskMutation } from "../hooks/use-task-mutations"
import { useTaskChildrenQuery } from "../hooks/use-task-detail-data"
import { InlineDatePicker } from "../shared/inline-date-picker"
import { AssigneePicker } from "./assignee-picker"
import { TagPicker } from "./tag-picker"

const PRIORITIES = ["H", "M", "L"] as const

type SubTaskListProps = {
  canCreate: boolean
  parentRef: string
  parentUUID: string
  projectSlug: string
  workspaceSlug: string
}

export function SubTaskList({
  canCreate,
  parentRef,
  parentUUID,
  projectSlug,
  workspaceSlug,
}: SubTaskListProps) {
  const { t } = useTranslation()
  const [showCompleted, setShowCompleted] = useState(false)
  // composer 默认不显示，点击「添加子任务」后展开（spec §7.2）。
  const [composerOpen, setComposerOpen] = useState(false)

  // 分别请求 open 与 all：默认只展示 open，展开已完成时用 all。
  // 这样切换 showCompleted 不会因为缓存切换而闪烁空态。
  const openChildren = useTaskChildrenQuery(workspaceSlug, parentRef, false)
  const allChildren = useTaskChildrenQuery(workspaceSlug, parentRef, true)

  const children = showCompleted ? allChildren.data : openChildren.data
  const isLoading = showCompleted
    ? allChildren.isPending
    : openChildren.isPending
  const error = showCompleted ? allChildren.error : openChildren.error
  const refetch = showCompleted ? allChildren.refetch : openChildren.refetch

  const openCount = openChildren.data?.length ?? 0
  const totalCount = allChildren.data?.length ?? 0
  const completedCount = Math.max(totalCount - openCount, 0)

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium">{t("taskDetail.subTasks")}</h2>
        {canCreate && !composerOpen ? (
          <Button
            onClick={() => setComposerOpen(true)}
            size="sm"
            variant="ghost"
          >
            {t("taskDetail.addSubTask")}
          </Button>
        ) : null}
      </div>

      {canCreate && composerOpen ? (
        <SubTaskComposer
          onCancel={() => setComposerOpen(false)}
          parentRef={parentRef}
          parentUUID={parentUUID}
          projectSlug={projectSlug}
          workspaceSlug={workspaceSlug}
        />
      ) : null}

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 2 }).map((_, i) => (
            <Skeleton className="h-8" key={i} />
          ))}
        </div>
      ) : error ? (
        <div className="space-y-2 text-sm text-destructive">
          <p>{t("taskDetail.subTasksError")}</p>
          <Button onClick={() => void refetch()} size="sm" variant="outline">
            {t("common.retry")}
          </Button>
        </div>
      ) : !children || children.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {canCreate
            ? t("taskDetail.subTasksEmpty")
            : t("taskDetail.subTasksNone")}
        </p>
      ) : (
        <ul className="space-y-1">
          {children.map((child) => (
            <SubTaskRow
              key={child.uuid}
              projectSlug={projectSlug}
              task={child}
              workspaceSlug={workspaceSlug}
            />
          ))}
        </ul>
      )}

      {completedCount > 0 ? (
        <Button
          onClick={() => setShowCompleted((v) => !v)}
          size="sm"
          variant="ghost"
        >
          {showCompleted
            ? t("taskDetail.hideCompleted")
            : t("taskDetail.showCompleted", { count: completedCount })}
        </Button>
      ) : null}
    </section>
  )
}

function SubTaskRow({
  projectSlug,
  task,
  workspaceSlug,
}: {
  projectSlug: string
  task: ProjectTask
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  // task.project 优先（子任务自身 project），其次父任务传入的 projectSlug。
  const projectSegment = task.project || projectSlug
  const slug = task.task_slug || task.uuid
  const href = projectSegment
    ? `/workspaces/${workspaceSlug}/projects/${projectSegment}/tasks/${slug}`
    : `/workspaces/${workspaceSlug}/tasks/${slug}`
  return (
    <li className="flex items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted/50">
      <Badge variant="outline">{taskStatusLabel(task.status, t)}</Badge>
      <a className="min-w-0 flex-1 truncate hover:underline" href={href}>
        {task.title}
      </a>
      {task.priority ? (
        <Badge variant="secondary">{task.priority}</Badge>
      ) : null}
      {task.assignees && task.assignees.length > 0 ? (
        <span className="truncate text-xs text-muted-foreground">
          {task.assignees
            .map((a: ProjectWorkbenchAssignee) => a.display_name || a.name || a.user_id)
            .join(", ")}
        </span>
      ) : null}
      {task.due ? (
        <span className="text-xs text-muted-foreground">
          {formatDue(task.due)}
        </span>
      ) : null}
    </li>
  )
}

function formatDue(due: string | number): string {
  // due 可能是 unix 秒（number）或 ISO（string）。
  const ms = typeof due === "number" ? due * 1000 : Date.parse(due)
  if (!Number.isFinite(ms)) return String(due)
  return new Date(ms).toLocaleDateString()
}

function SubTaskComposer({
  onCancel,
  parentRef,
  parentUUID,
  projectSlug,
  workspaceSlug,
}: {
  onCancel: () => void
  parentRef: string
  parentUUID: string
  projectSlug: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const createSubTask = useCreateSubTaskMutation(
    workspaceSlug,
    projectSlug,
    parentRef
  )
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [priority, setPriority] = useState("")
  const [due, setDue] = useState<number | null>(null)
  const [assignees, setAssignees] = useState<ProjectWorkbenchAssignee[]>([])
  const [tags, setTags] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)

  // 连续创建：提交后清空标题与临时字段，但保持 composer 打开（spec §9.2）。
  const resetFields = () => {
    setTitle("")
    setDescription("")
    setPriority("")
    setDue(null)
    setAssignees([])
    setTags([])
  }

  const submit = async () => {
    const normalizedTitle = title.trim()
    if (!normalizedTitle) {
      setError(t("taskDetail.subTaskTitleRequired"))
      return
    }
    setError(null)
    const assigneeIDs = assignees
      .map((a) => a.user_id ?? a.id)
      .filter((v): v is string => Boolean(v))
    try {
      await createSubTask.mutateAsync({
        title: normalizedTitle,
        parent: parentUUID,
        project: projectSlug,
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(priority ? { priority } : {}),
        ...(due !== null ? { due } : {}),
        ...(assigneeIDs.length > 0 ? { assignees: assigneeIDs } : {}),
        ...(tags.length > 0 ? { tags } : {}),
      })
      // 连续创建：清空草稿，composer 保持打开。
      resetFields()
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : String(err)
      setError(message)
      // 失败时保留草稿（spec §10）。
    }
  }

  return (
    <div className="space-y-3 rounded border p-3">
      <Input
        aria-label={t("taskDetail.subTaskTitle")}
        autoFocus
        onChange={(e) => {
          setTitle(e.target.value)
          setError(null)
        }}
        onKeyDown={(e) => {
          // 标题输入框 Enter 直接提交并保持 composer 打开（连续创建，spec §9.2）。
          // Shift+Enter 交给默认行为（不拦截）。
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault()
            void submit()
          }
        }}
        placeholder={t("taskDetail.subTaskTitlePlaceholder")}
        value={title}
      />
      <MarkdownEditor
        ariaLabel={t("taskDetail.subTaskDescription")}
        minHeight={120}
        onChange={setDescription}
        onModEnter={() => {
          // 多行描述编辑器用 Cmd/Ctrl+Enter 提交（spec §9.2）。
          void submit()
        }}
        placeholder={t("taskDetail.subTaskDescriptionPlaceholder")}
        value={description}
      />
      <p className="text-xs text-muted-foreground">
        {t("taskDetail.subTaskInheritProject", { project: projectSlug })}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Select onValueChange={setPriority} value={priority}>
          <SelectTrigger
            aria-label={t("taskDetail.subTaskPriority")}
            className="w-28"
          >
            <SelectValue placeholder={t("taskDetail.subTaskPriority")} />
          </SelectTrigger>
          <SelectContent>
            {PRIORITIES.map((item) => (
              <SelectItem key={item} value={item}>
                {item}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <InlineDatePicker
          ariaLabel={t("taskDetail.subTaskDue")}
          boundary="end"
          emptyLabel={t("taskDetail.subTaskDue")}
          onSave={setDue}
          value={due}
        />
        <AssigneePicker
          onSave={async (nextIDs) => {
            // picker 回传选中的 id 列表；用最小对象保留，避免重新拉取成员详情。
            setAssignees(nextIDs.map((id) => ({ user_id: id })))
          }}
          value={assignees}
          workspaceSlug={workspaceSlug}
        />
        <TagPicker
          onSave={async (next) => setTags(next)}
          projectSlug={projectSlug}
          value={tags}
          workspaceSlug={workspaceSlug}
        />
        <Button
          disabled={createSubTask.isPending}
          onClick={onCancel}
          size="sm"
          type="button"
          variant="outline"
        >
          {t("common.cancel")}
        </Button>
        <Button
          disabled={createSubTask.isPending}
          onClick={() => void submit()}
          size="sm"
          type="button"
        >
          {t("taskDetail.createSubTask")}
        </Button>
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  )
}
