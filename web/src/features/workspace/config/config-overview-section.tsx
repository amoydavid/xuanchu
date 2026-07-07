import type React from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import type { ConfigEffectiveValue } from "./config-definition-api"
import { formatConfigDisplayValue } from "./config-display"

type ConfigOverviewSectionProps = {
  rows: ConfigEffectiveValue[]
  isPending: boolean
  isError: boolean
  // "去配置定义"链接目标。默认 /settings；项目页传 /projects/$projectSlug/settings/definitions。
  goToDefinitionsTo?: string
  goToDefinitionsParams?: Record<string, string>
}

// ConfigOverviewSection 是配置概览只读区块，供全局首页和项目工作台复用。
// 展示 effective 配置值，date/datetime 按本地时区格式化。
export function ConfigOverviewSection({
  rows,
  isPending,
  isError,
  goToDefinitionsTo = "/settings",
  goToDefinitionsParams,
}: ConfigOverviewSectionProps) {
  const { t } = useTranslation()
  return (
    <section className="space-y-2">
      <SectionTitle
        action={
          <Link
            className="text-sm text-primary hover:underline"
            params={goToDefinitionsParams}
            to={goToDefinitionsTo}
          >
            {t("configDefinitions.overviewGoToDefinitions")}
          </Link>
        }
        title={t("configDefinitions.overviewTitle")}
      />
      {isPending ? (
        <OverviewSkeleton />
      ) : isError ? (
        <div className="border bg-card p-3 text-sm text-destructive">
          {t("common.error")}
        </div>
      ) : rows.length === 0 ? (
        <div className="border bg-card p-3 text-sm text-muted-foreground">
          {t("configDefinitions.overviewEmpty")}
        </div>
      ) : (
        <div className="divide-y rounded-lg border bg-card">
          {rows.map((row) => {
            const primary = row.definition.label || row.key
            const sourceText = sourceLabel(row.source, t)
            const displayValue =
              row.value === null
                ? t("configDefinitions.sourceMissing")
                : formatConfigDisplayValue(row.definition.value_type, row.value)
            return (
              <div className="flex items-center gap-3 p-3" key={row.key}>
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium">{primary}</div>
                  <code className="text-xs text-muted-foreground">{row.key}</code>
                </div>
                <div className="max-w-[40%] truncate text-sm text-muted-foreground">
                  {displayValue}
                </div>
                <Badge variant="outline">{sourceText}</Badge>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

function sourceLabel(
  source: string,
  t: (key: string) => string
): string {
  switch (source) {
    case "project":
      return t("configDefinitions.sourceProject")
    case "workspace":
      return t("configDefinitions.sourceWorkspace")
    case "default":
      return t("configDefinitions.sourceDefault")
    default:
      return t("configDefinitions.sourceMissing")
  }
}

function SectionTitle({
  action,
  title,
}: {
  action?: React.ReactNode
  title: string
}) {
  return (
    <div className="flex h-8 items-center justify-between">
      <h2 className="text-sm font-medium">{title}</h2>
      {action}
    </div>
  )
}

function OverviewSkeleton() {
  return (
    <div className="space-y-2 rounded-none border bg-card p-3">
      <Skeleton className="h-8 w-full" />
      <Skeleton className="h-8 w-2/3" />
      <Skeleton className="h-8 w-1/2" />
    </div>
  )
}
