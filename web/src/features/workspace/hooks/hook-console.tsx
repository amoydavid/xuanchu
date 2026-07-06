// hook-console 已被出站集成控制台取代。
// 保留导出以兼容旧测试；HookConsole 仅在测试场景下被引用。
// 生产路由（/hooks）现在直接渲染 OutboundConsole。
export function HookConsole() {
  return null
}
