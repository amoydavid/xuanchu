"use client"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"
import {
  CRON_PRESETS,
  TIMEZONE_OPTIONS,
  describeCron,
  isValidCronShape,
} from "./cron-schedule"

/**
 * CronScheduleInput 是可复用的 cron 表达式可视化输入器。
 *
 * 包含：表达式输入框（带实时校验）、6 个常用预设按钮、时区下拉、中文解读区。
 * 受控组件，调用方通过 value/timezone/onChange 持有状态。
 */
export interface CronScheduleInputProps {
  /** cron 表达式，如 "0 9 * * 1-5" */
  value: string
  /** IANA 时区，如 "Asia/Shanghai" */
  timezone: string
  /** 表达式与时区同时变化的回调 */
  onChange: (next: { value: string; timezone: string }) => void
  /** 是否禁用 */
  disabled?: boolean
  /** 标签文本，默认"cron 表达式" */
  label?: string
}

export function CronScheduleInput({
  value,
  timezone,
  onChange,
  disabled,
  label = "cron 表达式",
}: CronScheduleInputProps) {
  const valid = value.trim() === "" ? null : isValidCronShape(value)
  const description = value.trim() === "" ? "" : describeCron(value)
  const matchedPreset = CRON_PRESETS.find((p) => p.expr === value.trim())

  return (
    <div className="grid gap-3">
      <div className="grid gap-2">
        <Label className="text-sm font-medium">{label}</Label>
        <div className="flex items-center gap-2">
          <Input
            aria-label={label}
            aria-invalid={valid === false}
            value={value}
            onChange={(e) => onChange({ value: e.target.value, timezone })}
            disabled={disabled}
            placeholder="0 9 * * 1-5（分 时 日 月 周）"
            className="font-mono"
          />
          {valid === true && <span className="text-xs text-primary">✓</span>}
          {valid === false && <span className="text-xs text-destructive">✗</span>}
        </div>
      </div>

      <div className="grid gap-2">
        <Label className="text-xs text-muted-foreground">常用预设</Label>
        <div className="flex flex-wrap gap-1.5">
          {CRON_PRESETS.map((preset) => (
            <Button
              key={preset.expr}
              type="button"
              variant="outline"
              size="xs"
              disabled={disabled}
              data-active={matchedPreset?.expr === preset.expr}
              className={cn(
                "h-7 px-2 text-xs",
                matchedPreset?.expr === preset.expr &&
                  "border-primary bg-primary/10 text-primary",
              )}
              onClick={() => onChange({ value: preset.expr, timezone })}
            >
              {preset.label}
            </Button>
          ))}
        </div>
      </div>

      <div className="grid gap-2">
        <Label className="text-xs text-muted-foreground">时区</Label>
        <Select
          value={timezone || "Asia/Shanghai"}
          onValueChange={(tz) => onChange({ value, timezone: tz })}
          disabled={disabled}
        >
          <SelectTrigger className="w-full" aria-label="时区">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIMEZONE_OPTIONS.map((tz) => (
              <SelectItem key={tz} value={tz}>
                {tz}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {description && valid !== false && (
        <div className="rounded-md border border-primary/20 bg-primary/5 px-3 py-2 text-xs text-foreground">
          💡 {description}
        </div>
      )}
      {valid === false && (
        <div className="rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-xs text-destructive">
          ⚠ 表达式格式不正确，应为 5 段（分 时 日 月 周）
        </div>
      )}
    </div>
  )
}
