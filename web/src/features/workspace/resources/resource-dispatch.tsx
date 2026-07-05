import type React from "react"

import type { PageKey } from "@/components/AppShell"
import { AuditConsole } from "@/features/workspace/audit/audit-console"

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
    default:
      return fallback(resourceConfig(page, t, workspaceSlug))
  }
}
