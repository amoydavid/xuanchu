import type { ProjectAutomationRuleInput } from "./project-automations-api"

// AUTOMATION_EVENT_OPTIONS 是事件触发可选的事件类型，对齐后端 allowedHookEventTypes 白名单。
export const AUTOMATION_EVENT_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: "task.created", label: "task.created（任务创建）" },
  { value: "task.modified", label: "task.modified（任务修改）" },
  { value: "task.completed", label: "task.completed（任务完成）" },
  { value: "task.deleted", label: "task.deleted（任务删除）" },
  { value: "task.started", label: "task.started（任务开始）" },
  { value: "task.stopped", label: "task.stopped（任务停止）" },
  { value: "task.reopened", label: "task.reopened（任务重新打开）" },
  { value: "task.assigned", label: "task.assigned（任务分配）" },
  { value: "task.unassigned", label: "task.unassigned（移除负责人）" },
  { value: "task.blocked", label: "task.blocked（任务被阻塞）" },
  { value: "task.due_changed", label: "task.due_changed（截止日期变更）" },
  { value: "task.priority_changed", label: "task.priority_changed（优先级变更）" },
  { value: "task.project_changed", label: "task.project_changed（所属项目变更）" },
  { value: "task.tags_changed", label: "task.tags_changed（标签变更）" },
  { value: "task.unblocked", label: "task.unblocked（任务解除阻塞）" },
  { value: "task.user_mentioned", label: "task.user_mentioned（用户被提及）" },
  { value: "project.archived", label: "project.archived（项目归档）" },
  { value: "project.transitioned", label: "project.transitioned（项目状态转移）" },
  { value: "project.annotated", label: "project.annotated（项目添加备注）" },
  { value: "project.denotated", label: "project.denotated（项目删除备注）" },
]

// defaultAutomationSystemPrompt 是 system prompt 的默认值，与后端 defaultAutomationSystemPrompt 一致。
export const defaultAutomationSystemPrompt = "你是项目自动化执行 Agent。你会收到来自璇础的项目上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。"

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
  system_prompt: defaultAutomationSystemPrompt,
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
  system_prompt: defaultAutomationSystemPrompt,
}

// weeklyReviewAutomationInput 是「每周项目回顾」模板的默认表单值，演示 cron 定时。
export const weeklyReviewAutomationInput: ProjectAutomationRuleInput = {
  name: "每周项目回顾",
  description: "",
  enabled: true,
  trigger_type: "schedule",
  trigger_config: { schedule_type: "cron", schedule_value: "0 9 * * 1", timezone: "Asia/Shanghai" },
  condition: { task_filter: "status:pending or status:waiting", max_tasks: 50 },
  action: defaultScheduleAutomationInput.action,
  context: { include: ["workspace", "project", "task_summary", "matched_tasks", "project_config"] },
  instruction_template: "请读取这个项目的本周任务执行情况，生成每周项目回顾报告。如果项目配置中包含飞书群信息，请自行处理发送。",
  system_prompt: defaultAutomationSystemPrompt,
}
