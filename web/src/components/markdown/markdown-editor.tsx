import Placeholder from "@tiptap/extension-placeholder"
import { DOMParser as ProseMirrorDOMParser } from "@tiptap/pm/model"
import type { EditorView } from "@tiptap/pm/view"
import { EditorContent, useEditor } from "@tiptap/react"
import {
  BoldIcon,
  BracesIcon,
  Code2,
  CodeIcon,
  Eye,
  Heading1Icon,
  Heading2Icon,
  Heading3Icon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  ListTodoIcon,
  QuoteIcon,
  Redo2Icon,
  StrikethroughIcon,
  TableIcon,
  Undo2Icon,
} from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import {
  importTaskAttachmentURL,
  removeAttachment,
  uploadTaskAttachment,
} from "@/features/workspace/attachments"

import {
  markdownExtensionsWithAttachmentContext,
  normalizeMarkdownSource,
} from "./extensions"
import { AttachmentUploadQueue, type AttachmentUploadQueueAPI } from "./attachment-upload-queue"
import { dataURLToFile, sanitizeRichPaste, type PasteImageCandidate } from "./paste-sanitizer"
import { LinkDialog } from "./link-dialog"
import { TableBubbleMenu } from "./table-bubble-menu"
import {
  ReferenceSuggestionMenu,
  type FetchSuggestions,
  type ReferenceSuggestionMenuItem,
} from "./reference-suggestion-menu"
import {
  detectReferenceTrigger,
  type ReferenceSuggestionState,
  type ReferenceTriggerKind,
} from "./reference-suggestion"
import "./markdown.css"

type MarkdownEditorProps = {
  value: string
  onChange: (markdown: string) => void
  placeholder?: string
  ariaLabel?: string
  minHeight?: number
  className?: string
  disabled?: boolean
  onModEnter?: () => void
  onAttachmentPendingChange?: (pending: number) => void
  onAttachmentFailureChange?: (failed: number) => void
  onDraftAttachmentCreated?: (attachmentID: string) => void
  onDeferredAttachment?: (input: DeferredAttachment) => void
  // 附件上下文：taskRef 存在时启用图片上传和 @/# suggestion。
  attachmentContext?: {
    workspaceSlug: string
    taskRef: string
    projectRef?: string
    fetchSuggestions: FetchSuggestions
    attachmentAPI?: AttachmentUploadQueueAPI
  }
}

// DeferredAttachment 保存新建任务尚未拥有 taskRef 时的本地候选；只在内存中存在，
// 任务创建成功后必须上传并把 marker 替换为 canonical ref。
export type DeferredAttachment = {
  candidate: PasteImageCandidate
  marker: string
}

export function replaceDeferredAttachmentMarkers(
  markdown: string,
  attachments: Array<{ marker: string; id: string; alt: string }>
): string {
  return attachments.reduce(
    (next, attachment) =>
      replaceDeferredMarker(
        next,
        attachment.marker,
        `![${attachment.alt}](ref://attachment/${attachment.id})`
      ),
    markdown
  )
}

export function removeDeferredAttachmentMarkers(markdown: string, markers: string[]): string {
  return markers.reduce(
    (next, marker) => replaceDeferredMarker(next, marker, ""),
    markdown
  )
}

// Markdown serializer 会因上下文不同保留原 marker，或将 [] 转义为 \[\]。
// 创建 task 前必须识别两种表示，避免把内部 marker 泄漏进持久化 description。
function replaceDeferredMarker(markdown: string, marker: string, replacement: string): string {
  const escaped = marker.replace(/\[|\]/g, "\\$&")
  return markdown.split(marker).join(replacement).split(escaped).join(replacement)
}

const emptySuggestionState: ReferenceSuggestionState = {
  active: false,
  kind: null,
  query: "",
  range: null,
}

export function MarkdownEditor({
  ariaLabel,
  className,
  disabled = false,
  minHeight = 240,
  onChange,
  onAttachmentPendingChange,
  onAttachmentFailureChange,
  onDraftAttachmentCreated,
  onDeferredAttachment,
  onModEnter,
  placeholder,
  value,
  attachmentContext,
}: MarkdownEditorProps) {
  const emittedMarkdownValuesRef = useRef(new Set<string>())
  const [linkState, setLinkState] = useState<{ open: boolean; initialHref?: string }>(
    { open: false }
  )
  const [mode, setMode] = useState<"wysiwyg" | "source">("wysiwyg")
  const [suggestion, setSuggestion] = useState<ReferenceSuggestionState>(emptySuggestionState)
  const [failedUploads, setFailedUploads] = useState<Array<{ candidate: PasteImageCandidate; marker: string }>>([])
  const composingRef = useRef(false)
  const uploadQueueRef = useRef<AttachmentUploadQueue | null>(null)
  const queueTaskRef = useRef("")
  const editorRef = useRef<ReturnType<typeof useEditor>>(null)
  const pendingUploadsRef = useRef(0)
  const failedMarkersRef = useRef(new Set<string>())

  const replacePasteMarker = useCallback((marker: string, markdown: string) => {
    const currentEditor = editorRef.current
    if (!currentEditor) return
    const current = normalizeMarkdownSource(currentEditor.getMarkdown())
    const serializedMarker = marker.replace(/[\\[\]]/g, "\\$&")
    if (!current.includes(serializedMarker)) return
    currentEditor.commands.setContent(current.replace(serializedMarker, markdown), { contentType: "markdown" })
  }, [])

  const queueCandidate = useCallback((candidate: PasteImageCandidate, marker: string) => {
    if (!attachmentContext?.taskRef) {
      onDeferredAttachment?.({ candidate, marker })
      return
    }
    if (!uploadQueueRef.current || queueTaskRef.current !== attachmentContext.taskRef) {
      const api = attachmentContext.attachmentAPI ?? {
        uploadFile: (_taskRef, item, signal) => uploadTaskAttachment(attachmentContext.workspaceSlug, attachmentContext.taskRef, {
          file: item.file!, mode: "description_draft", displayName: item.alt,
        }, { signal }),
        importRemoteURL: (_taskRef, sourceURL, signal) => importTaskAttachmentURL(
          attachmentContext.workspaceSlug,
          attachmentContext.taskRef,
          { sourceURL, mode: "description_draft" },
          { signal }
        ),
        removeDraft: (id) => removeAttachment(attachmentContext.workspaceSlug, id),
      }
      uploadQueueRef.current = new AttachmentUploadQueue(api)
      uploadQueueRef.current.setTaskRef(attachmentContext.taskRef)
      queueTaskRef.current = attachmentContext.taskRef
    }
    const uploadable = candidate.kind === "data" && candidate.sourceURL
      ? { ...candidate, kind: "file" as const, file: dataURLToFile(candidate.sourceURL, candidate.alt) }
      : candidate
    pendingUploadsRef.current += 1
    onAttachmentPendingChange?.(pendingUploadsRef.current)
    void uploadQueueRef.current.enqueue(uploadable).then((attachment) => {
      onDraftAttachmentCreated?.(attachment.id)
      failedMarkersRef.current.delete(marker)
      onAttachmentFailureChange?.(failedMarkersRef.current.size)
      setFailedUploads((current) => current.filter((item) => item.marker !== marker))
      replacePasteMarker(marker, `![${candidate.alt}](ref://attachment/${attachment.id})`)
    }).catch(() => {
      // 保留 marker 以阻止保存；用户删除 marker 后可继续保存，重新粘贴即可重试。
      failedMarkersRef.current.add(marker)
      onAttachmentFailureChange?.(failedMarkersRef.current.size)
      setFailedUploads((current) => current.some((item) => item.marker === marker) ? current : [...current, { candidate, marker }])
    }).finally(() => {
      pendingUploadsRef.current -= 1
      onAttachmentPendingChange?.(pendingUploadsRef.current)
    })
  }, [attachmentContext, onAttachmentFailureChange, onAttachmentPendingChange, onDeferredAttachment, onDraftAttachmentCreated, replacePasteMarker])
  const editor = useEditor({
    content: normalizeMarkdownSource(value),
    contentType: "markdown",
    editable: !disabled,
    editorProps: {
      attributes: {
        "aria-label": ariaLabel ?? "Markdown editor",
        class:
          "markdown-prose min-h-[var(--markdown-editor-min-height)] px-3 py-2 text-sm leading-6 outline-none",
        role: "textbox",
        // 编辑区是 Tab 进入组件后的首个落点，工具栏按钮已 tabIndex=-1 移出 Tab 序列。
        tabindex: "0",
      },
      handleKeyDown: (_view, event) => {
        if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
          onModEnter?.()
          return Boolean(onModEnter)
        }
        if (event.key === "Escape" && suggestion.active) {
          setSuggestion(emptySuggestionState)
          return true
        }
        if (suggestion.active && (event.key === "ArrowDown" || event.key === "ArrowUp" || event.key === "Enter")) {
          // 让菜单自己处理。
          return false
        }
        return false
      },
      handleDOMEvents: {
        compositionstart: () => {
          composingRef.current = true
          return false
        },
        compositionend: () => {
          composingRef.current = false
          return false
        },
      },
      // 富文本粘贴：先提取 <img> 候选再用 DOMPurify 白名单清洗剩余 HTML，
      // 截图粘贴（只有 image file、没有 HTML）也走此路径。
      handlePaste: (view, event) => {
        // 内部 clipboard MIME 优先：同一璇础编辑器复制时保留 attachment/reference ID。
        const internalMarkdown = event.clipboardData?.getData("application/x-xuanchu-markdown") ?? ""
        if (internalMarkdown) {
          view.dispatch(view.state.tr.insertText(internalMarkdown))
          event.preventDefault()
          return true
        }
        // 提取剪贴板中的文件（截图粘贴的主要载体）。
        const files: File[] = []
        if (event.clipboardData) {
          for (const item of Array.from(event.clipboardData.items)) {
            if (item.kind === "file") {
              const file = item.getAsFile()
              if (file) files.push(file)
            }
          }
        }
        const html = event.clipboardData?.getData("text/html") ?? ""

        // 截图粘贴场景：有图片文件但没有 HTML。
        // 直接插入 Markdown 格式的占位文本（编辑器使用 contentType: markdown）。
        if (files.length > 0 && !html) {
          const imageFiles = files.filter((f) =>
            ["image/png", "image/jpeg", "image/gif", "image/webp"].includes(f.type)
          )
          if (imageFiles.length > 0) {
            if (!attachmentContext?.taskRef && !onDeferredAttachment) return false
            for (const file of imageFiles) {
              const key = `paste-image-${crypto.randomUUID()}`
              const marker = `[[xuanchu-paste:${key}:${file.name || "截图"}]]`
              const candidate = { key, kind: "file" as const, file, alt: file.name || "截图" }
              if (!attachmentContext?.taskRef && onDeferredAttachment) {
                insertDeferredAttachmentPreview(view, candidate, marker)
              } else {
                view.dispatch(view.state.tr.insertText(marker))
              }
              queueCandidate(candidate, marker)
            }
            event.preventDefault()
            return true
          }
        }

        // 富文本粘贴场景：有 HTML，先提取 <img> 候选再清洗。
        if (!html) return false
        if (!attachmentContext?.taskRef && !onDeferredAttachment) return false
        const { html: cleaned, images } = sanitizeRichPaste({ html, files })
        if (!cleaned) return false
        const temp = document.createElement("div")
        temp.innerHTML = cleaned
        const slice = ProseMirrorDOMParser.fromSchema(view.state.schema).parseSlice(temp)
        view.dispatch(view.state.tr.replaceSelection(slice))
        for (const candidate of images) {
          const marker = `[[xuanchu-paste:${candidate.key}:${candidate.alt}]]`
          queueCandidate(candidate, marker)
        }
        event.preventDefault()
        return true
      },
      // 拖拽文件：只接受 PNG/JPEG/GIF/WebP。
      handleDrop: (view, event) => {
        if (!event.dataTransfer?.files?.length) return false
        const allowed = ["image/png", "image/jpeg", "image/gif", "image/webp"]
        const files = Array.from(event.dataTransfer.files).filter((f) =>
          allowed.includes(f.type)
        )
        if (files.length === 0) return false
        if (!attachmentContext?.taskRef && !onDeferredAttachment) return false
        for (const file of files) {
          const key = `drop-image-${crypto.randomUUID()}`
          const marker = `[[xuanchu-paste:${key}:${file.name || "图片"}]]`
          const candidate = { key, kind: "file" as const, file, alt: file.name || "图片" }
          if (!attachmentContext?.taskRef && onDeferredAttachment) {
            insertDeferredAttachmentPreview(view, candidate, marker)
          } else {
            view.dispatch(view.state.tr.insertText(marker))
          }
          queueCandidate(candidate, marker)
        }
        event.preventDefault()
        return true
      },
    },
    extensions: [
      ...markdownExtensionsWithAttachmentContext(attachmentContext?.taskRef ? {
        workspaceSlug: attachmentContext.workspaceSlug,
        taskRef: attachmentContext.taskRef,
      } : undefined),
      Placeholder.configure({
        placeholder,
      }),
    ],
    immediatelyRender: false,
      onUpdate: ({ editor: updatedEditor }) => {
        const nextMarkdown = normalizeMarkdownSource(updatedEditor.getMarkdown())
        for (const marker of failedMarkersRef.current) {
          if (!nextMarkdown.includes(marker.replace(/[\\[\]]/g, "\\$&"))) {
            failedMarkersRef.current.delete(marker)
            onAttachmentFailureChange?.(failedMarkersRef.current.size)
          }
        }
      emittedMarkdownValuesRef.current.add(nextMarkdown)
      onChange(nextMarkdown)
      // 检测 @ / # 触发。
      if (!attachmentContext) return
      const selection = updatedEditor.state.selection
      const textBefore = updatedEditor.state.doc.textBetween(
        Math.max(0, selection.from - 64),
        selection.from,
        "\n"
      )
      const match = detectReferenceTrigger({
        textBeforeCaret: textBefore,
        composing: composingRef.current,
      })
      if (match) {
        setSuggestion({
          active: true,
          kind: match.kind,
          query: match.query,
          range: { from: selection.from - match.query.length - 1, to: selection.from },
        })
      } else if (suggestion.active) {
        setSuggestion(emptySuggestionState)
      }
    },
  })

  useEffect(() => {
    editorRef.current = editor
  }, [editor])

  useEffect(() => () => {
    // 关闭编辑器时中断未完成请求，并清理已完成但尚未绑定的 draft；服务端 janitor
    // 仅作为浏览器崩溃等异常场景的兜底，不能替代正常取消路径。
    uploadQueueRef.current?.cancelAll()
    void uploadQueueRef.current?.cleanupDrafts()
  }, [])

  useEffect(() => {
    if (!editor) {
      return
    }

    editor.setEditable(!disabled)
  }, [disabled, editor])

  // handleSuggestionSelect 把选中的 reference 替换为 markdown link。
  const handleSuggestionSelect = useCallback(
    (item: ReferenceSuggestionMenuItem) => {
      if (!editor || !suggestion.range) {
        setSuggestion(emptySuggestionState)
        return
      }
      const label =
        item.label ||
        (item.kind === "task" && item.description ? `#${item.description}` : item.id)
      const { from, to } = suggestion.range
      editor
        .chain()
        .focus()
        .deleteRange({ from, to })
        .insertContentAt(from, {
          type: "xuanchuReference",
          attrs: { kind: item.kind as ReferenceTriggerKind, id: item.id, label },
        })
        .run()
      setSuggestion(emptySuggestionState)
    },
    [editor, suggestion.range]
  )

  useEffect(() => {
    if (!editor) {
      return
    }

    const nextValue = normalizeMarkdownSource(value)
    if (emittedMarkdownValuesRef.current.has(nextValue)) {
      emittedMarkdownValuesRef.current.delete(nextValue)
      return
    }

    const currentValue = normalizeMarkdownSource(editor.getMarkdown())
    if (nextValue !== currentValue) {
      editor.commands.setContent(nextValue, { contentType: "markdown" })
    }
  }, [editor, value])

  if (!editor) {
    return null
  }

  const toolbarDisabled = disabled || !editor.isEditable

  return (
    <TooltipProvider>
    <div
      className={cn(
        "overflow-hidden border bg-card",
        disabled ? "opacity-70" : null,
        className
      )}
      style={
        {
          "--markdown-editor-min-height": `${minHeight}px`,
        } as React.CSSProperties
      }
    >
      <div className="flex flex-wrap items-center gap-1 border-b bg-muted/30 p-1">
        <ToolbarButton
          active={editor.isActive("bold")}
          disabled={toolbarDisabled}
          label="加粗"
          onClick={() => editor.chain().focus().toggleBold().run()}
        >
          <BoldIcon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("italic")}
          disabled={toolbarDisabled}
          label="斜体"
          onClick={() => editor.chain().focus().toggleItalic().run()}
        >
          <ItalicIcon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("strike")}
          disabled={toolbarDisabled}
          label="删除线"
          onClick={() => editor.chain().focus().toggleStrike().run()}
        >
          <StrikethroughIcon />
        </ToolbarButton>
        <ToolbarSeparator />
        <ToolbarButton
          active={editor.isActive("heading", { level: 1 })}
          disabled={toolbarDisabled}
          label="一级标题"
          onClick={() => editor.chain().focus().toggleHeading({ level: 1 }).run()}
        >
          <Heading1Icon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("heading", { level: 2 })}
          disabled={toolbarDisabled}
          label="二级标题"
          onClick={() => editor.chain().focus().toggleHeading({ level: 2 }).run()}
        >
          <Heading2Icon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("heading", { level: 3 })}
          disabled={toolbarDisabled}
          label="三级标题"
          onClick={() => editor.chain().focus().toggleHeading({ level: 3 }).run()}
        >
          <Heading3Icon />
        </ToolbarButton>
        <ToolbarSeparator />
        <ToolbarButton
          active={editor.isActive("bulletList")}
          disabled={toolbarDisabled}
          label="无序列表"
          onClick={() => editor.chain().focus().toggleBulletList().run()}
        >
          <ListIcon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("orderedList")}
          disabled={toolbarDisabled}
          label="有序列表"
          onClick={() => editor.chain().focus().toggleOrderedList().run()}
        >
          <ListOrderedIcon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("taskList")}
          disabled={toolbarDisabled}
          label="待办列表"
          onClick={() => editor.chain().focus().toggleTaskList().run()}
        >
          <ListTodoIcon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("blockquote")}
          disabled={toolbarDisabled}
          label="引用"
          onClick={() => editor.chain().focus().toggleBlockquote().run()}
        >
          <QuoteIcon />
        </ToolbarButton>
        <ToolbarSeparator />
        <ToolbarButton
          active={editor.isActive("code")}
          disabled={toolbarDisabled}
          label="行内代码"
          onClick={() => editor.chain().focus().toggleCode().run()}
        >
          <CodeIcon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("codeBlock")}
          disabled={toolbarDisabled}
          label="代码块"
          onClick={() => editor.chain().focus().toggleCodeBlock().run()}
        >
          <BracesIcon />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("link")}
          disabled={toolbarDisabled}
          label="链接"
          onClick={() => {
            const previous = editor.getAttributes("link").href as string | undefined
            setLinkState({ open: true, initialHref: previous })
          }}
        >
          <LinkIcon />
        </ToolbarButton>
        <ToolbarButton
          disabled={toolbarDisabled}
          label="插入表格"
          onClick={() =>
            editor
              .chain()
              .focus()
              .insertTable({ cols: 3, rows: 3, withHeaderRow: true })
              .run()
          }
        >
          <TableIcon />
        </ToolbarButton>
        <ToolbarSeparator />
        <ToolbarButton
          disabled={toolbarDisabled || !editor.can().undo()}
          label="撤销"
          onClick={() => editor.chain().focus().undo().run()}
        >
          <Undo2Icon />
        </ToolbarButton>
        <ToolbarButton
          disabled={toolbarDisabled || !editor.can().redo()}
          label="重做"
          onClick={() => editor.chain().focus().redo().run()}
        >
          <Redo2Icon />
        </ToolbarButton>
        <ToolbarSeparator />
        <ToolbarButton
          active={mode === "source"}
          disabled={toolbarDisabled}
          label={mode === "source" ? "切换到富文本" : "切换到源码"}
          onClick={() => {
            if (mode === "wysiwyg") {
              setMode("source")
              return
            }
            // 从源码切回富文本：强制重载最新 markdown，保持与现有防抖逻辑兼容
            const next = normalizeMarkdownSource(value)
            editor.commands.setContent(next, { contentType: "markdown" })
            setMode("wysiwyg")
          }}
        >
          {mode === "source" ? <Eye /> : <Code2 />}
        </ToolbarButton>
      </div>
      {mode === "source" ? (
        <textarea
          aria-label="源码编辑器"
          className="markdown-prose min-h-[var(--markdown-editor-min-height)] w-full resize-y bg-transparent px-3 py-2 font-mono text-sm leading-6 outline-none"
          disabled={disabled}
          onChange={(e) => onChange(normalizeMarkdownSource(e.target.value))}
          placeholder={placeholder}
          value={value}
        />
      ) : (
        <>
      <EditorContent editor={editor} />
      {failedUploads.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2 border-t px-3 py-2 text-xs text-destructive">
          {failedUploads.map(({ candidate, marker }) => (
            <span className="inline-flex items-center gap-1" key={marker}>
              <span>{candidate.alt} 上传失败</span>
              <Button onClick={() => queueCandidate(candidate, marker)} size="sm" type="button" variant="ghost">重试</Button>
              <Button onClick={() => {
                failedMarkersRef.current.delete(marker)
                onAttachmentFailureChange?.(failedMarkersRef.current.size)
                setFailedUploads((current) => current.filter((item) => item.marker !== marker))
                replacePasteMarker(marker, "")
              }} size="sm" type="button" variant="ghost">移除</Button>
              {candidate.kind === "remote" && /^https?:\/\//.test(candidate.sourceURL ?? "") ? (
                <Button onClick={() => {
                  failedMarkersRef.current.delete(marker)
                  onAttachmentFailureChange?.(failedMarkersRef.current.size)
                  setFailedUploads((current) => current.filter((item) => item.marker !== marker))
                  replacePasteMarker(marker, `[${candidate.alt}](${candidate.sourceURL})`)
                }} size="sm" type="button" variant="ghost">保留链接</Button>
              ) : null}
            </span>
          ))}
        </div>
      ) : null}
          <TableBubbleMenu editor={editor} />
          {suggestion.active && suggestion.kind && attachmentContext && (
            <div className="absolute left-1/2 top-full z-10 -translate-x-1/2 pt-1">
              <ReferenceSuggestionMenu
                kind={suggestion.kind}
                query={suggestion.query}
                fetchSuggestions={attachmentContext.fetchSuggestions}
                onSelect={(item) => {
                  handleSuggestionSelect(item)
                }}
                onClose={() => setSuggestion(emptySuggestionState)}
              />
            </div>
          )}
        </>
      )}
      <LinkDialog
        initialHref={linkState.initialHref}
        onOpenChange={(open) => setLinkState((prev) => ({ ...prev, open }))}
        open={linkState.open}
        onSubmit={(href) => {
          editor.chain().focus().extendMarkRange("link").setLink({ href }).run()
        }}
      />
    </div>
    </TooltipProvider>
  )
}

function ToolbarButton({
  active = false,
  children,
  disabled = false,
  label,
  onClick,
}: {
  active?: boolean
  children: React.ReactNode
  disabled?: boolean
  label: string
  onClick: () => void
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          aria-label={label}
          aria-pressed={active}
          className={cn(active ? "bg-accent text-accent-foreground" : null)}
          disabled={disabled}
          onClick={onClick}
          size="icon-sm"
          // 工具栏按钮移出 Tab 序列：鼠标点击照常可用，Tab 聚焦时直接落到编辑区。
          tabIndex={-1}
          type="button"
          variant="ghost"
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function ToolbarSeparator() {
  return <div className="mx-1 h-5 w-px bg-border" />
}

// insertDeferredAttachmentPreview 在新建 task 尚未拥有 target ID 时插入本地预览节点。
// 它的 Markdown serializer 仍输出 marker，因此父表单可以在提交前上传并原子替换。
function insertDeferredAttachmentPreview(
  view: EditorView,
  candidate: PasteImageCandidate,
  marker: string
) {
  const type = view.state.schema.nodes.xuanchuAttachment
  if (!type) {
    view.dispatch(view.state.tr.insertText(marker))
    return
  }
  const sourceURL = candidate.kind === "data"
    ? candidate.sourceURL ?? ""
    : candidate.file
      ? URL.createObjectURL(candidate.file)
      : ""
  const node = type.create({
    id: "",
    label: candidate.alt,
    image: true,
    marker,
    sourceURL,
    state: "loading",
  })
  view.dispatch(view.state.tr.replaceSelectionWith(node))
}
