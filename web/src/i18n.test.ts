import { describe, expect, it } from "vitest"

import { i18n, normalizeLanguage } from "./i18n"
import { enUS } from "./locales/en-US"
import { zhCN } from "./locales/zh-CN"

describe("i18n", () => {
  it("normalizes supported languages with zh-CN fallback", () => {
    expect(normalizeLanguage("zh")).toBe("zh-CN")
    expect(normalizeLanguage("en-US")).toBe("en-US")
    expect(normalizeLanguage("fr-FR")).toBe("zh-CN")
  })

  it("contains Chinese and English error messages", () => {
    i18n.changeLanguage("zh-CN")
    expect(i18n.t("api.error.token_scope_denied")).toContain("Token")
    i18n.changeLanguage("en-US")
    expect(i18n.t("api.error.token_scope_denied")).toContain("scope")
  })

  it("keeps Chinese and English locale keys aligned", () => {
    expect(flattenKeys(zhCN)).toEqual(flattenKeys(enUS))
  })
})

function flattenKeys(value: unknown, prefix = ""): string[] {
  if (!value || typeof value !== "object") {
    return [prefix]
  }
  return Object.entries(value)
    .flatMap(([key, child]) =>
      flattenKeys(child, prefix ? `${prefix}.${key}` : key)
    )
    .sort()
}
