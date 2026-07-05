import type { FormEvent } from "react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ApiError } from "@/lib/api"

import { createHook } from "./hooks-api"

const SCOPE_OPTIONS = ["workspace", "project"]

type HookCreateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

export function HookCreateDialog({
  onCreated,
  onOpenChange,
  open,
}: HookCreateDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState("")
  const [scopeType, setScopeType] = useState("workspace")
  const [sink, setSink] = useState("")
  const [eventTypes, setEventTypes] = useState("")
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  function resetForm() {
    setName("")
    setScopeType("workspace")
    setSink("")
    setEventTypes("")
    setSubmitError(null)
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) resetForm()
    onOpenChange(nextOpen)
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setSubmitError(null)

    const events = eventTypes
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)

    if (!name.trim()) {
      setSubmitError(t("hooks.createDialog.name") + ": required")
      return
    }
    if (!sink.trim()) {
      setSubmitError(t("hooks.createDialog.sink") + ": required")
      return
    }
    if (events.length === 0) {
      setSubmitError(t("hooks.createDialog.eventTypes") + ": required")
      return
    }

    setSubmitting(true)
    try {
      await createHook({
        name: name.trim(),
        scope_type: scopeType,
        sink: sink.trim(),
        event_types: events,
      })
      onCreated()
      handleOpenChange(false)
    } catch (err) {
      setSubmitError(
        err instanceof ApiError ? err.message : t("common.error")
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("hooks.createDialog.title")}</DialogTitle>
          <DialogDescription>{t("hooks.subtitle")}</DialogDescription>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <div className="space-y-1">
            <Label htmlFor="hook-name">{t("hooks.createDialog.name")}</Label>
            <Input
              id="hook-name"
              onChange={(e) => setName(e.target.value)}
              placeholder={t("hooks.createDialog.namePlaceholder")}
              value={name}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="hook-scope">{t("hooks.createDialog.scopeType")}</Label>
            <select
              className="h-9 w-full rounded-md border bg-transparent px-3 text-sm"
              id="hook-scope"
              onChange={(e) => setScopeType(e.target.value)}
              value={scopeType}
            >
              {SCOPE_OPTIONS.map((opt) => (
                <option key={opt} value={opt}>
                  {opt}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1">
            <Label htmlFor="hook-sink">{t("hooks.createDialog.sink")}</Label>
            <Input
              id="hook-sink"
              onChange={(e) => setSink(e.target.value)}
              value={sink}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="hook-events">
              {t("hooks.createDialog.eventTypes")}
            </Label>
            <Input
              id="hook-events"
              onChange={(e) => setEventTypes(e.target.value)}
              placeholder={t("hooks.createDialog.eventTypesPlaceholder")}
              value={eventTypes}
            />
          </div>
          {submitError ? (
            <Alert variant="destructive">
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              onClick={() => handleOpenChange(false)}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button disabled={submitting} type="submit">
              {t("hooks.createDialog.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
