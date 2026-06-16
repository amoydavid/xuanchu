// React 19 + testing-library 需要 act 环境标记。dev build 的 React.act 由
// vite.config.ts 中 test.define 强制 NODE_ENV=development 提供。
globalThis.IS_REACT_ACT_ENVIRONMENT = true

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
