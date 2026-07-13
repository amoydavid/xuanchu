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
const screenshotDir = path.join(tmpdir(), "xuanchu-editing-smoke")
mkdirSync(screenshotDir, { recursive: true })

const server = spawn("pnpm", [
  "exec",
  "vite",
  "--host",
  "127.0.0.1",
  "--port",
  String(port),
], {
  cwd: webRoot,
  env: {
    ...process.env,
    BROWSER: "none",
    VITE_XUANCHU_API_TARGET: "http://127.0.0.1:9",
  },
  stdio: ["ignore", "pipe", "pipe"],
})

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

  console.log(`editing smoke screenshots: ${screenshotDir}`)
}

async function runDesktopSmoke(browser) {
  const page = await newMockedPage(browser, { height: 900, width: 1280 })
  try {
    // 项目根路由显示概览；任务内容在 /tasks 子页面。
    await page.goto(`${baseURL}/workspaces/acme/projects/adsops`)
    await expectText(page, "项目信息")
    await assertNoHorizontalOverflow(page, "desktop project overview")

    await page.goto(`${baseURL}/workspaces/acme/projects/adsops/tasks`)
    await expectText(page, "投放日报")
    await expectText(page, "成员待办")
    await assertNoHorizontalOverflow(page, "desktop project tasks")
    await screenshot(page, "desktop-project")

    await page.goto(`${baseURL}/workspaces/acme/projects/adsops/tasks/ads-1`)
    await expectText(page, "整理素材表现")
    await expectMarkdownSmoke(page)
    await assertNoHorizontalOverflow(page, "desktop task detail")

    // 桌面端：子任务 composer 与右侧属性栏都包含「编辑标签」按钮，限定到属性栏 aside。
    await page
      .getByRole("complementary")
      .getByRole("button", { name: "编辑标签" })
      .click()
    await assertDialogVisible(page, "搜索标签")
    await page.getByRole("textbox", { name: "搜索标签" }).fill("daily")
    await page.getByRole("checkbox", { name: "daily" }).focus()
    await page.keyboard.press("Space")
    await page.getByRole("button", { name: "完成" }).click()
    await expectStatus(page, "已保存")

    await page
      .getByRole("complementary")
      .getByRole("button", { name: "编辑负责人" })
      .click()
    await assertDialogVisible(page, "搜索负责人")
    await page.getByRole("textbox", { name: "搜索负责人" }).fill("alice")
    await page
      .getByRole("checkbox", { name: "Alice alice@example.com" })
      .focus()
    await page.keyboard.press("Space")
    await page.getByRole("button", { name: "完成" }).click()
    await expectStatus(page, "已保存")

    await page.getByRole("button", { name: "编辑链接" }).click()
    await assertDialogVisible(page, "编辑链接")
    await page.getByRole("textbox", { name: "标题" }).fill("新版规格")
    await page.getByRole("button", { name: "保存链接" }).click()

    await page.getByRole("button", { name: "编辑注解" }).click()
    await assertDialogVisible(page, "编辑注解")
    await page.getByLabel("编辑注解内容").fill("## 补充复盘结论\n\n`CPA` 已确认")
    await page.getByRole("button", { name: "保存注解" }).click()
    await page.locator(".markdown-prose h2", { hasText: "补充复盘结论" }).waitFor()

    await page.getByRole("button", { name: "编辑描述" }).click()
    await assertDialogVisible(page, "编辑任务描述")
    await page
      .getByRole("textbox", { name: "任务描述" })
      .fill("# 调整后描述\n\n- 保留预算\n- 检查素材\n\n`channel` 字段已同步")
    await page.getByRole("button", { name: "保存" }).click()
    await page.locator(".markdown-prose h1", { hasText: "调整后描述" }).waitFor()

    await assertNoHorizontalOverflow(page, "desktop interactions")
    await screenshot(page, "desktop-task-detail")
  } finally {
    await page.close()
  }
}

async function runMobileSmoke(browser) {
  const page = await newMockedPage(browser, { height: 812, width: 375 })
  try {
    await page.goto(`${baseURL}/workspaces/acme/projects/adsops/tasks`)
    await expectText(page, "投放日报")
    await page.getByRole("button", { name: "编辑移动任务标题 ads-1" }).click()
    await page.getByLabel("编辑移动任务标题 ads-1").fill("投放日报草稿")
    await page.keyboard.press("Enter")
    await expectStatus(page, "已保存")
    await assertNoHorizontalOverflow(page, "mobile project cards")
    await screenshot(page, "mobile-project")

    await page.goto(`${baseURL}/workspaces/acme/projects/adsops/tasks/ads-1`)
    await expectText(page, "投放日报草稿")
    // 正文 tab：含描述与关联资源（链接）。
    await page.getByRole("tab", { name: "正文" }).click()
    await expectText(page, "新版规格")
    await page.getByRole("button", { name: "编辑链接" }).click()
    await assertDialogVisible(page, "编辑链接")
    await page.keyboard.press("Escape")

    // 活动 tab：含注解与变更历史。
    await page.getByRole("tab", { name: "活动" }).click()
    await expectText(page, "补充复盘结论")
    await page.getByRole("button", { name: "编辑注解" }).click()
    await assertDialogVisible(page, "编辑注解")
    await page.keyboard.press("Escape")

    await page.getByRole("tab", { name: "属性" }).click()
    await page.getByRole("button", { name: "编辑标签" }).click()
    await assertDialogVisible(page, "搜索标签")
    await page.keyboard.press("Escape")

    await assertNoHorizontalOverflow(page, "mobile task detail")
    await screenshot(page, "mobile-task-detail")
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
        actor: { name: "Alice" },
        effective_role: "owner",
        effective_workspace: { slug: "acme" },
        token: { scopes: ["*"], type: "workspace" },
      })
      return
    }

    if (method === "GET" && pathName === "/api/v1/projects/adsops") {
      await fulfill(route, project)
      return
    }

    if (method === "GET" && pathName === "/api/v1/projects/adsops/timeline") {
      await fulfill(route, [
        {
          action: "task.updated",
          actor: user,
          created_at: 1782600000,
          id: "evt-1",
          summary: "Alice 更新了投放日报",
        },
      ])
      return
    }

    if (
      method === "GET" &&
      pathName === "/api/v1/projects/adsops/task-summary"
    ) {
      await fulfill(route, {
        overdue_count: 0,
        overdue_refs: [],
        high_priority_open_count: 0,
        high_priority_open_refs: [],
        wait_ready_count: 0,
        wait_ready_refs: [],
        unassigned_open_count: 0,
        unassigned_open_refs: [],
        workload: [
          {
            label: "Alice",
            open_count: 1,
            overdue_count: 0,
            high_priority_count: 0,
          },
        ],
      })
      return
    }

    if (method === "GET" && pathName === "/api/v1/tasks") {
      // TaskViewPage 格式（spec §17.3）：{items, total, limit, offset, occurrence_mode}
      await fulfill(route, { items: tasks, total: tasks.length, limit: 200, offset: 0, occurrence_mode: "materialized" })
      return
    }

    if (method === "GET" && pathName === "/api/v1/tasks/ads-1") {
      await fulfill(route, task)
      return
    }

    // 子任务列表端点（任务详情页子任务区会请求）：mock 为空数组。
    if (
      method === "GET" &&
      pathName === "/api/v1/tasks/ads-1/children"
    ) {
      await fulfill(route, [])
      return
    }

    if (method === "GET" && pathName === "/api/v1/workspaces/acme/members") {
      await fulfill(route, members)
      return
    }

    if (method === "PATCH" && pathName === "/api/v1/tasks/ads-1") {
      const patch = await request.postDataJSON()
      Object.assign(task, patchToTask(patch))
      await fulfill(route, task)
      return
    }

    if (
      method === "PATCH" &&
      pathName === "/api/v1/tasks/ads-1/links/link-1"
    ) {
      const patch = await request.postDataJSON()
      Object.assign(task.links[0], patch)
      await fulfill(route, task.links[0])
      return
    }

    if (
      method === "PATCH" &&
      pathName === "/api/v1/tasks/ads-1/annotations/ann-1"
    ) {
      const patch = await request.postDataJSON()
      Object.assign(task.annotations[0], patch)
      await fulfill(route, task)
      return
    }

    await route.fulfill({
      contentType: "application/json",
      status: 404,
      body: JSON.stringify({
        error: { code: "not_mocked", message: `${method} ${pathName}` },
      }),
    })
  })
  return page
}

async function assertDialogVisible(page, title) {
  const dialog = page.getByRole("dialog", { name: title })
  await dialog.waitFor({ state: "visible" })
  await assertWithinViewport(dialog, title)
}

async function assertWithinViewport(locator, label) {
  const box = await locator.boundingBox()
  const viewport = locator.page().viewportSize()
  if (!box || !viewport) {
    throw new Error(`${label} has no visible box`)
  }
  if (box.x < -1 || box.y < -1 || box.x + box.width > viewport.width + 1) {
    throw new Error(`${label} is outside viewport: ${JSON.stringify(box)}`)
  }
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
  let lastError
  while (Date.now() < deadline) {
    try {
      const count = await matches.count()
      for (let index = 0; index < count; index += 1) {
        if (await matches.nth(index).isVisible()) {
          return
        }
      }
    } catch (error) {
      lastError = error
    }
    await page.waitForTimeout(100)
  }
  const bodyText = await page.locator("body").innerText().catch(() => "")
  throw new Error(
    `cannot find visible text "${text}". Page text: ${bodyText.slice(0, 800)}`,
    { cause: lastError }
  )
}

async function expectStatus(page, text) {
  const status = page.getByRole("status").filter({ hasText: text })
  await status.waitFor({ state: "visible", timeout: 5_000 })
  await assertWithinViewport(status, `status ${text}`)
  const box = await status.boundingBox()
  if (box && box.y < 48) {
    throw new Error(`status ${text} overlaps sticky header: ${JSON.stringify(box)}`)
  }
}

async function expectMarkdownSmoke(page) {
  await page.locator(".markdown-prose h1", { hasText: "素材复盘" }).waitFor()
  await page.locator(".markdown-prose code", { hasText: "channel" }).waitFor()
  const safeLink = page.locator('.markdown-prose a[href="https://example.com/spec"]')
  await safeLink.waitFor()
  const scriptCount = await page.locator(".markdown-prose script").count()
  if (scriptCount !== 0) {
    throw new Error(`markdown rendered raw script elements: ${scriptCount}`)
  }
  const unsafeLinkCount = await page
    .locator('.markdown-prose a[href^="javascript:"]')
    .count()
  if (unsafeLinkCount !== 0) {
    throw new Error(`markdown rendered unsafe javascript links: ${unsafeLinkCount}`)
  }
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

function patchToTask(patch) {
  const next = { ...patch }
  for (const key of Object.keys(next)) {
    if (key.startsWith("clear_") || key.startsWith("remove_")) {
      delete next[key]
    }
  }
  if (patch.clear_priority) {
    next.priority = null
  }
  if (patch.clear_due) {
    next.due = null
  }
  if (patch.assignees) {
    next.assignees = members.filter((member) =>
      patch.assignees.includes(member.user_id)
    )
  }
  if (patch.clear_assignees) {
    next.assignees = []
  }
  if (patch.clear_depends) {
    next.depends = []
    next.depends_info = []
  }
  if (patch.clear_tags) {
    next.tags = []
  }
  return next
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

const user = {
  email: "alice@example.com",
  external_ids: [],
  id: "user-alice",
  name: "Alice",
}

const members = [
  {
    email: "alice@example.com",
    joined_at: 1782500000,
    modified_at: 1782500000,
    name: "Alice",
    role: "owner",
    user_id: "user-alice",
  },
  {
    email: "bob@example.com",
    joined_at: 1782500000,
    modified_at: 1782500000,
    name: "Bob",
    role: "member",
    user_id: "user-bob",
  },
]

const project = {
  archived_at: null,
  completed_count: 1,
  created_at: 1782500000,
  description: "广告投放项目",
  id: "project-adsops",
  modified_at: 1782600000,
  name: "投放中台",
  pending_count: 2,
  slug: "adsops",
  status: "active",
  task_count: 3,
  workspace_id: "workspace-acme",
}

const task = {
  annotations: [
    {
      description: "初版素材已同步",
      entry: "2026-06-27T10:00:00Z",
      id: "ann-1",
    },
  ],
  assignees: [members[0]],
  blocked_by_info: [],
  depends: ["ads-2"],
  depends_info: [
    {
      task_slug: "ads-2",
      title: "准备素材包",
      uuid: "task-ads-2",
    },
  ],
  description:
    "# 素材复盘\n\n整理素材表现，补充预算和渠道字段。\n\n- 预算字段\n- 渠道字段\n\n`channel`\n\n[安全规格](https://example.com/spec)\n\n<script>alert(1)</script>\n\n[危险链接](javascript:alert(1))",
  due: "2026-07-03T00:00:00Z",
  links: [
    {
      created_at: "2026-06-27T10:00:00Z",
      created_by: user,
      id: "link-1",
      title: "需求规格",
      type: "spec",
      url: "https://example.com/spec",
    },
  ],
  priority: "H",
  project: "adsops",
  status: "pending",
  tags: ["ads", "daily"],
  task_slug: "ads-1",
  title: "投放日报",
  uuid: "task-ads-1",
  budget: "1200",
  channel: "meta",
  launch_date: "2026-07-03",
  reviewed: "true",
}

const tasks = [
  task,
  {
    ...task,
    annotations: [],
    assignees: [members[1]],
    depends: [],
    depends_info: [],
    due: "2026-07-05T00:00:00Z",
    links: [],
    priority: "M",
    tags: ["ads", "creative"],
    task_slug: "ads-2",
    title: "准备素材包",
    uuid: "task-ads-2",
  },
  {
    ...task,
    annotations: [],
    assignees: [],
    depends: [],
    depends_info: [],
    due: null,
    links: [],
    priority: "L",
    status: "completed",
    tags: ["daily"],
    task_slug: "ads-3",
    title: "复盘旧活动",
    uuid: "task-ads-3",
  },
]

try {
  await main()
} finally {
  server.kill("SIGTERM")
}
