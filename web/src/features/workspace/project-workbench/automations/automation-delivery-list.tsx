import { useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"

import { listProjectAutomationDeliveries } from "./project-automations-api"

type Props = {
  projectSlug: string
  ruleID?: string
  onSelectDelivery?: (deliveryID: string) => void
}

// AutomationDeliveryList 展示投递运行记录，支持点击查看详情。
export function AutomationDeliveryList({ projectSlug, ruleID, onSelectDelivery }: Props) {
  const deliveries = useQuery({
    queryKey: ["project", projectSlug, "automation-deliveries", ruleID ?? ""],
    queryFn: () => listProjectAutomationDeliveries(projectSlug, ruleID),
  })
  const rows = deliveries.data ?? []

  return (
    <section className="space-y-2">
      <h3 className="text-sm font-medium">运行记录</h3>
      {rows.length === 0 ? (
        <div className="rounded-md border p-4 text-sm text-muted-foreground">
          暂无运行记录
        </div>
      ) : (
        <div className="overflow-hidden rounded-md border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/40 text-left">
                <th className="p-2">状态</th>
                <th className="p-2">触发</th>
                <th className="p-2">HTTP</th>
                <th className="p-2">Provider ID</th>
                <th className="p-2">尝试</th>
                <th className="p-2">操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.id} className="border-b">
                  <td className="p-2">{row.status}</td>
                  <td className="p-2">{row.event_type || row.trigger_type}</td>
                  <td className="p-2">{row.response_status_code ?? "-"}</td>
                  <td className="p-2">{row.provider_request_id || "-"}</td>
                  <td className="p-2">{row.attempt_count}</td>
                  <td className="p-2">
                    {onSelectDelivery ? (
                      <Button variant="ghost" size="sm" onClick={() => onSelectDelivery(row.id)}>
                        详情
                      </Button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
