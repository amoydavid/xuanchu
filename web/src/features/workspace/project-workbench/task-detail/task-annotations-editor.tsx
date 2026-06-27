import { useState } from "react"
import { Trash2Icon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { TaskAnnotation } from "../api/task-api"
import { useTaskAnnotationMutations } from "../hooks/use-task-mutations"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"

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
  const [draft, setDraft] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [deleteID, setDeleteID] = useState<string | null>(null)
  const mutations = useTaskAnnotationMutations(workspaceSlug, projectSlug, taskRef)

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
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium">注解</h2>
      </div>
      {canWrite ? (
        <div className="space-y-2 border bg-card p-3">
          <Textarea
            aria-label="新增注解"
            disabled={mutations.add.isPending}
            onChange={(event) => {
              setDraft(event.target.value)
              setError(null)
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
              <div>
                <div>{annotation.description}</div>
                {annotation.entry ? (
                  <div className="mt-1 text-xs text-muted-foreground">
                    {annotation.entry}
                  </div>
                ) : null}
              </div>
              {canWrite && annotation.id ? (
                <Button
                  aria-label="删除注解"
                  onClick={() => setDeleteID(annotation.id ?? null)}
                  size="icon-sm"
                  type="button"
                  variant="ghost"
                >
                  <Trash2Icon />
                </Button>
              ) : null}
            </div>
          ))}
        </div>
      )}
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
