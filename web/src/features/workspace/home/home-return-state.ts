export type HomeReturnState = {
  focusId?: string
  scrollTop: number
}

const KEY_PREFIX = "xuanchu:home:return:"

export function saveHomeReturnState(
  workspaceSlug: string,
  actorID: string,
  state: HomeReturnState,
  storage: Storage = window.sessionStorage
) {
  const normalized = normalizeState(state)
  if (!workspaceSlug || !actorID || !normalized) return
  storage.setItem(stateKey(workspaceSlug, actorID), JSON.stringify(normalized))
}

export function takeHomeReturnState(
  workspaceSlug: string,
  actorID: string,
  storage: Storage = window.sessionStorage
): HomeReturnState | undefined {
  if (!workspaceSlug || !actorID) return undefined
  const key = stateKey(workspaceSlug, actorID)
  const raw = storage.getItem(key)
  if (raw === null) return undefined
  storage.removeItem(key)
  try {
    return normalizeState(JSON.parse(raw))
  } catch {
    return undefined
  }
}

export function restoreHomeReturnState(
  container: HTMLElement,
  state: HomeReturnState,
  scrollTo: (x: number, y: number) => void = window.scrollTo.bind(window)
) {
  const candidates = container.querySelectorAll<HTMLElement>(
    "[data-home-task-focus]"
  )
  const target = state.focusId
    ? [...candidates].find(
        (candidate) => candidate.dataset.homeTaskFocus === state.focusId
      )
    : undefined
  ;(target ?? container).focus({ preventScroll: true })
  scrollTo(0, state.scrollTop)
}

function stateKey(workspaceSlug: string, actorID: string) {
  return `${KEY_PREFIX}${encodeURIComponent(workspaceSlug)}:${encodeURIComponent(actorID)}`
}

function normalizeState(value: unknown): HomeReturnState | undefined {
  if (!value || typeof value !== "object") return undefined
  const candidate = value as Record<string, unknown>
  const scrollTop = Number(candidate.scrollTop)
  if (!Number.isFinite(scrollTop) || scrollTop < 0) return undefined
  const focusId =
    typeof candidate.focusId === "string" && candidate.focusId.trim()
      ? candidate.focusId
      : undefined
  return {
    ...(focusId ? { focusId } : {}),
    scrollTop,
  }
}
