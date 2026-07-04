import Link from "@tiptap/extension-link"
import { TaskItem, TaskList } from "@tiptap/extension-list"
import { Table, TableCell, TableHeader, TableRow } from "@tiptap/extension-table"
import { Markdown, MarkdownManager } from "@tiptap/markdown"
import StarterKit from "@tiptap/starter-kit"

import { escapeMarkdownHtml, isAllowedMarkdownHref } from "./markdown-safety"

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
    resizable: false,
  }),
  TableRow,
  TableHeader,
  TableCell,
]

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
