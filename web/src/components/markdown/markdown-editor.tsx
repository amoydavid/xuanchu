import { autoUpdate, computePosition, flip, offset, shift } from "@floating-ui/dom"
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
import { createPortal } from "react-dom"

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
import type { ReferenceTriggerKind } from "./reference-suggestion"
import {
  clearEditorConfig,
  createReferenceSuggestionExtension,
  registerMenuKeyHandler,
  setEditorConfig,
  type ReferenceSuggestionEvent,
} from "./reference-suggestion-plugin"
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

// mentionMenuClosed 是 suggestion 关闭事件的常量，避免每次 render 新建对象。
const mentionMenuClosed: ReferenceSuggestionEvent = { open: false }

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
  // suggestion 由 @tiptap/suggestion 引擎通过 onChange 回调驱动；这里只存渲染所需状态。
  const [suggestion, setSuggestion] = useState<ReferenceSuggestionEvent>(mentionMenuClosed)
  const [failedUploads, setFailedUploads] = useState<Array<{ candidate: PasteImageCandidate; marker: string }>>([])
  const composingRef = useRef(false)
  const uploadQueueRef = useRef<AttachmentUploadQueue | null>(null)
  const queueTaskRef = useRef("")
  const editorRef = useRef<ReturnType<typeof useEditor>>(null)
  const pendingUploadsRef = useRef(0)
  const failedMarkersRef = useRef(new Set<string>())

  // suggestionExtension 在挂载时创建一次（useEditor 也只在挂载时消费 extensions）。
  // extension 不持有配置：配置通过模块级 WeakMap（key=editor）由下面的 effect 写入，
  // extension 通过 this.editor 读取，因此 render 中不传递任何 React ref。
  const [suggestionExtension] = useState(() =>
    attachmentContext ? createReferenceSuggestionExtension() : null
  )

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
        // mention 的 Escape / 方向键 / Enter 由 @tiptap/suggestion 插件接管，
        // 这里不再手动处理。
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
      // @ / # 触发由 @tiptap/suggestion 引擎接管；suggestionExtension 挂载时创建。
      ...(suggestionExtension ? [suggestionExtension] : []),
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
      // @ / # 触发已交给 @tiptap/suggestion 插件，这里不再手动检测。
    },
  })

  useEffect(() => {
    editorRef.current = editor
  }, [editor])

  // 把最新的 fetchSuggestions / onChange 写入 editor 对应的模块级配置（WeakMap）。
  // suggestion 引擎通过 this.editor 读取；effect 挂载后立即执行，远早于引擎首次（debounce 150ms）请求。
  useEffect(() => {
    if (!editor || !attachmentContext || !suggestionExtension) return
    setEditorConfig(editor, {
      fetchSuggestions: attachmentContext.fetchSuggestions,
      onChange: (event) => setSuggestion(event),
      lastError: null,
    })
    return () => {
      clearEditorConfig(editor)
    }
  }, [editor, attachmentContext, suggestionExtension])

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
          {suggestion.open && attachmentContext ? (
            <MentionFloatingMenu
              clientRect={suggestion.clientRect}
              kind={suggestion.kind}
              query={suggestion.query}
              items={suggestion.items}
              loading={suggestion.loading}
              error={suggestion.error}
              onSelect={(item) => {
                suggestion.command(item)
                setSuggestion(mentionMenuClosed)
              }}
              onClose={() => setSuggestion(mentionMenuClosed)}
            />
          ) : null}
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

// MentionFloatingMenu 把 @ / # 候选菜单通过 portal 渲染到 document.body，
// 并用 Floating UI 相对触发点（Suggestion 引擎提供的 clientRect）定位。
//
// 这样做的原因：编辑器外层容器是 overflow-hidden，且常出现在可滚动的 Dialog 内，
// 原来用 absolute top-full 挂在编辑器底部会让弹层被裁切（看不见），
// 位置也不跟随光标。Floating UI 的 flip/shift 中间件自动避让视口边界，
// portal 脱离 overflow 容器，从根本上解决裁切 + 定位两个问题。
function MentionFloatingMenu({
  clientRect,
  kind,
  query,
  items,
  loading,
  error,
  onSelect,
  onClose,
}: {
  clientRect: () => DOMRect | null
  kind: ReferenceTriggerKind
  query: string
  items: ReferenceSuggestionMenuItem[]
  loading: boolean
  error: string | null
  onSelect: (item: ReferenceSuggestionMenuItem) => void
  onClose: () => void
}) {
  const floatingRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const floating = floatingRef.current
    if (!floating) return
    // 虚拟锚点：getBoundingClientRect 委托给 Suggestion 引擎的 clientRect，
    // 它返回 @ / # 触发点的实时坐标。
    const virtualReference = {
      getBoundingClientRect: () => clientRect() ?? new DOMRect(),
    }
    const update = () => {
      void computePosition(virtualReference, floating, {
        placement: "bottom-start",
        middleware: [offset(4), flip({ padding: 8 }), shift({ padding: 8 })],
      }).then(({ x, y }) => {
        floating.style.left = `${x}px`
        floating.style.top = `${y}px`
      })
    }
    update()
    // autoUpdate 监听滚动/resize/光标移动，在会话期间保持弹层跟随。
    const cleanup = autoUpdate(virtualReference, floating, update)
    return cleanup
    // clientRect 是 Suggestion 引擎提供的稳定闭包，会话期间不变。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 把菜单的按键处理注册到 suggestion 引擎，让方向键/Enter 由菜单接管。
  // 这里用 ref 持有最新 handler，通过 registerMenuKeyHandler 转发给插件。
  const onKeyDownRef = useRef<(event: KeyboardEvent) => boolean>(() => false)
  useEffect(() => {
    const handler = (event: KeyboardEvent) => onKeyDownRef.current(event)
    registerMenuKeyHandler(handler)
    return () => {
      registerMenuKeyHandler(null)
    }
  }, [])

  return createPortal(
    <div
      ref={floatingRef}
      className="pointer-events-auto fixed z-50"
      style={{ left: -9999, top: -9999 }}
    >
      <ReferenceSuggestionMenu
        kind={kind}
        query={query}
        items={items}
        loading={loading}
        error={error}
        onSelect={onSelect}
        onClose={onClose}
        registerKeyHandler={(handler) => {
          onKeyDownRef.current = handler
        }}
      />
    </div>,
    document.body
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
