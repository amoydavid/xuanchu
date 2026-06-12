import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Outlet, useLocation, useNavigate } from "@tanstack/react-router"

import { AppShell } from "@/components/AppShell"
import {
  getWorkspaceToken,
  clearWorkspaceToken,
} from "@/features/workspace/session/workspace-token"
import { useMe } from "@/features/workspace/session/useMe"
import { LoginPage } from "@/pages/LoginPage"
import { ApiError } from "@/lib/api"

export function WorkspaceRootRoute() {
  const [signedIn, setSignedIn] = useState(() => getWorkspaceToken() !== null)
  const queryClient = useQueryClient()
  const location = useLocation()
  const navigate = useNavigate()
  const me = useMe(signedIn)
  const authFailed = me.error instanceof ApiError && me.error.status === 401
  const redirectPath = sanitizeRedirectPath(location.href)

  useEffect(() => {
    if (authFailed) {
      clearWorkspaceToken()
    }
  }, [authFailed])

  if (!signedIn || authFailed) {
    return (
      <LoginPage
        onSignedIn={() => {
          setSignedIn(true)
          void queryClient.invalidateQueries({ queryKey: ["me"] })
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
      actorName={me.data?.actor.name}
      onLogout={() => {
        clearWorkspaceToken()
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
