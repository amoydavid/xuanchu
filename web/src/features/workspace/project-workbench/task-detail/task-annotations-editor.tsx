import { useState } from "react"
import { PencilIcon, Trash2Icon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { MarkdownEditor, MarkdownView } from "@/components/markdown"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { TaskAnnotation } from "../api/task-api"
import { useTaskAnnotationMutations } from "../hooks/use-task-mutations"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"
import { useEditFeedback } from "../shared/edit-feedback"

type TaskAnnotationsEditorProps = {
  annotations?: TaskAnnotation[]
  canWrite: boolean
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

export function TaskAnnotationsEditor({
  annotations = [],
  canWrite,
  projectSlug,
  taskRef,
  workspaceSlug,
}: TaskAnnotationsEditorProps) {
  const feedback = useEditFeedback()
  const [draft, setDraft] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [editID, setEditID] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState("")
  const [editError, setEditError] = useState<string | null>(null)
  const [deleteID, setDeleteID] = useState<string | null>(null)
  const mutations = useTaskAnnotationMutations(workspaceSlug, projectSlug, taskRef)

  const updateDraft = (nextDraft: string) => {
    setDraft((previousDraft) => {
      if (nextDraft !== previousDraft) {
        setError(null)
      }
      return nextDraft
    })
  }

  const updateEditDraft = (nextDraft: string) => {
    setEditDraft((previousDraft) => {
      if (nextDraft !== previousDraft) {
        setEditError(null)
      }
      return nextDraft
    })
  }

  const add = async () => {
    const description = draft.trim()
    if (!description) {
      setError("注解不能为空")
      return
    }
    setError(null)
    try {
      await mutations.add.mutateAsync({ description })
      setDraft("")
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure("添加注解", message)
    }
  }

  const update = async () => {
    const description = editDraft.trim()
    if (!description) {
      setEditError("注解不能为空")
      return
    }
    if (!editID) {
      return
    }
    setEditError(null)
    try {
      await mutations.update.mutateAsync({
        annotationID: editID,
        input: { description },
      })
      setEditID(null)
      setEditDraft("")
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setEditError(message)
      feedback.failure("编辑注解", message)
    }
  }

  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium">注解</h2>
      </div>
      {canWrite ? (
        <div className="space-y-2 border bg-card p-3">
          <MarkdownEditor
            ariaLabel="新增注解"
            disabled={mutations.add.isPending}
            minHeight={120}
            onChange={updateDraft}
            onModEnter={() => {
              void add()
            }}
            placeholder="记录进展、背景或决策..."
            value={draft}
          />
          <div className="flex items-center justify-between gap-2">
            {error ? <p className="text-xs text-destructive">{error}</p> : <span />}
            <Button
              disabled={mutations.add.isPending}
              onClick={() => {
                void add()
              }}
              size="sm"
              type="button"
            >
              添加注解
            </Button>
          </div>
        </div>
      ) : null}
      {annotations.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无注解</p>
      ) : (
        <div className="space-y-2">
          {annotations.map((annotation, index) => (
            <div
              className="flex items-start justify-between gap-3 border bg-card p-3 text-sm"
              key={annotation.id || index}
            >
              <div className="min-w-0">
                <MarkdownView className="text-sm">
                  {annotation.description}
                </MarkdownView>
                {annotation.entry ? (
                  <div className="mt-1 text-xs text-muted-foreground">
                    {annotation.entry}
                  </div>
                ) : null}
              </div>
              {canWrite && annotation.id ? (
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    aria-label="编辑注解"
                    onClick={() => {
                      setEditID(annotation.id ?? null)
                      setEditDraft(annotation.description)
                      setEditError(null)
                    }}
                    size="icon-sm"
                    type="button"
                    variant="ghost"
                  >
                    <PencilIcon />
                  </Button>
                  <Button
                    aria-label="删除注解"
                    onClick={() => setDeleteID(annotation.id ?? null)}
                    size="icon-sm"
                    type="button"
                    variant="ghost"
                  >
                    <Trash2Icon />
                  </Button>
                </div>
              ) : null}
            </div>
          ))}
        </div>
      )}
      <Dialog
        open={editID !== null}
        onOpenChange={(open) => {
          if (!open) {
            setEditID(null)
            setEditError(null)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑注解</DialogTitle>
            <DialogDescription>
              修正或补充这条任务注解。
            </DialogDescription>
          </DialogHeader>
          <MarkdownEditor
            ariaLabel="编辑注解内容"
            disabled={mutations.update.isPending}
            minHeight={160}
            onChange={updateEditDraft}
            onModEnter={() => {
              void update()
            }}
            value={editDraft}
          />
          {editError ? (
            <p className="text-xs text-destructive">{editError}</p>
          ) : null}
          <DialogFooter>
            <Button
              onClick={() => setEditID(null)}
              type="button"
              variant="outline"
            >
              取消
            </Button>
            <Button
              disabled={mutations.update.isPending}
              onClick={() => {
                void update()
              }}
              type="button"
            >
              保存注解
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <DestructiveConfirmDialog
        confirmLabel="删除"
        description="删除后这条注解将不再显示。"
        onConfirm={async () => {
          if (!deleteID) {
            return
          }
          await mutations.remove.mutateAsync(deleteID)
          setDeleteID(null)
        }}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteID(null)
          }
        }}
        open={deleteID !== null}
        pending={mutations.remove.isPending}
        title="确认删除注解"
      />
    </section>
  )
}
