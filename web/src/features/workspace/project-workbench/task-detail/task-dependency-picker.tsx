import { useMemo, useState } from "react"
import { PlusIcon, XIcon } from "lucide-react"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import {
  getProjectTasks,
  type ProjectWorkbenchTask,
  type ProjectWorkbenchTaskRef,
} from "../api/project-api"
import { useEditFeedback } from "../shared/edit-feedback"

type TaskDependencyPickerProps = {
  disabled?: boolean
  onSave: (depends: string[]) => Promise<void> | void
  projectSlug: string
  refs?: ProjectWorkbenchTaskRef[]
  taskUUID: string
  value?: string[]
  workspaceSlug: string
}

export function TaskDependencyPicker({
  disabled = false,
  onSave,
  projectSlug,
  refs = [],
  taskUUID,
  value = [],
  workspaceSlug,
}: TaskDependencyPickerProps) {
  const { t } = useTranslation()
  const feedback = useEditFeedback()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [selected, setSelected] = useState<string[]>(() => value)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const tasks = useQuery({
    queryKey: ["project-dependency-tasks", workspaceSlug, projectSlug],
    queryFn: () => getProjectTasks(workspaceSlug, projectSlug),
    enabled: open && workspaceSlug.length > 0 && projectSlug.length > 0,
  })
  const selectedSet = useMemo(() => new Set(selected), [selected])
  const taskByUUID = useMemo(
    () => new Map((tasks.data ?? []).map((task) => [task.uuid, task])),
    [tasks.data]
  )
  const refByUUID = useMemo(
    () => new Map(refs.map((ref) => [ref.uuid, ref])),
    [refs]
  )
  const visibleTasks = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    const allTasks = (tasks.data ?? []).filter((task) => task.uuid !== taskUUID)
    if (!keyword) {
      return allTasks
    }
    return allTasks.filter((task) =>
      `${task.task_slug ?? ""} ${task.title} ${task.status}`
        .toLowerCase()
        .includes(keyword)
    )
  }, [query, taskUUID, tasks.data])

  const submit = async () => {
    setSaving(true)
    setError(null)
    try {
      await onSave(selected)
      setOpen(false)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure(t("projectReadonly.depends"), message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {value.length === 0 ? (
          <span className="text-xs text-muted-foreground">
            {t("projectReadonly.noDependencies")}
          </span>
        ) : (
          value.map((uuid) => {
            const ref = refByUUID.get(uuid)
            return (
              <Badge key={uuid} variant="outline">
                {ref?.task_slug ? `${ref.task_slug} ` : ""}
                {ref?.title || uuid.slice(0, 8)}
              </Badge>
            )
          })
        )}
        <Button
          aria-label={t("projectReadonly.editDependencies")}
          disabled={disabled}
          onClick={() => {
            setSelected(value)
            setQuery("")
            setError(null)
            setOpen(true)
          }}
          size="icon-sm"
          type="button"
          variant="outline"
        >
          <PlusIcon />
        </Button>
      </div>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("projectReadonly.searchDependencyTasks")}</DialogTitle>
            <DialogDescription>
              {t("projectReadonly.dependencyDescription")}
            </DialogDescription>
          </DialogHeader>
          <Input
            aria-label={t("projectReadonly.searchDependencyTasks")}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("projectReadonly.dependencySearchPlaceholder")}
            value={query}
          />
          <div className="max-h-72 space-y-1 overflow-auto border bg-background p-1">
            {tasks.isPending ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">
                {t("projectReadonly.loadingTasks")}
              </div>
            ) : visibleTasks.length === 0 ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">
                {t("projectReadonly.noMatchingTasks")}
              </div>
            ) : (
              visibleTasks.map((task) => (
                <DependencyOption
                  checked={selectedSet.has(task.uuid)}
                  key={task.uuid}
                  onChange={(checked) => {
                    setSelected((current) =>
                      checked
                        ? Array.from(new Set([...current, task.uuid]))
                        : current.filter((item) => item !== task.uuid)
                    )
                  }}
                  task={task}
                />
              ))
            )}
          </div>
          {selected.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {selected.map((uuid) => {
                const task = taskByUUID.get(uuid)
                const ref = refByUUID.get(uuid)
                const label =
                  task?.task_slug ||
                  ref?.task_slug ||
                  task?.title ||
                      ref?.title ||
                      uuid
                return (
                  <Badge key={uuid} variant="secondary">
                    {label}
                    <Button
                      aria-label={t("projectReadonly.removeDependency", {
                        label,
                      })}
                      className="-mr-1 size-4 rounded-full text-muted-foreground hover:text-foreground"
                      onClick={() =>
                        setSelected((current) =>
                          current.filter((item) => item !== uuid)
                        )
                      }
                      size="icon-xs"
                      type="button"
                      variant="ghost"
                    >
                      <XIcon className="size-3" />
                    </Button>
                  </Badge>
                )
              })}
            </div>
          ) : null}
          {error ? <p className="text-xs text-destructive">{error}</p> : null}
          <DialogFooter>
            <Button
              disabled={saving}
              onClick={() => setSelected([])}
              type="button"
              variant="outline"
            >
              {t("projectReadonly.clearDependencies")}
            </Button>
            <Button
              disabled={saving}
              onClick={() => void submit()}
              type="button"
            >
              {t("common.done")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function DependencyOption({
  checked,
  onChange,
  task,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  task: ProjectWorkbenchTask
}) {
  const { t } = useTranslation()
  const slug = task.task_slug || task.uuid.slice(0, 8)
  return (
    <label className="grid cursor-pointer grid-cols-[auto_1fr_auto] items-center gap-3 rounded-sm px-2 py-2 text-sm hover:bg-muted">
      <Checkbox
        aria-label={`${slug} ${task.title}`}
        checked={checked}
        onCheckedChange={(next) => onChange(Boolean(next))}
      />
      <span className="min-w-0">
        <code className="text-[11px] text-muted-foreground">{slug}</code>
        <span className="block truncate font-medium">{task.title}</span>
      </span>
      <Badge className="shrink-0" variant="outline">
        {taskStatusLabel(task.status, t)}
      </Badge>
    </label>
  )
}

export function splitList(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
}
