import { ShieldAlert } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"

export function AdminRiskBadge() {
  const { t } = useTranslation()

  return (
    <Badge
      className="gap-1 border-destructive/40 bg-destructive/10 text-destructive"
      variant="outline"
    >
      <ShieldAlert className="size-3" />
      {t("admin.highRisk")}
    </Badge>
  )
}
