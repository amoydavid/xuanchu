import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"

import {
  createProjectAutomation,
  listProjectAutomations,
  previewProjectAutomation,
  testProjectAutomationRule,
  type ProjectAutomationPreview,
  type ProjectAutomationRuleInput,
} from "./project-automations-api"
import { AutomationDeliveryList } from "./automation-delivery-list"
import { AutomationPreviewDialog } from "./automation-preview-dialog"
import {
  AutomationRuleForm,
  assigneeFeishuTemplateInput,
  defaultScheduleAutomationInput,
} from "./automation-rule-form"

type Props = {
  projectSlug: string
  workspaceSlug: string
}

// ProjectAutomationsPage 是项目自动化 tab 主页面：规则列表、编辑表单、预览弹窗和运行记录。
export function ProjectAutomationsPage({ projectSlug }: Props) {
  const layout = useProjectLayout()
  const queryClient = useQueryClient()
  const writeDisabled = layout.closed
  const [draft, setDraft] = useState<ProjectAutomationRuleInput>(defaultScheduleAutomationInput)
  const [preview, setPreview] = useState<ProjectAutomationPreview | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)

  const rulesQueryKey = ["project", projectSlug, "automations"]
  const rules = useQuery({
    queryKey: rulesQueryKey,
    queryFn: () => listProjectAutomations(projectSlug, true),
  })

  const invalidateRules = () => queryClient.invalidateQueries({ queryKey: rulesQueryKey })

  const previewMutation = useMutation({
    mutationFn: (input: ProjectAutomationRuleInput) => previewProjectAutomation(projectSlug, input),
    onSuccess: (data) => {
      setPreview(data)
      setPreviewOpen(true)
    },
  })
  const createMutation = useMutation({
    mutationFn: (input: ProjectAutomationRuleInput) => createProjectAutomation(projectSlug, input),
    onSuccess: () => invalidateRules(),
  })
  const testMutation = useMutation({
    mutationFn: (ruleID: string) => testProjectAutomationRule(projectSlug, ruleID),
  })

  if (rules.isPending) {
    return <Skeleton className="h-48 w-full" />
  }

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold">自动化</h2>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={writeDisabled}
            onClick={() => setDraft(assigneeFeishuTemplateInput)}
          >
            从模板创建
          </Button>
          <Button type="button" disabled={writeDisabled}>新建规则</Button>
        </div>
      </div>
      <div className="overflow-hidden rounded-md border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-muted/40 text-left">
              <th className="p-2">状态</th>
              <th className="p-2">名称</th>
              <th className="p-2">触发器</th>
              <th className="p-2">动作</th>
            </tr>
          </thead>
          <tbody>
            {(rules.data ?? []).map((rule) => (
              <tr key={rule.id} className="border-b">
                <td className="p-2">{rule.enabled ? "启用" : "停用"}</td>
                <td className="p-2 font-medium">{rule.name}</td>
                <td className="p-2">{triggerSummary(rule)}</td>
                <td className="p-2">Agent Provider</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <AutomationRuleForm
        value={draft}
        onChange={setDraft}
        onPreview={() => previewMutation.mutate(draft)}
        onSave={() => createMutation.mutate(draft)}
        onTest={() => {
          const firstRule = rules.data?.[0]
          if (firstRule) {
            testMutation.mutate(firstRule.id)
          }
        }}
        disabled={writeDisabled}
      />
      <AutomationDeliveryList projectSlug={projectSlug} />
      <AutomationPreviewDialog open={previewOpen} onOpenChange={setPreviewOpen} preview={preview} />
    </section>
  )
}

function triggerSummary(rule: {
  trigger_type: string
  trigger_config: { schedule_value?: string; event_type?: string }
}) {
  if (rule.trigger_type === "schedule") {
    return `schedule / 每天 ${rule.trigger_config.schedule_value ?? ""}`
  }
  return `event / ${rule.trigger_config.event_type ?? ""}`
}
