import { afterEach, describe, expect, it, vi } from "vitest"
import { act, renderHook } from "@testing-library/react"

import * as attachmentApi from "@/features/workspace/attachments/attachment-api"

import { useDescriptionDraftCleanup } from "./use-description-draft-cleanup"

afterEach(() => {
  vi.restoreAllMocks()
})

describe("useDescriptionDraftCleanup", () => {
  it("tracks and cleans up drafts on cancel", async () => {
    const removeSpy = vi
      .spyOn(attachmentApi, "removeAttachment")
      .mockResolvedValue(undefined)
    const { result } = renderHook(() => useDescriptionDraftCleanup("ws"))
    act(() => {
      result.current.track("a1")
      result.current.track("a2")
    })
    expect(result.current.currentDrafts()).toEqual(["a1", "a2"])
    await act(async () => {
      await result.current.cleanup()
    })
    expect(removeSpy).toHaveBeenCalledTimes(2)
    expect(result.current.currentDrafts()).toEqual([])
  })

  it("reset clears tracked drafts without deleting", async () => {
    const removeSpy = vi
      .spyOn(attachmentApi, "removeAttachment")
      .mockResolvedValue(undefined)
    const { result } = renderHook(() => useDescriptionDraftCleanup("ws"))
    act(() => {
      result.current.track("a1")
      result.current.reset()
    })
    expect(result.current.currentDrafts()).toEqual([])
    await act(async () => {
      await result.current.cleanup()
    })
    expect(removeSpy).not.toHaveBeenCalled()
  })

  it("untrack removes a single draft after save", () => {
    const { result } = renderHook(() => useDescriptionDraftCleanup("ws"))
    act(() => {
      result.current.track("a1")
      result.current.track("a2")
      result.current.untrack("a1")
    })
    expect(result.current.currentDrafts()).toEqual(["a2"])
  })

  it("cleanup does not throw when remove fails", async () => {
    vi.spyOn(attachmentApi, "removeAttachment").mockRejectedValue(
      new Error("network")
    )
    const { result } = renderHook(() => useDescriptionDraftCleanup("ws"))
    act(() => {
      result.current.track("a1")
    })
    await act(async () => {
      await expect(result.current.cleanup()).resolves.toBeUndefined()
    })
  })
})
