import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Navigate, Outlet } from "@tanstack/react-router"

import { ApiError } from "@/lib/api"
import { AdminShell } from "@/features/admin/components/AdminShell"
import {
  clearAdminToken,
  getAdminToken,
} from "@/features/admin/session/admin-token"
import { useAdminSession } from "@/features/admin/session/useAdminSession"

export function AdminGuardRoute() {
  const [signedIn, setSignedIn] = useState(() => getAdminToken() !== null)
  const queryClient = useQueryClient()
  const session = useAdminSession(signedIn)
  const authFailed =
    session.error instanceof ApiError &&
    (session.error.status === 401 || session.error.status === 404)

  useEffect(() => {
    if (authFailed) {
      clearAdminToken()
    }
  }, [authFailed])

  if (!signedIn || authFailed) {
    return <Navigate to="/admin/login" />
  }

  return (
    <AdminShell
      onLogout={() => {
        clearAdminToken()
        setSignedIn(false)
        void queryClient.removeQueries({ queryKey: ["admin"] })
      }}
      onRefresh={() =>
        void queryClient.invalidateQueries({ queryKey: ["admin"] })
      }
      tokenName={session.data?.token_name}
    >
      <Outlet />
    </AdminShell>
  )
}
