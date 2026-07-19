import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { ApiError } from "@/lib/api"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

import {
  acquireAttachmentBlob,
  resetAttachmentBlobCache,
} from "./attachment-blob-cache"
import type { Attachment } from "./attachment-api"

export type AuthenticatedAttachmentImageProps = {
  workspaceSlug: string
  attachment: Pick<Attachment, "id" | "sha256" | "display_name">
  alt?: string
  className?: string
}

// AuthenticatedAttachmentImage 通过鉴权 fetch 加载附件图片，
// 再把 blob 转成 object URL 交给真实 <img>。
//
// 直接使用裸 <img src=content_url> 无法在 PAT/acting 模式下携带 Authorization header，
// 因此所有附件图片都必须走此组件。
export function AuthenticatedAttachmentImage({
  workspaceSlug,
  attachment,
  alt,
  className,
}: AuthenticatedAttachmentImageProps) {
  const { t } = useTranslation()
  const [url, setUrl] = useState<string | null>(null)
  const [error, setError] = useState<ApiError | null>(null)

  useEffect(() => {
    let active = true
    let release: (() => void) | null = null
    setUrl(null)
    setError(null)
    acquireAttachmentBlob(workspaceSlug, attachment)
      .then((result) => {
        if (!active) {
          result.release()
          return
        }
        release = result.release
        setUrl(result.url)
      })
      .catch((err: unknown) => {
        if (!active) return
        if (err instanceof ApiError) {
          setError(err)
        } else {
          setError(new ApiError(0, "attachment_load_failed", "attachment_load_failed"))
        }
      })
    return () => {
      active = false
      if (release) release()
    }
  }, [workspaceSlug, attachment.id, attachment.sha256])

  if (error) {
    return (
      <div
        className={cn(
          "flex h-24 w-full items-center justify-center rounded-md bg-muted text-xs text-muted-foreground",
          className
        )}
        role="img"
        aria-label={alt ?? attachment.display_name}
      >
        {error.code === "attachment_content_gone"
          ? t("task.attachments.content_gone")
          : t("task.attachments.load_failed")}
      </div>
    )
  }

  if (!url) {
    return <Skeleton className={cn("h-24 w-full", className)} aria-hidden />
  }

  return (
    <img
      src={url}
      alt={alt ?? attachment.display_name}
      className={cn("rounded-md object-contain", className)}
      loading="lazy"
    />
  )
}

// 重置 cache 的具名导出，便于测试和 session 切换调用。
export { resetAttachmentBlobCache }
