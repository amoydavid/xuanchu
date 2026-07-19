import type { ContentReferenceKey, ContentReferenceResolution } from "./content-reference-api"

// BatchLoaderEntry 描述 microtask batch 的一个 pending 项。
type BatchLoaderEntry = {
  key: ContentReferenceKey
  resolve: (res: ContentReferenceResolution) => void
}

let pending: BatchLoaderEntry[] = []
let scheduled = false

// loadContentReference 在同一 microtask 内合并多个请求为一批。
//
// 同一 (type, id) 在同一批次中只发一次请求，结果共享给所有等待者。
export function loadContentReference(
  key: ContentReferenceKey
): Promise<ContentReferenceResolution> {
  return new Promise<ContentReferenceResolution>((resolve) => {
    pending.push({ key, resolve })
    if (!scheduled) {
      scheduled = true
      void Promise.resolve().then(() => {
        scheduled = false
        const batch = pending
        pending = []
        void runBatch(batch)
      })
    }
  })
}

async function runBatch(batch: BatchLoaderEntry[]): Promise<void> {
  if (batch.length === 0) return
  // 按 type/id 去重，保持输入顺序。
  const seen = new Set<string>()
  const uniqueKeys: ContentReferenceKey[] = []
  for (const entry of batch) {
    const k = `${entry.key.type}:${entry.key.id}`
    if (seen.has(k)) continue
    seen.add(k)
    uniqueKeys.push(entry.key)
  }
  // 截断到 200。
  const capped = uniqueKeys.slice(0, 200)
  // 动态 import 避免循环依赖。
  const { resolveContentReferences } = await import("./content-reference-api")
  let results: ContentReferenceResolution[]
  try {
    results = await resolveContentReferences(capped)
  } catch {
    results = capped.map((k) => ({ type: k.type, id: k.id, status: "unavailable" as const }))
  }
  const byKey = new Map<string, ContentReferenceResolution>()
  results.forEach((r) => byKey.set(`${r.type}:${r.id}`, r))
  for (const entry of batch) {
    const k = `${entry.key.type}:${entry.key.id}`
    entry.resolve(byKey.get(k) ?? { type: entry.key.type, id: entry.key.id, status: "unavailable" })
  }
}

// resetContentReferenceLoader 清空 pending batch（用于测试）。
export function resetContentReferenceLoader(): void {
  pending = []
  scheduled = false
}
