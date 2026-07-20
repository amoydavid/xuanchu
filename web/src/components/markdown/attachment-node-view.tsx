import { NodeViewWrapper } from "@tiptap/react"
import type { NodeViewProps } from "@tiptap/react"
import { useQuery } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api"
import {
  AuthenticatedAttachmentImage,
  formatAttachmentSize,
  getAttachment,
  getAttachmentBlob,
} from "@/features/workspace/attachments"

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
    state?: "resolved" | "loading" | "failed"
    sourceURL?: string
  }
  // workspaceSlug 由编辑器上层通过 extension.storage 注入；为简化，这里要求上层
  // 通过 MarkdownEditor attachmentContext 传入。当前未注入时降级为文本。
  const workspaceSlug = (extension.storage?.workspaceSlug as string | undefined) ?? ""
  const taskRef = (extension.storage?.taskRef as string | undefined) ?? ""

  // 新建任务还没有 task ID，因此本地截图先作为 editor-only preview 节点显示；
  // sourceURL 只能是本地产生的 blob/data URL，远程 URL 不会被拿来直接加载。
  useEffect(() => {
    const sourceURL = attrs.sourceURL ?? ""
    return () => {
      if (sourceURL.startsWith("blob:")) URL.revokeObjectURL(sourceURL)
    }
  }, [attrs.sourceURL])

  if (attrs.state !== "resolved") {
    return (
      <NodeViewWrapper as="span" className="inline-flex flex-col gap-1 rounded border bg-muted/30 p-1">
        {attrs.image && attrs.sourceURL ? (
          <img
            src={attrs.sourceURL}
            alt={attrs.label || t("task.attachments.title")}
            className="max-h-48 max-w-full rounded object-contain"
          />
        ) : null}
        <span className="text-xs text-muted-foreground">
          {attrs.label || t("task.attachments.title")}：等待上传
        </span>
      </NodeViewWrapper>
    )
  }

  if (!workspaceSlug || !taskRef || !attrs.id) {
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
      <AttachmentInlineView workspaceSlug={workspaceSlug} taskRef={taskRef} attrs={attrs} />
    </NodeViewWrapper>
  )
}

// AttachmentNodeContent 根据 metadata 渲染图片或文件卡片；加载中显示 skeleton。
export function AttachmentInlineView({
  workspaceSlug,
  taskRef,
  attrs,
}: {
  workspaceSlug: string
  taskRef: string
  attrs: { id: string; label: string; image?: boolean }
}) {
  const { t } = useTranslation()
  const [downloadFailed, setDownloadFailed] = useState(false)
  const { data: found, isLoading } = useQuery({
    queryKey: ["attachment", workspaceSlug, attrs.id],
    queryFn: () => getAttachment(workspaceSlug, attrs.id),
    enabled: Boolean(workspaceSlug && taskRef && attrs.id),
    retry: false,
  })

  if (isLoading) {
    return <Skeleton className="h-12 w-24 inline-block" />
  }

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

  const download = async () => {
    setDownloadFailed(false)
    try {
      // 浏览器直接导航到 content_url 时，PAT/acting token 无法进入 Authorization
      // header。文件卡片与图片必须使用同一鉴权 blob 通道，不能退化为裸链接。
      const blob = await getAttachmentBlob(workspaceSlug, found.id)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = found.display_name || found.id
      document.body.appendChild(anchor)
      anchor.click()
      document.body.removeChild(anchor)
      window.setTimeout(() => URL.revokeObjectURL(url), 0)
    } catch {
      setDownloadFailed(true)
    }
  }

  return (
    <span className="inline-flex flex-col gap-1">
      <button
        type="button"
        onClick={() => void download()}
        aria-label={found.display_name}
        className="inline-flex items-center gap-1 rounded border px-2 py-1 text-left text-xs hover:bg-muted"
      >
        <span className="font-medium">{found.display_name}</span>
        <span className="text-muted-foreground">
          {found.media_type} · {formatAttachmentSize(found.size_bytes)}
        </span>
      </button>
      {downloadFailed ? (
        <span role="alert" className="text-xs text-destructive">
          {t("task.attachments.load_failed")}
        </span>
      ) : null}
    </span>
  )
}

// 防止 unused import 警告。
export type _PurgeFallback = ApiError
