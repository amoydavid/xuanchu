import { useEffect, useRef, useState, type ChangeEvent, type KeyboardEvent } from "react"

import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

type InlineTextEditorProps = {
  ariaLabel: string
  value: string | null | undefined
  onSave: (value: string) => Promise<void> | void
  className?: string
  disabled?: boolean
  displayClassName?: string
  emptyLabel?: string
  multiline?: boolean
  placeholder?: string
  validate?: (value: string) => string | null | undefined
}

export function InlineTextEditor({
  ariaLabel,
  className,
  disabled = false,
  displayClassName,
  emptyLabel = "-",
  multiline = false,
  onSave,
  placeholder,
  validate,
  value,
}: InlineTextEditorProps) {
  const normalizedValue = value ?? ""
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(normalizedValue)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    if (!editing) {
      setDraft(normalizedValue)
    }
  }, [editing, normalizedValue])

  useEffect(() => {
    if (editing) {
      const field = multiline ? textareaRef.current : inputRef.current
      field?.focus()
      field?.select()
    }
  }, [editing, multiline])

  const cancel = () => {
    setDraft(normalizedValue)
    setError(null)
    setEditing(false)
  }

  const save = async () => {
    const validationError = validate?.(draft)
    if (validationError) {
      setError(validationError)
      return
    }
    setSaving(true)
    setError(null)
    try {
      await onSave(draft)
      setEditing(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  if (!editing) {
    return (
      <button
        aria-label={ariaLabel}
        className={cn(
          "block min-h-6 max-w-full truncate rounded-sm text-left outline-none transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-60",
          displayClassName
        )}
        disabled={disabled}
        onClick={() => {
          if (!disabled) {
            setError(null)
            setEditing(true)
          }
        }}
        type="button"
      >
        {normalizedValue || emptyLabel}
      </button>
    )
  }

  const commonProps = {
    "aria-invalid": error ? true : undefined,
    "aria-label": ariaLabel,
    className,
    disabled: saving,
    onBlur: () => {
      if (!saving) {
        void save()
      }
    },
    onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      setDraft(event.target.value)
      setError(null)
    },
    onKeyDown: (event: KeyboardEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      if (event.key === "Escape") {
        event.preventDefault()
        cancel()
        return
      }
      if (event.key === "Enter" && (!multiline || event.metaKey || event.ctrlKey)) {
        event.preventDefault()
        void save()
      }
    },
    placeholder,
    value: draft,
  }

  return (
    <span className="block">
      {multiline ? (
        <Textarea {...commonProps} ref={textareaRef} />
      ) : (
        <Input {...commonProps} ref={inputRef} />
      )}
      {error ? <span className="mt-1 block text-xs text-destructive">{error}</span> : null}
    </span>
  )
}
