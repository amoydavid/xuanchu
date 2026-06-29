import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useState } from "react"
import { PencilIcon } from "lucide-react"

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
import { Label } from "@/components/ui/label"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApiError } from "@/lib/api"
import { navigateToDocument } from "@/lib/browser-navigation"
import {
  setAdminActingContext,
  setAdminActingToken,
} from "@/features/workspace/session/workspace-token"

import {
  createAdminActingSession,
  fetchAdminWorkspaceDetail,
  modifyAdminWorkspaceUser,
  type AdminActingCandidate,
  type AdminWorkspaceMember,
} from "./admin-workspace-api"

type AdminWorkspaceDetailPageProps = {
  workspaceSlug: string
}

export function AdminWorkspaceDetailPage({
  workspaceSlug,
}: AdminWorkspaceDetailPageProps) {
  const { t } = useTranslation()
  const [actingOpen, setActingOpen] = useState(false)
  const [editingMember, setEditingMember] = useState<AdminWorkspaceMember | null>(
    null
  )

  const query = useQuery({
    queryKey: ["admin", "workspaces", "detail", workspaceSlug],
    queryFn: () => fetchAdminWorkspaceDetail(workspaceSlug),
  })

  if (query.isLoading) {
    return (
      <div className="space-y-2 rounded-none border bg-card p-3">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-2/3" />
      </div>
    )
  }
  if (query.isError) {
    return (
      <div className="border bg-card p-4 text-sm text-destructive">
        {errorMessage(query.error, t("common.error"))}
      </div>
    )
  }
  const detail = query.data
  if (!detail) {
    return null
  }

  const hasCandidates = detail.acting_candidates.length > 0
  const archived = detail.workspace.archived_at != null

  return (
    <div className="space-y-4">
      <div>
        <div className="text-sm text-muted-foreground">
          {t("admin.workspace.title")}
        </div>
        <h1 className="text-lg font-medium">
          {detail.workspace.name}{" "}
          <span className="text-muted-foreground">({detail.workspace.slug})</span>
        </h1>
        <div className="mt-1 text-xs text-muted-foreground">
          <Badge variant="outline">
            {t(`admin.visibility.${detail.workspace.visibility}`)}
          </Badge>{" "}
          <Badge variant={archived ? "destructive" : "default"}>
            {archived
              ? t("admin.workspace.statusArchived")
              : t("admin.workspace.statusActive")}
          </Badge>
        </div>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button
          disabled={archived || !hasCandidates}
          onClick={() => setActingOpen(true)}
          size="sm"
        >
          {t("admin.workspace.enterAsAdmin")}
        </Button>
      </div>

      {!hasCandidates ? (
        <div className="border bg-card p-4 text-sm text-muted-foreground">
          {t("admin.workspace.noActingCandidates")}
        </div>
      ) : null}

      <section className="space-y-2">
        <h2 className="text-sm font-medium">{t("admin.workspace.members")}</h2>
        <div className="rounded-none border bg-card">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-muted-foreground">
                  {t("common.actor")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.role")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.joinedAt")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {detail.members.map((member) => (
                <TableRow key={member.user.id}>
                  <TableCell>
                    <div className="flex min-w-0 items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="truncate text-sm">
                          {displayUserName(member.user)}
                        </div>
                        <div className="truncate text-xs text-muted-foreground">
                          {member.user.name}
                        </div>
                        {member.user.email ? (
                          <div className="truncate text-xs text-muted-foreground">
                            {member.user.email}
                          </div>
                        ) : null}
                      </div>
                      <Button
                        aria-label={t("admin.workspace.editDisplayNameAria", {
                          name: displayUserName(member.user),
                        })}
                        onClick={() => setEditingMember(member)}
                        size="icon"
                        type="button"
                        variant="ghost"
                      >
                        <PencilIcon />
                      </Button>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">{member.role}</Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatTime(member.joined_at)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        <div className="text-xs text-muted-foreground">
          {t("admin.workspace.tokenSummary", {
            active: detail.token_counts.active,
            revoked: detail.token_counts.revoked,
            expired: detail.token_counts.expired,
          })}
        </div>
      </section>

      <ActingSessionDialog
        candidates={detail.acting_candidates}
        onOpenChange={setActingOpen}
        open={actingOpen}
        workspaceName={detail.workspace.name}
        workspaceSlug={detail.workspace.slug}
      />
      <DisplayNameDialog
        member={editingMember}
        onOpenChange={(open) => {
          if (!open) {
            setEditingMember(null)
          }
        }}
        workspaceSlug={detail.workspace.slug}
      />
    </div>
  )
}

function DisplayNameDialog({
  member,
  onOpenChange,
  workspaceSlug,
}: {
  member: AdminWorkspaceMember | null
  onOpenChange: (open: boolean) => void
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const open = member !== null
  const draftSource = member
    ? `${member.user.id}\u0000${member.user.display_name ?? ""}`
    : ""
  const [draftState, setDraftState] = useState({ source: "", value: "" })
  const value =
    draftState.source === draftSource
      ? draftState.value
      : member?.user.display_name ?? ""
  const mutation = useMutation({
    mutationFn: async (displayName: string) => {
      if (!member) {
        return null
      }
      return modifyAdminWorkspaceUser(workspaceSlug, member.user.id, {
        display_name: displayName,
      })
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["admin", "workspaces", "detail", workspaceSlug],
      })
      handleOpenChange(false)
    },
  })

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setDraftState({ source: "", value: "" })
      mutation.reset()
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("admin.workspace.editDisplayName")}</DialogTitle>
          <DialogDescription>
            {member
              ? t("admin.workspace.editDisplayNameDescription", {
                  name: member.user.name,
                })
              : ""}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="admin-member-display-name">
            {t("admin.workspace.field.displayName")}
          </Label>
          <Input
            id="admin-member-display-name"
            onChange={(event) =>
              setDraftState({ source: draftSource, value: event.target.value })
            }
            value={value}
          />
        </div>
        {mutation.isError ? (
          <div className="text-sm text-destructive">
            {errorMessage(
              mutation.error instanceof Error ? mutation.error : null,
              t("common.error")
            )}
          </div>
        ) : null}
        <DialogFooter>
          <Button
            onClick={() => handleOpenChange(false)}
            type="button"
            variant="outline"
          >
            {t("common.cancel")}
          </Button>
          <Button
            disabled={mutation.isPending}
            onClick={() => {
              void mutation.mutateAsync(value.trim())
            }}
            type="button"
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type ActingSessionDialogProps = {
  candidates: AdminActingCandidate[]
  open: boolean
  onOpenChange: (open: boolean) => void
  workspaceName: string
  workspaceSlug: string
}

function ActingSessionDialog({
  candidates,
  open,
  onOpenChange,
  workspaceName,
  workspaceSlug,
}: ActingSessionDialogProps) {
  const { t } = useTranslation()
  const firstCandidate = candidates[0]
  const firstCandidateID = firstCandidate?.user.id ?? ""
  const [selectedState, setSelectedState] = useState(() => ({
    source: firstCandidateID,
    value: firstCandidateID,
  }))
  const selectedUserId =
    selectedState.source === firstCandidateID
      ? selectedState.value
      : firstCandidateID
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // 当 candidates 变化时同步默认选中项。
  const candidate =
    candidates.find((c) => c.user.id === selectedUserId) ?? firstCandidate

  async function handleConfirm() {
    if (!candidate) {
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      const created = await createAdminActingSession(workspaceSlug, {
        user: candidate.user.email ?? candidate.user.id,
      })
      // 只写入 acting token + context，不覆盖普通 workspace token 或 admin token。
      setAdminActingToken(created.token)
      setAdminActingContext({
        workspaceSlug: created.workspace.slug,
        workspaceName: created.workspace.name,
        actorName: created.actor.name,
        role: created.role,
        adminTokenName: created.admin_token_name,
      })
      // 跳转到 workspace console（SPA fallback 会渲染前端）。
      navigateToDocument(
        `/workspaces/${encodeURIComponent(created.workspace.slug)}/projects`
      )
    } catch (err) {
      setError(err instanceof ApiError ? err.code : "unknown")
      setSubmitting(false)
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("admin.acting.title")}</DialogTitle>
          <DialogDescription>
            {t("admin.acting.summary", {
              workspace: workspaceName,
              actor: candidate?.user.name ?? "",
              role: candidate?.role ?? "",
            })}
          </DialogDescription>
        </DialogHeader>
        {candidates.length > 1 ? (
          <div className="space-y-1.5">
            <Label>{t("admin.acting.user")}</Label>
            <Select
              onValueChange={(next) =>
                setSelectedState({ source: firstCandidateID, value: next })
              }
              value={selectedUserId}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {candidates.map((c) => (
                  <SelectItem key={c.user.id} value={c.user.id}>
                    {c.user.name} ({c.role})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : null}
        {error ? (
          <div className="text-sm text-destructive">{error}</div>
        ) : null}
        <DialogFooter>
          <Button
            disabled={submitting || !candidate}
            onClick={handleConfirm}
          >
            {t("admin.acting.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function formatTime(unix: number): string {
  return new Date(unix * 1000).toLocaleString()
}

function displayUserName(user: { name: string; display_name?: string }): string {
  return user.display_name?.trim() || user.name
}

function errorMessage(error: Error | null, fallback: string): string {
  if (error instanceof ApiError) {
    return `${fallback}: ${error.code}`
  }
  return fallback
}
