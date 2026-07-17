import type { MeResponse } from "@/features/workspace/session/useMe"
import { HomePage } from "@/features/workspace/home/home-page"

type OverviewPageProps = {
  me?: MeResponse
}

// OverviewPage 保留旧导出名，路由与其它页面无需为首页改名。
export function OverviewPage({ me }: OverviewPageProps) {
  return <HomePage me={me} />
}

export function PageHeader({
  description,
  title,
}: {
  description?: string
  title: string
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
        {description ? (
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
    </div>
  )
}
