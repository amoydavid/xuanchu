import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"
import {
  deleteWorkspaceCustomField,
  listWorkspaceCustomFields,
  setWorkspaceCustomField,
} from "./custom-field-api"

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiDelete: vi.fn(),
  workspaceApiGet: vi.fn(),
  workspaceApiPut: vi.fn(),
}))

describe("custom field API", () => {
  beforeEach(() => vi.clearAllMocks())

  it("uses the typed Workspace resource and encodes workspace/name", async () => {
    vi.mocked(workspaceApiGet).mockResolvedValue([])
    await listWorkspaceCustomFields("my workspace")
    expect(workspaceApiGet).toHaveBeenCalledWith(
      "/api/v1/udas?workspace=my%20workspace"
    )

    const input = {
      type: "numeric" as const,
      label: "工作量",
      values: ["1", "2"],
      default: "2",
    }
    vi.mocked(workspaceApiPut).mockResolvedValue({} as never)
    await setWorkspaceCustomField("acme", "work.load", input)
    expect(workspaceApiPut).toHaveBeenCalledWith(
      "/api/v1/udas/work.load?workspace=acme",
      input
    )

    vi.mocked(workspaceApiDelete).mockResolvedValue(undefined)
    await deleteWorkspaceCustomField("acme", "work.load")
    expect(workspaceApiDelete).toHaveBeenCalledWith(
      "/api/v1/udas/work.load?workspace=acme"
    )
  })
})
