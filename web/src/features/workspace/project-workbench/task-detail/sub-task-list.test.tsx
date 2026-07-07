import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import type { ProjectTask } from "../api/task-api"
import { createTask, listTaskChildren } from "../api/task-api"
import { SubTaskList } from "./sub-task-list"

// Mock task-api：createTask / listTaskChildren 用 vi.fn，其余保持实际实现。
vi.mock("../api/task-api", async () => {
  const actual = await vi.importActual<typeof import("../api/task-api")>(
    "../api/task-api"
  )
  return {
    ...actual,
    createTask: vi.fn(),
    listTaskChildren: vi.fn(),
  }
})

// Mock AssigneePicker / TagPicker：它们内部会发请求，测试中替换为最小可控组件。
vi.mock("./assignee-picker", () => ({
  AssigneePicker: ({ onSave }: { onSave: (ids: string[]) => void }) => (
    <button onClick={() => onSave(["u1"])} type="button">
      pick-assignee
    </button>
  ),
}))

vi.mock("./tag-picker", () => ({
  TagPicker: ({ onSave }: { onSave: (tags: string[]) => void }) => (
    <button onClick={() => onSave(["t1"])} type="button">
      pick-tag
    </button>
  ),
}))

const createTaskMock = createTask as unknown as ReturnType<typeof vi.fn>
const listTaskChildrenMock = listTaskChildren as unknown as ReturnType<
  typeof vi.fn
>

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
      {children}
    </QueryClientProvider>
  )
}

function child(overrides: Partial<ProjectTask> = {}): ProjectTask {
  return {
    uuid: "child-1",
    title: "已存在子任务",
    status: "pending",
    ...overrides,
  }
}

describe("SubTaskList", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
    createTaskMock.mockReset()
    listTaskChildrenMock.mockReset()
  })

  it("Enter 提交后清空标题并保持 composer 打开，payload 含 parent 与 project", async () => {
    listTaskChildrenMock.mockResolvedValue([])
    createTaskMock.mockResolvedValue({ uuid: "new-child" })

    render(
      <SubTaskList
        canCreate
        parentRef="parent-ref"
        parentUUID="parent-uuid"
        projectSlug="ads"
        workspaceSlug="acme"
      />,
      { wrapper: Wrapper }
    )

    const input = await screen.findByPlaceholderText("子任务标题")
    await userEvent.type(input, "新子任务")
    await userEvent.keyboard("{Enter}")

    await waitFor(() => {
      expect(createTaskMock).toHaveBeenCalledTimes(1)
    })
    // payload 含 parent 与 project。
    expect(createTaskMock).toHaveBeenCalledWith(
      "acme",
      expect.objectContaining({
        title: "新子任务",
        parent: "parent-uuid",
        project: "ads",
      })
    )
    // composer 保持打开，标题已清空。
    const titleInput = screen.getByPlaceholderText("子任务标题") as HTMLInputElement
    expect(titleInput.value).toBe("")
  })

  it("空标题提交时不出请求，显示校验错误", async () => {
    listTaskChildrenMock.mockResolvedValue([])
    render(
      <SubTaskList
        canCreate
        parentRef="parent-ref"
        parentUUID="parent-uuid"
        projectSlug="ads"
        workspaceSlug="acme"
      />,
      { wrapper: Wrapper }
    )

    await screen.findByPlaceholderText("子任务标题")
    await userEvent.keyboard("{Enter}")
    expect(createTaskMock).not.toHaveBeenCalled()
    expect(
      (await screen.findAllByText("子任务标题不能为空")).length
    ).toBeGreaterThan(0)
  })

  it("不可创建时显示引导空态文案且不渲染 composer", async () => {
    listTaskChildrenMock.mockResolvedValue([])
    render(
      <SubTaskList
        canCreate={false}
        parentRef="parent-ref"
        parentUUID="parent-uuid"
        projectSlug="ads"
        workspaceSlug="acme"
      />,
      { wrapper: Wrapper }
    )
    expect((await screen.findAllByText("暂无子任务")).length).toBeGreaterThan(0)
    expect(screen.queryByPlaceholderText("子任务标题")).toBeNull()
  })

  it("渲染已存在子任务列表", async () => {
    listTaskChildrenMock.mockResolvedValue([child()])
    render(
      <SubTaskList
        canCreate
        parentRef="parent-ref"
        parentUUID="parent-uuid"
        projectSlug="ads"
        workspaceSlug="acme"
      />,
      { wrapper: Wrapper }
    )
    expect((await screen.findAllByText("已存在子任务")).length).toBeGreaterThan(0)
  })
})
