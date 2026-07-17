import { beforeEach, describe, expect, it, vi } from "vitest"

import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { getHome, type HomeView } from "./home-api"

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiGet: vi.fn(),
}))

describe("home api", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("loads the current actor's home summary", async () => {
    const response: HomeView = {
      generated_at: 1_784_246_400,
      today: "2026-07-17",
      actor_type: "user",
      my_work: {
        open_count: 1,
        started_count: 1,
        overdue_count: 0,
        due_today_count: 1,
        high_priority_open_count: 1,
        items: [
          {
            task: {
              id: "task-1",
              uuid: "task-1",
              task_slug: "OPS-7",
              title: "确认上线清单",
              status: "pending",
              project: "ops",
              assignees: [{ id: "user-1", name: "alice" }],
            },
            reasons: ["started", "due_today", "high_priority"],
          },
        ],
      },
      project_attention: [
        {
          project: {
            id: "project-1",
            workspace_id: "workspace-1",
            slug: "ops",
            name: "上线准备",
            status: "active",
            task_count: 4,
            pending_count: 2,
            completed_count: 2,
            created_at: 1,
            modified_at: 2,
          },
          overdue_count: 1,
          high_priority_open_count: 1,
          wait_ready_count: 0,
          unassigned_open_count: 0,
          series_metrics: {
            recurring_series_count: 1,
            active_recurring_series_count: 1,
            open_recurring_occurrence_count: 1,
            overdue_recurring_occurrence_count: 1,
          },
          latest_update: {
            id: "annotation-1",
            project_id: "project-1",
            entry: 2,
            content: "已确认发布窗口",
            created_by: {
              type: "user",
              user: { id: "user-2", name: "bob", display_name: "李四" },
            },
            created_at: 2,
          },
        },
      ],
    }
    vi.mocked(workspaceApiGet).mockResolvedValue(response)

    await expect(getHome()).resolves.toBe(response)
    expect(workspaceApiGet).toHaveBeenCalledWith("/api/v1/home")
  })

  it("supports a system actor without personal work", () => {
    const response: HomeView = {
      generated_at: 1,
      today: "2026-07-17",
      actor_type: "tenant_access_token",
      my_work: null,
      project_attention: [],
    }
    expect(response.my_work).toBeNull()
  })
})
