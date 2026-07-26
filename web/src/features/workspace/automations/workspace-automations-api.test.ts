import { describe, expect, it } from "vitest"

import { WORKSPACE_AUTOMATION_EVENTS } from "@/features/workspace/automations/workspace-automations-api"

describe("WORKSPACE_AUTOMATION_EVENTS", () => {
  it("only allows project.created in v1", () => {
    expect(WORKSPACE_AUTOMATION_EVENTS).toHaveLength(1)
    expect(WORKSPACE_AUTOMATION_EVENTS[0].value).toBe("project.created")
  })
})
