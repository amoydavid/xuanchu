import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PlayIcon, SquareIcon, Trash2Icon, CheckIcon, RotateCcwIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { ProjectTask } from "../api/task-api"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"
import { useTaskActionMutation } from "../hooks/use-task-mutations"
import { skipTaskSeriesOccurrence } from "../api/task-series-api"

type TaskActionBarProps = {
  // 权限层面的可写（不含任务状态判断）。
  permissionCanWrite: boolean
  projectSlug: string
  task: ProjectTask
  taskRef: string
  workspaceSlug: string
}

export function TaskActionBar({
  permissionCanWrite,
  projectSlug,
  task,
  taskRef,
  workspaceSlug,
}: TaskActionBarProps) {
  const [confirmDelete, setConfirmDelete] = useState(false)
  const start = useTaskActionMutation(workspaceSlug, projectSlug, "start")
  const stop = useTaskActionMutation(workspaceSlug, projectSlug, "stop")
  const done = useTaskActionMutation(workspaceSlug, projectSlug, "done")
  const reopen = useTaskActionMutation(workspaceSlug, projectSlug, "reopen")
  const remove = useTaskActionMutation(workspaceSlug, projectSlug, "delete")
	const queryClient = useQueryClient()
	const occurrence = task.recurrence_info
	const skip = useMutation({
		mutationFn: () => skipTaskSeriesOccurrence(workspaceSlug, occurrence!.series_id, taskRef),
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["project-task"] })
			await queryClient.invalidateQueries({ queryKey: ["project-tasks"] })
			await queryClient.invalidateQueries({ queryKey: ["task-series"] })
		},
	})
  const completed = task.status === "completed"
  const deleted = task.status === "deleted"
  const pending =
    start.isPending ||
    stop.isPending ||
    done.isPending ||
    reopen.isPending ||
		remove.isPending || skip.isPending

  // deleted 是真正的终态，无可执行动作；无权限也直接隐藏。
  if (deleted || !permissionCanWrite) {
    return null
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {!completed && task.start ? (
          <Button
            disabled={pending}
            onClick={() => {
              void stop.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <SquareIcon />
            停止
          </Button>
        ) : null}
        {!completed && !task.start ? (
          <Button
            disabled={pending}
            onClick={() => {
              void start.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <PlayIcon />
            开始
          </Button>
        ) : null}
        {!completed ? (
          <Button
            disabled={pending}
            onClick={() => {
              void done.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <CheckIcon />
			{occurrence ? "完成本次" : "完成"}
          </Button>
        ) : null}
        {completed ? (
          <Button
            disabled={pending}
            onClick={() => {
              void reopen.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <RotateCcwIcon />
			{occurrence ? "重新打开本次" : "重新打开"}
          </Button>
        ) : (
          <Button
            disabled={pending}
            onClick={() => setConfirmDelete(true)}
            size="sm"
            type="button"
            variant="destructive"
          >
			<Trash2Icon />
			{occurrence ? "跳过本次" : "删除"}
          </Button>
        )}
      </div>
      <DestructiveConfirmDialog
		confirmLabel={occurrence ? "跳过本次" : "删除任务"}
		description={occurrence ? "本次实例会标记为已跳过，不影响后续循环。" : "删除后任务会从当前项目视图中移除。这个操作不可撤销。"}
		onConfirm={async () => {
			if (occurrence) await skip.mutateAsync()
			else await remove.mutateAsync(taskRef)
          setConfirmDelete(false)
        }}
        onOpenChange={setConfirmDelete}
        open={confirmDelete}
		pending={remove.isPending || skip.isPending}
		title={occurrence ? "确认跳过本次" : "确认删除任务"}
      />
    </>
  )
}
