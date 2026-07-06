import { createContext, useContext, type ReactNode } from "react"
import { useTranslation } from "react-i18next"

import type { BrandInfo } from "./brand"
import { DEFAULT_BRAND } from "./brand"

const BrandContext = createContext<BrandInfo>(DEFAULT_BRAND)

export function BrandProvider({
  brand,
  children,
}: {
  brand: BrandInfo
  children: ReactNode
}) {
  return (
    <BrandContext.Provider value={brand}>{children}</BrandContext.Provider>
  )
}

/** 获取完整品牌信息（中/英两个名字）。 */
export function useBrand(): BrandInfo {
  return useContext(BrandContext)
}

/**
 * 按当前 i18n 语言返回对应的品牌词：
 * 中文（zh-CN）用 name_zh，其他语言用 name_en。
 */
export function useBrandName(): string {
  const brand = useBrand()
  const { i18n } = useTranslation()
  return i18n.language?.startsWith("zh") ? brand.name_zh : brand.name_en
}
