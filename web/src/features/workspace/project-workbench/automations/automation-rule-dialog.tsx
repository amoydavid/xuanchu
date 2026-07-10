import { useMutation } from "@tanstack/react-query"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"

import {
  createProjectAutomation,
  previewProjectAutomation,
  updateProjectAutomation,
  type ProjectAutomationPreview,
  type ProjectAutomationRule,
  type ProjectAutomationRuleInput,
} from "./project-automations-api"
import { AutomationPreviewDialog } from "./automation-preview-dialog"
import { TemplateVariablePicker } from "./template-variable-picker"
import {
  AUTOMATION_EVENT_OPTIONS,
  defaultAutomationSystemPrompt,
  defaultScheduleAutomationInput,
} from "./automation-rule-form"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: () => void
  projectSlug: string
  // initial 存在 → 编辑模式；否则新建模式。
  initial?: ProjectAutomationRule
  // template 新建时可预填模板（从模板创建）。
  template?: ProjectAutomationRuleInput | null
  disabled?: boolean
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
    system_prompt: rule.system_prompt,
  }
}

// AutomationRuleDialog 是新建/编辑自动化规则的弹窗表单，参照 sink-form-dialog 模式。
// initial 存在时为编辑模式，提交调 updateProjectAutomation；否则新建，调 createProjectAutomation。
export function AutomationRuleDialog({
  open,
  onOpenChange,
  onSaved,
  projectSlug,
  initial,
  template,
  disabled,
}: Props) {
  const feedback = useEditFeedback()
  const [form, setForm] = useState<ProjectAutomationRuleInput>(() =>
    initial ? ruleToInput(initial) : (template ?? defaultScheduleAutomationInput)
  )
  const [preview, setPreview] = useState<ProjectAutomationPreview | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (initial) {
        return updateProjectAutomation(projectSlug, initial.id, form)
      }
      return createProjectAutomation(projectSlug, form)
    },
    onSuccess: () => {
      feedback.success(initial ? "规则已更新" : "规则已创建")
      onSaved()
      onOpenChange(false)
    },
    onError: (err) => {
      feedback.failure(initial ? "更新失败" : "保存失败", errorMessage(err))
    },
  })

  const previewMutation = useMutation({
    mutationFn: () => previewProjectAutomation(projectSlug, form),
    onSuccess: (data) => {
      setPreview(data)
      setPreviewOpen(true)
    },
    onError: (err) => {
      feedback.failure("预览失败", errorMessage(err))
    },
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    saveMutation.mutate()
  }

  const submitPending = saveMutation.isPending

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{initial ? "编辑自动化规则" : "新建自动化规则"}</DialogTitle>
            <DialogDescription className="sr-only">
              配置触发器、动作和指令模板，保存后规则立即生效。
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="automation-name">名称</label>
              <Input
                id="automation-name"
                value={form.name}
                onChange={(event) => setForm({ ...form, name: event.target.value })}
                disabled={disabled}
              />
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="automation-trigger-type">触发类型</label>
              <select
                id="automation-trigger-type"
                aria-label="触发类型"
                value={form.trigger_type}
                onChange={(event) =>
                  setForm({
                    ...form,
                    trigger_type: event.target.value as ProjectAutomationRuleInput["trigger_type"],
                  })
                }
                disabled={disabled}
                className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
              >
                <option value="schedule">定时触发</option>
                <option value="event">事件触发</option>
              </select>
            </div>
            {form.trigger_type === "schedule" ? (
              <div className="grid gap-2">
                <label className="text-sm font-medium" htmlFor="automation-time">时间</label>
                <Input
                  id="automation-time"
                  aria-label="时间"
                  value={form.trigger_config.schedule_value ?? ""}
                  onChange={(event) =>
                    setForm({
                      ...form,
                      trigger_config: { ...form.trigger_config, schedule_value: event.target.value },
                    })
                  }
                  disabled={disabled}
                  placeholder="09:30"
                />
              </div>
            ) : (
              <div className="grid gap-2">
                <label className="text-sm font-medium">事件</label>
                <Select
                  value={form.trigger_config.event_type ?? ""}
                  onValueChange={(event_type) =>
                    setForm({
                      ...form,
                      trigger_config: { ...form.trigger_config, event_type },
                    })
                  }
                  disabled={disabled}
                >
                  <SelectTrigger aria-label="事件" className="w-full">
                    <SelectValue placeholder="选择事件" />
                  </SelectTrigger>
                  <SelectContent>
                    {AUTOMATION_EVENT_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="automation-system-prompt">系统提示词</label>
              <Textarea
                id="automation-system-prompt"
                value={form.system_prompt}
                onChange={(event) => setForm({ ...form, system_prompt: event.target.value })}
                disabled={disabled}
                rows={3}
                placeholder={defaultAutomationSystemPrompt}
              />
              <TemplateVariablePicker
                projectSlug={projectSlug}
                trigger={form.trigger_type}
                disabled={disabled}
                onInsert={(token) => setForm({ ...form, system_prompt: form.system_prompt + token })}
              />
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="automation-instruction">指令模板</label>
              <Textarea
                id="automation-instruction"
                value={form.instruction_template}
                onChange={(event) => setForm({ ...form, instruction_template: event.target.value })}
                disabled={disabled}
                rows={6}
              />
              <TemplateVariablePicker
                projectSlug={projectSlug}
                trigger={form.trigger_type}
                disabled={disabled}
                onInsert={(token) => setForm({ ...form, instruction_template: form.instruction_template + token })}
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => previewMutation.mutate()}
                disabled={disabled || previewMutation.isPending}
              >
                {previewMutation.isPending ? "生成中..." : "预览投递 JSON"}
              </Button>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                取消
              </Button>
              <Button type="submit" disabled={disabled || submitPending}>
                {submitPending ? "保存中..." : initial ? "更新" : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <AutomationPreviewDialog open={previewOpen} onOpenChange={setPreviewOpen} preview={preview} />
    </>
  )
}
