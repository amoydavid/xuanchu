import { useQuery } from "@tanstack/react-query"
import { CheckIcon, ChevronsUpDownIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  listWorkspaceMembers,
  type WorkspaceMemberRow,
} from "@/features/workspace/members/members-api"
import { useMe } from "@/features/workspace/session/useMe"
import { cn } from "@/lib/utils"

export type UserPickerProps = {
  /** 选中的 user_id；"" 表示「我自己」语义。 */
  value: string
  onChange: (userId: string) => void
  disabled?: boolean
  className?: string
}

export function UserPicker({
  value,
  onChange,
  disabled = false,
  className,
}: UserPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const me = useMe()
  const slug = me.data?.effective_workspace.slug ?? ""
  const membersQuery = useQuery({
    enabled: slug !== "",
    queryKey: ["workspace", "members", slug],
    queryFn: () => listWorkspaceMembers(slug),
  })
  const members: WorkspaceMemberRow[] = membersQuery.data ?? []
  const selfId = me.data?.actor.id ?? ""

  // value === "" → 我自己；否则匹配 members 中的具体成员
  const selectedMember = members.find((m) => m.user_id === value)
  const selectedName =
    value === ""
      ? t("token.user.self")
      : selectedMember?.display_name ?? selectedMember?.name ?? value

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button
          aria-expanded={open}
          aria-label={t("token.field.user")}
          className={cn("w-full justify-between font-normal", className)}
          disabled={disabled}
          role="combobox"
          type="button"
          variant="outline"
        >
          <span className="truncate">{selectedName}</span>
          <ChevronsUpDownIcon className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-(--radix-popover-trigger-width) p-0"
      >
        <Command>
          <CommandInput placeholder={t("token.user.searchPlaceholder")} />
          <CommandList>
            <CommandEmpty>{t("common.empty")}</CommandEmpty>
            <CommandGroup>
              {/* 「我自己」始终置顶，value="" */}
              <CommandItem
                onSelect={() => {
                  onChange("")
                  setOpen(false)
                }}
                value={`self ${me.data?.actor.name ?? ""} ${me.data?.actor.display_name ?? ""} ${me.data?.actor.email ?? ""}`}
              >
                <CheckIcon
                  className={cn(
                    "mr-2 size-4",
                    value === "" ? "opacity-100" : "opacity-0"
                  )}
                />
                <span>{t("token.user.self")}</span>
                {me.data?.effective_role ? (
                  <span className="ml-auto text-xs text-muted-foreground">
                    {me.data.effective_role}
                  </span>
                ) : null}
              </CommandItem>
              {members
                .filter((m) => m.user_id !== selfId)
                .map((m) => (
                  <CommandItem
                    key={m.user_id}
                    onSelect={() => {
                      onChange(m.user_id)
                      setOpen(false)
                    }}
                    value={`${m.display_name || m.name} ${m.email ?? ""} ${m.name}`}
                  >
                    <CheckIcon
                      className={cn(
                        "mr-2 size-4",
                        value === m.user_id ? "opacity-100" : "opacity-0"
                      )}
                    />
                    <span>{m.display_name || m.name}</span>
                    {m.email ? (
                      <span className="ml-2 truncate text-xs text-muted-foreground">
                        {m.email}
                      </span>
                    ) : null}
                    <span className="ml-auto text-xs text-muted-foreground">
                      {m.role}
                    </span>
                  </CommandItem>
                ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
