import { useState } from "react"
import { VariableIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"

import { useWorkspaceAutomationTemplateVars } from "@/features/workspace/automations/workspace-automations-api"

type Props = {
  trigger: "schedule" | "event"
  onInsert: (token: string) => void
  disabled?: boolean
}

// WorkspaceTemplateVariablePicker 是 Workspace 自动化规则 Dialog 里的「插入变量」选择器。
// 读取 /api/v1/automations/template-vars，按 trigger 分组展示可用变量，
// 点击追加 {{变量名}} 到 Textarea。
// 与 Project 版本的区别：不展示 project_config:key（Workspace 规则的 project_config
// 在运行时才确定，不能在编辑时绑定具体 key），只展示通用变量。
export function WorkspaceTemplateVariablePicker({ trigger, onInsert, disabled }: Props) {
  const [open, setOpen] = useState(false)
  const { data } = useWorkspaceAutomationTemplateVars()

  const triggerGroup = data?.triggers?.find((t) => t.trigger === trigger)
  const vars = (triggerGroup?.vars ?? []).filter((v) => !v.is_prefix)

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
          <CommandList className="max-h-[300px] overflow-y-auto" style={{ overscrollBehavior: "contain" }}>
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
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
