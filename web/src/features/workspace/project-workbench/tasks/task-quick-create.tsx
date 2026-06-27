import { useState, type KeyboardEvent } from "react"
import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { ProjectTaskFilterParams } from "../api/project-api"
import { useCreateTaskMutation } from "../hooks/use-task-mutations"
import { isClosedProjectStatus } from "../project/project-status-menu"

type TaskQuickCreateProps = {
  canCreate: boolean
  projectSlug: string
  projectStatus: string
  workspaceSlug: string
  filters?: ProjectTaskFilterParams | string
}

const PRIORITIES = ["H", "M", "L"] as const

export function TaskQuickCreate({
  canCreate,
  filters,
  projectSlug,
  projectStatus,
  workspaceSlug,
}: TaskQuickCreateProps) {
  const [title, setTitle] = useState("")
  const [priority, setPriority] = useState("")
  const [due, setDue] = useState("")
  const [error, setError] = useState<string | null>(null)
  const createTask = useCreateTaskMutation(workspaceSlug, projectSlug, filters)

  if (!canCreate || isClosedProjectStatus(projectStatus)) {
    return null
  }

  const submit = async () => {
    const normalizedTitle = title.trim()
    if (!normalizedTitle) {
      setError("任务标题不能为空")
      return
    }
    setError(null)
    try {
      await createTask.mutateAsync({
        ...(due ? { due: dateToUnix(due) } : {}),
        ...(priority ? { priority } : {}),
        project: projectSlug,
        title: normalizedTitle,
      })
      setTitle("")
      setPriority("")
      setDue("")
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const onTitleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter") {
      event.preventDefault()
      void submit()
    }
  }

  return (
    <section className="border bg-card p-3">
      <div className="grid gap-2 md:grid-cols-[minmax(12rem,1fr)_8rem_10rem_auto] md:items-center">
        <div className="relative">
          <PlusIcon className="pointer-events-none absolute top-1/2 left-2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="新任务标题"
            className="pl-8"
            disabled={createTask.isPending}
            onChange={(event) => {
              setTitle(event.target.value)
              setError(null)
            }}
            onKeyDown={onTitleKeyDown}
            placeholder="输入任务标题..."
            value={title}
          />
        </div>
        <Select
          disabled={createTask.isPending}
          onValueChange={(value) => {
            setPriority(value)
            setError(null)
          }}
          value={priority}
        >
          <SelectTrigger aria-label="任务优先级" className="w-full">
            <SelectValue placeholder="优先级" />
          </SelectTrigger>
          <SelectContent>
            {PRIORITIES.map((item) => (
              <SelectItem key={item} value={item}>
                {item}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          aria-label="截止日期"
          disabled={createTask.isPending}
          onChange={(event) => {
            setDue(event.target.value)
            setError(null)
          }}
          type="date"
          value={due}
        />
        <Button
          disabled={createTask.isPending}
          onClick={() => {
            void submit()
          }}
          type="button"
        >
          创建任务
        </Button>
      </div>
      {error ? <p className="mt-2 text-xs text-destructive">{error}</p> : null}
    </section>
  )
}

function dateToUnix(value: string): number {
  return Math.floor(new Date(`${value}T00:00:00Z`).getTime() / 1000)
}
