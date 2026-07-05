import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import type { NotificationSink } from "../outbound-api"
import { SinkFormDialog } from "./sink-form-dialog"

function renderDialog(props: Parameters<typeof SinkFormDialog>[0]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <SinkFormDialog {...props} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

describe("SinkFormDialog", () => {
  beforeEach(async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ data: {} }), { status: 200 }))
    )
    await i18n.changeLanguage("zh-CN")
  })

  it("creates a webhook sink with static_url by default", async () => {
    const onSaved = vi.fn()
    const onOpenChange = vi.fn()
    renderDialog({ open: true, onOpenChange, onSaved })

    await userEvent.type(screen.getByLabelText("名称"), "audit-stream")
    await userEvent.type(
      screen.getByLabelText("URL"),
      "https://audit.example.com/hook"
    )
    await userEvent.click(screen.getByRole("button", { name: /保存/ }))

    await waitFor(() => {
      expect(onSaved).toHaveBeenCalled()
    })
    const calls = vi.mocked(globalThis.fetch).mock.calls
    const createCall = calls.find(
      ([url]) => typeof url === "string" && url.includes("/api/v1/notification-sinks")
    )
    expect(createCall).toBeTruthy()
    const body = (createCall![1] as RequestInit).body
    const parsed = JSON.parse(body as string)
    expect(parsed).toMatchObject({
      name: "audit-stream",
      type: "webhook",
      endpoint_mode: "static_url",
      url: "https://audit.example.com/hook",
    })
    // webhook 创建时未填 secret，不应提交 secret 字段。
    expect(parsed.secret).toBeUndefined()
  })

  it("requires allowed_hosts for config_value endpoint", async () => {
    renderDialog({ open: true, onOpenChange: () => {}, onSaved: () => {} })
    await userEvent.type(screen.getByLabelText("名称"), "feishu-bot")
    const selects = document.querySelectorAll("select")
    fireEvent.change(selects[1], { target: { value: "config_value" } })
    await userEvent.click(screen.getByRole("button", { name: /保存/ }))
    await waitFor(() => {
      expect(screen.getByText(/allowed hosts/)).toBeTruthy()
    })
  })

  it("does not prefill secret when editing", async () => {
    const initial: NotificationSink = {
      id: "s1",
      workspace_id: "ws1",
      name: "audit-stream",
      type: "webhook",
      endpoint_mode: "static_url",
      url: "https://audit.example.com/hook",
      enabled: true,
      timeout_seconds: 10,
      max_attempts: 3,
      max_concurrency: 0,
      created_at: 0,
      modified_at: 0,
    }
    renderDialog({ open: true, initial, onOpenChange: () => {}, onSaved: () => {} })
    const secretInput = screen.getByPlaceholderText("••••••") as HTMLInputElement
    expect(secretInput.value).toBe("")
    // 编辑时 name 已预填
    expect((screen.getByLabelText("名称") as HTMLInputElement).value).toBe(
      "audit-stream"
    )
  })
})
