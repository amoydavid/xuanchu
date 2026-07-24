import { Search, Trash2, X } from "lucide-react"
import { useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

import type { CandidateKind } from "../api/project-template-api"
import type {
  CandidateSummary,
  CandidateSelectionStore,
} from "./candidate-picker"

const kindLabel: Record<CandidateKind, string> = {
  task: "任务",
  series: "循环任务",
  config: "配置",
  automation: "自动化",
}

type SelectedItemsSheetProps = {
  lockedConfigKeys?: Set<string>
  onClearAll: () => void
  onOpenChange: (open: boolean) => void
  onRemove: (kind: CandidateKind, ref: string) => void
  open: boolean
  selection: CandidateSelectionStore
}

export function SelectedItemsSheet({
  lockedConfigKeys,
  onClearAll,
  onOpenChange,
  onRemove,
  open,
  selection,
}: SelectedItemsSheetProps) {
  const [q, setQ] = useState("")
  const items = useMemo(() => selectedItems(selection, q), [q, selection])

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent
        aria-describedby="selected-items-description"
        className="w-screen max-w-none border-l sm:w-[28rem] sm:max-w-[90vw]"
        side="right"
      >
        <SheetHeader className="h-auto min-h-14 flex-col items-start gap-0.5 py-3 pr-12">
          <SheetTitle>已选内容</SheetTitle>
          <SheetDescription id="selected-items-description">
            显式选择会在切换分类、筛选和翻页时保留。
          </SheetDescription>
        </SheetHeader>
        <SelectedItemsPanel
          items={items}
          lockedConfigKeys={lockedConfigKeys}
          onClearAll={onClearAll}
          onQueryChange={setQ}
          onRemove={onRemove}
          query={q}
        />
      </SheetContent>
    </Sheet>
  )
}

export function SelectedItemsDrawer({
  lockedConfigKeys,
  onClearAll,
  onRemove,
  selection,
}: Pick<
  SelectedItemsSheetProps,
  "lockedConfigKeys" | "onClearAll" | "onRemove" | "selection"
>) {
  const [q, setQ] = useState("")
  const items = useMemo(() => selectedItems(selection, q), [q, selection])
  return (
    <aside
      aria-label="已选内容"
      className="rounded-lg sticky top-4 hidden max-h-[calc(100vh-8rem)] min-h-0 flex-col border bg-background lg:flex"
    >
      <div className="border-b px-3 py-2">
        <div className="text-sm font-medium">已选内容</div>
        <div className="text-xs text-muted-foreground">跨页显式清单</div>
      </div>
      <SelectedItemsPanel
        items={items}
        lockedConfigKeys={lockedConfigKeys}
        onClearAll={onClearAll}
        onQueryChange={setQ}
        onRemove={onRemove}
        query={q}
      />
    </aside>
  )
}

function SelectedItemsPanel({
  items,
  lockedConfigKeys = new Set(),
  onClearAll,
  onQueryChange,
  onRemove,
  query,
}: {
  items: Array<CandidateSummary & { kind: CandidateKind }>
  lockedConfigKeys?: Set<string>
  onClearAll: () => void
  onQueryChange: (value: string) => void
  onRemove: (kind: CandidateKind, ref: string) => void
  query: string
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-b p-3">
        <div className="relative">
          <Search className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
          <Input
            aria-label="搜索已选内容"
            className="pl-8"
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="搜索已选内容"
            value={query}
          />
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {items.length === 0 ? (
          <p className="p-5 text-center text-xs text-muted-foreground">
            {query ? "没有匹配的已选内容" : "尚未选择内容"}
          </p>
        ) : (
          <ul className="divide-y">
            {items.map((item) => (
              <li
                className="flex items-start gap-2 p-3"
                key={`${item.kind}:${item.ref}`}
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">
                    {item.label}
                  </div>
                  <div className="mt-1 flex items-center gap-2">
                    <Badge variant="outline">{kindLabel[item.kind]}</Badge>
                    <span className="truncate font-mono text-[10px] text-muted-foreground">
                      {item.ref}
                    </span>
                  </div>
				  {item.kind === "config" && lockedConfigKeys.has(item.ref) ? (
					  <div className="mt-1 text-[11px] text-muted-foreground">由已选自动化依赖，取消自动化后才可移除</div>
				  ) : null}
                </div>
                <Button
                  aria-label={`移除 ${item.label}`}
				  disabled={item.kind === "config" && lockedConfigKeys.has(item.ref)}
                  onClick={() => onRemove(item.kind, item.ref)}
                  size="icon-sm"
                  variant="ghost"
                >
                  <X />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
      <div className="border-t p-3">
        <Button
          className="w-full"
          disabled={totalSelected(items) === 0 && !query}
          onClick={onClearAll}
          variant="outline"
        >
          <Trash2 />
          清除全部已选
        </Button>
      </div>
    </div>
  )
}

function selectedItems(selection: CandidateSelectionStore, q: string) {
  const needle = q.trim().toLocaleLowerCase()
  return (Object.keys(selection) as CandidateKind[]).flatMap((kind) =>
    [...selection[kind].values()]
      .filter((item) =>
        needle
          ? `${item.label} ${item.ref}`.toLocaleLowerCase().includes(needle)
          : true
      )
      .map((item) => ({ ...item, kind }))
  )
}

function totalSelected(items: Array<unknown>) {
  return items.length
}
