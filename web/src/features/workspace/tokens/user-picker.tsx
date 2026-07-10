import { useQuery } from "@tanstack/react-query"
import { CheckIcon, ChevronsUpDownIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import { useEffect, useMemo, useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandLoading,
} from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  listWorkspaceMembers,
  type WorkspaceMemberRow,
} from "@/features/workspace/members/members-api"
import { memberRoleLabel } from "@/features/workspace/members/role-label"
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
  // useMemo 稳定 members 引用，避免下游 useEffect 因 ?? [] 每次产生新数组而频繁触发。
  const members: WorkspaceMemberRow[] = useMemo(
    () => membersQuery.data ?? [],
    [membersQuery.data]
  )
  const selfId = me.data?.actor.id ?? ""
  const selfRole = me.data?.effective_role ?? ""

  // 选中的成员已离开 workspace（成员列表加载完成后找不到匹配）时，重置为「我自己」，
  // 避免 trigger 显示裸 user_id 并持续查询一个不可达的用户。
  useEffect(() => {
    if (
      value !== "" &&
      !membersQuery.isLoading &&
      !members.some((m) => m.id === value)
    ) {
      onChange("")
    }
  }, [value, members, membersQuery.isLoading, onChange])

  // value === "" → 我自己；否则匹配 members 中的具体成员
  const selectedMember = members.find((m) => m.id === value)
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
            {/* 成员列表加载中：cmdk 的 CommandLoading 不会参与搜索过滤，避免
                误显示「暂无数据」。 */}
            {membersQuery.isLoading ? (
              <CommandLoading>{t("common.loading")}</CommandLoading>
            ) : (
              <>
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
                    {selfRole ? (
                      <span className="ml-auto text-xs text-muted-foreground">
                        {memberRoleLabel(t, selfRole)}
                      </span>
                    ) : null}
                  </CommandItem>
                  {members
                    // me 未加载完成时（selfId 为空）排除全部成员，避免与「我自己」
                    // 同时出现当前用户的两行；me 加载完成后正常按 selfId 过滤。
                    .filter((m) => selfId !== "" && m.id !== selfId)
                    .map((m) => (
                      <CommandItem
                        key={m.id}
                        onSelect={() => {
                          onChange(m.id)
                          setOpen(false)
                        }}
                        // value 只用于 cmdk 搜索评分；onSelect 用闭包里的 user_id，
                        // 重名成员不会互相覆盖。
                        value={`${m.display_name || m.name} ${m.email ?? ""} ${m.name}`}
                      >
                        <CheckIcon
                          className={cn(
                            "mr-2 size-4",
                            value === m.id ? "opacity-100" : "opacity-0"
                          )}
                        />
                        <span>{m.display_name || m.name}</span>
                        {m.email ? (
                          <span className="ml-2 truncate text-xs text-muted-foreground">
                            {m.email}
                          </span>
                        ) : null}
                        <span className="ml-auto text-xs text-muted-foreground">
                          {memberRoleLabel(t, m.role)}
                        </span>
                      </CommandItem>
                    ))}
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
