import { describe, expect, it, vi } from "vitest"

import type { ProjectWorkbenchTask } from "../api/project-api"
import type { WorkspaceMemberCandidate } from "../api/users-api"
import {
  buildTaskImportTemplateRows,
  parseTaskImportJSON,
  preflightTaskImport,
  rowsToTaskImportPayload,
} from "./task-import"

const NOW = "2026-06-28T00:00:00.000Z"

function member(
  overrides: Partial<WorkspaceMemberCandidate> = {}
): WorkspaceMemberCandidate {
  return {
    user_id: "user-alice",
    name: "alice",
    email: "alice@example.com",
    role: "member",
    joined_at: 1,
    modified_at: 1,
    ...overrides,
  }
}

function existingTask(
  overrides: Partial<ProjectWorkbenchTask> = {}
): ProjectWorkbenchTask {
  return {
    uuid: "existing-task",
    task_slug: "ads-1",
    title: "现有任务",
    status: "pending",
    project: "adsops",
    ...overrides,
  }
}

describe("task import preprocessing", () => {
  it("parses JSON arrays and wraps tasks with project defaults", () => {
    vi.spyOn(crypto, "randomUUID").mockReturnValue(
      "11111111-1111-4111-8111-111111111111"
    )
    const payload = parseTaskImportJSON(
      JSON.stringify({
        tasks: [
          {
            title: "补齐导入说明",
            description: "支持 Markdown\n\n- 依赖\n- 人员",
            tags: ["docs"],
            estimate: 3,
            project: "other",
          },
        ],
      }),
      { nowISO: NOW, projectSlug: "adsops" }
    )

    expect(payload.tasks).toMatchObject([
      {
        uuid: "11111111-1111-4111-8111-111111111111",
        title: "补齐导入说明",
        description: "支持 Markdown\n\n- 依赖\n- 人员",
        status: "pending",
        entry: NOW,
        modified: NOW,
        project: "adsops",
        tags: ["docs"],
        estimate: "3",
      },
    ])
    expect(payload.warnings).toContainEqual(
      expect.objectContaining({ code: "project_rebound" })
    )
  })

  it("turns spreadsheet rows into canonical task import payload", () => {
    vi.spyOn(crypto, "randomUUID").mockReturnValue(
      "22222222-2222-4222-8222-222222222222"
    )
    const payload = rowsToTaskImportPayload(
      [
        {
          id: "review",
          title: "创建投放复盘",
          description: "复盘内容\n\n- 素材\n- 预算",
          priority: "H",
          due: "2026-07-01",
          tags: "ops,review",
          assignees: "alice, bob@example.com",
          blocked_by: "existing-task",
          "uda.estimate": "3",
        },
      ],
      { nowISO: NOW, projectSlug: "adsops" }
    )

    expect(payload.tasks).toMatchObject([
      {
        uuid: "22222222-2222-4222-8222-222222222222",
        title: "创建投放复盘",
        description: "复盘内容\n\n- 素材\n- 预算",
        priority: "H",
        due: "2026-07-01T00:00:00.000Z",
        tags: ["ops", "review"],
        assignees: ["alice", "bob@example.com"],
        depends: ["existing-task"],
        project: "adsops",
        estimate: "3",
      },
    ])
  })

  it("maps import-local ids in blocked_by to generated task uuids", () => {
    vi.spyOn(crypto, "randomUUID")
      .mockReturnValueOnce("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
      .mockReturnValueOnce("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
    const payload = parseTaskImportJSON(
      JSON.stringify({
        tasks: [
          { id: "prepare", title: "准备素材" },
          { id: "launch", title: "配置投放计划", blocked_by: ["prepare"] },
        ],
      }),
      { nowISO: NOW, projectSlug: "adsops" }
    )

    expect(payload.tasks).toMatchObject([
      {
        uuid: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        title: "准备素材",
      },
      {
        uuid: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
        title: "配置投放计划",
        depends: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
      },
    ])
    expect(payload.tasks[0]).not.toHaveProperty("id")
    expect(payload.tasks[0]).not.toHaveProperty("import_id")
  })

  it("accepts any non-empty string as import-local id without UUID format checks", () => {
    vi.spyOn(crypto, "randomUUID")
      .mockReturnValueOnce("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
      .mockReturnValueOnce("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
    const payload = parseTaskImportJSON(
      JSON.stringify({
        tasks: [
          { id: "素材准备/第 1 步", title: "准备素材" },
          {
            id: "launch plan",
            title: "配置投放计划",
            blocked_by: ["素材准备/第 1 步"],
          },
        ],
      }),
      { nowISO: NOW, projectSlug: "adsops" }
    )

    expect(payload.tasks[1]).toMatchObject({
      depends: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
    })
    const result = preflightTaskImport(payload.tasks, {
      currentProjectSlug: "adsops",
      existingTasks: [],
      members: [],
    })
    expect(result.blockers).toEqual([])
  })

  it("preflights duplicate import-local ids and non-string JSON ids", () => {
    const payload = parseTaskImportJSON(
      JSON.stringify({
        tasks: [
          { id: "same", title: "准备素材" },
          { import_id: "same", title: "配置投放计划" },
          { id: 123, title: "数字 ID" },
        ],
      }),
      { nowISO: NOW, projectSlug: "adsops" }
    )

    const result = preflightTaskImport(payload.tasks, {
      currentProjectSlug: "adsops",
      existingTasks: [],
      members: [],
    })

    expect(result.blockers).toContainEqual(
      expect.objectContaining({
        code: "invalid_import_id",
        ref: "same",
      })
    )
    expect(result.blockers).toContainEqual(
      expect.objectContaining({
        code: "invalid_import_id",
        ref: "123",
      })
    )
  })

  it("preflights missing assignees and dependency references before import", () => {
    const payload = rowsToTaskImportPayload(
      [
        {
          uuid: "new-task",
          title: "新增任务",
          assignees: "alice, missing-user, feishu:ou_open",
          blocked_by: "existing-task, missing-task",
        },
      ],
      { nowISO: NOW, projectSlug: "adsops" }
    )

    const result = preflightTaskImport(payload.tasks, {
      currentProjectSlug: "adsops",
      existingTasks: [existingTask()],
      members: [member()],
    })

    expect(result.blockers).toContainEqual(
      expect.objectContaining({
        code: "assignee_not_member",
        ref: "missing-user",
      })
    )
    expect(result.blockers).toContainEqual(
      expect.objectContaining({
        code: "dependency_not_found",
        ref: "missing-task",
      })
    )
    expect(result.warnings).toContainEqual(
      expect.objectContaining({
        code: "external_assignee_unverified",
        ref: "feishu:ou_open",
      })
    )
  })

  it("allows dependencies that are included in the same import batch", () => {
    const payload = rowsToTaskImportPayload(
      [
        { id: "task-a", title: "前置任务" },
        { id: "task-b", title: "后置任务", blocked_by: "task-a" },
      ],
      { nowISO: NOW, projectSlug: "adsops" }
    )

    const result = preflightTaskImport(payload.tasks, {
      currentProjectSlug: "adsops",
      existingTasks: [],
      members: [],
    })

    expect(result.blockers).toEqual([])
  })

  it("requires blocked_by references to resolve to imported ids or existing UUIDs", () => {
    const payload = rowsToTaskImportPayload(
      [{ id: "new-task", title: "新增任务", blocked_by: "ads-1" }],
      { nowISO: NOW, projectSlug: "adsops" }
    )

    const result = preflightTaskImport(payload.tasks, {
      currentProjectSlug: "adsops",
      existingTasks: [existingTask({ task_slug: "ads-1" })],
      members: [],
    })

    expect(result.blockers).toContainEqual(
      expect.objectContaining({
        code: "dependency_not_found",
        ref: "ads-1",
      })
    )
  })

  it("builds template rows with core import columns and UDA examples", () => {
    expect(buildTaskImportTemplateRows()).toEqual([
      expect.objectContaining({
        title: "示例任务",
        description: "可使用 Markdown 记录详细说明",
        assignees: "alice, bob@example.com",
        blocked_by: "",
        "uda.estimate": "3",
      }),
    ])
  })
})
