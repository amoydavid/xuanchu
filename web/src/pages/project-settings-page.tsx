import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api"
import {
  addProjectAnnotation,
  deleteProjectAnnotation,
  deleteProjectConfig,
  getProject,
  listProjectAnnotations,
  listProjectConfig,
  setProjectConfig,
  transitionProject,
  type ProjectConfigEntry,
  type ProjectAnnotationInfo,
  type ProjectStatus,
  type ProjectWorkbenchProject,
} from "@/features/workspace/project-workbench/api/project-api"
import { MarkdownView } from "@/components/markdown"

type ProjectSettingsPageProps = {
  canManage: boolean
  projectSlug: string
  workspaceSlug: string
}

const STATUS_OPTIONS: ProjectStatus[] = [
  "planning",
  "active",
  "archived",
  "cancelled",
]

export function ProjectSettingsPage({
  canManage,
  projectSlug,
  workspaceSlug,
}: ProjectSettingsPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const project = useQuery<ProjectWorkbenchProject>({
    queryKey: ["project", workspaceSlug, projectSlug],
    queryFn: () => getProject(workspaceSlug, projectSlug),
  })

  const transition = useMutation({
    mutationFn: (status: ProjectStatus) =>
      transitionProject(workspaceSlug, projectSlug, status),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["project", workspaceSlug, projectSlug],
      })
    },
  })

  if (project.isError) {
    return (
      <section className="max-w-3xl border bg-card p-6 text-sm text-destructive">
        {project.error instanceof ApiError
          ? project.error.message
          : t("common.error")}
      </section>
    )
  }
  if (project.isLoading || !project.data) {
    return (
      <section className="max-w-3xl border bg-card p-6 text-sm text-muted-foreground">
        {t("common.loading")}
      </section>
    )
  }

  const p = project.data

  return (
    <div className="max-w-3xl space-y-6">
      <div>
        <nav className="text-xs text-muted-foreground">
          <a className="hover:text-foreground" href={`/workspaces/${workspaceSlug}/projects/${projectSlug}`}>
            {projectSlug}
          </a>
          {" / "}
          <span>{t("projectSettings.title")}</span>
        </nav>
        <h1 className="mt-2 text-xl font-semibold tracking-normal">
          {t("projectSettings.title")}
        </h1>
      </div>

      <section className="space-y-3 border bg-card p-4">
        <h2 className="text-sm font-medium">{t("projectSettings.basic")}</h2>
        <div className="grid gap-2 text-sm">
          <div>
            <span className="text-muted-foreground">{t("common.name")}：</span>
            {p.name}
          </div>
          <div>
            <span className="text-muted-foreground">slug：</span>
            <code className="break-all">{p.slug}</code>
          </div>
          {p.description ? (
            <div>
              <span className="text-muted-foreground">
                {t("projectReadonly.description")}：
              </span>
              <MarkdownView>{p.description}</MarkdownView>
            </div>
          ) : null}
        </div>
      </section>

      <section className="space-y-3 border bg-card p-4">
        <h2 className="text-sm font-medium">{t("projectSettings.status")}</h2>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline">{p.status}</Badge>
          {canManage ? (
            <>
              {STATUS_OPTIONS.map((status) => (
                <Button
                  disabled={transition.isPending || status === p.status}
                  key={status}
                  onClick={() => {
                    if (
                      status === "archived" ||
                      status === "cancelled"
                    ) {
                      if (!window.confirm(t("projectSettings.transitionConfirm", { status }))) {
                        return
                      }
                    }
                    transition.mutate(status)
                  }}
                  size="sm"
                  variant={status === p.status ? "default" : "outline"}
                >
                  {status}
                </Button>
              ))}
            </>
          ) : null}
          {transition.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {transition.error instanceof ApiError
                  ? transition.error.message
                  : t("common.error")}
              </AlertDescription>
            </Alert>
          ) : null}
        </div>
      </section>

      <ProjectConfigEditor
        canManage={canManage}
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
      />

      <ProjectAnnotationsEditor
        canManage={canManage}
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
      />
    </div>
  )
}

function ProjectConfigEditor({
  canManage,
  projectSlug,
  workspaceSlug,
}: {
  canManage: boolean
  projectSlug: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery<ProjectConfigEntry[]>({
    queryKey: ["project", workspaceSlug, projectSlug, "config"],
    queryFn: () => listProjectConfig(workspaceSlug, projectSlug),
  })
  const [newKey, setNewKey] = useState("")
  const [newValue, setNewValue] = useState("")
  const [error, setError] = useState<string | null>(null)

  const setMut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setProjectConfig(workspaceSlug, projectSlug, key, value),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["project", workspaceSlug, projectSlug, "config"],
      })
      setNewKey("")
      setNewValue("")
      setError(null)
    },
    onError: (err) => {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    },
  })

  const delMut = useMutation({
    mutationFn: (key: string) =>
      deleteProjectConfig(workspaceSlug, projectSlug, key),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["project", workspaceSlug, projectSlug, "config"],
      })
    },
  })

  const entries = query.data ?? []

  return (
    <section className="space-y-3 border bg-card p-4">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium">{t("projectSettings.configTitle")}</h2>
      </div>
      {entries.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("common.empty")}</p>
      ) : (
        <div className="space-y-1">
          {entries.map((entry) => (
            <div
              className="flex items-center justify-between gap-2 border-b py-1 text-sm last:border-b-0"
              key={entry.key}
            >
              <div className="min-w-0">
                <code className="text-xs">{entry.key}</code>
                <div className="truncate text-muted-foreground">{entry.value}</div>
              </div>
              {canManage ? (
                <Button
                  onClick={() => {
                    if (window.confirm(t("projectSettings.configDeleteConfirm"))) {
                      delMut.mutate(entry.key)
                    }
                  }}
                  size="sm"
                  variant="outline"
                >
                  {t("common.delete")}
                </Button>
              ) : null}
            </div>
          ))}
        </div>
      )}
      {canManage ? (
        <form
          className="grid gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (!newKey.trim()) return
            setMut.mutate({ key: newKey.trim(), value: newValue })
          }}
        >
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-[12rem_minmax(0,1fr)]">
            <Input
              aria-label={t("projectSettings.configKey")}
              onChange={(e) => setNewKey(e.target.value)}
              placeholder={t("projectSettings.configKey")}
              value={newKey}
            />
            <Input
              aria-label={t("projectSettings.configValue")}
              onChange={(e) => setNewValue(e.target.value)}
              placeholder={t("projectSettings.configValue")}
              value={newValue}
            />
          </div>
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <Button disabled={setMut.isPending} size="sm" type="submit">
            {t("common.save")}
          </Button>
        </form>
      ) : null}
    </section>
  )
}

function ProjectAnnotationsEditor({
  canManage,
  projectSlug,
  workspaceSlug,
}: {
  canManage: boolean
  projectSlug: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery<ProjectAnnotationInfo[]>({
    queryKey: ["project", workspaceSlug, projectSlug, "annotations"],
    queryFn: () => listProjectAnnotations(workspaceSlug, projectSlug),
  })
  const [content, setContent] = useState("")
  const [error, setError] = useState<string | null>(null)

  const addMut = useMutation({
    mutationFn: (text: string) =>
      addProjectAnnotation(workspaceSlug, projectSlug, text),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["project", workspaceSlug, projectSlug, "annotations"],
      })
      setContent("")
      setError(null)
    },
    onError: (err) => {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    },
  })

  const delMut = useMutation({
    mutationFn: (id: string) =>
      deleteProjectAnnotation(workspaceSlug, projectSlug, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["project", workspaceSlug, projectSlug, "annotations"],
      })
    },
  })

  const entries = query.data ?? []

  return (
    <section className="space-y-3 border bg-card p-4">
      <h2 className="text-sm font-medium">{t("projectSettings.annotationsTitle")}</h2>
      {entries.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("common.empty")}</p>
      ) : (
        <div className="space-y-2">
          {entries.map((ann) => (
            <div
              className="border bg-background p-3 text-sm"
              key={ann.id}
            >
              <div className="mb-1 text-xs text-muted-foreground">
                {ann.created_by?.name ?? "-"} · {formatTime(ann.created_at)}
              </div>
              <MarkdownView>{ann.content}</MarkdownView>
              {canManage ? (
                <div className="mt-2 text-right">
                  <Button
                    onClick={() => {
                      if (window.confirm(t("projectSettings.annotationDeleteConfirm"))) {
                        delMut.mutate(ann.id)
                      }
                    }}
                    size="sm"
                    variant="outline"
                  >
                    {t("common.delete")}
                  </Button>
                </div>
              ) : null}
            </div>
          ))}
        </div>
      )}
      {canManage ? (
        <form
          className="space-y-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (!content.trim()) return
            addMut.mutate(content.trim())
          }}
        >
          <Label htmlFor="project-annotation-content">
            {t("projectSettings.annotationNew")}
          </Label>
          <Textarea
            id="project-annotation-content"
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
            {t("projectSettings.annotationAdd")}
          </Button>
        </form>
      ) : null}
    </section>
  )
}

function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  const hh = String(d.getHours()).padStart(2, "0")
  const mm = String(d.getMinutes()).padStart(2, "0")
  return `${y}-${m}-${day} ${hh}:${mm}`
}
