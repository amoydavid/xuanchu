import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { ProjectAutomationsPage } from "./project-automations-page"
import {
  createProjectAutomation,
  testProjectAutomationRule,
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

vi.mock("./project-automations-api", async () => {
  return {
    listProjectAutomations: vi.fn(async () => [
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
    ]),
    previewProjectAutomation: vi.fn(async () => ({
      method: "POST",
      url: "https://agent.example.com/v1/chat/completions",
      headers: { Authorization: "Bearer ****", "Content-Type": "application/json" },
      body: { model: "project-operator", messages: [] },
      warnings: [],
    })),
    createProjectAutomation: vi.fn(async () => ({ id: "rule-2" })),
    updateProjectAutomation: vi.fn(async () => ({ id: "rule-1" })),
    testProjectAutomationRule: vi.fn(async () => ({ id: "delivery-1" })),
    enableProjectAutomationRule: vi.fn(async () => ({ id: "rule-1", enabled: true })),
    disableProjectAutomationRule: vi.fn(async () => ({ id: "rule-1", enabled: false })),
    deleteProjectAutomation: vi.fn(async () => ({ deleted: true })),
    listProjectAutomationDeliveries: vi.fn(async () => []),
  }
})

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

  it("disables write buttons for closed projects", async () => {
    layoutState.closed = true
    renderPage()
    expect(await screen.findByText("每日项目巡检")).toBeTruthy()
    expect((screen.getByRole("button", { name: "新建规则" }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole("button", { name: "立即测试" }) as HTMLButtonElement).disabled).toBe(true)
  })

  it("creates from template and tests a rule", async () => {
    renderPage()
    await userEvent.click(await screen.findByRole("button", { name: "从模板创建" }))
    expect(screen.getByDisplayValue("分配任务后拉群")).toBeTruthy()
    expect(screen.getByDisplayValue("task.assigned")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "保存" }))
    expect(createProjectAutomation).toHaveBeenCalled()
    await userEvent.click(screen.getByRole("button", { name: "立即测试" }))
    expect(testProjectAutomationRule).toHaveBeenCalledWith("adsops", "rule-1")
  })
})
