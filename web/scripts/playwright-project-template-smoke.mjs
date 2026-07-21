import { spawn } from "node:child_process"
import { mkdirSync, mkdtempSync, rmSync } from "node:fs"
import net from "node:net"
import { tmpdir } from "node:os"
import path from "node:path"
import { fileURLToPath } from "node:url"

import { chromium } from "playwright"

const webRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)))
const projectRoot = path.resolve(webRoot, "..")
const runtimeDir = mkdtempSync(path.join(tmpdir(), "xuanchu-project-template-smoke-"))
const screenshotDir = path.join(tmpdir(), "xuanchu-project-template-smoke-screenshots")
const binary = path.join(runtimeDir, "xuanchu")
const database = path.join(runtimeDir, "xuanchu.db")
const apiPort = await getFreePort()
const apiBaseURL = `http://127.0.0.1:${apiPort}`
const webBaseURL = apiBaseURL

mkdirSync(screenshotDir, { recursive: true })

let apiServer
let token = ""

try {
  await prepareRuntime()
  apiServer = startProcess(
    binary,
    ["server", "--listen", `127.0.0.1:${apiPort}`, "--db", database],
    projectRoot,
    "xuanchu server"
  )
  await waitForHTTP(`${apiBaseURL}/healthz`, apiServer)
  await waitForHTTP(webBaseURL, apiServer)

  const fixture = await seedFixture()
  const browser = await chromium.launch({ headless: true })
  try {
    await runDesktopSmoke(browser, fixture)
    await runMobileSmoke(browser, fixture)
    await removeFixtureMember(fixture)
    await runInstantiationSmoke(browser, fixture)
  } finally {
    await browser.close()
  }

  console.log(`project-template smoke screenshots: ${screenshotDir}`)
} finally {
  await stopProcess(apiServer, "SIGINT")
  await assertPortReleased(apiPort, "xuanchu server")
  rmSync(runtimeDir, { force: true, recursive: true })
}

async function prepareRuntime() {
  // 发布 smoke 使用生产构建并由真实 xuanchu server 提供嵌入式 Console；
  // 这样同时验证前端产物可嵌入，且不受 React StrictMode 的 dev-only 双 effect 影响。
  await runCommand("pnpm", ["exec", "vite", "build"], webRoot)
  await runCommand("go", ["build", "-o", binary, "./cmd/xuanchu"], projectRoot)
  await runCommand(
    binary,
    ["--db", database, "--workspace", "local", "project", "add", "source", "name:模板来源项目"],
    projectRoot
  )
  await runCommand(
    binary,
    ["--db", database, "user", "add", "bob-smoke", "email:bob-smoke@example.test"],
    projectRoot
  )
  await runCommand(
    binary,
    ["--db", database, "--workspace", "local", "member", "add", "bob-smoke", "role:member"],
    projectRoot
  )
  for (const [key, value] of [
    ["agent.provider.base_url", "https://agent.example.test"],
    ["agent.provider.api_key", "source-secret-must-not-leak"],
    ["agent.provider.model", "smoke-model"],
    ["agent.provider.allowed_hosts", '["agent.example.test"]'],
  ]) {
    await runCommand(
      binary,
      ["--db", database, "--workspace", "local", "project", "config", "set", "source", key, value],
      projectRoot
    )
  }
  const raw = await runCommand(
    binary,
    [
      "--db",
      database,
      "--json",
      "--workspace",
      "local",
      "token",
      "create",
      "project-template-smoke",
      "--expires-in",
      "1h",
      "--scope",
      "*",
    ],
    projectRoot
  )
  token = JSON.parse(raw).token
  if (!token) throw new Error(`token create did not return a token: ${raw}`)
}

async function seedFixture() {
  const blocker = await apiJSON("POST", "/api/v1/tasks?workspace=local", {
    title: "历史阻断任务",
    project: "source",
    assignees: ["bob-smoke"],
  })
  const dependent = await apiJSON("POST", "/api/v1/tasks?workspace=local", {
    title: "依赖阻断任务",
    project: "source",
    assignees: ["bob-smoke"],
    depends: [blocker.uuid],
  })
  await apiJSON("POST", `/api/v1/tasks/${encodeURIComponent(blocker.uuid)}/done?workspace=local`)

  for (let index = 1; index <= 50; index += 1) {
    await apiJSON("POST", "/api/v1/tasks?workspace=local", {
      title: `跨页任务 ${String(index).padStart(2, "0")}`,
      project: "source",
    })
  }

  await apiJSON("POST", "/api/v1/task-series?workspace=local", {
    title: "每日模板巡检",
    project: "source",
    recurrence_rule: "daily",
    first_due_date: "2030-01-01",
  })

  await apiJSON("POST", "/api/v1/projects/source/automations?workspace=local", {
    name: "模板自动化",
    enabled: true,
    trigger_type: "schedule",
    trigger_config: {
      schedule_type: "daily_at",
      schedule_value: "09:30",
      timezone: "Asia/Shanghai",
    },
    action: {
      protocol: "chat_completions",
      base_url_config_key: "agent.provider.base_url",
      api_key_config_key: "agent.provider.api_key",
      model_config_key: "agent.provider.model",
      temperature: 0.2,
    },
    context: { include: ["project", "project_config"] },
    instruction_template: "检查新项目初始化状态",
  })

  const taskCandidates = await apiJSON(
    "GET",
    "/api/v1/projects/source/template-candidates/tasks?workspace=local&limit=100&offset=0"
  )
  if (taskCandidates.total !== 51 || taskCandidates.items.length !== 51) {
    throw new Error(
      `fixture task candidates are incomplete: ${JSON.stringify(taskCandidates)}`
    )
  }
  const secondTaskPage = await apiJSON(
    "GET",
    "/api/v1/projects/source/template-candidates/tasks?workspace=local&limit=50&offset=50"
  )
  if (secondTaskPage.items.length !== 1) {
    throw new Error(`fixture does not span two task pages: ${JSON.stringify(secondTaskPage)}`)
  }

  return {
    blocker,
    dependent,
    firstPageTaskTitle: taskCandidates.items[0].title,
    secondPageTaskTitle: secondTaskPage.items[0].title,
    unavailableMemberName: "bob-smoke",
  }
}

async function runDesktopSmoke(browser, fixture) {
  const page = await newAuthenticatedPage(browser, { height: 960, width: 1440 })
  try {
    await openCaptureWizard(page)
    const dialog = page.getByRole("dialog", { name: "保存项目模板" })
    await dialog.getByLabel("稳定 Key").fill("launch-template")
    await dialog.getByLabel("模板名称").fill("发布流程模板")
    await dialog.getByLabel("说明").fill("真实 server 端到端模板")
    await dialog.getByRole("button", { name: /下一步/ }).click()

    await expectVisibleText(dialog, "51 条")
    await ensureChecked(
      dialog.getByRole("checkbox", { name: "选择本页 50 项" })
    )
    await dialog.getByRole("button", { name: "下一页" }).click()
    const crossPageTask = dialog.getByRole("checkbox", {
      name: fixture.secondPageTaskTitle,
    })
    await crossPageTask.waitFor()
    await ensureChecked(crossPageTask)
    await dialog.getByRole("textbox", { name: "搜索任务" }).fill("依赖阻断任务")
    await dialog.getByRole("checkbox", { name: "依赖阻断任务" }).waitFor()

    await activateWithKeyboard(dialog.getByRole("tab", { name: /循环任务/ }))
    await ensureChecked(dialog.getByRole("checkbox", { name: "每日模板巡检" }))
    await activateWithKeyboard(dialog.getByRole("tab", { name: /配置/ }))
    const configPageCheckbox = dialog.getByRole("checkbox", { name: /选择本页 4 项/ })
    await configPageCheckbox.waitFor()
    await ensureChecked(configPageCheckbox)
    await activateWithKeyboard(dialog.getByRole("tab", { name: /自动化/ }))
    await ensureChecked(dialog.getByRole("checkbox", { name: "模板自动化" }))
    await dialog.getByRole("button", { name: /^已选 / }).click()
    const selectedSheet = page.getByRole("dialog", { name: "已选内容" })
    await selectedSheet.getByRole("textbox", { name: "搜索已选内容" }).fill("跨页任务 50")
    await expectVisibleText(selectedSheet, "跨页任务 50")
    await page.keyboard.press("Escape")

    await dialog.getByRole("button", { name: /下一步/ }).click()
    await dialog.getByRole("button", { name: "生成预览" }).click()
    await dialog.getByRole("button", { name: "补选引用任务" }).click()
    await dialog
      .getByRole("button", { name: /^(生成|重新)预览$/ })
      .click()
    await expectVisibleText(dialog, "没有阻断问题")
    await dialog.getByRole("button", { name: /下一步/ }).click()
    await dialog.getByRole("button", { name: "保存模板" }).click()
    await dialog.waitFor({ state: "detached" })

    await page.goto(`${webBaseURL}/settings/project-templates`)
    await expectVisibleText(page, "发布流程模板")
    await expectVisibleText(page, "当前版本")
    await expectVisibleText(page, "v1")
    await page.screenshot({
      fullPage: true,
      path: path.join(screenshotDir, "desktop-template-library.png"),
    })

    const templates = await apiJSON("GET", "/api/v1/project-templates?workspace=local&status=all")
    if (templates.total !== 1 || templates.items[0]?.current_snapshot?.version !== 1) {
      throw new Error(`template version was not persisted: ${JSON.stringify(templates)}`)
    }
    if (templates.items[0].current_snapshot.counts.tasks !== 52) {
      throw new Error(`dependency resolution did not capture 52 tasks: ${JSON.stringify(templates.items[0])}`)
    }
    if (fixture.blocker.uuid === fixture.dependent.uuid) {
      throw new Error("fixture dependency uses the same task identity")
    }
  } finally {
    await page.close()
  }
}

async function runMobileSmoke(browser, fixture) {
  const page = await newAuthenticatedPage(browser, { height: 812, width: 375 })
  try {
    await openCaptureWizard(page)
    const dialog = page.getByRole("dialog", { name: "保存项目模板" })
    await dialog.getByRole("button", { name: /下一步/ }).click()
    await ensureChecked(
      dialog.getByRole("checkbox", { name: "选择本页 50 项" })
    )
    const selectedTrigger = dialog.getByRole("button", { name: /^已选 / })
    await selectedTrigger.waitFor()
    await selectedTrigger.click()
    const selectedSheet = page.getByRole("dialog", { name: "已选内容" })
    await selectedSheet.waitFor()
    const sheetElement = await selectedSheet.elementHandle()
    await page.waitForFunction(
      (element) => {
        const box = element.getBoundingClientRect()
        return box.left <= 2 && box.right >= window.innerWidth - 2
      },
      sheetElement
    )
    const box = await selectedSheet.boundingBox()
    if (!box || box.width < 370 || box.x > 2) {
      throw new Error(`mobile selected-items sheet is not full width: ${JSON.stringify(box)}`)
    }
    await selectedSheet
      .getByRole("textbox", { name: "搜索已选内容" })
      .fill(fixture.firstPageTaskTitle)
    await expectVisibleText(selectedSheet, fixture.firstPageTaskTitle)
    await page.keyboard.press("Escape")
    await selectedSheet.waitFor({ state: "detached" })
    const focusText = await page.evaluate(() => document.activeElement?.textContent ?? "")
    if (!focusText.includes("已选")) {
      throw new Error(`mobile selected-items focus was not restored: ${JSON.stringify(focusText)}`)
    }
    await page.screenshot({
      fullPage: true,
      path: path.join(screenshotDir, "mobile-template-capture.png"),
    })
    await dialog.getByRole("button", { name: "取消" }).click()
  } finally {
    await page.close()
  }
}

async function removeFixtureMember(fixture) {
  await apiJSON(
    "DELETE",
    `/api/v1/workspaces/local/members/${encodeURIComponent(fixture.unavailableMemberName)}`
  )
}

async function runInstantiationSmoke(browser) {
  const page = await newAuthenticatedPage(browser, { height: 960, width: 1440 })
  try {
    await page.goto(`${webBaseURL}/projects`)
    await page.getByRole("button", { name: "从模板创建" }).click()
    const sheet = page.getByRole("dialog", { name: "从模板创建项目" })
    await sheet.getByRole("button", { name: /发布流程模板/ }).click()
    await sheet.getByRole("button", { name: /下一步：项目信息/ }).click()
    await sheet.getByLabel("项目 Slug").fill("newlaunch")
    await sheet.getByLabel("项目名称").fill("新发布项目")
    await sheet.getByLabel("开始日期").fill("2026-08-01")
    await sheet.getByRole("button", { name: "生成预览" }).click()

    await sheet.getByLabel("agent.provider.api_key").fill("new-project-secret")
    const memberResolution = sheet.getByRole("combobox", { name: /处理bob-smoke/i })
    await memberResolution.selectOption({ index: 2 })
    await sheet.getByRole("button", { name: "重新预览" }).click()
    await sheet.getByRole("button", { name: /下一步：确认/ }).click()
    await expectVisibleText(sheet, "自动化创建后保持停用")
    await sheet.getByRole("button", { name: "创建项目" }).click()
    await page.waitForURL(/\/workspaces\/local\/projects\/newlaunch(?:\/|$)/)

    const tasks = await apiJSON(
      "GET",
      "/api/v1/tasks?workspace=local&project=newlaunch&limit=200&occurrence_mode=materialized"
    )
    if (tasks.total !== 52 || tasks.items.length !== 52) {
      throw new Error(`instantiated task counts are wrong: ${JSON.stringify(tasks)}`)
    }
    const dependent = tasks.items.find((item) => item.title === "依赖阻断任务")
    const blocker = tasks.items.find((item) => item.title === "历史阻断任务")
    if (!dependent || !blocker || !dependent.depends.includes(blocker.uuid)) {
      throw new Error(`instantiated task dependency was not remapped: ${JSON.stringify(dependent)}`)
    }
    if (
      dependent.assignees.length !== 1 ||
      dependent.assignees[0].name !== "local"
    ) {
      throw new Error(
        `unavailable member was not replaced with local: ${JSON.stringify(dependent.assignees)}`
      )
    }

    const series = await apiJSON("GET", "/api/v1/task-series?workspace=local&project=newlaunch")
    if (series.total !== 1 || series.items[0]?.status !== "active") {
      throw new Error(`instantiated series is wrong: ${JSON.stringify(series)}`)
    }
    const materializedOccurrences = await apiJSON(
      "GET",
      "/api/v1/tasks?workspace=local&project=newlaunch&task_type=occurrence&occurrence_mode=materialized"
    )
    if (materializedOccurrences.total !== 0) {
      throw new Error(`series occurrence/history was copied: ${JSON.stringify(materializedOccurrences)}`)
    }

    const automations = await apiJSON(
      "GET",
      "/api/v1/projects/newlaunch/automations?workspace=local&all=true"
    )
    if (automations.length !== 1 || automations[0].enabled !== false) {
      throw new Error(`instantiated automation is not disabled: ${JSON.stringify(automations)}`)
    }
    const deliveries = await apiJSON(
      "GET",
      "/api/v1/projects/newlaunch/automation-deliveries?workspace=local"
    )
    if (deliveries.length !== 0) {
      throw new Error(`automation delivery/history was copied: ${JSON.stringify(deliveries)}`)
    }
    await page.screenshot({
      fullPage: true,
      path: path.join(screenshotDir, "desktop-instantiated-project.png"),
    })
  } finally {
    await page.close()
  }
}

async function openCaptureWizard(page) {
  await page.goto(`${webBaseURL}/workspaces/local/projects/source`)
  await expectVisibleText(page, "模板来源项目")
  await page.getByRole("button", { name: "更多操作" }).click()
  await page.getByRole("menuitem", { name: "另存为模板" }).click()
  await page.getByRole("dialog", { name: "保存项目模板" }).waitFor()
}

async function newAuthenticatedPage(browser, viewport) {
  const page = await browser.newPage({ viewport })
  await page.addInitScript(
    ({ rawToken }) => {
      localStorage.setItem("xuanchu.console.language", "zh-CN")
      sessionStorage.setItem("xuanchu.console.token", rawToken)
    },
    { rawToken: token }
  )
  return page
}

async function apiJSON(method, pathname, body) {
  const response = await fetch(`${apiBaseURL}${pathname}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const raw = await response.text()
  let payload
  try {
    payload = raw ? JSON.parse(raw) : {}
  } catch {
    throw new Error(`${method} ${pathname} returned non-JSON ${response.status}: ${raw}`)
  }
  if (!response.ok) {
    throw new Error(`${method} ${pathname} returned ${response.status}: ${raw}`)
  }
  return payload.data
}

async function expectVisibleText(root, text) {
  const locator = root.getByText(text, { exact: false })
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    const count = await locator.count()
    for (let index = 0; index < count; index += 1) {
      if (await locator.nth(index).isVisible()) return
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  const body = await root.locator("body").innerText().catch(async () =>
    root.innerText().catch(() => "")
  )
  throw new Error(
    `cannot find visible text ${JSON.stringify(text)}\nvisible text:\n${body.slice(0, 5000)}`
  )
}

async function ensureChecked(locator) {
  await locator.waitFor()
  if (!(await locator.isChecked())) await locator.click()
  if (!(await locator.isChecked())) {
    throw new Error(`checkbox did not become checked: ${await locator.getAttribute("aria-label")}`)
  }
}

async function activateWithKeyboard(locator) {
  await locator.focus()
  await locator.press("Enter")
  if ((await locator.getAttribute("data-state")) !== "active") {
    throw new Error(`tab did not become active: ${await locator.textContent()}`)
  }
}

function startProcess(command, args, cwd, label, extraEnv = {}) {
  const child = spawn(command, args, {
    cwd,
    detached: true,
    env: { ...process.env, ...extraEnv },
    stdio: ["ignore", "pipe", "pipe"],
  })
  child.label = label
  child.log = ""
  child.stdout.on("data", (chunk) => {
    child.log += chunk.toString()
  })
  child.stderr.on("data", (chunk) => {
    child.log += chunk.toString()
  })
  return child
}

async function runCommand(command, args, cwd) {
  const child = spawn(command, args, {
    cwd,
    env: process.env,
    stdio: ["ignore", "pipe", "pipe"],
  })
  let stdout = ""
  let stderr = ""
  child.stdout.on("data", (chunk) => {
    stdout += chunk.toString()
  })
  child.stderr.on("data", (chunk) => {
    stderr += chunk.toString()
  })
  const code = await new Promise((resolve, reject) => {
    child.once("error", reject)
    child.once("exit", resolve)
  })
  if (code !== 0) {
    throw new Error(`${command} ${args.join(" ")} exited ${code}\n${stdout}${stderr}`)
  }
  return stdout.trim()
}

async function waitForHTTP(url, child) {
  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`${child.label} exited before ${url} was ready\n${child.log}`)
    }
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(500) })
      if (response.ok) return
    } catch {
      // Process is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 200))
  }
  throw new Error(`${child.label} did not become ready at ${url}\n${child.log}`)
}

async function stopProcess(child, signal) {
  if (!child || child.exitCode !== null) return
  try {
    process.kill(-child.pid, signal)
  } catch (error) {
    if (error.code !== "ESRCH") throw error
  }
  if (await waitForExit(child, 5_000)) return
  try {
    process.kill(-child.pid, "SIGKILL")
  } catch (error) {
    if (error.code !== "ESRCH") throw error
  }
  if (!(await waitForExit(child, 5_000))) {
    throw new Error(`${child.label} did not exit after SIGKILL\n${child.log}`)
  }
}

async function waitForExit(child, timeoutMs) {
  if (child.exitCode !== null) return true
  return new Promise((resolve) => {
    const timeout = setTimeout(() => {
      child.removeListener("exit", onExit)
      resolve(false)
    }, timeoutMs)
    function onExit() {
      clearTimeout(timeout)
      resolve(true)
    }
    child.once("exit", onExit)
  })
}

async function assertPortReleased(port, label) {
  const deadline = Date.now() + 5_000
  while (Date.now() < deadline) {
    if (!(await canConnect(port))) return
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error(`${label} left port ${port} listening after cleanup`)
}

async function canConnect(port) {
  return new Promise((resolve) => {
    const socket = net.createConnection({ host: "127.0.0.1", port })
    socket.setTimeout(300)
    socket.once("connect", () => {
      socket.destroy()
      resolve(true)
    })
    socket.once("error", () => resolve(false))
    socket.once("timeout", () => {
      socket.destroy()
      resolve(false)
    })
  })
}

async function getFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer()
    server.once("error", reject)
    server.listen(0, "127.0.0.1", () => {
      const address = server.address()
      server.close(() => {
        if (address && typeof address === "object") resolve(address.port)
        else reject(new Error("cannot allocate a free port"))
      })
    })
  })
}
