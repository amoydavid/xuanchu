import { useMemo, useState } from "react"
import { CalendarIcon, EyeIcon, EyeOffIcon, XIcon } from "lucide-react"
import { useTranslation } from "react-i18next"
import { format, parseISO } from "date-fns"

import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Input } from "@/components/ui/input"
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
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import type { ConfigValueType } from "./config-definition-api"

type ConfigValueControlProps = {
  definition: Pick<ConfigSchemaControlDefinition, "value_type" | "enum_values" | "secret">
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  id?: string
  allowEmpty?: boolean
  allowSecretReveal?: boolean
  ariaDescribedBy?: string
  ariaInvalid?: boolean
}

// 只依赖 schema 的少量字段，便于 project-config-row 等复用。
type ConfigSchemaControlDefinition = {
  value_type: ConfigValueType | string
  enum_values: string[]
  secret: boolean
}

export function ConfigValueControl({
  definition,
  value,
  onChange,
  disabled,
  id,
  allowEmpty = false,
  allowSecretReveal = true,
  ariaDescribedBy,
  ariaInvalid,
}: ConfigValueControlProps) {
  const { t } = useTranslation()
  const enumValues = definition.enum_values ?? []

  // secret reveal 状态只在控件内部维护；外部 value 始终是明文。
  const [revealed, setRevealed] = useState(false)

  // enum 优先级最高
  if (enumValues.length > 0) {
    return (
      <Select
        disabled={disabled}
        onValueChange={(next) => onChange(next === EMPTY_VALUE ? "" : next)}
        value={allowEmpty && value === "" ? EMPTY_VALUE : value}
      >
        <SelectTrigger aria-describedby={ariaDescribedBy} aria-invalid={ariaInvalid} className="w-full" id={id}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {allowEmpty ? <SelectItem value={EMPTY_VALUE}>未选择</SelectItem> : null}
          {enumValues.map((v) => (
            <SelectItem key={v} value={v}>
              {v}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    )
  }

  if (definition.value_type === "boolean") {
    if (allowEmpty) {
      return (
        <Select
          disabled={disabled}
          onValueChange={(next) => onChange(next === EMPTY_VALUE ? "" : next)}
          value={value === "" ? EMPTY_VALUE : value}
        >
          <SelectTrigger aria-describedby={ariaDescribedBy} aria-invalid={ariaInvalid} className="w-full" id={id}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={EMPTY_VALUE}>未选择</SelectItem>
            <SelectItem value="true">是</SelectItem>
            <SelectItem value="false">否</SelectItem>
          </SelectContent>
        </Select>
      )
    }
    const checked = value === "true"
    return (
      <Switch
        aria-label={t("configDefinitions.value")}
        checked={checked}
        disabled={disabled}
        id={id}
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid}
        onCheckedChange={(next) => onChange(next ? "true" : "false")}
      />
    )
  }

  if (definition.value_type === "json") {
    return (
      <JsonControl
        disabled={disabled}
        id={id}
        ariaDescribedBy={ariaDescribedBy}
        ariaInvalid={ariaInvalid}
        invalidHint={t("configDefinitions.jsonInvalid")}
        onChange={onChange}
        value={value}
      />
    )
  }

  if (definition.value_type === "date") {
    return (
      <DateControl
        disabled={disabled}
        id={id}
        ariaDescribedBy={ariaDescribedBy}
        ariaInvalid={ariaInvalid}
        onChange={onChange}
        placeholder={t("configDefinitions.datePlaceholder")}
        clearLabel={t("configDefinitions.clearDate")}
        value={value}
      />
    )
  }

  if (definition.value_type === "datetime") {
    return (
      <DateTimeControl
        disabled={disabled}
        id={id}
        ariaDescribedBy={ariaDescribedBy}
        ariaInvalid={ariaInvalid}
        onChange={onChange}
        placeholder={t("configDefinitions.datetimePlaceholder")}
        timeLabel={t("configDefinitions.timeLabel")}
        clearLabel={t("configDefinitions.clearDate")}
        value={value}
      />
    )
  }

  const isSecret = definition.secret === true
  if (isSecret) {
    return (
      <div className="flex items-center gap-1">
        <Input
          disabled={disabled}
          id={id}
          aria-describedby={ariaDescribedBy}
          aria-invalid={ariaInvalid}
          onChange={(e) => onChange(e.target.value)}
          type={revealed ? "text" : "password"}
          value={value}
        />
        {allowSecretReveal ? <Button
          aria-label={
            revealed
              ? t("configDefinitions.hideSecret")
              : t("configDefinitions.revealSecret")
          }
          onClick={() => setRevealed((v) => !v)}
          size="icon-sm"
          type="button"
          variant="ghost"
        >
          {revealed ? <EyeOffIcon /> : <EyeIcon />}
        </Button> : null}
      </div>
    )
  }

  return (
    <Input
      disabled={disabled}
      id={id}
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid}
      onChange={(e) => onChange(e.target.value)}
      type={definition.value_type === "number" ? "number" : "text"}
      value={value}
    />
  )
}

const EMPTY_VALUE = "__xuanchu_empty__"

// JsonControl 只做 parse 提示，不阻止输入。
function JsonControl(props: {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  id?: string
  invalidHint: string
  ariaDescribedBy?: string
  ariaInvalid?: boolean
}) {
  const { value, onChange, disabled, id, invalidHint, ariaDescribedBy, ariaInvalid } = props
  const isInvalid = useMemo(() => isInvalidJson(value), [value])
  return (
    <div className="space-y-1">
      <Textarea
        className="font-mono text-xs"
        disabled={disabled}
        id={id}
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid}
        onChange={(e) => onChange(e.target.value)}
        rows={4}
        value={value}
      />
      {isInvalid ? <p className="text-xs text-destructive">{invalidHint}</p> : null}
    </div>
  )
}

function isInvalidJson(value: string): boolean {
  if (value.trim() === "") return false
  try {
    JSON.parse(value)
    return false
  } catch {
    return true
  }
}

// DateControl: date 类型编辑控件。value 是 YYYY-MM-DD 字符串。
function DateControl(props: {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  id?: string
  placeholder: string
  clearLabel: string
  ariaDescribedBy?: string
  ariaInvalid?: boolean
}) {
  const { value, onChange, disabled, id, placeholder, clearLabel, ariaDescribedBy, ariaInvalid } = props
  const [open, setOpen] = useState(false)
  const selected = useMemo(() => safeParseISO(value), [value])
  const label = selected ? format(selected, "yyyy-MM-dd") : ""

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          aria-label={placeholder}
          aria-describedby={ariaDescribedBy}
          aria-invalid={ariaInvalid}
          className="w-full justify-start font-normal"
          data-empty={!selected}
          disabled={disabled}
          id={id}
          size="sm"
          type="button"
          variant="outline"
        >
          <CalendarIcon className="size-4" />
          <span className={cn(!selected && "text-muted-foreground")}>
            {selected ? label : placeholder}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto p-0">
        <Calendar
          captionLayout="dropdown"
          defaultMonth={selected ?? undefined}
          mode="single"
          onSelect={(next) => {
            if (next) {
              onChange(format(next, "yyyy-MM-dd"))
              setOpen(false)
            }
          }}
          selected={selected ?? undefined}
        />
        {selected ? (
          <div className="border-t p-2">
            <Button
              className="w-full justify-start"
              onClick={() => onChange("")}
              size="xs"
              type="button"
              variant="ghost"
            >
              <XIcon className="size-4" />
              {clearLabel}
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}

// DateTimeControl: datetime 类型编辑控件。value 是 RFC3339 UTC 字符串。
// Calendar 选日期 + <input type="time"> 选时间；输出统一 toISOString()（UTC）。
function DateTimeControl(props: {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  id?: string
  placeholder: string
  timeLabel: string
  clearLabel: string
  ariaDescribedBy?: string
  ariaInvalid?: boolean
}) {
  const { value, onChange, disabled, id, placeholder, timeLabel, clearLabel, ariaDescribedBy, ariaInvalid } = props
  const [open, setOpen] = useState(false)
  // 拆出日期段（本地日历用）和时间段（time input 用），都基于本地时区展示。
  const parsed = useMemo(() => safeParseISO(value), [value])
  const dateForCalendar = parsed ?? null
  // time input 用 HH:mm，从本地时区的展示取
  const timeValue = parsed ? format(parsed, "HH:mm") : ""

  const applyDate = (next: Date | undefined) => {
    if (!next) return
    // 保留现有时间段（如果有），否则用 00:00
    const base = parsed
    if (base) {
      const merged = new Date(next)
      merged.setHours(base.getHours(), base.getMinutes(), 0, 0)
      onChange(merged.toISOString())
    } else {
      next.setHours(0, 0, 0, 0)
      onChange(next.toISOString())
    }
    setOpen(false)
  }

  const applyTime = (hhmm: string) => {
    if (!/^\d{2}:\d{2}$/.test(hhmm)) return
    const [h, m] = hhmm.split(":").map(Number)
    const base = parsed ? new Date(parsed) : new Date()
    base.setHours(h, m, 0, 0)
    onChange(base.toISOString())
  }

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-1">
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              aria-label={placeholder}
              aria-describedby={ariaDescribedBy}
              aria-invalid={ariaInvalid}
              className="flex-1 justify-start font-normal"
              data-empty={!parsed}
              disabled={disabled}
              id={id}
              size="sm"
              type="button"
              variant="outline"
            >
              <CalendarIcon className="size-4" />
              <span className={cn(!parsed && "text-muted-foreground")}>
                {parsed ? format(parsed, "yyyy-MM-dd") : placeholder}
              </span>
            </Button>
          </PopoverTrigger>
          <PopoverContent align="start" className="w-auto p-0">
            <Calendar
              captionLayout="dropdown"
              defaultMonth={dateForCalendar ?? undefined}
              mode="single"
              onSelect={applyDate}
              selected={dateForCalendar ?? undefined}
            />
            {parsed ? (
              <div className="border-t p-2">
                <Button
                  className="w-full justify-start"
                  onClick={() => onChange("")}
                  size="xs"
                  type="button"
                  variant="ghost"
                >
                  <XIcon className="size-4" />
                  {clearLabel}
                </Button>
              </div>
            ) : null}
          </PopoverContent>
        </Popover>
        <Input
          aria-label={timeLabel}
          className="w-28"
          disabled={disabled || !parsed}
          onChange={(e) => applyTime(e.target.value)}
          type="time"
          value={timeValue}
        />
      </div>
    </div>
  )
}

function safeParseISO(value: string): Date | null {
  if (!value) return null
  try {
    const parsed = parseISO(value)
    if (Number.isNaN(parsed.getTime())) return null
    return parsed
  } catch {
    return null
  }
}
