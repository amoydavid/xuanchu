import { useState, type ReactNode } from "react"
import { useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import type { ProjectWorkbenchProject } from "../api/project-api"
import { useModifyProjectMutation } from "../hooks/use-project-mutations"

type ProjectSettingsDialogProps = {
  project: ProjectWorkbenchProject
  trigger: ReactNode
  workspaceSlug: string
}

export function ProjectSettingsDialog({
  project,
  trigger,
  workspaceSlug,
}: ProjectSettingsDialogProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [slug, setSlug] = useState(project.slug)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()
  const modifyProject = useModifyProjectMutation(workspaceSlug, project.slug)

  const save = async () => {
    const normalizedSlug = slug.trim()
    if (!/^[a-z][a-z0-9]{2,31}$/.test(normalizedSlug)) {
      setError(t("projectWorkbench.project.settingsSlugInvalid"))
      return
    }
    setError(null)
    try {
      const updated = await modifyProject.mutateAsync({ slug: normalizedSlug })
      setOpen(false)
      if (updated.slug !== project.slug) {
        void navigate({
          to: "/workspaces/$workspaceSlug/projects/$projectSlug",
          params: { workspaceSlug, projectSlug: updated.slug },
        })
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("projectWorkbench.project.settings")}</DialogTitle>
          <DialogDescription>
            {t("projectWorkbench.project.settingsDescription")}
          </DialogDescription>
        </DialogHeader>
        <label className="grid gap-2 text-sm">
          Slug
          <Input
            aria-invalid={error ? true : undefined}
            onChange={(event) => {
              setSlug(event.target.value)
              setError(null)
            }}
            value={slug}
          />
        </label>
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
        <DialogFooter>
          <Button
            disabled={modifyProject.isPending}
            onClick={() => setOpen(false)}
            type="button"
            variant="outline"
          >
            {t("common.cancel")}
          </Button>
          <Button disabled={modifyProject.isPending} onClick={save} type="button">
            {t("projectWorkbench.project.saveSettings")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
