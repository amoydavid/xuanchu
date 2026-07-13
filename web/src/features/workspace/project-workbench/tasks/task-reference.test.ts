import { describe, expect, it } from "vitest"

import type { ProjectWorkbenchTask } from "../api/project-api"
import {
  canonicalTaskRouteRef,
  taskDisplayRef,
  taskRouteRef,
} from "./task-reference"

function task(
  overrides: Partial<ProjectWorkbenchTask> = {}
): ProjectWorkbenchTask {
  return {
    id: "occ:series-1:1784390399",
    title: "每日巡检",
    status: "pending",
    project: "ops",
    recurrence_info: {
      role: "occurrence",
      series_id: "series-1",
      series_status: "active",
      rule: "daily",
      recurrence_at: 1_784_476_799,
      materialization: "projected",
    },
    ...overrides,
  }
}

describe("task reference helpers", () => {
  it("keeps projected display separate from its stable route ref", () => {
    const projected = task()
    expect(taskDisplayRef(projected, "zh-CN")).toBe("↻07-19")
    expect(taskRouteRef(projected)).toBe("occ:series-1:1784390399")
    expect(canonicalTaskRouteRef(projected)).toBeNull()
  })

  it("uses the readable slug for materialized occurrences", () => {
    const materialized = task({
      uuid: "materialized-uuid",
      task_slug: "ops-7",
      recurrence_info: {
        ...task().recurrence_info!,
        materialization: "materialized",
      },
    })
    expect(taskDisplayRef(materialized, "zh-CN")).toBe("ops-7")
    expect(taskRouteRef(materialized)).toBe("ops-7")
    expect(canonicalTaskRouteRef(materialized)).toBe("ops-7")
  })

  it("falls back to UUID for ordinary tasks", () => {
    const ordinary = task({
      id: "ordinary-uuid",
      uuid: "ordinary-uuid",
      recurrence_info: null,
    })
    expect(taskDisplayRef(ordinary, "zh-CN")).toBe("ordinary")
    expect(taskRouteRef(ordinary)).toBe("ordinary-uuid")
    expect(canonicalTaskRouteRef(ordinary)).toBe("ordinary-uuid")
  })
})
