import type { FormEvent } from "react"
import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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

import { HOOK_EVENT_GROUPS } from "../outbound-event-types"
import {
  createHook,
  listNotificationSinks,
  listProjects,
  modifyHook,
  type Hook,
  type HookCreateInput,
  type HookModifyInput,
  type NotificationSink,
  type ProjectSummary,
} from "../outbound-api"

type HookScope = "workspace" | "project"

type FormState = {
  name: string
  scope_type: HookScope
  project_ref: string
  sink: string
  event_types: string[]
  timeout_seconds: number
  max_attempts: number
}

function emptyForm(): FormState {
  return {
    name: "",
    scope_type: "workspace",
    project_ref: "",
    sink: "",
    event_types: [],
    timeout_seconds: 10,
    max_attempts: 3,
  }
}

function fromHook(hook: Hook): FormState {
  return {
    name: hook.name ?? "",
    scope_type: (hook.scope_type as HookScope) ?? "workspace",
    project_ref: hook.project_id ?? "",
    sink: hook.sink_id ?? "",
    event_types: hook.event_types ?? [],
    timeout_seconds: hook.timeout_seconds ?? 10,
    max_attempts: hook.max_attempts ?? 3,
  }
}

type HookFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: () => void
  initial?: Hook
  workspaceSlug?: string
}

export function HookFormDialog({
  initial,
  onOpenChange,
  onSaved,
  open,
  workspaceSlug,
}: HookFormDialogProps) {
  const { t } = useTranslation()
  const [form, setForm] = useState<FormState>(() =>
    initial ? fromHook(initial) : emptyForm()
  )
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const [sinks, setSinks] = useState<NotificationSink[]>([])
  const [projects, setProjects] = useState<ProjectSummary[]>([])

  useEffect(() => {
    if (!open) return
    setForm(initial ? fromHook(initial) : emptyForm())
    setError(null)
    // 异步加载 sink 列表（包括 disabled 以便给出风险提示）和 project 列表。
    void listNotificationSinks({ includeDisabled: true })
      .then(setSinks)
      .catch(() => setSinks([]))
    if (workspaceSlug) {
      void listProjects(workspaceSlug)
        .then(setProjects)
        .catch(() => setProjects([]))
    }
  }, [open, initial, workspaceSlug])

  function patch(partial: Partial<FormState>) {
    setForm((prev) => ({ ...prev, ...partial }))
  }

  function toggleEvent(eventType: string) {
    setForm((prev) => {
      const exists = prev.event_types.includes(eventType)
      return {
        ...prev,
        event_types: exists
          ? prev.event_types.filter((e) => e !== eventType)
          : [...prev.event_types, eventType],
      }
    })
  }

  function selectGroup(events: readonly string[], on: boolean) {
    setForm((prev) => {
      const set = new Set(prev.event_types)
      for (const e of events) {
        if (on) set.add(e)
        else set.delete(e)
      }
      return { ...prev, event_types: Array.from(set) }
    })
  }

  const selectedSink = useMemo(
    () => sinks.find((s) => s.id === form.sink),
    [sinks, form.sink]
  )
  const sinkDisabled = selectedSink && !selectedSink.enabled

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!form.name.trim()) {
      setError(`${t("outbound.hookName")}: required`)
      return
    }
    if (!form.sink) {
      setError(`${t("outbound.hookSink")}: required`)
      return
    }
    if (form.event_types.length === 0) {
      setError(`${t("outbound.hookEvents")}: required`)
      return
    }
    if (form.scope_type === "project" && !form.project_ref.trim()) {
      setError(`${t("outbound.hookProject")}: required`)
      return
    }

    setSubmitting(true)
    try {
      if (initial) {
        await modifyHook(initial.id, buildModifyPayload(form))
      } else {
        await createHook(buildCreatePayload(form))
      }
      onSaved()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {initial ? t("outbound.hookEdit") : t("outbound.hookCreate")}
          </DialogTitle>
          <DialogDescription>{t("outbound.subtitle")}</DialogDescription>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <div className="space-y-1">
            <Label className="text-xs">{t("outbound.hookName")}</Label>
            <Input
              aria-label={t("outbound.hookName")}
              onChange={(e) => patch({ name: e.target.value })}
              value={form.name}
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.hookScope")}</Label>
              <select
                aria-label={t("outbound.hookScope")}
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) => patch({ scope_type: e.target.value as HookScope })}
                value={form.scope_type}
              >
                <option value="workspace">workspace</option>
                <option value="project">project</option>
              </select>
            </div>
            {form.scope_type === "project" ? (
              <div className="space-y-1">
                <Label className="text-xs">{t("outbound.hookProject")}</Label>
                <select
                  aria-label={t("outbound.hookProject")}
                  className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                  onChange={(e) => patch({ project_ref: e.target.value })}
                  value={form.project_ref}
                >
                  <option value="">—</option>
                  {projects.map((p) => (
                    <option key={p.id} value={p.slug}>
                      {p.slug}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
          </div>

          <div className="space-y-1">
            <Label className="text-xs">{t("outbound.hookSink")}</Label>
            <select
              aria-label={t("outbound.hookSink")}
              className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
              onChange={(e) => patch({ sink: e.target.value })}
              value={form.sink}
            >
              <option value="">—</option>
              {sinks.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} ({s.type}, {s.enabled ? "enabled" : "disabled"})
                </option>
              ))}
            </select>
          </div>
          {sinkDisabled ? (
            <Alert>
              <AlertDescription>
                {t("outbound.hookDisabledSinkWarning")}
              </AlertDescription>
            </Alert>
          ) : null}

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs">{t("outbound.hookEvents")}</Label>
              <div className="flex gap-2 text-xs">
                <Button
                  onClick={() =>
                    selectGroup(
                      [
                        ...HOOK_EVENT_GROUPS[0].events,
                        ...HOOK_EVENT_GROUPS[1].events,
                        ...HOOK_EVENT_GROUPS[2].events,
                      ],
                      true
                    )
                  }
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  {t("outbound.hookEventsSelectAllTask")}
                </Button>
                <Button
                  onClick={() => selectGroup(HOOK_EVENT_GROUPS[3].events, true)}
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  {t("outbound.hookEventsSelectAllProject")}
                </Button>
              </div>
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {HOOK_EVENT_GROUPS.map((group) => (
                <div className="space-y-1 border p-2" key={group.id}>
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-medium">
                      {t(`outbound.hookEventGroup.${group.id}`)}
                    </span>
                    <button
                      className="text-xs text-muted-foreground underline"
                      onClick={() => selectGroup(group.events, true)}
                      type="button"
                    >
                      +
                    </button>
                  </div>
                  {group.events.map((event) => (
                    <label
                      className="flex items-center gap-2 text-xs"
                      key={event}
                    >
                      <Checkbox
                        checked={form.event_types.includes(event)}
                        onCheckedChange={() => toggleEvent(event)}
                      />
                      <span>{event}</span>
                    </label>
                  ))}
                </div>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.hookTimeout")}</Label>
              <Input
                aria-label={t("outbound.hookTimeout")}
                min={1}
                onChange={(e) =>
                  patch({ timeout_seconds: Number(e.target.value) || 0 })
                }
                type="number"
                value={form.timeout_seconds}
              />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.hookMaxAttempts")}</Label>
              <Input
                aria-label={t("outbound.hookMaxAttempts")}
                min={1}
                onChange={(e) =>
                  patch({ max_attempts: Number(e.target.value) || 0 })
                }
                type="number"
                value={form.max_attempts}
              />
            </div>
          </div>

          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button
              onClick={() => onOpenChange(false)}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button disabled={submitting} type="submit">
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function buildCreatePayload(form: FormState): HookCreateInput {
  const payload: HookCreateInput = {
    name: form.name.trim(),
    scope_type: form.scope_type,
    event_types: form.event_types,
    sink: form.sink,
    timeout_seconds: form.timeout_seconds,
    max_attempts: form.max_attempts,
  }
  if (form.scope_type === "project" && form.project_ref.trim()) {
    payload.project_ref = form.project_ref.trim()
  }
  return payload
}

function buildModifyPayload(form: FormState): HookModifyInput {
  return {
    name: form.name.trim(),
    event_types: [...form.event_types],
    sink: form.sink,
    timeout_seconds: form.timeout_seconds,
    max_attempts: form.max_attempts,
  }
}
