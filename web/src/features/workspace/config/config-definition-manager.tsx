import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import {
  type ConfigAllowedScope,
  type ConfigSchemaDefinition,
  type ConfigSchemaInput,
  type ConfigSchemaUsage,
  deleteConfigSchema,
  getConfigSchemaUsage,
  listConfigSchema,
  setConfigSchema,
} from "./config-definition-api"
import { ConfigDefinitionForm } from "./config-definition-form"

type ConfigDefinitionManagerProps = {
  variant: "workspace" | "project"
  title: string
  description: string
  defaultScopes: ConfigAllowedScope[]
  canManage: boolean
}

type ScopeFilter = "all" | "workspace" | "project"

export function ConfigDefinitionManager({
  variant,
  title,
  description,
  defaultScopes,
  canManage,
}: ConfigDefinitionManagerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const queryKey = useMemo(() => ["config-schema"] as const, [])

  const schemaQuery = useQuery({
    queryKey,
    queryFn: () => listConfigSchema(),
  })

  const [search, setSearch] = useState("")
  // project variant 默认只看 project 可写
  const [scopeFilter, setScopeFilter] = useState<ScopeFilter>(
    variant === "project" ? "project" : "all"
  )
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [usageCache, setUsageCache] = useState<Record<string, ConfigSchemaUsage>>(
    {}
  )
  const [rowError, setRowError] = useState<string | null>(null)

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey })
    setUsageCache({})
  }

  const saveMutation = useMutation({
    mutationFn: (args: { key: string; input: ConfigSchemaInput }) =>
      setConfigSchema(args.key, args.input),
    onSuccess: () => {
      invalidate()
      setEditingKey(null)
      setCreating(false)
      setRowError(null)
    },
    onError: (err) => {
      setRowError(err instanceof ApiError ? err.message : t("common.error"))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (key: string) => deleteConfigSchema(key, true),
    onSuccess: () => {
      invalidate()
      setEditingKey(null)
      setCreating(false)
      setRowError(null)
    },
    onError: (err) => {
      setRowError(err instanceof ApiError ? err.message : t("common.error"))
    },
  })

  const openEditor = async (key: string) => {
    setCreating(false)
    setEditingKey(key)
    setRowError(null)
    if (!usageCache[key]) {
      try {
        const usage = await getConfigSchemaUsage(key)
        setUsageCache((prev) => ({ ...prev, [key]: usage }))
      } catch {
        // usage 读取失败不阻塞编辑；表单按 0 处理
      }
    }
  }

  const allDefinitions = schemaQuery.data ?? []
  const visible = allDefinitions.filter((def) => {
    if (!matchesScopeFilter(def, scopeFilter)) return false
    if (search.trim() !== "") {
      const needle = search.trim().toLowerCase()
      const hay = `${def.key} ${def.label}`.toLowerCase()
      if (!hay.includes(needle)) return false
    }
    return true
  })

  return (
    <section className="space-y-4">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold">{title}</h2>
        <p className="text-sm text-muted-foreground">{description}</p>
      </header>

      <div className="flex flex-wrap items-center gap-2">
        <Input
          aria-label={t("configDefinitions.search")}
          className="max-w-xs"
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t("configDefinitions.search")}
          value={search}
        />
        <Select
          onValueChange={(v) => setScopeFilter(v as ScopeFilter)}
          value={scopeFilter}
        >
          <SelectTrigger className="w-auto" aria-label={t("configDefinitions.scopeAll")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("configDefinitions.scopeAll")}</SelectItem>
            <SelectItem value="workspace">
              {t("configDefinitions.scopeWorkspace")}
            </SelectItem>
            <SelectItem value="project">
              {t("configDefinitions.scopeProject")}
            </SelectItem>
          </SelectContent>
        </Select>
        {canManage ? (
          <Button
            onClick={() => {
              setEditingKey(null)
              setCreating(true)
              setRowError(null)
            }}
            size="sm"
            type="button"
          >
            {t("configDefinitions.create")}
          </Button>
        ) : null}
      </div>

      {rowError ? (
        <p className="text-xs text-destructive">{rowError}</p>
      ) : null}

      {schemaQuery.isError ? (
        <p className="text-sm text-destructive">{t("common.error")}</p>
      ) : null}

      {schemaQuery.isPending ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : null}

      {!schemaQuery.isPending && visible.length === 0 && !creating ? (
        <p className="text-sm text-muted-foreground">
          {t("configDefinitions.noValues")}
        </p>
      ) : null}

      <div className="divide-y rounded-lg border">
        {creating ? (
          <div className="p-4">
            <ConfigDefinitionForm
              mode="create"
              defaultScopes={defaultScopes}
              canManage={canManage}
              onSubmit={async (key, input) => {
                await saveMutation.mutateAsync({ key, input })
              }}
            />
          </div>
        ) : null}

        {visible.map((def) => {
          const isEditing = editingKey === def.key
          return (
            <div key={def.key} className="p-3">
              <DefinitionRow
                def={def}
                usage={usageCache[def.key]}
                canManage={canManage}
                editing={isEditing}
                onEdit={() => openEditor(def.key)}
              />
              {isEditing ? (
                <div className="mt-3 border-t pt-3">
                  <ConfigDefinitionForm
                    mode="edit"
                    initial={def}
                    usage={usageCache[def.key]}
                    defaultScopes={defaultScopes}
                    canManage={canManage}
                    onSubmit={async (key, input) => {
                      await saveMutation.mutateAsync({ key, input })
                    }}
                    onDelete={async (key) => {
                      await deleteMutation.mutateAsync(key)
                    }}
                  />
                </div>
              ) : null}
            </div>
          )
        })}
      </div>
    </section>
  )
}

function matchesScopeFilter(
  def: ConfigSchemaDefinition,
  filter: ScopeFilter
): boolean {
  if (filter === "all") return true
  const scopes = def.allowed_scopes as string[]
  return scopes.includes(filter)
}

function DefinitionRow({
  def,
  usage,
  canManage,
  editing,
  onEdit,
}: {
  def: ConfigSchemaDefinition
  usage?: ConfigSchemaUsage
  canManage: boolean
  editing: boolean
  onEdit: () => void
}) {
  const { t } = useTranslation()
  const total = usage?.total_values
  return (
    <div className="flex flex-wrap items-center gap-3">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <code className="text-xs">{def.key}</code>
          {def.label ? (
            <span className="text-sm">{def.label}</span>
          ) : null}
          {(def.allowed_scopes as string[]).map((scope) => (
            <Badge key={scope} variant="outline">
              {scope}
            </Badge>
          ))}
          {def.secret ? (
            <Badge variant="secondary">{t("configDefinitions.secret")}</Badge>
          ) : null}
          {def.show_on_console_home ? (
            <Badge variant="secondary">
              {t("configDefinitions.showOnHome")}
            </Badge>
          ) : null}
        </div>
        {def.description ? (
          <p className="text-xs text-muted-foreground">{def.description}</p>
        ) : null}
      </div>
      <div className="text-xs text-muted-foreground">
        {total === undefined
          ? "—"
          : t("configDefinitions.usageValues", {
              workspace: usage?.workspace_values ?? 0,
              project: usage?.project_values ?? 0,
            })}
      </div>
      {canManage ? (
        <Button
          onClick={onEdit}
          size="sm"
          type="button"
          variant={editing ? "secondary" : "outline"}
        >
          {editing ? t("common.cancel") : t("configDefinitions.edit")}
        </Button>
      ) : null}
    </div>
  )
}
