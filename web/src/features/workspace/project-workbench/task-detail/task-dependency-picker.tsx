import { useState, type KeyboardEvent } from "react"

import { Input } from "@/components/ui/input"

type TaskDependencyPickerProps = {
  disabled?: boolean
  onSave: (depends: string[]) => Promise<void> | void
  value?: string[]
}

export function TaskDependencyPicker({
  disabled = false,
  onSave,
  value = [],
}: TaskDependencyPickerProps) {
  const [draft, setDraft] = useState(value.join(", "))
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const save = async () => {
    setSaving(true)
    setError(null)
    try {
      await onSave(splitList(draft))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter") {
      event.preventDefault()
      void save()
    }
  }

  return (
    <div className="space-y-1">
      <Input
        aria-label="依赖任务"
        disabled={disabled || saving}
        onBlur={() => {
          if (!disabled && !saving && draft !== value.join(", ")) {
            void save()
          }
        }}
        onChange={(event) => {
          setDraft(event.target.value)
          setError(null)
        }}
        onKeyDown={onKeyDown}
        placeholder="task-1, task-2"
        value={draft}
      />
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}

export function splitList(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
}
