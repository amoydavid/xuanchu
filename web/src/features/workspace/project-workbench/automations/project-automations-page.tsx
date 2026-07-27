import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { MoreHorizontal } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { RuleStatusDot } from "@/features/workspace/automations/shared/automation-status"
import { listProjectConfig } from "@/features/workspace/project-workbench/api/project-api"
import { useProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"
import { DestructiveConfirmDialog } from "@/features/workspace/project-workbench/shared/destructive-confirm-dialog"
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"
import { describeCron } from "@/features/workspace/shared/cron-schedule"

import {
  deleteProjectAutomation,
  listProjectAutomations,
  testProjectAutomationRule,
  useToggleProjectAutomationRule,
  type ProjectAutomationRule,
  type ProjectAutomationRuleInput,
} from "./project-automations-api"
import { AutomationDeliveryList } from "./automation-delivery-list"
import { AutomationProviderConfigSection, isProviderConfigComplete, providerConfigFromEntries } from "./automation-provider-config"
import { AutomationRuleDialog } from "./automation-rule-dialog"
import { assigneeFeishuTemplateInput } from "./automation-rule-form"
import { AutomationTestDebugDialog } from "./automation-test-debug-dialog"

type Props = {
  projectSlug: string
  workspaceSlug: string
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// ProjectAutomationsPage 是项目自动化 tab 主页面：规则列表、新建/编辑弹窗和运行记录。
export function ProjectAutomationsPage({ projectSlug, workspaceSlug }: Props) {
  const layout = useProjectLayout()
  const feedback = useEditFeedback()
  const queryClient = useQueryClient()
  const writeDisabled = layout.closed

  const [creating, setCreating] = useState(false)
  const [createTemplate, setCreateTemplate] = useState<ProjectAutomationRuleInput | null>(null)
  const [editing, setEditing] = useState<ProjectAutomationRule | null>(null)
  const [deleting, setDeleting] = useState<ProjectAutomationRule | null>(null)
  // debugDeliveryId 非 null 时打开测试投递 Debug 弹窗。
  const [debugDeliveryId, setDebugDeliveryId] = useState<string | null>(null)

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

  const testMutation = useMutation({
    mutationFn: (ruleID: string) => testProjectAutomationRule(projectSlug, ruleID),
    onSuccess: (delivery) => {
      feedback.success("已创建测试投递")
      queryClient.invalidateQueries({ queryKey: ["project", projectSlug, "automation-deliveries"] })
      if (delivery?.id) {
        setDebugDeliveryId(delivery.id)
      }
    },
    onError: (err) => {
      feedback.failure("测试失败", errorMessage(err))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (ruleID: string) => deleteProjectAutomation(projectSlug, ruleID),
    onSuccess: () => {
      setDeleting(null)
      invalidateRules()
    },
    onError: (err) => {
      feedback.failure("删除失败", errorMessage(err))
    },
  })

  // enable/disable 共用一个 mutation；onSuccess 内由调用方决定 success 文案。
  const toggle = useToggleProjectAutomationRule(projectSlug)

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
            onClick={() => {
              setCreateTemplate(assigneeFeishuTemplateInput)
              setCreating(true)
            }}
          >
            从模板创建
          </Button>
          <Button
            type="button"
            disabled={writeDisabled}
            onClick={() => {
              setCreateTemplate(null)
              setCreating(true)
            }}
          >
            新建规则
          </Button>
        </div>
      </div>
      <div className="overflow-hidden rounded-md border">
        <table className="w-full table-fixed text-sm">
          <colgroup>
            <col className="w-[48px]" />
            <col />
            <col className="w-[180px]" />
            <col className="w-[120px]" />
            <col className="w-[48px]" />
          </colgroup>
          <thead>
            <tr className="border-b bg-muted/40 text-left">
              <th className="p-2">状态</th>
              <th className="p-2">名称</th>
              <th className="p-2">触发器</th>
              <th className="p-2">动作</th>
              <th className="p-2" aria-label="操作" />
            </tr>
          </thead>
          <tbody>
            {(rules.data ?? []).map((rule) => (
              <tr key={rule.id} className="border-b">
                <td className="p-2">
                  <div className="flex items-center gap-2">
                    <RuleStatusDot enabled={rule.enabled} />
                    <span className="sr-only">{rule.enabled ? "启用" : "停用"}</span>
                  </div>
                </td>
                <td className="p-2 font-medium">
                  <span className="block truncate" title={rule.name}>
                    {rule.name}
                  </span>
                </td>
                <td className="p-2">
                  <span className="block truncate" title={triggerSummary(rule)}>
                    {triggerSummary(rule)}
                  </span>
                </td>
                <td className="p-2">Agent Provider</td>
                <td className="p-2 text-right">
                  {writeDisabled ? null : (
                    <ProjectRuleActions
                      rule={rule}
                      togglePending={toggle.isPending && toggle.variables?.ruleId === rule.id}
                      testPending={testMutation.isPending}
                      onToggle={() =>
                        toggle.mutate(
                          { ruleId: rule.id, enable: !rule.enabled },
                          {
                            onSuccess: () => {
                              invalidateRules()
                              feedback.success(!rule.enabled ? "规则已启用" : "规则已停用")
                            },
                            onError: (err) => feedback.failure("操作失败", errorMessage(err)),
                          },
                        )
                      }
                      onEdit={() => setEditing(rule)}
                      onTest={() => testMutation.mutate(rule.id)}
                      onDelete={() => setDeleting(rule)}
                    />
                  )}
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
      <AutomationDeliveryList projectSlug={projectSlug} onSelectDelivery={(id) => setDebugDeliveryId(id)} />
      {/* 新建弹窗 */}
      <AutomationRuleDialog
        key={creating ? "creating-open" : "creating-closed"}
        open={creating}
        onOpenChange={(o) => !o && setCreating(false)}
        onSaved={invalidateRules}
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
        template={createTemplate}
        disabled={writeDisabled}
      />
      {/* 编辑弹窗 */}
      <AutomationRuleDialog
        key={editing?.id ?? "editing-closed"}
        open={!!editing}
        onOpenChange={(o) => !o && setEditing(null)}
        onSaved={invalidateRules}
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
        initial={editing ?? undefined}
        disabled={writeDisabled}
      />
      {/* 删除确认 */}
      <DestructiveConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="删除自动化规则"
        description={`确认删除规则「${deleting?.name ?? ""}」？此操作不可撤销。`}
        confirmLabel="删除"
        pending={deleteMutation.isPending}
        onConfirm={() => {
          if (deleting) {
            deleteMutation.mutate(deleting.id)
          }
        }}
      />
      {/* 测试投递 Debug 弹窗 */}
      <AutomationTestDebugDialog
        open={debugDeliveryId !== null}
        onOpenChange={(o) => !o && setDebugDeliveryId(null)}
        projectSlug={projectSlug}
        deliveryID={debugDeliveryId}
      />
    </section>
  )
}

// ProjectRuleActions 把行内操作收进 ⋯ 下拉，与工作区页 RuleActions 视觉一致。
// 项目页比工作区页多一个「立即测试」入口。
function ProjectRuleActions({
  rule,
  togglePending,
  testPending,
  onToggle,
  onEdit,
  onTest,
  onDelete,
}: {
  rule: ProjectAutomationRule
  togglePending: boolean
  testPending: boolean
  onToggle: () => void
  onEdit: () => void
  onTest: () => void
  onDelete: () => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="h-8 w-8 p-0" aria-label="操作">
          <MoreHorizontal className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={onEdit}>编辑</DropdownMenuItem>
        <DropdownMenuItem onClick={onToggle} disabled={togglePending}>
          {rule.enabled ? "停用" : "启用"}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onTest} disabled={testPending}>
          立即测试
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="text-destructive" onClick={onDelete}>
          删除
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function triggerSummary(rule: {
  trigger_type: string
  trigger_config: { schedule_type?: string; schedule_value?: string; event_type?: string }
}) {
  if (rule.trigger_type === "schedule") {
    const value = rule.trigger_config.schedule_value ?? ""
    if (rule.trigger_config.schedule_type === "cron") {
      // cron 用中文解读展示；解读不命中时回退为原始表达式。
      return `schedule / ${describeCron(value)}`
    }
    return `schedule / 每天 ${value}`
  }
  return `event / ${rule.trigger_config.event_type ?? ""}`
}
