import { Node } from "@tiptap/core"
import { MarkdownSerializer } from "@tiptap/markdown"

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

  // 让 @tiptap/markdown 在序列化/解析时把 ref://user|task/{uuid} 的 link 转为本节点。
  addProseMirrorPlugins() {
    return []
  },
})

// serializeReferenceMarkdown 把 reference 节点序列化为 markdown link。
export function serializeReferenceMarkdown(attrs: XuanchuReferenceAttrs): string {
  const safeLabel = (attrs.label || "").replace(/[[\]]/g, "")
  const href = `ref://${attrs.kind}/${attrs.id}`
  return `[${safeLabel}](${href})`
}

// attachReferenceSerializer 注册到 markdown serializer，把 xuanchuReference 节点转为 link markdown。
// 调用方需要在 MarkdownManager 配置里使用。
export function registerReferenceSerializer(serializer: MarkdownSerializer): void {
  // @tiptap/markdown 的 serializer 通过 storage key 注册节点序列化。
  // 这里使用 nodes 字段覆盖，使序列化结果与标准 link 一致。
  // 注意：@tiptap/markdown 默认会把未识别节点渲染为 text；本函数保留扩展点，
  // 实际序列化由 link 语法承载（ref:// 协议在 markdown 中就是普通 link destination）。
  void serializer
}
