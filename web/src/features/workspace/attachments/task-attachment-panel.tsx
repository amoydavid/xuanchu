import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { DownloadIcon, Trash2Icon, UploadIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { ApiError } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"

import {
  AuthenticatedAttachmentImage,
  type Attachment,
  formatAttachmentSize,
  getAttachmentBlob,
  listTaskAttachments,
  removeAttachment,
  renameAttachment,
  uploadTaskAttachment,
} from "@/features/workspace/attachments"

type TaskAttachmentPanelProps = {
  workspaceSlug: string
  taskRef: string
  canWrite: boolean
  embeddedImageAttachmentIDs?: ReadonlySet<string>
}

const ATTACHMENTS_KEY = "task-attachments"

// useTaskAttachments 返回 task 附件列表 query。
export function useTaskAttachments(workspaceSlug: string, taskRef: string) {
  return useQuery({
    queryKey: [ATTACHMENTS_KEY, workspaceSlug, taskRef],
    queryFn: () => listTaskAttachments(workspaceSlug, taskRef),
  })
}

export function TaskAttachmentPanel({
  workspaceSlug,
  taskRef,
  canWrite,
  embeddedImageAttachmentIDs = new Set(),
}: TaskAttachmentPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: attachments, isLoading } = useTaskAttachments(
    workspaceSlug,
    taskRef
  )
  const [error, setError] = useState<string | null>(null)
  const [renamingID, setRenamingID] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState("")
  const visibleAttachments = standaloneAttachments(
    attachments ?? [],
    embeddedImageAttachmentIDs
  )

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: [ATTACHMENTS_KEY, workspaceSlug, taskRef],
    })

  const upload = useMutation({
    mutationFn: async (files: FileList) => {
      setError(null)
      const results: Promise<Attachment>[] = []
      for (const file of Array.from(files)) {
        results.push(
          uploadTaskAttachment(workspaceSlug, taskRef, {
            file,
            mode: "attachment",
          })
        )
      }
      await Promise.all(results)
    },
    onSuccess: invalidate,
    onError: (err: unknown) => {
      setError(extractErrorMessage(err, t))
    },
  })

  const rename = useMutation({
    mutationFn: async ({ id, name }: { id: string; name: string }) =>
      renameAttachment(workspaceSlug, id, name),
    onSuccess: () => {
      setRenamingID(null)
      invalidate()
    },
    onError: (err: unknown) => setError(extractErrorMessage(err, t)),
  })

  const remove = useMutation({
    mutationFn: async (id: string) => removeAttachment(workspaceSlug, id),
    onSuccess: invalidate,
    onError: (err: unknown) => setError(extractErrorMessage(err, t)),
  })

  const onDownload = async (attachment: Attachment) => {
    try {
      const blob = await getAttachmentBlob(workspaceSlug, attachment.id)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = attachment.display_name || attachment.id
      document.body.appendChild(anchor)
      anchor.click()
      document.body.removeChild(anchor)
      URL.revokeObjectURL(url)
    } catch (err) {
      setError(extractErrorMessage(err, t))
    }
  }

  const onFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    if (event.target.files && event.target.files.length > 0) {
      upload.mutate(event.target.files)
      event.target.value = ""
    }
  }

  return (
    <section className="space-y-3" data-testid="task-attachments-section">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">
          {t("task.attachments.title")}
        </h3>
        {canWrite && (
          <label className="inline-flex cursor-pointer items-center gap-1 text-xs text-primary">
            <UploadIcon className="size-3.5" />
            <span>{t("task.attachments.add")}</span>
            <input
              type="file"
              multiple
              className="hidden"
              onChange={onFileChange}
              aria-label={t("task.attachments.add")}
            />
          </label>
        )}
      </div>
      {error && (
        <div
          role="alert"
          className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive"
        >
          {error}
        </div>
      )}
      {isLoading ? (
        <Skeleton className="h-12 w-full" />
      ) : !attachments || attachments.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("task.attachments.empty")}
        </p>
      ) : visibleAttachments.length > 0 ? (
        <ul className="space-y-2">
          {visibleAttachments.map((attachment) => (
            <li
              key={attachment.id}
              className="flex items-start justify-between gap-3 rounded-md border px-3 py-2"
              data-testid={`attachment-row-${attachment.id}`}
            >
              <div className="space-y-1">
                {attachment.inline_capable ? (
                  <AuthenticatedAttachmentImage
                    workspaceSlug={workspaceSlug}
                    attachment={attachment}
                    alt={attachment.display_name}
                    className="max-h-32 max-w-full"
                  />
                ) : null}
                {renamingID === attachment.id ? (
                  <form
                    className="flex items-center gap-1"
                    onSubmit={(event) => {
                      event.preventDefault()
                      rename.mutate({ id: attachment.id, name: renameValue })
                    }}
                  >
                    <Input
                      value={renameValue}
                      onChange={(event) => setRenameValue(event.target.value)}
                      className="h-7 text-xs"
                    />
                    <Button type="submit" size="sm" variant="ghost">
                      {t("common.save")}
                    </Button>
                  </form>
                ) : (
                  <button
                    type="button"
                    className="block text-left text-sm font-medium"
                    onClick={() => {
                      if (!canWrite) return
                      setRenamingID(attachment.id)
                      setRenameValue(attachment.display_name)
                    }}
                  >
                    {attachment.display_name}
                  </button>
                )}
                <p className="text-xs text-muted-foreground">
                  {attachment.media_type} · {formatAttachmentSize(attachment.size_bytes)}
                </p>
              </div>
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => onDownload(attachment)}
                  aria-label={t("task.attachments.download")}
                >
                  <DownloadIcon className="size-3.5" />
                </Button>
                {canWrite && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => remove.mutate(attachment.id)}
                    aria-label={t("task.attachments.remove")}
                  >
                    <Trash2Icon className="size-3.5" />
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  )
}

// embeddedImageAttachmentIDs 只识别描述中实际会渲染为图片的内部 attachment URI。
// 普通链接不算内嵌预览，仍应在附件栏中保留下载入口。
export function embeddedImageAttachmentIDs(markdown: string): ReadonlySet<string> {
  return new Set(
    [...markdown.matchAll(/!\[[^\]]*\]\(ref:\/\/attachment\/([^)]+)\)/g)].map(
      (match) => match[1]
    )
  )
}

// standaloneAttachments 返回需要在附件栏单独展示的附件：只有已在描述中预览的图片会隐藏。
export function standaloneAttachments(
  attachments: Attachment[],
  embeddedIDs: ReadonlySet<string>
): Attachment[] {
  return attachments.filter(
    (attachment) =>
      !attachment.inline_capable || !embeddedIDs.has(attachment.id)
  )
}

function extractErrorMessage(err: unknown, t: (key: string) => string): string {
  if (err instanceof ApiError) {
    if (err.code === "attachment_in_use") {
      return t("task.attachments.inUse")
    }
    return err.code
  }
  if (err instanceof Error) return err.message
  return t("common.error")
}
