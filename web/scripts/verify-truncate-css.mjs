// 验证 table-fixed + block truncate 是否真正触发文本截断（scrollWidth > clientWidth）。
// 这是回归 2026-07-27 指令摘要列溢出 bug 的最小验证。
import { chromium } from "playwright"

const longText =
  "为新建任务自动指派飞书群组里的负责人，需要先查询项目配置中的默认群组再拉取成员列表并匹配任务 assignee 字段，这是一段很长的指令模板"

const html = `<!DOCTYPE html>
<html><head><meta charset="utf-8">
<script src="https://cdn.tailwindcss.com"></script>
<style>body{font-family:sans-serif;font-size:14px}</style>
</head><body>
<h3>table-layout: auto（旧行为，列被撑开）</h3>
<table class="w-full text-sm border">
  <colgroup><col class="w-[48px]"><col><col class="w-[180px]"><col><col class="w-[48px]"></colgroup>
  <thead><tr class="border-b"><th class="p-2 text-left">状态</th><th class="p-2 text-left">名称</th><th class="p-2 text-left">触发器</th><th class="p-2 text-left">指令摘要</th><th class="p-2 text-left">操作</th></tr></thead>
  <tbody>
    <tr class="border-b" id="row-auto">
      <td class="p-2">●</td>
      <td class="p-2 font-medium"><span class="truncate block">${longText}</span></td>
      <td class="p-2">schedule / 每天 09:30</td>
      <td class="p-2"><span class="block truncate" id="cell-auto">${longText}</span></td>
      <td class="p-2">⋯</td>
    </tr>
  </tbody>
</table>

<h3 style="margin-top:24px">table-layout: fixed（新行为，列宽由 colgroup 决定）</h3>
<table class="w-full table-fixed text-sm border">
  <colgroup><col class="w-[48px]"><col><col class="w-[180px]"><col><col class="w-[48px]"></colgroup>
  <thead><tr class="border-b"><th class="p-2 text-left">状态</th><th class="p-2 text-left">名称</th><th class="p-2 text-left">触发器</th><th class="p-2 text-left">指令摘要</th><th class="p-2 text-left">操作</th></tr></thead>
  <tbody>
    <tr class="border-b" id="row-fixed">
      <td class="p-2">●</td>
      <td class="p-2 font-medium"><span class="truncate block">${longText}</span></td>
      <td class="p-2">schedule / 每天 09:30</td>
      <td class="p-2"><span class="block truncate" id="cell-fixed">${longText}</span></td>
      <td class="p-2">⋯</td>
    </tr>
  </tbody>
</table>
</body></html>`

const browser = await chromium.launch()
const page = await browser.newPage()
await page.setViewportSize({ width: 1280, height: 800 })
await page.setContent(html)

async function measure(selector) {
  return page.$eval(selector, (el) => ({
    clientWidth: el.clientWidth,
    scrollWidth: el.scrollWidth,
    truncated: el.scrollWidth > el.clientWidth,
    tableWidth: el.closest("table").offsetWidth,
  }))
}

const auto = await measure("#cell-auto")
const fixed = await measure("#cell-fixed")

console.log("table-layout: auto  →", auto)
console.log("table-layout: fixed →", fixed)

const fixedTruncates = fixed.truncated === true
const autoDoesNotTruncateProperly = auto.clientWidth > 600 // auto 列被撑得很宽

console.log("")
if (fixedTruncates) {
  console.log("✅ fixed 布局下指令摘要真正截断（scrollWidth > clientWidth）")
} else {
  console.log("❌ fixed 布局下仍未截断，方案有误")
  process.exitCode = 1
}

await browser.close()
