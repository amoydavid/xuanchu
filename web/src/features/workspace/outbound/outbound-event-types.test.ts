import { describe, expect, it } from "vitest"

import {
  ALL_HOOK_EVENT_TYPES,
  HOOK_EVENT_GROUPS,
  TASK_BASIC_EVENTS,
  TASK_FIELDS_EVENTS,
  TASK_STATUS_EVENTS,
  PROJECT_EVENTS,
  isValidHookEventType,
} from "./outbound-event-types"

describe("outbound event types", () => {
  it("matches backend whitelist", () => {
    expect(ALL_HOOK_EVENT_TYPES).toEqual([
      "task.created",
      "task.modified",
      "task.completed",
      "task.deleted",
      "task.started",
      "task.stopped",
      "task.blocked",
      "task.unblocked",
      "task.assigned",
      "task.unassigned",
      "task.due_changed",
      "task.priority_changed",
      "task.project_changed",
      "task.tags_changed",
      "project.archived",
      "project.transitioned",
      "project.annotated",
      "project.denotated",
    ])
  })

  it("does not contain task.done", () => {
    for (const e of ALL_HOOK_EVENT_TYPES) {
      expect(e).not.toBe("task.done")
    }
  })

  it("groups cover all events without duplicates", () => {
    const allInGroups = HOOK_EVENT_GROUPS.flatMap((g) => g.events)
    expect(new Set(allInGroups).size).toBe(allInGroups.length)
    expect(allInGroups.length).toBe(ALL_HOOK_EVENT_TYPES.length)
    for (const e of ALL_HOOK_EVENT_TYPES) {
      expect(allInGroups).toContain(e)
    }
  })

  it("task.completed is in basic group", () => {
    expect(TASK_BASIC_EVENTS).toContain("task.completed")
  })

  it("isValidHookEventType accepts whitelisted events", () => {
    expect(isValidHookEventType("task.completed")).toBe(true)
    expect(isValidHookEventType("project.archived")).toBe(true)
    expect(isValidHookEventType("task.done")).toBe(false)
  })

  it("status/fields/project groups are stable", () => {
    expect(TASK_STATUS_EVENTS).toContain("task.started")
    expect(TASK_FIELDS_EVENTS).toContain("task.due_changed")
    expect(PROJECT_EVENTS).toContain("project.denotated")
  })
})
