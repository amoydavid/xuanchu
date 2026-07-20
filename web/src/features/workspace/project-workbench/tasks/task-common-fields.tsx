import { useEffect, useMemo, useState } from "react"
import { CheckIcon, UserPlusIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { MarkdownEditor, type DeferredAttachment } from "@/components/markdown"
import { resolutionToMenuItem } from "@/components/markdown/reference-suggestion-menu"
import { suggestContentReferences } from "@/features/workspace/content-references"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  getWorkspaceMembers,
  type WorkspaceMemberCandidate,
} from "../api/users-api"
import {
  listWorkspaceTaskUDADefinitions,
  type TaskUDADefinition,
} from "./task-uda-definitions"

export type TaskCommonFieldValue = {
  title: string
  description: string
  priority: string
  assignees: string[]
  tags: string
  udas: Record<string, string>
}

export function TaskCommonFields({
  autoFocus = false,
  disabled = false,
  onChange,
  onSubmit,
  onDeferredAttachment,
  value,
  workspaceSlug,
}: {
  autoFocus?: boolean
  disabled?: boolean
  onChange: (value: TaskCommonFieldValue) => void
  onSubmit?: () => void
  onDeferredAttachment?: (input: DeferredAttachment) => void
  value: TaskCommonFieldValue
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const update = (patch: Partial<TaskCommonFieldValue>) =>
    onChange({ ...value, ...patch })

  return (
    <div className="grid gap-4" data-testid="task-common-fields">
      <div className="grid gap-2">
        <Label htmlFor="task-common-title">{t("taskCreate.title")}</Label>
        <Input
          aria-label={t("taskCreate.title")}
          autoFocus={autoFocus}
          disabled={disabled}
          id="task-common-title"
          onChange={(event) => update({ title: event.target.value })}
          onKeyDown={(event) => {
            if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
              onSubmit?.()
            }
          }}
          placeholder={t("taskCreate.title")}
          value={value.title}
        />
      </div>
      <div className="grid gap-2">
        <Label>{t("taskCreate.details")}</Label>
        <MarkdownEditor
          attachmentContext={{
            workspaceSlug,
            taskRef: "",
            fetchSuggestions: async ({ kind, query, signal }) =>
              (await suggestContentReferences({ type: kind, query, limit: 20 }, { signal }))
                .map(resolutionToMenuItem)
                .filter((item) => item !== null),
          }}
          ariaLabel={t("taskCreate.details")}
          minHeight={150}
          onChange={(description) => update({ description })}
          onDeferredAttachment={onDeferredAttachment}
          onModEnter={() => onSubmit?.()}
          placeholder={t("taskCreate.detailsPlaceholder")}
          value={value.description}
        />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Select
          disabled={disabled}
          onValueChange={(priority) =>
            update({ priority: priority === "none" ? "" : priority })
          }
          value={value.priority || "none"}
        >
          <SelectTrigger aria-label={t("taskCreate.priority")}>
            <SelectValue placeholder={t("taskCreate.priority")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">
              {t("taskSeries.form.noPriority")}
            </SelectItem>
            {(["H", "M", "L"] as const).map((priority) => (
              <SelectItem key={priority} value={priority}>
                {priority}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <CommonAssigneeSelector
          disabled={disabled}
          onChange={(assignees) => update({ assignees })}
          selected={value.assignees}
          workspaceSlug={workspaceSlug}
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="task-common-tags">{t("taskCreate.tags")}</Label>
        <Input
          aria-label={t("taskCreate.tags")}
          disabled={disabled}
          id="task-common-tags"
          onChange={(event) => update({ tags: event.target.value })}
          placeholder={t("taskCreate.tagsPlaceholder")}
          value={value.tags}
        />
      </div>
      <CommonUDAFields
        disabled={disabled}
        onChange={(udas) => update({ udas })}
        value={value.udas}
        workspaceSlug={workspaceSlug}
      />
    </div>
  )
}

function CommonAssigneeSelector({
  disabled,
  onChange,
  selected,
  workspaceSlug,
}: {
  disabled: boolean
  onChange: (selected: string[]) => void
  selected: string[]
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [members, setMembers] = useState<WorkspaceMemberCandidate[]>([])
  const [loaded, setLoaded] = useState(false)
  const selectedSet = useMemo(() => new Set(selected), [selected])
  const memberByID = useMemo(
    () => new Map(members.map((member) => [member.id, member])),
    [members]
  )
  // 挂载即加载成员：编辑场景回填的 assignee id 需要立刻解析为姓名，
  // 而不是等用户打开下拉（否则触发器和 chip 会先显示裸 UUID）。
  useEffect(() => {
    if (loaded) return
    let active = true
    void getWorkspaceMembers(workspaceSlug)
      .then((items) => {
        if (active) {
          setMembers(items)
          setLoaded(true)
        }
      })
      .catch(() => {
        if (active) setLoaded(true)
      })
    return () => {
      active = false
    }
  }, [loaded, workspaceSlug])
  const handleOpenChange = (next: boolean) => {
    setOpen(next)
  }

  return (
    <div className="space-y-2">
      <Popover open={open} onOpenChange={handleOpenChange}>
        <PopoverTrigger asChild>
          <Button
            className="w-full justify-start overflow-hidden font-normal"
            disabled={disabled}
            type="button"
            variant="outline"
          >
            <UserPlusIcon />
            <span className="truncate">
              {selected.length > 0
                ? selected
                    .map((id) => memberLabel(memberByID.get(id)) || id)
                    .join(", ")
                : t("taskCreate.selectAssignees")}
            </span>
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-80 space-y-2">
          <div className="max-h-64 space-y-1 overflow-auto">
            {!loaded ? (
              <p className="px-2 py-3 text-sm text-muted-foreground">
                {t("taskCreate.loadingAssignees")}
              </p>
            ) : members.length === 0 ? (
              <p className="px-2 py-3 text-sm text-muted-foreground">
                {t("taskCreate.noAssignees")}
              </p>
            ) : (
              members.map((member) => {
                const checked = selectedSet.has(member.id)
                const label = memberLabel(member)
                return (
                  <label
                    className="flex cursor-pointer items-center gap-3 rounded-md px-2 py-2 text-sm hover:bg-muted"
                    key={member.id}
                  >
                    <Checkbox
                      aria-label={`${label} ${member.email ?? ""}`.trim()}
                      checked={checked}
                      onCheckedChange={(next) =>
                        onChange(
                          next
                            ? Array.from(new Set([...selected, member.id]))
                            : selected.filter((id) => id !== member.id)
                        )
                      }
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">
                        {label}
                      </span>
                      {member.email ? (
                        <span className="block truncate text-xs text-muted-foreground">
                          {member.email}
                        </span>
                      ) : null}
                    </span>
                    {checked ? <CheckIcon className="size-4" /> : null}
                  </label>
                )
              })
            )}
          </div>
          <div className="flex justify-between gap-2">
            <Button
              onClick={() => onChange([])}
              size="sm"
              type="button"
              variant="outline"
            >
              {t("taskCreate.clearAssignees")}
            </Button>
            <Button onClick={() => setOpen(false)} size="sm" type="button">
              {t("taskCreate.assigneeDone")}
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      {selected.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {selected.map((id) => (
            <Badge key={id} variant="secondary">
              {memberLabel(memberByID.get(id)) || id}
            </Badge>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function CommonUDAFields({
  disabled,
  onChange,
  value,
  workspaceSlug,
}: {
  disabled: boolean
  onChange: (value: Record<string, string>) => void
  value: Record<string, string>
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const [definitions, setDefinitions] = useState<TaskUDADefinition[]>([])

  useEffect(() => {
    let active = true
    void listWorkspaceTaskUDADefinitions(workspaceSlug)
      .then((next) => {
        if (active) setDefinitions(next)
      })
      .catch(() => {
        if (active) setDefinitions([])
      })
    return () => {
      active = false
    }
  }, [workspaceSlug])

  if (definitions.length === 0) return null

  const updateField = (name: string, raw: string) => {
    const next = { ...value }
    if (raw === "") delete next[name]
    else next[name] = raw
    onChange(next)
  }

  return (
    <div className="space-y-3">
      <Label>{t("taskCreate.customFields")}</Label>
      <div className="grid gap-3 sm:grid-cols-2">
        {definitions.map((definition) => (
          <TaskUDAField
            definition={definition}
            disabled={disabled}
            key={definition.name}
            onChange={(raw) => updateField(definition.name, raw)}
            value={value[definition.name] ?? ""}
          />
        ))}
      </div>
    </div>
  )
}

function TaskUDAField({
  definition,
  disabled,
  onChange,
  value,
}: {
  definition: TaskUDADefinition
  disabled: boolean
  onChange: (value: string) => void
  value: string
}) {
  const { t } = useTranslation()
  const id = `task-common-uda-${definition.name.replace(/[^a-zA-Z0-9_-]/g, "-")}`
  const label = definition.label || definition.name
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>
        {label}
        {label !== definition.name ? (
          <span className="font-normal text-muted-foreground">
            {definition.name}
          </span>
        ) : null}
      </Label>
      {definition.values.length > 0 ? (
        <Select
          disabled={disabled}
          onValueChange={(next) => onChange(next === "__unset__" ? "" : next)}
          value={value || "__unset__"}
        >
          <SelectTrigger aria-label={label} id={id}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__unset__">
              {t("taskCreate.customFieldUnset")}
            </SelectItem>
            {definition.values.map((option) => (
              <SelectItem key={option} value={option}>
                {option}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <Input
          aria-label={label}
          disabled={disabled}
          id={id}
          inputMode={definition.type === "numeric" ? "decimal" : undefined}
          onChange={(event) => onChange(event.target.value)}
          placeholder={definition.defaultValue ?? undefined}
          type={definition.type === "date" ? "date" : "text"}
          value={formatUDAInputValue(definition.type, value)}
        />
      )}
    </div>
  )
}

function formatUDAInputValue(type: TaskUDADefinition["type"], value: string) {
  if (type !== "date" || !value) return value
  return value.slice(0, 10)
}

function memberLabel(member?: WorkspaceMemberCandidate): string {
  return member?.display_name || member?.name || member?.email || ""
}
