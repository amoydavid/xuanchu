import { CalendarIcon, XIcon } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { cn } from "@/lib/utils"
import { useEditFeedback } from "./edit-feedback"

type InlineDatePickerProps = {
  ariaLabel: string
  value?: number | null
  onSave: (value: number | null) => Promise<void> | void
  className?: string
  disabled?: boolean
  emptyLabel?: string
}

export function InlineDatePicker({
  ariaLabel,
  className,
  disabled = false,
  emptyLabel = "-",
  onSave,
  value,
}: InlineDatePickerProps) {
  const { t } = useTranslation()
  const feedback = useEditFeedback()
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const date = unixToDate(value)
  const label = date ? formatDate(date) : emptyLabel

  const save = async (next: number | null) => {
    if ((value ?? null) === next) {
      setOpen(false)
      return
    }
    setSaving(true)
    setError(null)
    try {
      await onSave(next)
      setOpen(false)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure(ariaLabel, message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <span className="inline-flex w-full flex-col gap-1">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            aria-label={ariaLabel}
            className={cn("w-full justify-start font-normal", className)}
            data-empty={!date}
            disabled={disabled || saving}
            size="sm"
            type="button"
            variant="outline"
          >
            <CalendarIcon data-icon="inline-start" />
            <span className={cn(!date && "text-muted-foreground")}>{label}</span>
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-auto p-0">
          <Calendar
            captionLayout="dropdown"
            defaultMonth={date ?? undefined}
            mode="single"
            onSelect={(next) => {
              void save(next ? dateToUnix(next) : null)
            }}
            selected={date ?? undefined}
          />
          <div className="border-t p-2">
            <Button
              className="w-full justify-start"
              disabled={saving || !date}
              onClick={() => {
                void save(null)
              }}
              size="xs"
              type="button"
              variant="ghost"
            >
              <XIcon data-icon="inline-start" />
              {t("projectReadonly.clearDate")}
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      {error ? <span className="text-xs text-destructive">{error}</span> : null}
    </span>
  )
}

function unixToDate(value?: number | null): Date | null {
  if (typeof value !== "number") {
    return null
  }
  return new Date(value * 1000)
}

function formatDate(date: Date): string {
  return date.toISOString().slice(0, 10)
}

function dateToUnix(date: Date): number {
  return Math.floor(
    Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) / 1000
  )
}
