import { CopyIcon, SettingsIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { ProjectWorkbenchProject } from "../api/project-api"
import { InlineTextEditor } from "../shared/inline-text-editor"
import { useModifyProjectMutation } from "../hooks/use-project-mutations"
import { ProjectSettingsDialog } from "./project-settings-dialog"
import { ProjectStatusMenu } from "./project-status-menu"

type ProjectHeaderEditorProps = {
  canManage: boolean
  onCopyLink: () => void
  project: ProjectWorkbenchProject
  workspaceSlug: string
}

export function ProjectHeaderEditor({
  canManage,
  onCopyLink,
  project,
  workspaceSlug,
}: ProjectHeaderEditorProps) {
  const modifyProject = useModifyProjectMutation(workspaceSlug, project.slug)

  return (
    <section className="border-b pb-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0 flex-1">
          <div className="text-xs text-muted-foreground">
            {workspaceSlug} / {project.slug}
          </div>
          {canManage ? (
            <InlineTextEditor
              ariaLabel="项目名称"
              displayClassName="mt-1 text-2xl font-semibold tracking-normal"
              onSave={async (name) => {
                await modifyProject.mutateAsync({ name })
              }}
              validate={(value) => (value.trim() ? null : "项目名称不能为空")}
              value={project.name}
            />
          ) : (
            <h1 className="mt-1 text-2xl font-semibold tracking-normal">
              {project.name}
            </h1>
          )}
          {canManage ? (
            <InlineTextEditor
              ariaLabel="项目描述"
              displayClassName="mt-2 max-w-3xl text-sm text-muted-foreground"
              emptyLabel="添加项目描述"
              multiline
              onSave={async (description) => {
                await modifyProject.mutateAsync({ description })
              }}
              value={project.description ?? ""}
            />
          ) : project.description ? (
            <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
              {project.description}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <ProjectStatusMenu
            canManage={canManage}
            project={project}
            workspaceSlug={workspaceSlug}
          />
          <Button onClick={onCopyLink} size="sm" variant="outline">
            <CopyIcon />
            复制链接
          </Button>
          {canManage ? (
            <ProjectSettingsDialog
              project={project}
              trigger={
                <Button aria-label="项目设置" size="icon-sm" variant="outline">
                  <SettingsIcon />
                </Button>
              }
              workspaceSlug={workspaceSlug}
            />
          ) : null}
        </div>
      </div>
    </section>
  )
}
