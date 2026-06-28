export function navigateToDocument(path: string) {
  if (typeof window === "undefined" || !window.location) {
    return
  }
  window.location.assign(path)
}
