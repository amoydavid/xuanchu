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
  createReminderRule,
  deleteReminderRule,
  disableReminderRule,
  enableReminderRule,
  listNotificationSinks,
  listReminderRules,
  type ReminderRule,
  type ReminderRuleCreateInput,
} from "../outbound-api"
import { TemplateVarHints } from "../template-vars/template-var-hints"

const QUERY_KEY = ["outbound", "reminder-rules"] as const

const AUDIENCE_TYPES = ["assignees", "explicit_users", "assignees_and_explicit_users"]
const SCHEDULE_TYPES = ["daily_at", "hourly", "weekly_at", "cron"]

export function ReminderRuleList({ canWrite }: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<ReminderRule | null>(null)

  const query = useQuery<ReminderRule[]>({
    queryKey: QUERY_KEY,
    queryFn: listReminderRules,
  })

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: QUERY_KEY })
  }

  const enable = useMutation({
    mutationFn: (id: string) => enableReminderRule(id),
    onSuccess: invalidate,
  })
  const disable = useMutation({
    mutationFn: (id: string) => disableReminderRule(id),
    onSuccess: invalidate,
  })
  const remove = useMutation({
    mutationFn: (id: string) => deleteReminderRule(id),
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
            {t("outbound.reminderRuleCreate")}
          </Button>
        ) : null}
      </div>

      <ReminderRuleCreateDialog
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
              {t("outbound.reminderRuleDeleteConfirm")}
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
                <TableHead>{t("outbound.reminderRuleScheduleType")}</TableHead>
                <TableHead>{t("outbound.reminderRuleAudience")}</TableHead>
                <TableHead>{t("resource.enabled")}</TableHead>
                {canWrite ? <TableHead>{t("common.actions")}</TableHead> : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {(query.data ?? []).map((rule) => (
                <TableRow key={rule.id}>
                  <TableCell className="font-medium">{rule.name}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {rule.schedule_type ?? "-"}
                    {rule.schedule_value ? `: ${rule.schedule_value}` : ""}
                  </TableCell>
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

function ReminderRuleCreateDialog({
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
  const [scheduleType, setScheduleType] = useState("daily_at")
  const [scheduleValue, setScheduleValue] = useState("08:50")
  const [filterSource, setFilterSource] = useState("status:pending")
  const [audienceType, setAudienceType] = useState("assignees")
  const [recipients, setRecipients] = useState("")
  const [sink, setSink] = useState("")
  const [sinks, setSinks] = useState<Awaited<ReturnType<typeof listNotificationSinks>>>([])
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!open) return
    void listNotificationSinks({ includeDisabled: true })
      .then(setSinks)
      .catch(() => setSinks([]))
  }, [open])

  function reset() {
    setName("")
    setProjectRef("")
    setScheduleType("daily_at")
    setScheduleValue("08:50")
    setFilterSource("status:pending")
    setAudienceType("assignees")
    setRecipients("")
    setSink("")
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
      setError(`${t("outbound.reminderRuleName")}: required`)
      return
    }
    if (!sink) {
      setError(`${t("outbound.reminderRuleSink")}: required`)
      return
    }
    const payload: ReminderRuleCreateInput = {
      name: name.trim(),
      schedule_type: scheduleType,
      schedule_value: scheduleValue,
      filter_source: filterSource,
      audience_type: audienceType,
      sink_ref: sink,
    }
    if (projectRef.trim()) payload.project_ref = projectRef.trim()
    const recipientList = recipients
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
    if (recipientList.length > 0) payload.recipients = recipientList

    setSubmitting(true)
    try {
      await createReminderRule(payload)
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
          <DialogTitle>{t("outbound.reminderRuleCreate")}</DialogTitle>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <div className="space-y-1">
            <Label className="text-xs">{t("outbound.reminderRuleName")}</Label>
            <Input
              aria-label={t("outbound.reminderRuleName")}
              onChange={(e) => setName(e.target.value)}
              value={name}
            />
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.reminderRuleScheduleType")}</Label>
              <select
                aria-label={t("outbound.reminderRuleScheduleType")}
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) => setScheduleType(e.target.value)}
                value={scheduleType}
              >
                {SCHEDULE_TYPES.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.reminderRuleScheduleValue")}</Label>
              <Input
                aria-label={t("outbound.reminderRuleScheduleValue")}
                onChange={(e) => setScheduleValue(e.target.value)}
                value={scheduleValue}
              />
            </div>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.reminderRuleFilter")}</Label>
              <Input
                aria-label={t("outbound.reminderRuleFilter")}
                onChange={(e) => setFilterSource(e.target.value)}
                value={filterSource}
              />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t("outbound.reminderRuleAudience")}</Label>
              <select
                aria-label={t("outbound.reminderRuleAudience")}
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
            </div>
          </div>
          <div className="space-y-1">
            <Label className="text-xs">{t("outbound.reminderRuleProject")}</Label>
            <Input
              aria-label={t("outbound.reminderRuleProject")}
              onChange={(e) => setProjectRef(e.target.value)}
              value={projectRef}
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">{t("outbound.reminderRuleRecipients")}</Label>
            <Input
              aria-label={t("outbound.reminderRuleRecipients")}
              onChange={(e) => setRecipients(e.target.value)}
              value={recipients}
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">{t("outbound.reminderRuleSink")}</Label>
            <select
              aria-label={t("outbound.reminderRuleSink")}
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
          </div>
          {sink ? (
            <TemplateVarHints
              trigger="reminder"
              sink={sinks.find((s) => s.id === sink) ?? null}
            />
          ) : null}
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
