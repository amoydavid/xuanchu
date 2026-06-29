import { describe, expect, it } from "vitest"

import {
  userModifyPath,
  userListPath,
  workspaceMemberAddPath,
  workspaceMembersPath,
} from "./users-api"

describe("project workbench users api paths", () => {
  it("builds encoded workspace member paths", () => {
    expect(workspaceMembersPath("local")).toBe("/api/v1/workspaces/local/members")
    expect(workspaceMembersPath("workspace 1")).toBe(
      "/api/v1/workspaces/workspace%201/members"
    )
    expect(workspaceMembersPath("ops/team")).toBe(
      "/api/v1/workspaces/ops%2Fteam/members"
    )
  })

  it("builds user and workspace member write paths", () => {
    expect(userListPath()).toBe("/api/v1/users")
    expect(userModifyPath("user 1")).toBe("/api/v1/users/user%201")
    expect(workspaceMemberAddPath("workspace 1")).toBe(
      "/api/v1/workspaces/workspace%201/members"
    )
  })
})
