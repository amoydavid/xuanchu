import { BubbleMenu } from "@tiptap/react/menus"
import type { Editor } from "@tiptap/react"
import { useEditorState } from "@tiptap/react"
import { CellSelection, selectedRect } from "@tiptap/pm/tables"
import {
  BetweenHorizontalEnd,
  BetweenHorizontalStart,
  BetweenVerticalEnd,
  BetweenVerticalStart,
  Rows3,
  Columns3,
  Table2,
  Trash2,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

type TableBubbleMenuProps = {
  editor: Editor
}

/**
 * 表格浮动操作栏，遵循 Word 心智：
 * - 默认（光标在单元格）：提供「选行/选列」入口 + 上下左右插入行列
 * - 整行选中：删除该行 / 上下加行
 * - 整列选中：删除该列 / 左右加列
 * - 整表选中：删除表格
 */
export function TableBubbleMenu({ editor }: TableBubbleMenuProps) {
  // BubbleMenu 的 children 不会随选区自动重渲染，必须用 useEditorState 订阅 transaction
  const selectionKind = useEditorState({
    editor,
    selector: ({ editor }) => detectSelectionKind(editor),
  })

  return (
    <BubbleMenu
      editor={editor}
      shouldShow={({ editor }) => editor.isActive("table")}
    >
      <div className="flex items-center gap-1 rounded-md border bg-popover p-1 shadow-md">
        {selectionKind === "table" ? (
          <DeleteTableButton editor={editor} />
        ) : selectionKind === "row" ? (
          <>
            <InsertRowButton editor={editor} dir="before" />
            <InsertRowButton editor={editor} dir="after" />
            <DeleteRowButton editor={editor} />
            <DeleteTableButton editor={editor} />
          </>
        ) : selectionKind === "column" ? (
          <>
            <InsertColumnButton editor={editor} dir="before" />
            <InsertColumnButton editor={editor} dir="after" />
            <DeleteColumnButton editor={editor} />
            <DeleteTableButton editor={editor} />
          </>
        ) : (
          <>
            <SelectRowButton editor={editor} />
            <SelectColumnButton editor={editor} />
            <ToolbarDivider />
            <InsertRowButton editor={editor} dir="before" />
            <InsertRowButton editor={editor} dir="after" />
            <InsertColumnButton editor={editor} dir="before" />
            <InsertColumnButton editor={editor} dir="after" />
          </>
        )}
      </div>
    </BubbleMenu>
  )
}

type SelectionKind = "cell" | "row" | "column" | "table"

function detectSelectionKind(editor: Editor): SelectionKind {
  const sel = editor.state.selection
  if (!(sel instanceof CellSelection)) {
    return "cell"
  }
  const isRow = sel.isRowSelection()
  const isCol = sel.isColSelection()
  if (isRow && isCol) return "table"
  if (isRow) return "row"
  if (isCol) return "column"
  return "cell"
}

/** 选中当前光标所在整行 */
function selectRow(editor: Editor) {
  const rect = selectedRect(editor.state)
  const { tableStart, map, table } = rect
  const anchor = tableStart + map.positionAt(rect.top, 0, table)
  const head = tableStart + map.positionAt(rect.top, map.width - 1, table)
  editor.commands.setCellSelection({ anchorCell: anchor, headCell: head })
}

/** 选中当前光标所在整列 */
function selectColumn(editor: Editor) {
  const rect = selectedRect(editor.state)
  const { tableStart, map, table } = rect
  const anchor = tableStart + map.positionAt(0, rect.left, table)
  const head = tableStart + map.positionAt(map.height - 1, rect.left, table)
  editor.commands.setCellSelection({ anchorCell: anchor, headCell: head })
}

function ToolbarDivider() {
  return <div className="mx-0.5 h-4 w-px bg-border" />
}

function MenuButton({
  active = false,
  ariaLabel,
  destructive = false,
  onClick,
  children,
}: {
  active?: boolean
  ariaLabel: string
  destructive?: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          aria-label={ariaLabel}
          className={cn(active ? "bg-accent text-accent-foreground" : null)}
          onClick={onClick}
          size="icon-xs"
          type="button"
          variant={destructive ? "destructive" : "ghost"}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{ariaLabel}</TooltipContent>
    </Tooltip>
  )
}

function SelectRowButton({ editor }: { editor: Editor }) {
  return (
    <MenuButton ariaLabel="选中整行" onClick={() => selectRow(editor)}>
      <Rows3 />
    </MenuButton>
  )
}

function SelectColumnButton({ editor }: { editor: Editor }) {
  return (
    <MenuButton ariaLabel="选中整列" onClick={() => selectColumn(editor)}>
      <Columns3 />
    </MenuButton>
  )
}

function InsertRowButton({
  editor,
  dir,
}: {
  editor: Editor
  dir: "before" | "after"
}) {
  return (
    <MenuButton
      ariaLabel={dir === "before" ? "上方加行" : "下方加行"}
      onClick={() =>
        dir === "before"
          ? editor.chain().focus().addRowBefore().run()
          : editor.chain().focus().addRowAfter().run()
      }
    >
      {dir === "before" ? <BetweenVerticalStart /> : <BetweenVerticalEnd />}
    </MenuButton>
  )
}

function InsertColumnButton({
  editor,
  dir,
}: {
  editor: Editor
  dir: "before" | "after"
}) {
  return (
    <MenuButton
      ariaLabel={dir === "before" ? "左侧加列" : "右侧加列"}
      onClick={() =>
        dir === "before"
          ? editor.chain().focus().addColumnBefore().run()
          : editor.chain().focus().addColumnAfter().run()
      }
    >
      {dir === "before" ? <BetweenHorizontalStart /> : <BetweenHorizontalEnd />}
    </MenuButton>
  )
}

function DeleteRowButton({ editor }: { editor: Editor }) {
  return (
    <MenuButton
      ariaLabel="删除行"
      destructive
      onClick={() => editor.chain().focus().deleteRow().run()}
    >
      <Trash2 />
    </MenuButton>
  )
}

function DeleteColumnButton({ editor }: { editor: Editor }) {
  return (
    <MenuButton
      ariaLabel="删除列"
      destructive
      onClick={() => editor.chain().focus().deleteColumn().run()}
    >
      <Trash2 />
    </MenuButton>
  )
}

function DeleteTableButton({ editor }: { editor: Editor }) {
  return (
    <MenuButton
      ariaLabel="删除表格"
      destructive
      onClick={() => editor.chain().focus().deleteTable().run()}
    >
      <Table2 />
    </MenuButton>
  )
}
