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
  listHookDeliveries,
  replayHookDelivery,
  type HookDelivery,
} from "../outbound-api"
import { DeliveryDetailDialog } from "./delivery-detail-dialog"

const REPLAYABLE_STATUSES = new Set([
  "dead_lettered",
  "retry_wait",
  "failed",
  "timeout",
])

export function HookDeliveryTable({
  hookId,
  canWrite,
}: {
  hookId: string
  canWrite: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [detailId, setDetailId] = useState<string | null>(null)

  const query = useQuery<HookDelivery[]>({
    queryKey: ["outbound", "hooks", hookId, "deliveries"],
    queryFn: () => listHookDeliveries(hookId),
  })

  const replay = useMutation({
    mutationFn: (deliveryId: string) => replayHookDelivery(deliveryId),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: ["outbound", "hooks", hookId, "deliveries"],
      }),
  })

  if (query.isLoading) {
    return (
      <div className="py-3 text-xs text-muted-foreground">
        {t("common.loading")}
      </div>
    )
  }
  if (query.isError) {
    return <div className="py-3 text-xs text-destructive">{t("common.error")}</div>
  }
  const deliveries = query.data ?? []
  if (deliveries.length === 0) {
    return (
      <div className="py-3 text-xs text-muted-foreground">{t("common.empty")}</div>
    )
  }

  return (
    <>
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
            const replayable = REPLAYABLE_STATUSES.has(delivery.status)
            return (
              <TableRow key={delivery.id}>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                  {formatTime(delivery.last_attempt_at ?? delivery.created_at)}
                </TableCell>
                <TableCell className="text-xs">{delivery.event_type}</TableCell>
                <TableCell>
                  <DeliveryStatusBadge status={delivery.status} />
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
                  <div className="flex flex-wrap gap-2">
                    <Button
                      onClick={() => setDetailId(delivery.id)}
                      size="sm"
                      variant="outline"
                    >
                      {t("outbound.deliveryDetail")}
                    </Button>
                    {canWrite && replayable ? (
                      <Button
                        onClick={() => replay.mutate(delivery.id)}
                        size="sm"
                        variant="outline"
                      >
                        {t("outbound.hookReplay")}
                      </Button>
                    ) : null}
                  </div>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
      <DeliveryDetailDialog
        deliveryId={detailId}
        kind="hook"
        onOpenChange={(o) => !o && setDetailId(null)}
      />
    </>
  )
}

function DeliveryStatusBadge({ status }: { status: string }) {
  const variant =
    status === "succeeded"
      ? ("outline" as const)
      : status === "dead_lettered"
        ? ("destructive" as const)
        : ("secondary" as const)
  return <Badge variant={variant}>{status}</Badge>
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
