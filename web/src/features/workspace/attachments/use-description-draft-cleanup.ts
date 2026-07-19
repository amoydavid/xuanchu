import { useCallback, useRef } from "react"

import type { Attachment } from "@/features/workspace/attachments"
import { removeAttachment } from "@/features/workspace/attachments"

// useDescriptionDraftCleanup 提供「取消编辑时清理本次会话创建的 draft」能力。
//
// spec §5.2：用户取消编辑时，前端尽力删除本次 draft；浏览器中断留下的 draft 由
// 24h janitor 兜底。保存成功后清空「本会话 draft」集合（服务端已原子激活）。
//
// 当前实现是 best-effort：因为 description 中的 attachment 是通过 ref://attachment/{id}
// 引用的，调用方在每次保存/取消后只需调用 reset/cleanup。具体的 draft 收集由
// 上层根据本次编辑会话期间新插入的 attachment 节点决定。
export function useDescriptionDraftCleanup(workspaceSlug: string) {
  const draftsRef = useRef<Set<string>>(new Set())

  // track 把本次会话创建的 attachment ID 加入待清理集合。
  const track = useCallback((attachmentID: string) => {
    draftsRef.current.add(attachmentID)
  }, [])

  // untrack 在保存成功后从待清理集合中移除（服务端已激活，不再需要清理）。
  const untrack = useCallback((attachmentID: string) => {
    draftsRef.current.delete(attachmentID)
  }, [])

  // reset 清空待清理集合（保存成功后调用）。
  const reset = useCallback(() => {
    draftsRef.current.clear()
  }, [])

  // cleanup 取消编辑时尽力删除本次会话创建的 draft。
  // 删除失败不阻塞关闭，由 24h janitor 兜底。
  const cleanup = useCallback(async () => {
    const ids = Array.from(draftsRef.current)
    draftsRef.current.clear()
    await Promise.all(
      ids.map((id) =>
        removeAttachment(workspaceSlug, id).catch(() => {
          // best-effort：失败留给 janitor 兜底。
        })
      )
    )
  }, [workspaceSlug])

  return {
    track,
    untrack,
    reset,
    cleanup,
    // currentDrafts 暴露当前待清理的 attachment ID 快照，便于测试。
    currentDrafts: () => Array.from(draftsRef.current) as Attachment["id"][],
  }
}
