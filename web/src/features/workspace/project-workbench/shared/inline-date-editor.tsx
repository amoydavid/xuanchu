import { useState } from "react"

import { Input } from "@/components/ui/input"

type InlineDateEditorProps = {
  ariaLabel: string
  value?: number | null
  onSave: (value: number | null) => Promise<void> | void
  className?: string
  disabled?: boolean
}

export function InlineDateEditor({
  ariaLabel,
  className,
  disabled = false,
  onSave,
  value,
}: InlineDateEditorProps) {
  const [draft, setDraft] = useState(formatUnixDate(value))
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const save = async () => {
    setSaving(true)
    setError(null)
    try {
      await onSave(draft ? dateToUnix(draft) : null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <span className="inline-flex flex-col gap-1">
      <Input
        aria-label={ariaLabel}
        className={className}
        disabled={disabled || saving}
        type="date"
        value={draft}
        onBlur={() => {
          void save()
        }}
        onChange={(event) => {
          setDraft(event.target.value)
          setError(null)
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault()
            void save()
          }
          if (event.key === "Escape") {
            event.preventDefault()
            setDraft(formatUnixDate(value))
            setError(null)
          }
        }}
      />
      {error ? <span className="text-xs text-destructive">{error}</span> : null}
    </span>
  )
}

function formatUnixDate(value?: number | null): string {
  if (typeof value !== "number") {
    return ""
  }
  return new Date(value * 1000).toISOString().slice(0, 10)
}

function dateToUnix(value: string): number {
  return Math.floor(new Date(`${value}T00:00:00Z`).getTime() / 1000)
}
