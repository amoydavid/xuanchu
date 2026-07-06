import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { ApiError } from "@/lib/api"

import {
  getHookDelivery,
  getNotificationDelivery,
} from "../outbound-api"

type Props = {
  deliveryId: string | null
  kind: "hook" | "notification"
  onOpenChange: (open: boolean) => void
}

export function DeliveryDetailDialog({
  deliveryId,
  kind,
  onOpenChange,
}: Props) {
  const { t } = useTranslation()
  const open = !!deliveryId

  const query = useQuery({
    enabled: !!deliveryId,
    queryKey: ["outbound", "delivery", kind, deliveryId],
    queryFn: async () => {
      if (!deliveryId) return null
      return kind === "hook"
        ? getHookDelivery(deliveryId)
        : getNotificationDelivery(deliveryId)
    },
  })

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("outbound.deliveryDetail")}</DialogTitle>
          <DialogDescription>{deliveryId ?? ""}</DialogDescription>
        </DialogHeader>
        {query.isLoading ? (
          <div className="text-sm text-muted-foreground">
            {t("common.loading")}
          </div>
        ) : query.isError ? (
          <div className="text-sm text-destructive">
            {query.error instanceof ApiError
              ? query.error.message
              : t("common.error")}
          </div>
        ) : (
          <DeliveryDetailBody data={query.data} />
        )}
      </DialogContent>
    </Dialog>
  )
}

function DeliveryDetailBody({ data }: { data: unknown }) {
  const { t } = useTranslation()
  if (!data || typeof data !== "object") return null
  const row = data as Record<string, unknown>

  return (
    <div className="space-y-3 text-xs">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {DetailField({ row, key: "status" })}
        {DetailField({ row, key: "event_type" })}
        {DetailField({ row, key: "event_id" })}
        {DetailField({ row, key: "attempt_count", label: "attempts" })}
        {DetailField({ row, key: "last_status_code", label: "http" })}
        {DetailField({ row, key: "last_error", label: "error" })}
        {DetailField({ row, key: "next_attempt_at", label: "next_attempt_at" })}
        {DetailField({ row, key: "resolved_url", label: "resolved_url" })}
        {DetailField({
          row,
          key: "resolved_endpoint_source",
          label: "endpoint_source",
        })}
        {DetailField({
          row,
          key: "resolved_endpoint_fingerprint",
          label: "fingerprint",
        })}
        {DetailField({ row, key: "rendered_method", label: "method" })}
        {DetailField({
          row,
          key: "rendered_content_type",
          label: "content_type",
        })}
      </div>
      {row.payload ? (
        <JsonBlock label={t("outbound.deliveryPayload")} value={row.payload} />
      ) : null}
      {row.headers && typeof row.headers === "object" ? (
        <JsonBlock
          label={t("outbound.deliveryHeaders")}
          value={row.headers}
        />
      ) : null}
      {row.rendered_headers && typeof row.rendered_headers === "object" ? (
        <JsonBlock
          label={t("outbound.deliveryHeaders")}
          value={row.rendered_headers}
        />
      ) : null}
      {typeof row.rendered_body === "string" && row.rendered_body ? (
        <div className="space-y-1">
          <div className="font-medium">rendered_body</div>
          <pre className="overflow-x-auto border bg-muted/30 p-2 text-xs">
            {row.rendered_body}
          </pre>
        </div>
      ) : null}
    </div>
  )
}

function DetailField({
  row,
  key: fieldKey,
  label,
}: {
  row: Record<string, unknown>
  key: string
  label?: string
}) {
  const value = row[fieldKey]
  if (value === undefined || value === null || value === "") return null
  return (
    <div>
      <div className="text-muted-foreground">{label ?? fieldKey}</div>
      <div className="font-medium break-all">{formatValue(value)}</div>
    </div>
  )
}

function formatValue(v: unknown): string {
  if (typeof v === "object") return JSON.stringify(v)
  return String(v)
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="space-y-1">
      <div className="font-medium">{label}</div>
      <pre className="max-h-80 overflow-auto border bg-muted/30 p-2 text-xs">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  )
}
