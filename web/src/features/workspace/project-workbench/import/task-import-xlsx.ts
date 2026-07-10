import { read, utils, writeFile, type WorkBook, type WorkSheet } from "xlsx"

import {
  buildTaskImportTemplateRows,
  rowsToTaskImportPayload,
  type TaskImportBuildOptions,
  type TaskImportPayload,
  type TaskImportRow,
} from "./task-import"

const TEMPLATE_SHEET_NAME = "Tasks"
const TEMPLATE_HELP_SHEET_NAME = "字段说明"
const DATE_FORMAT = "yyyy-mm-dd"

export async function parseTaskImportXLSXFile(
  file: File,
  options: TaskImportBuildOptions
): Promise<TaskImportPayload> {
  const data = await file.arrayBuffer()
  const workbook = read(data, { cellDates: true })
  return workbookToTaskImportPayload(workbook, options)
}

export function workbookToTaskImportPayload(
  workbook: WorkBook,
  options: TaskImportBuildOptions
): TaskImportPayload {
  const firstSheetName = workbook.SheetNames[0]
  if (!firstSheetName) {
    throw new Error("xlsx sheet is required")
  }
  const sheet = workbook.Sheets[firstSheetName]
  const rows = utils.sheet_to_json<TaskImportRow>(sheet, {
    defval: "",
    raw: false,
  })
  return rowsToTaskImportPayload(rows, options)
}

export function buildTaskImportTemplateWorkbook(): WorkBook {
  const workbook = utils.book_new()
  const sheet = utils.json_to_sheet(buildTaskImportTemplateRows())
  applyTaskTemplateSheetHints(sheet)
  utils.book_append_sheet(workbook, sheet, TEMPLATE_SHEET_NAME)
  const helpSheet = utils.json_to_sheet(buildTaskImportTemplateHelpRows())
  applyHelpSheetHints(helpSheet)
  utils.book_append_sheet(workbook, helpSheet, TEMPLATE_HELP_SHEET_NAME)
  return workbook
}

export function downloadTaskImportTemplate(filename = "xuanchu-tasks-template.xlsx") {
  writeFile(buildTaskImportTemplateWorkbook(), filename)
}

function buildTaskImportTemplateHelpRows(): TaskImportRow[] {
  const rows: TaskImportRow[] = [
    {
      field: "id",
      required: "否",
      type: "string",
      allowed_values: "任意非空字符串，批次内唯一",
      format: "不要求 UUID 格式；建议短且稳定，如 task-1",
      description: "导入文件内临时 ID，必须是字符串且批次内不重复；不要求 UUID 格式；只用于 blocked_by 引用，导入后不入库。",
      example: "task-1",
    },
    {
      field: "title",
      required: "是",
      type: "string",
      allowed_values: "非空",
      format: "短标题",
      description: "任务标题，短标签。",
      example: "准备素材",
    },
    {
      field: "description",
      required: "否",
      type: "markdown string",
      allowed_values: "任意字符串",
      format: "Markdown",
      description: "详细描述，默认按 Markdown 编写。",
      example: "## 验收标准",
    },
    {
      field: "status",
      required: "否",
      type: "enum",
      allowed_values: "pending, completed, deleted, waiting, recurring",
      format: "默认 pending",
      description: "任务状态。普通导入建议留空或填写 pending。",
      example: "pending",
    },
    {
      field: "priority",
      required: "否",
      type: "enum",
      allowed_values: "H, M, L",
      format: "H=高，M=中，L=低",
      description: "任务优先级。",
      example: "M",
    },
    {
      field: "tags",
      required: "否",
      type: "string list",
      allowed_values: "任意 tag",
      format: "多个值用逗号或换行分隔",
      description: "任务标签。",
      example: "docs,import",
    },
    {
      field: "assignees",
      required: "否",
      type: "string list",
      allowed_values: "workspace 用户名、邮箱或用户 ID",
      format: "多个值用逗号或换行分隔",
      description: "指派人稳定引用；可配合 assignee_display_names 和 assignee_emails 按顺序补充展示姓名和邮箱。",
      example: "alice, bob@example.com",
    },
    {
      field: "assignee_display_names",
      required: "否",
      type: "string list",
      allowed_values: "任意展示姓名",
      format: "多个值用逗号或换行分隔；与 assignees 按顺序配对",
      description: "指派人的展示姓名。用于预检创建缺失用户时写入 display_name，也用于预览；不作为唯一身份凭证。",
      example: "张三, 李四",
    },
    {
      field: "assignee_emails",
      required: "否",
      type: "string list",
      allowed_values: "邮箱地址",
      format: "多个值用逗号或换行分隔；与 assignees 按顺序配对",
      description: "指派人的邮箱。用于匹配已有用户或创建缺失用户；比邮箱前缀更可靠。",
      example: "alice@example.com, bob@example.com",
    },
    {
      field: "blocked_by",
      required: "否",
      type: "string list",
      allowed_values: "本文件内 id/import_id 或当前项目已有任务 UUID",
      format: "多个值用逗号或换行分隔",
      description: "当前任务被哪些任务阻塞；填写本文件内 id 或已有任务 UUID，多个值用逗号分隔。",
      example: "task-1, 00000000-0000-0000-0000-000000000001",
    },
    {
      field: "due",
      required: "否",
      type: "date",
      allowed_values: "有效日期",
      format: DATE_FORMAT,
      description: "截止日期。",
      example: "2026-07-01",
    },
    {
      field: "wait",
      required: "否",
      type: "date",
      allowed_values: "有效日期",
      format: DATE_FORMAT,
      description: "等待到该日期后才进入可执行状态。",
      example: "2026-07-01",
    },
    {
      field: "scheduled",
      required: "否",
      type: "date",
      allowed_values: "有效日期",
      format: DATE_FORMAT,
      description: "计划开始日期。",
      example: "2026-07-01",
    },
    {
      field: "until",
      required: "否",
      type: "date",
      allowed_values: "有效日期",
      format: DATE_FORMAT,
      description: "任务有效期截止日期。",
      example: "2026-07-01",
    },
    {
      field: "start",
      required: "否",
      type: "date",
      allowed_values: "有效日期",
      format: DATE_FORMAT,
      description: "任务开始日期。普通导入通常留空。",
      example: "2026-07-01",
    },
    {
      field: "end",
      required: "否",
      type: "date",
      allowed_values: "有效日期",
      format: DATE_FORMAT,
      description: "任务结束日期。普通导入通常留空。",
      example: "2026-07-01",
    },
    {
      field: "recur",
      required: "否",
      type: "string",
      allowed_values: "Taskwarrior recurrence 表达式",
      format: "如 weekly、monthly",
      description: "循环任务规则。",
      example: "weekly",
    },
    {
      field: "parent",
      required: "否",
      type: "string",
      allowed_values: "父任务 UUID",
      format: "UUID",
      description: "父任务引用。普通导入通常留空。",
      example: "00000000-0000-0000-0000-000000000001",
    },
    {
      field: "task_slug",
      required: "否",
      type: "string",
      allowed_values: "当前 workspace/project 内唯一短标识",
      format: "建议小写字母、数字和连字符",
      description: "任务短标识，便于后续 URL 或 API 引用。",
      example: "ads-setup",
    },
    {
      field: "annotations",
      required: "否",
      type: "JSON array",
      allowed_values: "JSON 数组",
      format: `[{"description":"备注","entry":"2026-07-01"}]`,
      description: "任务注解。简单导入可留空。",
      example: `[{"description":"已确认素材"}]`,
    },
    {
      field: "links",
      required: "否",
      type: "JSON array",
      allowed_values: "JSON 数组",
      format: `[{"type":"spec","url":"https://example.com","title":"规格"}]`,
      description: "任务关联链接。简单导入可留空。",
      example: `[{"type":"spec","url":"https://example.com"}]`,
    },
    {
      field: "uda.estimate",
      required: "否",
      type: "string",
      allowed_values: "按 workspace UDA schema",
      format: "示例扩展字段，可按需要改名或新增 uda.* 列",
      description: "估算工作量示例字段。实际含义取决于 workspace UDA schema。",
      example: "3",
    },
    {
      field: "uda.*",
      required: "否",
      type: "string",
      allowed_values: "按 workspace UDA schema",
      format: "列名以 uda. 开头，例如 uda.estimate",
      description: "扩展字段；列名以 uda. 开头，例如 uda.estimate。",
      example: "3",
    },
  ]
  return rows
}

function applyTaskTemplateSheetHints(sheet: WorkSheet) {
  sheet["!cols"] = [
    { wch: 18 },
    { wch: 24 },
    { wch: 42 },
    { wch: 14 },
    { wch: 10 },
    { wch: 18 },
    { wch: 28 },
    { wch: 28 },
    { wch: 34 },
    { wch: 30 },
    { wch: 14 },
    { wch: 14 },
    { wch: 14 },
    { wch: 14 },
    { wch: 14 },
    { wch: 14 },
    { wch: 14 },
    { wch: 38 },
    { wch: 18 },
    { wch: 42 },
    { wch: 42 },
    { wch: 14 },
  ]
  sheet["!autofilter"] = { ref: "A1:T2" }
  for (const cell of ["I2", "J2", "K2", "L2", "M2", "N2"]) {
    if (sheet[cell]) {
      sheet[cell].z = DATE_FORMAT
    }
  }
  if (sheet["I2"]) {
    sheet["I2"].t = "d"
    sheet["I2"].v = new Date("2026-07-01T00:00:00.000Z")
    sheet["I2"].z = DATE_FORMAT
  }
}

function applyHelpSheetHints(sheet: WorkSheet) {
  sheet["!cols"] = [
    { wch: 16 },
    { wch: 10 },
    { wch: 18 },
    { wch: 44 },
    { wch: 38 },
    { wch: 64 },
    { wch: 42 },
  ]
  sheet["!autofilter"] = { ref: "A1:G22" }
}
