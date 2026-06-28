import { PencilIcon } from "lucide-react"
import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { useEditFeedback } from "./edit-feedback"

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
  const feedback = useEditFeedback()
  const normalizedValue = value ?? ""
  const [editing, setEditing] = useState(false)
  const [draftState, setDraftState] = useState(() => ({
    source: normalizedValue,
    value: normalizedValue,
  }))
  const draft =
    editing || draftState.source === normalizedValue
      ? draftState.value
      : normalizedValue
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const ignoreNextBlurRef = useRef(false)
  const lastSubmittedRef = useRef<string | null>(null)
  const savingRef = useRef(false)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  const setDraft = (next: string) =>
    setDraftState({ source: normalizedValue, value: next })

  useEffect(() => {
    if (editing) {
      const field = multiline ? textareaRef.current : inputRef.current
      field?.focus()
      field?.select()
    }
  }, [editing, multiline])

  const cancel = () => {
    ignoreNextBlurRef.current = true
    setDraftState({ source: normalizedValue, value: normalizedValue })
    setError(null)
    setEditing(false)
  }

  const save = async () => {
    if (savingRef.current) {
      return
    }
    const nextValue = draft.trim()
    const validationError = validate?.(nextValue)
    if (validationError) {
      setError(validationError)
      return
    }
    if (nextValue === normalizedValue.trim()) {
      setDraft(normalizedValue)
      setError(null)
      setEditing(false)
      return
    }
    if (nextValue === lastSubmittedRef.current) {
      return
    }
    lastSubmittedRef.current = nextValue
    savingRef.current = true
    setSaving(true)
    setError(null)
    try {
      await onSave(nextValue)
      setEditing(false)
    } catch (err) {
      lastSubmittedRef.current = null
      const message = err instanceof Error ? err.message : String(err)
      setError(message)
      feedback.failure(ariaLabel, message)
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }

  if (!editing) {
    return (
      <Button
        aria-label={ariaLabel}
        className={cn(
          "group h-auto min-h-6 max-w-full justify-start rounded-sm px-1 py-0 text-left font-normal hover:bg-muted disabled:opacity-60",
          displayClassName
        )}
        disabled={disabled}
        onClick={() => {
          if (!disabled) {
            setDraftState({ source: normalizedValue, value: normalizedValue })
            setError(null)
            setEditing(true)
          }
        }}
        size="xs"
        type="button"
        variant="ghost"
      >
        <span className="truncate">{normalizedValue || emptyLabel}</span>
        {!disabled ? (
          <PencilIcon className="size-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-60 group-focus-visible:opacity-60" />
        ) : null}
      </Button>
    )
  }

  const commonProps = {
    "aria-invalid": error ? true : undefined,
    "aria-label": ariaLabel,
    className,
    disabled: saving,
    onBlur: () => {
      if (ignoreNextBlurRef.current) {
        ignoreNextBlurRef.current = false
        return
      }
      if (!saving) {
        void save()
      }
    },
    onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      setDraft(event.target.value)
      lastSubmittedRef.current = null
      setError(null)
    },
    onKeyDown: (
      event: KeyboardEvent<HTMLInputElement | HTMLTextAreaElement>
    ) => {
      if (event.key === "Escape") {
        event.preventDefault()
        cancel()
        return
      }
      if (
        event.key === "Enter" &&
        (!multiline || event.metaKey || event.ctrlKey)
      ) {
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
      {error ? (
        <span className="mt-1 block text-xs text-destructive">{error}</span>
      ) : null}
    </span>
  )
}
