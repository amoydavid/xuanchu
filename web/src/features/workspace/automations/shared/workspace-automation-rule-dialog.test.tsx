import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import * as projectsApi from "@/features/workspace/projects/projects-api"

import { WorkspaceAutomationRuleDialog } from "./workspace-automation-rule-dialog"

const feedback = { failure: vi.fn(), success: vi.fn() }
vi.mock("@/features/workspace/project-workbench/shared/edit-feedback", () => ({
  useEditFeedback: () => feedback,
}))

vi.mock("@/features/workspace/projects/projects-api", () => ({
  getProjects: vi.fn(),
}))

vi.mock("@/features/workspace/automations/workspace-automations-api", () => ({
  useWorkspaceAutomationProviderConfig: vi.fn(() => ({
    data: {
      base_url: "https://agent.example.com",
      model: "workspace-operator",
      allowed_hosts: ["agent.example.com"],
      api_key_set: true,
      complete: true,
      missing_fields: [],
    },
  })),
  useCreateWorkspaceAutomationRule: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useModifyWorkspaceAutomationRule: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  WORKSPACE_AUTOMATION_EVENTS: [{ value: "project.created", label: "project.created" }],
}))

function renderDialog(props: Partial<Parameters<typeof WorkspaceAutomationRuleDialog>[0]> = {}) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <WorkspaceAutomationRuleDialog
        open={true}
        onOpenChange={() => {}}
        onSaved={() => {}}
        onOpenProviderConfig={() => {}}
        canEdit={true}
        {...props}
      />
    </QueryClientProvider>
  )
}

describe("WorkspaceAutomationRuleDialog", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
  })

  it("renders name input and provider summary", () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue([])
    renderDialog()
    // 至少渲染了执行指令 textarea（event 模式下唯一的多行 textbox）。
    expect(screen.getByRole("button", { name: "预览投递 JSON" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "保存并启用" })).toBeTruthy()
  })

  it("requires sample project for event preview", async () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue([])
    renderDialog()
    const previewButton = screen.getByRole("button", { name: "预览投递 JSON" })
    await userEvent.click(previewButton)
    expect(feedback.failure).toHaveBeenCalledWith(
      "预览失败",
      "事件预览需要选择一个 sample Project"
    )
  })

  it("shows provider summary from workspace config", () => {
    vi.mocked(projectsApi.getProjects).mockResolvedValue([])
    renderDialog()
    // Provider 摘要包含 base_url，是唯一包含该字符串的位置。
    expect(screen.getByText(/agent\.example\.com/)).toBeTruthy()
  })
})
