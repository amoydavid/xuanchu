import { ApiError, requestJson } from "@/lib/api"

import { clearAdminToken, getAdminToken } from "./admin-token"

function assertAdminPath(path: string) {
  if (!path.startsWith("/api/v1/admin/")) {
    throw new ApiError(0, "admin_api_path_invalid", "admin_api_path_invalid")
  }
}

async function adminRequest<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  assertAdminPath(path)
  return requestJson<T>({
    body,
    getToken: getAdminToken,
    method,
    onUnauthorized: clearAdminToken,
    path,
  })
}

export function adminApiGet<T>(path: string): Promise<T> {
  return adminRequest<T>("GET", path)
}

export function adminApiPost<T>(path: string, body?: unknown): Promise<T> {
  return adminRequest<T>("POST", path, body)
}

export function adminApiPatch<T>(path: string, body: unknown): Promise<T> {
  return adminRequest<T>("PATCH", path, body)
}

export function adminApiDelete<T>(path: string): Promise<T> {
  return adminRequest<T>("DELETE", path)
}
