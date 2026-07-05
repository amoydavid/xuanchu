import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import type { Hook } from "../outbound-api"
import { HookFormDialog } from "./hook-form-dialog"

function renderDialog(props: Parameters<typeof HookFormDialog>[0]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <HookFormDialog {...props} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const sinksResponse = {
  data: [
    {
      id: "s1",
      workspace_id: "ws1",
      name: "audit-stream",
      type: "webhook",
      endpoint_mode: "static_url",
      url: "https://audit.example.com",
      enabled: true,
      timeout_seconds: 10,
      max_attempts: 3,
      max_concurrency: 0,
      created_at: 0,
      modified_at: 0,
    },
    {
      id: "s2",
      workspace_id: "ws1",
      name: "ci-webhook",
      type: "webhook",
      endpoint_mode: "static_url",
      url: "https://ci.example.com",
      enabled: false,
      timeout_seconds: 10,
      max_attempts: 3,
      max_concurrency: 0,
      created_at: 0,
      modified_at: 0,
    },
  ],
}

const projectsResponse = {
  data: [{ id: "p1", slug: "agentapi", name: "AgentAPI", status: "active" }],
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/notification-sinks")) {
      return Promise.resolve(new Response(JSON.stringify(sinksResponse), { status: 200 }))
    }
    if (url.startsWith("/api/v1/projects")) {
      return Promise.resolve(new Response(JSON.stringify(projectsResponse), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: {} }), { status: 200 }))
  })
}

describe("HookFormDialog", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("submits hook with sink selected from list and event checkboxes", async () => {
    const onSaved = vi.fn()
    renderDialog({ open: true, onOpenChange: () => {}, onSaved, workspaceSlug: "dajee" })

    await userEvent.type(screen.getByLabelText("名称"), "task-audit")
    // 选 sink
    const sinkSelect = screen.getByLabelText("目标 Sink") as HTMLSelectElement
    fireEvent.change(sinkSelect, { target: { value: "s1" } })
    // 勾选一个事件
    const completedLabel = Array.from(document.querySelectorAll("label")).find(
      (el) => el.textContent === "task.completed"
    )!
    const checkbox = completedLabel.querySelector('button[type="button"]')!
    await userEvent.click(checkbox)

    await userEvent.click(screen.getByRole("button", { name: /保存/ }))

    await waitFor(() => {
      expect(onSaved).toHaveBeenCalled()
    })
    const calls = vi.mocked(globalThis.fetch).mock.calls
    const createCall = calls.find(
      ([url]) => typeof url === "string" && url === "/api/v1/hooks"
    )
    expect(createCall).toBeTruthy()
    const body = (createCall![1] as RequestInit).body
    const parsed = JSON.parse(body as string)
    expect(parsed).toMatchObject({
      name: "task-audit",
      scope_type: "workspace",
      sink: "s1",
      event_types: ["task.completed"],
    })
  })

  it("requires project when scope_type is project", async () => {
    const onSaved = vi.fn()
    renderDialog({ open: true, onOpenChange: () => {}, onSaved, workspaceSlug: "dajee" })

    await userEvent.type(screen.getByLabelText("名称"), "project-ci")
    fireEvent.change(screen.getByLabelText("范围"), {
      target: { value: "project" },
    })
    const sinkSelect = screen.getByLabelText("目标 Sink") as HTMLSelectElement
    fireEvent.change(sinkSelect, { target: { value: "s1" } })
    const completedLabel = Array.from(document.querySelectorAll("label")).find(
      (el) => el.textContent === "project.archived"
    )!
    await userEvent.click(completedLabel.querySelector('button[type="button"]')!)

    await userEvent.click(screen.getByRole("button", { name: /保存/ }))

    await waitFor(() => {
      expect(screen.getByText(/项目: required/)).toBeTruthy()
    })
    expect(onSaved).not.toHaveBeenCalled()
  })

  it("preselects events and sink when editing", async () => {
    const initial: Hook = {
      id: "h1",
      name: "task-audit",
      scope_type: "workspace",
      workspace_id: "ws1",
      event_types: ["task.completed"],
      sink_id: "s1",
      sink_name: "audit-stream",
      sink_type: "webhook",
      enabled: true,
      timeout_seconds: 10,
      max_attempts: 3,
      created_at: 0,
      modified_at: 0,
    }
    renderDialog({ open: true, initial, onOpenChange: () => {}, onSaved: () => {}, workspaceSlug: "dajee" })

    await waitFor(() => {
      expect((screen.getByLabelText("名称") as HTMLInputElement).value).toBe(
        "task-audit"
      )
    })
    const sinkSelect = screen.getByLabelText("目标 Sink") as HTMLSelectElement
    expect(sinkSelect.value).toBe("s1")
  })

  it("shows warning when selecting disabled sink", async () => {
    renderDialog({ open: true, onOpenChange: () => {}, onSaved: () => {}, workspaceSlug: "dajee" })
    await waitFor(() => {
      expect(screen.getByLabelText("名称")).toBeTruthy()
    })
    const sinkSelect = screen.getByLabelText("目标 Sink") as HTMLSelectElement
    fireEvent.change(sinkSelect, { target: { value: "s2" } })
    await waitFor(() => {
      expect(screen.getByText(/所选 Sink 当前已暂停/)).toBeTruthy()
    })
  })
})
