import { describe, expect, it } from "vitest"
import { CRON_PRESETS, describeCron, isValidCronShape } from "./cron-schedule"

describe("isValidCronShape", () => {
  it("接受标准 5 段表达式", () => {
    expect(isValidCronShape("0 9 * * 1-5")).toBe(true)
    expect(isValidCronShape("*/15 * * * *")).toBe(true)
    expect(isValidCronShape("0 9 1 * *")).toBe(true)
  })

  it("拒绝字段数量不对", () => {
    expect(isValidCronShape("0 9 * *")).toBe(false)
    expect(isValidCronShape("0 9 * * * *")).toBe(false)
  })

  it("拒绝空串和非法字符", () => {
    expect(isValidCronShape("")).toBe(false)
    expect(isValidCronShape("0 9 # * *")).toBe(false)
  })
})

describe("describeCron", () => {
  it("映射常见预设为中文", () => {
    expect(describeCron("0 9 * * *")).toBe("每天 09:00 触发")
    expect(describeCron("0 9 * * 1-5")).toBe("每工作日 09:00 触发")
    expect(describeCron("0 9 * * 1")).toBe("每周一 09:00 触发")
    expect(describeCron("0 * * * *")).toBe("每小时整点触发")
    expect(describeCron("*/15 * * * *")).toBe("每 15 分钟触发")
    expect(describeCron("0 9 1 * *")).toBe("每月 1 日 09:00 触发")
  })

  it("支持每 N 小时整点（含工作日限定）", () => {
    // 用户实际配置场景：0 */6 * * 1-5 = 工作日每 6 小时整点
    expect(describeCron("0 */6 * * 1-5")).toBe("工作日每 6 小时触发")
    expect(describeCron("0 */6 * * *")).toBe("每 6 小时触发")
    expect(describeCron("0 */6 * * 1")).toBe("周一 每 6 小时触发")
  })

  it("未匹配的非常规表达式回退为原始表达式", () => {
    expect(describeCron("*/20 8-18 * * 1-5")).toBe("*/20 8-18 * * 1-5")
  })

  it("非 5 段表达式回退", () => {
    expect(describeCron("0 9 * *")).toBe("0 9 * *")
  })
})

describe("CRON_PRESETS", () => {
  it("每个预设都能被 describeCron 解读", () => {
    for (const preset of CRON_PRESETS) {
      const desc = describeCron(preset.expr)
      expect(desc).not.toBe(preset.expr)
    }
  })
})
