import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { ProjectAutomationsPage } from "./project-automations-page"
import {
  createProjectAutomation,
  listProjectAutomationDeliveries,
  listProjectAutomations,
  previewProjectAutomation,
  testProjectAutomationRule,
  updateProjectAutomation,
} from "./project-automations-api"

// 可变的 layout mock，允许单个测试切换 closed 状态。
const layoutState = { closed: false }

vi.mock("@/features/workspace/project-workbench/project/project-layout", () => ({
  useProjectLayout: () => ({
    closed: layoutState.closed,
    canManage: !layoutState.closed,
    project: { status: layoutState.closed ? "archived" : "active" },
    workspaceSlug: "local",
    projectSlug: "adsops",
    setTabActions: () => undefined,
    canReadTasks: true,
  }),
}))

// 页面依赖 EditFeedback（ProjectLayout 提供），单测未挂 provider，mock 为 vi.fn。
const feedback = { failure: vi.fn(), success: vi.fn() }
vi.mock("@/features/workspace/project-workbench/shared/edit-feedback", () => ({
  useEditFeedback: () => feedback,
}))

vi.mock("./project-automations-api", () => ({
  listProjectAutomations: vi.fn(),
  previewProjectAutomation: vi.fn(),
  createProjectAutomation: vi.fn(),
  updateProjectAutomation: vi.fn(),
  testProjectAutomationRule: vi.fn(),
  enableProjectAutomationRule: vi.fn(),
  disableProjectAutomationRule: vi.fn(),
  deleteProjectAutomation: vi.fn(),
  listProjectAutomationDeliveries: vi.fn(),
  EMPTY_AUTOMATION_PROVIDER_CONFIG: { base_url: "", api_key_set: false, model: "", allowed_hosts: "" },
  AUTOMATION_PROVIDER_KEYS: ["agent.provider.base_url"],
}))

// 页面集成 AutomationProviderConfigSection，需要 mock project-api 的 config 读写。
vi.mock("@/features/workspace/project-workbench/api/project-api", () => ({
  listProjectConfig: vi.fn(async () => [
    { key: "agent.provider.base_url", value: "https://agent.example.com" },
    { key: "agent.provider.api_key", value: "sk-test" },
    { key: "agent.provider.model", value: "project-operator" },
    { key: "agent.provider.allowed_hosts", value: '["agent.example.com"]' },
  ]),
  setProjectConfig: vi.fn(async () => undefined),
}))

const samplePreview = {
  method: "POST",
  url: "https://agent.example.com/v1/chat/completions",
  headers: { Authorization: "Bearer ****", "Content-Type": "application/json" },
  body: { model: "project-operator", messages: [] },
  warnings: [],
}

function renderPage() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <ProjectAutomationsPage projectSlug="adsops" workspaceSlug="local" />
    </QueryClientProvider>
  )
}

describe("ProjectAutomationsPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    layoutState.closed = false
    vi.mocked(listProjectAutomations).mockResolvedValue([
      {
        id: "rule-1",
        workspace_id: "ws",
        project_id: "proj",
        name: "每日项目巡检",
        description: "每天检查",
        enabled: true,
        trigger_type: "schedule",
        trigger_config: { schedule_type: "daily_at", schedule_value: "09:30", timezone: "Asia/Shanghai" },
        condition: { task_filter: "status:pending", max_tasks: 50 },
        action_type: "openai_compatible",
        action: {
          protocol: "chat_completions",
          base_url_config_key: "agent.provider.base_url",
          api_key_config_key: "agent.provider.api_key",
          model_config_key: "agent.provider.model",
          temperature: 0.2,
        },
        context: { include: ["workspace", "project", "task_summary", "matched_tasks", "project_config"] },
        instruction_template: "生成巡检",
        created_at: 1,
        modified_at: 1,
      },
    ])
    vi.mocked(previewProjectAutomation).mockResolvedValue(samplePreview)
    vi.mocked(createProjectAutomation).mockResolvedValue({ id: "rule-2" } as never)
    vi.mocked(updateProjectAutomation).mockResolvedValue({ id: "rule-1" } as never)
    vi.mocked(testProjectAutomationRule).mockResolvedValue({ id: "delivery-1" } as never)
    vi.mocked(listProjectAutomationDeliveries).mockResolvedValue([])
    await i18n.changeLanguage("zh-CN")
  })

  it("shows rules and opens delivery JSON preview", async () => {
    renderPage()
    expect(await screen.findByText("每日项目巡检")).toBeTruthy()
    expect(screen.getByText("schedule / 每天 09:30")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "预览投递 JSON" }))
    expect(await screen.findByRole("dialog", { name: "预览投递 JSON" })).toBeTruthy()
    await waitFor(() =>
      expect(screen.getByText(/https:\/\/agent.example.com\/v1\/chat\/completions/)).toBeTruthy()
    )
  })

  it("reports feedback and keeps dialog closed when preview fails", async () => {
    vi.mocked(previewProjectAutomation).mockRejectedValue(new Error("缺少 config:agent.provider.base_url"))
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "预览投递 JSON" }))
    await waitFor(() => expect(feedback.failure).toHaveBeenCalledWith("预览失败", expect.stringContaining("base_url")))
    expect(screen.queryByRole("dialog", { name: "预览投递 JSON" })).toBeNull()
  })

  it("disables write buttons for closed projects", async () => {
    layoutState.closed = true
    renderPage()
    expect(await screen.findByText("每日项目巡检")).toBeTruthy()
    expect((screen.getByRole("button", { name: "新建规则" }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole("button", { name: "立即测试" }) as HTMLButtonElement).disabled).toBe(true)
  })

  it("loads event template with event select set to task.assigned", async () => {
    renderPage()
    await userEvent.click(await screen.findByRole("button", { name: "从模板创建" }))
    expect(screen.getByDisplayValue("分配任务后拉群")).toBeTruthy()
    // 事件类型用 shadcn Select，trigger 是 combobox，展示选中值文本。
    const eventTrigger = screen.getByRole("combobox", { name: "事件" })
    expect(eventTrigger).toBeTruthy()
    expect(eventTrigger.textContent).toContain("task.assigned")
  })

  it("creates a new rule from template and tests it", async () => {
    renderPage()
    await userEvent.click(await screen.findByRole("button", { name: "从模板创建" }))
    await userEvent.click(screen.getByRole("button", { name: "保存" }))
    await waitFor(() => expect(createProjectAutomation).toHaveBeenCalled())
    expect(feedback.success).toHaveBeenCalledWith("规则已创建")
    // 保存成功后 draft 重置为默认 schedule 模板。
    expect(screen.getByDisplayValue("每日项目巡检")).toBeTruthy()
    // 立即测试按钮在规则行内。
    await userEvent.click(screen.getByRole("button", { name: "立即测试" }))
    await waitFor(() => expect(testProjectAutomationRule).toHaveBeenCalledWith("adsops", "rule-1"))
    expect(feedback.success).toHaveBeenCalledWith("已创建测试投递")
  })

  it("edits an existing rule and updates it", async () => {
    renderPage()
    await screen.findByText("每日项目巡检")
    // 点击规则行的编辑按钮，加载规则到表单。
    await userEvent.click(screen.getByRole("button", { name: "编辑" }))
    // 编辑模式下保存按钮文案为「更新」。
    expect(screen.getByRole("button", { name: "更新" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "取消" })).toBeTruthy()
    // 表单加载了规则名称。
    expect(screen.getByDisplayValue("每日项目巡检")).toBeTruthy()
    // 修改名称后点更新。
    const nameInput = screen.getByDisplayValue("每日项目巡检")
    await userEvent.clear(nameInput)
    await userEvent.type(nameInput, "每日巡检改")
    await userEvent.click(screen.getByRole("button", { name: "更新" }))
    await waitFor(() => expect(updateProjectAutomation).toHaveBeenCalled())
    const call = vi.mocked(updateProjectAutomation).mock.calls[0]
    expect(call[0]).toBe("adsops")
    expect(call[1]).toBe("rule-1")
    expect(feedback.success).toHaveBeenCalledWith("规则已更新")
  })
})
