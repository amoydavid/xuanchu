import { useQuery } from "@tanstack/react-query"

import { Skeleton } from "@/components/ui/skeleton"
import { useProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"

import { listProjectAutomations } from "./project-automations-api"

type Props = {
  projectSlug: string
  workspaceSlug: string
}

// ProjectAutomationsPage 占位实现，Task 6 会替换为完整规则列表和编辑表单。
export function ProjectAutomationsPage({ projectSlug }: Props) {
  const layout = useProjectLayout()
  void layout
  const rules = useQuery({
    queryKey: ["project", projectSlug, "automations"],
    queryFn: () => listProjectAutomations(projectSlug, true),
  })

  if (rules.isPending) {
    return <Skeleton className="h-48 w-full" />
  }

  return (
    <section className="space-y-4">
      <h2 className="text-base font-semibold">自动化</h2>
      <p className="text-sm text-muted-foreground">项目自动化规则与投递记录。</p>
    </section>
  )
}
