import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"

import type { ProjectAutomationRuleInput } from "./project-automations-api"

// AUTOMATION_EVENT_OPTIONS 是事件触发可选的事件类型，对齐后端 allowedHookEventTypes 白名单。
export const AUTOMATION_EVENT_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: "task.assigned", label: "task.assigned（任务分配）" },
  { value: "task.completed", label: "task.completed（任务完成）" },
  { value: "task.modified", label: "task.modified（任务修改）" },
  { value: "task.unblocked", label: "task.unblocked（任务解除阻塞）" },
  { value: "project.annotated", label: "project.annotated（项目备注变更）" },
  { value: "project.transitioned", label: "project.transitioned（项目状态转移）" },
]

// defaultScheduleAutomationInput 是「每日项目巡检」模板的默认表单值。
export const defaultScheduleAutomationInput: ProjectAutomationRuleInput = {
  name: "每日项目巡检",
  description: "",
  enabled: true,
  trigger_type: "schedule",
  trigger_config: { schedule_type: "daily_at", schedule_value: "09:30", timezone: "Asia/Shanghai" },
  condition: { task_filter: "status:pending or status:waiting", max_tasks: 50 },
  action: {
    protocol: "chat_completions",
    base_url_config_key: "agent.provider.base_url",
    api_key_config_key: "agent.provider.api_key",
    model_config_key: "agent.provider.model",
    temperature: 0.2,
  },
  context: { include: ["workspace", "project", "task_summary", "matched_tasks", "project_config"] },
  instruction_template: "请读取这个项目的任务执行情况，生成项目巡检报告。如果项目配置中包含飞书群信息，请自行处理发送。",
}

// assigneeFeishuTemplateInput 是「分配任务后拉群」模板的默认表单值。
export const assigneeFeishuTemplateInput: ProjectAutomationRuleInput = {
  name: "分配任务后拉群",
  description: "",
  enabled: true,
  trigger_type: "event",
  trigger_config: { event_type: "task.assigned" },
  condition: { only_added_assignees: true },
  action: defaultScheduleAutomationInput.action,
  context: { include: ["event", "task", "added_assignees", "project", "project_config"] },
  instruction_template: "有任务分配给了新负责人。请根据 added_assignees 和项目配置，完成后续协作动作。",
}

type Props = {
  value: ProjectAutomationRuleInput
  onChange: (value: ProjectAutomationRuleInput) => void
  onPreview: () => void
  onSave: () => void
  onCancel?: () => void
  disabled?: boolean
  previewPending?: boolean
  savePending?: boolean
  saveLabel?: string
}

// AutomationRuleForm 是定时/事件规则的编辑表单，受控组件。
export function AutomationRuleForm({
  value,
  onChange,
  onPreview,
  onSave,
  onCancel,
  disabled,
  previewPending,
  savePending,
  saveLabel,
}: Props) {
  return (
    <form className="space-y-4" onSubmit={(event) => event.preventDefault()}>
      <div className="grid gap-2">
        <label className="text-sm font-medium" htmlFor="automation-name">名称</label>
        <Input
          id="automation-name"
          value={value.name}
          onChange={(event) => onChange({ ...value, name: event.target.value })}
          disabled={disabled}
        />
      </div>
      <div className="grid gap-2">
        <label className="text-sm font-medium" htmlFor="automation-trigger-type">触发类型</label>
        <select
          id="automation-trigger-type"
          aria-label="触发类型"
          value={value.trigger_type}
          onChange={(event) =>
            onChange({
              ...value,
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
      {value.trigger_type === "schedule" ? (
        <div className="grid gap-2">
          <label className="text-sm font-medium" htmlFor="automation-time">时间</label>
          <Input
            id="automation-time"
            aria-label="时间"
            value={value.trigger_config.schedule_value ?? ""}
            onChange={(event) =>
              onChange({
                ...value,
                trigger_config: { ...value.trigger_config, schedule_value: event.target.value },
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
            value={value.trigger_config.event_type ?? ""}
            onValueChange={(event_type) =>
              onChange({
                ...value,
                trigger_config: { ...value.trigger_config, event_type },
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
        <label className="text-sm font-medium" htmlFor="automation-instruction">指令模板</label>
        <Textarea
          id="automation-instruction"
          value={value.instruction_template}
          onChange={(event) => onChange({ ...value, instruction_template: event.target.value })}
          disabled={disabled}
          rows={5}
        />
      </div>
      <div className="flex gap-2">
        <Button type="button" variant="outline" onClick={onPreview} disabled={disabled || previewPending}>
          {previewPending ? "生成中..." : "预览投递 JSON"}
        </Button>
        <Button type="button" onClick={onSave} disabled={disabled || savePending}>
          {savePending ? "保存中..." : (saveLabel ?? "保存")}
        </Button>
        {onCancel ? (
          <Button type="button" variant="outline" onClick={onCancel} disabled={disabled}>取消</Button>
        ) : null}
      </div>
    </form>
  )
}
