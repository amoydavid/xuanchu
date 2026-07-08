import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApiError } from "@/lib/api"

import {
  ALL_HOOK_EVENT_TYPES,
} from "../outbound-event-types"
import {
  createNotificationRule,
  deleteNotificationRule,
  disableNotificationRule,
  enableNotificationRule,
  listNotificationRules,
  listNotificationSinks,
  type NotificationRule,
  type NotificationRuleCreateInput,
} from "../outbound-api"
import { TemplateVarHints } from "../template-vars/template-var-hints"

const QUERY_KEY = ["outbound", "notification-rules"] as const

const AUDIENCE_TYPES = ["assignees", "explicit_users", "assignees_and_explicit_users"]

export function NotificationRuleList({ canWrite }: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<NotificationRule | null>(null)

  const query = useQuery<NotificationRule[]>({
    queryKey: QUERY_KEY,
    queryFn: listNotificationRules,
  })

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: QUERY_KEY })
  }

  const enable = useMutation({
    mutationFn: (id: string) => enableNotificationRule(id),
    onSuccess: invalidate,
  })
  const disable = useMutation({
    mutationFn: (id: string) => disableNotificationRule(id),
    onSuccess: invalidate,
  })
  const remove = useMutation({
    mutationFn: (id: string) => deleteNotificationRule(id),
    onSuccess: () => {
      setDeleting(null)
      invalidate()
    },
  })

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">{t("outbound.subtitle")}</p>
        {canWrite ? (
          <Button onClick={() => setCreating(true)} size="sm">
            {t("outbound.notificationRuleCreate")}
          </Button>
        ) : null}
      </div>

      <NotificationRuleCreateDialog
        onOpenChange={(o) => !o && setCreating(false)}
        onSaved={invalidate}
        open={creating}
      />

      <AlertDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("outbound.notificationRuleDeleteConfirm")}
            </AlertDialogTitle>
            <AlertDialogDescription>{deleting?.name ?? ""}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => deleting && remove.mutate(deleting.id)}
            >
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {query.isLoading ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("common.loading")}
        </div>
      ) : query.isError ? (
        <div className="border bg-card p-4 text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : t("common.error")}
        </div>
      ) : (query.data ?? []).length === 0 ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("common.empty")}
        </div>
      ) : (
        <div className="border bg-card">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("outbound.notificationRuleEvent")}</TableHead>
                <TableHead>{t("outbound.notificationRuleAudience")}</TableHead>
                <TableHead>{t("resource.enabled")}</TableHead>
                {canWrite ? <TableHead>{t("common.actions")}</TableHead> : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {(query.data ?? []).map((rule) => (
                <TableRow key={rule.id}>
                  <TableCell className="font-medium">{rule.name}</TableCell>
                  <TableCell className="text-xs">{rule.event_type}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {rule.audience_type ?? "-"}
                  </TableCell>
                  <TableCell>
                    {rule.enabled ? (
                      <Badge variant="outline">{t("hooks.enabled")}</Badge>
                    ) : (
                      <Badge variant="secondary">{t("hooks.disabled")}</Badge>
                    )}
                  </TableCell>
                  {canWrite ? (
                    <TableCell>
                      <div className="flex flex-wrap gap-2">
                        {rule.enabled ? (
                          <Button
                            onClick={() => disable.mutate(rule.id)}
                            size="sm"
                            variant="outline"
                          >
                            {t("hooks.disable")}
                          </Button>
                        ) : (
                          <Button
                            onClick={() => enable.mutate(rule.id)}
                            size="sm"
                            variant="outline"
                          >
                            {t("hooks.enable")}
                          </Button>
                        )}
                        <Button
                          onClick={() => setDeleting(rule)}
                          size="sm"
                          variant="outline"
                        >
                          {t("common.delete")}
                        </Button>
                      </div>
                    </TableCell>
                  ) : null}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}

function NotificationRuleCreateDialog({
  open,
  onOpenChange,
  onSaved,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState("")
  const [projectRef, setProjectRef] = useState("")
  const [eventType, setEventType] = useState("task.completed")
  const [filterSource, setFilterSource] = useState("")
  const [audienceType, setAudienceType] = useState("assignees")
  const [recipients, setRecipients] = useState("")
  const [sink, setSink] = useState("")
  const [templateSubject, setTemplateSubject] = useState("")
  const [templateBody, setTemplateBody] = useState("")
  const [sinks, setSinks] = useState<Awaited<ReturnType<typeof listNotificationSinks>>>([])
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // 弹窗打开时加载 sinks。
  useStateEffectOnOpen(open, () => {
    void listNotificationSinks({ includeDisabled: true })
      .then(setSinks)
      .catch(() => setSinks([]))
  })

  function reset() {
    setName("")
    setProjectRef("")
    setEventType("task.completed")
    setFilterSource("")
    setAudienceType("assignees")
    setRecipients("")
    setSink("")
    setTemplateSubject("")
    setTemplateBody("")
    setError(null)
  }

  function handleOpenChange(next: boolean) {
    if (!next) reset()
    onOpenChange(next)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!name.trim()) {
      setError(`${t("outbound.notificationRuleName")}: required`)
      return
    }
    if (!sink) {
      setError(`${t("outbound.notificationRuleSink")}: required`)
      return
    }
    const payload: NotificationRuleCreateInput = {
      name: name.trim(),
      event_type: eventType,
      audience_type: audienceType,
      sink,
    }
    if (projectRef.trim()) payload.project_ref = projectRef.trim()
    if (filterSource.trim()) payload.filter_source = filterSource.trim()
    const recipientList = recipients
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
    if (recipientList.length > 0) payload.recipients = recipientList
    if (templateSubject.trim()) payload.template_subject = templateSubject.trim()
    if (templateBody.trim()) payload.template_body = templateBody.trim()

    setSubmitting(true)
    try {
      await createNotificationRule(payload)
      onSaved()
      handleOpenChange(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("outbound.notificationRuleCreate")}</DialogTitle>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <LabeledInput
            label={t("outbound.notificationRuleName")}
            onChange={setName}
            value={name}
          />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <LabeledField label={t("outbound.notificationRuleEvent")}>
              <select
                aria-label={t("outbound.notificationRuleEvent")}
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) => setEventType(e.target.value)}
                value={eventType}
              >
                {ALL_HOOK_EVENT_TYPES.map((ev) => (
                  <option key={ev} value={ev}>
                    {ev}
                  </option>
                ))}
              </select>
            </LabeledField>
            <LabeledField label={t("outbound.notificationRuleAudience")}>
              <select
                aria-label={t("outbound.notificationRuleAudience")}
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) => setAudienceType(e.target.value)}
                value={audienceType}
              >
                {AUDIENCE_TYPES.map((a) => (
                  <option key={a} value={a}>
                    {a}
                  </option>
                ))}
              </select>
            </LabeledField>
          </div>
          <LabeledInput
            label={t("outbound.notificationRuleProject")}
            onChange={setProjectRef}
            value={projectRef}
          />
          <LabeledInput
            label={t("outbound.notificationRuleFilter")}
            onChange={setFilterSource}
            value={filterSource}
          />
          <LabeledInput
            label={t("outbound.notificationRuleRecipients")}
            onChange={setRecipients}
            value={recipients}
          />
          <LabeledField label={t("outbound.notificationRuleSink")}>
            <select
              aria-label={t("outbound.notificationRuleSink")}
              className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
              onChange={(e) => setSink(e.target.value)}
              value={sink}
            >
              <option value="">—</option>
              {sinks.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} ({s.type})
                </option>
              ))}
            </select>
          </LabeledField>
          {sink ? (
            <TemplateVarHints
              trigger="event"
              sink={sinks.find((s) => s.id === sink) ?? null}
            />
          ) : null}
          <LabeledInput
            label={t("outbound.notificationRuleSubject")}
            onChange={setTemplateSubject}
            value={templateSubject}
          />
          <LabeledField label={t("outbound.notificationRuleBody")}>
            <Textarea
              aria-label={t("outbound.notificationRuleBody")}
              className="min-h-[100px] font-mono text-xs"
              onChange={(e) => setTemplateBody(e.target.value)}
              value={templateBody}
            />
          </LabeledField>
          {error ? (
            <AlertDialogDescription>
              <span className="text-xs text-destructive">{error}</span>
            </AlertDialogDescription>
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
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function LabeledInput({
  label,
  onChange,
  value,
}: {
  label: string
  onChange: (v: string) => void
  value: string
}) {
  return (
    <LabeledField label={label}>
      <Input aria-label={label} onChange={(e) => onChange(e.target.value)} value={value} />
    </LabeledField>
  )
}

function LabeledField({
  children,
  label,
}: {
  children: React.ReactNode
  label: string
}) {
  return (
    <div className="space-y-1">
      <Label className="text-xs">{label}</Label>
      {children}
    </div>
  )
}

// useStateEffectOnOpen 在 open 由 false 变 true 时调用 effect。
function useStateEffectOnOpen(open: boolean, fn: () => void) {
  useEffect(() => {
    if (open) fn()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])
}
