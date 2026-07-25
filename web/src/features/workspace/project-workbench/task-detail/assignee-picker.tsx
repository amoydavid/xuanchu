import { useMemo, useState } from "react"
import { PlusIcon, XIcon } from "lucide-react"
import { useQuery } from "@tanstack/react-query"

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
import { stopScrollPropagation } from "@/lib/scroll-propagation"
import type { ProjectWorkbenchAssignee } from "../api/project-api"
import { getWorkspaceMembers } from "../api/users-api"
import { useEditFeedback } from "../shared/edit-feedback"

type AssigneePickerProps = {
  disabled?: boolean
  onSave: (assignees: string[]) => Promise<void> | void
  value?: ProjectWorkbenchAssignee[]
  workspaceSlug: string
}

export function AssigneePicker({
  disabled = false,
  onSave,
  value = [],
  workspaceSlug,
}: AssigneePickerProps) {
  const feedback = useEditFeedback()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [selected, setSelected] = useState<string[]>(() => assigneeRefs(value))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const members = useQuery({
    queryKey: ["workspace-members", workspaceSlug],
    queryFn: () => getWorkspaceMembers(workspaceSlug),
    enabled: open && workspaceSlug.length > 0,
  })
  const selectedSet = useMemo(() => new Set(selected), [selected])
  const memberByID = useMemo(
    () =>
      new Map((members.data ?? []).map((member) => [member.id, member])),
    [members.data]
  )
  const visibleMembers = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    const allMembers = members.data ?? []
    if (!keyword) {
      return allMembers
    }
    return allMembers.filter((member) =>
      `${member.name} ${member.email ?? ""} ${member.id}`
        .toLowerCase()
        .includes(keyword)
    )
  }, [members.data, query])

  const submit = async () => {
    setSaving(true)
    setError(null)
    try {
      await onSave(selected)
      setOpen(false)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure("负责人", message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {value.length === 0 ? (
          <span className="text-xs text-muted-foreground">未分配</span>
        ) : (
          value.map((assignee) => (
            <Badge key={assigneeRef(assignee)} variant="outline">
              {assignee.name || assignee.email || assigneeRef(assignee)}
            </Badge>
          ))
        )}
        <Button
          aria-label="编辑负责人"
          disabled={disabled}
          onClick={() => {
            setSelected(assigneeRefs(value))
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
            <DialogTitle>搜索负责人</DialogTitle>
            <DialogDescription>
              从当前 workspace 成员中选择任务负责人。
            </DialogDescription>
          </DialogHeader>
          <Input
            aria-label="搜索负责人"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="输入姓名、邮箱或用户 ID"
            value={query}
          />
          <div className="rounded-lg max-h-72 space-y-1 overflow-auto border bg-background p-1" onWheelCapture={stopScrollPropagation}>
            {members.isPending ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">
                正在加载成员...
              </div>
            ) : visibleMembers.length === 0 ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">
                没有匹配成员
              </div>
            ) : (
              visibleMembers.map((member) => {
                const checked = selectedSet.has(member.id)
                return (
                  <label
                    className="flex cursor-pointer items-center gap-3 rounded-sm px-2 py-2 text-sm hover:bg-muted"
                    key={member.id}
                  >
                    <Checkbox
                      aria-label={`${member.name} ${member.email ?? ""}`.trim()}
                      checked={checked}
                      onCheckedChange={(next) => {
                        setSelected((current) =>
                          next
                            ? Array.from(new Set([...current, member.id]))
                            : current.filter((item) => item !== member.id)
                        )
                      }}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">
                        {member.name}
                      </span>
                      {member.email ? (
                        <span className="block truncate text-xs text-muted-foreground">
                          {member.email}
                        </span>
                      ) : null}
                    </span>
                  </label>
                )
              })
            )}
          </div>
          {selected.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {selected.map((id) => {
                const member = memberByID.get(id)
                return (
                  <Badge key={id} variant="secondary">
                    {member?.name || id}
                    <Button
                      aria-label={`移除负责人 ${member?.name || id}`}
                      className="-mr-1 size-4 rounded-full text-muted-foreground hover:text-foreground"
                      onClick={() =>
                        setSelected((current) =>
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
          {error ? <p className="text-xs text-destructive">{error}</p> : null}
          <DialogFooter>
            <Button
              disabled={saving}
              onClick={() => setSelected([])}
              type="button"
              variant="outline"
            >
              清空负责人
            </Button>
            <Button
              disabled={saving}
              onClick={() => void submit()}
              type="button"
            >
              完成
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function assigneeRefs(assignees: ProjectWorkbenchAssignee[]): string[] {
  return assignees.map(assigneeRef).filter(Boolean)
}

function assigneeRef(assignee: ProjectWorkbenchAssignee): string {
  return (
    assignee.id || assignee.email || assignee.name || ""
  )
}
