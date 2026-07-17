import { describe, expect, it, vi } from "vitest"

import {
  restoreHomeReturnState,
  saveHomeReturnState,
  takeHomeReturnState,
} from "./home-return-state"

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

describe("Home return state", () => {
  it("round-trips scroll and focus once for the exact workspace and actor", () => {
    const storage = memoryStorage()

    saveHomeReturnState(
      "acme",
      "user-1",
      { focusId: "ops-7", scrollTop: 480 },
      storage
    )

    expect(takeHomeReturnState("acme", "user-2", storage)).toBeUndefined()
    expect(takeHomeReturnState("other", "user-1", storage)).toBeUndefined()
    expect(takeHomeReturnState("acme", "user-1", storage)).toEqual({
      focusId: "ops-7",
      scrollTop: 480,
    })
    expect(takeHomeReturnState("acme", "user-1", storage)).toBeUndefined()
  })

  it("restores focus to the matching home task and then restores scroll", () => {
    const container = document.createElement("section")
    container.tabIndex = -1
    const taskLink = document.createElement("a")
    taskLink.dataset.homeTaskFocus = "ops-7"
    taskLink.tabIndex = 0
    container.append(taskLink)
    document.body.append(container)
    const scrollTo = vi.fn()

    restoreHomeReturnState(
      container,
      { focusId: "ops-7", scrollTop: 720 },
      scrollTo
    )

    expect(document.activeElement).toBe(taskLink)
    expect(scrollTo).toHaveBeenCalledWith(0, 720)
    container.remove()
  })
})
