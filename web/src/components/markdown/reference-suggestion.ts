// ReferenceTriggerKind 表示 @ 用户 / # 任务 两种触发器。
// 触发检测本身由 @tiptap/suggestion 引擎（reference-suggestion-plugin.ts）接管，
// 这里只保留类型定义供菜单、插件、MarkdownEditor 共享。
export type ReferenceTriggerKind = "user" | "task"
