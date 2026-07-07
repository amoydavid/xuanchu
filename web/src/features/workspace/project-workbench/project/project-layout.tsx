import { createContext, useContext, useState, type ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { PanelRightCloseIcon, PanelRightOpenIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { listProjectEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { navigateToDocument } from "@/lib/browser-navigation"
import {
  useProjectQuery,
  useProjectTaskSummaryQuery,
  useProjectTimelineQuery,
} from "../hooks/use-project-data"
import { canProjectManage, canTaskRead } from "../permissions/permissions"
import { EditFeedbackProvider, useEditFeedback } from "../shared/edit-feedback"
import { ProjectClosedBanner } from "./project-closed-banner"
import { ProjectContextRail } from "./project-context-rail"
import { ProjectHeaderEditor } from "./project-header-editor"
import { isClosedProjectStatus } from "./project-status-menu"
import { ProjectTabs, type ProjectTabKey } from "./project-tabs"

type ProjectLayoutProps = {
  projectSlug: string
  workspaceSlug: string
  activeTab: ProjectTabKey
  children: ReactNode
}

export type ProjectLayoutContextValue = {
  workspaceSlug: string
  projectSlug: string
  project: NonNullable<ReturnType<typeof useProjectQuery>["data"]>
  canManage: boolean
  canReadTasks: boolean
  closed: boolean
  // setTabActions 允许子页面在 tabs 行右侧（收起/展开按钮左边）注册额外动作节点。
  // 例如任务页用它注册「导入任务」图标按钮。传 null 清空。
  setTabActions: (node: ReactNode | null) => void
}

const LayoutContext = createContext<ProjectLayoutContextValue | null>(null)

export function useProjectLayout(): ProjectLayoutContextValue {
  const ctx = useContext(LayoutContext)
  if (!ctx) {
    throw new Error("useProjectLayout must be used inside ProjectLayout")
  }
  return ctx
}

// ProjectLayout 负责项目级数据加载、Header、Tabs、右栏开合与整体布局。
// 子页面主体通过 children 注入；共享 Header 只放复制、状态、设置等项目级动作。
export function ProjectLayout(props: ProjectLayoutProps) {
  return (
    <EditFeedbackProvider>
      <ProjectLayoutContent {...props} />
    </EditFeedbackProvider>
  )
}

function ProjectLayoutContent({
  projectSlug,
  workspaceSlug,
  activeTab,
  children,
}: ProjectLayoutProps) {
  const { t } = useTranslation()
  const feedback = useEditFeedback()
  const [railOpen, setRailOpen] = useState(true)
  const [tabActions, setTabActions] = useState<ReactNode | null>(null)
  const me = useMe()
  const project = useProjectQuery(workspaceSlug, projectSlug)
  const timeline = useProjectTimelineQuery(workspaceSlug, projectSlug)
  const canManage = canProjectManage({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const canReadTasks = canTaskRead({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const summary = useProjectTaskSummaryQuery(
    workspaceSlug,
    projectSlug,
    canReadTasks
  )
  const homeConfig = useQuery({
    queryKey: [
      "project-workbench",
      workspaceSlug,
      projectSlug,
      "config-effective",
      "console-home",
    ],
    queryFn: () => listProjectEffectiveConfig(projectSlug, { consoleHome: true }),
  })

  if (isPermissionError(project.error)) {
    return (
      <ProjectState
        actionLabel={t("projectReadonly.backToOverview")}
        description={t("projectWorkbench.project.permissionDescription", {
          project: `${workspaceSlug} / ${projectSlug}`,
        })}
        detail="project:read"
        title={t("projectWorkbench.project.permissionTitle")}
      />
    )
  }
  if (isNotFoundError(project.error)) {
    return (
      <ProjectState
        actionLabel={t("projectReadonly.backToOverview")}
        description={t("projectWorkbench.project.notFoundDescription")}
        title={t("projectWorkbench.project.notFoundTitle")}
      />
    )
  }
  if (project.isPending) {
    return <ProjectSkeleton />
  }
  if (project.isError) {
    const code = project.error instanceof ApiError ? project.error.code : "unknown"
    return (
      <ProjectState
        actionLabel={t("common.refresh")}
        description={code}
        onAction={() => window.location.reload()}
        title={t("projectWorkbench.project.loadFailed")}
      />
    )
  }

  const closed = isClosedProjectStatus(project.data.status)
  const layoutValue: ProjectLayoutContextValue = {
    workspaceSlug,
    projectSlug,
    project: project.data,
    canManage,
    canReadTasks,
    closed,
    setTabActions,
  }

  return (
    <LayoutContext.Provider value={layoutValue}>
      <div className="space-y-4">
        <ProjectHeaderEditor
          canManage={canManage}
          onCopyLink={() => {
            void navigator.clipboard?.writeText(window.location.href)
            feedback.success(t("projectReadonly.copied"))
          }}
          project={project.data}
          workspaceSlug={workspaceSlug}
        />
        <ProjectClosedBanner canManage={canManage} status={project.data.status} />
        <div className="flex items-center justify-between gap-2 border-b pb-2">
          <ProjectTabs
            activeTab={activeTab}
            projectSlug={projectSlug}
            workspaceSlug={workspaceSlug}
          />
          <div className="flex items-center gap-1">
            {tabActions}
            <Button
              aria-label={
                railOpen
                  ? t("projectSubpages.railCollapse")
                  : t("projectSubpages.railExpand")
              }
              onClick={() => setRailOpen((value) => !value)}
              size="icon"
              title={
                railOpen
                  ? t("projectSubpages.railCollapse")
                  : t("projectSubpages.railExpand")
              }
              type="button"
              variant="ghost"
            >
              {railOpen ? (
                <PanelRightCloseIcon className="h-4 w-4" />
              ) : (
                <PanelRightOpenIcon className="h-4 w-4" />
              )}
            </Button>
          </div>
        </div>
        <div className="flex flex-col gap-4 lg:flex-row">
          <div className="min-w-0 flex-1">{children}</div>
          {railOpen ? (
            <ProjectContextRail
              configError={homeConfig.isError}
              configRows={homeConfig.data}
              project={project.data}
              summary={summary.data}
              summaryError={summary.isError}
              timeline={timeline.data}
              timelineError={timeline.isError}
            />
          ) : null}
        </div>
      </div>
    </LayoutContext.Provider>
  )
}

function ProjectState({
  actionLabel,
  description,
  detail,
  onAction,
  title,
}: {
  actionLabel: string
  description: string
  detail?: string
  onAction?: () => void
  title: string
}) {
  return (
    <section className="max-w-2xl border bg-card p-6">
      <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
      <p className="mt-3 text-sm text-muted-foreground">{description}</p>
      {detail ? <code className="mt-4 block text-xs">{detail}</code> : null}
      <Button
        className="mt-5"
        onClick={onAction ?? (() => navigateToDocument("/"))}
        variant="outline"
      >
        {actionLabel}
      </Button>
    </section>
  )
}

function ProjectSkeleton() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-10 w-72" />
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  )
}

function isPermissionError(error: Error | null): boolean {
  return error instanceof ApiError && error.status === 403
}

function isNotFoundError(error: Error | null): boolean {
  return error instanceof ApiError && error.status === 404
}
