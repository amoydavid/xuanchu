import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { AutomationDeliveryList } from "./automation-delivery-list"
import { listProjectAutomationDeliveries } from "./project-automations-api"

vi.mock("./project-automations-api", () => ({
  listProjectAutomationDeliveries: vi.fn(),
}))

function renderList() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <AutomationDeliveryList projectSlug="adsops" />
    </QueryClientProvider>,
  )
}

describe("AutomationDeliveryList 状态列", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
  })

  it("用状态点 + 中文标签渲染投递中状态", async () => {
    vi.mocked(listProjectAutomationDeliveries).mockResolvedValue([
      {
        id: "d-1",
        workspace_id: "ws",
        project_id: "proj",
        rule_id: "r-1",
        trigger_type: "manual_test",
        event_id: "",
        event_type: "",
        status: "delivering",
        resolved_url: "https://agent.example.com",
        rendered_method: "POST",
        rendered_headers: {},
        request_body_preview: "{}",
        request_body_hash: "",
        response_body_preview: "",
        provider_request_id: "",
        usage: {},
        attempt_count: 0,
        last_error: "",
        created_at: 1,
        modified_at: 1,
      },
    ])
    renderList()
    expect(await screen.findByText("投递中")).toBeTruthy()
  })

  it("用状态点 + 中文标签渲染成功状态", async () => {
    vi.mocked(listProjectAutomationDeliveries).mockResolvedValue([
      {
        id: "d-2",
        workspace_id: "ws",
        project_id: "proj",
        rule_id: "r-1",
        trigger_type: "schedule",
        event_id: "",
        event_type: "",
        status: "succeeded",
        resolved_url: "https://agent.example.com",
        rendered_method: "POST",
        rendered_headers: {},
        request_body_preview: "{}",
        request_body_hash: "",
        response_body_preview: "",
        provider_request_id: "",
        usage: {},
        attempt_count: 1,
        last_error: "",
        created_at: 1,
        modified_at: 2,
      },
    ])
    renderList()
    expect(await screen.findByText("成功")).toBeTruthy()
  })

  it("空状态展示提示", async () => {
    vi.mocked(listProjectAutomationDeliveries).mockResolvedValue([])
    renderList()
    expect(await screen.findByText("暂无运行记录")).toBeTruthy()
  })
})
