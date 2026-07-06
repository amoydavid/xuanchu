import { render, screen, fireEvent } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import "@/i18n"
import { ProjectNotesTimeline } from "./project-notes-timeline"

describe("ProjectNotesTimeline", () => {
  it("空数组渲染空状态", () => {
    render(
      <ProjectNotesTimeline
        entries={[]}
        canManage={false}
        onDelete={() => {}}
        emptyLabel="暂无备注"
      />
    )
    expect(screen.getByText("暂无备注")).toBeDefined()
  })

  it("按时间倒序渲染节点", () => {
    const entries = [
      {
        id: "a",
        project_id: "p",
        entry: 1000,
        content: "第一条",
        created_by: { id: "u1", name: "alice" },
        created_at: 1000,
      },
      {
        id: "b",
        project_id: "p",
        entry: 2000,
        content: "第二条",
        created_by: { id: "u2", name: "bob" },
        created_at: 2000,
      },
    ]
    const { container } = render(
      <ProjectNotesTimeline
        entries={entries}
        canManage={false}
        onDelete={() => {}}
        emptyLabel="暂无备注"
      />
    )
    const nodes = container.querySelectorAll('[role="listitem"]')
    expect(nodes.length).toBe(2)
    expect(nodes[0].textContent ?? "").toContain("第二条")
    expect(nodes[1].textContent ?? "").toContain("第一条")
  })

  it("canManage 时展示删除按钮并触发回调", () => {
    const onDelete = vi.fn()
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true)
    render(
      <ProjectNotesTimeline
        entries={[
          {
            id: "a",
            project_id: "p",
            entry: 1000,
            content: "hello",
            created_by: { id: "u1", name: "alice" },
            created_at: 1000,
          },
        ]}
        canManage={true}
        onDelete={onDelete}
        emptyLabel="暂无备注"
        deleteLabel="删除"
        deleteConfirm="确认删除该备注？"
      />
    )
    fireEvent.click(screen.getByText("删除"))
    expect(confirmSpy).toHaveBeenCalled()
    expect(onDelete).toHaveBeenCalledWith("a")
    confirmSpy.mockRestore()
  })
})
