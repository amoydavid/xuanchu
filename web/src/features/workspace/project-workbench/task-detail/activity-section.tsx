import { useTranslation } from "react-i18next"

import type { ProjectTask } from "../api/task-api"
import { TaskAnnotationsEditor } from "./task-annotations-editor"
import { TaskChangeHistory } from "./task-change-history"

type ActivitySectionProps = {
  annotations?: ProjectTask["annotations"]
  canWrite: boolean
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

// 过渡态实现（spec §9.4 合并契约）：
// 当前注解与变更历史是两个独立数据源，未按统一时间戳交错排序。
// 视觉上归入同一 Activity 区块、共用标题与间距，先达成「同一时间轴感」。
// TODO（后续）：合并数据源，按统一时间戳倒序交错渲染，避免「上半段全是注解、下半段全是变更」。
export function ActivitySection({
  annotations,
  canWrite,
  projectSlug,
  taskRef,
  workspaceSlug,
}: ActivitySectionProps) {
  const { t } = useTranslation()
  return (
    <section className="space-y-4 border bg-card p-4">
      <h2 className="text-sm font-medium">{t("taskDetail.activity")}</h2>
      <TaskAnnotationsEditor
        annotations={annotations}
        canWrite={canWrite}
        projectSlug={projectSlug}
        taskRef={taskRef}
        workspaceSlug={workspaceSlug}
      />
      <TaskChangeHistory taskRef={taskRef} workspaceSlug={workspaceSlug} />
    </section>
  )
}
