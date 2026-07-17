import { useState } from "react"
import { PencilIcon, Trash2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"

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
  showTitle?: boolean
  taskRef: string
  workspaceSlug: string
}

export function TaskAnnotationsEditor({
  annotations = [],
  canWrite,
  projectSlug,
  showTitle = true,
  taskRef,
  workspaceSlug,
}: TaskAnnotationsEditorProps) {
  const { t } = useTranslation()
  const feedback = useEditFeedback()
  const [draft, setDraft] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [composerOpen, setComposerOpen] = useState(false)
  const [editID, setEditID] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState("")
  const [editError, setEditError] = useState<string | null>(null)
  const [deleteID, setDeleteID] = useState<string | null>(null)
  const mutations = useTaskAnnotationMutations(
    workspaceSlug,
    projectSlug,
    taskRef
  )

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
      setError(t("taskDetail.annotationRequired"))
      return
    }
    setError(null)
    try {
      await mutations.add.mutateAsync({ description })
      setDraft("")
      setComposerOpen(false)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure(t("taskDetail.addAnnotation"), message)
    }
  }

  const update = async () => {
    const description = editDraft.trim()
    if (!description) {
      setEditError(t("taskDetail.annotationRequired"))
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
      feedback.failure(t("taskDetail.editAnnotation"), message)
    }
  }

  return (
    <section className="space-y-2">
      {showTitle || (canWrite && !composerOpen) ? (
        <div className="flex items-center justify-between gap-2">
          {showTitle ? (
            <h2 className="text-sm font-medium">
              {t("taskDetail.annotations")}
            </h2>
          ) : null}
          {canWrite && !composerOpen ? (
            <Button
              onClick={() => setComposerOpen(true)}
              size="sm"
              type="button"
              variant="ghost"
            >
              {t("taskDetail.writeUpdate")}
            </Button>
          ) : null}
        </div>
      ) : null}
      {canWrite && composerOpen ? (
        <div className="space-y-2">
          <MarkdownEditor
            ariaLabel={t("taskDetail.newAnnotation")}
            disabled={mutations.add.isPending}
            minHeight={120}
            onChange={updateDraft}
            onModEnter={() => {
              void add()
            }}
            placeholder={t("taskDetail.annotationPlaceholder")}
            value={draft}
          />
          <div className="flex items-center justify-between gap-2">
            {error ? (
              <p className="text-xs text-destructive">{error}</p>
            ) : (
              <span />
            )}
            <Button
              disabled={mutations.add.isPending}
              onClick={() => {
                void add()
              }}
              size="sm"
              type="button"
            >
              {t("taskDetail.addAnnotation")}
            </Button>
          </div>
        </div>
      ) : null}
      {annotations.length > 0 ? (
        <div className="space-y-2">
          {annotations.map((annotation, index) => (
            <div
              className="flex items-start justify-between gap-3 py-2 text-sm"
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
                    aria-label={t("taskDetail.editAnnotation")}
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
                    aria-label={t("taskDetail.deleteAnnotation")}
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
      ) : null}
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
            <DialogTitle>{t("taskDetail.editAnnotation")}</DialogTitle>
            <DialogDescription>
              {t("taskDetail.editAnnotationDescription")}
            </DialogDescription>
          </DialogHeader>
          <MarkdownEditor
            ariaLabel={t("taskDetail.editAnnotationContent")}
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
              {t("taskDetail.cancel")}
            </Button>
            <Button
              disabled={mutations.update.isPending}
              onClick={() => {
                void update()
              }}
              type="button"
            >
              {t("taskDetail.saveAnnotation")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <DestructiveConfirmDialog
        cancelLabel={t("taskDetail.cancel")}
        confirmLabel={t("taskDetail.deleteAnnotation")}
        description={t("taskDetail.deleteAnnotationDescription")}
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
        title={t("taskDetail.confirmDeleteAnnotation")}
      />
    </section>
  )
}
