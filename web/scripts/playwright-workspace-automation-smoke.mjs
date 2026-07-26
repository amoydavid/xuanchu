// Workspace Automation Playwright smoke。
// 验证 /automations 桌面/移动布局、规则 Dialog、Provider Dialog、运行记录抽屉
// 在 mock API 下渲染正常、无横向滚动、不暴露 API key 明文。
//
// 运行：pnpm --dir web run smoke:workspace-automation
import { mkdirSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { spawn } from "node:child_process"
import net from "node:net"
import { chromium } from "playwright"

const webRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)))
const port = await getFreePort()
const baseURL = `http://127.0.0.1:${port}`
const screenshotDir = path.join(tmpdir(), "xuanchu-workspace-automation-smoke")
mkdirSync(screenshotDir, { recursive: true })

const server = spawn(
  "pnpm",
  ["exec", "vite", "--host", "127.0.0.1", "--port", String(port)],
  {
    cwd: webRoot,
    env: {
      ...process.env,
      BROWSER: "none",
      VITE_XUANCHU_API_TARGET: "http://127.0.0.1:9",
    },
    stdio: ["ignore", "pipe", "pipe"],
  }
)

let serverLog = ""
server.stdout.on("data", (chunk) => {
  serverLog += chunk.toString()
})
server.stderr.on("data", (chunk) => {
  serverLog += chunk.toString()
})

async function main() {
  await waitForServer(baseURL)

  const browser = await chromium.launch({ headless: true })
  try {
    await runDesktopSmoke(browser)
    await runMobileSmoke(browser)
  } finally {
    await browser.close()
  }

  console.log(`workspace automation smoke screenshots: ${screenshotDir}`)
}

// mock 数据：一条 workspace project.created 规则 + 一条 succeeded delivery + provider config。
const rule = {
  id: "rule-smoke-1",
  workspace_id: "ws-smoke",
  scope_type: "workspace",
  scope_id: "ws-smoke",
  name: "新项目知识库初始化",
  description: "",
  enabled: true,
  trigger_type: "event",
  trigger_config: { event_type: "project.created" },
  condition: { task_filter: "", max_tasks: 50 },
  action_type: "openai_compatible",
  action: {
    protocol: "chat_completions",
    base_url_config_key: "agent.provider.base_url",
    api_key_config_key: "agent.provider.api_key",
    model_config_key: "agent.provider.model",
    temperature: 0.2,
  },
  context: { include: ["workspace", "project", "project_config", "event"] },
  instruction_template:
    "你负责初始化新项目的知识库。先 project_config_list 检查 knowledge_id，没有再创建并 project_config_set 写回。",
  system_prompt: "",
  created_by: { id: "u1", name: "alice", display_name: "Alice" },
  created_at: 1785056400,
  modified_at: 1785056400,
  last_delivery: {
    delivery_id: "dlv-smoke-1",
    status: "succeeded",
    response_status_code: 200,
    created_at: 1785056460,
  },
}

const delivery = {
  id: "dlv-smoke-1",
  workspace_id: "ws-smoke",
  rule_scope_type: "workspace",
  rule_scope_id: "ws-smoke",
  project_id: "proj-smoke",
  rule_id: "rule-smoke-1",
  trigger_type: "event",
  event_id: "evt-smoke-1",
  event_type: "project.created",
  dedupe_key: "event:workspace:rule-smoke-1:evt-smoke-1",
  status: "succeeded",
  resolved_url: "https://agent.example.com/v1/chat/completions",
  rendered_method: "POST",
  rendered_headers: {
    Authorization: ["Bearer ****"],
    "Content-Type": ["application/json"],
  },
  request_body_preview: JSON.stringify({ model: "workspace-operator" }),
  request_body_hash: "sha256:abc",
  response_status_code: 200,
  response_body_preview: JSON.stringify({ id: "chatcmpl_smoke", usage: {} }),
  provider_request_id: "chatcmpl_smoke",
  usage: { prompt_tokens: 5 },
  attempt_count: 1,
  max_attempts: 5,
  last_error: "",
  created_at: 1785056460,
  modified_at: 1785056460,
  project: { id: "proj-smoke", slug: "atlas", name: "Atlas", status: "planning" },
}

const providerConfig = {
  base_url: "https://agent.example.com",
  model: "workspace-operator",
  allowed_hosts: ["agent.example.com"],
  api_key_set: false,
  complete: false,
  missing_fields: ["api_key"],
}

async function runDesktopSmoke(browser) {
  const page = await newMockedPage(browser, { height: 900, width: 1280 })
  try {
    await page.goto(`${baseURL}/automations`)
    await expectText(page, "工作空间自动化")
    await expectText(page, "新项目知识库初始化")
    await expectText(page, "project.created")
    await assertNoHorizontalOverflow(page, "desktop automation rules")
    await screenshot(page, "desktop-automations-rules")

    // 切换到运行记录 tab。
    await page.getByRole("button", { name: "运行记录" }).click()
    await expectText(page, "atlas")
    await expectText(page, "成功")
    await assertNoHorizontalOverflow(page, "desktop automation deliveries")
    await screenshot(page, "desktop-automations-deliveries")

    // 打开运行详情抽屉。
    await page.getByRole("button", { name: "操作" }).first().click()
    await page.getByRole("menuitem", { name: "详情" }).click()
    await expectText(page, "运行详情")
    await expectText(page, "Agent 调用成功")
    await assertNoHorizontalOverflow(page, "desktop automation delivery detail")
    await screenshot(page, "desktop-automations-detail")
    // 关闭抽屉回到列表。
    await page.keyboard.press("Escape")

    // 新建 Dialog。
    const createBtn = page.getByRole("button", { name: /新建自动化/ })
    await createBtn.click()
    await expectText(page, "新建工作空间自动化")
    await expectText(page, "Workspace config")
    // api_key 占位文案不暴露明文。
    const bodyText = await page.locator("body").innerText()
    if (bodyText.includes("sk-")) {
      throw new Error("desktop dialog leaked api key secret")
    }
    await assertNoHorizontalOverflow(page, "desktop automation rule dialog")
    await screenshot(page, "desktop-automations-rule-dialog")
    // 关闭新建 Dialog。
    await page.keyboard.press("Escape")

    // Provider 缺失 warning 中的「查看配置」入口（complete=false 时显示）。
    await page.getByRole("button", { name: "查看配置" }).click()
    await expectText(page, "Agent Provider 配置")
    await expectText(page, "API Key 永不回显")
    const providerText = await page.locator("body").innerText()
    if (providerText.includes("sk-")) {
      throw new Error("provider dialog leaked api key secret")
    }
    await screenshot(page, "desktop-automations-provider-dialog")
  } finally {
    await page.close()
  }
}

async function runMobileSmoke(browser) {
  const page = await newMockedPage(browser, { height: 812, width: 375 })
  try {
    await page.goto(`${baseURL}/automations`)
    await expectText(page, "新项目知识库初始化")
    // 移动端使用 card list（不是 Table）。
    await assertNoHorizontalOverflow(page, "mobile automation rules")
    await screenshot(page, "mobile-automations-rules")

    await page.getByRole("button", { name: "运行记录" }).click()
    await expectText(page, "atlas")
    await assertNoHorizontalOverflow(page, "mobile automation deliveries")
    await screenshot(page, "mobile-automations-deliveries")
  } finally {
    await page.close()
  }
}

async function newMockedPage(browser, viewport) {
  const page = await browser.newPage({ viewport })
  await page.addInitScript(() => {
    localStorage.setItem("xuanchu.console.language", "zh-CN")
    sessionStorage.setItem("xuanchu.console.token", "smoke-token")
  })
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const pathName = url.pathname
    const method = request.method()

    if (method === "GET" && pathName === "/api/v1/credentials/current") {
      await fulfill(route, {
        actor_type: "user",
        actor: { name: "Alice", id: "u1", display_name: "Alice" },
        effective_role: "owner",
        effective_workspace: { slug: "local", name: "Local" },
        token: { scopes: ["*"], type: "workspace" },
      })
      return
    }

    if (method === "GET" && pathName === "/api/v1/automations") {
      await fulfill(route, [rule])
      return
    }
    if (method === "GET" && pathName === "/api/v1/automations/template-vars") {
      await fulfill(route, { triggers: [] })
      return
    }
    if (method === "GET" && pathName === "/api/v1/automations/provider-config") {
      await fulfill(route, providerConfig)
      return
    }
    if (method === "GET" && pathName === "/api/v1/automation-deliveries") {
      await fulfill(route, [delivery])
      return
    }
    if (method === "GET" && pathName.startsWith("/api/v1/automation-deliveries/")) {
      await fulfill(route, delivery)
      return
    }
    if (method === "GET" && pathName === "/api/v1/projects") {
      // sample project list（event preview/test 选择器）。
      await fulfill(route, [
        { id: "proj-smoke", slug: "atlas", name: "Atlas", status: "planning", task_count: 0, pending_count: 0, completed_count: 0 },
      ])
      return
    }

    // 其它写操作（创建规则、保存 Provider config、replay）默认返回 200 空 data。
    await fulfill(route, { ok: true })
  })
  return page
}

async function assertNoHorizontalOverflow(page, label) {
  const overflow = await page.evaluate(() => ({
    body: document.body.scrollWidth,
    document: document.documentElement.scrollWidth,
    viewport: window.innerWidth,
  }))
  const maxWidth = Math.max(overflow.body, overflow.document)
  if (maxWidth > overflow.viewport + 2) {
    throw new Error(`${label} overflows horizontally: ${JSON.stringify(overflow)}`)
  }
}

async function expectText(page, text) {
  const deadline = Date.now() + 10_000
  const matches = page.getByText(text, { exact: false })
  while (Date.now() < deadline) {
    try {
      const count = await matches.count()
      for (let index = 0; index < count; index += 1) {
        if (await matches.nth(index).isVisible()) {
          return
        }
      }
    } catch {
      // retry
    }
    await page.waitForTimeout(100)
  }
  const bodyText = await page.locator("body").innerText().catch(() => "")
  throw new Error(`cannot find visible text "${text}". Page text: ${bodyText.slice(0, 800)}`)
}

async function screenshot(page, name) {
  await page.screenshot({
    fullPage: true,
    path: path.join(screenshotDir, `${name}.png`),
  })
}

async function fulfill(route, data) {
  await route.fulfill({
    contentType: "application/json",
    body: JSON.stringify({ data }),
  })
}

async function getFreePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer()
    srv.once("error", reject)
    srv.listen(0, "127.0.0.1", () => {
      const address = srv.address()
      srv.close(() => {
        if (address && typeof address === "object") {
          resolve(address.port)
          return
        }
        reject(new Error("cannot allocate port"))
      })
    })
  })
}

async function waitForServer(url) {
  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    if (server.exitCode !== null) {
      throw new Error(`vite exited early\n${serverLog}`)
    }
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(500) })
      if (response.ok) {
        return
      }
    } catch {
      // Vite is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error(`vite did not start\n${serverLog}`)
}

try {
  await main()
} finally {
  server.kill("SIGTERM")
}
