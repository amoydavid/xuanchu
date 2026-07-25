import { Extension, type Editor } from "@tiptap/core"
import { PluginKey } from "@tiptap/pm/state"
import {
  Suggestion,
  type SuggestionOptions,
  type SuggestionProps,
} from "@tiptap/suggestion"

import { XuanchuReference } from "./reference-extension"
import type { ReferenceTriggerKind } from "./reference-suggestion"
import type {
  FetchSuggestions,
  ReferenceSuggestionMenuItem,
} from "./reference-suggestion-menu"

// editorConfigs 用 editor 实例作为 key 存储每个 MarkdownEditor 的 suggestion 配置。
// 这样 extension（挂载时创建一次）能通过 this.editor 读到最新的 fetchSuggestions /
// onChange，而调用方无需在 render 中传递 React ref——符合 react-hooks/refs 规则。
// WeakMap 让 editor 销毁后配置自动 GC。
const editorConfigs = new WeakMap<Editor, ReferenceSuggestionPluginConfig>()

// setEditorConfig 由 MarkdownEditor 在 effect 中调用，写入当前 editor 的配置。
export function setEditorConfig(
  editor: Editor,
  config: ReferenceSuggestionPluginConfig
) {
  editorConfigs.set(editor, config)
}

export function clearEditorConfig(editor: Editor) {
  editorConfigs.delete(editor)
}

// ReferenceSuggestionEvent 是 Suggestion 引擎触发后推给 React 层的事件。
// React 层（MarkdownEditor）据此渲染 ReferenceSuggestionMenu，并用 clientRect
// 做 Floating UI 定位。这样弹层仍在 React 树内，i18n 等上下文正常可用。
//
// items / loading 由引擎统一获取（内置 debounce + AbortSignal），React 菜单只负责渲染，
// 不再重复 fetch，避免双重防抖。
export type ReferenceSuggestionEvent =
  | {
      kind: ReferenceTriggerKind
      open: true
      query: string
      items: ReferenceSuggestionMenuItem[]
      loading: boolean
      // error 是 items fetch 失败时的文案（成功后为 null）。
      error: string | null
      clientRect: () => DOMRect | null
      // command 由 Suggestion 引擎提供；菜单选中后调用它会删除触发区间并插入节点。
      command: (item: ReferenceSuggestionMenuItem) => void
    }
  | { open: false }

// 菜单按键处理器类型：React 层渲染的菜单通过 registerMenuKeyHandler 注册自己的
// onKeyDown，Suggestion 引擎的 onKeyDown 钩子把方向键/Enter 路由进来。
type MenuKeyHandler = (event: KeyboardEvent) => boolean

// ReferenceSuggestionPluginConfig 是挂载到 extension 上的可变配置容器。
// MarkdownEditor 在每次 render 时更新 holder.current，Suggestion 引擎通过 holder
// 读取最新的 fetchSuggestions / onChange，避免重建 editor。
export type ReferenceSuggestionPluginConfig = {
  fetchSuggestions: FetchSuggestions
  // onChange 由 Suggestion 的 render 钩子驱动，把触发/更新/关闭事件推给 React。
  onChange: (event: ReferenceSuggestionEvent) => void
  // lastError 供 items 选项写入 fetch 失败信息，再由 emit 读出推给 React。
  lastError: string | null
}

// pluginKey 用 kind 区分两套 Suggestion 插件（@ user / # task），
// 避免它们共享状态互相干扰。
const userSuggestionKey = new PluginKey<unknown>("xuanchu-user-suggestion")
const taskSuggestionKey = new PluginKey<unknown>("xuanchu-task-suggestion")

// currentMenuKeyHandler 是当前活跃 mention 菜单的按键处理器。
// 由 MarkdownEditor 在弹层渲染时通过 registerMenuKeyHandler 注册，
// 供 Suggestion 的 onKeyDown 路由方向键/Enter。
let currentMenuKeyHandler: MenuKeyHandler | null = null

// registerMenuKeyHandler 是 React 层注册当前菜单按键处理器的入口。
// MarkdownEditor 在 mention 弹层挂载时调用，卸载时传 null。
export function registerMenuKeyHandler(handler: MenuKeyHandler | null) {
  currentMenuKeyHandler = handler
}

// makeSuggestionOptions 构造单个触发字符的 Suggestion 配置。
//
// 关键点：
// - items 用引擎内置的 debounce（150ms）+ AbortSignal，取代手写 setTimeout/abort。
// - render 只把事件转译成 onChange 回调，定位和渲染交给 React 层（MarkdownEditor），
//   这样弹层留在 React 树内，i18n 等 context 可用。
// - command 删除触发区间并插入 xuanchuReference 节点，保留 ref:// 序列化不变。
// - onKeyDown 把方向键/Enter 委托给 React 菜单，Escape 直接关闭。
function makeSuggestionOptions(input: {
  editor: Editor
  kind: ReferenceTriggerKind
  pluginKey: PluginKey
}): SuggestionOptions<ReferenceSuggestionMenuItem> {
  const { editor, kind, pluginKey } = input
  return {
    editor,
    pluginKey,
    char: kind === "user" ? "@" : "#",
    allowedPrefixes: [" ", "\n", ""],
    // 不允许空格进入 query，和原来 detectReferenceTrigger 的「空白即结束」一致。
    allowSpaces: false,
    // 150ms 防抖，与原 ReferenceSuggestionMenu 的 debounceMs 对齐。
    debounce: 150,
    items: async ({ query, signal }) => {
      const config = editorConfigs.get(editor)
      if (!config) return []
      // 空 query 也发请求：后端用 LIKE '%%' 匹配全部，让用户刚敲 @ 就看到成员列表，
      // 而不是显示"无结果"。
      config.lastError = null
      try {
        return await config.fetchSuggestions({ kind, query: query.trim(), signal })
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return []
        config.lastError = err instanceof Error ? err.message : String(err)
        return []
      }
    },
    command: ({ editor, range, props }) => {
      const label =
        props.label ||
        (kind === "task" && props.description ? `#${props.description}` : props.id)
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: XuanchuReference.name,
          attrs: { kind, id: props.id, label },
        })
        .run()
    },
    render: () => {
      return {
        onStart: (props) => emit(editor, kind, props),
        onUpdate: (props) => emit(editor, kind, props),
        onExit: () => {
          editorConfigs.get(editor)?.onChange({ open: false })
        },
        onKeyDown: ({ event }) => {
          if (event.key === "Escape") {
            editorConfigs.get(editor)?.onChange({ open: false })
            return true
          }
          // 方向键/Enter 委托给 React 菜单的按键处理器。
          if (currentMenuKeyHandler) return currentMenuKeyHandler(event)
          return false
        },
      }
    },
  }
}

// emit 把 Suggestion props 转译成 React 层消费的事件。
// items / loading 来自引擎（它内部已防抖 + 取消旧请求），React 菜单直接渲染即可。
function emit(
  editor: Editor,
  kind: ReferenceTriggerKind,
  props: SuggestionProps<ReferenceSuggestionMenuItem>
) {
  const config = editorConfigs.get(editor)
  if (!config) return
  config.onChange({
    kind,
    open: true,
    query: props.query,
    items: props.items,
    loading: props.loading,
    error: config.lastError,
    clientRect: props.clientRect ?? (() => null),
    command: (item) => {
      props.command(item)
    },
  })
}

// createReferenceSuggestionExtension 创建携带 @ / # 两套 Suggestion 插件的 extension。
// 不接收配置容器：配置通过 setEditorConfig(editor, config) 写入模块级 WeakMap，
// extension 通过 this.editor 读取，调用方无需在 render 中传递 React ref。
export function createReferenceSuggestionExtension() {
  return Extension.create({
    name: "xuanchuReferenceSuggestion",
    addProseMirrorPlugins() {
      return [
        Suggestion<ReferenceSuggestionMenuItem>(
          makeSuggestionOptions({
            editor: this.editor,
            kind: "user",
            pluginKey: userSuggestionKey,
          })
        ),
        Suggestion<ReferenceSuggestionMenuItem>(
          makeSuggestionOptions({
            editor: this.editor,
            kind: "task",
            pluginKey: taskSuggestionKey,
          })
        ),
      ]
    },
  })
}
