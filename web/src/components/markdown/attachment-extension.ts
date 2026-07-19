import { Node } from "@tiptap/core"

// XuanchuAttachmentAttrs 描述 attachment 节点的属性。
//
// id 是附件 UUID，作为稳定身份；label 是保存时的展示文字快照；
// image=true 表示是内联图片（渲染为 <img>），false 表示文件卡片（渲染为 <a>）。
export type XuanchuAttachmentAttrs = {
  id: string
  label: string
  image: boolean
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
export const XuanchuAttachment = Node.create<{
  HTMLAttributes: Record<string, string>
}>({
  name: "xuanchuAttachment",
  group: "block inline",
  inline: true,
  atom: true,
  selectable: true,
  draggable: false,

  addAttributes() {
    return {
      id: { default: "" },
      label: { default: "" },
      image: { default: false },
    }
  },

  parseHTML() {
    return [
      {
        tag: "a[data-xuanchu-attachment]",
        getAttrs: (el) => {
          const node = el as HTMLElement
          const id = node.getAttribute("data-xuanchu-attachment") ?? ""
          const label = node.textContent ?? ""
          return { id, label, image: false }
        },
      },
      {
        tag: "img[data-xuanchu-attachment]",
        getAttrs: (el) => {
          const node = el as HTMLElement
          const id = node.getAttribute("data-xuanchu-attachment") ?? ""
          const label = node.getAttribute("alt") ?? ""
          return { id, label, image: true }
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
