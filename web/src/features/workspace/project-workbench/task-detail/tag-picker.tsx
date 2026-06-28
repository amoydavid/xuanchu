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
import { getProjectTasks } from "../api/project-api"
import { useEditFeedback } from "../shared/edit-feedback"

type TagPickerProps = {
  disabled?: boolean
  onSave: (tags: string[]) => Promise<void> | void
  projectSlug: string
  value?: string[]
  workspaceSlug: string
}

export function TagPicker({
  disabled = false,
  onSave,
  projectSlug,
  value = [],
  workspaceSlug,
}: TagPickerProps) {
  const feedback = useEditFeedback()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [selected, setSelected] = useState<string[]>(() => value)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const tasks = useQuery({
    queryKey: ["project-tag-tasks", workspaceSlug, projectSlug],
    queryFn: () => getProjectTasks(workspaceSlug, projectSlug),
    enabled: open && workspaceSlug.length > 0 && projectSlug.length > 0,
  })
  const selectedSet = useMemo(() => new Set(selected), [selected])
  const candidates = useMemo(() => {
    const counts = new Map<string, number>()
    for (const task of tasks.data ?? []) {
      for (const tag of task.tags ?? []) {
        const normalized = tag.trim()
        if (normalized) {
          counts.set(normalized, (counts.get(normalized) ?? 0) + 1)
        }
      }
    }
    for (const tag of value) {
      if (!counts.has(tag)) {
        counts.set(tag, 0)
      }
    }
    return Array.from(counts.entries())
      .map(([tag, count]) => ({ count, tag }))
      .sort((left, right) => {
        if (right.count !== left.count) {
          return right.count - left.count
        }
        return left.tag.localeCompare(right.tag)
      })
  }, [tasks.data, value])
  const visibleCandidates = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) {
      return candidates
    }
    return candidates.filter((item) => item.tag.toLowerCase().includes(keyword))
  }, [candidates, query])
  const createTag = query.trim()
  const canCreate =
    createTag.length > 0 &&
    !candidates.some(
      (item) => item.tag.toLowerCase() === createTag.toLowerCase()
    )

  const submit = async () => {
    setSaving(true)
    setError(null)
    try {
      await onSave(selected)
      setOpen(false)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure("标签", message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {value.length === 0 ? (
          <span className="text-xs text-muted-foreground">无标签</span>
        ) : (
          value.map((tag) => (
            <Badge key={tag} variant="outline">
              {tag}
            </Badge>
          ))
        )}
        <Button
          aria-label="编辑标签"
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
            <DialogTitle>搜索标签</DialogTitle>
            <DialogDescription>
              从当前项目已有标签中选择，也可以创建新标签。
            </DialogDescription>
          </DialogHeader>
          <Input
            aria-label="搜索标签"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="输入标签名"
            value={query}
          />
          <div className="max-h-72 space-y-1 overflow-auto border bg-background p-1">
            {tasks.isPending ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">
                正在加载标签...
              </div>
            ) : visibleCandidates.length === 0 ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">
                没有匹配标签
              </div>
            ) : (
              visibleCandidates.map((item) => {
                const checked = selectedSet.has(item.tag)
                return (
                  <label
                    className="grid cursor-pointer grid-cols-[auto_1fr_auto] items-center gap-3 rounded-sm px-2 py-2 text-sm hover:bg-muted"
                    key={item.tag}
                  >
                    <Checkbox
                      aria-label={item.tag}
                      checked={checked}
                      onCheckedChange={(next) => {
                        setSelected((current) =>
                          next
                            ? Array.from(new Set([...current, item.tag]))
                            : current.filter((tag) => tag !== item.tag)
                        )
                      }}
                    />
                    <span className="truncate font-medium">{item.tag}</span>
                    <span className="text-xs text-muted-foreground">
                      {item.count} tasks
                    </span>
                  </label>
                )
              })
            )}
          </div>
          {canCreate ? (
            <Button
              aria-label={`新建标签 ${createTag}`}
              onClick={() => {
                setSelected((current) =>
                  Array.from(new Set([...current, createTag]))
                )
                setQuery("")
              }}
              type="button"
              variant="outline"
            >
              <PlusIcon />
              新建标签：{createTag}
            </Button>
          ) : null}
          {selected.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {selected.map((tag) => (
                <Badge key={tag} variant="secondary">
                  {tag}
                  <Button
                    aria-label={`移除标签 ${tag}`}
                    className="-mr-1 size-4 rounded-full text-muted-foreground hover:text-foreground"
                    onClick={() =>
                      setSelected((current) =>
                        current.filter((item) => item !== tag)
                      )
                    }
                    size="icon-xs"
                    type="button"
                    variant="ghost"
                  >
                    <XIcon className="size-3" />
                  </Button>
                </Badge>
              ))}
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
              清空标签
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
