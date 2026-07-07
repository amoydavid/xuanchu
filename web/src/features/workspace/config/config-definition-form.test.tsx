import { render, screen } from "@testing-library/react"
import { userEvent } from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"

import { ConfigDefinitionForm } from "./config-definition-form"
import type { ConfigSchemaDefinition, ConfigSchemaUsage } from "./config-definition-api"

function baseDef(
  overrides: Partial<ConfigSchemaDefinition> = {}
): ConfigSchemaDefinition {
  return {
    key: "ads.budget",
    value_type: "number",
    allowed_scopes: ["workspace", "project"],
    label: "广告预算",
    description: "",
    enum_values: [],
    default_value: null,
    required: false,
    secret: false,
    show_on_console_home: false,
    created_at: 0,
    modified_at: 0,
    ...overrides,
  }
}

describe("ConfigDefinitionForm", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("disables key input in edit mode", () => {
    render(
      <ConfigDefinitionForm
        mode="edit"
        initial={baseDef()}
        defaultScopes={["workspace"]}
        canManage
        onSubmit={vi.fn()}
      />
    )
    const keyInput = screen.getByLabelText(/^Key$/) as HTMLInputElement
    expect(keyInput.disabled).toBe(true)
  })

  it("enables key input in create mode", () => {
    render(
      <ConfigDefinitionForm
        mode="create"
        defaultScopes={["workspace"]}
        canManage
        onSubmit={vi.fn()}
      />
    )
    const keyInput = screen.getByLabelText(/^Key$/) as HTMLInputElement
    expect(keyInput.disabled).toBe(false)
  })

  it("renders show_on_console_home checkbox", () => {
    render(
      <ConfigDefinitionForm
        mode="create"
        defaultScopes={["workspace"]}
        canManage
        onSubmit={vi.fn()}
      />
    )
    expect(screen.getByLabelText("显示在首页")).toBeTruthy()
  })

  it("disables value_type when usage total > 0 and shows lock hint", () => {
    const usage: ConfigSchemaUsage = {
      key: "ads.budget",
      workspace_values: 1,
      project_values: 0,
      total_values: 1,
    }
    render(
      <ConfigDefinitionForm
        mode="edit"
        initial={baseDef()}
        usage={usage}
        defaultScopes={["workspace"]}
        canManage
        onSubmit={vi.fn()}
      />
    )
    const typeTrigger = screen.getByLabelText(/^类型$/) as HTMLButtonElement
    // radix Select disabled 时 trigger button 带 disabled 属性
    expect(typeTrigger.disabled || typeTrigger.hasAttribute("data-disabled")).toBe(true)
    expect(screen.getByText("已有配置值，不能修改类型")).toBeTruthy()
  })

  it("requires at least one scope before submit", async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <ConfigDefinitionForm
        mode="create"
        defaultScopes={[]}
        canManage
        onSubmit={onSubmit}
      />
    )
    await user.type(screen.getByLabelText(/^Key$/), "ads.x")
    await user.click(screen.getByRole("button", { name: "保存" }))
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it("opens destructive delete dialog and requires key confirmation", async () => {
    const user = userEvent.setup()
    const onDelete = vi.fn()
    render(
      <ConfigDefinitionForm
        mode="edit"
        initial={baseDef()}
        defaultScopes={["workspace"]}
        canManage
        onSubmit={vi.fn()}
        onDelete={onDelete}
      />
    )
    await user.click(screen.getByRole("button", { name: "删除" }))
    expect(screen.getByText("删除配置定义")).toBeTruthy()
    const confirmBtn = screen.getByRole("button", {
      name: "删除定义和值",
    })
    expect(confirmBtn.hasAttribute("disabled")).toBe(true)
    await user.type(
      screen.getByPlaceholderText("请输入 key"),
      "ads.budget"
    )
    expect(confirmBtn.hasAttribute("disabled")).toBe(false)
  })
})
