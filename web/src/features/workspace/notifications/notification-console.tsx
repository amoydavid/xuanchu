import { useTranslation } from "react-i18next"

import { ResourcePage, statusCell, textCell } from "@/pages/ResourcePage"

// NotificationConsole 定位为「通知管控台」，不是用户消息收件箱。
// 当前能力分为 sinks / rules / deliveries，无 read/delete 收件箱语义（spec §2.4）。
// 第一版只展示 sinks；rules/deliveries 等后续 milestone 再扩展写操作。
export function NotificationConsole() {
  const { t } = useTranslation()
  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-normal">
          {t("page.notifications")}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("notificationConsole.managementHint")}
        </p>
      </div>
      <ResourcePage
        columns={[
          { key: "name", header: t("common.name"), render: textCell("name") },
          { key: "type", header: t("resource.type"), render: statusCell("type") },
          { key: "enabled", header: t("resource.enabled"), render: statusCell("enabled") },
        ]}
        path="/api/v1/notification-sinks"
        title={t("notificationConsole.sinksTitle")}
      />
    </div>
  )
}
