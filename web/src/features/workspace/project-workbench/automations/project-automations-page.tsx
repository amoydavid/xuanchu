import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { listProjectConfig } from "@/features/workspace/project-workbench/api/project-api"
import { useProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"

import {
  createProjectAutomation,
  listProjectAutomations,
  previewProjectAutomation,
  testProjectAutomationRule,
  updateProjectAutomation,
  type ProjectAutomationPreview,
  type ProjectAutomationRule,
  type ProjectAutomationRuleInput,
} from "./project-automations-api"
import { AutomationDeliveryList } from "./automation-delivery-list"
import { AutomationPreviewDialog } from "./automation-preview-dialog"
import {
  AutomationProviderConfigSection,
  isProviderConfigComplete,
  providerConfigFromEntries,
} from "./automation-provider-config"
import {
  AutomationRuleForm,
  assigneeFeishuTemplateInput,
  defaultScheduleAutomationInput,
} from "./automation-rule-form"

type Props = {
  projectSlug: string
  workspaceSlug: string
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// ruleToInput 把已保存规则转换为表单输入结构（去掉服务端字段）。
function ruleToInput(rule: ProjectAutomationRule): ProjectAutomationRuleInput {
  return {
    name: rule.name,
    description: rule.description,
    enabled: rule.enabled,
    trigger_type: rule.trigger_type,
    trigger_config: rule.trigger_config,
    condition: rule.condition,
    action: rule.action,
    context: rule.context,
    instruction_template: rule.instruction_template,
  }
}

// ProjectAutomationsPage 是项目自动化 tab 主页面：规则列表、编辑表单、预览弹窗和运行记录。
export function ProjectAutomationsPage({ projectSlug, workspaceSlug }: Props) {
  const layout = useProjectLayout()
  const feedback = useEditFeedback()
  const queryClient = useQueryClient()
  const writeDisabled = layout.closed
  const [draft, setDraft] = useState<ProjectAutomationRuleInput>(defaultScheduleAutomationInput)
  // editingRuleId 非 null 时表示表单处于编辑已存在规则模式。
  const [editingRuleId, setEditingRuleId] = useState<string | null>(null)
  const [preview, setPreview] = useState<ProjectAutomationPreview | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)

  const rulesQueryKey = ["project", projectSlug, "automations"]
  const rules = useQuery({
    queryKey: rulesQueryKey,
    queryFn: () => listProjectAutomations(projectSlug, true),
  })

  // 读取 project config 判断 provider 是否已配齐；与 AutomationProviderConfigSection 共享同一 query key，React Query 自动去重。
  const configQueryKey = ["project", projectSlug, "config"]
  const projectConfig = useQuery({
    queryKey: configQueryKey,
    queryFn: () => listProjectConfig(workspaceSlug, projectSlug),
  })
  const providerComplete = projectConfig.data
    ? isProviderConfigComplete(providerConfigFromEntries(projectConfig.data))
    : false

  const invalidateRules = () => queryClient.invalidateQueries({ queryKey: rulesQueryKey })

  const previewMutation = useMutation({
    mutationFn: (input: ProjectAutomationRuleInput) => previewProjectAutomation(projectSlug, input),
    onSuccess: (data) => {
      setPreview(data)
      setPreviewOpen(true)
    },
    onError: (err) => {
      feedback.failure("预览失败", errorMessage(err))
    },
  })
  const createMutation = useMutation({
    mutationFn: (input: ProjectAutomationRuleInput) => createProjectAutomation(projectSlug, input),
    onSuccess: () => {
      feedback.success("规则已创建")
      setDraft(defaultScheduleAutomationInput)
      invalidateRules()
    },
    onError: (err) => {
      feedback.failure("保存失败", errorMessage(err))
    },
  })
  const updateMutation = useMutation({
    mutationFn: ({ id, input }: { id: string; input: ProjectAutomationRuleInput }) =>
      updateProjectAutomation(projectSlug, id, input),
    onSuccess: () => {
      feedback.success("规则已更新")
      setEditingRuleId(null)
      setDraft(defaultScheduleAutomationInput)
      invalidateRules()
    },
    onError: (err) => {
      feedback.failure("更新失败", errorMessage(err))
    },
  })
  const testMutation = useMutation({
    mutationFn: (ruleID: string) => testProjectAutomationRule(projectSlug, ruleID),
    onSuccess: () => {
      feedback.success("已创建测试投递")
    },
    onError: (err) => {
      feedback.failure("测试失败", errorMessage(err))
    },
  })

  // startEdit 把规则加载到表单并切换到编辑模式。
  const startEdit = (rule: ProjectAutomationRule) => {
    setEditingRuleId(rule.id)
    setDraft(ruleToInput(rule))
  }

  // cancelEdit 退出编辑模式，恢复默认 draft。
  const cancelEdit = () => {
    setEditingRuleId(null)
    setDraft(defaultScheduleAutomationInput)
  }

  if (rules.isPending) {
    return <Skeleton className="h-48 w-full" />
  }

  const savePending = createMutation.isPending || updateMutation.isPending

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold">自动化</h2>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={writeDisabled}
            onClick={() => {
              setEditingRuleId(null)
              setDraft(assigneeFeishuTemplateInput)
            }}
          >
            从模板创建
          </Button>
          <Button
            type="button"
            disabled={writeDisabled}
            onClick={() => {
              setEditingRuleId(null)
              setDraft(defaultScheduleAutomationInput)
            }}
          >
            新建规则
          </Button>
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
              <th className="p-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {(rules.data ?? []).map((rule) => (
              <tr key={rule.id} className="border-b">
                <td className="p-2">{rule.enabled ? "启用" : "停用"}</td>
                <td className="p-2 font-medium">{rule.name}</td>
                <td className="p-2">{triggerSummary(rule)}</td>
                <td className="p-2">Agent Provider</td>
                <td className="p-2">
                  <div className="flex gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={writeDisabled}
                      onClick={() => startEdit(rule)}
                    >
                      编辑
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={writeDisabled || testMutation.isPending}
                      onClick={() => testMutation.mutate(rule.id)}
                    >
                      立即测试
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {providerComplete ? null : (
        <AutomationProviderConfigSection
          projectSlug={projectSlug}
          workspaceSlug={workspaceSlug}
          disabled={writeDisabled}
        />
      )}
      <AutomationRuleForm
        value={draft}
        onChange={setDraft}
        onPreview={() => previewMutation.mutate(draft)}
        onSave={() => {
          if (editingRuleId) {
            updateMutation.mutate({ id: editingRuleId, input: draft })
          } else {
            createMutation.mutate(draft)
          }
        }}
        onCancel={editingRuleId ? cancelEdit : undefined}
        saveLabel={editingRuleId ? "更新" : "保存"}
        disabled={writeDisabled}
        previewPending={previewMutation.isPending}
        savePending={savePending}
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
