import type React from "react"

import type { PageKey } from "@/components/AppShell"
import { AuditConsole } from "@/features/workspace/audit/audit-console"
import { WorkspaceConsole } from "@/features/workspace/workspaces/workspace-console"
import {
  OutboundConsole,
  type OutboundTab,
} from "@/features/workspace/outbound/outbound-console"

import { resourceConfig } from "./resource-config"

// ResourceDispatch 把资源页分发到专用控制台或保留的只读 ResourcePage。
// 优先级：高频且有明确操作闭环的资源走专用组件；低频资源走 fallback。
//
// `/hooks` 和 `/notifications` 共用同一个 OutboundConsole（出站集成控制台），
// 只是默认打开的 tab 不同。OutboundConsole 内部根据当前身份自动判定可写性。
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
      return (
        <OutboundConsole
          initialTab="hooks"
          workspaceSlug={workspaceSlug}
        />
      )
    case "notifications":
      return (
        <OutboundConsole
          initialTab="notification-rules"
          workspaceSlug={workspaceSlug}
        />
      )
    case "integrations":
      return (
        <OutboundConsole
          initialTab={"hooks" as OutboundTab}
          workspaceSlug={workspaceSlug}
        />
      )
    case "workspaces":
      return <WorkspaceConsole canWrite={true} />
    default:
      return fallback(resourceConfig(page, t, workspaceSlug))
  }
}
