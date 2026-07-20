import { afterEach, describe, expect, it, vi } from "vitest"

import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import React from "react"

import {
  ReferenceSuggestionMenu,
  resolutionToMenuItem,
  type FetchSuggestions,
  type ReferenceSuggestionMenuItem,
} from "./reference-suggestion-menu"

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "zh-CN" },
  }),
}))

afterEach(() => {
  vi.restoreAllMocks()
})

function makeItem(id: string, label: string): ReferenceSuggestionMenuItem {
  return {
    id,
    kind: "user",
    label,
    description: id,
    resolution: {
      type: "user",
      id,
      status: "resolved",
      user: { id, name: id, display_name: label },
    },
  }
}

describe("ReferenceSuggestionMenu", () => {
  it("does not fetch on empty query", async () => {
    const fetchSpy = vi.fn(async () => [makeItem("u1", "Alice")]) as FetchSuggestions
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query=""
        fetchSuggestions={fetchSpy}
        onSelect={() => {}}
        onClose={() => {}}
      />
    )
    await waitFor(() => {
      expect(screen.getByText("task.mentions.userPlaceholder")).toBeTruthy()
    })
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it("debounces 150ms before fetching", async () => {
    const fetchSpy = vi.fn(async () => [makeItem("u1", "Alice")]) as FetchSuggestions
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="ali"
        fetchSuggestions={fetchSpy}
        onSelect={() => {}}
        onClose={() => {}}
      />
    )
    expect(fetchSpy).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalled()
    })
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })

  it("renders resolved items and selects on click", async () => {
    const onSelect = vi.fn()
    const fetchSpy = vi.fn(async () => [
      makeItem("u1", "Alice"),
      makeItem("u2", "Bob"),
    ]) as FetchSuggestions
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        fetchSuggestions={fetchSpy}
        onSelect={onSelect}
        onClose={() => {}}
      />
    )
    const alice = await screen.findByRole("option", { name: /Alice/ })
    fireEvent.click(alice)
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "u1" }))
  })

  it("navigates with arrow keys and Enter", async () => {
    const onSelect = vi.fn()
    const fetchSpy = vi.fn(async () => [
      makeItem("u1", "Alice"),
      makeItem("u2", "Bob"),
    ]) as FetchSuggestions
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        fetchSuggestions={fetchSpy}
        onSelect={onSelect}
        onClose={() => {}}
      />
    )
    const listbox = await screen.findByRole("listbox")
    await waitFor(() => {
      expect(screen.getByRole("option", { name: /Alice/ })).toBeTruthy()
    })
    fireEvent.keyDown(listbox, { key: "ArrowDown" })
    fireEvent.keyDown(listbox, { key: "Enter" })
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "u2" }))
  })

  it("closes on Escape", async () => {
    const onClose = vi.fn()
    const fetchSpy = vi.fn(async () => [makeItem("u1", "Alice")]) as FetchSuggestions
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        fetchSuggestions={fetchSpy}
        onSelect={() => {}}
        onClose={onClose}
      />
    )
    const listbox = await screen.findByRole("listbox")
    fireEvent.keyDown(listbox, { key: "Escape" })
    expect(onClose).toHaveBeenCalled()
  })

  it("aborts stale request when query changes", async () => {
    let aborted = 0
    const fetchSpy = vi.fn(({ signal }: { signal: AbortSignal }) => {
      return new Promise<ReferenceSuggestionMenuItem[]>((resolve, reject) => {
        if (signal.aborted) {
          aborted += 1
          reject(new DOMException("aborted", "AbortError"))
          return
        }
        signal.addEventListener("abort", () => {
          aborted += 1
          reject(new DOMException("aborted", "AbortError"))
        })
        // 永不自然 resolve，靠 abort 触发。
        setTimeout(() => resolve([makeItem("u1", "Alice")]), 1000)
      })
    }) as FetchSuggestions
    const { rerender } = render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        fetchSuggestions={fetchSpy}
        onSelect={() => {}}
        onClose={() => {}}
      />
    )
    await waitFor(() => expect(fetchSpy).toHaveBeenCalled())
    rerender(
      <ReferenceSuggestionMenu
        kind="user"
        query="ab"
        fetchSuggestions={fetchSpy}
        onSelect={() => {}}
        onClose={() => {}}
      />
    )
    await waitFor(() => expect(aborted).toBeGreaterThanOrEqual(1))
  })

  it("aborts an in-flight request when the menu unmounts", async () => {
    let aborted = 0
    const fetchSpy = vi.fn(({ signal }: { signal: AbortSignal }) => {
      return new Promise<ReferenceSuggestionMenuItem[]>((_resolve, reject) => {
        signal.addEventListener("abort", () => {
          aborted += 1
          reject(new DOMException("aborted", "AbortError"))
        })
      })
    }) as FetchSuggestions
    const { unmount } = render(
      <ReferenceSuggestionMenu
        kind="user"
        query="alice"
        fetchSuggestions={fetchSpy}
        onSelect={() => {}}
        onClose={() => {}}
      />
    )
    await waitFor(() => expect(fetchSpy).toHaveBeenCalledOnce())
    unmount()
    await waitFor(() => expect(aborted).toBe(1))
  })
})

describe("resolutionToMenuItem", () => {
  it("converts resolved user", () => {
    const item = resolutionToMenuItem({
      type: "user",
      id: "u1",
      status: "resolved",
      user: { id: "u1", name: "alice", display_name: "Alice" },
    })
    expect(item?.label).toBe("Alice")
  })

  it("returns null for unavailable", () => {
    expect(
      resolutionToMenuItem({ type: "user", id: "x", status: "unavailable" })
    ).toBeNull()
  })
})
