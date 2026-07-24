import { useState } from "react"
import { useTranslation } from "react-i18next"

import { MarkdownEditor } from "@/components/markdown"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useTaskActivityQuery } from "../hooks/use-task-detail-data"
import { useTaskAnnotationMutations } from "../hooks/use-task-mutations"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"
import { useEditFeedback } from "../shared/edit-feedback"
import {
  TaskActivityTimeline,
  TaskActivityTimelineSkeleton,
} from "./task-activity-timeline"

type ActivitySectionProps = {
  canWrite: boolean
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

export function ActivitySection({
  canWrite,
  projectSlug,
  taskRef,
  workspaceSlug,
}: ActivitySectionProps) {
  const { t } = useTranslation()
  const feedback = useEditFeedback()
  const activity = useTaskActivityQuery(workspaceSlug, taskRef)
  const mutations = useTaskAnnotationMutations(
    workspaceSlug,
    projectSlug,
    taskRef
  )
  const [composerOpen, setComposerOpen] = useState(false)
  const [draft, setDraft] = useState("")
  const [composerError, setComposerError] = useState<string | null>(null)
  const [editID, setEditID] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState("")
  const [editError, setEditError] = useState<string | null>(null)
  const [deleteID, setDeleteID] = useState<string | null>(null)
  const entries = activity.data?.pages.flatMap((page) => page.entries) ?? []

  const addAnnotation = async () => {
    const description = draft.trim()
    if (!description) {
      setComposerError(t("taskDetail.annotationRequired"))
      return
    }
    setComposerError(null)
    try {
      await mutations.add.mutateAsync({ description })
      setDraft("")
      setComposerOpen(false)
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      setComposerError(message)
      feedback.failure(t("taskDetail.addAnnotation"), message)
    }
  }

  const updateAnnotation = async () => {
    const description = editDraft.trim()
    if (!description) {
      setEditError(t("taskDetail.annotationRequired"))
      return
    }
    if (!editID) return
    setEditError(null)
    try {
      await mutations.update.mutateAsync({
        annotationID: editID,
        input: { description },
      })
      setEditID(null)
      setEditDraft("")
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      setEditError(message)
      feedback.failure(t("taskDetail.editAnnotation"), message)
    }
  }

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium">{t("taskDetail.activity")}</h2>
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
      {canWrite && composerOpen ? (
        <div className="space-y-2">
          <MarkdownEditor
            ariaLabel={t("taskDetail.newAnnotation")}
            disabled={mutations.add.isPending}
            minHeight={120}
            onChange={(value) => {
              setDraft(value)
              setComposerError(null)
            }}
            onModEnter={() => void addAnnotation()}
            placeholder={t("taskDetail.annotationPlaceholder")}
            value={draft}
          />
          <div className="flex items-center justify-between gap-2">
            {composerError ? (
              <p className="text-xs text-destructive">{composerError}</p>
            ) : (
              <span />
            )}
            <div className="flex items-center gap-2">
              <Button
                onClick={() => {
                  setComposerOpen(false)
                  setComposerError(null)
                }}
                size="sm"
                type="button"
                variant="ghost"
              >
                {t("taskDetail.cancel")}
              </Button>
              <Button
                disabled={mutations.add.isPending}
                onClick={() => void addAnnotation()}
                size="sm"
                type="button"
              >
                {t("taskDetail.addAnnotation")}
              </Button>
            </div>
          </div>
        </div>
      ) : null}
      {activity.isPending ? (
        <TaskActivityTimelineSkeleton />
      ) : activity.isError ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span>{t("taskDetail.activityUnavailable")}</span>
          <button
            className="underline underline-offset-4"
            onClick={() => void activity.refetch()}
            type="button"
          >
            {t("taskDetail.activityRetry")}
          </button>
        </div>
      ) : (
        <TaskActivityTimeline
          canWrite={canWrite}
          entries={entries}
          hasNextPage={activity.hasNextPage}
          isFetchingNextPage={activity.isFetchingNextPage}
          onDeleteAnnotation={setDeleteID}
          onEditAnnotation={(annotation) => {
            setEditID(annotation.id)
            setEditDraft(annotation.description)
            setEditError(null)
          }}
          onLoadMore={() => void activity.fetchNextPage()}
        />
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
            <DialogTitle>{t("taskDetail.editAnnotation")}</DialogTitle>
            <DialogDescription>
              {t("taskDetail.editAnnotationDescription")}
            </DialogDescription>
          </DialogHeader>
          <MarkdownEditor
            ariaLabel={t("taskDetail.editAnnotationContent")}
            disabled={mutations.update.isPending}
            minHeight={160}
            onChange={(value) => {
              setEditDraft(value)
              setEditError(null)
            }}
            onModEnter={() => void updateAnnotation()}
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
              onClick={() => void updateAnnotation()}
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
          if (!deleteID) return
          await mutations.remove.mutateAsync(deleteID)
          setDeleteID(null)
        }}
        onOpenChange={(open) => {
          if (!open) setDeleteID(null)
        }}
        open={deleteID !== null}
        pending={mutations.remove.isPending}
        title={t("taskDetail.confirmDeleteAnnotation")}
      />
    </section>
  )
}
