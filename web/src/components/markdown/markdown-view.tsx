import { renderToReactElement } from "@tiptap/static-renderer/pm/react"

import { cn } from "@/lib/utils"

import { markdownExtensions, parseMarkdownToJSON } from "./extensions"
import "./markdown.css"

type MarkdownViewProps = {
  children: string
  className?: string
}

export function MarkdownView({ children, className }: MarkdownViewProps) {
  if (!children.trim()) {
    return null
  }

  return (
    <div className={cn("markdown-prose", className)}>
      {renderToReactElement({
        content: parseMarkdownToJSON(children),
        extensions: markdownExtensions,
      })}
    </div>
  )
}
