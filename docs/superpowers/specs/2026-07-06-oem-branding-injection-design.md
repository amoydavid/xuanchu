# OEM 品牌名注入设计（ldflags + 运行时 API）

## 1. 背景与目标

当前 web console（React SPA，经 Go `embed` 打进单二进制）展示的品牌名 "Xuanchu" / "璇础" 硬编码在前端 i18n locale、HTML title 等多处，没有集中配置。

需要支持 OEM：在 **Go 打包阶段** 通过 `-ldflags` 注入平台中文名和英文名，注入后 web console 显示新名称；不注入则使用默认值（璇础 / Xuanchu）。

### 范围边界（重要）

本次 **只覆盖前端展示名**（用户在 UI 上看到的品牌词）。以下 **不在范围内**，保持原样：

- token 前缀（`xuanchu_pat_` / `xuanchu_tenant_` / `xuanchu_act_` / `xuanchu_admin_` / `xuanchu_agent_`）
- cookie / header 名（`xuanchu_session`、`X-Xuanchu-CSRF`、`X-Xuanchu-Event` 等）
- 环境变量（`XUANCHU_DB`、`XUANCHU_DB_URL` 等）
- 配置路径（`~/.config/xuanchu`、`~/.local/share/xuanchu`）
- Go module path（`git.dajee.net/dajee/xuanchu`）
- 二进制名 / systemd 服务名
- logo 图片资源
- 版权 / 备案文案
- 后端 CLI 输出中的 "xuanchu" 字样

理由：这些是基础设施命名空间，改名会破坏向后兼容（存量 token 失配、存量配置失效），工程量大且收益不在本次目标内。

### 成功标准

1. `make build BRAND_NAME_ZH=ACME BRAND_NAME_EN=ACME` 产出的二进制，web console 显示 ACME（中英一致替换原"璇础/Xuanchu"的位置）。
2. `make build`（不传 OEM 变量）产出的二进制，行为与今天完全一致，显示"璇础/Xuanchu"。
3. 前端首屏不闪烁默认品牌名（登录页尤其不能闪）。
4. HTML `<title>` 正确反映 OEM 名。
5. `CGO_ENABLED=0 go build ./cmd/xuanchu` 不受影响。

## 2. 方案概述

```
打包时                  运行时
─────                  ─────
make build             main 初始化
  BRAND_NAME_ZH          ↓
  BRAND_NAME_EN        internal/branding 包变量（已被 ldflags 覆盖）
    ↓                    ↓
  go build             httpapi 注册 GET /api/branding（公开，无鉴权）
  -ldflags               ↓
  -X .../branding.     前端 main.tsx 在 React 挂载前 await 该接口
    NameZh / NameEn       ↓
                       branding 写入 React Context + localStorage
                         ↓
                       UI 用 useBrand() 读取；document.title 同步设置
```

链路：`-ldflags` → `internal/branding` 包变量 → `/api/branding` 端点 → 前端启动时拉取 → Context 注入 → 组件消费。

**为什么是这个方案（而不是前端 build 时注入）**：用户明确要求"在打包时通过类似 -ldflags 的方式向 golang 注入"，且 Go ldflags 是单一注入入口，不依赖前端构建环境变量。代价是首屏需要先 await 一次 API——通过"渲染前 await + localStorage 缓存"消除闪烁。

## 3. 详细设计

### 3.1 Go 侧：`internal/branding` 包

新建 `internal/branding/branding.go`：

```go
// Package branding 持有平台品牌名，可通过 -ldflags 在打包时注入（OEM）。
package branding

// 以下变量可通过 ldflags 覆盖：
//   go build -ldflags "-X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=ACME -X .../NameEn=ACME"
// 留空则使用下面的默认值。
var (
	NameZh = "璇础"
	NameEn = "Xuanchu"
)

// Info 是对外（API / 序列化）的品牌信息快照。
type Info struct {
	NameZh string `json:"name_zh"`
	NameEn string `json:"name_en"`
}

// Current 返回当前品牌信息。
func Current() Info {
	return Info{NameZh: NameZh, NameEn: NameEn}
}
```

设计要点：
- 默认值写在变量初始化器里，ldflags 注入时直接覆盖整个变量，**不需要 fallback 分支**（与 `main.version` 用 `if version == ""` 的模式不同，因为品牌名总有合法默认值，空值才需要 fallback 到默认）。
- 空值保护：在 `Current()` 里加一行 `if NameZh == "" { NameZh = "璇础" }`（防御性，避免 ldflags 注入空字符串）。
- 包内单元测试：默认值正确、ldflags 覆盖后正确、空值 fallback 正确。

### 3.2 HTTP API：`GET /api/branding`

**端点**：`GET /api/branding`
**鉴权**：**公开，无需登录**（登录页首屏就要用，所以不能挂在 auth middleware 后面）。
**响应**：

```json
{
  "name_zh": "璇础",
  "name_en": "Xuanchu"
}
```

**缓存**：加 `Cache-Control: public, max-age=300`（5 分钟），品牌名在二进制生命周期内不变，可安全缓存，减少首屏 await 延迟。

**注册位置**：`internal/httpapi/router.go` 的 `newRouter()`，与 `/healthz` 同级（公开路由组），不走 auth middleware。

**是否复用 huma**：与现有 `/healthz` 风格保持一致即可（healthz 看起来是直接挂在 ServeMux 上的简单 handler）。`Server` 结构体已经有 branding 信息可用（通过 `branding.Current()` 直接调，无需往 Server 注入字段）。

### 3.3 前端：启动时加载 + Context 注入

#### 3.3.1 API client

在 `web/src/lib/api.ts` 附近新增 branding 获取函数（纯 fetch，不带鉴权头，因为端点公开）：

```ts
export async function fetchBranding(): Promise<BrandInfo> {
  const res = await fetch(`${apiBase}/api/branding`);
  if (!res.ok) throw new Error("branding fetch failed");
  return res.json();
}

export interface BrandInfo {
  name_zh: string;
  name_en: string;
}
```

#### 3.3.2 BrandContext

新建 `web/src/brand/BrandContext.tsx`：

```tsx
const DEFAULT_BRAND: BrandInfo = { name_zh: "璇础", name_en: "Xuanchu" };
const CACHE_KEY = "xuanchu.console.brand"; // 注意：namespace 前缀不在 OEM 范围内，保持 xuanchu

const BrandContext = createContext<BrandInfo>(DEFAULT_BRAND);

export function BrandProvider({ brand, children }: { brand: BrandInfo; children: ReactNode }) {
  return <BrandContext.Provider value={brand}>{children}</BrandContext.Provider>;
}

export function useBrand(): BrandInfo {
  return useContext(BrandContext);
}
```

#### 3.3.3 main.tsx 启动流程（关键：消除闪烁）

修改 `web/src/main.tsx`，在 `createRoot(...).render()` **之前** 加载 branding：

```tsx
async function bootstrap() {
  // 1. 先用 localStorage 缓存值，避免完全空白
  let brand: BrandInfo = readCachedBrand() ?? DEFAULT_BRAND;

  try {
    // 2. await 拉取最新（接口有缓存头，很快）
    brand = await fetchBranding();
    writeCachedBrand(brand); // 成功后才写入缓存，失败不污染缓存
  } catch {
    // 拉取失败：用缓存值或默认值，不阻塞启动
  }

  // 3. 设置 HTML title
  document.title = `${brand.name_en} Web Admin Console`;

  // 4. 用拿到的 brand 挂载 React
  createRoot(container).render(
    <BrandProvider brand={brand}>
      <App />
    </BrandProvider>
  );
}

bootstrap();
```

要点：
- **先渲染缓存值再校准**：如果 localStorage 有上次的品牌名，首屏立即用它（即使这次 fetch 还没回来），fetch 回来后用最新值挂载。两者通常一致（同一部署），所以用户看不到任何闪烁。
- **fetch 失败不阻塞**：网络错误时用缓存/默认值，保证 console 可用。
- **title 用 JS 设置**：HTML 里 `<title>` 写一个默认值（保留现有 `Xuanchu Web Admin Console`），JS 启动后立即覆盖为 OEM 名。

#### 3.3.4 组件改造：用 `useBrand()` 替换硬编码

把以下硬编码改为读取 `useBrand()`：

| 位置 | 现状 | 改造 |
|------|------|------|
| `web/src/components/ProductLogo.tsx:43` | `{t("app.brand")}` | 根据 locale 选 `brand.name_zh` / `brand.name_en` |
| `web/src/components/AppShell.tsx:185` | `t("app.title")`（含品牌名） | 用 brand 拼接 |
| `web/src/pages/LoginPage.tsx` 登录标题 | `t("auth.signInTitle")` 含"璇础/Xuanchu" | 用 brand 拼接 |
| `web/index.html:6` `<title>` | 写死 `Xuanchu Web Admin Console` | 保留为默认，JS 启动覆盖 |

**locale 选择规则**：前端 i18n 已有 `i18n.language`，中文（zh-CN）用 `name_zh`，其他用 `name_en`。封装一个 `useBrandName()`：返回当前 locale 对应的品牌词。

**i18n 文案改造**：原来含品牌名的句子（如 `auth.signInTitle: "使用璇础访问凭证登录"`），改成带占位符的模板 `"使用{{brand}}访问凭证登录"`，渲染时注入 brand。需要更新 `zh-CN.ts` 和 `en-US.ts`。

### 3.4 Makefile

修改 `Makefile`，增加 OEM 变量（默认空 = 用代码默认值）：

```makefile
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")

# OEM 品牌名注入（留空使用代码默认值 璇础/Xuanchu）
BRAND_NAME_ZH ?=
BRAND_NAME_EN ?=

# 拼接 ldflags
BRAND_LDFLAGS := $(if $(BRAND_NAME_ZH),-X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=$(BRAND_NAME_ZH),) \
  $(if $(BRAND_NAME_EN),-X git.dajee.net/dajee/xuanchu/internal/branding.NameEn=$(BRAND_NAME_EN),)

LDFLAGS := $(if $(VERSION),-ldflags "-X main.version=$(VERSION) $(BRAND_LDFLAGS)",$(if $(BRAND_LDFLAGS),-ldflags "$(BRAND_LDFLAGS)",))
```

（实现时需小心 LDFLAGS 在 VERSION 为空、BRAND 非空时的拼接，写完后用三种组合验证：都不传 / 只传 VERSION / 只传 BRAND / 两者都传。）

OEM 打包示例：

```bash
make build BRAND_NAME_ZH="ACME 平台" BRAND_NAME_EN="ACME"
```

## 4. 测试与验收

### 4.1 Go 侧

- `internal/branding/branding_test.go`：
  - 默认值 = `{璇础, Xuanchu}`
  - 空字符串 fallback 到默认
- `internal/httpapi` 新增（或复用现有测试套）：
  - `GET /api/branding` 返回 200 + 正确 JSON
  - 无需鉴权头即可访问
  - 响应含 `Cache-Control` 头
- 验证 `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过
- 验证 `go test ./...` 通过

### 4.2 ldflags 注入验证

手动验证四种打包组合（写一个临时脚本或在 Makefile 旁加 `make build-oem-test` target 输出二进制后跑 `/api/branding`）：

```bash
# 默认
go build -o /tmp/x-default ./cmd/xuanchu
# 注入
go build -ldflags "-X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=测试 -X git.dajee.net/dajee/xuanchu/internal/branding.NameEn=Test" -o /tmp/x-oem ./cmd/xuanchu
# 启动后 curl /api/branding 验证
```

### 4.3 前端

- `main.tsx` 改造后，现有 `LoginPage.test.tsx` / `AppShell.test.tsx` 需同步更新（mock branding 接口）。
- 新增 `BrandContext` 的单元测试。
- 手动验证：
  - 默认打包：显示"璇础/Xuanchu"，无闪烁
  - OEM 打包：显示 OEM 名，无闪烁，HTML title 正确
  - branding 接口失败：回退默认值，不阻塞

### 4.4 验收清单

- [ ] `make build`（不传 OEM）行为与今天一致
- [ ] `make build BRAND_NAME_ZH=ACME BRAND_NAME_EN=ACME` 后 web console 显示 ACME
- [ ] 登录页首屏不闪烁默认名
- [ ] HTML `<title>` 反映 OEM 名
- [ ] `GET /api/branding` 公开可访问、有缓存头
- [ ] `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过
- [ ] `go test ./...` 通过
- [ ] 前端测试通过（`pnpm test`）

## 5. 风险与权衡

| 风险 | 说明 | 缓解 |
|------|------|------|
| 首屏 await 增加延迟 | branding 接口必须先返回才能渲染 | 接口加 `Cache-Control: max-age=300`；前端用 localStorage 缓存上次值先渲染；接口本身极轻（无 DB 查询） |
| 漏改硬编码点 | 品牌名分散在多个 i18n key | 在 spec 中列出所有改造点（3.3.4 表格），实现时逐一核对 |
| ldflags 拼接错误 | VERSION/BRAND 组合时空格/引号问题 | 用四种组合手动验证（4.2） |
| 现有前端测试断言 `app.brand` | 改造后断言会失败 | 同步更新测试，mock branding |

## 6. 不做（YAGNI）

- 不做 logo OEM（复杂度高，需另外的资产注入机制）。
- 不做版权/备案 OEM。
- 不做后端 CLI/MCP 输出的品牌名替换。
- 不做多套 OEM profile 的运行时切换（一次打包一个品牌）。
- 不做 branding 的配置文件覆盖（ldflags 是唯一入口）。
