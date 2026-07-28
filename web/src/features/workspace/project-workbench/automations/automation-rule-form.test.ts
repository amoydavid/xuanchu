import { describe, expect, it } from "vitest"

import { AUTOMATION_EVENT_OPTIONS } from "./automation-rule-form"

describe("AUTOMATION_EVENT_OPTIONS", () => {
  it("exposes every event supported by project automation", () => {
    expect(AUTOMATION_EVENT_OPTIONS.map((option) => option.value)).toEqual([
      "task.created",
      "task.modified",
      "task.completed",
      "task.deleted",
      "task.started",
      "task.stopped",
      "task.reopened",
      "task.assigned",
      "task.unassigned",
      "task.blocked",
      "task.due_changed",
      "task.priority_changed",
      "task.project_changed",
      "task.tags_changed",
      "task.unblocked",
      "task.user_mentioned",
      "project.archived",
      "project.transitioned",
      "project.annotated",
      "project.denotated",
    ])
  })
})
