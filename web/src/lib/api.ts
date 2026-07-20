type Envelope<T> = {
  data?: T
  error?: {
    code?: string
    message?: string
  }
}

type RequestJsonOptions = {
  body?: unknown
  getToken?: () => string | null
  method: string
  onUnauthorized?: () => void
  path: string
  signal?: AbortSignal
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

export async function requestJson<T>({
  body,
  getToken,
  method,
  onUnauthorized,
  path,
  signal,
}: RequestJsonOptions): Promise<T> {
  const token = getToken?.()
  const headers: Record<string, string> = {
    Accept: "application/json",
  }
  const hasToken = !!token
  if (hasToken) {
    headers.Authorization = `Bearer ${token}`
  }
  if (body !== undefined) {
    headers["Content-Type"] = "application/json"
  }
  // 无 token 时走 OIDC browser session cookie 模式：
  // - 携带 same-origin cookie（xuanchu_session）
  // - 写操作附加 X-Xuanchu-CSRF（从 xuanchu_csrf cookie 读，double-submit）
  let csrfHeader: string | undefined
  if (!hasToken && method !== "GET" && method !== "HEAD") {
    csrfHeader = readCsrfCookie()
    if (csrfHeader) {
      headers["X-Xuanchu-CSRF"] = csrfHeader
    }
  }
  const response = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    // 有 token 时仍发 same-origin credentials，让 cookie 一并带上（无副作用）；
    // 无 token 时依赖 cookie 鉴权。
    credentials: "same-origin",
    signal,
  })
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>
  if (!response.ok) {
    if (response.status === 401) {
      onUnauthorized?.()
    }
    const code = payload.error?.code || "unknown"
    throw new ApiError(response.status, code, code)
  }
  return payload.data as T
}

// readCsrfCookie 从 xuanchu_csrf cookie 读取 CSRF 明文值（非 HttpOnly，JS 可读）。
export function readCsrfCookie(): string | undefined {
  const match = document.cookie
    .split("; ")
    .find((row) => row.startsWith("xuanchu_csrf="))
  return match?.split("=")[1]
}
