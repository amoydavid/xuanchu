import { ApiError, requestJson } from "@/lib/api"

import {
  clearAdminActingSession,
  clearAdminActingToken,
  clearWorkspaceToken,
  getAdminActingToken,
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
  // 如果 acting token 存在，401 时只清理 acting session（保留 admin token 和普通 workspace token）。
  const actingToken = getAdminActingToken()
  const usingActing = actingToken !== null
  return requestJson<T>({
    body,
    getToken: usingActing ? getAdminActingToken : getWorkspaceToken,
    method,
    onUnauthorized: usingActing ? clearAdminActingSession : clearWorkspaceToken,
    path,
  })
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

export function workspaceApiDelete<T>(path: string): Promise<T> {
  return workspaceRequest<T>("DELETE", path)
}

// 仅供 workspace-token.ts 之外的内部测试/调试使用：直接清理 acting token（不清 context）。
export const _clearAdminActingTokenForTests = clearAdminActingToken
