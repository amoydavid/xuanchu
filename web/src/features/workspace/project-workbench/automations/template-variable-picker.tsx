import { useQuery } from "@tanstack/react-query"
import { useCallback, useState } from "react"
import { VariableIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { listProjectConfig } from "@/features/workspace/project-workbench/api/project-api"
import { installWheelScrollIsolation } from "@/lib/scroll-propagation"

import { useAutomationTemplateVars } from "./project-automations-api"

type Props = {
  projectSlug: string
  workspaceSlug: string
  trigger: "schedule" | "event"
  onInsert: (token: string) => void
  disabled?: boolean
}

// TemplateVariablePicker 是「插入变量」按钮 + Popover 变量选择器。
// 点击按钮打开 Popover，展示当前触发器下可用变量和项目配置项，
// 点击追加 {{变量名}} 到 Textarea。
export function TemplateVariablePicker({ projectSlug, workspaceSlug, trigger, onInsert, disabled }: Props) {
  const [open, setOpen] = useState(false)
  const { data } = useAutomationTemplateVars(projectSlug)

  // Popover 通过 portal 挂到 Dialog 外；在 CommandList 上使用原生 wheel 隔离，
  // 避免 react-remove-scroll 的 document 监听器取消列表滚动。使用 callback ref
  // 可确保监听器在 Radix 延迟挂载 portal 内容时仍会注册，并在卸载时自动清理。
  const setListRef = useCallback((element: HTMLDivElement | null) => {
    if (!element) return
    return installWheelScrollIsolation(element)
  }, [])

  // 获取项目配置项（非 secret），用于 project_config:xxx 变量。
  const configQuery = useQuery({
    queryKey: ["project", projectSlug, "config"],
    queryFn: () => listProjectConfig(workspaceSlug, projectSlug),
    enabled: open,
  })

  // 找到当前触发器对应的变量列表，过滤掉 is_prefix 的（它只是提示，实际选择 config key）。
  const triggerGroup = data?.triggers?.find((t) => t.trigger === trigger)
  const vars = (triggerGroup?.vars ?? []).filter((v) => !v.is_prefix)
  const configEntries = configQuery.data ?? []

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={disabled}>
          <VariableIcon className="mr-1 h-3 w-3" />
          插入变量
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-0" align="start">
        <Command className="overflow-hidden">
          <CommandInput placeholder="搜索变量..." />
          <CommandList ref={setListRef} className="max-h-[300px] overflow-y-auto" style={{ overscrollBehavior: "contain" }}>
            <CommandEmpty>无匹配变量</CommandEmpty>
            <CommandGroup heading="通用变量">
              {vars.map((v) => (
                <CommandItem
                  key={v.name}
                  value={v.name}
                  onSelect={() => {
                    onInsert(`{{${v.name}}}`)
                    setOpen(false)
                  }}
                >
                  <div className="flex flex-col">
                    <span className="font-mono text-xs">{`{{${v.name}}}`}</span>
                    <span className="text-xs text-muted-foreground">{v.description}</span>
                  </div>
                </CommandItem>
              ))}
            </CommandGroup>
            {configEntries.length > 0 ? (
              <CommandGroup heading="项目配置项">
                {configEntries.map((entry) => (
                  <CommandItem
                    key={entry.key}
                    value={entry.key}
                    onSelect={() => {
                      onInsert(`{{project_config:${entry.key}}}`)
                      setOpen(false)
                    }}
                  >
                    <div className="flex flex-col">
                      <span className="font-mono text-xs">{`{{project_config:${entry.key}}}`}</span>
                      <span className="max-w-[260px] truncate text-xs text-muted-foreground">{entry.value}</span>
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            ) : null}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
