import { useMemo, useState, type ChangeEvent, type ReactNode } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  AlertTriangleIcon,
  BracesIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  FileJsonIcon,
  FileSpreadsheetIcon,
  UploadIcon,
  UserPlusIcon,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApiError } from "@/lib/api"

import type { ProjectWorkbenchTask } from "../api/project-api"
import {
  addWorkspaceMember,
  createWorkspaceUser,
  getWorkspaceMembers,
  listWorkspaceUsers,
  type WorkspaceMemberCandidate,
  type WorkspaceUserCandidate,
} from "../api/users-api"
import { useImportTasksMutation } from "../hooks/use-task-mutations"
import {
  parseTaskImportJSON,
  preflightTaskImport,
  type TaskImportIssue,
  type TaskImportPayload,
} from "./task-import"
import {
  downloadTaskImportTemplate,
  parseTaskImportXLSXFile,
} from "./task-import-xlsx"
import { TASK_IMPORT_JSON_SCHEMA_TEXT } from "./task-import-schema"

type TaskImportDialogProps = {
  existingTasks: ProjectWorkbenchTask[]
  onOpenChange: (open: boolean) => void
  open: boolean
  projectSlug: string
  workspaceSlug: string
}

export function TaskImportDialog({
  existingTasks,
  onOpenChange,
  open,
  projectSlug,
  workspaceSlug,
}: TaskImportDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const importTasks = useImportTasksMutation(workspaceSlug, projectSlug)
  const members = useQuery({
    enabled: open,
    queryFn: () => getWorkspaceMembers(workspaceSlug),
    queryKey: ["workspace", workspaceSlug, "members"],
  })
  const users = useQuery({
    enabled: open,
    queryFn: () => listWorkspaceUsers(),
    queryKey: ["workspace", workspaceSlug, "users"],
  })
  const [payload, setPayload] = useState<TaskImportPayload | null>(null)
  const [fileName, setFileName] = useState("")
  const [parseError, setParseError] = useState<string | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [assigneeRepairError, setAssigneeRepairError] = useState<string | null>(null)
  const [schemaOpen, setSchemaOpen] = useState(false)

  const preflight = useMemo(() => {
    if (!payload || !members.data) {
      return { blockers: [], warnings: [] }
    }
    return preflightTaskImport(payload.tasks, {
      currentProjectSlug: projectSlug,
      existingTasks,
      members: members.data,
    })
  }, [existingTasks, members.data, payload, projectSlug])

  const warnings = useMemo(
    () => [...(payload?.warnings ?? []), ...preflight.warnings],
    [payload?.warnings, preflight.warnings]
  )
  const repairableAssigneeRefs = useMemo(
    () => assigneeBlockerRefs(preflight.blockers),
    [preflight.blockers]
  )
  const assigneeRepair = useMutation({
    mutationFn: async (refs: string[]) => {
      const resolved: WorkspaceMemberCandidate[] = []
      for (const ref of refs) {
        const existing = findUserByRef(users.data ?? [], ref)
        const user =
          existing ??
          (await createWorkspaceUser(userCreateInputFromAssigneeRef(ref)))
        await addWorkspaceMember(workspaceSlug, user.id, "member")
        resolved.push({
          user_id: user.id,
          name: user.name,
          email: user.email,
          role: "member",
          joined_at: currentUnix(),
          modified_at: currentUnix(),
        })
      }
      return resolved
    },
    onSuccess: async (resolved) => {
      queryClient.setQueryData<WorkspaceMemberCandidate[]>(
        ["workspace", workspaceSlug, "members"],
        (current = []) => mergeMembers(current, resolved)
      )
      await members.refetch()
      void queryClient.invalidateQueries({
        queryKey: ["workspace", workspaceSlug, "users"],
      })
    },
  })
  const canRepairAssignees =
    repairableAssigneeRefs.length > 0 &&
    !users.isPending &&
    !users.isError &&
    !assigneeRepair.isPending
  const canSubmit =
    Boolean(payload) &&
    preflight.blockers.length === 0 &&
    !members.isPending &&
    !members.isError &&
    !importTasks.isPending

  function reset() {
    setPayload(null)
    setFileName("")
    setParseError(null)
    setSubmitError(null)
    setAssigneeRepairError(null)
    importTasks.reset()
    assigneeRepair.reset()
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      reset()
    }
    onOpenChange(nextOpen)
  }

  async function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    setParseError(null)
    setSubmitError(null)
    setAssigneeRepairError(null)
    setPayload(null)
    setFileName(file?.name ?? "")
    if (!file) {
      return
    }
    try {
      const nowISO = new Date().toISOString()
      const nextPayload = isSpreadsheetFile(file)
        ? await parseTaskImportXLSXFile(file, { nowISO, projectSlug })
        : parseTaskImportJSON(await file.text(), { nowISO, projectSlug })
      setPayload(nextPayload)
    } catch (error) {
      setParseError(error instanceof Error ? error.message : String(error))
    }
  }

  async function handleSubmit() {
    if (!payload || !canSubmit) {
      return
    }
    setSubmitError(null)
    try {
      await importTasks.mutateAsync(payload.tasks)
      handleOpenChange(false)
    } catch (error) {
      const code = error instanceof ApiError ? error.code : "unknown"
      setSubmitError(
        t(`projectWorkbench.import.errors.${code}`, {
          defaultValue: error instanceof Error ? error.message : String(error),
        })
      )
    }
  }

  async function handleRepairAssignees() {
    if (!canRepairAssignees) {
      return
    }
    setAssigneeRepairError(null)
    try {
      await assigneeRepair.mutateAsync(repairableAssigneeRefs)
    } catch (error) {
      const code = error instanceof ApiError ? error.code : "unknown"
      setAssigneeRepairError(
        t(`projectWorkbench.import.errors.${code}`, {
          defaultValue: error instanceof Error ? error.message : String(error),
        })
      )
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="grid max-h-[90vh] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>{t("projectWorkbench.import.title")}</DialogTitle>
          <DialogDescription>
            {t("projectWorkbench.import.description", { project: projectSlug })}
          </DialogDescription>
        </DialogHeader>

        <div className="grid min-h-0 gap-4 overflow-y-auto pr-1">
          <div className="grid gap-3 rounded-lg border bg-muted/30 p-3 md:grid-cols-[1fr_auto] md:items-center">
            <div className="flex items-start gap-3">
              <FileSpreadsheetIcon className="mt-0.5 size-4 text-muted-foreground" />
              <div>
                <div className="text-sm font-medium">
                  {t("projectWorkbench.import.templateTitle")}
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("projectWorkbench.import.templateDescription")}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2 md:justify-end">
              <Button
                onClick={() => setSchemaOpen(true)}
                size="sm"
                type="button"
                variant="outline"
              >
                <BracesIcon />
                {t("projectWorkbench.import.viewSchema")}
              </Button>
              <Button
                onClick={() => downloadTaskImportTemplate()}
                size="sm"
                type="button"
                variant="outline"
              >
                <FileSpreadsheetIcon />
                {t("projectWorkbench.import.downloadTemplate")}
              </Button>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="task-import-file">
              {t("projectWorkbench.import.fileLabel")}
            </Label>
            <Input
              accept=".json,.xlsx,application/json,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
              id="task-import-file"
              onChange={(event) => {
                void handleFileChange(event)
              }}
              type="file"
            />
            <p className="text-xs text-muted-foreground">
              {t("projectWorkbench.import.fileHint")}
            </p>
          </div>

          {members.isPending ? (
            <Alert>
              <AlertDescription>
                {t("projectWorkbench.import.loadingMembers")}
              </AlertDescription>
            </Alert>
          ) : null}

          {members.isError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("projectWorkbench.import.memberLoadFailed")}</AlertTitle>
              <AlertDescription>
                {t("projectWorkbench.import.memberLoadFailedDescription")}
              </AlertDescription>
            </Alert>
          ) : null}

          {users.isError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("projectWorkbench.import.userLoadFailed")}</AlertTitle>
              <AlertDescription>
                {t("projectWorkbench.import.userLoadFailedDescription")}
              </AlertDescription>
            </Alert>
          ) : null}

          {parseError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("projectWorkbench.import.parseFailed")}</AlertTitle>
              <AlertDescription>{parseError}</AlertDescription>
            </Alert>
          ) : null}

          {payload ? (
            <ImportPreview
              blockers={preflight.blockers}
              fileName={fileName}
              onRepairAssignees={() => {
                void handleRepairAssignees()
              }}
              repairAssigneeCount={repairableAssigneeRefs.length}
              repairAssigneesDisabled={!canRepairAssignees}
              repairAssigneesPending={assigneeRepair.isPending}
              tasks={payload.tasks}
              warnings={warnings}
            />
          ) : (
            <div className="grid min-h-32 place-items-center rounded-lg border border-dashed text-center text-sm text-muted-foreground">
              <div>
                <UploadIcon className="mx-auto mb-2 size-5" />
                {t("projectWorkbench.import.emptyState")}
              </div>
            </div>
          )}

          {submitError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("projectWorkbench.import.submitFailed")}</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          ) : null}

          {assigneeRepairError ? (
            <Alert variant="destructive">
              <AlertTitle>
                {t("projectWorkbench.import.assigneeRepairFailed")}
              </AlertTitle>
              <AlertDescription>{assigneeRepairError}</AlertDescription>
            </Alert>
          ) : null}
        </div>

        <DialogFooter>
          <Button
            onClick={() => handleOpenChange(false)}
            type="button"
            variant="outline"
          >
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!canSubmit}
            onClick={() => {
              void handleSubmit()
            }}
            type="button"
          >
            {importTasks.isPending
              ? t("projectWorkbench.import.importing")
              : t("projectWorkbench.import.submit", {
                  count: payload?.tasks.length ?? 0,
                })}
          </Button>
        </DialogFooter>
      </DialogContent>
      </Dialog>

      <Dialog open={schemaOpen} onOpenChange={setSchemaOpen}>
        <DialogContent className="grid max-h-[85vh] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-4xl">
          <DialogHeader>
            <DialogTitle>{t("projectWorkbench.import.schemaTitle")}</DialogTitle>
            <DialogDescription>
              {t("projectWorkbench.import.schemaDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 overflow-auto rounded-lg border bg-muted/30">
            <pre className="min-w-[48rem] p-3 font-mono text-xs leading-relaxed whitespace-pre">
              {TASK_IMPORT_JSON_SCHEMA_TEXT}
            </pre>
          </div>
          <DialogFooter>
            <Button
              onClick={() => setSchemaOpen(false)}
              type="button"
              variant="outline"
            >
              {t("common.close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

const PREVIEW_PAGE_SIZE = 10

function ImportPreview({
  blockers,
  fileName,
  onRepairAssignees,
  repairAssigneeCount,
  repairAssigneesDisabled,
  repairAssigneesPending,
  tasks,
  warnings,
}: {
  blockers: TaskImportIssue[]
  fileName: string
  onRepairAssignees: () => void
  repairAssigneeCount: number
  repairAssigneesDisabled: boolean
  repairAssigneesPending: boolean
  tasks: TaskImportPayload["tasks"]
  warnings: TaskImportIssue[]
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(0)
  const totalPages = Math.max(1, Math.ceil(tasks.length / PREVIEW_PAGE_SIZE))
  const safePage = Math.min(page, totalPages - 1)
  const start = safePage * PREVIEW_PAGE_SIZE
  const visibleTasks = tasks.slice(start, start + PREVIEW_PAGE_SIZE)
  const rangeStart = tasks.length === 0 ? 0 : start + 1
  const rangeEnd = Math.min(start + PREVIEW_PAGE_SIZE, tasks.length)

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-card p-3">
        <FileJsonIcon className="size-4 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {fileName}
        </span>
        <Badge variant="secondary">
          {t("projectWorkbench.import.taskCount", { count: tasks.length })}
        </Badge>
        <Badge variant={blockers.length > 0 ? "destructive" : "secondary"}>
          {t("projectWorkbench.import.blockerCount", {
            count: blockers.length,
          })}
        </Badge>
        <Badge variant="outline">
          {t("projectWorkbench.import.warningCount", {
            count: warnings.length,
          })}
        </Badge>
      </div>

      {blockers.length > 0 ? (
        <div className="space-y-2">
          <IssueList
            icon={<AlertTriangleIcon className="size-4" />}
            issues={blockers}
            title={t("projectWorkbench.import.blockersTitle")}
            variant="destructive"
          />
          {repairAssigneeCount > 0 ? (
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/30 p-3">
              <div className="text-sm">
                <div className="font-medium">
                  {t("projectWorkbench.import.assigneeRepairTitle")}
                </div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {t("projectWorkbench.import.assigneeRepairDescription")}
                </div>
              </div>
              <Button
                disabled={repairAssigneesDisabled}
                onClick={onRepairAssignees}
                size="sm"
                type="button"
                variant="outline"
              >
                <UserPlusIcon />
                {repairAssigneesPending
                  ? t("projectWorkbench.import.assigneeRepairing")
                  : t("projectWorkbench.import.assigneeRepairAction", {
                      count: repairAssigneeCount,
                    })}
              </Button>
            </div>
          ) : null}
        </div>
      ) : null}

      {warnings.length > 0 ? (
        <IssueList
          issues={warnings}
          title={t("projectWorkbench.import.warningsTitle")}
        />
      ) : null}

      <div className="rounded-lg border">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
          <div>
            <div className="text-sm font-medium">
              {t("projectWorkbench.import.previewTitle")}
            </div>
            <div className="mt-0.5 text-xs text-muted-foreground">
              {t("projectWorkbench.import.previewDescription")}
            </div>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span>
              {t("projectWorkbench.import.previewRange", {
                count: tasks.length,
                end: rangeEnd,
                start: rangeStart,
              })}
            </span>
            <Button
              aria-label={t("projectWorkbench.import.previousPage")}
              disabled={safePage === 0}
              onClick={() => setPage((current) => Math.max(0, current - 1))}
              size="icon-sm"
              type="button"
              variant="outline"
            >
              <ChevronLeftIcon />
            </Button>
            <Button
              aria-label={t("projectWorkbench.import.nextPage")}
              disabled={safePage >= totalPages - 1}
              onClick={() =>
                setPage((current) => Math.min(totalPages - 1, current + 1))
              }
              size="icon-sm"
              type="button"
              variant="outline"
            >
              <ChevronRightIcon />
            </Button>
          </div>
        </div>
        <div className="max-h-80 overflow-auto">
          <Table className="min-w-[58rem]">
            <TableHeader className="sticky top-0 z-10 bg-background">
              <TableRow>
                <TableHead>{t("projectWorkbench.import.previewColumns.title")}</TableHead>
                <TableHead>{t("projectWorkbench.import.previewColumns.status")}</TableHead>
                <TableHead>{t("projectWorkbench.import.previewColumns.priority")}</TableHead>
                <TableHead>{t("projectWorkbench.import.previewColumns.assignees")}</TableHead>
                <TableHead>{t("projectWorkbench.import.previewColumns.blockedBy")}</TableHead>
                <TableHead>{t("projectWorkbench.import.previewColumns.due")}</TableHead>
                <TableHead>{t("projectWorkbench.import.previewColumns.tags")}</TableHead>
                <TableHead className="w-72">
                  {t("projectWorkbench.import.previewColumns.description")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visibleTasks.map((task, index) => (
                <TableRow key={task.uuid || `${task.title}-${start + index}`}>
                  <TableCell className="max-w-56 truncate font-medium">
                    {task.title || t("projectWorkbench.import.untitled")}
                  </TableCell>
                  <TableCell>{task.status}</TableCell>
                  <TableCell>{task.priority || "-"}</TableCell>
                  <TableCell className="max-w-48 truncate">
                    {formatAssignees(task.assignees)}
                  </TableCell>
                  <TableCell className="max-w-56 truncate">
                    {formatList(task.depends)}
                  </TableCell>
                  <TableCell>{formatDatePreview(task.due)}</TableCell>
                  <TableCell className="max-w-48 truncate">
                    {formatList(task.tags)}
                  </TableCell>
                  <TableCell className="max-w-72 whitespace-normal">
                    <span className="line-clamp-2 text-xs text-muted-foreground">
                      {task.description || "-"}
                    </span>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </div>
    </div>
  )
}

function IssueList({
  icon,
  issues,
  title,
  variant,
}: {
  icon?: ReactNode
  issues: TaskImportIssue[]
  title: string
  variant?: "destructive"
}) {
  return (
    <Alert variant={variant}>
      {icon}
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        <ul className="mt-1 list-disc space-y-1 pl-4">
          {issues.slice(0, 8).map((item, index) => (
            <li key={`${item.code}-${item.ref}-${item.row}-${index}`}>
              {item.row ? `#${item.row} ` : ""}
              {item.message}
            </li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  )
}

function isSpreadsheetFile(file: File): boolean {
  return file.name.toLowerCase().endsWith(".xlsx")
}

function assigneeBlockerRefs(blockers: TaskImportIssue[]): string[] {
  const refs = new Set<string>()
  for (const blocker of blockers) {
    if (blocker.code === "assignee_not_member" && blocker.ref) {
      refs.add(blocker.ref)
    }
  }
  return Array.from(refs)
}

function findUserByRef(
  users: WorkspaceUserCandidate[],
  ref: string
): WorkspaceUserCandidate | undefined {
  const normalized = ref.trim().toLowerCase()
  return users.find(
    (user) =>
      user.id.toLowerCase() === normalized ||
      user.name.toLowerCase() === normalized ||
      (user.email?.toLowerCase() ?? "") === normalized
  )
}

function userCreateInputFromAssigneeRef(ref: string) {
  const trimmed = ref.trim()
  if (trimmed.includes("@")) {
    const localPart = trimmed.split("@")[0]?.trim() || trimmed
    return { email: trimmed, name: localPart }
  }
  return { name: trimmed }
}

function mergeMembers(
  current: WorkspaceMemberCandidate[],
  additions: WorkspaceMemberCandidate[]
): WorkspaceMemberCandidate[] {
  const byID = new Map(current.map((member) => [member.user_id, member]))
  for (const member of additions) {
    byID.set(member.user_id, member)
  }
  return Array.from(byID.values())
}

function currentUnix(): number {
  return Math.floor(Date.now() / 1000)
}

function formatList(values: string[] | null | undefined): string {
  return values && values.length > 0 ? values.join(", ") : "-"
}

function formatAssignees(
  assignees: TaskImportPayload["tasks"][number]["assignees"]
): string {
  if (!assignees || assignees.length === 0) {
    return "-"
  }
  return assignees
    .map((assignee) => {
      if (typeof assignee === "string") {
        return assignee
      }
      return assignee.user_id ?? assignee.email ?? assignee.name ?? "-"
    })
    .join(", ")
}

function formatDatePreview(value: string | null | undefined): string {
  if (!value) {
    return "-"
  }
  return value.length > 10 && value.endsWith("T00:00:00.000Z")
    ? value.slice(0, 10)
    : value
}
