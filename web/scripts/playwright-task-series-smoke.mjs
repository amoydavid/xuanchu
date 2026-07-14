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
const screenshotDir = path.join(tmpdir(), "xuanchu-task-series-smoke")
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
    await runDesktopExecutionSmoke(browser)
    await runMyTasksReturnSmoke(browser)
    await runMobileSmoke(browser)
  } finally {
    await browser.close()
  }
  console.log(`task-series smoke screenshots: ${screenshotDir}`)
}

async function runDesktopExecutionSmoke(browser) {
  const state = createMockState()
  const page = await newMockedPage(browser, { height: 960, width: 1440 }, state)
  try {
    await page.goto(`${baseURL}/workspaces/acme/projects/ops/tasks`)
    await expectText(page, "一次性发布检查")
    await expectText(page, "每日检查投放消耗")
    await expectText(page, "OPS-7")
    await expectText(page, "计划实例")
    await expectText(page, "循环 · 每天")
    const recurringTab = page.getByRole("link", { name: "循环任务" }).first()
    await recurringTab.waitFor()
    await page.getByRole("combobox", { name: "任务类型" }).click()
    await page.getByRole("option", { name: "循环任务" }).click()
    await page.waitForURL(/task_type=occurrence/)
    await assertNoHorizontalOverflow(page, "desktop merged task list")

    await page.getByRole("button", { name: "新建任务选项" }).click()
    await page.getByRole("menuitem", { name: "新建循环任务" }).click()
    const createDialog = page.getByRole("dialog", { name: "新建任务" })
    await createDialog.waitFor()
    if ((await page.getByRole("dialog").count()) !== 1) {
      throw new Error("recurring create flow must expose exactly one dialog")
    }
    await createDialog.getByRole("textbox", { name: "任务标题" }).fill("每周预算复盘")
    await createDialog.getByRole("textbox", { name: "任务内容" }).fill("## 验收\n\n核对预算与异常账户")
    await createDialog.getByRole("combobox", { name: "优先级" }).click()
    await page.getByRole("option", { name: "H" }).click()
    await expectText(createDialog, "循环规则")
    await expectText(createDialog, "首次截止")
    await createDialog.getByRole("button", { name: "取消" }).click()

    await page.getByRole("link", { name: "OPS-7" }).first().click()
    await page.waitForURL(/\/tasks\/ops-7(?:\?|$)/)
    try {
      await page.getByRole("heading", { name: "每日检查投放消耗" }).waitFor()
    } catch (error) {
      await screenshot(page, "desktop-occurrence-detail-failure")
      const body = (await page.locator("body").innerText()).slice(0, 4000)
      throw new Error(
        `occurrence detail did not render at ${page.url()}\n${body}`,
        { cause: error }
      )
    }
    await expectText(page, "循环任务 · 每天")
    await expectText(page, "本次日期")
    await expectText(page, "所属循环任务：每日检查投放消耗")
    await page.getByRole("button", { name: "完成本次" }).waitFor()
    await page.getByRole("button", { name: /更多操作/ }).click()
    await page.getByRole("menuitem", { name: "跳过本次" }).waitFor()
    await page.keyboard.press("Escape")
    await assertNoHorizontalOverflow(page, "materialized occurrence detail")
    await screenshot(page, "desktop-occurrence-detail")

    await page.getByRole("link", { name: /查看循环任务/ }).first().click()
    await page.waitForURL(new RegExp(`/series/${seriesID}`))
    await page.getByTestId("task-series-page").waitFor()
    await expectText(page, "每日检查投放消耗")
    await expectText(page, "未完成实例")

    await page.getByTestId("series-edit-btn").click()
    const editDialog = page.getByRole("dialog", { name: "编辑循环任务" })
    await editDialog.waitFor()
    await expectText(editDialog, "已完成、已跳过或已单独覆盖的字段保持不变")
    await expectText(editDialog, "接下来三次")
    await editDialog.getByRole("button", { name: "取消" }).click()

    await page.getByTestId("series-stop-btn").click()
    const stopDialog = page.getByRole("alertdialog", { name: "停止循环任务" })
    await stopDialog.waitFor()
    await expectText(stopDialog, "停止后不会再生成新任务")
    await expectText(stopDialog, "同时跳过当前 2 条未完成实例")
    await page.waitForTimeout(300)
    await screenshot(page, "desktop-series-stop-confirm")
    await page.getByRole("button", { name: "取消" }).click()

    await page.goBack()
    await page.waitForURL(/\/tasks\/ops-7(?:\?|$)/)
    await expectText(page, "不会影响其它日期")
    await page.goForward()
    await page.waitForURL(new RegExp(`/series/${seriesID}`))
    await page.getByTestId("task-series-page").waitFor()

    const projectedRef = state.projected.id
    await page.goto(
      `${baseURL}/workspaces/acme/projects/ops/tasks/${encodeURIComponent(projectedRef)}`
    )
    await expectText(page, "计划实例")
    await expectText(page, "首次编辑或执行操作后会创建本次任务")
    if ((await page.getByText("OPS-8", { exact: true }).count()) !== 0) {
      throw new Error("projected occurrence must not expose a reserved task slug")
    }
    await page.getByRole("button", { name: "完成本次" }).click()
    await page.waitForURL(/\/tasks\/ops-8(?:\?|$)/)
    await expectText(page, "已完成")
    await page.getByRole("button", { name: "重新打开本次" }).waitFor()
    await expectText(page, "OPS-8")
    if (state.projected.id !== projectedRef) {
      throw new Error("materialization changed stable occurrence id")
    }
    await screenshot(page, "desktop-projected-materialized")
  } finally {
    await page.close()
  }
}

async function runMyTasksReturnSmoke(browser) {
  const state = createMockState()
  const page = await newMockedPage(browser, { height: 800, width: 1280 }, state)
  try {
    const search = "tab=overdue&priority=H&q=review&sort=priority&task_type=occurrence"
    await page.goto(`${baseURL}/my-tasks?${search}`)
    await page.getByRole("heading", { name: "我的任务" }).waitFor()
    for (const tab of ["未完成", "今日到期", "逾期", "无截止", "已完成"]) {
      await page.getByRole("tab", { name: tab }).waitFor()
    }
    const materializedRow = page
      .getByRole("row")
      .filter({ has: page.getByRole("link", { name: /ops-7/i }) })
      .first()
    const rowCheckbox = materializedRow.getByRole("checkbox")
    await rowCheckbox.click()
    await page.evaluate(() => window.scrollTo(0, 640))
    const beforeScroll = await page.evaluate(() => window.scrollY)
    if (beforeScroll < 100) {
      throw new Error(`my-tasks fixture did not become scrollable: ${beforeScroll}`)
    }
    await materializedRow.getByRole("link", { name: /ops-7/i }).click()
    await page.waitForURL(/\/workspaces\/acme\/projects\/ops\/tasks\/ops-7\?from=my-tasks/)
    await page.getByRole("link", { name: "返回我的任务" }).waitFor()

    await page.getByRole("link", { name: /查看循环任务/ }).first().click()
    await page.waitForURL(new RegExp(`/series/${seriesID}`))
    await page.getByTestId("task-series-page").waitFor()
    // series 现为独立 tab，返回任务详情通过浏览器后退。
    await page.goBack()
    await page.waitForURL(/\/tasks\/ops-7\?from=my-tasks/)
    await page.getByRole("link", { name: "返回我的任务" }).click()
    await page.waitForURL(/\/my-tasks\?/)

    const current = new URL(page.url())
    for (const [key, value] of new URLSearchParams(search)) {
      if (current.searchParams.get(key) !== value) {
        throw new Error(`my-tasks search ${key} was not restored: ${current.search}`)
      }
    }
    const restoredRow = page
      .getByRole("row")
      .filter({ has: page.getByRole("link", { name: /ops-7/i }) })
      .first()
    await restoredRow.getByRole("checkbox").waitFor()
    if (!(await restoredRow.getByRole("checkbox").isChecked())) {
      throw new Error("my-tasks selection was not restored")
    }
    await page.waitForFunction(() => window.scrollY >= 100)
    const focusedID = await page.evaluate(
      () => document.activeElement?.getAttribute("data-my-task-focus") ?? ""
    )
    if (focusedID !== state.materialized.id) {
      throw new Error(`my-tasks focus was not restored: ${focusedID}`)
    }
  } finally {
    await page.close()
  }
}

async function runMobileSmoke(browser) {
  const state = createMockState()
  const page = await newMockedPage(browser, { height: 812, width: 375 }, state)
  try {
    await page.goto(`${baseURL}/workspaces/acme/projects/ops/tasks`)
    await expectText(page, "OPS-7")
    // 循环任务现为独立 tab，移动端直接进入 series 列表页（全宽主区，无 Sheet）。
    await page.goto(`${baseURL}/workspaces/acme/projects/ops/series`)
    await page.getByTestId("task-series-page").waitFor()
    await expectText(page, "每日检查投放消耗")
    await assertNoHorizontalOverflow(page, "mobile task-series list")
    await screenshot(page, "mobile-task-series-list")

    await page.goto(
      `${baseURL}/workspaces/acme/projects/ops/tasks/${encodeURIComponent(state.projected.id)}`
    )
    await expectText(page, "计划实例")
    await expectText(page, "只影响")
    await page.getByRole("tab", { name: "属性" }).click()
    await expectText(page, "原循环日期")
    await assertNoHorizontalOverflow(page, "mobile projected detail")
    await screenshot(page, "mobile-projected-detail")
  } finally {
    await page.close()
  }
}

async function newMockedPage(browser, viewport, state) {
  const page = await browser.newPage({ viewport })
  await page.addInitScript(() => {
    localStorage.setItem("xuanchu.console.language", "zh-CN")
    sessionStorage.setItem("xuanchu.console.token", "series-smoke-token")
  })
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const pathname = url.pathname
    const method = request.method()

    if (method === "GET" && pathname === "/api/v1/credentials/current") {
      return fulfill(route, {
        actor_type: "user",
        actor: user,
        effective_role: "owner",
        effective_workspace: { id: "workspace-acme", slug: "acme", name: "Acme" },
        token: { scopes: ["*"], type: "workspace" },
      })
    }
    if (method === "GET" && pathname === "/api/v1/projects/ops") {
      return fulfill(route, project)
    }
    if (method === "GET" && pathname === "/api/v1/projects") {
      return fulfill(route, [project])
    }
    if (method === "GET" && pathname === "/api/v1/projects/ops/timeline") {
      return fulfill(route, [])
    }
    if (method === "GET" && pathname === "/api/v1/projects/ops/task-summary") {
      return fulfill(route, projectSummary)
    }
    if (method === "GET" && pathname === "/api/v1/projects/ops/config/effective") {
      return fulfill(route, [])
    }
    if (method === "GET" && pathname === "/api/v1/config") {
      return fulfill(route, {})
    }
    if (method === "GET" && pathname === "/api/v1/workspaces/acme/members") {
      return fulfill(route, members)
    }
    if (method === "GET" && pathname === "/api/v1/task-series") {
      return fulfill(route, { items: [state.series], total: 1, limit: 20, offset: 0 })
    }
    if (method === "GET" && pathname === `/api/v1/task-series/${seriesID}`) {
      return fulfill(route, state.series)
    }
    if (method === "GET" && pathname === "/api/v1/tasks") {
      const items = url.searchParams.has("assignee")
        ? [
            ...fillerTasks.slice(0, 14),
            state.materialized,
            state.projected,
            ...fillerTasks.slice(14),
          ]
        : [ordinary, state.materialized, state.projected]
      return fulfill(route, {
        items,
        total: items.length,
        limit: 200,
        offset: 0,
        occurrence_mode: url.searchParams.get("due_after") ? "expand" : "materialized",
      })
    }

    const taskMatch = pathname.match(
      /^\/api\/v1\/tasks\/([^/]+?)(?:\/(children|audit|done|start|stop|reopen))?$/
    )
    if (taskMatch) {
      const taskRef = decodeURIComponent(taskMatch[1])
      const action = taskMatch[2]
      const task = taskByRef(state, taskRef)
      if (!task) return reject(route, 404, "task_not_found")
      if (method === "GET" && action === "children") return fulfill(route, [])
      if (method === "GET" && action === "audit") return fulfill(route, [])
      if (method === "GET" && !action) return fulfill(route, task)
      if (method === "POST" && action) {
        if (action === "done") materializeAndComplete(state, task)
        if (action === "reopen") {
          task.status = "pending"
          task.end = null
        }
        if (action === "start") task.start = Math.floor(Date.now() / 1000)
        if (action === "stop") task.start = null
        return fulfill(route, task)
      }
      if (method === "DELETE" && !action) {
        task.status = "deleted"
        return fulfill(route, task)
      }
    }

    return reject(route, 404, "not_mocked", `${method} ${pathname}`)
  })
  return page
}

function taskByRef(state, ref) {
  if ([ordinary.id, ordinary.uuid, ordinary.task_slug].includes(ref)) return ordinary
  if ([state.materialized.id, state.materialized.uuid, state.materialized.task_slug].includes(ref)) {
    return state.materialized
  }
  if ([state.projected.id, state.projected.uuid, state.projected.task_slug].filter(Boolean).includes(ref)) {
    return state.projected
  }
  return fillerTasks.find((task) => [task.id, task.uuid, task.task_slug].includes(ref))
}

function materializeAndComplete(state, task) {
  task.status = "completed"
  task.end = 1784044799
  if (task.recurrence_info?.materialization === "projected") {
    task.uuid = "occurrence-ops-8"
    task.task_slug = "ops-8"
    task.project_seq = 8
    task.entry = 1784040000
    task.modified = 1784040000
    task.recurrence_info.materialization = "materialized"
  }
  state.series.open_occurrences = state.series.open_occurrences.filter(
    (item) => item.id !== task.id
  )
  state.series.recent_completed = [task]
  state.series.open_occurrence_count = state.series.open_occurrences.length
  state.series.completed_count += 1
}

function createMockState() {
  const materialized = occurrence({
    id: materializedRef,
    uuid: "occurrence-ops-7",
    task_slug: "ops-7",
    project_seq: 7,
    recurrenceAt: materializedSlot,
    due: materializedSlot,
    materialization: "materialized",
  })
  const projected = occurrence({
    id: projectedRef,
    uuid: null,
    task_slug: null,
    project_seq: null,
    recurrenceAt: projectedSlot,
    due: projectedSlot,
    materialization: "projected",
  })
  const series = {
    id: seriesID,
    workspace_id: "workspace-acme",
    project_id: "project-ops",
    title: "每日检查投放消耗",
    description: "检查昨日投放消耗、异常账户和预算余额。",
    status: "active",
    recurrence_rule: "daily",
    first_due: materializedSlot,
    until: null,
    priority: "M",
    tags: ["日报", "投放"],
    udas: {},
    assignees: [user],
    open_occurrence_count: 2,
    completed_count: 4,
    skipped_count: 1,
    overdue_count: 1,
    next_recurrence_at: projectedSlot,
    suggested_rule_effective_from: projectedSlot + 86400,
    created_by: user,
    created_at: 1783600000,
    modified_at: 1783900000,
    open_occurrences: [materialized, projected],
    recent_completed: [],
    recent_skipped: [],
  }
  return { materialized, projected, series }
}

function occurrence({ id, uuid, task_slug, project_seq, recurrenceAt, due, materialization }) {
  return {
    id,
    uuid,
    task_slug,
    project_seq,
    workspace_id: "workspace-acme",
    project_id: "project-ops",
    project: "ops",
    title: "每日检查投放消耗",
    description: "检查昨日投放消耗、异常账户和预算余额。",
    status: "pending",
    entry: materialization === "projected" ? null : 1783900000,
    modified: materialization === "projected" ? null : 1783900000,
    due,
    start: null,
    end: null,
    wait: null,
    scheduled: null,
    until: null,
    parent: null,
    priority: "M",
    tags: ["日报", "投放"],
    assignees: [user],
    depends: [],
    depends_info: [],
    blocked_by_info: [],
    annotations: [],
    links: [],
    udas: {},
    recurrence_info: {
      role: "occurrence",
      series_id: seriesID,
      series_title: "每日检查投放消耗",
      series_status: "active",
      rule: "daily",
      recurrence_at: recurrenceAt,
      materialization,
      overrides: [],
      until: null,
    },
  }
}

async function fulfill(route, data) {
  await route.fulfill({
    contentType: "application/json",
    body: JSON.stringify({ data }),
  })
}

async function reject(route, status, code, message = code) {
  await route.fulfill({
    contentType: "application/json",
    status,
    body: JSON.stringify({ error: { code, message } }),
  })
}

async function expectText(root, text) {
  const matches = root.getByText(text, { exact: false })
  const page = typeof root.page === "function" ? root.page() : root
  const deadline = Date.now() + 10_000
  while (Date.now() < deadline) {
    const count = await matches.count()
    for (let index = 0; index < count; index += 1) {
      if (await matches.nth(index).isVisible()) return
    }
    await page.waitForTimeout(100)
  }
  throw new Error(`cannot find visible text ${JSON.stringify(text)}`)
}

async function assertNoHorizontalOverflow(page, label) {
  const dimensions = await page.evaluate(() => ({
    body: document.body.scrollWidth,
    document: document.documentElement.scrollWidth,
    viewport: window.innerWidth,
  }))
  if (Math.max(dimensions.body, dimensions.document) > dimensions.viewport + 2) {
    throw new Error(`${label} overflows horizontally: ${JSON.stringify(dimensions)}`)
  }
}

async function screenshot(page, name) {
  await page.screenshot({ fullPage: true, path: path.join(screenshotDir, `${name}.png`) })
}

async function getFreePort() {
  return new Promise((resolve, rejectPromise) => {
    const socket = net.createServer()
    socket.once("error", rejectPromise)
    socket.listen(0, "127.0.0.1", () => {
      const address = socket.address()
      socket.close(() => {
        if (address && typeof address === "object") resolve(address.port)
        else rejectPromise(new Error("cannot allocate port"))
      })
    })
  })
}

async function waitForServer(url) {
  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    if (server.exitCode !== null) throw new Error(`vite exited early\n${serverLog}`)
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(500) })
      if (response.ok) return
    } catch {
      // Vite is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error(`vite did not start\n${serverLog}`)
}

const user = {
  id: "user-alice",
  name: "alice",
  display_name: "Alice",
  email: "alice@example.com",
  external_ids: [],
}

const members = [
  {
    ...user,
    role: "owner",
    joined_at: 1782500000,
    modified_at: 1782500000,
  },
]

const project = {
  id: "project-ops",
  workspace_id: "workspace-acme",
  slug: "ops",
  name: "投放运营",
  description: "广告投放运营项目",
  status: "active",
  task_count: 1,
  pending_count: 1,
  completed_count: 0,
  created_at: 1782500000,
  modified_at: 1783900000,
}

const projectSummary = {
  overdue_count: 1,
  overdue_refs: ["ops-7"],
  high_priority_open_count: 0,
  high_priority_open_refs: [],
  wait_ready_count: 0,
  wait_ready_refs: [],
  unassigned_open_count: 0,
  unassigned_open_refs: [],
  recurring_series_count: 1,
  active_recurring_series_count: 1,
  open_recurring_occurrence_count: 2,
  overdue_recurring_occurrence_count: 1,
  workload: [],
}

const ordinary = {
  id: "ordinary-ops-6",
  uuid: "ordinary-ops-6",
  task_slug: "ops-6",
  project_seq: 6,
  workspace_id: "workspace-acme",
  project_id: "project-ops",
  project: "ops",
  title: "一次性发布检查",
  description: "发布前检查",
  status: "pending",
  entry: 1783800000,
  modified: 1783800000,
  due: 1783958399,
  start: null,
  end: null,
  priority: "H",
  tags: [],
  assignees: [user],
  depends: [],
  annotations: [],
  links: [],
}

const fillerTasks = Array.from({ length: 28 }, (_, index) => ({
  ...ordinary,
  id: `ordinary-fill-${index}`,
  uuid: `ordinary-fill-${index}`,
  task_slug: `ops-${20 + index}`,
  project_seq: 20 + index,
  title: `历史任务 ${index + 1}`,
}))

const seriesID = "290d56bd-8b9d-44e9-8b76-babbf55c62b1"
const materializedSlot = 1783958399
const projectedSlot = materializedSlot + 86400
const materializedRef = `occ:${seriesID}:${materializedSlot}`
const projectedRef = `occ:${seriesID}:${projectedSlot}`

try {
  await main()
} finally {
  server.kill("SIGTERM")
}
