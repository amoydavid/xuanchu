import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { CheckIcon, UserPlusIcon, XIcon } from "lucide-react"

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
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import {
  getWorkspaceMembers,
  type WorkspaceMemberCandidate,
} from "../api/users-api"
import type { ProjectTaskFilterParams } from "../api/project-api"
import { useCreateTaskMutation } from "../hooks/use-task-mutations"
import { InlineDatePicker } from "../shared/inline-date-picker"

type TaskCreateDialogProps = {
  filters?: ProjectTaskFilterParams | string
  onOpenChange: (open: boolean) => void
  open: boolean
  projectSlug: string
  workspaceSlug: string
}

const PRIORITIES = ["H", "M", "L"] as const

export function TaskCreateDialog({
  filters,
  onOpenChange,
  open,
  projectSlug,
  workspaceSlug,
}: TaskCreateDialogProps) {
  const createTask = useCreateTaskMutation(workspaceSlug, projectSlug, filters)
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [priority, setPriority] = useState("")
  const [due, setDue] = useState<number | null>(null)
  const [scheduled, setScheduled] = useState<number | null>(null)
  const [wait, setWait] = useState<number | null>(null)
  const [until, setUntil] = useState<number | null>(null)
  const [tags, setTags] = useState("")
  const [assigneeOpen, setAssigneeOpen] = useState(false)
  const [selectedAssignees, setSelectedAssignees] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)

  const members = useQuery({
    queryKey: ["workspace-members", workspaceSlug],
    queryFn: () => getWorkspaceMembers(workspaceSlug),
    enabled: assigneeOpen,
  })
  const memberByID = useMemo(
    () =>
      new Map((members.data ?? []).map((member) => [member.user_id, member])),
    [members.data]
  )
  const selectedSet = useMemo(
    () => new Set(selectedAssignees),
    [selectedAssignees]
  )

  const reset = () => {
    setTitle("")
    setDescription("")
    setPriority("")
    setDue(null)
    setScheduled(null)
    setWait(null)
    setUntil(null)
    setTags("")
    setSelectedAssignees([])
    setError(null)
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
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(due !== null ? { due } : {}),
        ...(priority ? { priority } : {}),
        ...(scheduled !== null ? { scheduled } : {}),
        ...(selectedAssignees.length > 0
          ? { assignees: selectedAssignees }
          : {}),
        ...(splitCSV(tags).length > 0 ? { tags: splitCSV(tags) } : {}),
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
        if (!nextOpen && !createTask.isPending) {
          reset()
        }
        onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>新建任务</DialogTitle>
          <DialogDescription>
            在当前项目内创建任务，并一次性补齐负责人、日期和优先级。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <Input
            aria-label="任务标题"
            autoFocus
            onChange={(event) => {
              setTitle(event.target.value)
              setError(null)
            }}
            onKeyDown={(event) => {
              if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
                void submit()
              }
            }}
            placeholder="任务标题"
            value={title}
          />
          <Textarea
            aria-label="任务内容"
            className="min-h-28"
            onChange={(event) => setDescription(event.target.value)}
            placeholder="补充背景、验收标准或处理说明"
            value={description}
          />
          <div className="grid gap-3 md:grid-cols-2">
            <Select onValueChange={setPriority} value={priority}>
              <SelectTrigger aria-label="优先级">
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
            <AssigneeSelector
              memberByID={memberByID}
              members={members.data ?? []}
              onOpenChange={setAssigneeOpen}
              open={assigneeOpen}
              pending={members.isPending}
              selected={selectedAssignees}
              selectedSet={selectedSet}
              setSelected={setSelectedAssignees}
            />
            <InlineDatePicker
              ariaLabel="截止日期"
              emptyLabel="截止日期"
              onSave={setDue}
              value={due}
            />
            <InlineDatePicker
              ariaLabel="计划开始"
              emptyLabel="计划开始"
              onSave={setScheduled}
              value={scheduled}
            />
            <InlineDatePicker
              ariaLabel="暂缓到"
              emptyLabel="暂缓到"
              onSave={setWait}
              value={wait}
            />
            <InlineDatePicker
              ariaLabel="有效至"
              emptyLabel="有效至"
              onSave={setUntil}
              value={until}
            />
          </div>
          <Input
            aria-label="标签"
            onChange={(event) => setTags(event.target.value)}
            placeholder="标签，用逗号分隔"
            value={tags}
          />
          {selectedAssignees.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {selectedAssignees.map((id) => {
                const member = memberByID.get(id)
                return (
                  <Badge key={id} variant="secondary">
                    {memberLabel(member) || id}
                    <Button
                      aria-label={`移除负责人 ${memberLabel(member) || id}`}
                      className="-mr-1 size-4 rounded-full"
                      onClick={() =>
                        setSelectedAssignees((current) =>
                          current.filter((item) => item !== id)
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
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>
        <DialogFooter>
          <Button
            disabled={createTask.isPending}
            onClick={() => onOpenChange(false)}
            type="button"
            variant="outline"
          >
            取消
          </Button>
          <Button
            disabled={createTask.isPending}
            onClick={() => void submit()}
            type="button"
          >
            创建任务
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AssigneeSelector({
  memberByID,
  members,
  onOpenChange,
  open,
  pending,
  selected,
  selectedSet,
  setSelected,
}: {
  memberByID: Map<string, WorkspaceMemberCandidate>
  members: WorkspaceMemberCandidate[]
  onOpenChange: (open: boolean) => void
  open: boolean
  pending: boolean
  selected: string[]
  selectedSet: Set<string>
  setSelected: (selected: string[] | ((current: string[]) => string[])) => void
}) {
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          className="justify-start font-normal"
          type="button"
          variant="outline"
        >
          <UserPlusIcon />
          {selected.length > 0
            ? selected
                .map((id) => memberLabel(memberByID.get(id)) || id)
                .join(", ")
            : "选择负责人"}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 space-y-2">
        <div className="max-h-64 space-y-1 overflow-auto">
          {pending ? (
            <div className="px-2 py-3 text-sm text-muted-foreground">
              正在加载成员...
            </div>
          ) : members.length === 0 ? (
            <div className="px-2 py-3 text-sm text-muted-foreground">
              没有成员
            </div>
          ) : (
            members.map((member) => {
              const checked = selectedSet.has(member.user_id)
              const label = memberLabel(member)
              return (
                <label
                  className="flex cursor-pointer items-center gap-3 px-2 py-2 text-sm hover:bg-muted"
                  key={member.user_id}
                >
                  <Checkbox
                    aria-label={`${label} ${member.email ?? ""}`.trim()}
                    checked={checked}
                    onCheckedChange={(next) =>
                      setSelected((current) =>
                        next
                          ? Array.from(new Set([...current, member.user_id]))
                          : current.filter((item) => item !== member.user_id)
                      )
                    }
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">{label}</span>
                    {member.email ? (
                      <span className="block truncate text-xs text-muted-foreground">
                        {member.email}
                      </span>
                    ) : null}
                  </span>
                  {checked ? <CheckIcon className="size-4" /> : null}
                </label>
              )
            })
          )}
        </div>
        <div className="flex justify-between gap-2">
          <Button
            onClick={() => setSelected([])}
            size="sm"
            type="button"
            variant="outline"
          >
            清空负责人
          </Button>
          <Button onClick={() => onOpenChange(false)} size="sm" type="button">
            完成
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function memberLabel(member?: WorkspaceMemberCandidate): string {
  return member?.display_name || member?.name || member?.email || ""
}

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
}
