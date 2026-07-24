import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api"
import { ProjectNotesTimeline } from "@/features/workspace/project-workbench/project/project-notes-timeline"
import {
  addProjectAnnotation,
  deleteProjectAnnotation,
  listProjectAnnotations,
} from "@/features/workspace/project-workbench/api/project-api"

type ProjectNotesTabProps = {
  canManage: boolean
  closed: boolean
  projectSlug: string
  workspaceSlug: string
}

export function ProjectNotesTab({
  canManage,
  closed,
  projectSlug,
  workspaceSlug,
}: ProjectNotesTabProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ["project", workspaceSlug, projectSlug, "annotations"],
    queryFn: () => listProjectAnnotations(workspaceSlug, projectSlug),
  })
  const [content, setContent] = useState("")
  const [error, setError] = useState<string | null>(null)

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["project", workspaceSlug, projectSlug, "annotations"],
    })

  const addMut = useMutation({
    mutationFn: (text: string) =>
      addProjectAnnotation(workspaceSlug, projectSlug, text),
    onSuccess: () => {
      setContent("")
      setError(null)
      void invalidate()
    },
    onError: (err) => {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    },
  })

  const delMut = useMutation({
    mutationFn: (id: string) =>
      deleteProjectAnnotation(workspaceSlug, projectSlug, id),
    onSuccess: invalidate,
  })

  return (
    <section className="rounded-lg space-y-3 border bg-card p-4">
      <h2 className="text-sm font-medium">{t("projectSettings.notesTitle")}</h2>

      {closed ? (
        <Alert>
          <AlertDescription>
            {t("projectSettings.noteClosedReadonly")}
          </AlertDescription>
        </Alert>
      ) : null}

      {query.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : (
        <ProjectNotesTimeline
          canManage={canManage && !closed}
          onDelete={(id) => delMut.mutate(id)}
          emptyLabel={t("projectSettings.notesEmpty")}
          entries={query.data ?? []}
        />
      )}

      {canManage && !closed ? (
        <form
          className="grid gap-2 border-t pt-3"
          onSubmit={(e) => {
            e.preventDefault()
            const text = content.trim()
            if (!text) return
            addMut.mutate(text)
          }}
        >
          <Label htmlFor="project-note-content">
            {t("projectSettings.noteNew")}
          </Label>
          <Textarea
            id="project-note-content"
            onChange={(e) => setContent(e.target.value)}
            rows={3}
            value={content}
          />
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <Button disabled={addMut.isPending} size="sm" type="submit">
            {t("projectSettings.noteAdd")}
          </Button>
        </form>
      ) : null}
    </section>
  )
}
