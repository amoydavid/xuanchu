import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Outlet, useLocation, useNavigate } from "@tanstack/react-router"

import { AppShell } from "@/components/AppShell"
import {
  getWorkspaceToken,
  clearWorkspaceToken,
  getAdminActingToken,
  clearAdminActingSession,
  clearTenantSwitchContext,
  clearTenantSwitchSession,
  getTenantSwitchContext,
} from "@/features/workspace/session/workspace-token"
import { useMe } from "@/features/workspace/session/useMe"
import { LoginPage } from "@/pages/LoginPage"
import { ApiError } from "@/lib/api"

export function WorkspaceRootRoute() {
  // signedIn 涵盖两种登录态：
  // - 普通 workspace token（PAT/Agent）
  // - server admin acting token（acting mode 从 /admin/workspaces 进入）
  // acting token 失效时由 workspace-api 的 onUnauthorized 清理并跳回 /admin/workspaces。
  const [signedIn, setSignedIn] = useState(
    () => getWorkspaceToken() !== null || getAdminActingToken() !== null
  )
  const queryClient = useQueryClient()
  const location = useLocation()
  const navigate = useNavigate()
  const me = useMe(signedIn)
  const authFailed = me.error instanceof ApiError && me.error.status === 401
  const redirectPath = sanitizeRedirectPath(location.href)

  useEffect(() => {
    if (authFailed) {
      // 区分清理对象：acting mode 下只清 acting session，普通模式清 workspace token。
      if (getAdminActingToken() !== null) {
        clearAdminActingSession()
      } else if (getTenantSwitchContext() !== null) {
        clearTenantSwitchSession()
      } else {
        clearWorkspaceToken()
        clearTenantSwitchContext()
      }
    }
  }, [authFailed])

  if (!signedIn || authFailed) {
    return (
      <LoginPage
        onSignedIn={() => {
          setSignedIn(true)
          void queryClient.invalidateQueries({
            queryKey: ["credentials", "current"],
          })
          if (redirectPath) {
            void navigate({ to: redirectPath })
          }
        }}
        redirectPath={redirectPath}
      />
    )
  }

  return (
    <AppShell
      actorName={me.data?.actor.display_name ?? me.data?.actor.name}
      onLogout={() => {
        // acting mode 退出只清 acting session（admin token 留给超管控制面）；
        // 普通模式清 workspace token。
        if (getAdminActingToken() !== null) {
          clearAdminActingSession()
        } else if (getTenantSwitchContext() !== null) {
          clearTenantSwitchSession()
        } else {
          clearWorkspaceToken()
          clearTenantSwitchContext()
        }
        queryClient.clear()
        setSignedIn(false)
      }}
      onRefresh={() => void queryClient.invalidateQueries()}
      tokenType={me.data?.token.type}
      workspaceSlug={me.data?.effective_workspace.slug}
    >
      <Outlet />
    </AppShell>
  )
}

export function sanitizeRedirectPath(path: string): string | undefined {
  if (!path.startsWith("/") || path.startsWith("//")) {
    return undefined
  }
  if (hasControlCharacter(path)) {
    return undefined
  }
  if (path === "/" || path.startsWith("/admin")) {
    return undefined
  }
  return path
}

function hasControlCharacter(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index)
    if (code < 32 || code === 127) {
      return true
    }
  }
  return false
}
