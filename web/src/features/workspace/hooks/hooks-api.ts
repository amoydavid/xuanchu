// hooks-api 是历史模块，新版 outbound 控制台统一在
// @/features/workspace/outbound/outbound-api 维护 path、DTO 与请求函数。
// 这里保留 re-export 以避免破坏旧导入。
export {
  type Hook,
  type HookCreateInput,
  type HookCreateInput as HookCreateDialogInput,
  type HookDelivery,
  type HookModifyInput,
  createHook,
  deleteHook,
  disableHook,
  enableHook,
  hookDeliveriesPath,
  hookDeliveryReplayPath,
  hookDisablePath,
  hookEnablePath,
  hookPath,
  listHookDeliveries,
  listHooks,
  modifyHook,
  replayHookDelivery,
} from "@/features/workspace/outbound/outbound-api"
