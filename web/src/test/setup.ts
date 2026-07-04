// React 19 + testing-library 需要 act 环境标记。dev build 的 React.act 由
// vite.config.ts 中 test.define 强制 NODE_ENV=development 提供。
;(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false
}

if (!Element.prototype.setPointerCapture) {
  Element.prototype.setPointerCapture = () => undefined
}

if (!Element.prototype.releasePointerCapture) {
  Element.prototype.releasePointerCapture = () => undefined
}

if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => undefined
}

if (!document.elementFromPoint) {
  document.elementFromPoint = () => document.body
}

function emptyDOMRectList(): DOMRectList {
  const rects: DOMRect[] = []
  return {
    item: () => null,
    length: 0,
    [Symbol.iterator]: () => rects[Symbol.iterator](),
  }
}

if (!Element.prototype.getClientRects) {
  Element.prototype.getClientRects = () => emptyDOMRectList()
}

if (!Range.prototype.getClientRects) {
  Range.prototype.getClientRects = () => emptyDOMRectList()
}

if (!Range.prototype.getBoundingClientRect) {
  Range.prototype.getBoundingClientRect = () => new DOMRect()
}

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class ResizeObserver {
    disconnect() {}
    observe() {}
    unobserve() {}
  }
}

Object.defineProperty(window, "scrollTo", {
  configurable: true,
  value: () => undefined,
  writable: true,
})
