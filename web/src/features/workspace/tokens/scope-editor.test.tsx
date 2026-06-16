import { render, screen, fireEvent } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"

import { ScopeEditor } from "./scope-editor"

function renderEditor(props: {
  value: string[]
  onChange: (scopes: string[]) => void
  canImpersonate?: boolean
}) {
  return render(<ScopeEditor {...props} />)
}

describe("ScopeEditor", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders all resource groups", () => {
    renderEditor({ value: [], onChange: () => {} })
    // 每组都有「全选」按钮，10 组
    expect(screen.getAllByText("全选").length).toBe(10)
  })

  it("hides impersonate when canImpersonate is false", () => {
    renderEditor({ value: [], onChange: () => {}, canImpersonate: false })
    expect(screen.queryByText("impersonate")).toBeNull()
  })

  it("shows impersonate when canImpersonate is true", () => {
    renderEditor({ value: [], onChange: () => {}, canImpersonate: true })
    expect(screen.getByText("impersonate")).toBeTruthy()
  })

  it("clicking 全选 adds all scopes in a group via onChange", () => {
    const onChange = vi.fn()
    renderEditor({ value: [], onChange })
    // 点击第一个「全选」按钮（task 组）
    fireEvent.click(screen.getAllByText("全选")[0])
    expect(onChange).toHaveBeenCalledWith(["task:read", "task:write"])
  })

  it("clicking 清空 removes all scopes in a group via onChange", () => {
    const onChange = vi.fn()
    renderEditor({ value: ["task:read", "task:write"], onChange })
    // task 组全选后按钮文案变为「清空」
    fireEvent.click(screen.getAllByText("清空")[0])
    expect(onChange).toHaveBeenCalledWith([])
  })

  it("renders unknown legacy scopes with remove buttons", () => {
    const onChange = vi.fn()
    renderEditor({ value: ["task:read", "user:read"], onChange })
    // user:read 不在任何已知分组，应显示在「未识别」区
    expect(screen.getByText("user:read")).toBeTruthy()
    // 点击移除按钮（✕）
    const removeBtn = screen.getByTitle("移除")
    fireEvent.click(removeBtn)
    // 应移除 user:read，保留 task:read
    expect(onChange).toHaveBeenCalledWith(["task:read"])
  })
})
