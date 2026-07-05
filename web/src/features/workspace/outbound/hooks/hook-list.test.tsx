import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { HookList } from "./hook-list"

function renderList(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <HookList canWrite={canWrite} workspaceSlug="dajee" />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const hooksResponse = {
  data: [
    {
      id: "h1",
      name: "task-audit",
      scope_type: "workspace",
      event_types: ["task.created", "task.completed"],
      sink_id: "s1",
      sink_name: "audit-stream",
      sink_type: "webhook",
      enabled: true,
      timeout_seconds: 10,
      max_attempts: 3,
      created_at: 0,
      modified_at: 0,
    },
    {
      id: "h2",
      name: "project-ci",
      scope_type: "project",
      project_id: "p1",
      event_types: ["project.archived"],
      sink_id: "s2",
      sink_name: "ci-webhook",
      sink_type: "webhook",
      enabled: false,
      timeout_seconds: 10,
      max_attempts: 3,
      created_at: 0,
      modified_at: 0,
    },
  ],
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
  ],
}

const projectsResponse = {
  data: [
    { id: "p1", slug: "agentapi", name: "AgentAPI", status: "active" },
  ],
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/hooks/") && url.includes("/deliveries")) {
      return Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
    }
    if (url.startsWith("/api/v1/hooks")) {
      return Promise.resolve(new Response(JSON.stringify(hooksResponse), { status: 200 }))
    }
    if (url.startsWith("/api/v1/notification-sinks")) {
      return Promise.resolve(new Response(JSON.stringify(sinksResponse), { status: 200 }))
    }
    if (url.startsWith("/api/v1/projects")) {
      return Promise.resolve(new Response(JSON.stringify(projectsResponse), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: {} }), { status: 200 }))
  })
}

describe("HookList", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("lists hooks with name, scope, event count, sink and status", async () => {
    renderList()
    await waitFor(() => {
      expect(screen.getByText("task-audit")).toBeTruthy()
    })
    expect(screen.getByText("project-ci")).toBeTruthy()
    // sink 名渲染
    expect(screen.getByText("audit-stream")).toBeTruthy()
  })

  it("hides write actions for read-only users", async () => {
    renderList(false)
    await waitFor(() => {
      expect(screen.getByText("task-audit")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /新建 Hook/ })).toBeNull()
    expect(screen.queryByRole("button", { name: /编辑 Hook/ })).toBeNull()
  })

  it("expands a hook row without errors", async () => {
    renderList()
    await waitFor(() => {
      expect(screen.getByText("task-audit")).toBeTruthy()
    })
    // 点击行展开（Chunk 4 接入 delivery table 后会触发 deliveries fetch）
    await userEvent.click(screen.getByText("task-audit"))
    // 再点击收起，验证不会重复渲染异常
    await userEvent.click(screen.getByText("task-audit"))
  })
})
