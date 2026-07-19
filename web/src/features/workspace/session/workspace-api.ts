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

// resolveWorkspaceCredential 决定当前请求应使用的 bearer token 与 401 清理回调。
//
// acting token 优先；其次 workspace token；都没有时返回 null，调用方应走 OIDC cookie。
// 所有 JSON/blob/multipart helper 都应通过此函数保持一致的鉴权语义。
function resolveWorkspaceCredential() {
  const actingToken = getAdminActingToken()
  if (actingToken !== null) {
    return {
      token: actingToken,
      onUnauthorized: clearAdminActingSessionAndReturn,
    }
  }
  const tenantContext = getTenantSwitchContext()
  if (tenantContext) {
    return {
      token: getWorkspaceToken(),
      onUnauthorized: clearTenantSwitchSessionAndReturn,
    }
  }
  return {
    token: getWorkspaceToken(),
    onUnauthorized: clearWorkspaceSession,
  }
}

async function workspaceRequest<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  assertWorkspacePath(path)
  const { token, onUnauthorized } = resolveWorkspaceCredential()
  return requestJson<T>({
    body,
    getToken: () => token,
    method,
    onUnauthorized,
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

// workspaceApiBlob 以鉴权方式获取二进制 Blob。
//
// 浏览器原生 <img>、<a download> 无法附带 Authorization header，所以附件内容下载
// 必须走这条流式通道。401 行为与 JSON helper 一致。
export async function workspaceApiBlob(
  path: string,
  signal?: AbortSignal
): Promise<Blob> {
  assertWorkspacePath(path)
  const { token, onUnauthorized } = resolveWorkspaceCredential()
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  const response = await fetch(path, {
    method: "GET",
    headers,
    credentials: "same-origin",
    signal,
  })
  if (!response.ok) {
    if (response.status === 401) {
      onUnauthorized()
    }
    const payload = (await response.json().catch(() => ({}))) as {
      error?: { code?: string }
    }
    const code = payload.error?.code ?? "unknown"
    throw new ApiError(response.status, code, code)
  }
  return response.blob()
}

// WorkspaceMultipartOptions 描述 multipart 上传的进度回调与取消信号。
type WorkspaceMultipartOptions = {
  signal?: AbortSignal
  onProgress?: (sent: number, total: number) => void
}

// workspaceApiMultipart 以 multipart/form-data 上传表单。
//
// 走 XMLHttpRequest 以便支持上传进度回调；token 选择与 401 清理仍与 JSON helper 一致。
export function workspaceApiMultipart<T>(
  path: string,
  body: FormData,
  options?: WorkspaceMultipartOptions
): Promise<T> {
  assertWorkspacePath(path)
  const { token, onUnauthorized } = resolveWorkspaceCredential()
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open("POST", path)
    xhr.withCredentials = true
    if (token) {
      xhr.setRequestHeader("Authorization", `Bearer ${token}`)
    }
    if (options?.signal) {
      options.signal.addEventListener("abort", () => xhr.abort())
    }
    if (options?.onProgress) {
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) {
          options.onProgress!(event.loaded, event.total)
        }
      }
    }
    xhr.onload = () => {
      if (xhr.status === 401) {
        onUnauthorized()
      }
      let payload: { data?: T; error?: { code?: string } } = {}
      try {
        payload = JSON.parse(xhr.responseText) as typeof payload
      } catch {
        payload = {}
      }
      if (xhr.status < 200 || xhr.status >= 300) {
        const code = payload.error?.code ?? "unknown"
        reject(new ApiError(xhr.status, code, code))
        return
      }
      resolve(payload.data as T)
    }
    xhr.onerror = () =>
      reject(new ApiError(0, "network_error", "network_error"))
    xhr.onabort = () =>
      reject(new ApiError(0, "aborted", "aborted"))
    xhr.send(body)
  })
}
