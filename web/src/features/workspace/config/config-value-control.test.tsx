import { render, screen } from "@testing-library/react"
import { userEvent } from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import "@/i18n"

import { ConfigValueControl } from "./config-value-control"
import type { ConfigValueType } from "./config-definition-api"

function def(
  overrides: Partial<{
    value_type: ConfigValueType
    enum_values: string[]
    secret: boolean
  }> = {}
) {
  return {
    value_type: "string" as ConfigValueType,
    enum_values: [] as string[],
    secret: false,
    ...overrides,
  }
}

describe("ConfigValueControl", () => {
  it("renders a select when enum_values is non-empty", () => {
    render(
      <ConfigValueControl
        definition={def({ enum_values: ["auto", "manual"] })}
        value="auto"
        onChange={() => {}}
      />
    )
    // radix Select trigger 显示当前值
    expect(screen.getByText("auto")).toBeTruthy()
  })

  it("renders a number input for number type", () => {
    render(
      <ConfigValueControl
        definition={def({ value_type: "number" })}
        value="100"
        onChange={() => {}}
      />
    )
    const input = screen.getByDisplayValue("100") as HTMLInputElement
    expect(input.type).toBe("number")
  })

  it("renders a switch for boolean type and toggles output true/false", async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <ConfigValueControl
        definition={def({ value_type: "boolean" })}
        value="false"
        onChange={onChange}
      />
    )
    const sw = screen.getByRole("switch")
    expect(sw.getAttribute("data-state")).toBe("unchecked")
    await user.click(sw)
    expect(onChange).toHaveBeenCalledWith("true")
  })

  it("keeps an explicit unselected state for boolean when allowEmpty is enabled", () => {
    render(
      <ConfigValueControl
        allowEmpty
        definition={def({ value_type: "boolean" })}
        value=""
        onChange={() => {}}
      />
    )
    expect(screen.getByRole("combobox")).toBeTruthy()
    expect(screen.queryByRole("switch")).toBeNull()
  })

  it("renders a textarea for json type and shows parse error for invalid json", () => {
    const onChange = vi.fn()
    const { rerender } = render(
      <ConfigValueControl
        definition={def({ value_type: "json" })}
        value='{"a":1}'
        onChange={onChange}
      />
    )
    const textarea = screen.getByRole("textbox") as HTMLTextAreaElement
    expect(textarea.value).toBe('{"a":1}')
    // 合法 JSON 不显示错误
    expect(screen.queryByText(/json/i)).toBeNull()

    // 非法 JSON 触发错误提示
    rerender(
      <ConfigValueControl
        definition={def({ value_type: "json" })}
        value="not json"
        onChange={onChange}
      />
    )
    // 错误提示出现在带 text-destructive 的 <p> 中
    const errorNode = document.querySelector("p.text-destructive")
    expect(errorNode?.textContent ?? "").toMatch(/json/i)
  })

  it("renders a password input for secret type", () => {
    render(
      <ConfigValueControl
        definition={def({ secret: true })}
        value="topsecret"
        onChange={() => {}}
      />
    )
    const input = screen.getByDisplayValue("topsecret") as HTMLInputElement
    expect(input.type).toBe("password")
  })

  it("renders a reveal toggle for secret and exposes plaintext on click", async () => {
    const user = userEvent.setup()
    render(
      <ConfigValueControl
        definition={def({ secret: true })}
        value="topsecret"
        onChange={() => {}}
      />
    )
    const reveal = screen.getByRole("button", { name: /reveal|显示/i })
    await user.click(reveal)
    const input = screen.getByDisplayValue("topsecret") as HTMLInputElement
    expect(input.type).toBe("text")
  })

  it("can forbid revealing a secret for one-time template input", () => {
    render(
      <ConfigValueControl
        allowSecretReveal={false}
        definition={def({ secret: true })}
        value="topsecret"
        onChange={() => {}}
      />
    )
    expect((screen.getByDisplayValue("topsecret") as HTMLInputElement).type).toBe("password")
    expect(screen.queryByRole("button", { name: /reveal|显示/i })).toBeNull()
  })

  it("renders a date trigger showing the current date value", () => {
    render(
      <ConfigValueControl
        definition={def({ value_type: "date" })}
        value="2026-07-07"
        onChange={() => {}}
      />
    )
    // date 控件 trigger 显示当前日期
    expect(screen.getByText("2026-07-07")).toBeTruthy()
  })

  it("renders a time input for datetime type", () => {
    render(
      <ConfigValueControl
        definition={def({ value_type: "datetime" })}
        value="2026-07-07T12:00:00Z"
        onChange={() => {}}
      />
    )
    // datetime 控件含一个 time input
    const timeInput = document.querySelector('input[type="time"]') as HTMLInputElement | null
    expect(timeInput).not.toBeNull()
  })

  it("renders a placeholder for empty date value", () => {
    render(
      <ConfigValueControl
        definition={def({ value_type: "date" })}
        value=""
        onChange={() => {}}
      />
    )
    // 空值显示占位按钮（用 aria-label 定位）
    expect(screen.getByRole("button", { name: /选择日期|pick date/i })).toBeTruthy()
  })
})
