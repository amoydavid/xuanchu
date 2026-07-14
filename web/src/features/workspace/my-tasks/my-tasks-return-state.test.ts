import { describe, expect, it, vi } from "vitest"

import {
  saveMyTasksReturnState,
  restoreMyTasksReturnState,
  takeMyTasksReturnState,
} from "./my-tasks-return-state"

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => values.delete(key),
    setItem: (key, value) => values.set(key, value),
  }
}

describe("My Tasks return state", () => {
  it("round-trips scroll, focus and selection once for the exact query", () => {
    const storage = memoryStorage()
    const search = "tab=overdue&project=ops&task_type=occurrence"

    saveMyTasksReturnState(
      search,
      { focusId: "occ:series-1:100", scrollTop: 480, selectedIds: ["ops-7"] },
      storage
    )

    expect(takeMyTasksReturnState(search, storage)).toEqual({
      focusId: "occ:series-1:100",
      scrollTop: 480,
      selectedIds: ["ops-7"],
    })
    expect(takeMyTasksReturnState(search, storage)).toBeUndefined()
  })

  it("rejects malformed persisted state without throwing", () => {
    const storage = memoryStorage()
    storage.setItem("xuanchu:my-tasks:return:tab=today", "not-json")
    expect(takeMyTasksReturnState("tab=today", storage)).toBeUndefined()
  })

  it("restores focus to the matching row and then restores scroll", () => {
    const container = document.createElement("section")
    container.tabIndex = -1
    const first = document.createElement("a")
    first.dataset.myTaskFocus = "ops-1"
    first.tabIndex = 0
    const second = document.createElement("a")
    second.dataset.myTaskFocus = "ops-2"
    second.tabIndex = 0
    container.append(first, second)
    document.body.append(container)
    const scrollTo = vi.fn()

    restoreMyTasksReturnState(
      container,
      { focusId: "ops-2", scrollTop: 720, selectedIds: [] },
      scrollTo
    )

    expect(document.activeElement).toBe(second)
    expect(scrollTo).toHaveBeenCalledWith(0, 720)
    container.remove()
  })

  it("focuses the list container when the previous row disappeared", () => {
    const container = document.createElement("section")
    container.tabIndex = -1
    document.body.append(container)

    restoreMyTasksReturnState(
      container,
      { focusId: "missing", scrollTop: 10, selectedIds: [] },
      vi.fn()
    )

    expect(document.activeElement).toBe(container)
    container.remove()
  })
})
