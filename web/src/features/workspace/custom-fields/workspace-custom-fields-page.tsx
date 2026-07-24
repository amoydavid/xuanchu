import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApiError } from "@/lib/api"
import {
  deleteWorkspaceCustomField,
  listWorkspaceCustomFields,
  setWorkspaceCustomField,
  type WorkspaceCustomField,
  type WorkspaceCustomFieldInput,
} from "./custom-field-api"
import { CustomFieldDialog } from "./custom-field-dialog"

export const workspaceCustomFieldsQueryKey = (workspaceSlug: string) =>
  ["workspace-custom-fields", workspaceSlug] as const

export function WorkspaceCustomFieldsPage({
  canManage,
  workspaceSlug,
}: {
  canManage: boolean
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const queryKey = workspaceCustomFieldsQueryKey(workspaceSlug)
  const fieldsQuery = useQuery({
    enabled: !!workspaceSlug,
    queryKey,
    queryFn: () => listWorkspaceCustomFields(workspaceSlug),
  })
  const [search, setSearch] = useState("")
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<WorkspaceCustomField>()
  const [error, setError] = useState<string | null>(null)

  const invalidate = async () => {
    await queryClient.invalidateQueries({ queryKey })
    await queryClient.invalidateQueries({
      queryKey: ["workspace-task-uda-definitions", workspaceSlug],
    })
  }
  const saveMutation = useMutation({
    mutationFn: (args: { name: string; input: WorkspaceCustomFieldInput }) =>
      setWorkspaceCustomField(workspaceSlug, args.name, args.input),
    onSuccess: async () => {
      await invalidate()
      setDialogOpen(false)
      setEditing(undefined)
      setError(null)
    },
    onError: (caught) => setError(customFieldError(t, caught)),
  })
  const deleteMutation = useMutation({
    mutationFn: (name: string) =>
      deleteWorkspaceCustomField(workspaceSlug, name),
    onSuccess: async () => {
      await invalidate()
      setError(null)
    },
    onError: (caught) => setError(customFieldError(t, caught)),
  })

  const visible = useMemo(() => {
    const needle = search.trim().toLocaleLowerCase()
    if (!needle) return fieldsQuery.data ?? []
    return (fieldsQuery.data ?? []).filter((field) =>
      `${field.name} ${field.label}`.toLocaleLowerCase().includes(needle)
    )
  }, [fieldsQuery.data, search])

  const openEditor = (field?: WorkspaceCustomField) => {
    setEditing(field)
    setError(null)
    setDialogOpen(true)
  }

  return (
    <main className="space-y-5 p-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="text-xl font-semibold">{t("customFields.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("customFields.description")}
          </p>
        </div>
        {canManage ? (
          <Button onClick={() => openEditor()}>
            {t("customFields.create")}
          </Button>
        ) : null}
      </header>
      <Input
        aria-label={t("customFields.search")}
        className="max-w-sm"
        onChange={(event) => setSearch(event.target.value)}
        placeholder={t("customFields.search")}
        value={search}
      />
      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}
      {fieldsQuery.isPending ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : null}
      {fieldsQuery.isError ? (
        <p className="text-sm text-destructive">
          {t("customFields.loadError")}
        </p>
      ) : null}
      {!fieldsQuery.isPending && visible.length === 0 ? (
        <p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
          {t("customFields.empty")}
        </p>
      ) : null}
      {visible.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("customFields.name")}</TableHead>
                <TableHead>{t("customFields.label")}</TableHead>
                <TableHead>{t("customFields.type")}</TableHead>
                <TableHead>{t("customFields.default")}</TableHead>
                <TableHead>{t("customFields.usage")}</TableHead>
                <TableHead className="text-right">
                  {t("common.actions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((field) => (
                <TableRow key={field.name}>
                  <TableCell className="font-mono">{field.name}</TableCell>
                  <TableCell>{field.label || field.name}</TableCell>
                  <TableCell>{t(`customFields.types.${field.type}`)}</TableCell>
                  <TableCell>{field.default || "-"}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-2">
                      <span>
                        {t("customFields.usageText", {
                          tasks: field.task_value_count,
                          series: field.active_series_value_count,
                        })}
                      </span>
                      {field.source !== "database" ? (
                        <Badge variant="secondary">
                          {t(`customFields.sources.${field.source}`)}
                        </Badge>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-2">
                      {canManage ? (
                        <Button
                          onClick={() => openEditor(field)}
                          size="sm"
                          variant="outline"
                        >
                          {field.source === "runtime"
                            ? t("customFields.createOverride")
                            : t("customFields.edit")}
                        </Button>
                      ) : null}
                      {canManage && field.source !== "runtime" ? (
                        <Button
                          disabled={deleteMutation.isPending}
                          onClick={() => {
                            const message =
                              field.source === "database_override"
                                ? t("customFields.deleteOverrideConfirm")
                                : t("customFields.deleteConfirm")
                            if (window.confirm(message)) {
                              void deleteMutation.mutateAsync(field.name)
                            }
                          }}
                          size="sm"
                          variant="ghost"
                        >
                          {t("common.delete")}
                        </Button>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
      ) : null}
      <CustomFieldDialog
        error={error}
        field={editing}
        key={`${editing?.name ?? "new"}:${dialogOpen ? "open" : "closed"}`}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) setEditing(undefined)
        }}
        onSave={async (name, input) => {
          await saveMutation.mutateAsync({ name, input })
        }}
        open={dialogOpen}
        pending={saveMutation.isPending}
      />
    </main>
  )
}

function customFieldError(t: (key: string) => string, caught: unknown): string {
  if (!(caught instanceof ApiError)) return t("common.error")
  switch (caught.code) {
    case "uda_active_series_in_use":
      return t("customFields.errors.activeSeriesInUse")
    case "uda_active_series_incompatible":
      return t("customFields.errors.activeSeriesIncompatible")
    case "uda_runtime_readonly":
      return t("customFields.errors.runtimeReadonly")
    case "uda_definition_invalid":
      return t("customFields.errors.invalidDefinition")
    default:
      return caught.code
  }
}
