import { useState } from "react"
import { PlusIcon, Trash2Icon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import type { ProjectWorkbenchTaskLink } from "../api/project-api"
import { useTaskLinkMutations } from "../hooks/use-task-mutations"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"

type TaskLinksEditorProps = {
  canWrite: boolean
  links?: ProjectWorkbenchTaskLink[]
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

export function TaskLinksEditor({
  canWrite,
  links = [],
  projectSlug,
  taskRef,
  workspaceSlug,
}: TaskLinksEditorProps) {
  const [open, setOpen] = useState(false)
  const [type, setType] = useState("")
  const [url, setURL] = useState("")
  const [title, setTitle] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [deleteID, setDeleteID] = useState<string | null>(null)
  const mutations = useTaskLinkMutations(workspaceSlug, projectSlug, taskRef)

  const add = async () => {
    const normalizedType = type.trim()
    const normalizedURL = url.trim()
    const normalizedTitle = title.trim()
    if (!normalizedType) {
      setError("类型不能为空")
      return
    }
    if (!isValidURL(normalizedURL)) {
      setError("请输入有效 URL")
      return
    }
    setError(null)
    try {
      await mutations.add.mutateAsync({
        ...(normalizedTitle ? { title: normalizedTitle } : {}),
        type: normalizedType,
        url: normalizedURL,
      })
      setType("")
      setURL("")
      setTitle("")
      setOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium">链接</h2>
        {canWrite ? (
          <Button
            onClick={() => {
              setError(null)
              setOpen(true)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <PlusIcon />
            添加链接
          </Button>
        ) : null}
      </div>
      {links.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无链接</p>
      ) : (
        <ul className="space-y-1">
          {links.map((link) => (
            <li
              className="flex items-center justify-between gap-3 text-sm"
              key={link.id}
            >
              <div className="min-w-0">
                <a
                  className="text-primary underline-offset-4 hover:underline"
                  href={link.url}
                  rel="noreferrer"
                  target="_blank"
                >
                  {link.title || link.url}
                </a>
                <span className="text-muted-foreground">
                  {" · "}
                  {link.type}
                  {link.created_by?.name ? ` · ${link.created_by.name}` : ""}
                </span>
              </div>
              {canWrite ? (
                <Button
                  aria-label="删除链接"
                  onClick={() => setDeleteID(link.id)}
                  size="icon-sm"
                  type="button"
                  variant="ghost"
                >
                  <Trash2Icon />
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>添加链接</DialogTitle>
            <DialogDescription>
              链接用于关联规格、外部文档或运行产物。
            </DialogDescription>
          </DialogHeader>
          <label className="grid gap-2 text-sm">
            类型
            <Input
              aria-label="类型"
              onChange={(event) => {
                setType(event.target.value)
                setError(null)
              }}
              value={type}
            />
          </label>
          <label className="grid gap-2 text-sm">
            URL
            <Input
              aria-label="URL"
              onChange={(event) => {
                setURL(event.target.value)
                setError(null)
              }}
              value={url}
            />
          </label>
          <label className="grid gap-2 text-sm">
            标题
            <Input
              aria-label="标题"
              onChange={(event) => setTitle(event.target.value)}
              value={title}
            />
          </label>
          {error ? <p className="text-xs text-destructive">{error}</p> : null}
          <DialogFooter>
            <Button onClick={() => setOpen(false)} type="button" variant="outline">
              取消
            </Button>
            <Button
              disabled={mutations.add.isPending}
              onClick={() => {
                void add()
              }}
              type="button"
            >
              保存链接
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <DestructiveConfirmDialog
        confirmLabel="删除"
        description="删除后这条链接将不再显示。"
        onConfirm={async () => {
          if (!deleteID) {
            return
          }
          await mutations.remove.mutateAsync(deleteID)
          setDeleteID(null)
        }}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            setDeleteID(null)
          }
        }}
        open={deleteID !== null}
        pending={mutations.remove.isPending}
        title="确认删除链接"
      />
    </section>
  )
}

function isValidURL(value: string): boolean {
  try {
    const parsed = new URL(value)
    return parsed.protocol === "http:" || parsed.protocol === "https:"
  } catch {
    return false
  }
}
