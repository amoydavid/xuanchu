import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"

import { TemplateVarHints } from "./template-var-hints"
import type { NotificationSink } from "../outbound-api"
import type { NotificationTemplateVarsView } from "./template-vars"

function wrap(client: QueryClient) {
  return render(
    <QueryClientProvider client={client}>
      <TemplateVarHints trigger="reminder" sink={null} />
    </QueryClientProvider>,
  )
}

const varsView: NotificationTemplateVarsView = {
  triggers: [
    {
      trigger: "reminder",
      fields: [
        { field: "endpoint", vars: [{ name: "workspace.id", description: "工作区 ID", dynamic: false }] },
        {
          field: "body",
          vars: [
            { name: "task.title", description: "任务标题", dynamic: false },
            { name: "secret.*", description: "声明的密钥", dynamic: true, prefix_group: "secret" },
          ],
        },
      ],
    },
    { trigger: "event", fields: [{ field: "body", vars: [{ name: "event.type", description: "事件类型", dynamic: false }] }] },
  ],
}

const httpTemplateSink: NotificationSink = {
  id: "s1",
  workspace_id: "ws1",
  name: "feishu",
  type: "http_template",
  endpoint_mode: "static_url",
  body_template: '{"text":"{{task.title}}"}',
  enabled: true,
  timeout_seconds: 10,
  max_attempts: 5,
  max_concurrency: 1,
  created_at: 0,
  modified_at: 0,
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/notification-template-vars")) {
      // 后端返回 {data: ...} 信封，requestJson 解包后返回 payload.data。
      return Promise.resolve(new Response(JSON.stringify({ data: varsView }), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
  })
}

describe("TemplateVarHints", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("reminder trigger 显示 task.title，不显示 event.type", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    wrap(client)
    await waitFor(() => {
      expect(screen.getByText("task.title")).toBeTruthy()
    })
    expect(screen.queryByText("event.type")).toBeNull()
  })

  it("传入 http_template sink 时显示 body 预览并高亮变量", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <TemplateVarHints trigger="reminder" sink={httpTemplateSink} />
      </QueryClientProvider>,
    )
    await waitFor(() => {
      // body 预览里的变量被高亮成 chip
      expect(screen.getByText("{{task.title}}")).toBeTruthy()
    })
  })

  it("传入 webhook sink 时不显示 body 预览", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const webhookSink: NotificationSink = { ...httpTemplateSink, type: "webhook" }
    render(
      <QueryClientProvider client={client}>
        <TemplateVarHints trigger="reminder" sink={webhookSink} />
      </QueryClientProvider>,
    )
    await waitFor(() => {
      expect(screen.getByText("task.title")).toBeTruthy()
    })
    expect(screen.queryByText("{{task.title}}")).toBeNull()
  })

  it("event trigger 显示 event.type，不显示 task.title", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <TemplateVarHints trigger="event" sink={null} />
      </QueryClientProvider>,
    )
    await waitFor(() => {
      expect(screen.getByText("event.type")).toBeTruthy()
    })
    expect(screen.queryByText("task.title")).toBeNull()
  })
})
