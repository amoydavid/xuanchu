import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { VariableIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { listConfigSchema } from "@/features/workspace/config/config-definition-api"

import { useWorkspaceAutomationTemplateVars } from "@/features/workspace/automations/workspace-automations-api"

type Props = {
  trigger: "schedule" | "event"
  onInsert: (token: string) => void
  disabled?: boolean
}

// WorkspaceTemplateVariablePicker 是 Workspace 自动化规则 Dialog 里的「插入变量」选择器。
// 读取 /api/v1/automations/template-vars，按 trigger 分组展示可用变量，
// 点击追加 {{变量名}} 到 Textarea。
//
// 对于 event 触发（如 project.created），事件一定关联某个 Project，因此也会展示
// workspace 级 config schema 中所有非 secret 的 key 作为 {{project_config:key}} 变量。
// schedule 触发不绑定 Project，不展示 project_config 变量。
export function WorkspaceTemplateVariablePicker({ trigger, onInsert, disabled }: Props) {
  const [open, setOpen] = useState(false)
  const { data } = useWorkspaceAutomationTemplateVars()

  // event 触发时加载 config schema，展示非 secret key 作为 project_config 变量。
  const schemaQuery = useQuery({
    queryKey: ["config-schema"],
    queryFn: () => listConfigSchema(),
    enabled: open && trigger === "event",
  })

  const triggerGroup = data?.triggers?.find((t) => t.trigger === trigger)
  const vars = (triggerGroup?.vars ?? []).filter((v) => !v.is_prefix)
  // 过滤出 project scope 允许且非 secret 的 config key。
  const configKeys = (schemaQuery.data ?? [])
    .filter((d) => !d.secret && d.allowed_scopes.includes("project"))
    .map((d) => d.key)

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
            {configKeys.length > 0 ? (
              <CommandGroup heading="项目配置项（事件关联 Project）">
                {configKeys.map((key) => (
                  <CommandItem
                    key={key}
                    value={key}
                    onSelect={() => {
                      onInsert(`{{project_config:${key}}}`)
                      setOpen(false)
                    }}
                  >
                    <div className="flex flex-col">
                      <span className="font-mono text-xs">{`{{project_config:${key}}}`}</span>
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

