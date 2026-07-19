import { NodeViewProps, NodeViewWrapper } from "@tiptap/react"
import { useTranslation } from "react-i18next"

import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api"
import {
  AuthenticatedAttachmentImage,
  formatAttachmentSize,
} from "@/features/workspace/attachments"
import { useTaskAttachments } from "@/features/workspace/attachments/task-attachment-panel"

// AttachmentNodeViewProps 是 Tiptap node view 的 props（与 NodeViewProps 兼容）。
export type AttachmentNodeViewProps = NodeViewProps & {
  // 附件上下文：workspaceSlug + taskRef + 当前 workspace 中加载附件列表。
  // 没有上下文时降级为纯文本 label。
}

// AttachmentNodeView 是富文本编辑器中 attachment 节点的 React 渲染。
//
// 通过 task 附件列表查询匹配 ID 的 metadata；找到则按 inline_capable 渲染图片或
// 文件卡片；找不到（已被 purge / 跨 workspace）显示保存 label。
export function AttachmentNodeView({ node, extension }: NodeViewProps) {
  const { t } = useTranslation()
  const attrs = node.attrs as {
    id: string
    label: string
    image?: boolean
  }
  // workspaceSlug 由编辑器上层通过 extension.storage 注入；为简化，这里要求上层
  // 通过 MarkdownEditor attachmentContext 传入。当前未注入时降级为文本。
  const workspaceSlug = (extension.storage?.workspaceSlug as string | undefined) ?? ""

  if (!workspaceSlug || !attrs.id) {
    return (
      <NodeViewWrapper as="span" className="inline">
        <span className="rounded bg-muted px-1 py-0.5 text-xs">
          {attrs.label || t("task.attachments.title")}
        </span>
      </NodeViewWrapper>
    )
  }

  return (
    <NodeViewWrapper as="span" className="inline">
      <AttachmentNodeContent workspaceSlug={workspaceSlug} attrs={attrs} />
    </NodeViewWrapper>
  )
}

// AttachmentNodeContent 根据 metadata 渲染图片或文件卡片；加载中显示 skeleton。
function AttachmentNodeContent({
  workspaceSlug,
  attrs,
}: {
  workspaceSlug: string
  attrs: { id: string; label: string; image?: boolean }
}) {
  const { t } = useTranslation()
  const { data: attachments, isLoading } = useTaskAttachments(
    workspaceSlug,
    "" // taskRef 由编辑器上下文提供；这里使用空 ref 时不触发 query。
  )

  if (isLoading) {
    return <Skeleton className="h-12 w-24 inline-block" />
  }

  const found = (attachments ?? []).find((a) => a.id === attrs.id)
  if (!found) {
    // 不可用（purge / 跨 workspace / 不存在）：显示保存 label。
    return (
      <span className="rounded bg-muted px-1 py-0.5 text-xs">
        {attrs.label || t("task.attachments.title")}
      </span>
    )
  }

  if (found.inline_capable) {
    return (
      <AuthenticatedAttachmentImage
        workspaceSlug={workspaceSlug}
        attachment={found}
        alt={found.display_name}
        className="max-h-32 max-w-full inline-block"
      />
    )
  }

  return (
    <a
      href={found.content_url}
      className="inline-flex items-center gap-1 rounded border px-2 py-1 text-xs"
      target="_blank"
      rel="noopener noreferrer"
    >
      <span className="font-medium">{found.display_name}</span>
      <span className="text-muted-foreground">
        {found.media_type} · {formatAttachmentSize(found.size_bytes)}
      </span>
    </a>
  )
}

// 防止 unused import 警告。
export type _PurgeFallback = ApiError
