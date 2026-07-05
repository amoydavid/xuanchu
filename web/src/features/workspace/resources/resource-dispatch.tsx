import type React from "react"

import type { PageKey } from "@/components/AppShell"
import { AuditConsole } from "@/features/workspace/audit/audit-console"
import { HookConsole } from "@/features/workspace/hooks/hook-console"
import { NotificationConsole } from "@/features/workspace/notifications/notification-console"
import { WorkspaceConsole } from "@/features/workspace/workspaces/workspace-console"

import { resourceConfig } from "./resource-config"

// ResourceDispatch 把资源页分发到专用控制台或保留的只读 ResourcePage。
// 优先级：高频且有明确操作闭环的资源走专用组件；低频资源走 fallback。
type Translate = (key: string) => string

type FallbackProps = React.ComponentProps<
  typeof import("@/pages/ResourcePage").ResourcePage
>

export function ResourceDispatch({
  fallback,
  page,
  t,
  workspaceSlug,
}: {
  fallback: (props: FallbackProps) => React.ReactElement
  page: PageKey
  t: Translate
  workspaceSlug?: string
}) {
  switch (page) {
    case "audit":
      return <AuditConsole workspaceSlug={workspaceSlug} />
    case "hooks":
      return <HookConsole canWrite={true} />
    case "workspaces":
      return <WorkspaceConsole canWrite={true} />
    case "notifications":
      return <NotificationConsole />
    default:
      return fallback(resourceConfig(page, t, workspaceSlug))
  }
}
