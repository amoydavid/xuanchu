// 平台品牌信息。由后端 /api/branding 提供，打包时可通过 ldflags 注入（OEM）。

export interface BrandInfo {
  /** 中文品牌名，例如「璇础」 */
  name_zh: string
  /** 英文品牌名，例如「Xuanchu」 */
  name_en: string
}

/** 代码内默认品牌，仅用于后端不可达或首次加载的兜底。 */
export const DEFAULT_BRAND: BrandInfo = {
  name_zh: "璇础",
  name_en: "Xuanchu",
}

const CACHE_KEY = "xuanchu.console.brand"

/** 读取 localStorage 里上次的品牌信息（用于首屏先渲染再校准，消除闪烁）。 */
export function readCachedBrand(): BrandInfo | null {
  try {
    const raw = localStorage.getItem(CACHE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<BrandInfo>
    if (
      typeof parsed.name_zh === "string" &&
      typeof parsed.name_en === "string"
    ) {
      return { name_zh: parsed.name_zh, name_en: parsed.name_en }
    }
    return null
  } catch {
    return null
  }
}

/** 成功拉取后才写入缓存，避免失败响应污染缓存。 */
export function writeCachedBrand(brand: BrandInfo): void {
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify(brand))
  } catch {
    // 忽略 quota / 隐私模式写入失败
  }
}

/** 从后端拉取品牌信息。端点公开，无需鉴权头。 */
export async function fetchBranding(base = ""): Promise<BrandInfo> {
  const res = await fetch(`${base}/api/branding`)
  if (!res.ok) {
    throw new Error(`branding fetch failed: ${res.status}`)
  }
  const data = (await res.json()) as BrandInfo
  if (typeof data.name_zh !== "string" || typeof data.name_en !== "string") {
    throw new Error("branding response missing fields")
  }
  return data
}
