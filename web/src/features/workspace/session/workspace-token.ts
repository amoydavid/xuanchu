const tokenKey = "xuanchu.console.token"

export function getWorkspaceToken(): string | null {
  return sessionStorage.getItem(tokenKey)
}

export function setWorkspaceToken(token: string): void {
  sessionStorage.setItem(tokenKey, token)
}

export function clearWorkspaceToken(): void {
  sessionStorage.removeItem(tokenKey)
}
