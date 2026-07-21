export function isCurrentPreviewRevision(
  requestRevision: number,
  formRevision: number
) {
  return requestRevision === formRevision
}
