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
  deleteHook,
  disableHook,
  enableHook,
  listHooks,
  type Hook,
} from "../outbound-api"
import { HookDeliveryTable } from "../deliveries/hook-delivery-table"
import { HookFormDialog } from "./hook-form-dialog"

const HOOKS_QUERY_KEY = ["outbound", "hooks"] as const

export function HookList({
  canWrite,
  workspaceSlug,
}: {
  canWrite: boolean
  workspaceSlug?: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Hook | null>(null)
  const [deleting, setDeleting] = useState<Hook | null>(null)

  const query = useQuery<Hook[]>({
    queryKey: HOOKS_QUERY_KEY,
    queryFn: listHooks,
  })

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: HOOKS_QUERY_KEY })
  }

  const remove = useMutation({
    mutationFn: (id: string) => deleteHook(id),
    onSuccess: () => {
      setDeleting(null)
      invalidate()
    },
  })
  const enable = useMutation({
    mutationFn: (id: string) => enableHook(id),
    onSuccess: invalidate,
  })
  const disable = useMutation({
    mutationFn: (id: string) => disableHook(id),
    onSuccess: invalidate,
  })

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {t("outbound.subtitle")}
        </p>
        {canWrite ? (
          <Button onClick={() => setCreating(true)} size="sm">
            {t("outbound.hookCreate")}
          </Button>
        ) : null}
      </div>

      <HookFormDialog
        key={creating ? "creating-open" : "creating-closed"}
        onOpenChange={(o) => !o && setCreating(false)}
        onSaved={invalidate}
        open={creating}
        workspaceSlug={workspaceSlug}
      />
      <HookFormDialog
        key={editing?.id ?? "editing-closed"}
        initial={editing ?? undefined}
        onOpenChange={(o) => !o && setEditing(null)}
        onSaved={invalidate}
        open={!!editing}
        workspaceSlug={workspaceSlug}
      />

      <AlertDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("outbound.hookDeleteConfirm")}</AlertDialogTitle>
            <AlertDialogDescription>{deleting?.name ?? ""}</AlertDialogDescription>
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
          {t("common.empty")}
        </div>
      ) : (
        <div className="border bg-card">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("outbound.hookScope")}</TableHead>
                <TableHead>{t("outbound.hookEvents")}</TableHead>
                <TableHead>{t("outbound.hookSink")}</TableHead>
                <TableHead>{t("resource.enabled")}</TableHead>
                <TableHead>{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(query.data ?? []).map((hook) => (
                <HookRow
                  canWrite={canWrite}
                  hook={hook}
                  key={hook.id}
                  onDelete={setDeleting}
                  onDisable={(id) => disable.mutate(id)}
                  onEdit={setEditing}
                  onEnable={(id) => enable.mutate(id)}
                />
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}

function HookRow({
  canWrite,
  hook,
  onEdit,
  onEnable,
  onDisable,
  onDelete,
}: {
  canWrite: boolean
  hook: Hook
  onEdit: (hook: Hook) => void
  onEnable: (id: string) => void
  onDisable: (id: string) => void
  onDelete: (hook: Hook) => void
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  return (
    <>
      <TableRow
        className="cursor-pointer"
        onClick={() => setExpanded((v) => !v)}
      >
        <TableCell className="font-medium">{hook.name}</TableCell>
        <TableCell className="text-xs text-muted-foreground">
          {hook.scope_type}
        </TableCell>
        <TableCell className="text-xs text-muted-foreground">
          {hook.event_types.length}
        </TableCell>
        <TableCell className="text-xs">{hook.sink_name}</TableCell>
        <TableCell>
          {hook.enabled ? (
            <Badge variant="outline">{t("hooks.enabled")}</Badge>
          ) : (
            <Badge variant="secondary">{t("hooks.disabled")}</Badge>
          )}
        </TableCell>
        <TableCell>
          {canWrite ? (
            <div className="flex flex-wrap gap-2" onClick={(e) => e.stopPropagation()}>
              <Button
                onClick={() => onEdit(hook)}
                size="sm"
                variant="outline"
              >
                {t("outbound.hookEdit")}
              </Button>
              {hook.enabled ? (
                <Button
                  onClick={() => onDisable(hook.id)}
                  size="sm"
                  variant="outline"
                >
                  {t("hooks.disable")}
                </Button>
              ) : (
                <Button
                  onClick={() => onEnable(hook.id)}
                  size="sm"
                  variant="outline"
                >
                  {t("hooks.enable")}
                </Button>
              )}
              <Button
                onClick={() => onDelete(hook)}
                size="sm"
                variant="outline"
              >
                {t("common.delete")}
              </Button>
            </div>
          ) : null}
        </TableCell>
      </TableRow>
      {expanded ? (
        <TableRow className="bg-muted/30 hover:bg-muted/30">
          <TableCell colSpan={6}>
            <HookDeliveryTable canWrite={canWrite} hookId={hook.id} />
          </TableCell>
        </TableRow>
      ) : null}
    </>
  )
}
