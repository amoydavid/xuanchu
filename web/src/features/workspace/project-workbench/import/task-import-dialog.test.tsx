import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { EditFeedbackProvider } from "../shared/edit-feedback"
import { importTasks } from "../api/task-api"
import {
  addWorkspaceMember,
  createWorkspaceUser,
  getWorkspaceMembers,
  listWorkspaceUsers,
} from "../api/users-api"
import { TaskImportDialog } from "./task-import-dialog"
import { TASK_IMPORT_JSON_SCHEMA_TEXT } from "./task-import-schema"

vi.mock("../api/task-api", async () => {
  const actual = await vi.importActual<typeof import("../api/task-api")>(
    "../api/task-api"
  )
  return {
    ...actual,
    importTasks: vi.fn(),
  }
})

vi.mock("../api/users-api", async () => {
  const actual = await vi.importActual<typeof import("../api/users-api")>(
    "../api/users-api"
  )
  return {
    ...actual,
    addWorkspaceMember: vi.fn(),
    createWorkspaceUser: vi.fn(),
    getWorkspaceMembers: vi.fn(),
    listWorkspaceUsers: vi.fn(),
  }
})

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={makeQueryClient()}>
      <EditFeedbackProvider>{children}</EditFeedbackProvider>
    </QueryClientProvider>
  )
}

function renderDialog(onOpenChange = vi.fn()) {
  render(
    <TaskImportDialog
      existingTasks={[{ uuid: "existing-task", title: "现有任务", status: "pending" }]}
      onOpenChange={onOpenChange}
      open
      projectSlug="adsops"
      workspaceSlug="acme"
    />,
    { wrapper: Wrapper }
  )
  return { onOpenChange }
}

describe("TaskImportDialog", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getWorkspaceMembers).mockResolvedValue([
      {
        user_id: "user-alice",
        name: "alice",
        email: "alice@example.com",
        role: "member",
        joined_at: 1,
        modified_at: 1,
      },
    ])
    vi.mocked(listWorkspaceUsers).mockResolvedValue([
      {
        id: "user-alice",
        name: "alice",
        display_name: "Alice Chen",
        email: "alice@example.com",
        external_ids: [],
        active: true,
        created_at: 1,
        modified_at: 1,
      },
      {
        id: "user-bob",
        name: "bob",
        display_name: "Bob Li",
        email: "bob@example.com",
        external_ids: [],
        active: false,
        created_at: 1,
        modified_at: 1,
      },
    ])
    vi.mocked(createWorkspaceUser).mockResolvedValue({
      id: "user-created",
      name: "new-person",
      display_name: "新同事",
      external_ids: [],
      active: false,
      created_at: 1,
      modified_at: 1,
    })
    vi.mocked(addWorkspaceMember).mockResolvedValue({ ok: true })
    vi.mocked(importTasks).mockResolvedValue({ imported: 1 })
  })

  it("blocks import when an assignee is not an existing workspace member", async () => {
    const user = userEvent.setup()
    renderDialog()
    const file = new File(
      [
        JSON.stringify([
          { title: "导入任务", assignees: ["missing-user"] },
        ]),
      ],
      "tasks.json",
      { type: "application/json" }
    )

    await user.upload(screen.getByLabelText("上传文件"), file)

    await screen.findByText("1 个阻断")
    expect(screen.getAllByText(/missing-user/).length).toBeGreaterThan(0)
    expect(
      (screen.getByRole("button", { name: "导入 1 个任务" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(importTasks).not.toHaveBeenCalled()
  })

  it("can add an existing non-member assignee from preflight", async () => {
    const user = userEvent.setup()
    renderDialog()
    const file = new File(
      [
        JSON.stringify([
          { title: "导入任务", assignees: ["bob@example.com"] },
        ]),
      ],
      "tasks.json",
      { type: "application/json" }
    )

    await user.upload(screen.getByLabelText("上传文件"), file)
    await screen.findByText("1 个阻断")
    await user.click(screen.getByRole("button", { name: "创建或加入 1 个指派人" }))

    await waitFor(() => {
      expect(addWorkspaceMember).toHaveBeenCalledWith("acme", "user-bob", "member")
      expect(createWorkspaceUser).not.toHaveBeenCalled()
    })
  })

  it("can create a missing assignee and add it to the workspace from preflight", async () => {
    const user = userEvent.setup()
    renderDialog()
    const file = new File(
      [
        JSON.stringify([
          { title: "导入任务", assignees: ["new-person@example.com"] },
        ]),
      ],
      "tasks.json",
      { type: "application/json" }
    )

    await user.upload(screen.getByLabelText("上传文件"), file)
    await screen.findByText("1 个阻断")
    await user.click(screen.getByRole("button", { name: "创建或加入 1 个指派人" }))

    await waitFor(() => {
      expect(createWorkspaceUser).toHaveBeenCalledWith({
        email: "new-person@example.com",
        name: "new-person",
      })
      expect(addWorkspaceMember).toHaveBeenCalledWith(
        "acme",
        "user-created",
        "member"
      )
    })
  })

  it("creates a missing assignee with display name from imported JSON", async () => {
    const user = userEvent.setup()
    renderDialog()
    const file = new File(
      [
        JSON.stringify([
          {
            title: "导入任务",
            assignees: [
              {
                email: "new-person@example.com",
                display_name: "新同事",
              },
            ],
          },
        ]),
      ],
      "tasks.json",
      { type: "application/json" }
    )

    await user.upload(screen.getByLabelText("上传文件"), file)
    await screen.findByText("1 个阻断")
    await user.click(screen.getByRole("button", { name: "创建或加入 1 个指派人" }))

    await waitFor(() => {
      expect(createWorkspaceUser).toHaveBeenCalledWith({
        email: "new-person@example.com",
        name: "new-person",
        display_name: "新同事",
      })
    })
  })

  it("imports valid JSON into the current project", async () => {
    const user = userEvent.setup()
    const { onOpenChange } = renderDialog()
    const file = new File(
      [
        JSON.stringify([
          {
            title: "导入任务",
            description: "Markdown\n\n- detail",
            assignees: ["alice"],
            blocked_by: ["existing-task"],
            project: "other",
          },
        ]),
      ],
      "tasks.json",
      { type: "application/json" }
    )

    await user.upload(screen.getByLabelText("上传文件"), file)
    await screen.findByText("0 个阻断")
    await user.click(screen.getByRole("button", { name: "导入 1 个任务" }))

    await waitFor(() => {
      expect(importTasks).toHaveBeenCalledWith("acme", "adsops", [
        expect.objectContaining({
          assignees: ["alice"],
          depends: ["existing-task"],
          description: "Markdown\n\n- detail",
          project: "adsops",
          title: "导入任务",
        }),
      ])
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  it("shows the complete JSON schema in a secondary dialog", async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole("button", { name: "查看 JSON Schema" }))

    expect(screen.getByRole("dialog", { name: "导入 JSON Schema" })).toBeTruthy()
    expect(screen.getByText(/"\$schema"/)).toBeTruthy()
    expect(screen.getByText(/"blocked_by"/)).toBeTruthy()
    expect(screen.getAllByText(/Markdown/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/批次内不重复的字符串/).length).toBeGreaterThan(0)
  })

  it("copies the complete JSON schema from the secondary dialog", async () => {
    const user = userEvent.setup()
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    })
    renderDialog()

    await user.click(screen.getByRole("button", { name: "查看 JSON Schema" }))
    await user.click(screen.getByRole("button", { name: "复制 JSON Schema" }))

    expect(writeText).toHaveBeenCalledWith(TASK_IMPORT_JSON_SCHEMA_TEXT)
    await screen.findByRole("button", { name: "已复制" })
  })

  it("shows a paginated normalized task preview after upload", async () => {
    const user = userEvent.setup()
    renderDialog()
    const rows: Array<Record<string, unknown>> = Array.from({ length: 12 }, (_, index) => ({
      id: `task-${index + 1}`,
      title: `导入任务 ${index + 1}`,
      description: `**详情 ${index + 1}**`,
      status: "pending",
      priority: index % 2 === 0 ? "H" : "M",
      assignees: ["alice"],
      blocked_by: index === 1 ? ["task-1"] : [],
    }))
    rows[0] = { ...rows[0], uuid: "task-a" }
    const file = new File([JSON.stringify(rows)], "many-tasks.json", {
      type: "application/json",
    })

    await user.upload(screen.getByLabelText("上传文件"), file)

    await screen.findByText("导入成果预览")
    expect(screen.getByRole("columnheader", { name: "标题" })).toBeTruthy()
    expect(screen.getByRole("columnheader", { name: "被阻塞于" })).toBeTruthy()
    expect(screen.getByRole("columnheader", { name: "描述" })).toBeTruthy()
    expect(screen.getByText("第 1-10 条，共 12 条")).toBeTruthy()
    expect(screen.getByText("导入任务 1")).toBeTruthy()
    expect(screen.queryByText("导入任务 12")).toBeNull()

    await user.click(screen.getByRole("button", { name: "下一页" }))

    expect(screen.getByText("第 11-12 条，共 12 条")).toBeTruthy()
    expect(screen.getByText("导入任务 12")).toBeTruthy()
  })

  it("submits the same normalized dependency refs shown in preview", async () => {
    const user = userEvent.setup()
    renderDialog()
    const file = new File(
      [
        JSON.stringify([
          { id: "local-a", uuid: "task-a", title: "前置任务" },
          { id: "local-b", title: "后续任务", blocked_by: ["local-a"] },
        ]),
      ],
      "dependencies.json",
      { type: "application/json" }
    )

    await user.upload(screen.getByLabelText("上传文件"), file)
    await screen.findByText("导入成果预览")
    expect(screen.getByText("task-a")).toBeTruthy()

    await user.click(screen.getByRole("button", { name: "导入 2 个任务" }))

    await waitFor(() => {
      expect(importTasks).toHaveBeenCalledWith("acme", "adsops", [
        expect.objectContaining({
          title: "前置任务",
        }),
        expect.objectContaining({
          depends: ["task-a"],
          title: "后续任务",
        }),
      ])
    })
  })
})
