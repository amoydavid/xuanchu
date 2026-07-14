export type MyTasksReturnState = {
  focusId?: string
  scrollTop: number
  selectedIds: string[]
}

const KEY_PREFIX = "xuanchu:my-tasks:return:"

export function saveMyTasksReturnState(
  search: string,
  state: MyTasksReturnState,
  storage: Storage = window.sessionStorage
) {
  const normalized = normalizeState(state)
  if (!normalized) return
  storage.setItem(KEY_PREFIX + search, JSON.stringify(normalized))
}

export function takeMyTasksReturnState(
  search: string,
  storage: Storage = window.sessionStorage
): MyTasksReturnState | undefined {
  const key = KEY_PREFIX + search
  const raw = storage.getItem(key)
  if (raw === null) return undefined
  storage.removeItem(key)
  try {
    return normalizeState(JSON.parse(raw))
  } catch {
    return undefined
  }
}

export function restoreMyTasksReturnState(
  container: HTMLElement,
  state: MyTasksReturnState,
  scrollTo: (x: number, y: number) => void = window.scrollTo.bind(window)
) {
  const candidates = container.querySelectorAll<HTMLElement>(
    "[data-my-task-focus]"
  )
  const target = state.focusId
    ? [...candidates].find(
        (candidate) => candidate.dataset.myTaskFocus === state.focusId
      )
    : undefined
  ;(target ?? container).focus({ preventScroll: true })
  scrollTo(0, state.scrollTop)
}

function normalizeState(value: unknown): MyTasksReturnState | undefined {
  if (!value || typeof value !== "object") return undefined
  const candidate = value as Record<string, unknown>
  const scrollTop = Number(candidate.scrollTop)
  if (!Number.isFinite(scrollTop) || scrollTop < 0) return undefined
  const focusId =
    typeof candidate.focusId === "string" && candidate.focusId.trim()
      ? candidate.focusId
      : undefined
  const selectedIds = Array.isArray(candidate.selectedIds)
    ? candidate.selectedIds.filter(
        (item): item is string => typeof item === "string" && item !== ""
      )
    : []
  return {
    ...(focusId ? { focusId } : {}),
    scrollTop,
    selectedIds: [...new Set(selectedIds)],
  }
}
