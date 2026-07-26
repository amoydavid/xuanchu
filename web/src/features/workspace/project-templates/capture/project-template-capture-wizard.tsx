import { useQueryClient } from "@tanstack/react-query"
import {
  AlertTriangle,
  Check,
  ChevronLeft,
  ChevronRight,
  Loader2,
} from "lucide-react"
import {
  useCallback,
  useEffect,
  forwardRef,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
} from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api"

import {
  appendProjectTemplateSnapshot,
  createProjectTemplate,
  invalidateProjectTemplateMutation,
  previewProjectTemplateCapture,
  previewProjectTemplateSnapshotCapture,
  resolveProjectTemplateCandidateSelection,
  type CandidateKind,
  type CaptureInput,
  type CaptureConfigPolicy,
  type CaptureIssue,
  type CapturePreview,
  type ProjectTemplateConfig,
  type CaptureResolution,
} from "../api/project-template-api"
import {
  CandidatePicker,
  type CandidateSelectionStore,
  type CandidateSummary,
} from "./candidate-picker"
import { SelectedItemsDrawer, SelectedItemsSheet } from "./selected-items-sheet"

type SourceProject = { id: string; slug: string; name: string }

type ProjectTemplateCaptureWizardProps = {
  mode: "create" | "append"
  onOpenChange: (open: boolean) => void
  onSaved?: () => void
  open: boolean
  sourceProject: SourceProject
  templateRef?: string
  workspaceSlug: string
}

const kinds: CandidateKind[] = ["task", "series", "config", "automation"]
const kindLabels: Record<CandidateKind, string> = {
  task: "任务",
  series: "循环任务",
  config: "配置",
  automation: "自动化",
}
const stepLabels = ["模板信息", "选择内容", "检查冲突", "确认保存"]
const selectionLimits: Record<CandidateKind, number> = {
  task: 1000,
  series: 200,
  config: 500,
  automation: 200,
}

export function ProjectTemplateCaptureWizard(
  props: ProjectTemplateCaptureWizardProps
) {
  if (!props.open) return null
  return <ProjectTemplateCaptureWizardSession {...props} />
}

function ProjectTemplateCaptureWizardSession({
  mode,
  onOpenChange,
  onSaved,
  open,
  sourceProject,
  templateRef,
  workspaceSlug,
}: ProjectTemplateCaptureWizardProps) {
  const queryClient = useQueryClient()
  const [step, setStep] = useState(0)
  const [activeKind, setActiveKind] = useState<CandidateKind>("task")
  const [selection, setSelection] =
    useState<CandidateSelectionStore>(emptySelection)
  const [selectedOpen, setSelectedOpen] = useState(false)
  const [keyValue, setKeyValue] = useState("")
  const [name, setName] = useState(sourceProject.name)
  const [description, setDescription] = useState("")
  const [anchorDate, setAnchorDate] = useState(today)
  const [preview, setPreview] = useState<CapturePreview>()
  const [requiredConfigKeys, setRequiredConfigKeys] = useState<Set<string>>(
    () => new Set()
  )
  const [configPolicies, setConfigPolicies] = useState<
    Record<string, CaptureConfigPolicy>
  >({})
  const [resolution, setResolution] = useState<CaptureResolution>({})
  const [previewDirty, setPreviewDirty] = useState(false)
  const [pending, setPending] = useState(false)
  const [defaultsPending, setDefaultsPending] = useState(false)
  const [error, setError] = useState<string>()
  const [limitError, setLimitError] = useState<string>()
  const [defaultLimitKinds, setDefaultLimitKinds] = useState<CandidateKind[]>(
    []
  )
  const [refreshKey, setRefreshKey] = useState(0)
  const firstIssueRef = useRef<HTMLDivElement>(null)
  const defaultLoadedRef = useRef(false)
  const pendingRef = useRef(false)
  const defaultsPendingRef = useRef(false)
  const selectionRef = useRef(selection)

  const totalSelected = useMemo(
    () => kinds.reduce((total, kind) => total + selection[kind].size, 0),
    [selection]
  )
  const blockingIssues = preview?.blocking_issues ?? []
  const canAdvance =
    step === 0
      ? mode === "append" || Boolean(name.trim())
      : step === 1
        ? true
        : step === 2
          ? Boolean(preview && blockingIssues.length === 0 && !previewDirty)
          : false
  const canSave = Boolean(
    step === 3 &&
    preview &&
    blockingIssues.length === 0 &&
    !previewDirty &&
    !pending &&
    !defaultsPending
  )

  useEffect(() => {
    if (!open || mode !== "create" || defaultLoadedRef.current) return
    defaultLoadedRef.current = true
    defaultsPendingRef.current = true
    setDefaultsPending(true)
    let cancelled = false
    void loadDefaults().catch(() => undefined)

    async function loadDefaults() {
      const requests: Array<{
        kind: CandidateKind
        input: Parameters<typeof resolveProjectTemplateCandidateSelection>[2]
      }> = [
        { kind: "task", input: { kind: "task", task: { status: "pending" } } },
        { kind: "task", input: { kind: "task", task: { status: "waiting" } } },
        {
          kind: "series",
          input: { kind: "series", series: { status: "active" } },
        },
      ]
      const results = await Promise.allSettled(
        requests.map(async (request) => ({
          ...request,
          result: await resolveProjectTemplateCandidateSelection(
            workspaceSlug,
            sourceProject.slug,
            request.input
          ),
        }))
      )
      if (cancelled) return
      const limited = new Set<CandidateKind>()
      const defaults = emptySelection()
      for (const [index, result] of results.entries()) {
        if (result.status === "rejected") {
          if (
            result.reason instanceof ApiError &&
            result.reason.code === "project_template_candidate_limit_exceeded"
          ) {
            // 默认超过上限时整类保持未选择，不能静默截断。
            limited.add(requests[index].kind)
          }
          continue
        }
        if (limited.has(result.value.kind)) continue
        for (const ref of result.value.result.refs) {
          defaults[result.value.kind].set(ref, {
            ref,
            label: ref,
            secondary: "默认选择，打开对应分类查看摘要",
          })
        }
      }
      const next = cloneSelection(selectionRef.current)
      for (const kind of kinds) {
        if (limited.has(kind)) continue
        const merged = new Map(next[kind])
        for (const [ref, summary] of defaults[kind]) merged.set(ref, summary)
        if (merged.size > selectionLimits[kind]) {
          limited.add(kind)
          continue
        }
        next[kind] = merged
      }
      if (!sameSelectionStore(selectionRef.current, next)) {
        replaceSelection(next)
        setResolution((current) => sanitizeResolution(current, next))
        setPreview(undefined)
        setPreviewDirty(false)
      }
      setDefaultLimitKinds([...limited])
      defaultsPendingRef.current = false
      setDefaultsPending(false)
    }
    return () => {
      cancelled = true
      defaultLoadedRef.current = false
      defaultsPendingRef.current = false
      setDefaultsPending(false)
    }
  }, [mode, open, sourceProject.slug, workspaceSlug])

  useEffect(() => {
    if (preview?.blocking_issues.length) {
      firstIssueRef.current?.focus()
    }
  }, [preview])

  const close = useCallback(() => {
    if (!pendingRef.current && !pending) onOpenChange(false)
  }, [onOpenChange, pending])

  const applyPreviewSelection = useCallback((result: CapturePreview) => {
    // 旧服务端不会返回 required_config_keys；保留当前本地选择，避免把旧响应的
    // 不完整 selection 当作新的规范化结果。
    if (!Array.isArray(result.required_config_keys)) return
    const next = cloneSelection(selectionRef.current)
    const requiredKeys = result.required_config_keys
    const configByKey = new Map(
      (result.snapshot?.configs ?? []).map((config) => [config.key, config])
    )
    const apply = (kind: CandidateKind, refs: string[]) => {
      const current = next[kind]
      const updated = new Map<string, CandidateSummary>()
      for (const ref of refs) {
        const existing = current.get(ref)
        const config = kind === "config" ? configByKey.get(ref) : undefined
        updated.set(
          ref,
          existing ?? {
            ref,
            label: config?.key ?? ref,
            secondary:
              kind === "config" && requiredKeys.includes(ref)
                ? "由已选自动化依赖"
                : "由服务端归一化选择",
            secret:
              config?.mode === "secret_copy" || config?.mode === "secret_input",
          }
        )
      }
      next[kind] = updated
    }
    apply("task", result.selection.task_refs)
    apply("series", result.selection.series_refs)
    apply("config", result.selection.config_keys)
    apply("automation", result.selection.automation_rule_ids)
    selectionRef.current = next
    setSelection(next)
    const locked = new Set(requiredKeys)
    setRequiredConfigKeys(locked)
    setConfigPolicies((current) => {
      const normalized = syncConfigPolicies(current, next.config, locked)
      for (const key of locked) {
        if (normalized[key]?.strategy === "prompt") {
          normalized[key] = { key, strategy: "prompt", required: true }
        }
      }
      return normalized
    })
  }, [])

  const runPreview = useCallback(async () => {
    if (pendingRef.current || defaultsPendingRef.current) return
    pendingRef.current = true
    setPending(true)
    setError(undefined)
    try {
      const body = captureInput(
        selection,
        sourceProject.slug,
        anchorDate,
        resolution,
        configPolicies
      )
      const result =
        mode === "create"
          ? await previewProjectTemplateCapture(workspaceSlug, body)
          : await previewProjectTemplateSnapshotCapture(
              workspaceSlug,
              templateRef!,
              body
            )
      applyPreviewSelection(result)
      setPreview(result)
      setPreviewDirty(false)
    } catch (caught) {
      setError(errorMessage(caught, "生成预览失败，请检查选择和权限。"))
    } finally {
      pendingRef.current = false
      setPending(false)
    }
  }, [
    applyPreviewSelection,
    anchorDate,
    mode,
    resolution,
    configPolicies,
    selection,
    sourceProject.slug,
    templateRef,
    workspaceSlug,
  ])

  const save = useCallback(async () => {
    if (
      !canSave ||
      !preview ||
      pendingRef.current ||
      defaultsPendingRef.current
    )
      return
    pendingRef.current = true
    setPending(true)
    setError(undefined)
    const capture: CaptureInput = {
      ...captureInput(
        selection,
        sourceProject.slug,
        anchorDate,
        resolution,
        configPolicies
      ),
      expected_source_hash: preview.source_hash,
    }
    try {
      if (mode === "create") {
        const saved = await createProjectTemplate(workspaceSlug, {
          key: keyValue.trim(),
          name: name.trim(),
          description: description.trim() || undefined,
          capture,
        })
        await invalidateProjectTemplateMutation(queryClient, workspaceSlug, {
          kind: "create",
          ref: saved.template.key,
        })
      } else {
        await appendProjectTemplateSnapshot(
          workspaceSlug,
          templateRef!,
          capture
        )
        await invalidateProjectTemplateMutation(queryClient, workspaceSlug, {
          kind: "append",
          ref: templateRef,
        })
      }
      onSaved?.()
      onOpenChange(false)
    } catch (caught) {
      if (
        caught instanceof ApiError &&
        caught.code === "project_template_source_changed"
      ) {
        setStep(1)
        setPreview(undefined)
        setPreviewDirty(false)
        setResolution({})
        setRefreshKey((value) => value + 1)
        setError("来源项目已变化，请检查选择后重新预览。")
      } else {
        setError(errorMessage(caught, "保存模板失败，请重试。"))
      }
    } finally {
      pendingRef.current = false
      setPending(false)
    }
  }, [
    anchorDate,
    canSave,
    configPolicies,
    description,
    keyValue,
    mode,
    name,
    onOpenChange,
    onSaved,
    preview,
    queryClient,
    resolution,
    selection,
    sourceProject.slug,
    templateRef,
    workspaceSlug,
  ])

  const goNext = useCallback(() => {
    if (pending || !canAdvance) return
    setError(undefined)
    setStep((value) => Math.min(3, value + 1))
  }, [canAdvance, pending])

  useEffect(() => {
    if (!open) return
    function shortcut(event: globalThis.KeyboardEvent) {
      if (!(event.metaKey || event.ctrlKey) || event.key !== "Enter") return
      event.preventDefault()
      if (step === 2 && (!preview || previewDirty)) void runPreview()
      else if (step === 3) void save()
      else goNext()
    }
    document.addEventListener("keydown", shortcut)
    return () => document.removeEventListener("keydown", shortcut)
  }, [goNext, open, preview, previewDirty, runPreview, save, step])

  function replaceSelection(next: CandidateSelectionStore) {
    selectionRef.current = next
    setSelection(next)
  }

  function updateSelection(
    kind: CandidateKind,
    next: Map<string, CandidateSummary>
  ) {
    const nextSelection = { ...selectionRef.current, [kind]: next }
    replaceSelection(nextSelection)
    if (kind === "config") {
      setConfigPolicies((current) =>
        syncConfigPolicies(current, next, requiredConfigKeys)
      )
    }
    if (kind === "automation") setRequiredConfigKeys(new Set())
    setResolution((current) => sanitizeResolution(current, nextSelection))
    setPreview(undefined)
    setPreviewDirty(false)
  }

  function removeSelection(kind: CandidateKind, ref: string) {
    if (kind === "config" && requiredConfigKeys.has(ref)) return
    const next = new Map(selection[kind])
    next.delete(ref)
    updateSelection(kind, next)
  }

  function addResolution(next: CaptureResolution) {
    setResolution((current) =>
      sanitizeResolution(mergeResolution(current, next), selectionRef.current)
    )
    setPreviewDirty(true)
  }

  function changeAnchorDate(next: string) {
    setAnchorDate(next)
    setResolution({})
    setPreview(undefined)
    setPreviewDirty(false)
    setError(undefined)
  }

  function clearAllSelection() {
    const next = emptySelection()
    replaceSelection(next)
    setRequiredConfigKeys(new Set())
    setConfigPolicies({})
    setResolution((current) => sanitizeResolution(current, next))
    setPreview(undefined)
    setPreviewDirty(false)
  }

  function updateSummaries(
    kind: CandidateKind,
    next: Map<string, CandidateSummary>
  ) {
    if (sameSelectionMap(selectionRef.current[kind], next)) return
    replaceSelection({ ...selectionRef.current, [kind]: next })
    if (kind === "config") {
      setConfigPolicies((current) =>
        syncConfigPolicies(current, next, requiredConfigKeys)
      )
    }
  }

  function handleDialogKeyDown(event: ReactKeyboardEvent) {
    if (event.key === "Escape" && (pendingRef.current || pending)) {
      event.preventDefault()
    }
  }

  return (
    <Dialog
      onOpenChange={(next) => {
        if (!next) close()
      }}
      open={open}
    >
      <DialogContent
        className="flex max-h-[calc(100svh-2rem)] w-[min(76rem,calc(100%-2rem))] max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:max-w-none"
        onEscapeKeyDown={(event) => {
          if (pendingRef.current || pending) event.preventDefault()
        }}
        onKeyDown={handleDialogKeyDown}
        showCloseButton={!pending}
      >
        <DialogHeader className="border-b px-5 py-4 pr-12">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <DialogTitle>
                {mode === "create" ? "保存项目模板" : "从项目更新快照"}
              </DialogTitle>
              <DialogDescription className="mt-1">
                {sourceProject.name} · {sourceProject.slug}
              </DialogDescription>
            </div>
            <Badge variant="outline">显式选择 {totalSelected} 项</Badge>
          </div>
          <ol
            className="mt-3 grid grid-cols-4 gap-px border bg-border"
            aria-label="保存步骤"
          >
            {stepLabels.map((label, index) => (
              <li
                aria-current={index === step ? "step" : undefined}
                className={`flex min-w-0 items-center gap-2 bg-background px-3 py-2 text-xs ${
                  index === step
                    ? "font-medium text-foreground"
                    : "text-muted-foreground"
                }`}
                key={label}
              >
                <span className="grid size-5 shrink-0 place-items-center border text-[10px] tabular-nums">
                  {index < step ? <Check /> : index + 1}
                </span>
                <span className="truncate">{label}</span>
              </li>
            ))}
          </ol>
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {error ? (
            <Alert className="m-4 mb-0" variant="destructive">
              <AlertTriangle />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {step === 0 ? (
            <MetadataStep
              anchorDate={anchorDate}
              description={description}
              keyValue={keyValue}
              mode={mode}
              name={name}
              onAnchorDateChange={changeAnchorDate}
              onDescriptionChange={setDescription}
              onKeyChange={setKeyValue}
              onNameChange={setName}
              sourceProject={sourceProject}
            />
          ) : step === 1 ? (
            <div className="grid min-h-[31rem] gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
              <div className="min-w-0">
                {limitError ? (
                  <Alert className="mb-3" variant="destructive">
                    <AlertTriangle />
                    <AlertDescription>{limitError}</AlertDescription>
                  </Alert>
                ) : null}
                {defaultLimitKinds.length ? (
                  <Alert className="mb-3">
                    <AlertTriangle />
                    <AlertTitle>默认选择超过上限</AlertTitle>
                    <AlertDescription>
                      {defaultLimitKinds
                        .map((kind) => kindLabels[kind])
                        .join("、")}
                      未自动选择，请先筛选再选择。
                    </AlertDescription>
                  </Alert>
                ) : null}
                <Tabs
                  onValueChange={(value) =>
                    setActiveKind(value as CandidateKind)
                  }
                  value={activeKind}
                >
                  <div className="flex flex-wrap items-center justify-between gap-2 border-b pb-2">
                    <TabsList variant="line">
                      {kinds.map((kind) => (
                        <TabsTrigger key={kind} value={kind}>
                          {kindLabels[kind]}
                          <span className="tabular-nums">
                            {selection[kind].size}
                          </span>
                        </TabsTrigger>
                      ))}
                    </TabsList>
                    <Button
                      onClick={() => setSelectedOpen(true)}
                      size="sm"
                      variant="outline"
                    >
                      已选 {totalSelected} 项
                    </Button>
                  </div>
                  {kinds.map((kind) => (
                    <TabsContent
                      className="pt-3"
                      forceMount
                      key={kind}
                      value={kind}
                    >
                      <div hidden={activeKind !== kind}>
                        <CandidatePicker
                          kind={kind}
                          lockedRefs={
                            kind === "config" ? requiredConfigKeys : undefined
                          }
                          onChange={(next) => updateSelection(kind, next)}
                          onLimitError={setLimitError}
                          onSummariesChange={(next) =>
                            updateSummaries(kind, next)
                          }
                          refreshKey={refreshKey}
                          selected={selection[kind]}
                          sourceProjectRef={sourceProject.slug}
                          workspaceSlug={workspaceSlug}
                        />
                      </div>
                    </TabsContent>
                  ))}
                </Tabs>
              </div>
              <SelectedItemsDrawer
                configPolicies={configPolicies}
                lockedConfigKeys={requiredConfigKeys}
                onClearAll={clearAllSelection}
                onRemove={removeSelection}
                onConfigPolicyChange={(policy) => {
                  setConfigPolicies((current) => ({
                    ...current,
                    [policy.key]: policy,
                  }))
                  setPreview(undefined)
                  setPreviewDirty(false)
                }}
                selection={selection}
              />
              <SelectedItemsSheet
                configPolicies={configPolicies}
                onClearAll={() => {
                  clearAllSelection()
                  setSelectedOpen(false)
                }}
                onOpenChange={setSelectedOpen}
                onRemove={removeSelection}
                onConfigPolicyChange={(policy) => {
                  setConfigPolicies((current) => ({
                    ...current,
                    [policy.key]: policy,
                  }))
                  setPreview(undefined)
                  setPreviewDirty(false)
                }}
                open={selectedOpen}
                lockedConfigKeys={requiredConfigKeys}
                selection={selection}
              />
            </div>
          ) : step === 2 ? (
            <PreviewStep
              onAddResolution={addResolution}
              onSelectTask={(ref, parentSourceRef) => {
                const next = new Map(selection.task)
                next.set(ref, { ref, label: ref, secondary: "由冲突处理补选" })
                updateSelection("task", next)
                if (parentSourceRef) {
                  setResolution((current) => ({
                    ...current,
                    drop_parent_task_refs:
                      current.drop_parent_task_refs?.filter(
                        (source) => source !== parentSourceRef
                      ) ?? [],
                  }))
                }
              }}
              preview={preview}
              previewDirty={previewDirty}
              firstIssueRef={firstIssueRef}
            />
          ) : (
            <ReviewStep
              anchorDate={anchorDate}
              mode={mode}
              name={name}
              preview={preview!}
              sourceProject={sourceProject}
              templateRef={templateRef}
            />
          )}
        </div>

        <DialogFooter className="mx-0 mb-0 shrink-0 rounded-none px-4 py-3">
          <div className="mr-auto hidden self-center text-xs text-muted-foreground sm:block">
            ⌘/Ctrl + Enter 提交当前步骤
          </div>
          <Button disabled={pending} onClick={close} variant="ghost">
            取消
          </Button>
          {step > 0 ? (
            <Button
              disabled={pending || defaultsPending}
              onClick={() => setStep((value) => value - 1)}
              variant="outline"
            >
              <ChevronLeft /> 返回
            </Button>
          ) : null}
          {step < 2 ? (
            <Button disabled={!canAdvance || pending} onClick={goNext}>
              下一步 <ChevronRight />
            </Button>
          ) : step === 2 && !preview ? (
            <Button
              disabled={pending || defaultsPending}
              onClick={() => void runPreview()}
            >
              {pending ? <Loader2 className="animate-spin" /> : null}生成预览
            </Button>
          ) : step === 2 ? (
            <>
              <Button
                disabled={pending || defaultsPending}
                onClick={() => void runPreview()}
                variant="outline"
              >
                {previewDirty ? "重新预览" : "刷新预览"}
              </Button>
              <Button disabled={!canAdvance || pending} onClick={goNext}>
                下一步 <ChevronRight />
              </Button>
              <Button disabled>保存快照</Button>
            </>
          ) : (
            <Button disabled={!canSave} onClick={() => void save()}>
              {pending ? <Loader2 className="animate-spin" /> : null}
              {mode === "create" ? "保存模板" : "保存快照"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function MetadataStep({
  anchorDate,
  description,
  keyValue,
  mode,
  name,
  onAnchorDateChange,
  onDescriptionChange,
  onKeyChange,
  onNameChange,
  sourceProject,
}: {
  anchorDate: string
  description: string
  keyValue: string
  mode: "create" | "append"
  name: string
  onAnchorDateChange: (value: string) => void
  onDescriptionChange: (value: string) => void
  onKeyChange: (value: string) => void
  onNameChange: (value: string) => void
  sourceProject: SourceProject
}) {
  return (
    <div className="mx-auto grid max-w-3xl gap-5 p-5 sm:grid-cols-2 sm:p-8">
      <div className="rounded-lg border bg-muted/20 p-4 sm:col-span-2">
        <div className="text-xs text-muted-foreground">来源项目</div>
        <div className="mt-1 text-sm font-medium">{sourceProject.name}</div>
        <div className="font-mono text-xs text-muted-foreground">
          {sourceProject.slug}
        </div>
      </div>
      {mode === "create" ? (
        <>
          <div className="space-y-2">
            <Label htmlFor="capture-key">模板标识（可选）</Label>
            <Input
              id="capture-key"
              onChange={(e) => onKeyChange(e.target.value)}
              placeholder={`留空使用项目 slug：${sourceProject.slug}`}
              value={keyValue}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="capture-name">模板名称</Label>
            <Input
              id="capture-name"
              onChange={(e) => onNameChange(e.target.value)}
              value={name}
            />
          </div>
          <div className="space-y-2 sm:col-span-2">
            <Label htmlFor="capture-description">说明</Label>
            <Textarea
              id="capture-description"
              onChange={(e) => onDescriptionChange(e.target.value)}
              value={description}
            />
          </div>
        </>
      ) : null}
      <div className="space-y-2">
        <Label htmlFor="capture-anchor">日期锚点</Label>
        <Input
          id="capture-anchor"
          onChange={(e) => onAnchorDateChange(e.target.value)}
          type="date"
          value={anchorDate}
        />
      </div>
    </div>
  )
}

function PreviewStep({
  firstIssueRef,
  onAddResolution,
  onSelectTask,
  preview,
  previewDirty,
}: {
  firstIssueRef: RefObject<HTMLDivElement | null>
  onAddResolution: (resolution: CaptureResolution) => void
  onSelectTask: (ref: string, parentSourceRef?: string) => void
  preview?: CapturePreview
  previewDirty: boolean
}) {
  if (!preview) {
    return (
      <div className="grid min-h-72 place-content-center p-8 text-center">
        <div className="text-sm font-medium">生成快照预览</div>
        <p className="mt-1 max-w-md text-xs text-muted-foreground">
          服务端会验证关系、内容引用、日期和循环边界；存在阻断问题时不能保存。
        </p>
        <span className="mt-4 text-xs text-muted-foreground">
          使用下方“生成预览”继续
        </span>
      </div>
    )
  }
  const configSummary = summarizeCapturedConfigs(
    preview.snapshot?.configs ?? []
  )
  return (
    <div className="mx-auto max-w-4xl space-y-4 p-5 sm:p-8">
      <dl className="grid grid-cols-4 gap-px border bg-border">
        {(["tasks", "series", "configs", "automations"] as const).map((key) => (
          <div className="bg-background p-3 text-center" key={key}>
            <dt className="text-[11px] text-muted-foreground">{key}</dt>
            <dd className="mt-1 text-lg font-semibold tabular-nums">
              {preview.counts[key]}
            </dd>
          </div>
        ))}
      </dl>
      <p className="text-xs text-muted-foreground">
        固定配置 {configSummary.fixed} · 继承配置 {configSummary.inherit} ·
        创建时填写：必填 {configSummary.required}、选填 {configSummary.optional}
      </p>
      {previewDirty ? (
        <Alert>
          <AlertTriangle />
          <AlertTitle>处理方式已更新</AlertTitle>
          <AlertDescription>
            请重新预览，服务端确认冲突清零后才能保存。
          </AlertDescription>
        </Alert>
      ) : null}
      {preview.blocking_issues.length ? (
        <section>
          <h3 className="mb-2 text-sm font-medium">阻断问题</h3>
          <div className="space-y-2">
            {preview.blocking_issues.map((issue, index) => (
              <IssueCard
                issue={issue}
                key={`${issue.code}:${issue.source_ref}:${issue.target_ref}:${index}`}
                onAddResolution={onAddResolution}
                onSelectTask={onSelectTask}
                ref={index === 0 ? firstIssueRef : undefined}
              />
            ))}
          </div>
        </section>
      ) : (
        <Alert>
          <Check />
          <AlertTitle>没有阻断问题</AlertTitle>
          <AlertDescription>可进入确认步骤保存不可变快照。</AlertDescription>
        </Alert>
      )}
      {preview.warnings.length ? (
        <section>
          <h3 className="mb-2 text-sm font-medium">提醒</h3>
          <ul className="space-y-2">
            {preview.warnings.map((warning, index) => (
              <WarningCard
                issue={warning}
                key={`${warning.code}:${index}`}
                onAddResolution={onAddResolution}
              />
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  )
}

function summarizeCapturedConfigs(configs: ProjectTemplateConfig[]) {
  return configs.reduce(
    (counts, config) => {
      if (config.mode === "inherit") counts.inherit += 1
      else if (config.mode !== "prompt") counts.fixed += 1
      else if (config.prompt?.required) counts.required += 1
      else counts.optional += 1
      return counts
    },
    { fixed: 0, inherit: 0, required: 0, optional: 0 }
  )
}

const IssueCard = forwardRef<
  HTMLDivElement,
  {
    issue: CaptureIssue
    onAddResolution: (resolution: CaptureResolution) => void
    onSelectTask: (ref: string, parentSourceRef?: string) => void
  }
>(function IssueCard({ issue, onAddResolution, onSelectTask }, ref) {
  const relation = issue.relation ?? "depends"
  const [seriesDayOffset, setSeriesDayOffset] = useState("")
  const [seriesLocalTime, setSeriesLocalTime] = useState("")
  const [clearUntil, setClearUntil] = useState(true)
  const isAttachment = issue.code === "project_template_attachment_unsupported"
  const isContentDrop =
    issue.code === "project_template_dependency_missing" &&
    relation === "content" &&
    Boolean(issue.source_ref && issue.target_ref) &&
    (issue.source_kind === "task" || issue.source_kind === "series")
  const isDependencyDrop =
    issue.code === "project_template_dependency_missing" &&
    (relation === "parent" || relation === "depends")
  const isSeriesSchedule =
    issue.code === "project_template_series_schedule_confirmation_required" &&
    Boolean(issue.source_ref)
  const dayOffset = Number(seriesDayOffset)
  const scheduleReady =
    seriesDayOffset.trim().length > 0 &&
    Number.isInteger(dayOffset) &&
    /^([01]\d|2[0-3]):[0-5]\d:[0-5]\d$/.test(seriesLocalTime)
  return (
    <div
      aria-label={issue.message}
      className="border border-destructive/40 bg-destructive/5 p-3 outline-none focus:ring-2 focus:ring-ring"
      ref={ref}
      role="alert"
      tabIndex={-1}
    >
      <div className="text-sm font-medium">{issue.message}</div>
      <div className="mt-1 font-mono text-[11px] text-muted-foreground">
        {issue.source_ref ?? "-"} → {issue.target_ref ?? "-"}
      </div>
      {isAttachment ? (
        <p className="mt-2 text-xs text-muted-foreground">
          附件引用不能作为模板内容保留，请先从来源内容移除。
        </p>
      ) : null}
      <div className="mt-3 flex flex-wrap gap-2">
        {issue.target_ref && !isAttachment ? (
          <Button
            onClick={() =>
              onSelectTask(
                issue.target_ref!,
                relation === "parent" ? issue.source_ref : undefined
              )
            }
            size="sm"
            variant="outline"
          >
            补选引用任务
          </Button>
        ) : null}
        {isDependencyDrop ? (
          <Button
            onClick={() =>
              onAddResolution(
                relation === "parent"
                  ? { drop_parent_task_refs: [issue.source_ref!] }
                  : {
                      drop_depends: [
                        {
                          source_task_ref: issue.source_ref!,
                          relation,
                          target_task_ref: issue.target_ref!,
                        },
                      ],
                    }
              )
            }
            size="sm"
            variant="outline"
          >
            {relation === "parent" ? "移除父任务关系" : "移除依赖关系"}
          </Button>
        ) : null}
        {isContentDrop ? (
          <Button
            onClick={() =>
              onAddResolution({
                drop_content_task_refs: [
                  {
                    source_kind: issue.source_kind ?? "task",
                    source_ref: issue.source_ref!,
                    target_task_ref: issue.target_ref!,
                  },
                ],
              })
            }
            size="sm"
            variant="outline"
          >
            移除内容引用
          </Button>
        ) : null}
        {issue.code.includes("date") && issue.source_ref && issue.field ? (
          <Button
            onClick={() =>
              onAddResolution({
                task_date_overrides: [
                  {
                    source_task_ref: issue.source_ref!,
                    field: issue.field!,
                    value: null,
                  },
                ],
              })
            }
            size="sm"
            variant="outline"
          >
            清除任务日期
          </Button>
        ) : null}
        {isSeriesSchedule ? (
          <div className="grid w-full gap-2 border p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
            <label className="grid gap-1 text-xs">
              循环首次到期日偏移
              <Input
                aria-label="循环首次到期日偏移"
                inputMode="numeric"
                onChange={(event) => setSeriesDayOffset(event.target.value)}
                value={seriesDayOffset}
              />
            </label>
            <label className="grid gap-1 text-xs">
              循环首次到期时间
              <Input
                aria-label="循环首次到期时间"
                onChange={(event) => setSeriesLocalTime(event.target.value)}
                placeholder="HH:MM:SS"
                value={seriesLocalTime}
              />
            </label>
            <div className="flex flex-col justify-end gap-2">
              <label className="flex items-center gap-2 text-xs">
                <Checkbox
                  aria-label="清除循环结束时间"
                  checked={clearUntil}
                  onCheckedChange={(checked) => setClearUntil(checked === true)}
                />
                清除结束时间
              </label>
              <Button
                disabled={!scheduleReady}
                onClick={() =>
                  onAddResolution({
                    series_schedule_overrides: [
                      {
                        source_series_ref: issue.source_ref!,
                        first_due: {
                          day_offset: dayOffset,
                          local_time: seriesLocalTime,
                        },
                        until: null,
                        clear_until: clearUntil,
                      },
                    ],
                  })
                }
                size="sm"
                variant="outline"
              >
                确认循环排期
              </Button>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  )
})

function WarningCard({
  issue,
  onAddResolution,
}: {
  issue: CaptureIssue
  onAddResolution: (resolution: CaptureResolution) => void
}) {
  const canOverrideDate =
    issue.source_kind === "task" &&
    Boolean(issue.source_ref && issue.field) &&
    issue.code.startsWith("project_template_date_")
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 border p-3 text-xs">
      <span>{issue.message}</span>
      {canOverrideDate ? (
        <Button
          onClick={() =>
            onAddResolution({
              task_date_overrides: [
                {
                  source_task_ref: issue.source_ref!,
                  field: issue.field!,
                  value: null,
                },
              ],
            })
          }
          size="sm"
          variant="outline"
        >
          清除 {issue.field} 日期
        </Button>
      ) : null}
    </li>
  )
}

function ReviewStep({
  anchorDate,
  mode,
  name,
  preview,
  sourceProject,
  templateRef,
}: {
  anchorDate: string
  mode: "create" | "append"
  name: string
  preview: CapturePreview
  sourceProject: SourceProject
  templateRef?: string
}) {
  return (
    <div className="mx-auto max-w-3xl space-y-4 p-5 sm:p-8">
      <Alert>
        <Check />
        <AlertTitle>预览已通过</AlertTitle>
        <AlertDescription>
          保存会生成不可变快照；自动化依赖的项目级机密值会以加密形式复制，且不会显示。
        </AlertDescription>
      </Alert>
      <dl className="grid gap-px border bg-border sm:grid-cols-2">
        {[
          ["目标", mode === "create" ? name : (templateRef ?? "-")],
          ["来源项目", `${sourceProject.name} (${sourceProject.slug})`],
          ["日期锚点", anchorDate],
          ["来源 Hash", preview.source_hash],
        ].map(([label, value]) => (
          <div className="bg-background p-3" key={label}>
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="mt-1 text-sm break-all">{value}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function captureInput(
  selection: CandidateSelectionStore,
  sourceProject: string,
  anchorDate: string,
  resolution: CaptureResolution,
  configPolicies: Record<string, CaptureConfigPolicy>
): CaptureInput {
  const input: CaptureInput = {
    source_project: sourceProject,
    anchor_date: anchorDate,
    selection: {
      task_refs: [...selection.task.keys()].sort(),
      series_refs: [...selection.series.keys()].sort(),
      config_keys: [...selection.config.keys()].sort(),
      automation_rule_ids: [...selection.automation.keys()].sort(),
    },
    config_policies: [...selection.config.keys()]
      .sort()
      .map((key) => configPolicies[key])
      .filter((policy): policy is CaptureConfigPolicy => Boolean(policy)),
  }
  if (Object.values(resolution).some((value) => value?.length)) {
    input.resolution = resolution
  }
  return input
}

function syncConfigPolicies(
  current: Record<string, CaptureConfigPolicy>,
  selected: Map<string, CandidateSummary>,
  locked: Set<string>
) {
  const next: Record<string, CaptureConfigPolicy> = {}
  for (const [key, summary] of selected) {
    const existing = current[key]
    if (existing) {
      next[key] =
        existing.strategy === "prompt" && locked.has(key)
          ? { key, strategy: "prompt", required: true }
          : existing
      continue
    }
    const canFixed = summary.configCanFixed !== false
    next[key] = canFixed
      ? { key, strategy: "fixed" }
      : { key, strategy: "inherit" }
  }
  return next
}

function emptySelection(): CandidateSelectionStore {
  return {
    task: new Map(),
    series: new Map(),
    config: new Map(),
    automation: new Map(),
  }
}

function cloneSelection(
  selection: CandidateSelectionStore
): CandidateSelectionStore {
  return {
    task: new Map(selection.task),
    series: new Map(selection.series),
    config: new Map(selection.config),
    automation: new Map(selection.automation),
  }
}

function mergeResolution(
  current: CaptureResolution,
  next: CaptureResolution
): CaptureResolution {
  const result: CaptureResolution = { ...current }
  for (const key of Object.keys(next) as Array<keyof CaptureResolution>) {
    const values = next[key]
    if (!values) continue
    ;(result as Record<string, unknown>)[key] = [
      ...((current[key] ?? []) as Array<unknown>),
      ...values,
    ]
  }
  return dedupeResolution(result)
}

function sanitizeResolution(
  resolution: CaptureResolution,
  selection: CandidateSelectionStore
): CaptureResolution {
  const selectedTasks = new Set(selection.task.keys())
  const selectedSeries = new Set(selection.series.keys())
  return dedupeResolution({
    drop_parent_task_refs: resolution.drop_parent_task_refs?.filter((source) =>
      selectedTasks.has(source)
    ),
    drop_depends: resolution.drop_depends?.filter(
      (item) =>
        selectedTasks.has(item.source_task_ref) &&
        !selectedTasks.has(item.target_task_ref)
    ),
    drop_content_task_refs: resolution.drop_content_task_refs?.filter(
      (item) =>
        (item.source_kind === "task"
          ? selectedTasks.has(item.source_ref)
          : selectedSeries.has(item.source_ref)) &&
        !selectedTasks.has(item.target_task_ref)
    ),
    task_date_overrides: resolution.task_date_overrides?.filter((item) =>
      selectedTasks.has(item.source_task_ref)
    ),
    series_schedule_overrides: resolution.series_schedule_overrides?.filter(
      (item) => selectedSeries.has(item.source_series_ref)
    ),
  })
}

function dedupeResolution(resolution: CaptureResolution): CaptureResolution {
  return {
    drop_parent_task_refs: uniqueBy(
      resolution.drop_parent_task_refs ?? [],
      (item) => item
    ),
    drop_depends: uniqueBy(
      resolution.drop_depends ?? [],
      (item) =>
        `${item.source_task_ref}\u0000${item.relation}\u0000${item.target_task_ref}`
    ),
    drop_content_task_refs: uniqueBy(
      resolution.drop_content_task_refs ?? [],
      (item) =>
        `${item.source_kind}\u0000${item.source_ref}\u0000${item.target_task_ref}`
    ),
    task_date_overrides: uniqueBy(
      resolution.task_date_overrides ?? [],
      (item) => `${item.source_task_ref}\u0000${item.field}`
    ),
    series_schedule_overrides: uniqueBy(
      resolution.series_schedule_overrides ?? [],
      (item) => item.source_series_ref
    ),
  }
}

function uniqueBy<T>(items: T[], key: (item: T) => string): T[] {
  const seen = new Set<string>()
  const result: T[] = []
  for (const item of [...items].reverse()) {
    const value = key(item)
    if (seen.has(value)) continue
    seen.add(value)
    result.push(item)
  }
  return result.reverse()
}

function sameSelectionMap(
  left: Map<string, CandidateSummary>,
  right: Map<string, CandidateSummary>
) {
  if (left.size !== right.size) return false
  for (const [ref, item] of left) {
    const other = right.get(ref)
    if (
      !other ||
      item.label !== other.label ||
      item.secondary !== other.secondary ||
      item.secret !== other.secret
    ) {
      return false
    }
  }
  return true
}

function sameSelectionStore(
  left: CandidateSelectionStore,
  right: CandidateSelectionStore
) {
  return kinds.every((kind) => sameSelectionMap(left[kind], right[kind]))
}

function today() {
  const now = new Date()
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 10)
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError ? `${fallback} (${error.code})` : fallback
}
