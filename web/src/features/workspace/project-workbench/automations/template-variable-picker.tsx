import { useState } from "react"
import { VariableIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"

import { useAutomationTemplateVars } from "./project-automations-api"

type Props = {
  projectSlug: string
  trigger: "schedule" | "event"
  onInsert: (token: string) => void
  disabled?: boolean
}

// TemplateVariablePicker 是「插入变量」按钮 + Popover 变量选择器。
// 点击按钮打开 Popover，展示当前触发器下可用变量，点击追加 {{变量名}} 到 Textarea。
export function TemplateVariablePicker({ projectSlug, trigger, onInsert, disabled }: Props) {
  const [open, setOpen] = useState(false)
  const { data } = useAutomationTemplateVars(projectSlug)

  // 找到当前触发器对应的变量列表。
  const triggerGroup = data?.triggers?.find((t) => t.trigger === trigger)
  const vars = triggerGroup?.vars ?? []

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={disabled}>
          <VariableIcon className="mr-1 h-3 w-3" />
          插入变量
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0" align="start">
        <Command>
          <CommandInput placeholder="搜索变量..." />
          <CommandList>
            <CommandEmpty>无匹配变量</CommandEmpty>
            <CommandGroup>
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
