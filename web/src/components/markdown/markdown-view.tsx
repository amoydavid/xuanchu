import { renderToReactElement } from "@tiptap/static-renderer/pm/react"

import { cn } from "@/lib/utils"

import { markdownExtensions, parseMarkdownToJSON } from "./extensions"
import { AttachmentInlineView } from "./attachment-node-view"
import { ReferenceInlineView } from "./reference-inline-view"
import "./markdown.css"

type MarkdownViewProps = {
  children: string
  className?: string
  headingOffset?: number
  attachmentContext?: { workspaceSlug: string; taskRef: string }
}

export function MarkdownView({
  children,
  className,
  headingOffset = 0,
  attachmentContext,
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
        options: {
          nodeMapping: {
            xuanchuAttachment: ({ node }: { node: { attrs: unknown } }) => {
              const attrs = node.attrs as { id: string; label: string; image?: boolean }
              if (attachmentContext) {
                return <AttachmentInlineView workspaceSlug={attachmentContext.workspaceSlug} taskRef={attachmentContext.taskRef} attrs={attrs} />
              }
              return <span className="rounded bg-muted px-1 py-0.5 text-xs">{attrs.label}</span>
            },
            xuanchuReference: ({ node }) => {
              const attrs = node.attrs as { kind: "user" | "task"; id: string; label: string }
              return <ReferenceInlineView kind={attrs.kind} id={attrs.id} label={attrs.label} />
            },
          },
        },
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
