import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"

// Mock 数据 hooks：默认返回空、非 loading。
vi.mock("@/features/workspace/automations/workspace-automations-api", () => ({
  useWorkspaceAutomationRules: vi.fn(() => ({ data: [], isLoading: false, isError: false })),
  useToggleWorkspaceAutomationRule: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useDeleteWorkspaceAutomationRule: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useWorkspaceAutomationDeliveries: vi.fn(() => ({ data: [] })),
  useReplayWorkspaceAutomationDelivery: vi.fn(() => ({ mutate: vi.fn() })),
  useWorkspaceAutomationProviderConfig: vi.fn(() => ({ data: { complete: true } })),
}))

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({ data: { effective_role: "owner", actor_type: "user", token: { type: "session", scopes: [] } } }),
}))

vi.mock("@/features/workspace/automations/workspace-automation-permissions", () => ({
  canWriteAutomation: () => true,
  canReadAutomation: () => true,
}))

vi.mock("@/features/workspace/project-workbench/shared/edit-feedback", () => ({
  useEditFeedback: () => ({ failure: vi.fn(), success: vi.fn() }),
}))

// Mock 三个 dialog 子组件为空壳，让测试聚焦 RulesTab 渲染。
vi.mock("@/features/workspace/automations/shared/workspace-automation-rule-dialog", () => ({
  WorkspaceAutomationRuleDialog: () => null,
}))
vi.mock("@/features/workspace/automations/shared/provider-config-dialog", () => ({
  ProviderConfigDialog: () => null,
}))
vi.mock("@/features/workspace/automations/shared/workspace-automation-delivery-detail", () => ({
  WorkspaceAutomationDeliveryDetail: () => null,
}))

import { WorkspaceAutomationsConsole } from "./workspace-automations-page"
import { useWorkspaceAutomationRules } from "@/features/workspace/automations/workspace-automations-api"

function renderPage() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <WorkspaceAutomationsConsole workspaceSlug="local" />
    </QueryClientProvider>,
  )
}

const longInstruction =
  "为新建任务自动指派飞书群组里的负责人，需要先查询项目配置中的默认群组再拉取成员列表并匹配任务 assignee 字段，这是一段很长的指令模板用于验证截断行为"

describe("WorkspaceAutomationsConsole 指令摘要列", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    vi.mocked(useWorkspaceAutomationRules).mockReturnValue({
      data: [
        {
          id: "rule-1",
          workspace_id: "ws",
          scope_type: "workspace",
          scope_id: "ws",
          project_id: null,
          name: "长指令规则",
          description: "",
          enabled: true,
          trigger_type: "event",
          trigger_config: { event_type: "project.created" },
          condition: {},
          action_type: "openai_compatible",
          action: {
            protocol: "chat_completions",
            base_url_config_key: "k",
            api_key_config_key: "k",
            model_config_key: "k",
            temperature: 0.2,
          },
          context: { include: [] },
          instruction_template: longInstruction,
          system_prompt: "",
          created_by: null,
          created_at: 1,
          modified_at: 1,
        },
      ],
      isLoading: false,
      isError: false,
    } as never)
    await i18n.changeLanguage("zh-CN")
  })

  it("指令摘要列单行截断且 title 属性含完整内容", async () => {
    renderPage()
    // 桌面表格与移动 card 都渲染该文本（md:hidden 在 jsdom 不生效），取第一个（桌面表格行）。
    const summaries = await screen.findAllByText(longInstruction)
    const summary = summaries[0]
    expect(summary).toBeTruthy()
    // title 提供 hover tooltip 完整内容
    expect(summary.getAttribute("title")).toBe(longInstruction)
    // 截断 class 存在
    expect(summary.className).toContain("truncate")
  })
})
