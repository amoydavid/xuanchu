import { Badge } from "@/components/ui/badge"

export function RiskBadge({ risk }: { risk?: string }) {
  if (risk === "high") {
    return <Badge variant="destructive">high</Badge>
  }
  return <Badge variant="outline">normal</Badge>
}
