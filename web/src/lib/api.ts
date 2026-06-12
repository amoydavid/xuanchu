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
}: RequestJsonOptions): Promise<T> {
  const token = getToken?.()
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
      onUnauthorized?.()
    }
    const code = payload.error?.code || "unknown"
    throw new ApiError(response.status, code, code)
  }
  return payload.data as T
}
