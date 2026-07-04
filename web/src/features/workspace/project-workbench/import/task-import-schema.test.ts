import { describe, expect, it } from "vitest"

import { TASK_IMPORT_JSON_SCHEMA_TEXT } from "./task-import-schema"

describe("任务导入 JSON Schema 文案", () => {
  it("使用明确导入语义，不使用模糊兼容性表述", () => {
    expect(TASK_IMPORT_JSON_SCHEMA_TEXT).not.toMatch(/Taskwarrior/i)
    expect(TASK_IMPORT_JSON_SCHEMA_TEXT).not.toContain("风格")

    expect(TASK_IMPORT_JSON_SCHEMA_TEXT).toContain(
      "uuid 是最终任务标识的候选值"
    )
    expect(TASK_IMPORT_JSON_SCHEMA_TEXT).toContain(
      "重复规则字符串；当前导入流程仅原样保存"
    )
  })
})
