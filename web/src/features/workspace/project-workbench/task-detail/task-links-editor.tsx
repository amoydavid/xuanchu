import { useState } from "react"
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"

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
import { useEditFeedback } from "../shared/edit-feedback"

type TaskLinksEditorProps = {
  canWrite: boolean
  links?: ProjectWorkbenchTaskLink[]
  onOpenChange?: (open: boolean) => void
  open?: boolean
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

export function TaskLinksEditor({
  canWrite,
  links = [],
  onOpenChange,
  open: controlledOpen,
  projectSlug,
  taskRef,
  workspaceSlug,
}: TaskLinksEditorProps) {
  const { t } = useTranslation()
  const feedback = useEditFeedback()
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false)
  const [type, setType] = useState("")
  const [url, setURL] = useState("")
  const [title, setTitle] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [editingLink, setEditingLink] =
    useState<ProjectWorkbenchTaskLink | null>(null)
  const [deleteID, setDeleteID] = useState<string | null>(null)
  const mutations = useTaskLinkMutations(workspaceSlug, projectSlug, taskRef)
  const open = controlledOpen ?? uncontrolledOpen
  const setOpen = (nextOpen: boolean) => {
    if (controlledOpen === undefined) setUncontrolledOpen(nextOpen)
    onOpenChange?.(nextOpen)
  }

  const save = async () => {
    const normalizedType = type.trim()
    const normalizedURL = url.trim()
    const normalizedTitle = title.trim()
    if (!normalizedType) {
      setError(t("taskDetail.linkTypeRequired"))
      return
    }
    if (!isValidURL(normalizedURL)) {
      setError(t("taskDetail.linkURLInvalid"))
      return
    }
    setError(null)
    try {
      const input = {
        ...(normalizedTitle ? { title: normalizedTitle } : {}),
        type: normalizedType,
        url: normalizedURL,
      }
      if (editingLink) {
        await mutations.update.mutateAsync({ input, linkID: editingLink.id })
      } else {
        await mutations.add.mutateAsync(input)
      }
      resetForm()
      setOpen(false)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure(
        editingLink ? t("taskDetail.editLink") : t("taskDetail.addLink"),
        message
      )
    }
  }

  // 详情页以受控方式使用本组件。没有内容且未主动创建时不占据默认视图；
  // 仍保留非受控模式，供独立编辑器入口继续显示创建按钮。
  if (controlledOpen !== undefined && links.length === 0 && !open) {
    return null
  }

  return (
    <section className="space-y-2" data-testid="task-links-section">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium">
          {t("taskDetail.relatedResources")}
        </h2>
        {canWrite ? (
          <Button
            onClick={() => {
              resetForm()
              setOpen(true)
            }}
            size="sm"
            type="button"
            variant={links.length > 0 ? "ghost" : "outline"}
          >
            <PlusIcon />
            {t("taskDetail.addLink")}
          </Button>
        ) : null}
      </div>
      {links.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t("taskDetail.linksEmpty")}
        </p>
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
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    aria-label={t("taskDetail.editLink")}
                    onClick={() => {
                      setEditingLink(link)
                      setType(link.type)
                      setURL(link.url)
                      setTitle(link.title ?? "")
                      setError(null)
                      setOpen(true)
                    }}
                    size="icon-sm"
                    type="button"
                    variant="ghost"
                  >
                    <PencilIcon />
                  </Button>
                  <Button
                    aria-label={t("taskDetail.deleteLink")}
                    onClick={() => setDeleteID(link.id)}
                    size="icon-sm"
                    type="button"
                    variant="ghost"
                  >
                    <Trash2Icon />
                  </Button>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editingLink ? t("taskDetail.editLink") : t("taskDetail.addLink")}
            </DialogTitle>
            <DialogDescription>
              {t("taskDetail.linkDescription")}
            </DialogDescription>
          </DialogHeader>
          <label className="grid gap-2 text-sm">
            {t("taskDetail.linkType")}
            <Input
              aria-label={t("taskDetail.linkType")}
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
            {t("taskDetail.linkTitle")}
            <Input
              aria-label={t("taskDetail.linkTitle")}
              onChange={(event) => setTitle(event.target.value)}
              value={title}
            />
          </label>
          {error ? <p className="text-xs text-destructive">{error}</p> : null}
          <DialogFooter>
            <Button
              onClick={() => setOpen(false)}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button
              disabled={mutations.add.isPending || mutations.update.isPending}
              onClick={() => {
                void save()
              }}
              type="button"
            >
              {t("taskDetail.saveLink")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <DestructiveConfirmDialog
        confirmLabel={t("common.delete")}
        description={t("taskDetail.deleteLinkDescription")}
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
        title={t("taskDetail.confirmDeleteLink")}
      />
    </section>
  )

  function resetForm() {
    setEditingLink(null)
    setType("")
    setURL("")
    setTitle("")
    setError(null)
  }
}

function isValidURL(value: string): boolean {
  try {
    const parsed = new URL(value)
    return parsed.protocol === "http:" || parsed.protocol === "https:"
  } catch {
    return false
  }
}
