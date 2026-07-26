import { CopyIcon, ExternalLinkIcon } from "lucide-react"
import { Link } from "@tanstack/react-router"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useWorkspaceAutomationDeliveries } from "@/features/workspace/automations/workspace-automations-api"
import {
  deliveryStatusLabel,
  DeliveryStatusDot,
} from "@/features/workspace/automations/shared/automation-status"
import type { WorkspaceAutomationDelivery } from "@/features/workspace/automations/workspace-automations-api"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  deliveryID: string | null
  workspaceSlug?: string
  onReplay?: (deliveryID: string) => void
  replayPending?: boolean
}

function prettyJSON(raw: string): string {
  if (!raw) return ""
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function headersToText(headers: Record<string, string[]>): string {
  return Object.entries(headers)
    .map(([key, values]) => `${key}: ${values.join(", ")}`)
    .join("\n")
}

// WorkspaceAutomationDeliveryDetail 是运行记录详情抽屉（桌面右侧 Sheet + 移动全高）。
// 展示 frozen request/response/usage/error，delivery/event/provider ID，Project 引用和 replay。
// spec §16.4: 「Delivery 详情展示 frozen request/response、delivery/event/provider ID、
//  Project Audit 链接和 replay；不重新按当前 Rule 渲染。」
//
// 详情通过 list 接口（200 条以内）在前端过滤出目标 ID；真实分页由上层 list tab 保证。
export function WorkspaceAutomationDeliveryDetail({
  open,
  onOpenChange,
  deliveryID,
  workspaceSlug,
  onReplay,
  replayPending,
}: Props) {
  const list = useWorkspaceAutomationDeliveries({ limit: 200 })
  const delivery: WorkspaceAutomationDelivery | undefined = (list.data ?? []).find(
    (d) => d.id === deliveryID
  )

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 sm:max-w-lg md:max-w-xl"
      >
        <SheetHeader className="space-y-2 border-b px-4 py-3">
          <SheetTitle className="flex items-center gap-2 text-base">
            运行详情
            {delivery ? (
              <>
                <DeliveryStatusDot status={delivery.status} />
                <span className="text-sm font-normal text-muted-foreground">
                  {deliveryStatusLabel(delivery.status)}
                </span>
              </>
            ) : null}
          </SheetTitle>
          <SheetDescription className="sr-only">
            查看 delivery 的 frozen request/response、provider request ID、Project 引用和 replay。
          </SheetDescription>
          {delivery?.status === "succeeded" ? (
            <p className="text-xs text-muted-foreground">
              Agent 调用成功，不是「业务动作成功」的证明。业务结果请查看 Project config / Audit。
            </p>
          ) : null}
        </SheetHeader>
        {delivery ? (
          <DeliveryDetailBody
            delivery={delivery}
            workspaceSlug={workspaceSlug}
            onReplay={onReplay}
            replayPending={replayPending}
          />
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            加载中...
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}

function DeliveryDetailBody({
  delivery,
  workspaceSlug,
  onReplay,
  replayPending,
}: {
  delivery: WorkspaceAutomationDelivery
  workspaceSlug?: string
  onReplay?: (deliveryID: string) => void
  replayPending?: boolean
}) {
  const requestBody = prettyJSON(delivery.request_body_preview)
  const responseBody = prettyJSON(delivery.response_body_preview)
  const headersText = headersToText(delivery.rendered_headers)
  const usageEntries = Object.entries(delivery.usage)

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="space-y-3 border-b px-4 py-3 text-sm">
        <DetailRow label="delivery" value={delivery.id} mono />
        <DetailRow label="event" value={delivery.event_id || delivery.event_type || "—"} mono />
        <DetailRow
          label="provider"
          value={delivery.provider_request_id || "—"}
          mono
        />
        <DetailRow
          label="HTTP"
          value={
            delivery.response_status_code ? String(delivery.response_status_code) : "—"
          }
          mono
        />
        <DetailRow
          label="project"
          valueNode={
            delivery.project ? (
              workspaceSlug ? (
                <Link
                  to="/workspaces/$workspaceSlug/projects/$projectSlug"
                  params={{
                    workspaceSlug,
                    projectSlug: delivery.project.slug,
                  }}
                  className="inline-flex items-center gap-1 text-primary hover:underline"
                >
                  {delivery.project.slug}
                  <ExternalLinkIcon className="size-3" />
                </Link>
              ) : (
                <span>
                  {delivery.project.slug} · {delivery.project.name}
                </span>
              )
            ) : (
              <span className="text-muted-foreground">—（schedule 无关联 Project）</span>
            )
          }
        />
        {delivery.replay_of_delivery_id ? (
          <DetailRow
            label="replay of"
            value={delivery.replay_of_delivery_id}
            mono
          />
        ) : null}
        <DetailRow
          label="attempts"
          value={`${delivery.attempt_count}/${delivery.max_attempts || "—"}`}
          mono
        />
        {delivery.last_error ? (
          <div className="space-y-1">
            <div className="text-xs font-medium uppercase text-muted-foreground">last_error</div>
            <pre className="max-h-32 overflow-auto rounded-md border border-destructive/30 bg-destructive/5 p-3 text-xs whitespace-pre-wrap text-destructive">
              {delivery.last_error}
            </pre>
          </div>
        ) : null}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
        <Tabs defaultValue="request">
          <TabsList>
            <TabsTrigger value="request">请求</TabsTrigger>
            <TabsTrigger value="response">响应</TabsTrigger>
            <TabsTrigger value="usage">Usage</TabsTrigger>
          </TabsList>
          <TabsContent value="request" className="space-y-2">
            <div className="break-all text-sm">
              <span className="font-medium">{delivery.rendered_method || "POST"}</span>{" "}
              <span className="text-muted-foreground">{delivery.resolved_url}</span>
            </div>
            {headersText ? (
              <pre className="max-h-32 overflow-auto rounded-md border bg-muted p-3 text-xs whitespace-pre-wrap">
                {headersText}
              </pre>
            ) : null}
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">Request Body</span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void navigator.clipboard?.writeText(requestBody)}
              >
                <CopyIcon className="mr-1 h-3 w-3" />
                复制
              </Button>
            </div>
            <pre className="max-h-[280px] overflow-auto rounded-md border bg-muted p-3 text-xs">
              {requestBody || "(空)"}
            </pre>
          </TabsContent>
          <TabsContent value="response" className="space-y-2">
            <div className="flex items-center gap-3 text-sm">
              {delivery.response_status_code ? (
                <Badge
                  variant={
                    delivery.response_status_code >= 200 && delivery.response_status_code < 300
                      ? "default"
                      : "destructive"
                  }
                >
                  HTTP {delivery.response_status_code}
                </Badge>
              ) : (
                <span className="text-muted-foreground">无响应状态码</span>
              )}
            </div>
            <pre className="max-h-[280px] overflow-auto rounded-md border bg-muted p-3 text-xs">
              {responseBody || "(空)"}
            </pre>
          </TabsContent>
          <TabsContent value="usage" className="space-y-1">
            {usageEntries.length > 0 ? (
              usageEntries.map(([key, value]) => (
                <div key={key} className="flex justify-between text-sm">
                  <span className="text-muted-foreground">{key}</span>
                  <span className="font-medium">{String(value)}</span>
                </div>
              ))
            ) : (
              <span className="text-sm text-muted-foreground">无 usage 数据</span>
            )}
          </TabsContent>
        </Tabs>
      </div>
      {onReplay ? (
        <div className="flex justify-end gap-2 border-t px-4 py-3">
          <Button
            variant="outline"
            disabled={replayPending}
            onClick={() => onReplay(delivery.id)}
          >
            {replayPending ? "重新投递中..." : "重新投递"}
          </Button>
        </div>
      ) : null}
    </div>
  )
}

function DetailRow({
  label,
  value,
  valueNode,
  mono,
}: {
  label: string
  value?: string
  valueNode?: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="shrink-0 text-xs uppercase text-muted-foreground">{label}</span>
      <span className={`min-w-0 text-right ${mono ? "font-mono text-xs" : "text-sm"}`}>
        {valueNode ?? value}
      </span>
    </div>
  )
}
