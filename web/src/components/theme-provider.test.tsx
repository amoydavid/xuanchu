import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { ThemeProvider, useTheme } from "./theme-provider"

function Probe() {
  const { setTheme, theme } = useTheme()
  return (
    <button type="button" onClick={() => setTheme("dark")}>
      {theme}
    </button>
  )
}

describe("ThemeProvider", () => {
  it("defaults to system and persists explicit dark mode", async () => {
    localStorage.clear()
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>
    )
    expect(screen.getByRole("button", { name: "system" })).toBeTruthy()
    await userEvent.click(screen.getByRole("button"))
    expect(localStorage.getItem("xuanchu.console.theme")).toBe("dark")
    expect(document.documentElement.classList.contains("dark")).toBe(true)
  })
})
