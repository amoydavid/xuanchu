import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { installWheelScrollIsolation } from "@/lib/scroll-propagation"
import type { ContentReferenceResolution } from "@/features/workspace/content-references"

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
// 仅供 suggestion 插件（reference-suggestion-plugin.ts）调用；菜单本身不再 fetch。
export type FetchSuggestions = (input: {
  kind: ReferenceTriggerKind
  query: string
  signal: AbortSignal
}) => Promise<ReferenceSuggestionMenuItem[]>

// ReferenceSuggestionMenuProps 描述菜单组件的 props。
//
// 数据来源：items / loading / error 由 @tiptap/suggestion 引擎统一获取（内置防抖 +
// AbortSignal），菜单只负责渲染 + 键盘/鼠标交互。定位由父层（MentionFloatingMenu）
// 用 Floating UI 接管。
export type ReferenceSuggestionMenuProps = {
  kind: ReferenceTriggerKind
  query: string
  items: ReferenceSuggestionMenuItem[]
  loading: boolean
  error: string | null
  onSelect: (item: ReferenceSuggestionMenuItem) => void
  onClose: () => void
  // registerKeyHandler 让菜单注册自己的 onKeyDown；suggestion 引擎把方向键/Enter
  // 路由进来（编辑器持有焦点，React 事件不会在菜单上触发）。
  registerKeyHandler?: (handler: (event: KeyboardEvent) => boolean) => void
}

// maxItems 是菜单最多展示数量。
const maxItems = 20

// ReferenceSuggestionMenu 是 @/# 触发后的下拉菜单。
//
// 行为：
// - 数据由 suggestion 引擎统一获取并经 props 传入；菜单不重复 fetch。
// - 支持上下键、Enter、Esc、鼠标点击。
// - 请求失败显示错误文案；空结果显示「无匹配」。
export function ReferenceSuggestionMenu({
  kind,
  query,
  items,
  loading,
  error,
  onSelect,
  onClose,
  registerKeyHandler,
}: ReferenceSuggestionMenuProps) {
  const { t } = useTranslation()
  const menuRef = useRef<HTMLDivElement>(null)
  // activeIndex 用 query 作为 key，让每次新查询（@a → @ab）都从 0 开始，
  // 避免 useEffect 里 setState 重置高亮项。
  const [activeIndex, setActiveIndex] = useState({ query, index: 0 })
  // query 变化时回到 0；index 永远 >= 0，渲染时再按 items.length clamp。
  const effectiveIndex =
    activeIndex.query === query ? activeIndex.index : 0
  const clampedIndex = Math.min(effectiveIndex, Math.max(0, items.length - 1))

  // activeIndexRef 让按键处理器连按方向键时立即读到最新值（state 更新是异步的）。
  // 在 effect 里写 ref，符合 React 19 的 ref 使用规范。
  const activeIndexRef = useRef(0)
  const stateRef = useRef({ items, onSelect, onClose })
  useEffect(() => {
    activeIndexRef.current = clampedIndex
    stateRef.current = { items, onSelect, onClose }
  })

  // 菜单通过 portal 挂到 body；用原生目标监听器隔离 Dialog 的 document 滚动锁。
  useEffect(() => {
    const element = menuRef.current
    if (!element) return
    return installWheelScrollIsolation(element)
  }, [])

  const moveIndex = (delta: number) => {
    const currentItems = stateRef.current.items
    const next = Math.max(0, Math.min(activeIndexRef.current + delta, currentItems.length - 1))
    activeIndexRef.current = next
    setActiveIndex({ query, index: next })
  }

  // 按键处理器只在挂载时注册一次：内部通过 ref 读最新状态。
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent): boolean => {
      const { items: currentItems, onSelect: currentOnSelect, onClose: currentOnClose } = stateRef.current
      if (event.key === "ArrowDown") {
        moveIndex(1)
        return true
      }
      if (event.key === "ArrowUp") {
        moveIndex(-1)
        return true
      }
      if (event.key === "Enter") {
        const item = currentItems[activeIndexRef.current]
        if (item) currentOnSelect(item)
        return true
      }
      if (event.key === "Escape") {
        currentOnClose()
        return true
      }
      return false
    }
    registerKeyHandler?.(handleKeyDown)
    // 仅在挂载时注册一次；handler 通过 ref 读最新状态。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const visibleItems = items.slice(0, maxItems)

  return (
    <div
      ref={menuRef}
      role="listbox"
      aria-label={kind === "user" ? t("task.mentions.userPlaceholder") : t("task.mentions.taskPlaceholder")}
      className="max-h-60 w-72 overflow-auto rounded-md border bg-popover shadow-md"
      tabIndex={-1}
    >
      {error && (
        <div className="px-3 py-2 text-xs text-destructive">{error}</div>
      )}
      {!error && !loading && visibleItems.length === 0 && (
        <div className="px-3 py-2 text-xs text-muted-foreground">
          {t("common.noResults")}
        </div>
      )}
      {loading && (
        <div className="px-3 py-2 text-xs text-muted-foreground">
          {t("common.loading")}
        </div>
      )}
      {visibleItems.map((item, index) => (
        <button
          key={`${item.kind}-${item.id}`}
          type="button"
          role="option"
          aria-selected={index === clampedIndex}
          className={
            "block w-full px-3 py-2 text-left text-xs " +
            (index === clampedIndex ? "bg-accent" : "")
          }
          onMouseEnter={() => {
            activeIndexRef.current = index
            setActiveIndex({ query, index })
          }}
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
