import { useState, type KeyboardEvent } from "react"
import { PlusIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

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
import { InlineDatePicker } from "../shared/inline-date-picker"

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
  const { t } = useTranslation()
  const [title, setTitle] = useState("")
  const [priority, setPriority] = useState("")
  const [due, setDue] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const createTask = useCreateTaskMutation(workspaceSlug, projectSlug, filters)

  if (!canCreate || isClosedProjectStatus(projectStatus)) {
    return null
  }

  const submit = async () => {
    const normalizedTitle = title.trim()
    if (!normalizedTitle) {
      setError(t("projectReadonly.taskTitleRequired"))
      return
    }
    setError(null)
    try {
      await createTask.mutateAsync({
        ...(due !== null ? { due } : {}),
        ...(priority ? { priority } : {}),
        project: projectSlug,
        title: normalizedTitle,
      })
      setTitle("")
      setPriority("")
      setDue(null)
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
            aria-label={t("projectReadonly.newTaskTitle")}
            className="pl-8"
            disabled={createTask.isPending}
            onChange={(event) => {
              setTitle(event.target.value)
              setError(null)
            }}
            onKeyDown={onTitleKeyDown}
            placeholder={t("projectReadonly.newTaskTitlePlaceholder")}
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
          <SelectTrigger
            aria-label={t("projectReadonly.taskPriorityPlain")}
            className="w-full"
          >
            <SelectValue placeholder={t("projectReadonly.priority")} />
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
          ariaLabel={t("projectReadonly.dueDate")}
          className="h-9"
          disabled={createTask.isPending}
          emptyLabel={t("projectReadonly.dueDate")}
          onSave={(next) => {
            setDue(next)
            setError(null)
          }}
          value={due}
        />
        <Button
          disabled={createTask.isPending}
          onClick={() => {
            void submit()
          }}
          type="button"
        >
          {t("projectReadonly.createTask")}
        </Button>
      </div>
      {error ? <p className="mt-2 text-xs text-destructive">{error}</p> : null}
    </section>
  )
}
