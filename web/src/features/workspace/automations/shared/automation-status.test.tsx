import { render } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import {
  DeliveryStatusDot,
  RuleStatusDot,
  deliveryDotColor,
  deliveryStatusLabel,
} from "./automation-status"

describe("deliveryDotColor", () => {
  it("succeeded 落 primary 色，无动画", () => {
    expect(deliveryDotColor("succeeded")).toBe("bg-primary")
  })
  it("dead_lettered 落 destructive 色，无动画", () => {
    expect(deliveryDotColor("dead_lettered")).toBe("bg-destructive")
  })
  it("retry_wait 落琥珀色，无动画", () => {
    expect(deliveryDotColor("retry_wait")).toBe("bg-amber-500")
  })
  it("delivering 落 primary/60 且带呼吸动画 class", () => {
    expect(deliveryDotColor("delivering")).toBe("bg-primary/60 animate-automation-delivering")
  })
  it("queued / 未知 落 muted，无动画", () => {
    expect(deliveryDotColor("queued")).toBe("bg-muted-foreground/40")
  })
})

describe("deliveryStatusLabel", () => {
  it("把状态码翻译成中文", () => {
    expect(deliveryStatusLabel("queued")).toBe("排队中")
    expect(deliveryStatusLabel("delivering")).toBe("投递中")
    expect(deliveryStatusLabel("retry_wait")).toBe("等待重试")
    expect(deliveryStatusLabel("succeeded")).toBe("成功")
    expect(deliveryStatusLabel("dead_lettered")).toBe("失败")
  })
  it("未知状态原样返回", () => {
    expect(deliveryStatusLabel("unknown")).toBe("unknown")
  })
})

describe("DeliveryStatusDot", () => {
  it("delivering 状态渲染含动画 class 的圆点", () => {
    const { container } = render(<DeliveryStatusDot status="delivering" />)
    const dot = container.querySelector("span")
    expect(dot?.className).toContain("animate-automation-delivering")
    expect(dot?.className).toContain("bg-primary/60")
  })
  it("succeeded 状态无动画 class", () => {
    const { container } = render(<DeliveryStatusDot status="succeeded" />)
    const dot = container.querySelector("span")
    expect(dot?.className).not.toContain("animate-automation-delivering")
  })
})

describe("RuleStatusDot", () => {
  it("启用态落 primary 色", () => {
    const { container } = render(<RuleStatusDot enabled={true} />)
    const dot = container.querySelector("span")
    expect(dot?.className).toContain("bg-primary")
  })
  it("停用态落 muted 色", () => {
    const { container } = render(<RuleStatusDot enabled={false} />)
    const dot = container.querySelector("span")
    expect(dot?.className).toContain("bg-muted-foreground/40")
  })
})
