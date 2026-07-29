import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import * as projectApi from "@/features/workspace/project-workbench/api/project-api"
import { renderWithRouter } from "@/test/router-wrapper"

import { AutomationProviderConfigSection } from "./automation-provider-config"

const feedback = { failure: vi.fn(), success: vi.fn() }
vi.mock("@/features/workspace/project-workbench/shared/edit-feedback", () => ({
  useEditFeedback: () => feedback,
}))

vi.mock("@/features/workspace/project-workbench/api/project-api", () => ({
  listProjectConfig: vi.fn(),
  setProjectConfig: vi.fn(async () => undefined),
}))

// useMe 走网络请求，这里直接 mock 成内存对象，避免测试真的发起 fetch。
vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({ data: { effective_workspace: { slug: "acme" } } }),
}))

function renderSection() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    renderWithRouter(
      <QueryClientProvider client={qc}>
        <AutomationProviderConfigSection projectSlug="adsops" workspaceSlug="local" />
      </QueryClientProvider>
    )
  )
}

describe("AutomationProviderConfigSection", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
  })

  it("declares writes go to project-level config and links to workspace config", async () => {
    vi.mocked(projectApi.listProjectConfig).mockResolvedValue([
      { key: "agent.provider.base_url", value: "" },
    ])
    renderSection()
    // 声明写入 project 级配置
    expect(await screen.findByText(/当前项目（project 级）配置/)).toBeTruthy()
    // 引导链接指向当前 workspace（slug=acme）
    const link = screen.getByRole("link", { name: "acme" })
    expect(link.getAttribute("href")).toContain("/workspaces/acme/config")
  })

  it("shows missing hint when provider config incomplete", async () => {
    vi.mocked(projectApi.listProjectConfig).mockResolvedValue([
      { key: "agent.provider.base_url", value: "" },
    ])
    renderSection()
    expect(await screen.findByText(/缺少/)).toBeTruthy()
    expect(screen.getByText("预览和测试需要先配置 Agent Provider")).toBeTruthy()
  })

  it("shows configured status when all provider keys set", async () => {
    vi.mocked(projectApi.listProjectConfig).mockResolvedValue([
      { key: "agent.provider.base_url", value: "https://agent.example.com" },
      { key: "agent.provider.api_key", value: "sk-test" },
      { key: "agent.provider.model", value: "project-operator" },
      { key: "agent.provider.allowed_hosts", value: '["agent.example.com"]' },
    ])
    renderSection()
    expect(await screen.findByText("已配置")).toBeTruthy()
  })

  it("treats provider config as complete without allowed_hosts", async () => {
    // allowed_hosts 是可选项，未配置时不应阻塞（不显示「缺少」）。
    vi.mocked(projectApi.listProjectConfig).mockResolvedValue([
      { key: "agent.provider.base_url", value: "https://agent.example.com" },
      { key: "agent.provider.api_key", value: "sk-test" },
      { key: "agent.provider.model", value: "project-operator" },
    ])
    renderSection()
    expect(await screen.findByText("已配置")).toBeTruthy()
    expect(screen.queryByText(/缺少/)).toBeNull()
  })

  it("saves config via setProjectConfig", async () => {
    // 预填一部分，保证 draft 基线有值；测试只覆盖 api_key 输入和保存。
    vi.mocked(projectApi.listProjectConfig).mockResolvedValue([
      { key: "agent.provider.base_url", value: "https://agent.example.com" },
      { key: "agent.provider.model", value: "project-operator" },
      { key: "agent.provider.allowed_hosts", value: '["agent.example.com"]' },
    ])
    renderSection()
    const apiKeyInput = await screen.findByPlaceholderText("sk-...")
    await userEvent.type(apiKeyInput, "sk-secret")
    await userEvent.click(screen.getByRole("button", { name: "保存配置" }))
    await waitFor(() => expect(projectApi.setProjectConfig).toHaveBeenCalled())
    const calls = vi.mocked(projectApi.setProjectConfig).mock.calls
    const keys = calls.map((c) => c[2])
    expect(keys).toContain("agent.provider.base_url")
    expect(keys).toContain("agent.provider.api_key")
    expect(keys).toContain("agent.provider.model")
    expect(keys).toContain("agent.provider.allowed_hosts")
    expect(feedback.success).toHaveBeenCalledWith("Agent Provider 配置已保存")
  })
})
