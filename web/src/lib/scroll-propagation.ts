// Radix Dialog 用 react-remove-scroll 锁定 body 滚动：它会拦截发生在锁容器
// （DialogContent）DOM 子树之外的 wheel/touchmove 事件。通过 portal 渲染到
// document.body 的浮层（Popover/自定义 portal 菜单）不在 DialogContent 子树内，
// 因此鼠标在浮层上滚动时会被 preventDefault，列表滚不动。
//
// stopScrollPropagation 在捕获阶段阻止 wheel 冒泡到 document 级别的锁监听器，
// 让浮层自身的 overflow-auto 生效；仅当容器确实可滚动时才拦截，避免短列表
// 吃掉本该滚动 Dialog 的滚轮。
//
// 注意：必须用 nativeEvent.stopImmediatePropagation，而不是合成的 stopPropagation。
// react-remove-scroll 是 document 上的原生监听器，合成事件的 stopPropagation 只作用于
// 合成事件树，拦不住原生监听器；nativeEvent.stopImmediatePropagation 能在同一捕获
// 阶段阻断事件继续到 document。
export function stopScrollPropagation(event: React.WheelEvent<HTMLDivElement>) {
  const el = event.currentTarget
  if (el.scrollHeight > el.clientHeight) {
    event.nativeEvent.stopImmediatePropagation()
    event.nativeEvent.stopPropagation()
  }
}

// installWheelScrollIsolation 直接在可滚动元素上注册原生 wheel 监听器。
//
// React portal 中的合成 onWheelCapture 不保证先于 react-remove-scroll 的 document
// 监听器执行；真实浏览器中会出现测试环境通过但滚轮仍被 preventDefault 的情况。
// 原生目标监听器一定先于 document 冒泡监听器执行；这里直接更新 scrollTop，
// 再取消默认行为并停止传播，彻底绕开 Dialog 滚动锁对浏览器默认滚动的取消。
export function installWheelScrollIsolation(element: HTMLElement): () => void {
  const handleWheel = (event: WheelEvent) => {
    if (element.scrollHeight > element.clientHeight) {
      const delta =
        event.deltaMode === 1
          ? event.deltaY * 16
          : event.deltaMode === 2
            ? event.deltaY * element.clientHeight
            : event.deltaY
      const maxScrollTop = element.scrollHeight - element.clientHeight
      element.scrollTop = Math.max(
        0,
        Math.min(maxScrollTop, element.scrollTop + delta)
      )
      event.preventDefault()
      event.stopPropagation()
    }
  }
  element.addEventListener("wheel", handleWheel, { passive: false })
  return () => {
    element.removeEventListener("wheel", handleWheel)
  }
}
