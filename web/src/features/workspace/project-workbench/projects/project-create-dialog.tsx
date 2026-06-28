import type { FormEvent } from "react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
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
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api"

import type { ProjectWorkbenchProject } from "../api/project-api"
import { useCreateProjectMutation } from "../hooks/use-project-mutations"

const PROJECT_SLUG_PATTERN = /^[a-z][a-z0-9]{2,9}$/

type ProjectCreateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (project: ProjectWorkbenchProject) => void
  workspaceSlug: string
}

type FieldErrors = {
  name?: string
  slug?: string
}

export function ProjectCreateDialog({
  open,
  onCreated,
  onOpenChange,
  workspaceSlug,
}: ProjectCreateDialogProps) {
  const { t } = useTranslation()
  const createProject = useCreateProjectMutation(workspaceSlug)
  const [slug, setSlug] = useState("")
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [submitError, setSubmitError] = useState<string | null>(null)

  function resetForm() {
    setSlug("")
    setName("")
    setDescription("")
    setFieldErrors({})
    setSubmitError(null)
    createProject.reset()
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetForm()
    }
    onOpenChange(nextOpen)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextErrors: FieldErrors = {}
    const normalizedSlug = slug.trim()
    const normalizedName = name.trim()
    const normalizedDescription = description.trim()

    if (!normalizedName) {
      nextErrors.name = t("projectWorkbench.projects.create.nameRequired")
    }
    if (!PROJECT_SLUG_PATTERN.test(normalizedSlug)) {
      nextErrors.slug = t("projectWorkbench.projects.create.slugInvalid")
    }
    setFieldErrors(nextErrors)
    setSubmitError(null)
    if (Object.keys(nextErrors).length > 0) {
      return
    }

    try {
      const project = await createProject.mutateAsync({
        slug: normalizedSlug,
        name: normalizedName,
        ...(normalizedDescription ? { description: normalizedDescription } : {}),
      })
      resetForm()
      onCreated(project)
    } catch (error) {
      const code = error instanceof ApiError ? error.code : "unknown"
      setSubmitError(
        t(`projectWorkbench.projects.create.errors.${code}`, {
          defaultValue: t("projectWorkbench.projects.create.errors.unknown"),
        })
      )
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form className="space-y-4" onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>
              {t("projectWorkbench.projects.create.title")}
            </DialogTitle>
            <DialogDescription>
              {t("projectWorkbench.projects.create.description")}
            </DialogDescription>
          </DialogHeader>

          {submitError ? (
            <Alert variant="destructive">
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          ) : null}

          <div className="space-y-2">
            <Label htmlFor="project-create-slug">
              {t("projectWorkbench.projects.create.slug")}
            </Label>
            <Input
              id="project-create-slug"
              aria-invalid={fieldErrors.slug ? true : undefined}
              value={slug}
              onChange={(event) => setSlug(event.target.value)}
              placeholder="ops"
              autoComplete="off"
            />
            {fieldErrors.slug ? (
              <p className="text-xs text-destructive">{fieldErrors.slug}</p>
            ) : (
              <p className="text-xs text-muted-foreground">
                {t("projectWorkbench.projects.create.slugHint")}
              </p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="project-create-name">
              {t("projectWorkbench.projects.create.name")}
            </Label>
            <Input
              id="project-create-name"
              aria-invalid={fieldErrors.name ? true : undefined}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
            {fieldErrors.name ? (
              <p className="text-xs text-destructive">{fieldErrors.name}</p>
            ) : null}
          </div>

          <div className="space-y-2">
            <Label htmlFor="project-create-description">
              {t("projectWorkbench.projects.create.projectDescription")}
            </Label>
            <Textarea
              id="project-create-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              rows={3}
            />
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              {t("projectWorkbench.projects.create.cancel")}
            </Button>
            <Button type="submit" disabled={createProject.isPending}>
              {createProject.isPending
                ? t("projectWorkbench.projects.create.creating")
                : t("projectWorkbench.projects.create.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
