import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { MarkdownEditor } from "@/components/markdown"
import { auditPath, type AuditRow } from "@/features/workspace/audit/audit-api"
import { useMe } from "@/features/workspace/session/useMe"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { addProjectAnnotation } from "../api/project-api"
import { useProjectTimelineQuery } from "../hooks/use-project-data"
import { canAuditRead } from "../permissions/permissions"
import { useProjectLayout } from "../project/project-layout"
import {
  ProjectActivityTimeline,
  mergeActivity,
  type ActivityFilter,
} from "./project-activity-timeline"

type ProjectActivityPageProps = {
  projectSlug: string
  workspaceSlug: string
}

// ProjectActivityPage 是项目事实流：项目更新输入框 + 时间线 + 可选审计。
// 列表以 project timeline 为主；audit 仅在有 audit:read 时请求，不把 annotations 列表再拼到 timeline。
export function ProjectActivityPage({
  projectSlug,
  workspaceSlug,
}: ProjectActivityPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [content, setContent] = useState("")
  const [filterKey, setFilterKey] = useState<
    "all" | "project" | "task" | "audit"
  >("all")
  const { canManage, closed } = useProjectLayout()
  const timeline = useProjectTimelineQuery(workspaceSlug, projectSlug)

  // audit 仅在 audit:read 时请求；不能把 403 当页面错误。
  const me = useMe()
  const auditEnabled = canAuditRead({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const audit = useQuery<AuditRow[]>({
    queryKey: ["project-activity-audit", workspaceSlug, projectSlug],
    queryFn: () =>
      workspaceApiGet<AuditRow[]>(
        auditPath({ project: projectSlug, limit: 100 })
      ),
    enabled: auditEnabled,
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: ["project", workspaceSlug, projectSlug, "timeline"],
    })
    void queryClient.invalidateQueries({
      queryKey: ["project", workspaceSlug, projectSlug],
    })
    void queryClient.invalidateQueries({
      queryKey: ["home", workspaceSlug],
    })
  }

  const addMutation = useMutation({
    mutationFn: (text: string) =>
      addProjectAnnotation(workspaceSlug, projectSlug, text),
    onSuccess: () => {
      setContent("")
      invalidate()
    },
  })

  const filter: ActivityFilter = useMemo(() => {
    switch (filterKey) {
      case "project":
        return { project: true }
      case "task":
        return { task: true }
      case "audit":
        return { audit: true }
      default:
        return { all: true }
    }
  }, [filterKey])

  const items = useMemo(
    () =>
      mergeActivity(
        timeline.data ?? [],
        auditEnabled ? audit.data : undefined,
        filter,
        { workspaceSlug, projectSlug }
      ),
    [timeline.data, audit.data, auditEnabled, filter, workspaceSlug, projectSlug]
  )

  const canPublish = canManage && !closed

  return (
    <div className="space-y-4">
      {canPublish ? (
        <section className="space-y-2">
          <MarkdownEditor
            ariaLabel={t("projectSubpages.activityPublish")}
            disabled={addMutation.isPending}
            minHeight={120}
            onChange={setContent}
            onModEnter={() => {
              if (content.trim()) {
                addMutation.mutate(content)
              }
            }}
            placeholder={t("projectSubpages.activityPublishPlaceholder")}
            value={content}
          />
          <div className="flex items-center justify-end gap-2">
            <Button
              disabled={!content.trim() || addMutation.isPending}
              onClick={() => addMutation.mutate(content)}
              size="sm"
              type="button"
            >
              {t("projectSubpages.activityPublish")}
            </Button>
          </div>
        </section>
      ) : closed ? (
        <p className="text-sm text-muted-foreground">
          {t("projectSubpages.activityClosedReadonly")}
        </p>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <FilterButton
          active={filterKey === "all"}
          label={t("projectSubpages.activityFilterAll")}
          onClick={() => setFilterKey("all")}
        />
        <FilterButton
          active={filterKey === "project"}
          label={t("projectSubpages.activityFilterProject")}
          onClick={() => setFilterKey("project")}
        />
        <FilterButton
          active={filterKey === "task"}
          label={t("projectSubpages.activityFilterTask")}
          onClick={() => setFilterKey("task")}
        />
        {auditEnabled ? (
          <FilterButton
            active={filterKey === "audit"}
            label={t("projectSubpages.activityFilterAudit")}
            onClick={() => setFilterKey("audit")}
          />
        ) : null}
      </div>

      {timeline.isError ? (
        <p className="text-sm text-destructive">
          {t("projectSubpages.railTimelineError")}
        </p>
      ) : (
        <ProjectActivityTimeline
          items={items}
          projectSlug={projectSlug}
          workspaceSlug={workspaceSlug}
        />
      )}
    </div>
  )
}

function FilterButton({
  active,
  label,
  onClick,
}: {
  active: boolean
  label: string
  onClick: () => void
}) {
  return (
    <Button
      onClick={onClick}
      size="sm"
      type="button"
      variant={active ? "default" : "outline"}
    >
      {label}
    </Button>
  )
}
