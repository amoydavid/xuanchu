import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { renderWithRouter } from "@/test/router-wrapper"

import { WorkspaceConfigPage } from "./workspace-config-page"

// 构造一个 effective view 行。
function effectiveRow(
  key: string,
  source: "workspace" | "default" | "missing",
  opts: {
    value?: string
    allowedScopes?: string[]
    workspaceValue?: string | null
  } = {}
) {
  return {
    key,
    value: opts.value ?? null,
    source,
    workspace_value: opts.workspaceValue ?? null,
    default_value: null,
    definition: {
      key,
      value_type: "string",
      allowed_scopes: opts.allowedScopes ?? ["workspace"],
      label: "",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: false,
      created_at: 0,
      modified_at: 0,
    },
    show_on_console_home: false,
    missing_required: false,
  }
}

function renderPage(slug = "acme") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    renderWithRouter(
      <QueryClientProvider client={client}>
        <ThemeProvider>
          <TooltipProvider>
            <WorkspaceConfigPage workspaceSlug={slug} />
          </TooltipProvider>
        </ThemeProvider>
      </QueryClientProvider>
    )
  )
}

describe("WorkspaceConfigPage", () => {
  beforeEach(async () => {
    setWorkspaceToken("xuanchu_pat_test")
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = typeof input === "string" ? input : (input as Request).url
      if (url.includes("/credentials/current")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              data: {
                actor_type: "user",
                actor: { id: "u1", name: "U" },
                token: { type: "xuanchu_pat", scopes: ["config:write"] },
                effective_workspace: { slug: "acme", name: "ACME" },
                effective_role: "owner",
              },
            }),
            { status: 200 }
          )
        )
      }
      if (url.includes("/config/effective")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              data: [
                // workspace 可写 + 已配置：应有「编辑」按钮
                effectiveRow("agent.provider.base_url", "workspace", {
                  value: "https://a.example.com",
                  allowedScopes: ["workspace"],
                  workspaceValue: "https://a.example.com",
                }),
                // project-only：在 workspace 页只读
                effectiveRow("uda.cost", "missing", { allowedScopes: ["project"] }),
              ],
            }),
            { status: 200 }
          )
        )
      }
      return Promise.resolve(
        new Response(JSON.stringify({ data: {} }), { status: 200 })
      )
    })
    await i18n.changeLanguage("zh-CN")
  })

  it("renders effective rows for the current workspace", async () => {
    renderPage("acme")
    await waitFor(() => {
      expect(screen.getByText("agent.provider.base_url")).toBeTruthy()
    })
    expect(screen.getByText("uda.cost")).toBeTruthy()
  })

  it("allows editing workspace-scoped keys but marks project-only keys readonly", async () => {
    renderPage("acme")
    await waitFor(() => {
      expect(screen.getByText("uda.cost")).toBeTruthy()
    })
    // project-only 的 uda.cost 行显示「只读」badge
    expect(screen.getByText("只读")).toBeTruthy()
    // 该行没有「编辑」按钮
    const editButtons = screen.queryAllByRole("button", { name: /^编辑$/ })
    // uda.cost（project-only）无编辑按钮；agent.provider.base_url（workspace 可写）有
    // 共应有且仅有 1 个编辑按钮（来自 base_url 行）
    expect(editButtons.length).toBe(1)
  })

  it("shows readonly notice when slug is not the effective workspace", async () => {
    renderPage("other")
    await waitFor(() => {
      expect(
        screen.getByText(/仅可在当前工作空间上下文编辑配置/)
      ).toBeTruthy()
    })
  })
})
