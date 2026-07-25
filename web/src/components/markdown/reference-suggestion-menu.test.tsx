import { describe, expect, it, vi } from "vitest"

import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import React from "react"

import {
  ReferenceSuggestionMenu,
  resolutionToMenuItem,
  type ReferenceSuggestionMenuItem,
} from "./reference-suggestion-menu"

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "zh-CN" },
  }),
}))

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

// 捕获菜单注册给 suggestion 引擎的按键处理器，用于模拟方向键/Enter/Esc。
function captureKeyHandler() {
  const captured = { current: null as null | ((e: KeyboardEvent) => boolean) }
  const register = (handler: (e: KeyboardEvent) => boolean) => {
    captured.current = handler
  }
  return { captured, register }
}

function fireKey(handler: ((e: KeyboardEvent) => boolean) | null, key: string) {
  if (!handler) throw new Error("key handler not registered")
  const event = new KeyboardEvent("keydown", { key, bubbles: true })
  Object.defineProperty(event, "key", { get: () => key })
  return handler(event)
}

describe("ReferenceSuggestionMenu", () => {
  it("renders items passed via props and selects on click", async () => {
    const onSelect = vi.fn()
    const { register } = captureKeyHandler()
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        items={[makeItem("u1", "Alice"), makeItem("u2", "Bob")]}
        loading={false}
        error={null}
        onSelect={onSelect}
        onClose={() => {}}
        registerKeyHandler={register}
      />
    )
    const alice = screen.getByRole("option", { name: /Alice/ })
    await userEvent.click(alice)
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "u1" }))
  })

  it("shows loading placeholder while loading", () => {
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        items={[]}
        loading={true}
        error={null}
        onSelect={() => {}}
        onClose={() => {}}
      />
    )
    expect(screen.getByText("common.loading")).toBeTruthy()
  })

  it("shows noResults when not loading and empty items", () => {
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        items={[]}
        loading={false}
        error={null}
        onSelect={() => {}}
        onClose={() => {}}
      />
    )
    expect(screen.getByText("common.noResults")).toBeTruthy()
  })

  it("shows error text when error is set", () => {
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        items={[]}
        loading={false}
        error="boom"
        onSelect={() => {}}
        onClose={() => {}}
      />
    )
    expect(screen.getByText("boom")).toBeTruthy()
  })

  it("navigates with arrow keys and Enter via the registered key handler", () => {
    const onSelect = vi.fn()
    const { captured, register } = captureKeyHandler()
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        items={[makeItem("u1", "Alice"), makeItem("u2", "Bob")]}
        loading={false}
        error={null}
        onSelect={onSelect}
        onClose={() => {}}
        registerKeyHandler={register}
      />
    )
    expect(captured.current).toBeTruthy()
    fireKey(captured.current, "ArrowDown")
    fireKey(captured.current, "Enter")
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "u2" }))
  })

  it("closes on Escape via the registered key handler", () => {
    const onClose = vi.fn()
    const { captured, register } = captureKeyHandler()
    render(
      <ReferenceSuggestionMenu
        kind="user"
        query="a"
        items={[makeItem("u1", "Alice")]}
        loading={false}
        error={null}
        onSelect={() => {}}
        onClose={onClose}
        registerKeyHandler={register}
      />
    )
    expect(fireKey(captured.current, "Escape")).toBe(true)
    expect(onClose).toHaveBeenCalled()
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
