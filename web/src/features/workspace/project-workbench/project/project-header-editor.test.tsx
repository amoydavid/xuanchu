import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { modifyProject } from "../api/project-api"
import { ProjectHeaderEditor } from "./project-header-editor"

vi.mock("../api/project-api", async () => {
  const actual = await vi.importActual<typeof import("../api/project-api")>(
    "../api/project-api"
  )
  return {
    ...actual,
    modifyProject: vi.fn(),
  }
})

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

function project() {
  return {
    id: "project-1",
    workspace_id: "workspace-1",
    slug: "adsops",
    name: "广告投放自动化",
    description: "每日巡检投放任务",
    status: "active",
    task_count: 12,
    pending_count: 7,
    completed_count: 5,
    created_at: 1,
    modified_at: 1,
  }
}

describe("ProjectHeaderEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(modifyProject).mockResolvedValue(project())
  })

  it("saves project name inline", async () => {
    render(
      <ProjectHeaderEditor
        canManage={true}
        onCopyLink={() => undefined}
        project={project()}
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "项目名称" }))
    await userEvent.clear(screen.getByLabelText("项目名称"))
    await userEvent.type(screen.getByLabelText("项目名称"), "投放日报{Enter}")

    expect(modifyProject).toHaveBeenCalledWith("acme", "adsops", {
      name: "投放日报",
    })
  })

  it("saves an empty description as an empty string", async () => {
    render(
      <ProjectHeaderEditor
        canManage={true}
        onCopyLink={() => undefined}
        project={project()}
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "项目描述" }))
    await userEvent.clear(screen.getByLabelText("项目描述"))
    await userEvent.keyboard("{Control>}{Enter}{/Control}")

    expect(modifyProject).toHaveBeenCalledWith("acme", "adsops", {
      description: "",
    })
  })

  it("renders plain text when project manage is denied", () => {
    render(
      <ProjectHeaderEditor
        canManage={false}
        onCopyLink={() => undefined}
        project={project()}
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.queryByRole("button", { name: "项目名称" })).toBeNull()
    expect(screen.getByText("广告投放自动化")).toBeTruthy()
    expect(screen.getByText("每日巡检投放任务")).toBeTruthy()
  })
})
