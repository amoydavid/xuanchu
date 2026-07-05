import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import type { NotificationSink } from "../outbound-api"
import { SinkTestDialog } from "./sink-test-dialog"

function renderDialog(props: Parameters<typeof SinkTestDialog>[0]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <SinkTestDialog {...props} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const sink: NotificationSink = {
  id: "s1",
  workspace_id: "ws1",
  name: "audit-stream",
  type: "webhook",
  endpoint_mode: "static_url",
  url: "https://example.com",
  enabled: true,
  timeout_seconds: 10,
  max_attempts: 3,
  max_concurrency: 0,
  created_at: 0,
  modified_at: 0,
}

const configSink: NotificationSink = {
  ...sink,
  id: "s2",
  name: "feishu",
  endpoint_mode: "config_value",
  config_key: "feishu.webhook",
}

const testResponse = {
  data: {
    status: "succeeded",
    status_code: 200,
    duration_ms: 42,
    resolved_endpoint_source: "static_url",
    resolved_endpoint_fingerprint: "sha256:abcd",
    rendered_method: "POST",
    rendered_headers: { "Content-Type": ["application/json"] },
    rendered_body_preview: "{}",
    error: "",
  },
}

describe("SinkTestDialog", () => {
  beforeEach(async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify(testResponse), { status: 200 }))
    )
    await i18n.changeLanguage("zh-CN")
  })

  it("defaults to kind=hook, event_type=task.completed", () => {
    renderDialog({ sink, open: true, onOpenChange: () => {} })
    const eventSelect = screen.getByLabelText("事件类型") as HTMLSelectElement
    expect(eventSelect.value).toBe("task.completed")
    const kindSelect = screen.getByLabelText("投递类型") as HTMLSelectElement
    expect(kindSelect.value).toBe("hook")
  })

  it("shows project selector for config_value sinks", () => {
    renderDialog({ sink: configSink, open: true, onOpenChange: () => {} })
    expect(screen.getByLabelText("解析项目（可选）")).toBeTruthy()
  })

  it("hides project selector for static_url sinks", () => {
    renderDialog({ sink, open: true, onOpenChange: () => {} })
    expect(screen.queryByLabelText("解析项目（可选）")).toBeNull()
  })

  it("submits to sink test endpoint and renders status code", async () => {
    renderDialog({ sink, open: true, onOpenChange: () => {} })
    await userEvent.click(screen.getByRole("button", { name: /发送测试/ }))
    await waitFor(() => {
      expect(screen.getByText(/HTTP 200/)).toBeTruthy()
    })
    expect(vi.mocked(globalThis.fetch)).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/notification-sinks/s1/test"),
      expect.anything()
    )
  })

  it("renders failed result without closing dialog", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            data: {
              status: "failed",
              status_code: 500,
              duration_ms: 10,
              resolved_endpoint_source: "static_url",
              resolved_endpoint_fingerprint: "sha256:abcd",
              rendered_method: "POST",
              rendered_headers: {},
              rendered_body_preview: "{}",
              error: "boom",
            },
          }),
          { status: 200 }
        )
      )
    )
    renderDialog({ sink, open: true, onOpenChange: () => {} })
    await userEvent.click(screen.getByRole("button", { name: /发送测试/ }))
    await waitFor(() => {
      expect(screen.getByText(/HTTP 500/)).toBeTruthy()
      expect(screen.getByText(/error: boom/)).toBeTruthy()
    })
  })
})
