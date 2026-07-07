import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { ProjectSettingsLayout, activeSettingsTab } from "./project-settings-layout"

describe("ProjectSettingsLayout", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders three tabs: config, definitions, notes", async () => {
    render(
      renderWithRouter(
        <ProjectSettingsLayout projectSlug="api" workspaceSlug="local" />
      )
    )
    await waitFor(() => expect(screen.getByText("配置项")).toBeTruthy())
    expect(screen.getByText("配置定义")).toBeTruthy()
    expect(screen.getByText("项目备注")).toBeTruthy()
  })
})

describe("activeSettingsTab", () => {
  it("returns notes for /notes suffix", () => {
    expect(
      activeSettingsTab("/projects/api/settings/notes")
    ).toBe("notes")
  })

  it("returns definitions for /definitions suffix", () => {
    expect(
      activeSettingsTab("/projects/api/settings/definitions")
    ).toBe("definitions")
  })

  it("defaults to config for other paths", () => {
    expect(
      activeSettingsTab("/projects/api/settings/config")
    ).toBe("config")
  })
})
