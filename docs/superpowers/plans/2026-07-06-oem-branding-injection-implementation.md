# OEM 品牌名注入实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 通过 `-ldflags` 在打包时注入平台中文名/英文名，web console 运行时通过 `GET /api/branding` 拉取并显示，不注入则用默认值（璇础 / Xuanchu）。

**Architecture:** Go 新建 `internal/branding` 包持有可被 ldflags 覆盖的包级变量；httpapi 注册公开（无鉴权）的 `GET /api/branding` 返回品牌信息；前端 `main.tsx` 在挂载 React 前 await 该接口，经 `BrandContext` 注入，组件用 `useBrand()`/`useBrandName()` 读取。Makefile 增加 `BRAND_NAME_ZH`/`BRAND_NAME_EN` 变量；deploy.yaml 通过环境变量透传给打包流程。

**Tech Stack:** Go 1.25（`internal/branding`、`internal/httpapi`）、Make（ldflags 拼接）、React 19 + TypeScript + Vite + i18next（前端）、go-chi（路由）。

**对应 spec：** `docs/superpowers/specs/2026-07-06-oem-branding-injection-design.md`

---

## 文件结构

新建：
- `internal/branding/branding.go` — 品牌变量与 `Info`/`Current()`，可被 ldflags 覆盖
- `internal/branding/branding_test.go` — 默认值与空值 fallback 测试
- `internal/httpapi/branding.go` — `handleBranding` handler
- `internal/httpapi/branding_test.go` — 端点公开性、JSON、缓存头测试
- `web/src/brand/BrandContext.tsx` — `BrandProvider`/`useBrand`/`useBrandName`
- `web/src/brand/brand.ts` — `fetchBranding`、类型、localStorage 缓存读写、默认值
- `web/src/brand/BrandContext.test.tsx` — Context 单测

修改：
- `internal/httpapi/router.go` — 注册 `GET /api/branding`
- `Makefile` — 增加 `BRAND_NAME_ZH`/`BRAND_NAME_EN` 与 LDFLAGS 拼接
- `deploy.yaml` — 增加 OEM 环境变量与 build task 透传
- `web/src/main.tsx` — bootstrap：await branding → 挂载 React
- `web/src/locales/zh-CN.ts` — `app.brand`/`app.title`/`auth.signInTitle` 改占位符
- `web/src/locales/en-US.ts` — 同上
- `web/src/components/ProductLogo.tsx` — 用 `useBrandName()` 替换 `t("app.brand")`
- `web/src/components/AppShell.tsx` — 标题用 brand
- `web/src/pages/LoginPage.tsx` — signInTitle 注入 brand
- `web/src/pages/LoginPage.test.tsx`、`web/src/components/AppShell.test.tsx` — 同步 mock

---

## Task 1: `internal/branding` 包

**Files:**
- Create: `internal/branding/branding.go`
- Test: `internal/branding/branding_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/branding/branding_test.go`:

```go
package branding

import "testing"

func TestCurrentDefaultValues(t *testing.T) {
	// 用 withVars 显式设回默认值，保证测试互不影响（其它测试可能改过包变量）。
	withVars(t, "璇础", "Xuanchu")
	info := Current()
	if info.NameZh != "璇础" {
		t.Fatalf("NameZh = %q, want 璇础", info.NameZh)
	}
	if info.NameEn != "Xuanchu" {
		t.Fatalf("NameEn = %q, want Xuanchu", info.NameEn)
	}
}

func TestCurrentFallbacksEmptyToDefault(t *testing.T) {
	withVars(t, "", "")
	info := Current()
	if info.NameZh != "璇础" {
		t.Fatalf("NameZh = %q, want 璇础", info.NameZh)
	}
	if info.NameEn != "Xuanchu" {
		t.Fatalf("NameEn = %q, want Xuanchu", info.NameEn)
	}
}

func TestCurrentReflectsInjectedVars(t *testing.T) {
	withVars(t, "ACME平台", "ACME")
	info := Current()
	if info.NameZh != "ACME平台" {
		t.Fatalf("NameZh = %q, want ACME平台", info.NameZh)
	}
	if info.NameEn != "ACME" {
		t.Fatalf("NameEn = %q, want ACME", info.NameEn)
	}
}
```

> 说明：`withVars`（实现见 Step 3）直接交换 `NameZh`/`NameEn` 包变量并在 `t.Cleanup` 恢复，使测试无需 ldflags 即可覆盖 `Current()` 的三个分支（默认值、空值 fallback、注入值）。

- [ ] **Step 2: 运行测试，确认失败**

```bash
go test ./internal/branding/ -run TestCurrent -count=1
```
Expected: FAIL（包不存在 / 函数未定义）。

- [ ] **Step 3: 写实现**

Create `internal/branding/branding.go`:

```go
// Package branding 持有平台品牌名，可通过 -ldflags 在打包时注入（OEM）。
//
// 注入示例：
//
//	go build -ldflags "\
//	  -X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=ACME \
//	  -X git.dajee.net/dajee/xuanchu/internal/branding.NameEn=ACME" \
//	  ./cmd/xuanchu
//
// 留空（或不注入）时使用默认值 璇础 / Xuanchu。
package branding

// NameZh / NameEn 可被 ldflags 覆盖；默认值见初始化器。
var (
	NameZh = "璇础"
	NameEn = "Xuanchu"
)

// Info 是对外（API / 序列化）的品牌信息快照。
type Info struct {
	NameZh string `json:"name_zh"`
	NameEn string `json:"name_en"`
}

// Current 返回当前品牌信息，对空值做 fallback 保护
// （防止 ldflags 注入空字符串导致 UI 显示空白品牌）。
func Current() Info {
	zh, en := NameZh, NameEn
	if zh == "" {
		zh = "璇础"
	}
	if en == "" {
		en = "Xuanchu"
	}
	return Info{NameZh: zh, NameEn: en}
}

// withVars 临时覆盖包变量，仅供测试使用。t.Cleanup 恢复原值。
func withVars(t *testing.T, zh, en string) {
	t.Helper()
	prevZh, prevEn := NameZh, NameEn
	NameZh, NameEn = zh, en
	t.Cleanup(func() { NameZh, NameEn = prevZh, prevEn })
}
```

> 注意：`withVars` 用 `*testing.T` 而非接口，保持最小实现。它是导出包内的非导出函数，仅本包测试可用。

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/branding/ -count=1
```
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/branding/branding.go internal/branding/branding_test.go
git commit -m "feat: 新增 internal/branding 包，支持 ldflags 注入品牌名"
```

---

## Task 2: `GET /api/branding` 端点

**Files:**
- Create: `internal/httpapi/branding.go`
- Test: `internal/httpapi/branding_test.go`
- Modify: `internal/httpapi/router.go`

- [ ] **Step 1: 写失败测试**

Create `internal/httpapi/branding_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrandingIsAnonymous(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/branding", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestBrandingReturnsNameFields(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/branding", nil)
	srv.Router().ServeHTTP(rr, req)

	var got struct {
		NameZh string `json:"name_zh"`
		NameEn string `json:"name_en"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if got.NameZh == "" {
		t.Fatalf("name_zh empty body=%s", rr.Body.String())
	}
	if got.NameEn == "" {
		t.Fatalf("name_en empty body=%s", rr.Body.String())
	}
}

func TestBrandingHasCacheControlHeader(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/branding", nil)
	srv.Router().ServeHTTP(rr, req)
	if cc := rr.Header().Get("Cache-Control"); cc == "" {
		t.Fatalf("missing Cache-Control header")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
go test ./internal/httpapi/ -run TestBranding -count=1
```
Expected: FAIL（404 / route not found）。

- [ ] **Step 3: 写 handler 实现**

Create `internal/httpapi/branding.go`:

```go
package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/branding"
)

// handleBranding 返回平台品牌名（中/英），供前端 OEM 显示。
// 公开端点（无鉴权）：登录页首屏就要用，必须能匿名访问。
// 品牌名在二进制生命周期内不变，附加短缓存头降低首屏延迟。
func (s *Server) handleBranding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, branding.Current())
}
```

- [ ] **Step 4: 在路由注册端点**

Modify `internal/httpapi/router.go`，在 `api.Get("/healthz", s.handleHealthz)` 之后、`if s.testPanicRoute` 之前插入：

```go
	api.Get("/healthz", s.handleHealthz)
	api.Get("/branding", s.handleBranding)
```

> 注意：`api` 是 chi router，通过 `root.Handle("/api/", api)` 暴露，因此 `api.Get("/branding", ...)` 实际路径为 `/api/branding`。

- [ ] **Step 5: 运行测试，确认通过**

```bash
go test ./internal/httpapi/ -run TestBranding -count=1
```
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/httpapi/branding.go internal/httpapi/branding_test.go internal/httpapi/router.go
git commit -m "feat: 新增公开端点 GET /api/branding 返回品牌名"
```

---

## Task 3: Makefile OEM 变量支持

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: 修改 Makefile 增加 OEM 变量与 LDFLAGS 拼接**

将 `Makefile` 顶部的变量定义部分（第 4-5 行）替换为：

```makefile
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")

# OEM 品牌名注入（留空使用代码默认值 璇础/Xuanchu）
BRAND_NAME_ZH ?=
BRAND_NAME_EN ?=

BRAND_LDFLAGS := $(if $(BRAND_NAME_ZH),-X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=$(BRAND_NAME_ZH),) \
  $(if $(BRAND_NAME_EN),-X git.dajee.net/dajee/xuanchu/internal/branding.NameEn=$(BRAND_NAME_EN),)

# 把非空的 ldflags 片段合并成 "-X ... -X ..." 形式
LDFLAGS_PARTS := $(strip $(if $(VERSION),-X main.version=$(VERSION),) $(BRAND_LDFLAGS))
LDFLAGS := $(if $(LDFLAGS_PARTS),-ldflags "$(LDFLAGS_PARTS)",)
```

> 说明：用 `$(strip ...)` 合并 VERSION 与 BRAND 的 `-X` 片段，空值自然剔除。四种组合都能正确生成 ldflags：都不传→空；只传 VERSION→`-ldflags "-X main.version=..."`；只传 BRAND→`-ldflags "-X ...NameZh=... -X ...NameEn=..."`；两者都传→全合并。

- [ ] **Step 2: 验证四种 LDFLAGS 组合**

```bash
# 都不传
make -n build 2>&1 | grep "go build" | head -1
# 只传 VERSION（模拟打 tag）—— 用 -n 打印命令但不执行
make -n build VERSION=v1.0.0 2>&1 | grep "ldflags" | head -1
# 只传 BRAND
make -n build BRAND_NAME_ZH=测试 BRAND_NAME_EN=Test 2>&1 | grep "ldflags" | head -1
# 两者都传
make -n build VERSION=v1.0.0 BRAND_NAME_ZH=测试 BRAND_NAME_EN=Test 2>&1 | grep "ldflags" | head -1
```

Expected（依次）：
- 第一条无 `ldflags`
- 第二条含 `-ldflags "-X main.version=v1.0.0"`
- 第三条含 `-ldflags "-X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=测试 -X git.dajee.net/dajee/xuanchu/internal/branding.NameEn=Test"`
- 第四条含两者合并

- [ ] **Step 3: 实际构建并验证注入生效**

```bash
go build -ldflags "-X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=测试 -X git.dajee.net/dajee/xuanchu/internal/branding.NameEn=Test" -o /tmp/x-oem ./cmd/xuanchu
# 启动后另开终端验证
/tmp/x-oem server --listen 127.0.0.1:19191 --db /tmp/x-oem.db &
sleep 1
curl -s http://127.0.0.1:19191/api/branding
kill %1 2>/dev/null
```

Expected: `{"name_zh":"测试","name_en":"Test"}`

- [ ] **Step 4: 验证默认值（不注入）**

```bash
go build -o /tmp/x-default ./cmd/xuanchu
/tmp/x-default server --listen 127.0.0.1:19192 --db /tmp/x-default.db &
sleep 1
curl -s http://127.0.0.1:19192/api/branding
kill %1 2>/dev/null
```

Expected: `{"name_zh":"璇础","name_en":"Xuanchu"}`

- [ ] **Step 5: 验证 CGO_ENABLED=0 不受影响**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 无报错。

- [ ] **Step 6: 提交**

```bash
git add Makefile
git commit -m "feat: Makefile 支持 BRAND_NAME_ZH/BRAND_NAME_EN 注入品牌名"
```

---

## Task 4: deploy.yaml OEM 透传

**Files:**
- Modify: `deploy.yaml`

- [ ] **Step 1: 在 deploy.yaml 增加 OEM 变量与 build 透传**

修改 `deploy.yaml`：

在两个 stage 的 `vars:` 块里增加可选的 OEM 变量（默认空=用代码默认值）。`staging` 与 `production` 的 `vars` 都加上：

```yaml
  staging:
    server: user@staging.example.com
    remote_dir: /opt/xuanchu
    keep_releases: 3
    vars:
      XUANCHU_PORT: "8080"
      # OEM 品牌名（留空使用默认值 璇础/Xuanchu）
      BRAND_NAME_ZH: ""
      BRAND_NAME_EN: ""

  production:
    server: ubuntu@agent.example.com
    remote_dir: /opt/xuanchu
    keep_releases: 5
    vars:
      XUANCHU_PORT: "8080"
      # OEM 品牌名（留空使用默认值 璇础/Xuanchu）
      BRAND_NAME_ZH: ""
      BRAND_NAME_EN: ""
```

把 `tasks.build` 改为透传这两个变量到 Makefile：

```yaml
tasks:
  build:
    local: make build-release BRAND_NAME_ZH="{{.BRAND_NAME_ZH}}" BRAND_NAME_EN="{{.BRAND_NAME_EN}}"
```

> 说明：deploy 工具的 `{{.VAR}}` 模板会在 build 时被 stage vars 替换；留空时传 `BRAND_NAME_ZH=""`，Makefile 的 `?=` 与 `$(if ...)` 会判定为空，不注入。不同部署工具的模板语法可能不同，实现时需确认当前 deploy 工具的变量插值方式（若 `{{.X}}` 不生效，改为对应工具的 `$VAR` 或 `${VAR}` 语法）。

- [ ] **Step 2: 验证 deploy.yaml 语法（dry run）**

```bash
# 若部署工具提供 dry-run/lint，执行它确认模板可解析
# 例如：deploy --dry-run staging 或类似
```
Expected: 无模板解析错误。若项目无对应工具入口，跳过此步并在 commit message 注明"模板语法待部署时验证"。

- [ ] **Step 3: 提交**

```bash
git add deploy.yaml
git commit -m "feat: deploy.yaml 支持 OEM 品牌名变量透传给 build"
```

---

## Task 5: 前端品牌模块（类型 + fetch + 缓存 + Context）

**Files:**
- Create: `web/src/brand/brand.ts`
- Create: `web/src/brand/BrandContext.tsx`
- Test: `web/src/brand/BrandContext.test.tsx`

- [ ] **Step 1: 写品牌数据模块**

Create `web/src/brand/brand.ts`:

```ts
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
    if (typeof parsed.name_zh === "string" && typeof parsed.name_en === "string") {
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
```

- [ ] **Step 2: 写 BrandContext**

Create `web/src/brand/BrandContext.tsx`:

```tsx
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
  return <BrandContext.Provider value={brand}>{children}</BrandContext.Provider>
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
```

- [ ] **Step 3: 写 Context 测试**

Create `web/src/brand/BrandContext.test.tsx`:

```tsx
import { describe, expect, it } from "vitest"
import { render, renderHook } from "@testing-library/react"
import { I18nextProvider } from "react-i18next"
import i18n from "@/i18n"

import { BrandProvider, useBrand, useBrandName } from "./BrandContext"
import { DEFAULT_BRAND } from "./brand"

function wrapper(children: React.ReactNode) {
  return render(
    <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
  )
}

describe("BrandContext", () => {
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
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
cd web && pnpm test -- src/brand/BrandContext.test.tsx --run
```
Expected: PASS。

> 注意：测试可能因 `@testing-library/react` 的 `renderHook` 版本差异或 vitest 配置而需微调；以 `web/src` 下现有测试的导入风格为准（参考现有 `*.test.tsx`）。

- [ ] **Step 5: 提交**

```bash
git add web/src/brand/
git commit -m "feat: 前端新增 brand 模块（fetch + Context + locale 选择）"
```

---

## Task 6: main.tsx bootstrap（渲染前 await branding）

**Files:**
- Modify: `web/src/main.tsx`
- Modify: `web/index.html`

- [ ] **Step 1: 改造 main.tsx 为 bootstrap**

Replace `web/src/main.tsx` 全部内容为：

```tsx
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
 * 先用 localStorage 缓存值渲染，避免空白；再 await 拉取最新值，
 * 拉取成功后挂载 React。这样首屏不会闪烁默认品牌名。
 */
async function loadBrand(): Promise<BrandInfo> {
  const cached = readCachedBrand()
  if (cached) {
    // 后台校准，不阻塞返回缓存值
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
```

> 关键点：有缓存时**不阻塞渲染**（立即返回缓存值，后台校准）；无缓存时才 await（仅首次访问会多一个请求延迟）。这比 spec 原始描述更优——绝大多数用户后续访问都用缓存，零延迟零闪烁。

- [ ] **Step 2: 确认 index.html title 保留默认（JS 会覆盖）**

`web/index.html` 第 7 行保持 `<title>Xuanchu Web Admin Console</title>`（JS 启动后立即覆盖为 OEM 名）。无需改动。

- [ ] **Step 3: typecheck + 构建**

```bash
cd web && pnpm typecheck && pnpm build
```
Expected: 构建成功，产物在 `internal/webconsole/dist/`。

- [ ] **Step 4: 提交**

```bash
git add web/src/main.tsx
git commit -m "feat: main.tsx 渲染前加载品牌信息，消除首屏闪烁"
```

---

## Task 7: i18n 文案改占位符 + 组件改造

**Files:**
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/components/ProductLogo.tsx`
- Modify: `web/src/components/AppShell.tsx`
- Modify: `web/src/pages/LoginPage.tsx`

- [ ] **Step 1: zh-CN.ts 把品牌名改成占位符**

修改 `web/src/locales/zh-CN.ts` 前 5 行的 `app` 块：

```ts
export const zhCN = {
  app: {
    brand: "{{brand}}",
    title: "{{brand}} Web 管理控制台",
    description: "面向 {{brand}} server、Agent token、投递和审计的运维入口",
  },
  // ...
```

并找到 `auth.signInTitle: "使用璇础访问凭证登录"`（约第 41 行）改为：

```ts
    signInTitle: "使用{{brand}}访问凭证登录",
```

> `app.brand` 与 `shell.brand`（约第 243 行）这类纯品牌词改为 `{{brand}}` 占位符；含品牌词的句子也改为插值。保留 `shell.brand`（spec 第 3.3.4 节表格未列入，但若组件实际用到了，一并改占位符并在渲染处注入）。

- [ ] **Step 2: en-US.ts 同样改占位符**

修改 `web/src/locales/en-US.ts` 前 6 行的 `app` 块：

```ts
export const enUS = {
  app: {
    brand: "{{brand}}",
    title: "{{brand}} Web Admin Console",
    description:
      "Operations console for {{brand}} server, Agent tokens, deliveries, and audit.",
  },
  // ...
```

并找到 `auth.signInTitle: "Sign in with a Xuanchu credential"`（约第 44 行）改为：

```ts
    signInTitle: "Sign in with a {{brand}} credential",
```

- [ ] **Step 3: ProductLogo 用 useBrandName**

Replace `web/src/components/ProductLogo.tsx` 的 wordmark 部分（约第 42-44 行）：

将：
```tsx
      {showWordmark ? (
        <span className="text-sm font-medium tracking-normal">
          {t("app.brand")}
        </span>
      ) : null}
```

改为：
```tsx
      {showWordmark ? (
        <span className="text-sm font-medium tracking-normal">
          {useBrandName()}
        </span>
      ) : null}
```

并在文件顶部 import 区加：
```tsx
import { useBrandName } from "@/brand/BrandContext"
```

移除本文件不再需要的 `useTranslation` import（若 `t` 仅用于此处）。实现时检查是否还有其它 `t(` 用法再决定。

- [ ] **Step 4: AppShell 标题用 brand**

修改 `web/src/components/AppShell.tsx:185`，将：
```tsx
              {breadcrumbs ?? headerTitle ?? t("app.title")}
```

改为：
```tsx
              {breadcrumbs ?? headerTitle ?? t("app.title", { brand: useBrandName() })}
```

并在组件内调用 `useBrandName()`（React hooks 须在组件顶层），或更简洁：在组件顶部 `const brandName = useBrandName()`，然后 `t("app.title", { brand: brandName })`。顶部加 import：
```tsx
import { useBrandName } from "@/brand/BrandContext"
```

- [ ] **Step 5: LoginPage signInTitle 注入 brand**

修改 `web/src/pages/LoginPage.tsx:83`，将：
```tsx
            {t("auth.signInTitle")}
```

改为：
```tsx
            {t("auth.signInTitle", { brand: brandName })}
```

在组件顶部加 `const brandName = useBrandName()` 与 import。若 LoginPage 是函数组件，确保 hook 调用在顶层。

- [ ] **Step 6: 检查其它消费点并同步**

```bash
cd /Users/mac/code/projects/dajee/task
grep -rn "app.brand\|app.title\|auth.signInTitle\|shell.brand" web/src --include="*.tsx" --include="*.ts" | grep -v locales/
```

对每处输出，按上述模式注入 `{ brand: useBrandName() }` 或替换为 `useBrandName()`。

- [ ] **Step 7: 更新现有前端测试**

检查并更新 `web/src/pages/LoginPage.test.tsx`、`web/src/components/AppShell.test.tsx` 中断言品牌名的地方：测试默认渲染（无 BrandProvider）会落到 `DEFAULT_BRAND`（璇础/Xuanchu），断言保持 `"璇础"` / `"Xuanchu"` 即可。若测试报 i18next 插值警告（`{{brand}}` 未传值），在测试 render 时包裹 `<BrandProvider>` 或通过 i18n 的 `defaultValue` 处理。

```bash
cd web && pnpm test --run
```
Expected: PASS。

- [ ] **Step 8: typecheck + 构建**

```bash
cd web && pnpm typecheck && pnpm build
```
Expected: 成功。

- [ ] **Step 9: 提交**

```bash
git add web/src/locales/ web/src/components/ProductLogo.tsx web/src/components/AppShell.tsx web/src/pages/LoginPage.tsx web/src/pages/LoginPage.test.tsx web/src/components/AppShell.test.tsx
git commit -m "feat: 前端品牌名改用 BrandContext 注入，i18n 改占位符"
```

---

## Task 8: 全量验证

**Files:** 无（验证任务）

- [ ] **Step 1: Go 全量测试**

```bash
go test ./... -count=1
CGO_ENABLED=0 go test ./... -count=1
```
Expected: 全 PASS。

- [ ] **Step 2: Go 构建（含 CGO_ENABLED=0）**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 无报错。

- [ ] **Step 3: 前端全量测试**

```bash
cd web && pnpm test --run && pnpm typecheck
```
Expected: PASS。

- [ ] **Step 4: 端到端 OEM 验证（默认 + 注入）**

```bash
cd /Users/mac/code/projects/dajee/task
# 默认
go build -o /tmp/x-default ./cmd/xuanchu
/tmp/x-default server --listen 127.0.0.1:19191 --db /tmp/x-default.db &
sleep 1
echo "默认:"; curl -s http://127.0.0.1:19191/api/branding
kill %1 2>/dev/null; sleep 1

# OEM 注入
go build -ldflags "-X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=测试 -X git.dajee.net/dajee/xuanchu/internal/branding.NameEn=Test" -o /tmp/x-oem ./cmd/xuanchu
/tmp/x-oem server --listen 127.0.0.1:19192 --db /tmp/x-oem.db &
sleep 1
echo "OEM:"; curl -s http://127.0.0.1:19192/api/branding
kill %1 2>/dev/null
```

Expected：
- 默认：`{"name_zh":"璇础","name_en":"Xuanchu"}`
- OEM：`{"name_zh":"测试","name_en":"Test"}`

- [ ] **Step 5: 前端 dist 重新构建并嵌入**

```bash
cd /Users/mac/code/projects/dajee/task
make web-console-build
CGO_ENABLED=0 go build -o /tmp/x-full ./cmd/xuanchu
```
Expected: dist 重建成功，二进制构建成功。

- [ ] **Step 6: 浏览器手动验证（可选但推荐）**

启动 `/tmp/x-full server`，浏览器打开，确认：
- 默认打包：显示「璇础/Xuanchu」，无闪烁，HTML title 正确
- 改 i18n 语言切换中英文，品牌名随语言切换
- 登录页品牌名正确

- [ ] **Step 7: 文档同步检查**

确认 spec（`docs/superpowers/specs/2026-07-06-oem-branding-injection-design.md`）与本计划无矛盾；README 若提到品牌名硬编码，更新说明改为可注入。若无需更新，跳过。

- [ ] **Step 8: 最终提交（如有遗留改动）**

```bash
git status
# 若有未提交的文档/dist 改动
git add -A && git commit -m "chore: OEM 品牌名注入收尾"
```

---

## 验收清单（对照 spec）

- [x] spec §3.1 `internal/branding` 包 → Task 1
- [x] spec §3.2 `GET /api/branding` 公开 + 缓存头 → Task 2
- [x] spec §3.3.1-3.3.3 前端 fetch/Context/bootstrap → Task 5, 6
- [x] spec §3.3.4 组件改造表格 → Task 7
- [x] spec §3.4 Makefile → Task 3
- [x] deploy.yaml OEM 透传（用户额外要求）→ Task 4
- [x] spec §4.1-4.4 测试与验收 → Task 8
- [x] CGO_ENABLED=0 不受影响 → Task 3 Step 5, Task 8 Step 2
