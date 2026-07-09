import { useQuery } from "@tanstack/react-query"
import { CopyIcon, LoaderIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

import { getProjectAutomationDelivery, type ProjectAutomationDelivery } from "./project-automations-api"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectSlug: string
  deliveryID: string | null
}

const FINAL_STATUSES = ["succeeded", "dead_lettered"]

// prettyJSON 尝试美化 JSON 字符串，失败则原样返回。
function prettyJSON(raw: string): string {
  if (!raw) return ""
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function statusBadgeVariant(status: string) {
  switch (status) {
    case "succeeded":
      return "default" as const
    case "dead_lettered":
      return "destructive" as const
    default:
      return "secondary" as const
  }
}

function statusLabel(status: string) {
  switch (status) {
    case "queued":
      return "排队中"
    case "delivering":
      return "投递中"
    case "retry_wait":
      return "等待重试"
    case "succeeded":
      return "成功"
    case "dead_lettered":
      return "失败（死信）"
    default:
      return status
  }
}

// headersToText 把 rendered_headers（map<string, string[]>）格式化为可读文本。
function headersToText(headers: Record<string, string[]>): string {
  return Object.entries(headers)
    .map(([key, values]) => `${key}: ${values.join(", ")}`)
    .join("\n")
}

// usageToEntries 把 usage map 转为可读的 key-value 行。
function usageToEntries(usage: Record<string, unknown>): Array<[string, string]> {
  return Object.entries(usage).map(([key, value]) => [key, String(value)])
}

// DeliveryDetail 渲染单条投递的请求/响应/usage/错误详情，供 Dialog 内部使用。
function DeliveryDetail({ delivery }: { delivery: ProjectAutomationDelivery }) {
  const requestBody = prettyJSON(delivery.request_body_preview)
  const responseBody = prettyJSON(delivery.response_body_preview)
  const usageEntries = usageToEntries(delivery.usage)
  const headersText = headersToText(delivery.rendered_headers)

  return (
    <Tabs defaultValue="request" className="w-full">
      <TabsList>
        <TabsTrigger value="request">请求</TabsTrigger>
        <TabsTrigger value="response">响应</TabsTrigger>
        <TabsTrigger value="usage">Usage</TabsTrigger>
        {(delivery.last_error || delivery.status === "dead_lettered" || delivery.status === "retry_wait") ? (
          <TabsTrigger value="error">错误</TabsTrigger>
        ) : null}
      </TabsList>

      {/* 请求 tab */}
      <TabsContent value="request" className="space-y-2">
        <div className="text-sm">
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
          <Button variant="ghost" size="sm" onClick={() => void navigator.clipboard?.writeText(requestBody)}>
            <CopyIcon className="mr-1 h-3 w-3" />
            复制
          </Button>
        </div>
        <pre className="max-h-[300px] overflow-auto rounded-md border bg-muted p-3 text-xs">
          {requestBody || "(空)"}
        </pre>
      </TabsContent>

      {/* 响应 tab */}
      <TabsContent value="response" className="space-y-2">
        <div className="flex items-center gap-3 text-sm">
          {delivery.response_status_code ? (
            <Badge variant={delivery.response_status_code >= 200 && delivery.response_status_code < 300 ? "default" : "destructive"}>
              HTTP {delivery.response_status_code}
            </Badge>
          ) : (
            <span className="text-muted-foreground">无响应状态码</span>
          )}
          {delivery.provider_request_id ? (
            <span className="text-muted-foreground">provider_id: {delivery.provider_request_id}</span>
          ) : null}
        </div>
        <span className="text-xs text-muted-foreground">Response Body</span>
        <pre className="max-h-[300px] overflow-auto rounded-md border bg-muted p-3 text-xs">
          {responseBody || "(空)"}
        </pre>
      </TabsContent>

      {/* Usage tab */}
      <TabsContent value="usage" className="space-y-2">
        {usageEntries.length > 0 ? (
          <div className="space-y-1">
            {usageEntries.map(([key, value]) => (
              <div key={key} className="flex justify-between text-sm">
                <span className="text-muted-foreground">{key}</span>
                <span className="font-medium">{value}</span>
              </div>
            ))}
          </div>
        ) : (
          <span className="text-sm text-muted-foreground">无 usage 数据</span>
        )}
      </TabsContent>

      {/* 错误 tab */}
      <TabsContent value="error" className="space-y-2">
        {delivery.last_error ? (
          <pre className="max-h-[300px] overflow-auto rounded-md border bg-destructive/10 p-3 text-xs whitespace-pre-wrap text-destructive">
            {delivery.last_error}
          </pre>
        ) : (
          <span className="text-sm text-muted-foreground">无错误信息</span>
        )}
        <div className="text-xs text-muted-foreground">
          尝试次数: {delivery.attempt_count}
          {delivery.next_attempt_at ? ` · 下次重试: ${new Date(delivery.next_attempt_at * 1000).toLocaleString()}` : ""}
        </div>
      </TabsContent>
    </Tabs>
  )
}

// AutomationTestDebugDialog 是测试投递的 Debug 弹窗。
// 通过 deliveryID 轮询单条投递，直到终态后展示完整请求/响应/usage/错误。
export function AutomationTestDebugDialog({ open, onOpenChange, projectSlug, deliveryID }: Props) {
  const isFinal = (status: string | undefined) =>
    status !== undefined && FINAL_STATUSES.includes(status)

  const query = useQuery({
    queryKey: ["project", projectSlug, "automation-delivery", deliveryID ?? ""],
    queryFn: () => {
      if (!deliveryID) return null
      return getProjectAutomationDelivery(projectSlug, deliveryID)
    },
    enabled: open && !!deliveryID,
    // 非终态时每 2 秒轮询，终态后停止。
    refetchInterval: (query) => {
      const status = (query.state.data as ProjectAutomationDelivery | null | undefined)?.status
      return isFinal(status) ? false : 2000
    },
  })

  const delivery = query.data
  const pending = !delivery || !isFinal(delivery.status)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            测试投递结果
            {delivery ? (
              <Badge variant={statusBadgeVariant(delivery.status)}>
                {pending ? (
                  <LoaderIcon className="mr-1 h-3 w-3 animate-spin" />
                ) : null}
                {statusLabel(delivery.status)}
              </Badge>
            ) : (
              <Badge variant="secondary">
                <LoaderIcon className="mr-1 h-3 w-3 animate-spin" />
                加载中
              </Badge>
            )}
          </DialogTitle>
          <DialogDescription className="sr-only">
            查看投递的请求、响应、usage 和错误详情。
          </DialogDescription>
        </DialogHeader>
        {delivery ? (
          <DeliveryDetail delivery={delivery} />
        ) : (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            <LoaderIcon className="mr-2 h-4 w-4 animate-spin" />
            正在获取投递信息...
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
