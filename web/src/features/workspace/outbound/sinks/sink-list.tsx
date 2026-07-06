import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApiError } from "@/lib/api"

import {
  deleteNotificationSink,
  disableNotificationSink,
  enableNotificationSink,
  listNotificationSinks,
  type NotificationSink,
} from "../outbound-api"
import { SinkFormDialog } from "./sink-form-dialog"
import { SinkTestDialog } from "./sink-test-dialog"

const SINKS_QUERY_KEY = ["outbound", "sinks"] as const

export function SinkList({ canWrite }: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<NotificationSink | null>(null)
  const [creating, setCreating] = useState(false)
  const [testing, setTesting] = useState<NotificationSink | null>(null)
  const [deleting, setDeleting] = useState<NotificationSink | null>(null)

  const query = useQuery<NotificationSink[]>({
    queryKey: SINKS_QUERY_KEY,
    queryFn: () => listNotificationSinks({ includeDisabled: true }),
  })

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: SINKS_QUERY_KEY })
  }

  const remove = useMutation({
    mutationFn: (id: string) => deleteNotificationSink(id),
    onSuccess: () => {
      setDeleting(null)
      invalidate()
    },
  })

  const enable = useMutation({
    mutationFn: (id: string) => enableNotificationSink(id),
    onSuccess: invalidate,
  })
  const disable = useMutation({
    mutationFn: (id: string) => disableNotificationSink(id),
    onSuccess: invalidate,
  })

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">{t("outbound.sinkHint")}</p>
        {canWrite ? (
          <Button onClick={() => setCreating(true)} size="sm">
            {t("outbound.createSink")}
          </Button>
        ) : null}
      </div>

      <SinkFormDialog
        onOpenChange={(o) => !o && setCreating(false)}
        onSaved={invalidate}
        open={creating}
      />
      <SinkFormDialog
        initial={editing ?? undefined}
        onOpenChange={(o) => !o && setEditing(null)}
        onSaved={invalidate}
        open={!!editing}
      />
      <SinkTestDialog
        sink={testing}
        onOpenChange={(o) => !o && setTesting(null)}
        open={!!testing}
      />

      <AlertDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("outbound.deleteSink")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("outbound.sinkDeleteConfirm", {
                defaultValue: `确认删除 Sink「${deleting?.name ?? ""}」？`,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {remove.isError ? (
            <div className="text-xs text-destructive">
              {remove.error instanceof ApiError
                ? remove.error.message
                : t("common.error")}
            </div>
          ) : null}
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

      {query.isError ? (
        <div className="border bg-card p-4 text-sm text-destructive">
          {query.error instanceof ApiError
            ? query.error.message
            : t("common.error")}
        </div>
      ) : query.isLoading ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("common.loading")}
        </div>
      ) : (query.data ?? []).length === 0 ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("outbound.sinkEmpty")}
        </div>
      ) : (
        <div className="border bg-card">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("resource.type")}</TableHead>
                <TableHead>endpoint</TableHead>
                <TableHead>{t("resource.enabled")}</TableHead>
                <TableHead>{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(query.data ?? []).map((sink) => (
                <TableRow key={sink.id}>
                  <TableCell className="font-medium">{sink.name}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {sink.type}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {endpointSummary(sink)}
                  </TableCell>
                  <TableCell>
                    {sink.enabled ? (
                      <Badge variant="outline">{t("hooks.enabled")}</Badge>
                    ) : (
                      <Badge variant="secondary">{t("hooks.disabled")}</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-2">
                      {canWrite ? (
                        <>
                          <Button
                            onClick={() => setEditing(sink)}
                            size="sm"
                            variant="outline"
                          >
                            {t("outbound.editSink")}
                          </Button>
                          {sink.enabled ? (
                            <Button
                              onClick={() => disable.mutate(sink.id)}
                              size="sm"
                              variant="outline"
                            >
                              {t("hooks.disable")}
                            </Button>
                          ) : (
                            <Button
                              onClick={() => enable.mutate(sink.id)}
                              size="sm"
                              variant="outline"
                            >
                              {t("hooks.enable")}
                            </Button>
                          )}
                          <Button
                            onClick={() => setTesting(sink)}
                            size="sm"
                            variant="outline"
                          >
                            {t("outbound.sinkTest")}
                          </Button>
                          <Button
                            onClick={() => setDeleting(sink)}
                            size="sm"
                            variant="outline"
                          >
                            {t("common.delete")}
                          </Button>
                        </>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}

function endpointSummary(sink: NotificationSink): string {
  switch (sink.endpoint_mode) {
    case "static_url":
      return sink.url ?? "-"
    case "template":
      return sink.url_template ?? "-"
    case "config_value":
      return sink.config_key ? `config:${sink.config_key}` : "-"
    default:
      return sink.endpoint_mode ?? "-"
  }
}
