import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"

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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { ApiError } from "@/lib/api"
import { cn } from "@/lib/utils"

import {
  deleteHook,
  disableHook,
  enableHook,
  listHookDeliveries,
  listHooks,
  replayHookDelivery,
  type Hook,
  type HookDelivery,
} from "./hooks-api"
import { HookCreateDialog } from "./hook-create-dialog"

export function HookConsole({ canWrite }: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const query = useQuery<Hook[]>({
    queryKey: ["hooks"],
    queryFn: listHooks,
  })

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-normal">
            {t("hooks.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("hooks.subtitle")}</p>
        </div>
        {canWrite ? (
          <Button
            onClick={() => setCreateOpen(true)}
            size="sm"
            variant="default"
          >
            {t("hooks.create")}
          </Button>
        ) : null}
      </div>
      <HookCreateDialog
        onCreated={() => void queryClient.invalidateQueries({ queryKey: ["hooks"] })}
        onOpenChange={setCreateOpen}
        open={createOpen}
      />

      {query.isError ? (
        <div className="border bg-card p-4 text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : t("common.error")}
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
                <TableHead>{t("hooks.events")}</TableHead>
                <TableHead>{t("hooks.sink")}</TableHead>
                <TableHead>{t("resource.enabled")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(query.data ?? []).map((hook) => (
                <HookRow canWrite={canWrite} hook={hook} key={hook.id} />
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}

function HookRow({ canWrite, hook }: { canWrite: boolean; hook: Hook }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [expanded, setExpanded] = useState(false)

  const enable = useMutation({
    mutationFn: () => enableHook(hook.id),
    onSuccess: () => invalidate(queryClient),
  })
  const disable = useMutation({
    mutationFn: () => disableHook(hook.id),
    onSuccess: () => invalidate(queryClient),
  })
  const remove = useMutation({
    mutationFn: () => deleteHook(hook.id),
    onSuccess: () => invalidate(queryClient),
  })

  const deliveries = useQuery<HookDelivery[]>({
    enabled: expanded,
    queryKey: ["hooks", hook.id, "deliveries"],
    queryFn: () => listHookDeliveries(hook.id),
  })

  return (
    <>
      <TableRow
        className="cursor-pointer"
        onClick={() => setExpanded((v) => !v)}
      >
        <TableCell className="font-medium">
          <span className={cn(expanded && "text-foreground")}>{hook.name}</span>
        </TableCell>
        <TableCell className="text-xs text-muted-foreground">
          {hook.event_types.join(", ")}
        </TableCell>
        <TableCell className="text-xs">{hook.sink_name}</TableCell>
        <TableCell>
          {hook.enabled ? (
            <Badge variant="outline">{t("hooks.enabled")}</Badge>
          ) : (
            <Badge variant="secondary">{t("hooks.disabled")}</Badge>
          )}
        </TableCell>
      </TableRow>
      {expanded ? (
        <TableRow className="bg-muted/30 hover:bg-muted/30">
          <TableCell colSpan={4}>
            <div className="flex flex-wrap items-center gap-2 pb-3">
              {canWrite ? (
                <>
                  {hook.enabled ? (
                    <Button
                      onClick={() => disable.mutate()}
                      size="sm"
                      variant="outline"
                    >
                      {t("hooks.disable")}
                    </Button>
                  ) : (
                    <Button
                      onClick={() => enable.mutate()}
                      size="sm"
                      variant="outline"
                    >
                      {t("hooks.enable")}
                    </Button>
                  )}
                  <Button
                    onClick={() => {
                      if (window.confirm(t("hooks.deleteConfirm"))) {
                        remove.mutate()
                      }
                    }}
                    size="sm"
                    variant="outline"
                  >
                    {t("common.delete")}
                  </Button>
                </>
              ) : null}
            </div>
            <div className="text-xs font-medium text-muted-foreground">
              {t("hooks.deliveries")}
            </div>
            {deliveries.isLoading ? (
              <div className="py-3 text-xs text-muted-foreground">
                {t("common.loading")}
              </div>
            ) : deliveries.isError ? (
              <div className="py-3 text-xs text-destructive">{t("common.error")}</div>
            ) : (deliveries.data ?? []).length === 0 ? (
              <div className="py-3 text-xs text-muted-foreground">
                {t("common.empty")}
              </div>
            ) : (
              <DeliveryTable deliveries={deliveries.data!} canWrite={canWrite} />
            )}
          </TableCell>
        </TableRow>
      ) : null}
    </>
  )
}

function DeliveryTable({
  deliveries,
  canWrite,
}: {
  deliveries: HookDelivery[]
  canWrite: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const replay = useMutation({
    mutationFn: (deliveryId: string) => replayHookDelivery(deliveryId),
    onSuccess: () => {
      // 简化：直接刷新整个 hooks query（deliveries 是子 key）
      void queryClient.invalidateQueries({ queryKey: ["hooks"] })
    },
  })

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("audit.time")}</TableHead>
          <TableHead>{t("hooks.event")}</TableHead>
          <TableHead>{t("common.status")}</TableHead>
          <TableHead>{t("hooks.code")}</TableHead>
          <TableHead>{t("common.actions")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {deliveries.map((delivery) => {
          const failed = delivery.status !== "succeeded"
          return (
            <TableRow key={delivery.id}>
              <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                {formatTime(delivery.last_attempt_at ?? delivery.created_at)}
              </TableCell>
              <TableCell className="text-xs">{delivery.event_type}</TableCell>
              <TableCell>
                <Badge variant={failed ? "destructive" : "outline"}>
                  {delivery.status}
                </Badge>
                {delivery.last_error ? (
                  <span className="ml-2 text-xs text-muted-foreground">
                    {delivery.last_error}
                  </span>
                ) : null}
              </TableCell>
              <TableCell className="text-xs">
                {delivery.last_status_code ?? "-"}
              </TableCell>
              <TableCell>
                {canWrite && failed ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        onClick={() => replay.mutate(delivery.id)}
                        size="sm"
                        variant="outline"
                      >
                        {t("hooks.replay")}
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t("hooks.replayTooltip")}</TooltipContent>
                  </Tooltip>
                ) : null}
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

function invalidate(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: ["hooks"] })
}

function formatTime(ts: number | null | undefined): string {
  if (!ts) return "-"
  const d = new Date(ts * 1000)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  const hh = String(d.getHours()).padStart(2, "0")
  const mm = String(d.getMinutes()).padStart(2, "0")
  return `${y}-${m}-${day} ${hh}:${mm}`
}
