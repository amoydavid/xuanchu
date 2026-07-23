import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import {
  TaskCommonFields,
  type TaskCommonFieldValue,
} from "./task-common-fields"

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiGet: vi.fn(),
}))

vi.mock("../api/users-api", () => ({
  getWorkspaceMembers: vi.fn().mockResolvedValue([]),
}))

const emptyValue: TaskCommonFieldValue = {
  title: "",
  description: "",
  priority: "",
  assignees: [],
  tags: "",
  udas: {},
}

describe("TaskCommonFields UDA schema", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
  })

  it("只渲染 workspace 已定义的 UDA，并按枚举限制输入", async () => {
    vi.mocked(workspaceApiGet).mockResolvedValue([
      {
        name: "channel",
        type: "string",
        label: "渠道",
        values: ["search", "social"],
        default: "",
        source: "database",
        task_value_count: 0,
        active_series_value_count: 0,
      },
    ])
    const onChange = vi.fn()

    render(
      <TaskCommonFields
        onChange={onChange}
        value={emptyValue}
        workspaceSlug="local"
      />
    )

    expect(await screen.findByRole("button", { name: "添加字段" })).toBeTruthy()
    expect(screen.queryByLabelText("字段名")).toBeNull()

    await userEvent.click(screen.getByRole("button", { name: "添加字段" }))
    await userEvent.click(screen.getByRole("button", { name: /渠道/ }))

    await userEvent.click(await screen.findByRole("combobox", { name: "渠道" }))
    await userEvent.click(screen.getByRole("option", { name: "search" }))
    expect(onChange).toHaveBeenLastCalledWith({
      ...emptyValue,
      udas: { channel: "search" },
    })
  })

  it("workspace 没有 UDA 定义时不显示空的自定义字段区", async () => {
    vi.mocked(workspaceApiGet).mockResolvedValue([])

    render(
      <TaskCommonFields
        onChange={vi.fn()}
        value={emptyValue}
        workspaceSlug="local"
      />
    )

    await waitFor(() => expect(workspaceApiGet).toHaveBeenCalledOnce())
    expect(screen.queryByText("自定义字段")).toBeNull()
    expect(screen.queryByLabelText("字段名")).toBeNull()
  })
})
