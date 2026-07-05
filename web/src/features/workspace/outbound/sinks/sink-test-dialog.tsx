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
import { Label } from "@/components/ui/label"
import { ApiError } from "@/lib/api"

import {
  ALL_HOOK_EVENT_TYPES,
  TASK_BASIC_EVENTS,
} from "../outbound-event-types"
import {
  testNotificationSink,
  type NotificationSink,
  type NotificationSinkTestInput,
  type NotificationSinkTestView,
} from "../outbound-api"

type Kind = "hook" | "notification"

type SinkTestDialogProps = {
  sink: NotificationSink | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SinkTestDialog({ sink, onOpenChange, open }: SinkTestDialogProps) {
  const { t } = useTranslation()
  const [kind, setKind] = useState<Kind>("hook")
  const [eventType, setEventType] = useState<string>("task.completed")
  const [projectRef, setProjectRef] = useState<string>("")
  const [result, setResult] = useState<NotificationSinkTestView | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  function reset() {
    setKind("hook")
    setEventType("task.completed")
    setProjectRef("")
    setResult(null)
    setError(null)
  }

  function handleOpenChange(next: boolean) {
    if (!next) reset()
    onOpenChange(next)
  }

  const showProjectRef = sink?.endpoint_mode === "config_value"

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!sink) return
    setError(null)
    setResult(null)
    const input: NotificationSinkTestInput = {
      kind,
      event_type: eventType,
    }
    if (showProjectRef && projectRef.trim()) {
      input.project_ref = projectRef.trim()
    }
    setSubmitting(true)
    try {
      const view = await testNotificationSink(sink.id, input)
      setResult(view)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("outbound.sinkTestTitle")}</DialogTitle>
          <DialogDescription>{sink?.name ?? ""}</DialogDescription>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.sinkTestKind")}</Label>
              <select
                aria-label={t("outbound.sinkTestKind")}
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) => setKind(e.target.value as Kind)}
                value={kind}
              >
                <option value="hook">hook</option>
                <option value="notification">notification</option>
              </select>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.sinkTestEvent")}</Label>
              <select
                aria-label={t("outbound.sinkTestEvent")}
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) => setEventType(e.target.value)}
                value={eventType}
              >
                {kind === "hook"
                  ? ALL_HOOK_EVENT_TYPES.map((ev) => (
                      <option key={ev} value={ev}>
                        {ev}
                      </option>
                    ))
                  : TASK_BASIC_EVENTS.map((ev) => (
                      <option key={ev} value={ev}>
                        {ev}
                      </option>
                    ))}
              </select>
            </div>
          </div>
          {showProjectRef ? (
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.sinkTestProject")}</Label>
              <input
                aria-label={t("outbound.sinkTestProject")}
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) => setProjectRef(e.target.value)}
                placeholder="project-slug"
                value={projectRef}
              />
            </div>
          ) : null}

          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          {result ? (
            <Alert variant={result.status === "succeeded" ? "default" : "destructive"}>
              <AlertDescription>
                <div className="space-y-1 text-xs">
                  <div>
                    <span className="font-medium">{t("outbound.sinkTestResult")}:</span>{" "}
                    {result.status}
                    {result.status_code ? ` (HTTP ${result.status_code})` : null}
                  </div>
                  <div>
                    duration: {result.duration_ms}ms · {result.resolved_endpoint_source} ·{" "}
                    <code className="text-[10px]">{result.resolved_endpoint_fingerprint}</code>
                  </div>
                  {result.error ? <div>error: {result.error}</div> : null}
                  {result.rendered_method ? (
                    <div>method: {result.rendered_method}</div>
                  ) : null}
                </div>
              </AlertDescription>
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
            <Button disabled={submitting || !sink} type="submit">
              {t("outbound.sinkTestRun")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
