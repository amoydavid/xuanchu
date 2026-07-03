import { ApiError, requestJson } from "@/lib/api"
import { navigateToDocument } from "@/lib/browser-navigation"

import {
  clearAdminActingSession,
  clearTenantSwitchContext,
  clearTenantSwitchSession,
  clearWorkspaceToken,
  getAdminActingToken,
  getTenantSwitchContext,
  getWorkspaceToken,
} from "./workspace-token"

function assertWorkspacePath(path: string) {
  if (!path.startsWith("/api/v1/") || path.startsWith("/api/v1/admin/")) {
    throw new ApiError(
      0,
      "workspace_api_path_invalid",
      "workspace_api_path_invalid"
    )
  }
}

async function workspaceRequest<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  assertWorkspacePath(path)
  // acting mode：优先用 acting token。普通 workspace token 作为 fallback。
  // 如果 acting token 存在，401 时只清理 acting session（保留 admin token 和普通 workspace token），
  // 并跳回 /admin/workspaces，避免无声地降级为普通 workspace 身份继续操作。
  const actingToken = getAdminActingToken()
  const usingActing = actingToken !== null
  const tenantContext = getTenantSwitchContext()
  return requestJson<T>({
    body,
    getToken: usingActing ? getAdminActingToken : getWorkspaceToken,
    method,
    onUnauthorized: usingActing
      ? clearAdminActingSessionAndReturn
      : tenantContext
        ? clearTenantSwitchSessionAndReturn
        : clearWorkspaceSession,
    path,
  })
}

// clearAdminActingSessionAndReturn 清理 acting session 并跳回超管界面。
// 这是 spec §8.2 的过期/失效语义：acting token 失效后不要无声降级，
// 必须让用户回到 /admin/workspaces 并（通过 admin token 仍在 sessionStorage）保持超管登录。
function clearAdminActingSessionAndReturn() {
  clearAdminActingSession()
  navigateToDocument("/admin/workspaces")
}

function clearTenantSwitchSessionAndReturn() {
  const context = getTenantSwitchContext()
  clearTenantSwitchSession()
  navigateToDocument(context?.returnTo ?? "/admin/workspaces")
}

function clearWorkspaceSession() {
  clearWorkspaceToken()
  clearTenantSwitchContext()
}

export function workspaceApiGet<T>(path: string): Promise<T> {
  return workspaceRequest<T>("GET", path)
}

export function workspaceApiPost<T>(path: string, body?: unknown): Promise<T> {
  return workspaceRequest<T>("POST", path, body)
}

export function workspaceApiPatch<T>(path: string, body: unknown): Promise<T> {
  return workspaceRequest<T>("PATCH", path, body)
}

export function workspaceApiPut<T>(path: string, body: unknown): Promise<T> {
  return workspaceRequest<T>("PUT", path, body)
}

export function workspaceApiDelete<T>(path: string): Promise<T> {
  return workspaceRequest<T>("DELETE", path)
}
