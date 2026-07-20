import Link from "@tiptap/extension-link"
import { TaskItem, TaskList } from "@tiptap/extension-list"
import { Table, TableCell, TableHeader, TableRow } from "@tiptap/extension-table"
import { Markdown, MarkdownManager } from "@tiptap/markdown"
import StarterKit from "@tiptap/starter-kit"

import {
  XuanchuAttachment,
  XuanchuAttachmentLinkMarkdown,
} from "./attachment-extension"
import { escapeMarkdownHtml, isAllowedMarkdownHref } from "./markdown-safety"
import { XuanchuReference } from "./reference-extension"

export const markdownExtension = Markdown.configure({
  markedOptions: {
    breaks: false,
    gfm: true,
  },
})

export const markdownExtensions = [
  StarterKit.configure({
    link: false,
  }),
  markdownExtension,
  Link.configure({
    autolink: true,
    HTMLAttributes: {
      rel: "noopener noreferrer",
      target: "_blank",
    },
    linkOnPaste: true,
    openOnClick: false,
    protocols: ["http", "https", "mailto"],
    validate: isAllowedMarkdownHref,
  }),
  TaskList,
  TaskItem.configure({
    nested: true,
  }),
  Table.configure({
    resizable: true,
    cellMinWidth: 80,
  }),
  TableRow,
  TableHeader,
  TableCell,
  // 自定义节点：附件与 user/task 引用。ref:// 协议仍以普通 link 的形式
  // 在 markdown 中 round-trip，节点定义负责编辑器内的 atom 渲染与序列化。
  XuanchuAttachment,
  XuanchuAttachmentLinkMarkdown,
  XuanchuReference,
]

// markdownExtensionsWithAttachmentContext 为可编辑 task description 注入节点视图所需
// 的鉴权查询上下文；只读渲染仍使用无上下文的共享 Markdown schema。
export function markdownExtensionsWithAttachmentContext(context?: {
  workspaceSlug: string
  taskRef: string
}) {
  if (!context) return markdownExtensions
  return markdownExtensions.map((extension) =>
    extension.name === XuanchuAttachment.name
      ? XuanchuAttachment.configure(context)
      : extension
  )
}

export const markdownManager = new MarkdownManager({
  extensions: markdownExtensions,
  markedOptions: {
    breaks: false,
    gfm: true,
  },
})

export function normalizeMarkdownSource(source: string): string {
  return escapeMarkdownHtml(source)
}

export function parseMarkdownToJSON(source: string) {
  return markdownManager.parse(normalizeMarkdownSource(source))
}
