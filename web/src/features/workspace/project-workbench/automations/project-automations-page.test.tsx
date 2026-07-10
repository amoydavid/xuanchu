import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { ProjectAutomationsPage } from "./project-automations-page"
import {
  createProjectAutomation,
  deleteProjectAutomation,
  getProjectAutomationDelivery,
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
  getProjectAutomationDelivery: vi.fn(),
  useAutomationTemplateVars: vi.fn(() => ({
    data: {
      triggers: [
        { trigger: "schedule", vars: [{ name: "project.slug", description: "项目 slug" }] },
        { trigger: "event", vars: [{ name: "event.type", description: "事件类型" }] },
      ],
    },
  })),
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
    vi.mocked(listProjectAutomationDeliveries).mockResolvedValue([])
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
        system_prompt: "",
        created_at: 1,
        modified_at: 1,
      },
    ])
    vi.mocked(previewProjectAutomation).mockResolvedValue(samplePreview)
    vi.mocked(createProjectAutomation).mockResolvedValue({ id: "rule-2" } as never)
    vi.mocked(updateProjectAutomation).mockResolvedValue({ id: "rule-1" } as never)
    vi.mocked(testProjectAutomationRule).mockResolvedValue({ id: "delivery-1" } as never)
    vi.mocked(deleteProjectAutomation).mockResolvedValue({ deleted: true } as never)
    await i18n.changeLanguage("zh-CN")
  })

  it("renders rules in the table", async () => {
    renderPage()
    expect(await screen.findByText("每日项目巡检")).toBeTruthy()
    expect(screen.getByText("schedule / 每天 09:30")).toBeTruthy()
  })

  it("opens create dialog and creates a rule", async () => {
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "新建规则" }))
    // 弹窗打开，标题为「新建自动化规则」。
    expect(await screen.findByRole("dialog", { name: "新建自动化规则" })).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "保存" }))
    await waitFor(() => expect(createProjectAutomation).toHaveBeenCalled())
    expect(feedback.success).toHaveBeenCalledWith("规则已创建")
  })

  it("opens create dialog from template with event prefilled", async () => {
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "从模板创建" }))
    const dialog = await screen.findByRole("dialog", { name: "新建自动化规则" })
    expect(dialog).toBeTruthy()
    expect(screen.getByDisplayValue("分配任务后拉群")).toBeTruthy()
    const eventTrigger = screen.getByRole("combobox", { name: "事件" })
    expect(eventTrigger.textContent).toContain("task.assigned")
  })

  it("opens edit dialog and updates a rule", async () => {
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "编辑" }))
    // 编辑弹窗标题为「编辑自动化规则」。
    expect(await screen.findByRole("dialog", { name: "编辑自动化规则" })).toBeTruthy()
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

  it("tests a rule and opens debug dialog", async () => {
    // 轮询返回 queued 状态（投递中）。
    vi.mocked(getProjectAutomationDelivery).mockResolvedValue({
      id: "delivery-1",
      workspace_id: "ws",
      project_id: "proj",
      rule_id: "rule-1",
      trigger_type: "manual_test",
      event_id: "",
      event_type: "",
      status: "queued",
      resolved_url: "https://agent.example.com/v1/chat/completions",
      rendered_method: "POST",
      rendered_headers: { Authorization: ["Bearer ****"], "Content-Type": ["application/json"] },
      request_body_preview: `{"model":"project-operator"}`,
      request_body_hash: "sha256:abc",
      response_body_preview: "",
      provider_request_id: "",
      usage: {},
      attempt_count: 0,
      last_error: "",
      created_at: 1,
      modified_at: 1,
    })
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "立即测试" }))
    await waitFor(() => expect(testProjectAutomationRule).toHaveBeenCalledWith("adsops", "rule-1"))
    // Debug 弹窗打开，标题含「测试投递结果」。
    expect(await screen.findByRole("dialog", { name: /测试投递结果/ })).toBeTruthy()
    // 请求 tab 展示 URL。
    expect(screen.getByText(/agent\.example\.com\/v1\/chat\/completions/)).toBeTruthy()
  })

  it("debug dialog shows response when delivery already succeeded", async () => {
    // 直接返回 succeeded 状态，验证 debug 弹窗展示响应数据。
    vi.mocked(getProjectAutomationDelivery).mockResolvedValue({
      id: "delivery-1", workspace_id: "ws", project_id: "proj", rule_id: "rule-1",
      trigger_type: "manual_test", event_id: "", event_type: "", status: "succeeded",
      resolved_url: "https://agent.example.com/v1/chat/completions", rendered_method: "POST",
      rendered_headers: {}, request_body_preview: `{"model":"x"}`, request_body_hash: "",
      response_body_preview: `{"id":"chatcmpl_123","usage":{"prompt_tokens":10}}`,
      response_status_code: 200, provider_request_id: "chatcmpl_123",
      usage: { prompt_tokens: 10, completion_tokens: 4 }, attempt_count: 1,
      last_error: "", created_at: 1, modified_at: 2,
    } as never)
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "立即测试" }))
    // Debug 弹窗打开并展示成功状态。
    await waitFor(() => expect(screen.getByText("成功")).toBeTruthy())
    // usage tab 展示 prompt_tokens。
    await userEvent.click(screen.getByRole("tab", { name: "Usage" }))
    await waitFor(() => expect(screen.getByText("prompt_tokens")).toBeTruthy())
  })

  it("deletes a rule with confirmation", async () => {
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "删除" }))
    // AlertDialog 出现。
    expect(await screen.findByText("删除自动化规则")).toBeTruthy()
    // 确认按钮在 AlertDialog 中，是页面上第二个「删除」按钮。
    const deleteButtons = screen.getAllByRole("button", { name: "删除" })
    await userEvent.click(deleteButtons[deleteButtons.length - 1])
    await waitFor(() => expect(deleteProjectAutomation).toHaveBeenCalledWith("adsops", "rule-1"))
  })

  it("disables write buttons for closed projects", async () => {
    layoutState.closed = true
    renderPage()
    expect(await screen.findByText("每日项目巡检")).toBeTruthy()
    expect((screen.getByRole("button", { name: "新建规则" }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole("button", { name: "编辑" }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole("button", { name: "删除" }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole("button", { name: "立即测试" }) as HTMLButtonElement).disabled).toBe(true)
  })

  it("opens preview dialog from within the create dialog", async () => {
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "新建规则" }))
    await screen.findByRole("dialog", { name: "新建自动化规则" })
    await userEvent.click(screen.getByRole("button", { name: "预览投递 JSON" }))
    expect(await screen.findByRole("dialog", { name: "预览投递 JSON" })).toBeTruthy()
    await waitFor(() =>
      expect(screen.getByText(/https:\/\/agent.example.com\/v1\/chat\/completions/)).toBeTruthy()
    )
  })

  it("reports feedback when preview fails", async () => {
    vi.mocked(previewProjectAutomation).mockRejectedValue(new Error("缺少 config:agent.provider.base_url"))
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "新建规则" }))
    await screen.findByRole("dialog", { name: "新建自动化规则" })
    await userEvent.click(screen.getByRole("button", { name: "预览投递 JSON" }))
    await waitFor(() => expect(feedback.failure).toHaveBeenCalledWith("预览失败", expect.stringContaining("base_url")))
  })
})
