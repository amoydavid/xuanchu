import { afterEach, describe, expect, it } from "vitest"

import {
  clearWorkspaceToken,
  setWorkspaceToken,
} from "@/features/workspace/session/workspace-token"

import { clearAdminToken, getAdminToken, setAdminToken } from "./admin-token"

describe("admin token storage", () => {
  afterEach(() => {
    sessionStorage.clear()
  })

  it("uses a separate session storage key", () => {
    setWorkspaceToken("xuanchu_pat_test")
    setAdminToken("xuanchu_admin_test")

    expect(getAdminToken()).toBe("xuanchu_admin_test")
    expect(sessionStorage.getItem("xuanchu.console.token")).toBe(
      "xuanchu_pat_test"
    )

    clearAdminToken()
    expect(sessionStorage.getItem("xuanchu.console.token")).toBe(
      "xuanchu_pat_test"
    )

    setAdminToken("xuanchu_admin_test")
    clearWorkspaceToken()
    expect(getAdminToken()).toBe("xuanchu_admin_test")
  })
})
