import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { ConfigOverviewSection } from "./config-overview-section"
import type { ConfigEffectiveValue } from "./config-definition-api"

function row(overrides: Partial<ConfigEffectiveValue> = {}): ConfigEffectiveValue {
  return {
    key: "ads.roi",
    value: "1.8",
    source: "workspace",
    definition: {
      key: "ads.roi",
      value_type: "string",
      allowed_scopes: ["workspace"],
      label: "ROI 阈值",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: true,
      created_at: 0,
      modified_at: 0,
    },
    show_on_console_home: true,
    missing_required: false,
    ...overrides,
  }
}

describe("ConfigOverviewSection", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("shows empty hint when no rows", async () => {
    render(
      renderWithRouter(
        <ConfigOverviewSection rows={[]} isPending={false} isError={false} />
      )
    )
    await waitFor(() =>
      expect(screen.getByText("没有标记为首页展示的配置")).toBeTruthy()
    )
  })

  it("renders label as primary text, formatted value and source badge", async () => {
    render(
      renderWithRouter(
        <ConfigOverviewSection
          rows={[row()]}
          isPending={false}
          isError={false}
        />
      )
    )
    await waitFor(() => expect(screen.getByText("ROI 阈值")).toBeTruthy())
    expect(screen.getByText("ads.roi")).toBeTruthy()
    expect(screen.getByText("1.8")).toBeTruthy()
    expect(screen.getByText("工作区")).toBeTruthy()
  })

  it("formats date value locally", async () => {
    render(
      renderWithRouter(
        <ConfigOverviewSection
          rows={[
            row({
              key: "ads.launch",
              value: "2026-07-10",
              definition: {
                ...row().definition,
                key: "ads.launch",
                value_type: "date",
                label: "上线日期",
              },
            }),
          ]}
          isPending={false}
          isError={false}
        />
      )
    )
    await waitFor(() => expect(screen.getByText("上线日期")).toBeTruthy())
    expect(screen.getByText("2026-07-10")).toBeTruthy()
  })

  it("renders the go-to-definitions link", async () => {
    render(
      renderWithRouter(
        <ConfigOverviewSection rows={[]} isPending={false} isError={false} />
      )
    )
    await waitFor(() => expect(screen.getByText("去配置定义")).toBeTruthy())
  })
})
