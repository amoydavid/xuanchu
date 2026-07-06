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
  listNotificationDeliveries,
  replayNotificationDelivery,
  type NotificationDelivery,
  type NotificationDeliveryQuery,
} from "../outbound-api"
import { DeliveryDetailDialog } from "./delivery-detail-dialog"

// 与后端 ReplayNotificationDelivery 允许的状态对齐：只有 dead_lettered / disabled_skipped 可重放。
const REPLAYABLE_STATUSES = new Set(["dead_lettered", "disabled_skipped"])

export function NotificationDeliveryTable({
  query,
  canWrite,
}: {
  query?: NotificationDeliveryQuery
  canWrite: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [detailId, setDetailId] = useState<string | null>(null)

  const listQuery = useQuery<NotificationDelivery[]>({
    queryKey: ["outbound", "deliveries", query ?? {}],
    queryFn: () => listNotificationDeliveries(query ?? {}),
  })

  const replay = useMutation({
    mutationFn: (deliveryId: string) => replayNotificationDelivery(deliveryId),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: ["outbound", "deliveries"],
      }),
  })

  if (listQuery.isLoading) {
    return (
      <div className="border bg-card p-6 text-sm text-muted-foreground">
        {t("common.loading")}
      </div>
    )
  }
  if (listQuery.isError) {
    return (
      <div className="border bg-card p-4 text-sm text-destructive">
        {t("common.error")}
      </div>
    )
  }
  const deliveries = listQuery.data ?? []
  if (deliveries.length === 0) {
    return (
      <div className="border bg-card p-6 text-sm text-muted-foreground">
        {t("common.empty")}
      </div>
    )
  }

  return (
    <>
      <div className="border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("audit.time")}</TableHead>
              <TableHead>{t("hooks.event")}</TableHead>
              <TableHead>sink</TableHead>
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
                    {formatTime(
                      delivery.last_attempt_at ?? delivery.created_at
                    )}
                  </TableCell>
                  <TableCell className="text-xs">{delivery.event_type}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {delivery.sink_id}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        delivery.status === "succeeded"
                          ? "outline"
                          : delivery.status === "dead_lettered"
                            ? "destructive"
                            : "secondary"
                      }
                    >
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
      </div>
      <DeliveryDetailDialog
        deliveryId={detailId}
        kind="notification"
        onOpenChange={(o) => !o && setDetailId(null)}
      />
    </>
  )
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
