import { describe, expect, it } from "vitest"

import { myTasksPath, type MyTasksFilter } from "./my-tasks-api"

describe("myTasksPath", () => {
  it("builds minimal path with assignee and limit", () => {
    const filter: MyTasksFilter = { assignee: "user-1", status: "pending" }
    expect(myTasksPath("dajee", filter)).toBe(
      "/api/v1/tasks?workspace=dajee&assignee=user-1&status=pending&limit=200"
    )
  })

  it("encodes project, priority, q and sort", () => {
    const filter: MyTasksFilter = {
      assignee: "user-1",
      project: "proj-a",
      priority: "H",
      q: "login bug",
      sort: "due",
    }
    expect(myTasksPath("dajee", filter)).toBe(
      "/api/v1/tasks?workspace=dajee&assignee=user-1&project=proj-a&priority=H&q=login+bug&sort=due&limit=200"
    )
  })

  it("encodes due range and due_empty", () => {
    const filter: MyTasksFilter = {
      assignee: "user-1",
      due_before: "1756752000",
      due_after: "1756665600",
      due_empty: "true",
    }
    expect(myTasksPath("dajee", filter)).toBe(
      "/api/v1/tasks?workspace=dajee&assignee=user-1&due_after=1756665600&due_before=1756752000&query=due.isnull&limit=200"
    )
  })

  it("omits empty values", () => {
    const filter: MyTasksFilter = {
      assignee: "user-1",
      project: "",
      priority: undefined,
      q: undefined,
    }
    expect(myTasksPath("dajee", filter)).toBe(
      "/api/v1/tasks?workspace=dajee&assignee=user-1&limit=200"
    )
  })

  it("appends assignee.isnull when assignee is empty string", () => {
    const filter: MyTasksFilter = { assignee: "", status: "pending" }
    expect(myTasksPath("dajee", filter)).toBe(
      "/api/v1/tasks?workspace=dajee&status=pending&query=assignee.isnull&limit=200"
    )
  })
})
