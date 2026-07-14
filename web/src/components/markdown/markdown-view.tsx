import { renderToReactElement } from "@tiptap/static-renderer/pm/react"

import { cn } from "@/lib/utils"

import { markdownExtensions, parseMarkdownToJSON } from "./extensions"
import "./markdown.css"

type MarkdownViewProps = {
  children: string
  className?: string
  headingOffset?: number
}

export function MarkdownView({
  children,
  className,
  headingOffset = 0,
}: MarkdownViewProps) {
  if (!children.trim()) {
    return null
  }

  const content = offsetHeadingLevels(
    parseMarkdownToJSON(children),
    headingOffset
  )

  return (
    <div className={cn("markdown-prose", className)}>
      {renderToReactElement({
        content,
        extensions: markdownExtensions,
      })}
    </div>
  )
}

type MarkdownJSONNode = ReturnType<typeof parseMarkdownToJSON>

function offsetHeadingLevels(
  node: MarkdownJSONNode,
  offset: number
): MarkdownJSONNode {
  if (offset <= 0) return node

  const headingLevel =
    node.type === "heading" && typeof node.attrs?.level === "number"
      ? Math.min(6, node.attrs.level + offset)
      : null

  return {
    ...node,
    ...(headingLevel === null
      ? {}
      : { attrs: { ...node.attrs, level: headingLevel } }),
    ...(node.content
      ? {
          content: node.content.map((child) =>
            offsetHeadingLevels(child, offset)
          ),
        }
      : {}),
  }
}
