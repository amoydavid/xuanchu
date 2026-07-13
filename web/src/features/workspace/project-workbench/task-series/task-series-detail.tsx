import type {
  TaskOccurrenceView,
  TaskSeriesView,
} from "@/features/workspace/project-workbench/api/task-series-api"
import { formatDateShort } from "./recurrence-preview"

// TaskSeriesDetail 是面板内的 series 详情（spec §15.6）。
// 显示摘要、规则、计数；ended/stopped 不显示编辑/停止。
export function TaskSeriesDetail({
  series,
  onBack,
  onEdit,
  onStop,
  canManage,
  workspaceSlug,
  projectSlug,
}: {
  series: TaskSeriesView
  onBack: () => void
  onEdit?: () => void
  onStop?: () => void
  canManage: boolean
  workspaceSlug: string
  projectSlug: string
}) {
  const active = series.status === "active"
  return (
    <article data-testid="task-series-detail" data-series-id={series.id}>
      <header>
        <button type="button" onClick={onBack} aria-label="返回循环任务列表">
          循环任务
        </button>
        <h3>{series.title}</h3>
        {canManage && active && onEdit && (
          <button type="button" onClick={onEdit} data-testid="series-edit-btn">编辑循环设置</button>
        )}
        {canManage && active && onStop && (
          <button type="button" onClick={onStop} data-testid="series-stop-btn">停止循环</button>
        )}
      </header>
      <dl>
        <dt>状态</dt>
        <dd>{series.status}</dd>
        <dt>规则</dt>
        <dd>{series.recurrence_rule}</dd>
        <dt>首次截止</dt>
        <dd>{series.first_due}</dd>
        {series.until != null && (
          <>
            <dt>有效至</dt>
            <dd>{series.until}</dd>
          </>
        )}
      </dl>
      <section>
        <h4>摘要</h4>
        <p>
          未完成 {series.open_occurrence_count} · 逾期 {series.overdue_count}
          {" · 已完成 "}{series.completed_count} · 已跳过 {series.skipped_count}
        </p>
        {series.next_recurrence_at != null && (
          <p>下一槽位：{series.next_recurrence_at}</p>
        )}
      </section>
      <OccurrenceGroup
        items={series.open_occurrences ?? []}
        projectSlug={projectSlug}
        title="未完成实例"
        workspaceSlug={workspaceSlug}
      />
      <OccurrenceGroup
        items={series.recent_completed ?? []}
        projectSlug={projectSlug}
        title="最近完成"
        workspaceSlug={workspaceSlug}
      />
      <OccurrenceGroup
        items={series.recent_skipped ?? []}
        projectSlug={projectSlug}
        title="最近跳过"
        workspaceSlug={workspaceSlug}
      />
    </article>
  )
}

function OccurrenceGroup({
  items,
  projectSlug,
  title,
  workspaceSlug,
}: {
  items: TaskOccurrenceView[]
  projectSlug: string
  title: string
  workspaceSlug: string
}) {
  return (
    <section>
      <h4>{title}</h4>
      {items.length === 0 ? (
        <p>暂无记录</p>
      ) : (
        <ul>
          {items.map((item) => (
            <li key={item.id}>
              <a
                aria-label={`查看本次任务 ${item.title}`}
                href={`/workspaces/${encodeURIComponent(workspaceSlug)}/projects/${encodeURIComponent(projectSlug)}/tasks/${encodeURIComponent(item.id)}`}
              >
                {item.due
                  ? formatDateShort(new Date(item.due * 1000))
                  : item.title}
              </a>
              {` · ${item.status}`}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
