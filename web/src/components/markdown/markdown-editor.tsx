import Placeholder from "@tiptap/extension-placeholder"
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
import { useEffect, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

import {
  markdownExtensions,
  normalizeMarkdownSource,
} from "./extensions"
import { sanitizeRichPaste } from "./paste-sanitizer"
import { LinkDialog } from "./link-dialog"
import { TableBubbleMenu } from "./table-bubble-menu"
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
}

export function MarkdownEditor({
  ariaLabel,
  className,
  disabled = false,
  minHeight = 240,
  onChange,
  onModEnter,
  placeholder,
  value,
}: MarkdownEditorProps) {
  const emittedMarkdownValuesRef = useRef(new Set<string>())
  const [linkState, setLinkState] = useState<{ open: boolean; initialHref?: string }>(
    { open: false }
  )
  const [mode, setMode] = useState<"wysiwyg" | "source">("wysiwyg")
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
        return false
      },
      // 富文本粘贴：先提取 <img> 候选再用 DOMPurify 白名单清洗剩余 HTML，
      // 普通 plain text 走默认 markdown 粘贴。
      handlePaste: (view, event) => {
        const html = event.clipboardData?.getData("text/html") ?? ""
        if (!html) return false
        const files: File[] = []
        if (event.clipboardData) {
          for (const item of Array.from(event.clipboardData.items)) {
            if (item.kind === "file") {
              const file = item.getAsFile()
              if (file) files.push(file)
            }
          }
        }
        const { html: cleaned } = sanitizeRichPaste({ html, files })
        if (!cleaned) return false
        const temp = document.createElement("div")
        temp.innerHTML = cleaned
        const slice = temp.innerHTML
        const { tr } = view.state
        view.dispatch(tr.insertContent(slice))
        event.preventDefault()
        return true
      },
    },
    extensions: [
      ...markdownExtensions,
      Placeholder.configure({
        placeholder,
      }),
    ],
    immediatelyRender: false,
    onUpdate: ({ editor: updatedEditor }) => {
      const nextMarkdown = normalizeMarkdownSource(updatedEditor.getMarkdown())
      emittedMarkdownValuesRef.current.add(nextMarkdown)
      onChange(nextMarkdown)
    },
  })

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
          <TableBubbleMenu editor={editor} />
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
