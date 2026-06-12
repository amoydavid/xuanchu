import { ApiError, requestJson } from "@/lib/api"

import { clearWorkspaceToken, getWorkspaceToken } from "./workspace-token"

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
  return requestJson<T>({
    body,
    getToken: getWorkspaceToken,
    method,
    onUnauthorized: clearWorkspaceToken,
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
