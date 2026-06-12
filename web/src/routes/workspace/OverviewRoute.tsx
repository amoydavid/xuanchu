import { OverviewPage } from "@/pages/OverviewPage"
import { useMe } from "@/features/workspace/session/useMe"

export function OverviewRoute() {
  const me = useMe()

  return <OverviewPage me={me.data} />
}
