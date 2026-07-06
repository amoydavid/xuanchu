import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"

import "./i18n"
import "./index.css"
import App from "./App.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"
import { TooltipProvider } from "@/components/ui/tooltip.tsx"
import { BrandProvider } from "@/brand/BrandContext"
import {
  DEFAULT_BRAND,
  fetchBranding,
  readCachedBrand,
  writeCachedBrand,
  type BrandInfo,
} from "@/brand/brand"

const queryClient = new QueryClient()

/**
 * 加载品牌信息。有 localStorage 缓存时立即返回缓存值并后台校准（零延迟零闪烁）；
 * 无缓存时才 await 拉取（仅首次访问会多一个请求）。失败回退默认值，不阻塞启动。
 */
async function loadBrand(): Promise<BrandInfo> {
  const cached = readCachedBrand()
  if (cached) {
    fetchBranding()
      .then(writeCachedBrand)
      .catch(() => {})
    return cached
  }
  try {
    const fresh = await fetchBranding()
    writeCachedBrand(fresh)
    return fresh
  } catch {
    return DEFAULT_BRAND
  }
}

async function bootstrap() {
  const brand = await loadBrand()
  // HTML title 用 JS 覆盖为 OEM 名（index.html 里保留默认值作为首屏占位）
  document.title = `${brand.name_en} Web Admin Console`

  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <BrandProvider brand={brand}>
        <ThemeProvider>
          <QueryClientProvider client={queryClient}>
            <TooltipProvider>
              <App />
            </TooltipProvider>
          </QueryClientProvider>
        </ThemeProvider>
      </BrandProvider>
    </StrictMode>
  )
}

void bootstrap()
