import { useQuery } from "@tanstack/react-query"
import { useState, type FormEvent } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { CronScheduleInput } from "@/features/workspace/shared/cron-schedule-input"
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"
import { getProjects } from "@/features/workspace/projects/projects-api"

import {
  useCreateWorkspaceAutomationRule,
  useModifyWorkspaceAutomationRule,
  useWorkspaceAutomationProviderConfig,
  type WorkspaceAutomationRule,
  type WorkspaceAutomationRuleInput,
  WORKSPACE_AUTOMATION_EVENTS,
} from "@/features/workspace/automations/workspace-automations-api"
import { AutomationPreviewDialog } from "@/features/workspace/project-workbench/automations/automation-preview-dialog"
import { ProviderConfigSummary } from "./provider-config-dialog"
import { WorkspaceTemplateVariablePicker } from "./workspace-template-variable-picker"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: () => void
  // initial 存在 → 编辑模式；否则新建。
  initial?: WorkspaceAutomationRule
  // 打开 Provider config Dialog 的回调，由父组件承载。
  onOpenProviderConfig: () => void
  canEdit: boolean
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function ruleToInput(rule: WorkspaceAutomationRule): WorkspaceAutomationRuleInput {
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
    system_prompt: rule.system_prompt,
  }
}

const defaultWorkspaceInput: WorkspaceAutomationRuleInput = {
  name: "",
  description: "",
  enabled: true,
  trigger_type: "event",
  trigger_config: { event_type: "project.created" },
  condition: { task_filter: "", max_tasks: 50 },
  action: {
    protocol: "chat_completions",
    base_url_config_key: "agent.provider.base_url",
    api_key_config_key: "agent.provider.api_key",
    model_config_key: "agent.provider.model",
    temperature: 0.2,
  },
  context: { include: ["workspace", "project", "project_config", "event"] },
  instruction_template: "",
  system_prompt: "",
}

const defaultWorkspaceScheduleInput: WorkspaceAutomationRuleInput = {
  name: "",
  description: "",
  enabled: true,
  trigger_type: "schedule",
  trigger_config: { schedule_type: "cron", schedule_value: "0 9 * * 1", timezone: "Asia/Shanghai" },
  condition: { task_filter: "", max_tasks: 50 },
  action: defaultWorkspaceInput.action,
  context: { include: ["workspace"] },
  instruction_template: "",
  system_prompt: "",
}

// WorkspaceAutomationRuleDialog 是 Workspace 规则新建/编辑 Dialog。
// 事件触发首版只接受 project.created；schedule 支持 daily_at / cron。
// preview 当前复用 Project automation preview dialog 展示组件；Workspace preview/test
// 在 sample project 选择后通过父组件承载（首版 Dialog 内只暴露 sample project 选择器
// 和「预览投递 JSON」按钮，真正的 sample-driven preview 由父组件 state 触发）。
export function WorkspaceAutomationRuleDialog({
  open,
  onOpenChange,
  onSaved,
  initial,
  onOpenProviderConfig,
  canEdit,
}: Props) {
  const feedback = useEditFeedback()
  const provider = useWorkspaceAutomationProviderConfig()
  const createMutation = useCreateWorkspaceAutomationRule()
  const modifyMutation = useModifyWorkspaceAutomationRule(initial?.id ?? "")

  const [form, setForm] = useState<WorkspaceAutomationRuleInput>(() =>
    initial ? ruleToInput(initial) : defaultWorkspaceInput
  )
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewBody, setPreviewBody] = useState<unknown>(null)
  // sample project 选择器：用于 event preview/test；schedule 不需要。
  const [sampleProject, setSampleProject] = useState("")

  // form 初始值在构造时通过 useState 设置；父组件用 key={editing?.id ?? creating}
  // 在打开不同规则/新建时重新挂载组件，避免 effect 级联 setState。

  const projectsQuery = useQuery({
    queryKey: ["projects", "open"],
    queryFn: () => getProjects(false),
    enabled: open && form.trigger_type === "event",
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const input = form
    if (initial) {
      modifyMutation.mutate(input, {
        onSuccess: () => {
          feedback.success("规则已更新")
          onSaved()
          onOpenChange(false)
        },
        onError: (err) => feedback.failure("更新失败", errorMessage(err)),
      })
    } else {
      createMutation.mutate(input, {
        onSuccess: () => {
          feedback.success("规则已创建")
          onSaved()
          onOpenChange(false)
        },
        onError: (err) => feedback.failure("保存失败", errorMessage(err)),
      })
    }
  }

  // preview 当前展示静态构造的 sample body（不调用后端 preview endpoint，避免 sample
  // project 校验引入额外复杂度）。父组件可在需要时升级为真实 preview。
  function handlePreview() {
    if (form.trigger_type === "event" && !sampleProject) {
      feedback.failure("预览失败", "事件预览需要选择一个 sample Project")
      return
    }
    const body = {
      model: "workspace-operator",
      messages: [
        { role: "system", content: form.system_prompt || "(系统默认提示词)" },
        { role: "user", content: form.instruction_template },
      ],
      temperature: form.action.temperature,
      _xuanchu: {
        automation_scope: "workspace",
        trigger_type: form.trigger_type,
        event_type: form.trigger_config.event_type ?? "",
        sample_project: sampleProject || undefined,
      },
    }
    setPreviewBody(body)
    setPreviewOpen(true)
  }

  const submitPending = createMutation.isPending || modifyMutation.isPending

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{initial ? "编辑工作空间自动化" : "新建工作空间自动化"}</DialogTitle>
            <DialogDescription className="sr-only">
              配置触发器、Provider 和指令模板；Workspace 规则监听整个工作空间。
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="grid gap-1.5">
              <Label htmlFor="wa-name">名称 *</Label>
              <Input
                id="wa-name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                disabled={!canEdit}
              />
            </div>

            <div className="grid gap-1.5">
              <Label>触发方式 *</Label>
              <select
                aria-label="触发方式"
                value={form.trigger_type}
                onChange={(e) => {
                  const next = e.target.value as "schedule" | "event"
                  setForm({
                    ...form,
                    trigger_type: next,
                    trigger_config:
                      next === "event"
                        ? { event_type: "project.created" }
                        : { schedule_type: "daily_at", schedule_value: "09:00", timezone: "Asia/Shanghai" },
                  })
                }}
                disabled={!canEdit}
                className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
              >
                <option value="event">工作空间事件</option>
                <option value="schedule">定时</option>
              </select>
            </div>

            {form.trigger_type === "event" ? (
              <div className="grid gap-1.5">
                <Label>事件</Label>
                <Select
                  value={form.trigger_config.event_type ?? ""}
                  onValueChange={(event_type) =>
                    setForm({ ...form, trigger_config: { ...form.trigger_config, event_type } })
                  }
                  disabled={!canEdit}
                >
                  <SelectTrigger aria-label="事件" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {WORKSPACE_AUTOMATION_EVENTS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ) : (
              <div className="grid gap-3">
                <div className="grid gap-1.5">
                  <Label>定时方式</Label>
                  <select
                    aria-label="定时方式"
                    value={form.trigger_config.schedule_type ?? "daily_at"}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        trigger_config: {
                          ...form.trigger_config,
                          schedule_type: e.target.value as "daily_at" | "cron",
                          schedule_value: "",
                        },
                      })
                    }
                    disabled={!canEdit}
                    className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
                  >
                    <option value="daily_at">每天定时</option>
                    <option value="cron">自定义 cron</option>
                  </select>
                </div>
                {form.trigger_config.schedule_type === "cron" ? (
                  <CronScheduleInput
                    value={form.trigger_config.schedule_value ?? ""}
                    timezone={form.trigger_config.timezone ?? "Asia/Shanghai"}
                    onChange={({ value, timezone }) =>
                      setForm({
                        ...form,
                        trigger_config: { ...form.trigger_config, schedule_value: value, timezone },
                      })
                    }
                    disabled={!canEdit}
                  />
                ) : (
                  <div className="flex items-center gap-2">
                    <Input
                      aria-label="时间"
                      value={form.trigger_config.schedule_value ?? ""}
                      onChange={(e) =>
                        setForm({
                          ...form,
                          trigger_config: { ...form.trigger_config, schedule_value: e.target.value },
                        })
                      }
                      disabled={!canEdit}
                      placeholder="09:00"
                    />
                    <Select
                      value={form.trigger_config.timezone ?? "Asia/Shanghai"}
                      onValueChange={(timezone) =>
                        setForm({ ...form, trigger_config: { ...form.trigger_config, timezone } })
                      }
                      disabled={!canEdit}
                    >
                      <SelectTrigger aria-label="时区" className="w-[160px]">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="Asia/Shanghai">Asia/Shanghai</SelectItem>
                        <SelectItem value="UTC">UTC</SelectItem>
                        <SelectItem value="America/Los_Angeles">America/Los_Angeles</SelectItem>
                        <SelectItem value="America/New_York">America/New_York</SelectItem>
                        <SelectItem value="Europe/London">Europe/London</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                )}
              </div>
            )}

            {/* Provider 摘要 + 查看配置入口。Project 样本不会覆盖这里的 Provider。 */}
            <div className="space-y-1.5">
              <button
                type="button"
                onClick={onOpenProviderConfig}
                className="block w-full text-left"
              >
                <ProviderConfigSummary data={provider.data} />
              </button>
              <p className="text-xs text-muted-foreground">
                Workspace Provider 只读 Workspace config。点击上方卡片查看/编辑配置。
              </p>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="wa-instruction">执行指令 *</Label>
              <Textarea
                id="wa-instruction"
                value={form.instruction_template}
                onChange={(e) => setForm({ ...form, instruction_template: e.target.value })}
                disabled={!canEdit}
                rows={6}
              />
              <div className="flex items-center justify-between">
                <p className="text-xs text-muted-foreground">
                  提示：写清目标、前置条件、幂等条件和 project_config_set 回写规则。
                </p>
                <WorkspaceTemplateVariablePicker
                  trigger={form.trigger_type}
                  onInsert={(token) => setForm({ ...form, instruction_template: form.instruction_template + token })}
                  disabled={!canEdit}
                />
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="wa-system-prompt">系统提示词（可选）</Label>
              <Textarea
                id="wa-system-prompt"
                value={form.system_prompt}
                onChange={(e) => setForm({ ...form, system_prompt: e.target.value })}
                disabled={!canEdit}
                rows={3}
                placeholder="留空使用默认 Workspace 自动化提示词"
              />
              <WorkspaceTemplateVariablePicker
                trigger={form.trigger_type}
                onInsert={(token) => setForm({ ...form, system_prompt: form.system_prompt + token })}
                disabled={!canEdit}
              />
            </div>

            {form.trigger_type === "event" ? (
              <div className="grid gap-1.5">
                <Label>测试样本（用于预览/立即测试）</Label>
                <Select value={sampleProject} onValueChange={setSampleProject} disabled={!canEdit}>
                  <SelectTrigger aria-label="测试样本 Project" className="w-full">
                    <SelectValue placeholder="选择一个现有 Project 用于预览/立即测试" />
                  </SelectTrigger>
                  <SelectContent>
                    {(projectsQuery.data ?? []).map((p) => (
                      <SelectItem key={p.id} value={p.slug}>
                        {p.slug} · {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  事件预览/测试必须选择当前 Workspace 的现有 Project；schedule 不需要。
                </p>
              </div>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={handlePreview}
                disabled={!canEdit}
              >
                预览投递 JSON
              </Button>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                取消
              </Button>
              <Button type="submit" disabled={!canEdit || submitPending}>
                {submitPending ? "保存中..." : initial ? "更新" : "保存并启用"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <AutomationPreviewDialog
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        preview={
          previewBody
            ? {
                method: "POST",
                url: "https://agent.example.com/v1/chat/completions",
                headers: {
                  Authorization: "Bearer ****",
                  "Content-Type": "application/json",
                },
                body: previewBody,
                warnings: [],
              }
            : null
        }
      />
    </>
  )
}

// defaultWorkspaceInputs 暴露给父组件用作「从模板创建」入口。
export const workspaceAutomationTemplates: Array<{ label: string; input: WorkspaceAutomationRuleInput }> = [
  { label: "新项目知识库初始化", input: defaultWorkspaceInput },
  { label: "每周项目治理巡检", input: defaultWorkspaceScheduleInput },
]
