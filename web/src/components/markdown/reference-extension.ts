import { Extension, Node } from "@tiptap/core"

// XuanchuReferenceAttrs 描述 user/task 引用节点的属性。
export type XuanchuReferenceAttrs = {
  kind: "user" | "task"
  id: string
  label: string
}

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    xuanchuReference: {
      insertReference: (attrs: XuanchuReferenceAttrs) => ReturnType
    }
  }
}

// REFERENCE_HREF_RE 匹配 canonical ref://user/{uuid} 或 ref://task/{uuid}。
const REFERENCE_HREF_RE =
  /^ref:\/\/(user|task)\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/

// parseReferenceRef 把 link destination 解析为 (kind, id)。
// 非 ref:// 引用返回 null；非法格式返回 null（不抛错，由 link 节点处理普通链接）。
export function parseReferenceRef(href: string): { kind: "user" | "task"; id: string } | null {
  const match = REFERENCE_HREF_RE.exec(href)
  if (!match) return null
  return { kind: match[1] as "user" | "task", id: href.slice(`ref://${match[1]}/`.length) }
}

// XuanchuReference 是 user/task 引用的 Tiptap inline atom 节点。
//
// Markdown 层面序列化为标准 link：`[@label](ref://user/{id})` / `[#label](ref://task/{id})`。
export const XuanchuReference = Node.create({
  name: "xuanchuReference",
  // 必须早于 Link mark 接管 canonical ref://user|task link。
  priority: 2000,
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      kind: { default: "user" },
      id: { default: "" },
      label: { default: "" },
    }
  },

  parseHTML() {
    return [
      {
        tag: "a[data-xuanchu-reference]",
        getAttrs: (el) => {
          const node = el as HTMLElement
          const kind = (node.getAttribute("data-xuanchu-reference") ?? "user") as "user" | "task"
          const id = node.getAttribute("data-xuanchu-reference-id") ?? ""
          const label = node.textContent ?? ""
          return { kind, id, label }
        },
      },
    ]
  },

  renderHTML({ node, HTMLAttributes }) {
    const attrs = node.attrs as XuanchuReferenceAttrs
    const merged = {
      ...HTMLAttributes,
      "data-xuanchu-reference": attrs.kind,
      "data-xuanchu-reference-id": attrs.id,
      href: `ref://${attrs.kind}/${attrs.id}`,
    }
    return ["a", merged, attrs.label]
  },

  addCommands() {
    return {
      insertReference:
        (attrs) =>
        ({ commands }) =>
          commands.insertContent({
            type: this.name,
            attrs,
          }),
    }
  },

  // Marked 把标准 Markdown link 产出为 link token；只接管 canonical user/task
  // URI，其余链接继续由 Link mark 处理。
  markdownTokenName: "link",
  parseMarkdown(token, helpers) {
    const href = String(token.href ?? "")
    const parsed = parseReferenceRef(href)
    if (!parsed) return helpers.parseInline(token.tokens ?? [])
    return helpers.createNode("xuanchuReference", {
      kind: parsed.kind,
      id: parsed.id,
      label: String(token.text ?? ""),
    })
  },

  renderMarkdown(node) {
    return serializeReferenceMarkdown(node.attrs as XuanchuReferenceAttrs)
  },

  // 让 @tiptap/markdown 在序列化/解析时把 ref://user|task/{uuid} 的 link 转为本节点。
  addProseMirrorPlugins() {
    return []
  },
})

// 当前 extension 已直接处理 link token。保留空 export 以便未来新增实体引用时
// 仍能将 parser 拆分为独立 extension，避免改变稳定的节点名称。
export const XuanchuReferenceMarkdown = Extension.create({
  name: "xuanchuReferenceMarkdown",
})

// serializeReferenceMarkdown 把 reference 节点序列化为 markdown link。
export function serializeReferenceMarkdown(attrs: XuanchuReferenceAttrs): string {
  const safeLabel = (attrs.label || "").replace(/[[\]]/g, "")
  const href = `ref://${attrs.kind}/${attrs.id}`
  return `[${safeLabel}](${href})`
}
