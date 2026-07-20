import { Extension, Node } from "@tiptap/core"
import { ReactNodeViewRenderer } from "@tiptap/react"

import { AttachmentNodeView } from "./attachment-node-view"
import { isAllowedMarkdownHref } from "./markdown-safety"
import { parseReferenceRef } from "./reference-extension"

// XuanchuAttachmentAttrs 描述 attachment 节点的属性。
//
// id 是附件 UUID，作为稳定身份；label 是保存时的展示文字快照；
// image=true 表示是内联图片（渲染为 <img>），false 表示文件卡片（渲染为 <a>）。
export type XuanchuAttachmentAttrs = {
  id: string
  label: string
  image: boolean
  // loading/failed 是编辑器内临时状态，不能作为可保存内容提交；resolved
  // 节点才会被序列化为 canonical ref://attachment URI。
  state: "resolved" | "loading" | "failed"
  // 新建 task 尚未拥有稳定 task ID 时，临时节点以 marker 保持在内存 Markdown
  // 中；提交创建后由 task-create-dialog 原子替换为 attachment URI。
  marker?: string
  sourceURL?: string
}

export type XuanchuAttachmentStorage = {
  workspaceSlug: string
  taskRef: string
  retryPendingAttachment?: (key: string) => void
  removePendingAttachment?: (key: string) => void
  fallbackPendingAttachmentToLink?: (key: string) => void
}

declare module "@tiptap/core" {
  // 让 @tiptap/markdown 知道本节点的 attrs 类型。
  interface Commands<ReturnType> {
    xuanchuAttachment: {
      insertAttachment: (attrs: XuanchuAttachmentAttrs) => ReturnType
    }
  }
}

// ATTACHMENT_HREF_RE 匹配 canonical `ref://attachment/{canonical-lowercase-uuid}` URI。
// 不接受大小写漂移、userinfo、port、query、fragment、额外 path segment、percent-encoding。
const ATTACHMENT_HREF_RE = /^ref:\/\/attachment\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/

// parseAttachmentRef 把 link/image destination 解析为 (id, image)。
// 非 attachment URI 返回 null；非法格式抛错（由 parse 入口决定如何处理）。
export function parseAttachmentRef(href: string): { id: string; image: boolean } | null {
  if (!href.startsWith("ref://attachment/")) return null
  if (!ATTACHMENT_HREF_RE.test(href)) {
    throw new Error("description_reference_invalid")
  }
  const id = href.slice("ref://attachment/".length)
  return { id, image: false }
}

// XuanchuAttachment 是附件/图片节点的 Tiptap 定义。
//
// Markdown 层面：图片序列化为 `![label](ref://attachment/{id})`，
// 文件附件序列化为 `[label](ref://attachment/{id})`。parseMarkdown 时根据
// markdown 文本中的 image/link destination 命中 ref://attachment/ 自动转为本节点。
export const XuanchuAttachment = Node.create<
  { HTMLAttributes: Record<string, string>; workspaceSlug: string; taskRef: string },
  XuanchuAttachmentStorage
>({
  name: "xuanchuAttachment",
  // 必须先于 Link 接管同一个 marked link token。
  priority: 1100,
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,
  draggable: false,

  addOptions() {
    return { HTMLAttributes: {}, workspaceSlug: "", taskRef: "" }
  },

  addAttributes() {
    return {
      id: { default: "" },
      label: { default: "" },
      image: { default: false },
      state: { default: "resolved" },
      marker: { default: "" },
      sourceURL: { default: "" },
    }
  },

  addStorage() {
    return { workspaceSlug: this.options.workspaceSlug, taskRef: this.options.taskRef }
  },

  parseHTML() {
    return [
      {
        tag: "a[data-xuanchu-attachment]",
        getAttrs: (el) => {
          const node = el as HTMLElement
          const id = node.getAttribute("data-xuanchu-attachment") ?? ""
          const label = node.textContent ?? ""
          return { id, label, image: false, state: "resolved" }
        },
      },
      {
        tag: "img[data-xuanchu-attachment]",
        getAttrs: (el) => {
          const node = el as HTMLElement
          const id = node.getAttribute("data-xuanchu-attachment") ?? ""
          const label = node.getAttribute("alt") ?? ""
          return { id, label, image: true, state: "resolved" }
        },
      },
    ]
  },

  renderHTML({ node, HTMLAttributes }) {
    const attrs = node.attrs as XuanchuAttachmentAttrs
    const merged = {
      ...HTMLAttributes,
      "data-xuanchu-attachment": attrs.id,
    }
    if (attrs.image) {
      return ["img", { ...merged, alt: attrs.label }]
    }
    return ["a", merged, attrs.label]
  },

  addNodeView() {
    return ReactNodeViewRenderer(AttachmentNodeView)
  },

  markdownTokenName: "image",

  parseMarkdown(token, helpers) {
    const href = String(token.href ?? "")
    const parsed = parseAttachmentRef(href)
    if (!parsed) return helpers.parseInline(token.tokens ?? [])
    return helpers.createNode("xuanchuAttachment", {
      id: parsed.id,
      label: String(token.text ?? ""),
      image: true,
      state: "resolved",
    })
  },

  renderMarkdown(node) {
    const attrs = node.attrs as XuanchuAttachmentAttrs
    // 新建 task 的本地预览节点仍须在受控 value 中保留 marker，供提交时上传并
    // 替换为 canonical URI；这个 marker 只存在编辑会话，永不进入最终任务数据。
    if (attrs.state !== "resolved") return attrs.marker || ""
    return serializeAttachmentMarkdown(attrs)
  },

  addCommands() {
    return {
      insertAttachment:
        (attrs) =>
        ({ commands }) =>
          commands.insertContent({
            type: this.name,
            attrs,
          }),
    }
  },
})

// Marked 对普通链接和图片使用不同 token 名称。这个 parser extension 与
// XuanchuAttachment 共用 node 名称，专门把 ref://attachment 链接转为 atom。
export const XuanchuAttachmentLinkMarkdown = Extension.create({
  name: "xuanchuAttachmentLinkMarkdown",
  // Link mark 的 Markdown parser 会吞掉任何标准 link；必须更早执行。
  priority: 2000,
  markdownTokenName: "link",
  parseMarkdown(token, helpers) {
    const href = String(token.href ?? "")
    const parsed = parseAttachmentRef(href)
    if (parsed) {
      return helpers.createNode("xuanchuAttachment", {
        id: parsed.id,
        label: String(token.text ?? ""),
        image: false,
        state: "resolved",
      })
    }
    // MarkdownManager 的 inline parser 只尝试第一个 link handler；因此所有
    // canonical ref 实体必须在此处统一分流，不能依赖后续 handler fallback。
    const reference = parseReferenceRef(href)
    if (reference) {
      return helpers.createNode("xuanchuReference", {
        kind: reference.kind,
        id: reference.id,
        label: String(token.text ?? ""),
      })
    }
    // Inline parser 仅调用优先级最高的 link handler，因此这里必须复现普通 Link
    // 的安全回退，不能返回 null 让合法 https 链接消失。
    const content = helpers.parseInline(token.tokens ?? [])
    if (!isAllowedMarkdownHref(href)) return content
    return helpers.applyMark("link", content, {
      href,
      ...(token.title ? { title: String(token.title) } : {}),
    })
  },
})

// serializeAttachmentMarkdown 把 attachment 节点序列化为 markdown。
//
// 图片：`![label](ref://attachment/{id})`
// 文件：`[label](ref://attachment/{id})`
// 由 @tiptap/markdown 的 markdown serializer 通过 `markdownStorageKey: "xuanchuAttachment"` 调用。
export function serializeAttachmentMarkdown(attrs: XuanchuAttachmentAttrs): string {
  const safeLabel = (attrs.label || "").replace(/[[\]]/g, "")
  const href = `ref://attachment/${attrs.id}`
  if (attrs.image) {
    return `![${safeLabel}](${href})`
  }
  return `[${safeLabel}](${href})`
}
