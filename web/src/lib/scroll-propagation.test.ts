import { describe, expect, it, vi } from "vitest"

import {
  installWheelScrollIsolation,
  stopScrollPropagation,
} from "./scroll-propagation"

// stopScrollPropagation 用于修复 Dialog + portal 浮层的滚轮滚动问题：
// 仅当容器内容溢出（可滚动）时才在原生层 stopImmediatePropagation，
// 否则放行让 Dialog 滚动。必须用 nativeEvent（react-remove-scroll 是原生 document 监听器）。
function makeEvent(scrollHeight: number, clientHeight: number) {
  const stopImmediate = vi.fn()
  const stop = vi.fn()
  const nativeEvent = {
    stopImmediatePropagation: stopImmediate,
    stopPropagation: stop,
  } as unknown as Event
  return {
    event: {
      currentTarget: { scrollHeight, clientHeight } as HTMLDivElement,
      nativeEvent,
    } as unknown as React.WheelEvent<HTMLDivElement>,
    stopImmediate,
    stop,
  }
}

describe("stopScrollPropagation", () => {
  it("可滚动时（scrollHeight > clientHeight）用 nativeEvent 阻止冒泡", () => {
    const { event, stopImmediate, stop } = makeEvent(200, 100)
    stopScrollPropagation(event)
    expect(stopImmediate).toHaveBeenCalled()
    expect(stop).toHaveBeenCalled()
  })

  it("不可滚动时（scrollHeight <= clientHeight）放行给 Dialog", () => {
    const { event, stopImmediate } = makeEvent(80, 100)
    stopScrollPropagation(event)
    expect(stopImmediate).not.toHaveBeenCalled()
  })
})

describe("installWheelScrollIsolation", () => {
  it("在可滚动元素的原生 wheel 目标阶段阻止事件到达 document", () => {
    const element = document.createElement("div")
    Object.defineProperty(element, "scrollHeight", { value: 200 })
    Object.defineProperty(element, "clientHeight", { value: 100 })
    document.body.append(element)
    const documentHandler = vi.fn()
    document.addEventListener("wheel", documentHandler)
    const cleanup = installWheelScrollIsolation(element)
    try {
      const event = new WheelEvent("wheel", {
        bubbles: true,
        cancelable: true,
        deltaY: 40,
      })
      element.dispatchEvent(event)
      expect(documentHandler).not.toHaveBeenCalled()
      expect(element.scrollTop).toBe(40)
      expect(event.defaultPrevented).toBe(true)
    } finally {
      cleanup()
      document.removeEventListener("wheel", documentHandler)
      element.remove()
    }
  })

  it("元素不可滚动时允许 wheel 继续冒泡", () => {
    const element = document.createElement("div")
    Object.defineProperty(element, "scrollHeight", { value: 100 })
    Object.defineProperty(element, "clientHeight", { value: 100 })
    document.body.append(element)
    const documentHandler = vi.fn()
    document.addEventListener("wheel", documentHandler)
    const cleanup = installWheelScrollIsolation(element)
    try {
      element.dispatchEvent(
        new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: 40 })
      )
      expect(documentHandler).toHaveBeenCalledOnce()
    } finally {
      cleanup()
      document.removeEventListener("wheel", documentHandler)
      element.remove()
    }
  })
})
