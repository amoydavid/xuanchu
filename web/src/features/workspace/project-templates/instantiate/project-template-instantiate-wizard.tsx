import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Check,
  KeyRound,
  Loader2,
  Users,
} from "lucide-react"
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Textarea } from "@/components/ui/textarea"
import { getHome } from "@/features/workspace/home/home-api"
import {
  listWorkspaceMembers,
  type WorkspaceMemberRow,
} from "@/features/workspace/members/members-api"
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"
import { ApiError } from "@/lib/api"
import { cn } from "@/lib/utils"

import {
  getProjectTemplate,
  instantiateProjectTemplate,
  invalidateProjectTemplateMutation,
  listProjectTemplates,
  previewProjectTemplateInstantiation,
  projectTemplateDetailQueryKey,
  projectTemplateListQueryKey,
  type AssigneeIssue,
  type InstantiateInput,
  type InstantiatePreview,
  type ProjectTemplateIssue,
} from "../api/project-template-api"
import { canInstantiateProjectTemplate } from "../project-template-instantiation-permissions"
import { isCurrentPreviewRevision } from "./project-template-instantiate-revision"

export type ProjectTemplateInstantiateSelection = {
  templateRef: string
  snapshotID: string
  snapshotHash: string
}

type ProjectTemplateInstantiateWizardProps = {
  canInstantiate?: boolean
  canManage?: boolean
  initialSelection?: ProjectTemplateInstantiateSelection
  onOpenChange: (open: boolean) => void
  open: boolean
  writeScopes?: string[] | null
  workspaceSlug: string
}

const steps = ["选择模板", "项目信息", "处理问题", "确认创建"]
const PROJECT_SLUG_PATTERN = /^[a-z][a-z0-9]{2,9}$/
const REMOVE_ASSIGNEE = "__remove__"
const TEMPLATE_PAGE_SIZE = 20

export function ProjectTemplateInstantiateWizard(
  props: ProjectTemplateInstantiateWizardProps
) {
  if (!props.open || !props.canInstantiate) return null
  return <ProjectTemplateInstantiateWizardSession {...props} />
}

function ProjectTemplateInstantiateWizardSession({
  canManage = false,
  initialSelection,
  onOpenChange,
  open,
  writeScopes,
  workspaceSlug,
}: ProjectTemplateInstantiateWizardProps) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const feedback = useEditFeedback()
  const [step, setStep] = useState(initialSelection ? 1 : 0)
  const [selection, setSelection] = useState<
    ProjectTemplateInstantiateSelection | undefined
  >(initialSelection)
  const [slug, setSlug] = useState("")
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [startDate, setStartDate] = useState("")
  const [secretInputs, setSecretInputs] = useState<Record<string, string>>({})
  const [assigneeReplacements, setAssigneeReplacements] = useState<
    Record<string, string | null>
  >({})
  const [preview, setPreview] = useState<InstantiatePreview>()
  const [previewRevision, setPreviewRevision] = useState<number>()
  const [previewDirty, setPreviewDirty] = useState(false)
  const [previewPending, setPreviewPending] = useState(false)
  const [submitPending, setSubmitPending] = useState(false)
  const [error, setError] = useState<string>()
  const [searchDraft, setSearchDraft] = useState("")
  const [search, setSearch] = useState("")
  const [templateOffset, setTemplateOffset] = useState(0)
  const [fieldErrors, setFieldErrors] = useState<{
    slug?: string
    name?: string
    startDate?: string
  }>({})
  const initializedDescription = useRef<string | undefined>(undefined)
  const initializedStartDate = useRef(false)
  const formRevision = useRef(0)
  const submitPendingRef = useRef(false)

  const listQuery = useQuery({
    enabled: open,
    queryKey: projectTemplateListQueryKey(
      workspaceSlug,
      "active",
      search,
      TEMPLATE_PAGE_SIZE,
      templateOffset
    ),
    queryFn: () =>
      listProjectTemplates(workspaceSlug, {
        status: "active",
        q: search,
        limit: TEMPLATE_PAGE_SIZE,
        offset: templateOffset,
      }),
  })
  const homeQuery = useQuery({
    enabled: open,
    queryKey: ["home", workspaceSlug, "instantiate-date"],
    queryFn: getHome,
  })
  const detailQuery = useQuery({
    enabled: Boolean(selection),
    queryKey: projectTemplateDetailQueryKey(
      workspaceSlug,
      selection?.templateRef ?? "",
      selection?.snapshotID
    ),
    queryFn: () =>
      getProjectTemplate(
        workspaceSlug,
        selection!.templateRef,
        selection!.snapshotID
      ),
  })
  const membersQuery = useQuery({
    enabled: step >= 2,
    queryKey: ["workspace", "members", workspaceSlug],
    queryFn: () => listWorkspaceMembers(workspaceSlug),
  })

  useEffect(() => {
    if (!selection || !detailQuery.data) return
    const key = `${selection.templateRef}:${selection.snapshotID}`
    if (initializedDescription.current === key) return
    initializedDescription.current = key
    setDescription(detailQuery.data.snapshot?.project.description ?? "")
  }, [detailQuery.data, selection])

  useEffect(() => {
    if (initializedStartDate.current || !homeQuery.data?.today) return
    initializedStartDate.current = true
    setStartDate(homeQuery.data.today)
  }, [homeQuery.data?.today])

  const unresolvedBlocking = useMemo(
    () =>
      preview?.issues.filter((issue) => issue.severity === "blocking") ?? [],
    [preview]
  )

  function invalidatePreview() {
    formRevision.current += 1
    setPreviewRevision(undefined)
    setPreviewDirty(true)
  }

  function markPreviewDirty() {
    invalidatePreview()
    setError(undefined)
  }

  function selectTemplate(
    templateRef: string,
    snapshotID: string,
    hash: string
  ) {
    formRevision.current += 1
    setSelection({ templateRef, snapshotID, snapshotHash: hash })
    initializedDescription.current = undefined
    setDescription("")
    setSecretInputs({})
    setAssigneeReplacements({})
    setPreview(undefined)
    setPreviewRevision(undefined)
    setPreviewDirty(false)
    setError(undefined)
  }

  function validateProjectFields() {
    const next: typeof fieldErrors = {}
    if (!PROJECT_SLUG_PATTERN.test(slug.trim())) {
      next.slug = "Slug 必须为 3-10 位小写字母或数字，并以字母开头"
    }
    if (!name.trim()) next.name = "项目名称不能为空"
    if (!/^\d{4}-\d{2}-\d{2}$/.test(startDate)) {
      next.startDate = "请选择有效的开始日期"
    }
    setFieldErrors(next)
    return Object.keys(next).length === 0
  }

  function instantiateInput(pinned = selection): InstantiateInput | undefined {
    if (!pinned) return undefined
    const secrets = Object.fromEntries(
      Object.entries(secretInputs).filter(([, value]) => value !== "")
    )
    return {
      snapshot_id: pinned.snapshotID,
      expected_snapshot_hash: pinned.snapshotHash,
      project_slug: slug.trim(),
      project_name: name.trim(),
      description: description.trim(),
      start_date: startDate,
      ...(Object.keys(secrets).length ? { secret_inputs: secrets } : {}),
      ...(Object.keys(assigneeReplacements).length
        ? { assignee_replacements: assigneeReplacements }
        : {}),
    }
  }

  async function handleSnapshotDrift() {
    setSelection(undefined)
    setPreview(undefined)
    setPreviewDirty(false)
    setSecretInputs({})
    setAssigneeReplacements({})
    setStep(0)
    setError("模板版本已变化。请重新选择版本；系统不会自动切换到新版本。")
    await queryClient.invalidateQueries({
      queryKey: ["project-templates", workspaceSlug],
    })
  }

  async function runPreview() {
    if (
      !selection ||
      !validateProjectFields() ||
      previewPending ||
      !homeQuery.data?.today
    ) {
      return false
    }
    const input = instantiateInput()
    if (!input) return false
    const revision = formRevision.current
    setPreviewPending(true)
    setError(undefined)
    try {
      const next = await previewProjectTemplateInstantiation(
        workspaceSlug,
        selection.templateRef,
        input
      )
      if (!isCurrentPreviewRevision(revision, formRevision.current)) return false
      if (
        next.snapshot.id !== selection.snapshotID ||
        next.snapshot.hash !== selection.snapshotHash
      ) {
        await handleSnapshotDrift()
        return false
      }
      const pinned = {
        templateRef: selection.templateRef,
        snapshotID: next.snapshot.id,
        snapshotHash: next.snapshot.hash,
      }
      setSelection(pinned)
      setPreview(next)
      setPreviewRevision(revision)
      setPreviewDirty(false)
      setStep(2)
      return true
    } catch (caught) {
      if (!isCurrentPreviewRevision(revision, formRevision.current)) {
        return false
      }
      if (
        caught instanceof ApiError &&
        caught.code === "project_template_snapshot_hash_mismatch"
      ) {
        await handleSnapshotDrift()
      } else {
        setError(errorLabel(caught, "生成预览失败，请检查输入后重试。"))
      }
      return false
    } finally {
      setPreviewPending(false)
    }
  }

  async function submit() {
    if (
      submitPendingRef.current ||
      !selection ||
      !preview ||
      previewRevision !== formRevision.current ||
      previewDirty ||
      unresolvedBlocking.length > 0
    ) {
      return
    }
    const pinned = {
      templateRef: selection.templateRef,
      snapshotID: preview.snapshot.id,
      snapshotHash: preview.snapshot.hash,
    }
    const input = instantiateInput(pinned)
    if (!input) return
    submitPendingRef.current = true
    setSubmitPending(true)
    setError(undefined)
    try {
      const result = await instantiateProjectTemplate(
        workspaceSlug,
        pinned.templateRef,
        input
      )
      setSecretInputs({})
      await invalidateProjectTemplateMutation(queryClient, workspaceSlug, {
        kind: "instantiate",
        ref: pinned.templateRef,
      })
      feedback.success(successCounts(result.counts))
      onOpenChange(false)
      void navigate({
        to: "/workspaces/$workspaceSlug/projects/$projectSlug",
        params: { workspaceSlug, projectSlug: result.project.slug },
      })
    } catch (caught) {
      setSecretInputs({})
      invalidatePreview()
      if (
        caught instanceof ApiError &&
        caught.code === "project_template_snapshot_hash_mismatch"
      ) {
        await handleSnapshotDrift()
      } else {
        setPreviewDirty(true)
        setStep(2)
        setError(
          errorLabel(
            caught,
            "创建失败。项目信息已保留，请重新填写机密值后重试。"
          )
        )
      }
    } finally {
      submitPendingRef.current = false
      setSubmitPending(false)
    }
  }

  function handleOpenChange(next: boolean) {
    if (
      !next &&
      (submitPendingRef.current || submitPending || previewPending)
    ) {
      return
    }
    if (!next) setSecretInputs({})
    onOpenChange(next)
  }

  function handleDialogKeyDown(event: ReactKeyboardEvent) {
    if (event.key === "Escape" && (submitPending || previewPending)) {
      event.preventDefault()
      return
    }
    if (
      event.key !== "Enter" ||
      (!event.metaKey && !event.ctrlKey) ||
      submitPending ||
      previewPending
    ) {
      return
    }
    event.preventDefault()
    if (step === 0) {
      if (selection && !detailQuery.isPending && !detailQuery.isError) {
        setStep(1)
      }
      return
    }
    if (step === 1) {
      void runPreview()
      return
    }
    if (step === 2) {
      if (previewDirty || unresolvedBlocking.length > 0) {
        void runPreview()
      } else {
        setStep(3)
      }
      return
    }
    void submit()
  }

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
        <SheetContent
          className="w-full max-w-none sm:w-[min(56rem,calc(100vw-2rem))] sm:max-w-[min(56rem,calc(100vw-2rem))]"
          onEscapeKeyDown={(event) => {
            if (submitPending || previewPending) event.preventDefault()
          }}
          onPointerDownOutside={(event) => {
            if (submitPending || previewPending) event.preventDefault()
          }}
          onKeyDown={handleDialogKeyDown}
          side="right"
        >
          <SheetHeader className="h-auto min-h-16 items-start justify-between gap-4 py-3">
            <div>
              <SheetTitle>从模板创建项目</SheetTitle>
              <SheetDescription className="mt-1">
                固定一个不可变模板版本，预检后一次性创建完整项目。
              </SheetDescription>
            </div>
            <Button
              disabled={submitPending || previewPending}
              onClick={() => handleOpenChange(false)}
              size="sm"
              type="button"
              variant="ghost"
            >
              关闭
            </Button>
          </SheetHeader>

          <nav className="border-b px-4 py-3" aria-label="创建步骤">
            <ol className="grid grid-cols-4 gap-1">
              {steps.map((label, index) => (
                <li
                  aria-current={step === index ? "step" : undefined}
                  className={cn(
                    "border-t-2 pt-2 text-[11px] text-muted-foreground",
                    step === index &&
                      "border-foreground font-medium text-foreground",
                    step > index && "border-emerald-600 text-foreground"
                  )}
                  key={label}
                >
                  <span className="mr-1 tabular-nums">0{index + 1}</span>
                  <span className="hidden sm:inline">{label}</span>
                </li>
              ))}
            </ol>
          </nav>

          <div className="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
            {error ? (
              <Alert className="mb-4" variant="destructive">
                <AlertTriangle />
                <AlertTitle>操作未完成</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}

            {step === 0 ? (
              <TemplateStep
                canManage={canManage}
                isError={listQuery.isError}
                isPending={listQuery.isPending}
                items={listQuery.data?.items ?? []}
                offset={templateOffset}
                onNextPage={() =>
                  setTemplateOffset((current) => current + TEMPLATE_PAGE_SIZE)
                }
                onPreviousPage={() =>
                  setTemplateOffset((current) =>
                    Math.max(0, current - TEMPLATE_PAGE_SIZE)
                  )
                }
                onRetry={() => listQuery.refetch()}
                onSearch={() => {
                  setTemplateOffset(0)
                  setSearch(searchDraft.trim())
                }}
                onSearchDraftChange={setSearchDraft}
                onSelect={selectTemplate}
                searchDraft={searchDraft}
                selection={selection}
                total={listQuery.data?.total ?? 0}
                writeScopes={writeScopes}
              />
            ) : null}
            {step === 1 ? (
              <ProjectStep
                description={description}
                disabled={previewPending}
                detailError={detailQuery.isError}
                detailPending={detailQuery.isPending}
                fieldErrors={fieldErrors}
                name={name}
                onDescriptionChange={(value) => {
                  setDescription(value)
                  markPreviewDirty()
                }}
                onNameChange={(value) => {
                  setName(value)
                  markPreviewDirty()
                }}
                onDetailRetry={() => detailQuery.refetch()}
                onSlugChange={(value) => {
                  setSlug(value)
                  markPreviewDirty()
                }}
                onStartDateChange={(value) => {
                  initializedStartDate.current = true
                  setStartDate(value)
                  markPreviewDirty()
                }}
                selection={selection}
                slug={slug}
                startDate={startDate}
                workspaceDateError={homeQuery.isError}
                workspaceDatePending={homeQuery.isPending}
              />
            ) : null}
            {step === 2 && preview ? (
              <ResolutionStep
                assigneeReplacements={assigneeReplacements}
                disabled={previewPending}
                members={membersQuery.data ?? []}
                membersError={membersQuery.isError}
                onAssigneeChange={(userID, value) => {
                  setAssigneeReplacements((current) => ({
                    ...current,
                    [userID]: value === REMOVE_ASSIGNEE ? null : value,
                  }))
                  markPreviewDirty()
                }}
                onSecretChange={(key, value) => {
                  setSecretInputs((current) => ({ ...current, [key]: value }))
                  markPreviewDirty()
                }}
                preview={preview}
                previewDirty={previewDirty}
                secretInputs={secretInputs}
              />
            ) : null}
            {step === 3 && preview ? <ConfirmStep preview={preview} /> : null}
          </div>

          <footer className="flex shrink-0 flex-col-reverse gap-2 border-t bg-muted/30 p-4 sm:flex-row sm:items-center sm:justify-between">
            <Button
              disabled={
                submitPending ||
                previewPending ||
                (step === 0 && !selection)
              }
              onClick={() => {
                setError(undefined)
                setStep((current) => Math.max(0, current - 1))
              }}
              type="button"
              variant="outline"
            >
              <ArrowLeft />
              返回
            </Button>
            {step === 0 ? (
              <Button
                disabled={
                  !selection || detailQuery.isPending || detailQuery.isError
                }
                onClick={() => setStep(1)}
                type="button"
              >
                下一步：项目信息
                <ArrowRight />
              </Button>
            ) : null}
            {step === 1 ? (
              <Button
                disabled={
                  previewPending ||
                  detailQuery.isPending ||
                  detailQuery.isError ||
                  homeQuery.isPending ||
                  homeQuery.isError
                }
                onClick={runPreview}
                type="button"
              >
                {previewPending ? <Loader2 className="animate-spin" /> : null}
                生成预览
              </Button>
            ) : null}
            {step === 2 ? (
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                {previewDirty || unresolvedBlocking.length > 0 ? (
                  <Button
                    disabled={previewPending}
                    onClick={runPreview}
                    type="button"
                    variant="outline"
                  >
                    {previewPending ? (
                      <Loader2 className="animate-spin" />
                    ) : null}
                    重新预览
                  </Button>
                ) : null}
                <Button
                  disabled={previewDirty || unresolvedBlocking.length > 0}
                  onClick={() => setStep(3)}
                  type="button"
                >
                  下一步：确认
                  <ArrowRight />
                </Button>
              </div>
            ) : null}
            {step === 3 ? (
              <Button disabled={submitPending} onClick={submit} type="button">
                {submitPending ? (
                  <Loader2 className="animate-spin" />
                ) : (
                  <Check />
                )}
                {submitPending ? "正在创建" : "创建项目"}
              </Button>
            ) : null}
          </footer>
        </SheetContent>
    </Sheet>
  )
}

function TemplateStep({
  canManage,
  isError,
  isPending,
  items,
  offset,
  onNextPage,
  onPreviousPage,
  onRetry,
  onSearch,
  onSearchDraftChange,
  onSelect,
  searchDraft,
  selection,
  total,
  writeScopes,
}: {
  canManage: boolean
  isError: boolean
  isPending: boolean
  items: Awaited<ReturnType<typeof listProjectTemplates>>["items"]
  offset: number
  onNextPage: () => void
  onPreviousPage: () => void
  onRetry: () => void
  onSearch: () => void
  onSearchDraftChange: (value: string) => void
  onSelect: (ref: string, snapshotID: string, hash: string) => void
  searchDraft: string
  selection?: ProjectTemplateInstantiateSelection
  total: number
  writeScopes?: string[] | null
}) {
  if (isPending) return <p role="status">正在加载可用模板…</p>
  if (isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>加载模板失败</AlertTitle>
        <AlertDescription>
          <Button className="mt-2" onClick={onRetry} variant="outline">
            重试
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <section
      aria-labelledby="instantiate-template-heading"
      className="space-y-3"
    >
      <div>
        <h2 className="font-medium" id="instantiate-template-heading">
          选择 active 模板的当前版本
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">
          这里不会展示或自动切换历史版本；历史版本请从模板库进入。
        </p>
      </div>
      <div className="flex gap-2">
        <Input
          aria-label="搜索模板"
          onChange={(event) => onSearchDraftChange(event.target.value)}
          placeholder="搜索模板"
          value={searchDraft}
        />
        <Button aria-label="搜索" onClick={onSearch} type="button" variant="outline">
          搜索
        </Button>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        {items.map((item) => {
          const snapshot = item.current_snapshot
          if (!snapshot) return null
          const canInstantiate = canInstantiateProjectTemplate(
            canManage,
            writeScopes,
            snapshot.counts
          )
          const active =
            selection?.templateRef === item.key &&
            selection.snapshotID === snapshot.id
          return (
            <button
              aria-pressed={active}
              className={cn(
                "border p-4 text-left transition-colors hover:bg-muted/50",
                active && "border-foreground bg-muted/40",
                !canInstantiate && "cursor-not-allowed opacity-60"
              )}
              disabled={!canInstantiate}
              key={item.id}
              onClick={() => onSelect(item.key, snapshot.id, snapshot.hash)}
              type="button"
            >
              <span className="flex items-start justify-between gap-2">
                <span className="font-medium">{item.name}</span>
                <Badge variant="outline">v{snapshot.version} 当前</Badge>
              </span>
              <span className="mt-1 block font-mono text-xs text-muted-foreground">
                {item.key}
              </span>
              <span className="mt-3 block text-xs text-muted-foreground">
                {snapshot.counts.tasks} 任务 · {snapshot.counts.series} 循环 ·{" "}
                {snapshot.counts.configs} 配置 · {snapshot.counts.automations}{" "}
                自动化
              </span>
              {!canInstantiate ? (
                <span className="mt-2 block text-xs text-muted-foreground">
                  当前令牌缺少创建此模板所需的写权限。
                </span>
              ) : null}
            </button>
          )
        })}
      </div>
      {items.length === 0 ? (
        <div className="border border-dashed p-8 text-center text-sm text-muted-foreground">
          当前没有可用于创建项目的 active 模板。
        </div>
      ) : null}
      <div className="flex justify-between border-t pt-3">
        <Button
          disabled={offset === 0}
          onClick={onPreviousPage}
          type="button"
          variant="outline"
        >
          上一页
        </Button>
        <Button
          disabled={offset + TEMPLATE_PAGE_SIZE >= total}
          onClick={onNextPage}
          type="button"
          variant="outline"
        >
          下一页
        </Button>
      </div>
    </section>
  )
}

function ProjectStep({
  description,
  disabled,
  detailError,
  detailPending,
  fieldErrors,
  name,
  onDescriptionChange,
  onDetailRetry,
  onNameChange,
  onSlugChange,
  onStartDateChange,
  selection,
  slug,
  startDate,
  workspaceDateError,
  workspaceDatePending,
}: {
  description: string
  disabled: boolean
  detailError: boolean
  detailPending: boolean
  fieldErrors: { slug?: string; name?: string; startDate?: string }
  name: string
  onDescriptionChange: (value: string) => void
  onDetailRetry: () => void
  onNameChange: (value: string) => void
  onSlugChange: (value: string) => void
  onStartDateChange: (value: string) => void
  selection?: ProjectTemplateInstantiateSelection
  slug: string
  startDate: string
  workspaceDateError: boolean
  workspaceDatePending: boolean
}) {
  return (
    <section
      aria-labelledby="instantiate-project-heading"
      className="space-y-5"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b pb-3">
        <div>
          <h2 className="font-medium" id="instantiate-project-heading">
            设置新项目
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            模板 {selection?.templateRef} · 固定 Snapshot{" "}
            {selection?.snapshotID}
          </p>
        </div>
        <Badge variant="outline">不可变版本</Badge>
      </div>
      {detailError ? (
        <Alert variant="destructive">
          <AlertTriangle />
          <AlertTitle>读取模板版本失败</AlertTitle>
          <AlertDescription>
            项目说明尚未加载，不能生成预览。
            <Button
              className="mt-2 block"
              onClick={onDetailRetry}
              type="button"
              variant="outline"
            >
              重试
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="项目 Slug" error={fieldErrors.slug}>
          <Input
            aria-invalid={Boolean(fieldErrors.slug)}
            autoComplete="off"
            disabled={disabled}
            id="instantiate-project-slug"
            onChange={(event) => onSlugChange(event.target.value)}
            placeholder="launch26"
            value={slug}
          />
        </Field>
        <Field label="项目名称" error={fieldErrors.name}>
          <Input
            aria-invalid={Boolean(fieldErrors.name)}
            disabled={disabled}
            id="instantiate-project-name"
            onChange={(event) => onNameChange(event.target.value)}
            value={name}
          />
        </Field>
      </div>
      <Field label="开始日期" error={fieldErrors.startDate}>
        <Input
            aria-invalid={Boolean(fieldErrors.startDate)}
            className="sm:max-w-xs"
            disabled={disabled || workspaceDatePending || workspaceDateError}
          id="instantiate-project-start-date"
          onChange={(event) => onStartDateChange(event.target.value)}
          type="date"
          value={startDate}
        />
        <p className="mt-1 text-xs text-muted-foreground">
          {workspaceDateError
            ? "无法读取工作区日期，不能生成预览。"
            : workspaceDatePending
              ? "正在读取工作区日期…"
              : `模板中的相对日期会以 ${startDate || "所选日期"} 为基准恢复。`}
        </p>
      </Field>
      <Field label="项目说明">
        <Textarea
          disabled={detailPending || disabled}
          id="instantiate-project-description"
          onChange={(event) => onDescriptionChange(event.target.value)}
          rows={5}
          value={description}
        />
        <p className="mt-1 text-xs text-muted-foreground">
          已预填 Snapshot 中的项目说明，可在创建前覆盖。
        </p>
      </Field>
    </section>
  )
}

function Field({
  children,
  error,
  label,
}: {
  children: React.ReactNode
  error?: string
  label: string
}) {
  const id =
    label === "项目 Slug"
      ? "instantiate-project-slug"
      : label === "项目名称"
        ? "instantiate-project-name"
        : label === "开始日期"
          ? "instantiate-project-start-date"
          : "instantiate-project-description"
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}

function ResolutionStep({
  assigneeReplacements,
  disabled,
  members,
  membersError,
  onAssigneeChange,
  onSecretChange,
  preview,
  previewDirty,
  secretInputs,
}: {
  assigneeReplacements: Record<string, string | null>
  disabled: boolean
  members: WorkspaceMemberRow[]
  membersError: boolean
  onAssigneeChange: (userID: string, value: string) => void
  onSecretChange: (key: string, value: string) => void
  preview: InstantiatePreview
  previewDirty: boolean
  secretInputs: Record<string, string>
}) {
  const otherIssues = preview.issues.filter(
    (issue) =>
      issue.code !== "project_template_secret_required" &&
      issue.code !== "project_template_member_unavailable"
  )
  return (
    <section
      aria-labelledby="instantiate-resolution-heading"
      className="space-y-5"
    >
      <div>
        <h2 className="font-medium" id="instantiate-resolution-heading">
          补齐创建条件
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">
          Preview 已固定 v{preview.snapshot.version}（{preview.snapshot.id}
          ）。任何修改都需重新预览。
        </p>
      </div>
      {previewDirty ? (
        <Alert>
          <AlertTriangle />
          <AlertTitle>预览已过期</AlertTitle>
          <AlertDescription>
            请重新预览，确认这些输入仍可安全创建。
          </AlertDescription>
        </Alert>
      ) : null}

      <section
        className="space-y-3 border p-4"
        aria-labelledby="secret-heading"
      >
        <div className="flex items-center gap-2">
          <KeyRound className="size-4" />
          <h3 className="font-medium" id="secret-heading">
            机密值来源
          </h3>
        </div>
        {preview.secret_resolutions.length ? (
          <div className="space-y-3">
            {preview.secret_resolutions.map((resolution) => {
              const showInput =
                resolution.resolved_from === "missing" ||
                secretInputs[resolution.key] !== undefined ||
                (previewDirty && resolution.resolved_from === "input")
              const inputID = `instantiate-secret-${resolution.key}`
              return (
                <div
                  className="grid gap-2 sm:grid-cols-[1fr_11rem]"
                  key={resolution.key}
                >
                  <div>
                    {showInput ? (
                      <Label htmlFor={inputID}>{resolution.key}</Label>
                    ) : (
                      <div className="text-sm font-medium">
                        {resolution.key}
                      </div>
                    )}
                    <div className="mt-1 text-xs text-muted-foreground">
                      {previewDirty &&
                      secretInputs[resolution.key] !== undefined
                        ? "待重新检查"
                        : secretSourceLabel(resolution.resolved_from)}
                    </div>
                  </div>
                  {showInput ? (
                    <Input
                      autoComplete="new-password"
                      disabled={disabled}
                      id={inputID}
                      onChange={(event) =>
                        onSecretChange(resolution.key, event.target.value)
                      }
                      placeholder="输入后仅用于本次创建"
                      type="password"
                      value={secretInputs[resolution.key] ?? ""}
                    />
                  ) : (
                    <span className="self-center text-xs text-muted-foreground">
                      无需输入
                    </span>
                  )}
                </div>
              )
            })}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            此版本不需要机密输入。
          </p>
        )}
      </section>

      <section
        className="space-y-3 border p-4"
        aria-labelledby="member-heading"
      >
        <div className="flex items-center gap-2">
          <Users className="size-4" />
          <h3 className="font-medium" id="member-heading">
            成员映射
          </h3>
        </div>
        {membersError ? (
          <p className="text-xs text-destructive">
            无法加载替换成员；仍可选择移除原指派。
          </p>
        ) : null}
        {preview.assignee_issues.length ? (
          <div className="space-y-3">
            {preview.assignee_issues.map((issue) => (
              <AssigneeResolution
                disabled={disabled}
                issue={issue}
                key={issue.user.id}
                members={members}
                onChange={(value) => onAssigneeChange(issue.user.id, value)}
                value={replacementValue(assigneeReplacements, issue.user.id)}
              />
            ))}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            所有模板成员当前均可用。
          </p>
        )}
      </section>

      <IssueList issues={otherIssues} />
      <IssueList issues={preview.warnings} warning />
    </section>
  )
}

function AssigneeResolution({
  disabled,
  issue,
  members,
  onChange,
  value,
}: {
  disabled: boolean
  issue: AssigneeIssue
  members: WorkspaceMemberRow[]
  onChange: (value: string) => void
  value: string
}) {
  const label = issue.user.display_name || issue.user.name || issue.user.id
  return (
    <div className="grid gap-2 sm:grid-cols-[1fr_15rem] sm:items-center">
      <div>
        <div className="text-sm font-medium">{label}</div>
        <div className="text-xs text-muted-foreground">
          影响 {issue.affected_refs.join("、")}
        </div>
        {issue.resolution === "removed" ? (
          <div className="mt-1 text-xs text-emerald-700">已移除指派</div>
        ) : issue.resolution === "replaced" ? (
          <div className="mt-1 text-xs text-emerald-700">已替换成员</div>
        ) : null}
      </div>
      <select
        aria-label={`处理${label}`}
        className="h-9 border bg-background px-3 text-sm"
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        value={value}
      >
        <option disabled value="">
          选择处理方式
        </option>
        <option value={REMOVE_ASSIGNEE}>移除指派</option>
        {members
          .filter((member) => member.id !== issue.user.id)
          .map((member) => (
            <option key={member.id} value={member.id}>
              替换为 {member.display_name || member.name}
            </option>
          ))}
      </select>
    </div>
  )
}

function ConfirmStep({ preview }: { preview: InstantiatePreview }) {
  return (
    <section
      aria-labelledby="instantiate-confirm-heading"
      className="space-y-5"
    >
      <div>
        <h2 className="font-medium" id="instantiate-confirm-heading">
          确认创建 {preview.project.name}
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">
          {preview.project.slug} · {preview.project.start_date} · Snapshot v
          {preview.snapshot.version}
        </p>
      </div>
      <dl className="grid grid-cols-2 gap-px border bg-border sm:grid-cols-4">
        {[
          ["任务", preview.counts.tasks],
          ["循环任务", preview.counts.series],
          ["配置", preview.counts.configs],
          ["自动化", preview.counts.automations],
        ].map(([label, value]) => (
          <div className="bg-background p-4" key={label}>
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="mt-1 text-xl font-semibold tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
      <Alert>
        <AlertTitle>创建后的安全边界</AlertTitle>
        <AlertDescription>
          自动化创建后保持停用，需要在新项目中检查并手动启用。附件不会复制。
        </AlertDescription>
      </Alert>
      <p className="text-sm text-muted-foreground">
        所有内容会在一次事务中创建。任一项失败都不会留下半成品项目。
      </p>
    </section>
  )
}

function IssueList({
  issues,
  warning = false,
}: {
  issues: ProjectTemplateIssue[]
  warning?: boolean
}) {
  if (!issues.length) return null
  return (
    <Alert variant={warning ? "default" : "destructive"}>
      <AlertTriangle />
      <AlertTitle>{warning ? "注意事项" : "仍有阻断问题"}</AlertTitle>
      <AlertDescription>
        <ul className="list-disc space-y-1 pl-4">
          {issues.map((issue, index) => (
            <li key={`${issue.code}:${issue.source_ref ?? ""}:${index}`}>
              {issue.message || issue.code}
            </li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  )
}

function replacementValue(
  replacements: Record<string, string | null>,
  userID: string
) {
  if (!Object.prototype.hasOwnProperty.call(replacements, userID)) return ""
  return replacements[userID] === null
    ? REMOVE_ASSIGNEE
    : (replacements[userID] ?? "")
}

function secretSourceLabel(source: string) {
  switch (source) {
    case "input":
      return "本次输入"
    case "workspace":
      return "继承 workspace 值"
    case "default":
      return "使用定义默认值"
    default:
      return "当前无有效值"
  }
}

function successCounts(counts: InstantiatePreview["counts"]) {
  return `已创建 ${counts.tasks} 个任务、${counts.series} 个循环任务、${counts.configs} 项配置和 ${counts.automations} 条停用自动化`
}

function errorLabel(error: unknown, fallback: string) {
  if (!(error instanceof ApiError)) return fallback
  const labels: Record<string, string> = {
    project_slug_exists: "项目 Slug 已存在，请修改后重新预览。",
    project_template_member_unavailable: "成员状态已变化，请重新预览。",
    project_template_secret_required: "机密值不可用，请重新输入。",
  }
  return labels[error.code] ?? `${fallback}（${error.code}）`
}
