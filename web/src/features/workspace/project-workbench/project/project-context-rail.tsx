import { PanelRightCloseIcon, PanelRightOpenIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { ConfigEffectiveValue } from "@/features/workspace/config/config-definition-api"
import { cn } from "@/lib/utils"

import type {
  ProjectTaskSummary,
  ProjectTimelineEntry,
  ProjectWorkbenchProject,
} from "../api/project-api"
import { formatConfigDisplayValue } from "@/features/workspace/config/config-display"

type ProjectContextRailProps = {
  project: ProjectWorkbenchProject
  summary?: ProjectTaskSummary
  summaryError?: boolean
  configRows?: ConfigEffectiveValue[]
  configError?: boolean
  timeline?: ProjectTimelineEntry[]
  timelineError?: boolean
  railOpen: boolean
  onRailOpenChange: (open: boolean) => void
  // projectSlug/workspaceSlug 用于跳转配置定义和全部活动。
  projectSlug: string
  workspaceSlug: string
}

// ProjectContextRail 是项目右侧信息栏，在所有项目子页面保持一致。
// 默认打开，可由图标按钮收起；收起后只保留展开按钮，左侧主体自动占满剩余宽度。
// 所有数字都来自 ProjectTaskSummary 后端聚合，禁止用当前任务列表派生全量统计。
export function ProjectContextRail({
  project,
  summary,
  summaryError,
  configRows,
  configError,
  timeline,
  timelineError,
  railOpen,
  onRailOpenChange,
  projectSlug,
  workspaceSlug,
}: ProjectContextRailProps) {
  const { t } = useTranslation()

  if (!railOpen) {
    return (
      <RailExpandButton
        ariaLabel={t("projectSubpages.railExpand")}
        onOpen={() => onRailOpenChange(true)}
      />
    )
  }

  const completedRatio =
    project.task_count > 0
      ? Math.round((project.completed_count / project.task_count) * 100)
      : 0

  return (
    <aside
      aria-label={t("projectSubpages.railTitle")}
      className="flex w-full flex-col gap-4 lg:w-80 lg:shrink-0"
      data-testid="project-context-rail"
    >
      <div className="flex items-center justify-end">
        <Button
          aria-label={t("projectSubpages.railCollapse")}
          className="ml-auto"
          onClick={() => onRailOpenChange(false)}
          size="icon"
          title={t("projectSubpages.railCollapse")}
          type="button"
          variant="ghost"
        >
          <PanelRightCloseIcon className="h-4 w-4" />
        </Button>
      </div>

      <RailSection title={t("projectSubpages.railTitle")}>
        <RailRow
          label={t("projectSubpages.attributeStatus")}
          value={project.status}
        />
        <RailRow
          label={t("projectSubpages.attributeTasks")}
          value={t("projectSubpages.attributeTasksHint", {
            total: project.task_count,
            pending: project.pending_count,
          })}
        />
        <RailRow
          label={t("projectSubpages.attributeCreated")}
          value={formatUnixDay(project.created_at)}
        />
        <RailRow
          label={t("projectSubpages.attributeUpdated")}
          value={formatUnixDay(project.modified_at)}
        />
      </RailSection>

      {summaryError ? (
        <RailError text={t("projectSubpages.railSummaryError")} />
      ) : summary ? (
        <>
          <RailSection title={t("projectSubpages.progressTitle")}>
            <div className="space-y-1">
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">
                  {t("projectSubpages.progressLabel")}
                </span>
                <span className="font-medium">
                  {t("projectSubpages.progressRatio", { percent: completedRatio })}
                </span>
              </div>
              <div className="h-2 w-full overflow-hidden rounded bg-muted">
                <div
                  className="h-full bg-primary"
                  style={{ width: `${completedRatio}%` }}
                />
              </div>
            </div>
            <RailSummaryCount
              label={t("projectSubpages.riskOverdue")}
              value={summary.overdue_count}
            />
            <RailSummaryCount
              label={t("projectSubpages.riskHighPriority")}
              value={summary.high_priority_open_count}
            />
            <RailSummaryCount
              label={t("projectSubpages.riskWaitReady")}
              value={summary.wait_ready_count}
            />
            <RailSummaryCount
              label={t("projectSubpages.riskUnassigned")}
              value={summary.unassigned_open_count}
            />
          </RailSection>

          {summary.workload.length > 0 ? (
            <RailSection title={t("projectSubpages.workloadTitle")}>
              {summary.workload.slice(0, 5).map((row, index) => (
                <div className="text-sm" key={row.label || `wl-${index}`}>
                  <div className="font-medium">{row.label}</div>
                  <div className="text-muted-foreground">
                    {t("projectSubpages.workloadPending", {
                      count: row.open_count,
                    })}
                    {row.overdue_count > 0
                      ? " · " +
                        t("projectSubpages.workloadOverdue", {
                          count: row.overdue_count,
                        })
                      : ""}
                    {row.high_priority_count > 0
                      ? " · " +
                        t("projectSubpages.workloadHighPriority", {
                          count: row.high_priority_count,
                        })
                      : ""}
                  </div>
                </div>
              ))}
            </RailSection>
          ) : null}
        </>
      ) : null}

      {configError ? (
        <RailError text={t("projectSubpages.railConfigError")} />
      ) : configRows && configRows.length > 0 ? (
        <RailSection title={t("projectSubpages.factsTitle")}>
          {configRows.map((row) => (
            <RailConfigRow key={row.key} row={row} />
          ))}
        </RailSection>
      ) : null}

      {timelineError ? (
        <RailError text={t("projectSubpages.railTimelineError")} />
      ) : timeline && timeline.length > 0 ? (
        <RailSection title={t("projectSubpages.recentActivityTitle")}>
          {timeline.slice(0, 5).map((entry, index) => (
            <RailTimelineRow entry={entry} key={`rail-tl-${index}`} />
          ))}
        </RailSection>
      ) : null}
    </aside>
  )
}

function RailExpandButton({
  ariaLabel,
  onOpen,
}: {
  ariaLabel: string
  onOpen: () => void
}) {
  return (
    <div className="flex w-full justify-end lg:w-12 lg:shrink-0">
      <Button
        aria-label={ariaLabel}
        onClick={onOpen}
        size="icon"
        title={ariaLabel}
        type="button"
        variant="ghost"
      >
        <PanelRightOpenIcon className="h-4 w-4" />
      </Button>
    </div>
  )
}

function RailSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="rounded-lg border bg-card p-3">
      <h2 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </h2>
      <div className="space-y-2">{children}</div>
    </section>
  )
}

function RailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate font-medium">{value}</span>
    </div>
  )
}

function RailSummaryCount({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-center justify-between gap-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value}</span>
    </div>
  )
}

function RailConfigRow({ row }: { row: ConfigEffectiveValue }) {
  const { t } = useTranslation()
  const label = row.definition.label || row.key
  let value: React.ReactNode = "—"
  if (row.missing_required) {
    value = t("projectSubpages.factsMissingRequired")
  } else if (row.definition.secret) {
    // 后端对 secret 值已脱敏（••••）；有值即展示「已设置」或后端返回的脱敏值，
    // 不调用普通 formatter，避免泄露或二次处理。
    value = row.value ? t("projectSubpages.secretConfigured") : "—"
  } else if (row.value !== null && row.value !== "") {
    value = formatConfigDisplayValue(row.definition.value_type, row.value)
  }
  return (
    <div className="text-sm">
      <div className="font-medium">{label}</div>
      <div className="text-muted-foreground">{value}</div>
    </div>
  )
}

function RailTimelineRow({ entry }: { entry: ProjectTimelineEntry }) {
  const label = entry.source_label ?? entry.source_type ?? ""
  const content = entry.content ?? entry.action ?? entry.summary ?? ""
  return (
    <div className="text-sm">
      <div className="font-medium">{label}</div>
      {content ? (
        <div className={cn("text-muted-foreground", "line-clamp-2")}>{content}</div>
      ) : null}
    </div>
  )
}

function RailError({ text }: { text: string }) {
  return (
    <div className="rounded-lg border border-destructive/30 bg-card p-3 text-sm text-destructive">
      {text}
    </div>
  )
}

function formatUnixDay(unix: number | undefined | null): string {
  if (!unix) {
    return "—"
  }
  const date = new Date(unix * 1000)
  if (Number.isNaN(date.getTime())) {
    return "—"
  }
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}
