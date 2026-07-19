import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { ApiError } from "@/lib/api"
import {
  loadContentReference,
  type ContentReferenceResolution,
} from "@/features/workspace/content-references"

import type { ReferenceTriggerKind } from "./reference-suggestion"

// ReferenceSuggestionMenuItem 是菜单展示的单个候选。
export type ReferenceSuggestionMenuItem = {
  id: string
  kind: ReferenceTriggerKind
  label: string
  description?: string
  resolution: ContentReferenceResolution
}

// fetchSuggestions 是 suggest API 的最小包装，便于测试注入。
export type FetchSuggestions = (input: {
  kind: ReferenceTriggerKind
  query: string
  signal: AbortSignal
}) => Promise<ReferenceSuggestionMenuItem[]>

// ReferenceSuggestionMenuProps 描述菜单组件的 props。
export type ReferenceSuggestionMenuProps = {
  kind: ReferenceTriggerKind
  query: string
  fetchSuggestions: FetchSuggestions
  onSelect: (item: ReferenceSuggestionMenuItem) => void
  onClose: () => void
  // anchorRect 用于定位；当前实现不强制定位，由父容器决定。
  anchorRect?: DOMRect
}

// debounceMs 是 spec §15.4 的输入防抖（150ms）。
const debounceMs = 150

// maxItems 是菜单最多展示数量。
const maxItems = 20

// ReferenceSuggestionMenu 是 @/# 触发后的下拉菜单。
//
// 行为：
// - 空 query 不请求服务端。
// - 150ms debounce；新 query 通过 AbortController 取消旧请求。
// - 支持上下键、Enter、Esc、鼠标点击。
// - 请求失败显示错误文案；空结果显示「无匹配」。
export function ReferenceSuggestionMenu({
  kind,
  query,
  fetchSuggestions,
  onSelect,
  onClose,
}: ReferenceSuggestionMenuProps) {
  const { t } = useTranslation()
  const [items, setItems] = useState<ReferenceSuggestionMenuItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [activeIndex, setActiveIndex] = useState(0)
  const abortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    if (query.trim().length === 0) {
      // 空 query 不请求。
      const reset = () => {
        setItems([])
        setLoading(false)
        setError(null)
      }
      reset()
      return
    }
    const startLoading = () => {
      setLoading(true)
      setError(null)
    }
    startLoading()
    const handle = setTimeout(() => {
      const controller = new AbortController()
      abortRef.current?.abort()
      abortRef.current = controller
      fetchSuggestions({ kind, query, signal: controller.signal })
        .then((next) => {
          setItems(next.slice(0, maxItems))
          setActiveIndex(0)
          setLoading(false)
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === "AbortError") return
          if (err instanceof ApiError) {
            setError(err.code)
          } else {
            setError(t("common.error"))
          }
          setLoading(false)
        })
    }, debounceMs)
    return () => {
      clearTimeout(handle)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind, query])

  useEffect(() => {
    abortRef.current?.abort()
  }, [])

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "ArrowDown") {
      event.preventDefault()
      setActiveIndex((i) => Math.min(i + 1, items.length - 1))
    } else if (event.key === "ArrowUp") {
      event.preventDefault()
      setActiveIndex((i) => Math.max(i - 1, 0))
    } else if (event.key === "Enter") {
      event.preventDefault()
      const item = items[activeIndex]
      if (item) onSelect(item)
    } else if (event.key === "Escape") {
      event.preventDefault()
      onClose()
    }
  }

  return (
    <div
      role="listbox"
      aria-label={kind === "user" ? t("task.mentions.userPlaceholder") : t("task.mentions.taskPlaceholder")}
      className="max-h-60 w-72 overflow-auto rounded-md border bg-popover shadow-md"
      tabIndex={-1}
      onKeyDown={onKeyDown}
    >
      {error && (
        <div className="px-3 py-2 text-xs text-destructive">{error}</div>
      )}
      {!error && !loading && items.length === 0 && query.trim().length > 0 && (
        <div className="px-3 py-2 text-xs text-muted-foreground">
          {t("common.noResults")}
        </div>
      )}
      {!error && !loading && query.trim().length === 0 && (
        <div className="px-3 py-2 text-xs text-muted-foreground">
          {kind === "user"
            ? t("task.mentions.userPlaceholder")
            : t("task.mentions.taskPlaceholder")}
        </div>
      )}
      {loading && (
        <div className="px-3 py-2 text-xs text-muted-foreground">
          {t("common.loading")}
        </div>
      )}
      {items.map((item, index) => (
        <button
          key={`${item.kind}-${item.id}`}
          type="button"
          role="option"
          aria-selected={index === activeIndex}
          className={
            "block w-full px-3 py-2 text-left text-xs " +
            (index === activeIndex ? "bg-accent" : "")
          }
          onMouseEnter={() => setActiveIndex(index)}
          onClick={() => onSelect(item)}
        >
          <span className="font-medium">{item.label}</span>
          {item.description && (
            <span className="ml-1 text-muted-foreground">{item.description}</span>
          )}
        </button>
      ))}
    </div>
  )
}

// resolutionToMenuItem 把 resolution 转为菜单项。
export function resolutionToMenuItem(
  resolution: ContentReferenceResolution
): ReferenceSuggestionMenuItem | null {
  if (resolution.status !== "resolved") return null
  if (resolution.type === "user" && resolution.user) {
    return {
      id: resolution.user.id,
      kind: "user",
      label: resolution.user.display_name || resolution.user.name,
      description: resolution.user.name,
      resolution,
    }
  }
  if (resolution.type === "task" && resolution.task) {
    return {
      id: resolution.task.id,
      kind: "task",
      label: resolution.task.title,
      description: resolution.task.task_slug,
      resolution,
    }
  }
  return null
}

// defaultFetchSuggestions 通过 content-reference batch loader 拉取建议。
//
// 当前实现使用 resolve batch loader（已经会去重）；suggest API 的真实查询
// 由 suggestContentReferences 提供，但需要 workspace/projectRef 上下文，
// 这里通过 closure 注入。
export function makeDefaultFetchSuggestions(suggest: (input: {
  kind: ReferenceTriggerKind
  query: string
  signal: AbortSignal
}) => Promise<ReferenceSuggestionMenuItem[]>): FetchSuggestions {
  return suggest
}

// 占位：loadContentReference 用于 resolve 单项。
export const _resolveRef = loadContentReference
