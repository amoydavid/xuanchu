import { renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"

import { i18n } from "@/i18n"

import { DEFAULT_BRAND } from "./brand"
import { BrandProvider, useBrand, useBrandName } from "./BrandContext"

describe("BrandContext", () => {
  afterEach(() => {
    i18n.changeLanguage("zh-CN")
  })

  it("useBrand returns provider value", () => {
    const brand = { name_zh: "ACME平台", name_en: "ACME" }
    const { result } = renderHook(() => useBrand(), {
      wrapper: ({ children }) => (
        <BrandProvider brand={brand}>{children}</BrandProvider>
      ),
    })
    expect(result.current).toEqual(brand)
  })

  it("useBrand returns default when no provider", () => {
    const { result } = renderHook(() => useBrand())
    expect(result.current).toEqual(DEFAULT_BRAND)
  })

  it("useBrandName returns zh name for zh-CN", () => {
    i18n.changeLanguage("zh-CN")
    const brand = { name_zh: "ACME平台", name_en: "ACME" }
    const { result } = renderHook(() => useBrandName(), {
      wrapper: ({ children }) => (
        <BrandProvider brand={brand}>{children}</BrandProvider>
      ),
    })
    expect(result.current).toBe("ACME平台")
  })

  it("useBrandName returns en name for en-US", () => {
    i18n.changeLanguage("en-US")
    const brand = { name_zh: "ACME平台", name_en: "ACME" }
    const { result } = renderHook(() => useBrandName(), {
      wrapper: ({ children }) => (
        <BrandProvider brand={brand}>{children}</BrandProvider>
      ),
    })
    expect(result.current).toBe("ACME")
  })
})
