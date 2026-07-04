import { EditorContent, useEditor } from "@tiptap/react"
import {
  BoldIcon,
  BracesIcon,
  CodeIcon,
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
import { useEffect, useRef } from "react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import {
  markdownExtensions,
  normalizeMarkdownSource,
} from "./extensions"
import { isAllowedMarkdownHref } from "./markdown-safety"
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
      },
      handleKeyDown: (_view, event) => {
        if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
          onModEnter?.()
          return Boolean(onModEnter)
        }
        return false
      },
    },
    extensions: markdownExtensions,
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
            const next = window.prompt("链接地址", previous ?? "https://")
            if (!next) {
              return
            }
            if (!isAllowedMarkdownHref(next)) {
              return
            }
            editor.chain().focus().extendMarkRange("link").setLink({ href: next }).run()
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
      </div>
      <EditorContent
        data-placeholder={placeholder}
        editor={editor}
      />
    </div>
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
    <Button
      aria-label={label}
      aria-pressed={active}
      className={cn(active ? "bg-accent text-accent-foreground" : null)}
      disabled={disabled}
      onClick={onClick}
      size="icon-sm"
      type="button"
      variant="ghost"
    >
      {children}
    </Button>
  )
}

function ToolbarSeparator() {
  return <div className="mx-1 h-5 w-px bg-border" />
}
