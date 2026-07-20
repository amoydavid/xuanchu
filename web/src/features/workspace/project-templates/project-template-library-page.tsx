import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  Archive,
  Boxes,
  ChevronLeft,
  ChevronRight,
  FileClock,
  Plus,
  RotateCcw,
  Search,
} from "lucide-react"
import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

import {
  archiveProjectTemplate,
  getProjectTemplate,
  invalidateProjectTemplateMutation,
  listProjectTemplates,
  projectTemplateDetailQueryKey,
  projectTemplateListQueryKey,
  reactivateProjectTemplate,
  type ComponentCounts,
  type ProjectTemplateSummary,
} from "./api/project-template-api"

const PAGE_SIZE = 20

export type ProjectTemplateSelection = {
  templateRef: string
  snapshotID: string
  snapshotHash: string
}

type ProjectTemplateLibraryPageProps = {
  workspaceSlug: string
  canManage: boolean
  initialSearch?: string
  onCreateTemplate?: () => void
  onCaptureSnapshot?: (templateRef: string) => void
  onInstantiate?: (selection: ProjectTemplateSelection) => void
}

export function ProjectTemplateLibraryPage({
  workspaceSlug,
  canManage,
  initialSearch = "",
  onCreateTemplate,
  onCaptureSnapshot,
  onInstantiate,
}: ProjectTemplateLibraryPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [status, setStatus] = useState("all")
  const [searchDraft, setSearchDraft] = useState(initialSearch)
  const [q, setQ] = useState(initialSearch)
  const [offset, setOffset] = useState(0)
  const [selection, setSelection] = useState<{
    ref: string
    snapshotID?: string
  }>()

  const listQuery = useQuery({
    enabled: Boolean(workspaceSlug),
    queryKey: projectTemplateListQueryKey(
      workspaceSlug,
      status,
      q,
      PAGE_SIZE,
      offset
    ),
    queryFn: () =>
      listProjectTemplates(workspaceSlug, {
        status,
        q,
        limit: PAGE_SIZE,
        offset,
      }),
  })

  const listItems = listQuery.data?.items ?? []
  const selectedRef =
    selection && listItems.some((item) => item.key === selection.ref)
      ? selection.ref
      : listItems[0]?.key
  const selectedSnapshotID =
    selection && selection.ref === selectedRef
      ? selection.snapshotID
      : undefined

  const detailQuery = useQuery({
    enabled: Boolean(workspaceSlug && selectedRef),
    queryKey: projectTemplateDetailQueryKey(
      workspaceSlug,
      selectedRef ?? "",
      selectedSnapshotID
    ),
    queryFn: () =>
      getProjectTemplate(workspaceSlug, selectedRef!, selectedSnapshotID),
  })

  const archiveMutation = useMutation({
    mutationFn: (ref: string) => archiveProjectTemplate(workspaceSlug, ref),
    onSuccess: async (_data, ref) => {
      await invalidateProjectTemplateMutation(queryClient, workspaceSlug, {
        kind: "archive",
        ref,
      })
    },
  })
  const reactivateMutation = useMutation({
    mutationFn: (ref: string) => reactivateProjectTemplate(workspaceSlug, ref),
    onSuccess: async (_data, ref) => {
      await invalidateProjectTemplateMutation(queryClient, workspaceSlug, {
        kind: "reactivate",
        ref,
      })
    },
  })

  function submitSearch(event: FormEvent) {
    event.preventDefault()
    setOffset(0)
    setQ(searchDraft.trim())
  }

  if (listQuery.isPending) {
    return <LibraryLoading />
  }

  if (listQuery.isError) {
    return (
      <div className="p-6">
        <Alert variant="destructive">
          <AlertTitle>{t("projectTemplates.loadError")}</AlertTitle>
          <AlertDescription>
            {t("projectTemplates.loadErrorDescription")}
          </AlertDescription>
        </Alert>
        <Button
          className="mt-3"
          onClick={() => listQuery.refetch()}
          variant="outline"
        >
          <RotateCcw />
          {t("common.retry")}
        </Button>
      </div>
    )
  }

  const page = listQuery.data
  const empty = page.items.length === 0

  return (
    <section className="space-y-4 p-6" aria-labelledby="project-template-title">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold" id="project-template-title">
            {t("projectTemplates.title")}
          </h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            {t("projectTemplates.description")}
          </p>
        </div>
        {canManage && !empty ? (
          <Button onClick={onCreateTemplate}>
            <Plus />
            {t("projectTemplates.saveNew")}
          </Button>
        ) : null}
      </header>

      <div className="flex flex-wrap items-center gap-2 border-y bg-muted/20 p-2">
        <form className="flex min-w-64 flex-1 gap-2" onSubmit={submitSearch}>
          <Input
            aria-label={t("projectTemplates.search")}
            onChange={(event) => setSearchDraft(event.target.value)}
            placeholder={t("projectTemplates.search")}
            value={searchDraft}
          />
          <Button
            aria-label={t("projectTemplates.searchAction")}
            size="icon"
            type="submit"
            variant="outline"
          >
            <Search />
          </Button>
        </form>
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          {t("common.status")}
          <select
            aria-label={t("projectTemplates.statusFilter")}
            className="h-8 border bg-background px-2 text-xs text-foreground"
            onChange={(event) => {
              setStatus(event.target.value)
              setOffset(0)
              setSelection(undefined)
              archiveMutation.reset()
              reactivateMutation.reset()
            }}
            value={status}
          >
            <option value="all">{t("projectTemplates.statusAll")}</option>
            <option value="active">{t("projectTemplates.statusActive")}</option>
            <option value="archived">
              {t("projectTemplates.statusArchived")}
            </option>
          </select>
        </label>
        <span className="ml-auto text-xs text-muted-foreground tabular-nums">
          {t("projectTemplates.total", { count: page.total })}
        </span>
      </div>

      {empty ? (
        <LibraryEmpty
          canManage={canManage}
          hasSearch={q !== ""}
          onCreate={onCreateTemplate}
          onClearSearch={() => {
            setSearchDraft("")
            setQ("")
          }}
        />
      ) : (
        <div
          className="grid grid-cols-1 border lg:grid-cols-[minmax(17rem,0.72fr)_minmax(0,1.55fr)]"
          data-testid="project-template-library-layout"
        >
          <aside
            className="border-b lg:border-r lg:border-b-0"
            aria-label={t("projectTemplates.listLabel")}
          >
            <div className="divide-y">
              {page.items.map((item) => (
                <TemplateListItem
                  active={item.key === selectedRef}
                  item={item}
                  key={item.id}
                  onSelect={() => {
                    setSelection({ ref: item.key })
                    archiveMutation.reset()
                    reactivateMutation.reset()
                  }}
                />
              ))}
            </div>
            <Pagination
              limit={page.limit}
              offset={page.offset}
              onOffsetChange={setOffset}
              total={page.total}
            />
          </aside>

          <main className="min-h-[28rem] min-w-0">
            {detailQuery.isPending ? (
              <DetailLoading />
            ) : detailQuery.isError ? (
              <div className="p-6">
                <Alert variant="destructive">
                  <AlertTitle>{t("projectTemplates.detailError")}</AlertTitle>
                </Alert>
                <Button
                  className="mt-3"
                  onClick={() => detailQuery.refetch()}
                  variant="outline"
                >
                  <RotateCcw />
                  {t("common.retry")}
                </Button>
              </div>
            ) : detailQuery.data ? (
              <TemplateDetail
                canManage={canManage}
                detail={detailQuery.data}
                isLifecyclePending={
                  archiveMutation.isPending || reactivateMutation.isPending
                }
                lifecycleError={
                  archiveMutation.isError || reactivateMutation.isError
                }
                onArchive={(ref) => archiveMutation.mutate(ref)}
                onCapture={onCaptureSnapshot}
                onInstantiate={onInstantiate}
                onReactivate={(ref) => reactivateMutation.mutate(ref)}
                onSelectVersion={(snapshotID) =>
                  setSelection({ ref: selectedRef!, snapshotID })
                }
                selectedSnapshotID={selectedSnapshotID}
              />
            ) : null}
          </main>
        </div>
      )}
    </section>
  )
}

function LibraryLoading() {
  const { t } = useTranslation()
  return (
    <div
      aria-label={t("projectTemplates.loading")}
      className="space-y-4 p-6"
      role="status"
    >
      <Skeleton className="h-7 w-44" />
      <Skeleton className="h-10 w-full" />
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-[17rem_1fr]">
        <Skeleton className="h-72" />
        <Skeleton className="h-72" />
      </div>
    </div>
  )
}

function DetailLoading() {
  return (
    <div className="space-y-4 p-6" role="status">
      <Skeleton className="h-7 w-56" />
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-44 w-full" />
    </div>
  )
}

function LibraryEmpty({
  canManage,
  hasSearch,
  onClearSearch,
  onCreate,
}: {
  canManage: boolean
  hasSearch: boolean
  onClearSearch: () => void
  onCreate?: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-64 flex-col items-center justify-center border border-dashed p-8 text-center">
      <Boxes className="mb-3 size-8 text-muted-foreground" />
      <h2 className="text-sm font-medium">
        {t(
          hasSearch
            ? "projectTemplates.noSearchTitle"
            : "projectTemplates.emptyTitle"
        )}
      </h2>
      <p className="mt-1 max-w-md text-xs text-muted-foreground">
        {t(
          hasSearch
            ? "projectTemplates.noSearchDescription"
            : "projectTemplates.emptyDescription"
        )}
      </p>
      {hasSearch ? (
        <Button className="mt-4" onClick={onClearSearch} variant="outline">
          {t("projectTemplates.clearSearch")}
        </Button>
      ) : canManage ? (
        <Button className="mt-4" onClick={onCreate}>
          <Plus />
          {t("projectTemplates.saveNew")}
        </Button>
      ) : null}
    </div>
  )
}

function TemplateListItem({
  active,
  item,
  onSelect,
}: {
  active: boolean
  item: ProjectTemplateSummary
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const counts = item.current_snapshot?.counts
  return (
    <button
      aria-pressed={active}
      className={cn(
        "w-full border-l-2 p-3 text-left transition-colors hover:bg-muted/60",
        active ? "border-l-foreground bg-muted/50" : "border-l-transparent"
      )}
      onClick={onSelect}
      type="button"
    >
      <div className="flex items-start justify-between gap-2">
        <span className="truncate text-sm font-medium">{item.name}</span>
        <div className="flex shrink-0 items-center gap-1">
          {item.status === "archived" ? (
            <Badge variant="outline">{t("projectTemplates.archived")}</Badge>
          ) : null}
          <span className="text-xs text-muted-foreground tabular-nums">
            v{item.current_snapshot?.version ?? "-"}
          </span>
        </div>
      </div>
      <div className="mt-1 truncate font-mono text-[11px] text-muted-foreground">
        {item.key}
      </div>
      {counts ? (
        <div className="mt-2 text-[11px] text-muted-foreground">
          {t("projectTemplates.listCounts", counts)}
        </div>
      ) : null}
    </button>
  )
}

function TemplateDetail({
  canManage,
  detail,
  isLifecyclePending,
  lifecycleError,
  onArchive,
  onCapture,
  onInstantiate,
  onReactivate,
  onSelectVersion,
  selectedSnapshotID,
}: {
  canManage: boolean
  detail: import("./api/project-template-api").ProjectTemplateDetail
  isLifecyclePending: boolean
  lifecycleError: boolean
  onArchive: (ref: string) => void
  onCapture?: (ref: string) => void
  onInstantiate?: (selection: ProjectTemplateSelection) => void
  onReactivate: (ref: string) => void
  onSelectVersion: (id?: string) => void
  selectedSnapshotID?: string
}) {
  const { t, i18n } = useTranslation()
  const template = detail.template
  const currentID = template.current_snapshot?.id
  const selected =
    detail.versions.find((version) => version.id === selectedSnapshotID) ??
    detail.versions.find((version) => version.id === currentID) ??
    template.current_snapshot
  const historical = Boolean(selected && selected.id !== currentID)
  const archived = template.status === "archived"

  return (
    <div className="divide-y">
      <section className="space-y-4 p-5 sm:p-6">
        {archived ? (
          <Alert>
            <Archive />
            <AlertTitle>{t("projectTemplates.archivedTitle")}</AlertTitle>
            <AlertDescription>
              {t("projectTemplates.archivedBanner")}
            </AlertDescription>
          </Alert>
        ) : null}
        {lifecycleError ? (
          <Alert variant="destructive">
            <AlertTitle>{t("projectTemplates.lifecycleError")}</AlertTitle>
            <AlertDescription>
              {t("projectTemplates.lifecycleErrorDescription")}
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-lg font-semibold">
                {template.name}
              </h2>
              {historical ? (
                <Badge variant="outline">
                  {t("projectTemplates.historicalReadonly")}
                </Badge>
              ) : (
                <Badge>{t("projectTemplates.currentVersion")}</Badge>
              )}
            </div>
            <div className="mt-1 font-mono text-xs text-muted-foreground">
              {t("projectTemplates.keyLabel")}: {template.key}
            </div>
          </div>
          {canManage ? (
            <div className="flex flex-wrap gap-2">
              {archived ? (
                <Button
                  disabled={isLifecyclePending}
                  onClick={() => onReactivate(template.key)}
                  variant="outline"
                >
                  <RotateCcw />
                  {t("projectTemplates.reactivate")}
                </Button>
              ) : (
                <>
                  {selected ? (
                    <Button
                      onClick={() =>
                        onInstantiate?.({
                          templateRef: template.key,
                          snapshotID: selected.id,
                          snapshotHash: selected.hash,
                        })
                      }
                    >
                      {t(
                        historical
                          ? "projectTemplates.instantiateHistorical"
                          : "projectTemplates.instantiate"
                      )}
                    </Button>
                  ) : null}
                  {!historical ? (
                    <Button
                      onClick={() => onCapture?.(template.key)}
                      variant="outline"
                    >
                      {t("projectTemplates.capture")}
                    </Button>
                  ) : null}
                  <Button
                    disabled={isLifecyclePending}
                    onClick={() => onArchive(template.key)}
                    variant="destructive"
                  >
                    <Archive />
                    {t("projectTemplates.archive")}
                  </Button>
                </>
              )}
            </div>
          ) : null}
        </div>
        {template.description ? (
          <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
            {template.description}
          </p>
        ) : null}
        {selected ? (
          <>
            <dl className="grid grid-cols-2 gap-px border bg-border sm:grid-cols-4">
              <CountCell
                label={t("projectTemplates.tasks")}
                value={selected.counts.tasks}
              />
              <CountCell
                label={t("projectTemplates.series")}
                value={selected.counts.series}
              />
              <CountCell
                label={t("projectTemplates.configs")}
                value={selected.counts.configs}
              />
              <CountCell
                label={t("projectTemplates.automationsDisabled")}
                value={selected.counts.automations}
              />
            </dl>
            <div className="grid gap-3 text-xs sm:grid-cols-2">
              <div className="border p-3">
                <div className="text-muted-foreground">
                  {t("projectTemplates.source")}
                </div>
                <div className="mt-1 font-mono">
                  {selected.source_project_id}
                </div>
              </div>
              <div className="border p-3">
                <div className="text-muted-foreground">
                  {t("projectTemplates.snapshot")}
                </div>
                <div className="mt-1 flex items-center gap-2 tabular-nums">
                  v{selected.version}
                  <span>·</span>
                  {formatTimestamp(selected.created_at, i18n.language)}
                </div>
              </div>
            </div>
          </>
        ) : null}
      </section>

      <section className="p-5 sm:p-6">
        <div className="mb-3 flex items-center gap-2">
          <FileClock className="size-4 text-muted-foreground" />
          <h3 className="text-sm font-medium">
            {t("projectTemplates.versions")}
          </h3>
          <span className="text-xs text-muted-foreground">
            {t("projectTemplates.versionCount", {
              count: detail.versions.length,
            })}
          </span>
        </div>
        <ol
          className="divide-y border"
          aria-label={t("projectTemplates.versionListLabel")}
        >
          {detail.versions.map((version) => {
            const isCurrent = version.id === currentID
            const isSelected = version.id === selected?.id
            return (
              <li key={version.id}>
                <button
                  aria-current={isSelected ? "true" : undefined}
                  className={cn(
                    "grid w-full grid-cols-[3rem_minmax(0,1fr)] items-center gap-3 px-3 py-2 text-left hover:bg-muted/50 sm:grid-cols-[3rem_8rem_minmax(0,1fr)]",
                    isSelected && "bg-muted/60"
                  )}
                  onClick={() =>
                    onSelectVersion(isCurrent ? undefined : version.id)
                  }
                  type="button"
                >
                  <span className="text-sm font-medium tabular-nums">
                    v{version.version}
                  </span>
                  <span className="hidden text-xs text-muted-foreground sm:block">
                    {formatTimestamp(version.created_at, i18n.language)}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {versionCounts(version.counts)}
                    {isCurrent ? ` · ${t("projectTemplates.current")}` : ""}
                  </span>
                </button>
              </li>
            )
          })}
        </ol>
      </section>
    </div>
  )
}

function CountCell({ label, value }: { label: string; value: number }) {
  return (
    <div className="bg-background p-3">
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="mt-1 text-lg font-semibold tabular-nums">
        <span className="sr-only">{label} </span>
        {value}
      </dd>
      <div aria-hidden className="text-xs text-muted-foreground">
        {label} {value}
      </div>
    </div>
  )
}

function Pagination({
  limit,
  offset,
  onOffsetChange,
  total,
}: {
  limit: number
  offset: number
  onOffsetChange: (offset: number) => void
  total: number
}) {
  const { t } = useTranslation()
  const from = total === 0 ? 0 : offset + 1
  const to = Math.min(offset + limit, total)
  return (
    <div className="flex items-center justify-between border-t p-2">
      <span className="text-[11px] text-muted-foreground tabular-nums">
        {t("projectTemplates.range", { from, to, total })}
      </span>
      <div className="flex gap-1">
        <Button
          aria-label={t("projectTemplates.previousPage")}
          disabled={offset === 0}
          onClick={() => onOffsetChange(Math.max(0, offset - limit))}
          size="icon-xs"
          variant="ghost"
        >
          <ChevronLeft />
        </Button>
        <Button
          aria-label={t("projectTemplates.nextPage")}
          disabled={offset + limit >= total}
          onClick={() => onOffsetChange(offset + limit)}
          size="icon-xs"
          variant="ghost"
        >
          <ChevronRight />
        </Button>
      </div>
    </div>
  )
}

function formatTimestamp(value: number, language: string) {
  return new Intl.DateTimeFormat(language, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value * 1000))
}

function versionCounts(counts: ComponentCounts) {
  return `${counts.tasks}/${counts.series}/${counts.configs}/${counts.automations}`
}
