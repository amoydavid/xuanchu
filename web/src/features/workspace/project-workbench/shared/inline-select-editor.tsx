import { useState } from "react"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useEditFeedback } from "./edit-feedback"

export type InlineSelectOption = {
  label: string
  value: string
}

type InlineSelectEditorProps = {
  ariaLabel: string
  options: InlineSelectOption[]
  value?: string | null
  onSave: (value: string) => Promise<void> | void
  disabled?: boolean
  placeholder?: string
  className?: string
}

export function InlineSelectEditor({
  ariaLabel,
  className,
  disabled = false,
  onSave,
  options,
  placeholder,
  value,
}: InlineSelectEditorProps) {
  const feedback = useEditFeedback()
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const save = async (next: string) => {
    setSaving(true)
    setError(null)
    try {
      await onSave(next)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure(ariaLabel, message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <span className="inline-flex flex-col gap-1">
      <Select
        disabled={disabled || saving}
        value={value ?? ""}
        onValueChange={(next) => {
          void save(next)
        }}
      >
        <SelectTrigger aria-label={ariaLabel} className={className}>
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {error ? <span className="text-xs text-destructive">{error}</span> : null}
    </span>
  )
}
