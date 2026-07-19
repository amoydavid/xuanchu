import type { ContentReferenceResolution } from "@/features/workspace/content-references"

// ReferenceTriggerKind 表示 @ 用户 / # 任务 两种触发器。
export type ReferenceTriggerKind = "user" | "task"

// ReferenceSuggestionState 描述当前 suggestion 触发的状态。
export type ReferenceSuggestionState = {
  active: boolean
  kind: ReferenceTriggerKind | null
  query: string
  range: { from: number; to: number } | null
}

export const emptyReferenceSuggestionState: ReferenceSuggestionState = {
  active: false,
  kind: null,
  query: "",
  range: null,
}

// DetectReferenceTriggerInput 是 detectReferenceTrigger 的输入。
type DetectReferenceTriggerInput = {
  // textBeforeCaret 是从行首（或上一个空白）到 caret 的文本。
  textBeforeCaret: string
  // composing 表示当前是否处于 IME composing 阶段；composing 时不触发。
  composing: boolean
}

// ReferenceTriggerMatch 是 detectReferenceTrigger 的命中结果。
export type ReferenceTriggerMatch = {
  kind: ReferenceTriggerKind
  query: string
  // startOffset 是触发字符（@ 或 #）在 textBeforeCaret 中的位置。
  startOffset: number
}

// detectReferenceTrigger 在普通文本位置判断是否应该打开 @ / # 菜单。
//
// 规则：
// - composing 阶段不触发（spec §15.4）。
// - 触发字符必须是行首或前一个字符是空白（空格、制表符、换行）或段首，
//   避免在 `a@b.com` 这类文本里误触发。
// - 触发字符后是用户输入的 query（不含空白）。
// - 空 query 也返回 match（菜单可显示「输入关键字搜索」），但 fetch 层不会发请求。
export function detectReferenceTrigger(
  input: DetectReferenceTriggerInput
): ReferenceTriggerMatch | null {
  if (input.composing) return null
  const text = input.textBeforeCaret
  if (text.length === 0) return null
  // 从末尾向前查找最近的 @ 或 #，且其后没有空白。
  for (let i = text.length - 1; i >= 0; i--) {
    const ch = text[i]
    if (ch === " " || ch === "\t" || ch === "\n") {
      // 遇到空白：如果之前已经找到候选，保留；否则空白之后没有触发字符。
      return null
    }
    if (ch === "@" || ch === "#") {
      // 触发字符前必须是行首或空白。
      const prev = i > 0 ? text[i - 1] : ""
      if (i === 0 || prev === " " || prev === "\t" || prev === "\n") {
        const query = text.slice(i + 1)
        return {
          kind: ch === "@" ? "user" : "task",
          query,
          startOffset: i,
        }
      }
      // 触发字符前是非空白（例如 `a@b`），不触发。
      return null
    }
  }
  return null
}

// shouldFetchQuery 判断 query 是否值得发请求。
//
// 空 query 或纯空白不请求（spec §15.4）。
export function shouldFetchQuery(query: string): boolean {
  return query.trim().length >= 1
}

// buildInsertedReferenceMarkdown 把选中的 reference 构造为 markdown link。
export function buildInsertedReferenceMarkdown(input: {
  kind: ReferenceTriggerKind
  id: string
  label: string
}): string {
  const safeLabel = input.label.replace(/[[\]]/g, "")
  return `[${safeLabel}](ref://${input.kind}/${input.id})`
}

// pickSuggestionLabel 从 resolution 中取展示用的 label。
export function pickSuggestionLabel(resolution: ContentReferenceResolution): string {
  if (resolution.status !== "resolved") return ""
  if (resolution.type === "user" && resolution.user) {
    return resolution.user.display_name || resolution.user.name
  }
  if (resolution.type === "task" && resolution.task) {
    if (resolution.task.task_slug) {
      return `#${resolution.task.task_slug} · ${resolution.task.title}`
    }
    return resolution.task.title
  }
  return ""
}

// ReferenceSuggestionTriggerOptions 是 useReferenceSuggestionTrigger 的可调参数。
export type ReferenceSuggestionTriggerOptions = {
  debounceMs?: number
}
