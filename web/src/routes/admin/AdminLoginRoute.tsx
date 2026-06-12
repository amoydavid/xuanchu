import { useNavigate } from "@tanstack/react-router"

import { AdminLoginPage } from "@/pages/AdminLoginPage"

export function AdminLoginRoute() {
  const navigate = useNavigate()

  return (
    <AdminLoginPage
      onSignedIn={() => {
        void navigate({ to: "/admin" })
      }}
    />
  )
}
