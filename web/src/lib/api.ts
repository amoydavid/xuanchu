import { clearToken, getToken } from "./token"

type Envelope<T> = {
  data?: T
  error?: {
    code?: string
    message?: string
  }
}

export class ApiError extends Error {
  code: string
  status: number

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.code = code
  }
}

async function apiRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    Accept: "application/json",
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  if (body !== undefined) {
    headers["Content-Type"] = "application/json"
  }
  const response = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>
  if (!response.ok) {
    if (response.status === 401) {
      clearToken()
    }
    const code = payload.error?.code || "unknown"
    throw new ApiError(response.status, code, code)
  }
  return payload.data as T
}

export function apiGet<T>(path: string): Promise<T> {
  return apiRequest<T>("GET", path)
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return apiRequest<T>("POST", path, body)
}

export function apiPatch<T>(path: string, body: unknown): Promise<T> {
  return apiRequest<T>("PATCH", path, body)
}

export function apiDelete<T>(path: string): Promise<T> {
  return apiRequest<T>("DELETE", path)
}
