import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"

import type { ProjectAutomationRuleInput } from "./project-automations-api"

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
  onTest: () => void
  disabled?: boolean
}

// AutomationRuleForm 是定时/事件规则的编辑表单，受控组件。
export function AutomationRuleForm({ value, onChange, onPreview, onSave, onTest, disabled }: Props) {
  return (
    <form className="space-y-4" onSubmit={(event) => event.preventDefault()}>
      <div className="grid gap-2">
        <label className="text-sm font-medium">名称</label>
        <Input
          value={value.name}
          onChange={(event) => onChange({ ...value, name: event.target.value })}
          disabled={disabled}
        />
      </div>
      <div className="grid gap-2">
        <label className="text-sm font-medium">触发类型</label>
        <select
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
          <label className="text-sm font-medium">时间</label>
          <Input
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
          <Input
            value={value.trigger_config.event_type ?? ""}
            onChange={(event) =>
              onChange({
                ...value,
                trigger_config: { ...value.trigger_config, event_type: event.target.value },
              })
            }
            disabled={disabled}
            placeholder="task.assigned"
          />
        </div>
      )}
      <div className="grid gap-2">
        <label className="text-sm font-medium">指令模板</label>
        <Textarea
          value={value.instruction_template}
          onChange={(event) => onChange({ ...value, instruction_template: event.target.value })}
          disabled={disabled}
          rows={5}
        />
      </div>
      <div className="flex gap-2">
        <Button type="button" variant="outline" onClick={onPreview} disabled={disabled}>预览投递 JSON</Button>
        <Button type="button" onClick={onSave} disabled={disabled}>保存</Button>
        <Button type="button" variant="outline" onClick={onTest} disabled={disabled}>立即测试</Button>
      </div>
    </form>
  )
}
