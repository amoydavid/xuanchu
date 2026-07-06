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
  hasSsoBrowserSession,
  endSsoBrowserSession,
} from "@/features/workspace/session/workspace-token"
import { useMe } from "@/features/workspace/session/useMe"
import { LoginPage } from "@/pages/LoginPage"
import { navigateToDocument } from "@/lib/browser-navigation"
import { ApiError } from "@/lib/api"

export function WorkspaceRootRoute() {
  // signedIn 涵盖两种登录态：
  // - 普通 workspace token（PAT/Agent）
  // - server admin acting token（acting mode 从 /admin/workspaces 进入）
  // - OIDC browser session cookie（sso 登录后由服务端设置）
  const hasSsoCookie = () => {
    if (typeof document === "undefined") return false
    return document.cookie.split("; ").some((row) => row.startsWith("xuanchu_csrf="))
  }
  const [signedIn, setSignedIn] = useState(
    () => getWorkspaceToken() !== null || getAdminActingToken() !== null || hasSsoCookie()
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
      onLogout={async () => {
        // 登出分三种模式，先记录再清理：
        // - acting mode：退出只清 acting session（admin token 留给超管控制面）。
        // - tenant switch：清 tenant session。
        // - 普通模式：清 workspace token。
        // - SSO cookie 模式（无上述 token，但有 OIDC session cookie）：
        //   必须调后端 POST /auth/logout 清 HttpOnly cookie，否则刷新后仍为登录态。
        const wasActing = getAdminActingToken() !== null
        const wasTenant = getTenantSwitchContext() !== null
        const wasSso = !wasActing && !wasTenant && hasSsoBrowserSession()

        if (wasActing) {
          clearAdminActingSession()
        } else if (wasTenant) {
          clearTenantSwitchSession()
        } else {
          clearWorkspaceToken()
          clearTenantSwitchContext()
        }
        queryClient.clear()

        if (wasSso) {
          // SSO 模式：清后端 cookie 后整页跳转，避免 signedIn state 残留。
          try {
            await endSsoBrowserSession()
          } catch {
            // cookie 可能已失效，忽略错误，继续整页跳转到登录页。
          }
          navigateToDocument("/")
          return
        }
        setSignedIn(false)
      }}
      onRefresh={() => void queryClient.invalidateQueries()}
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
