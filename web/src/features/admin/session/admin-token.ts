const adminTokenKey = "xuanchu.console.admin_token"

export function getAdminToken(): string | null {
  return sessionStorage.getItem(adminTokenKey)
}

export function setAdminToken(token: string): void {
  sessionStorage.setItem(adminTokenKey, token)
}

export function clearAdminToken(): void {
  sessionStorage.removeItem(adminTokenKey)
}
