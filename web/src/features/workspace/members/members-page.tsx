import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { MoreHorizontalIcon, PlusIcon } from "lucide-react"
import type { FormEvent } from "react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { MeResponse } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { PageHeader } from "@/pages/OverviewPage"

import {
  addWorkspaceMember,
  getWorkspaceUser,
  listWorkspaceAudit,
  listWorkspaceMembers,
  listWorkspaceTokens,
  modifyWorkspaceMember,
  removeWorkspaceMember,
  unbindExternalID as unbindExternalIDAPI,
  type AddWorkspaceMemberInput,
  type WorkspaceAuditRow,
  type WorkspaceMemberRow,
  type WorkspaceTokenRow,
  type WorkspaceUserRow,
} from "./members-api"
import { ExternalIDBindDialog } from "./external-id-bind-dialog"

const ROLE_OPTIONS = ["viewer", "member", "admin", "owner"] as const
const ROLE_COUNT_OPTIONS = ["owner", "admin", "member", "viewer"] as const

type MemberRole = (typeof ROLE_OPTIONS)[number]
type Translate = ReturnType<typeof useTranslation>["t"]

type MembersPageProps = {
  workspaceSlug: string
  credential?: MeResponse | null
}

export function MembersPage({ credential, workspaceSlug }: MembersPageProps) {
  const { t } = useTranslation()
  const [queryText, setQueryText] = useState("")
  const [roleFilter, setRoleFilter] = useState("all")
  const [addOpen, setAddOpen] = useState(false)
  const [editingMember, setEditingMember] = useState<WorkspaceMemberRow | null>(
    null
  )
  const [roleMember, setRoleMember] = useState<WorkspaceMemberRow | null>(null)
  const [removingMember, setRemovingMember] = useState<WorkspaceMemberRow | null>(
    null
  )
  const query = useQuery({
    enabled: workspaceSlug !== "",
    queryKey: ["workspace", "members", workspaceSlug],
    queryFn: () => listWorkspaceMembers(workspaceSlug),
  })
  const members = useMemo(() => query.data ?? [], [query.data])
  const canWriteMembers =
    credential?.capabilities?.includes("member:write") ?? false
  const role = credential?.effective_role ?? ""
  const isOwner = role === "owner"
  const isAdmin = role === "admin"
  const isManager = isOwner || isAdmin
  const canManageMembers = canWriteMembers && isManager
  const visibleMembers = useMemo(() => {
    const normalized = queryText.trim().toLowerCase()
    return members.filter((member) => {
      if (roleFilter !== "all" && member.role !== roleFilter) {
        return false
      }
      if (!normalized) {
        return true
      }
      return [displayMemberName(member), member.name, member.email ?? ""]
        .join(" ")
        .toLowerCase()
        .includes(normalized)
    })
  }, [members, queryText, roleFilter])
  const counts = useMemo(() => memberCounts(members), [members])

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <PageHeader title={t("page.members")} />
        {canManageMembers ? (
          <Button onClick={() => setAddOpen(true)} type="button">
            <PlusIcon />
            {t("members.add")}
          </Button>
        ) : null}
      </div>

      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        {ROLE_COUNT_OPTIONS.map((item) => (
          <Badge key={item} variant="secondary">
            {t("members.roleCount", {
              count: counts[item],
              role: memberRoleLabel(t, item),
            })}
          </Badge>
        ))}
        {!canManageMembers ? (
          <Badge variant="outline">{t("members.readonly")}</Badge>
        ) : null}
        {credential?.actor_type === "tenant_access_token" ? (
          <Badge>{t("members.systemIdentity")}</Badge>
        ) : null}
      </div>

      {!canManageMembers ? (
        <Alert>
          <AlertDescription>
            {canWriteMembers
              ? t("members.readonlyRoleHint")
              : t("members.readonlyScopeHint")}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          aria-label={t("members.search")}
          className="sm:max-w-xs"
          onChange={(event) => setQueryText(event.target.value)}
          placeholder={t("members.searchPlaceholder")}
          value={queryText}
        />
        <Select value={roleFilter} onValueChange={setRoleFilter}>
          <SelectTrigger aria-label={t("members.roleFilter")} className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("members.allRoles")}</SelectItem>
            {ROLE_OPTIONS.map((item) => (
              <SelectItem key={item} value={item}>
                {memberRoleLabel(t, item)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {query.isLoading || workspaceSlug === "" ? (
        <div className="space-y-2 rounded-none border bg-card p-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-2/3" />
          <Skeleton className="h-8 w-1/2" />
        </div>
      ) : query.isError ? (
        <div className="border bg-card p-4 text-sm text-destructive">
          {errorMessage(query.error, t("common.error"))}
        </div>
      ) : (
        <div className="rounded-none border bg-card">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-muted-foreground">
                  {t("common.actor")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("resource.role")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("members.joinedAt")}
                </TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {visibleMembers.length === 0 ? (
                <TableRow>
                  <TableCell
                    className="h-24 text-center text-muted-foreground"
                    colSpan={4}
                  >
                    {members.length === 0
                      ? t("common.empty")
                      : t("members.noMatches")}
                  </TableCell>
                </TableRow>
              ) : (
                visibleMembers.map((member) => {
                  const manageable = canManageMember(member, isOwner, isAdmin)
                  const memberName = displayMemberName(member)
                  return (
                    <TableRow
                      aria-label={t("members.openDetail", { name: memberName })}
                      className="cursor-pointer focus-visible:bg-muted/60 focus-visible:outline-none"
                      key={member.user_id}
                      onClick={() => openMemberDetail(member.user_id)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault()
                          openMemberDetail(member.user_id)
                        }
                      }}
                      tabIndex={0}
                    >
                      <TableCell>
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium">
                            {memberName}
                          </div>
                          <div className="truncate text-xs text-muted-foreground">
                            {member.name}
                          </div>
                          {member.email ? (
                            <div className="truncate text-xs text-muted-foreground">
                              {member.email}
                            </div>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {memberRoleLabel(t, member.role)}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {formatTime(member.joined_at)}
                      </TableCell>
                      <TableCell
                        className="text-right"
                        onClick={(event) => event.stopPropagation()}
                        onKeyDown={(event) => event.stopPropagation()}
                      >
                        {canManageMembers && manageable ? (
                          <MemberActions
                            member={member}
                            onEditDisplayName={setEditingMember}
                            onRemove={setRemovingMember}
                            onRole={setRoleMember}
                          />
                        ) : null}
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>
      )}

      <AddMemberDialog
        actorIsOwner={isOwner}
        onOpenChange={setAddOpen}
        open={addOpen}
        workspaceSlug={workspaceSlug}
      />
      <MemberDisplayNameDialog
        member={editingMember}
        onOpenChange={(open) => {
          if (!open) {
            setEditingMember(null)
          }
        }}
        workspaceSlug={workspaceSlug}
      />
      <MemberRoleDialog
        actorIsOwner={isOwner}
        member={roleMember}
        onOpenChange={(open) => {
          if (!open) {
            setRoleMember(null)
          }
        }}
        workspaceSlug={workspaceSlug}
      />
      <RemoveMemberDialog
        member={removingMember}
        onOpenChange={(open) => {
          if (!open) {
            setRemovingMember(null)
          }
        }}
        workspaceSlug={workspaceSlug}
      />
    </div>
  )
}

function MemberActions({
  member,
  onEditDisplayName,
  onRemove,
  onRole,
}: {
  member: WorkspaceMemberRow
  onEditDisplayName: (member: WorkspaceMemberRow) => void
  onRemove: (member: WorkspaceMemberRow) => void
  onRole: (member: WorkspaceMemberRow) => void
}) {
  const { t } = useTranslation()
  const name = displayMemberName(member)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          aria-label={t("members.openActions", { name })}
          size="icon"
          type="button"
          variant="ghost"
        >
          <MoreHorizontalIcon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        <DropdownMenuItem asChild>
          <a href={memberDetailHref(member.user_id)}>
            {t("common.details")}
          </a>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => onEditDisplayName(member)}>
          {t("members.editDisplayName")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => onRole(member)}>
          {t("members.changeRole")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => onRemove(member)}
          variant="destructive"
        >
          {t("members.remove")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function MembersDetailPage({
  credential,
  userRef,
  workspaceSlug,
}: {
  credential?: MeResponse | null
  userRef: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const [editingMember, setEditingMember] = useState<WorkspaceMemberRow | null>(
    null
  )
  const [roleMember, setRoleMember] = useState<WorkspaceMemberRow | null>(null)
  const [removingMember, setRemovingMember] = useState<WorkspaceMemberRow | null>(
    null
  )
  const [bindingExternalId, setBindingExternalId] = useState(false)
  const membersQuery = useQuery({
    enabled: workspaceSlug !== "",
    queryKey: ["workspace", "members", workspaceSlug],
    queryFn: () => listWorkspaceMembers(workspaceSlug),
  })
  const userQuery = useQuery({
    enabled: userRef !== "",
    queryKey: ["workspace", "user", userRef],
    queryFn: () => getWorkspaceUser(userRef),
  })
  const queryClient = useQueryClient()
  const unbindExternalID = useMutation({
    mutationFn: ({ provider, externalID }: { provider: string; externalID: string }) =>
      unbindExternalIDAPI(userRef, provider, externalID),
    onSuccess: () => {
      void userQuery.refetch()
      void queryClient.invalidateQueries({
        queryKey: ["workspace", "user", userRef],
      })
    },
  })
  const canReadAudit = credential?.capabilities?.includes("audit:read") ?? false
  const auditQuery = useQuery({
    enabled: canReadAudit,
    queryKey: ["workspace", "member-audit", userRef],
    queryFn: () => listWorkspaceAudit(100),
  })
  const canReadTokens =
    credential?.capabilities?.includes("token:read") &&
    credential.actor_type !== "tenant_access_token"
  const tokenQuery = useQuery({
    enabled: canReadTokens,
    queryKey: ["resource", "/api/v1/tokens"],
    queryFn: listWorkspaceTokens,
  })
  const members = membersQuery.data ?? []
  const member = members.find(
    (row) => row.user_id === userRef || row.name === userRef
  )
  const user = userQuery.data ?? userFromMember(member)
  const role = credential?.effective_role ?? ""
  const isOwner = role === "owner"
  const isAdmin = role === "admin"
  const canWriteMembers =
    credential?.capabilities?.includes("member:write") ?? false
  const canManageCurrent =
    !!member && canWriteMembers && canManageMember(member, isOwner, isAdmin)
  const auditRows = filterMemberAudit(auditQuery.data ?? [], member, user)
  const tokenRows = filterMemberTokens(tokenQuery.data ?? [], member, user)
  const title = user ? displayUserName(user) : t("members.detailTitle")

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Button asChild size="sm" type="button" variant="ghost">
            <a href="/members">{t("members.backToMembers")}</a>
          </Button>
          <PageHeader title={title} />
        </div>
        {member ? (
          <Badge variant="secondary">{memberRoleLabel(t, member.role)}</Badge>
        ) : null}
      </div>

      {membersQuery.isLoading || userQuery.isLoading ? (
        <div className="space-y-2 rounded-none border bg-card p-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-2/3" />
          <Skeleton className="h-8 w-1/2" />
        </div>
      ) : membersQuery.isError || userQuery.isError ? (
        <div className="border bg-card p-4 text-sm text-destructive">
          {t("common.error")}
        </div>
      ) : !member || !user ? (
        <Alert>
          <AlertDescription>{t("members.detailNotFound")}</AlertDescription>
        </Alert>
      ) : (
        <>
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
            <section className="space-y-4 rounded-none border bg-card p-4">
              <div>
                <h2 className="text-sm font-medium">
                  {t("members.identitySnapshot")}
                </h2>
                <p className="text-xs text-muted-foreground">
                  {t("members.identityHint")}
                </p>
              </div>
              <dl className="grid gap-3 text-sm sm:grid-cols-2">
                <DetailItem label={t("members.displayName")} value={user.display_name || user.name} />
                <DetailItem label={t("members.stableName")} value={user.name} />
                <DetailItem label={t("members.email")} value={user.email ?? "-"} />
                <DetailItem label={t("members.userId")} value={user.id} />
                <DetailItem label={t("resource.role")} value={memberRoleLabel(t, member.role)} />
                <DetailItem label={t("members.joinedAt")} value={formatTime(member.joined_at)} />
                <DetailItem label={t("members.modifiedAt")} value={formatTime(member.modified_at)} />
              </dl>
            </section>

            <section className="space-y-3 rounded-none border bg-card p-4">
              <h2 className="text-sm font-medium">{t("members.management")}</h2>
              {canManageCurrent ? (
                <div className="flex flex-col gap-2">
                  <Button
                    onClick={() => setEditingMember(member)}
                    type="button"
                    variant="outline"
                  >
                    {t("members.editDisplayName")}
                  </Button>
                  <Button
                    onClick={() => setRoleMember(member)}
                    type="button"
                    variant="outline"
                  >
                    {t("members.changeRole")}
                  </Button>
                  <Button
                    onClick={() => setRemovingMember(member)}
                    type="button"
                    variant="destructive"
                  >
                    {t("members.remove")}
                  </Button>
                </div>
              ) : (
                <Alert>
                  <AlertDescription>{t("members.detailReadonlyHint")}</AlertDescription>
                </Alert>
              )}
            </section>
          </div>

          <section className="space-y-3 rounded-none border bg-card p-4">
            <div className="flex items-center justify-between gap-2">
              <h2 className="text-sm font-medium">{t("members.externalIdentities")}</h2>
              {canWriteMembers ? (
                <Button
                  onClick={() => setBindingExternalId(true)}
                  size="sm"
                  variant="outline"
                >
                  {t("members.externalIdBind")}
                </Button>
              ) : null}
            </div>
            {(user.external_ids ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("common.empty")}</p>
            ) : (
              <ul className="space-y-1">
                {(user.external_ids ?? []).map((item) => (
                  <li
                    className="flex items-center justify-between gap-2 text-sm"
                    key={`${item.provider}:${item.external_id}`}
                  >
                    <Badge variant="outline">
                      {item.provider}: {item.external_id}
                    </Badge>
                    {canWriteMembers ? (
                      <Button
                        onClick={() => {
                          if (
                            window.confirm(
                              t("members.externalIdUnbindConfirm", {
                                provider: item.provider,
                                externalId: item.external_id,
                              })
                            )
                          ) {
                            unbindExternalID.mutate({
                              provider: item.provider,
                              externalID: item.external_id,
                            })
                          }
                        }}
                        size="sm"
                        variant="outline"
                      >
                        {t("members.externalIdUnbind")}
                      </Button>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
            {canWriteMembers ? (
              <ExternalIDBindDialog
                onBound={() => {
                  void userQuery.refetch()
                }}
                onOpenChange={setBindingExternalId}
                open={bindingExternalId}
                userRef={userRef}
              />
            ) : null}
          </section>

          <div className="grid gap-4 lg:grid-cols-2">
            <section className="space-y-3 rounded-none border bg-card p-4">
              <div className="flex items-center justify-between gap-2">
                <h2 className="text-sm font-medium">{t("members.tokenSummary")}</h2>
                <Button asChild size="sm" type="button" variant="outline">
                  <a href={`/tokens?user=${encodeURIComponent(user.id)}`}>
                    {t("members.openTokens")}
                  </a>
                </Button>
              </div>
              {tokenQuery.isError ? (
                <p className="text-sm text-muted-foreground">
                  {t("members.tokenSummaryUnavailable")}
                </p>
              ) : (
                <div className="flex flex-wrap gap-2">
                  <Badge variant="secondary">
                    {t("members.tokenActive", { count: tokenRows.active })}
                  </Badge>
                  <Badge variant="secondary">
                    {t("members.tokenRevoked", { count: tokenRows.revoked })}
                  </Badge>
                </div>
              )}
            </section>

            <section className="space-y-3 rounded-none border bg-card p-4">
              <h2 className="text-sm font-medium">{t("members.memberAudit")}</h2>
              {!canReadAudit ? (
                <p className="text-sm text-muted-foreground">
                  {t("members.auditReadonlyHint")}
                </p>
              ) : auditQuery.isError ? (
                <p className="text-sm text-muted-foreground">
                  {t("members.auditUnavailable")}
                </p>
              ) : auditRows.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("common.empty")}</p>
              ) : (
                <div className="space-y-2">
                  {auditRows.slice(0, 5).map((row) => (
                    <div className="text-sm" key={row.id}>
                      <div>{row.action}</div>
                      <div className="text-xs text-muted-foreground">
                        {formatTime(row.created_at)}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>
          </div>
        </>
      )}

      <MemberDisplayNameDialog
        member={editingMember}
        onOpenChange={(open) => {
          if (!open) {
            setEditingMember(null)
          }
        }}
        workspaceSlug={workspaceSlug}
      />
      <MemberRoleDialog
        actorIsOwner={isOwner}
        member={roleMember}
        onOpenChange={(open) => {
          if (!open) {
            setRoleMember(null)
          }
        }}
        workspaceSlug={workspaceSlug}
      />
      <RemoveMemberDialog
        member={removingMember}
        onOpenChange={(open) => {
          if (!open) {
            setRemovingMember(null)
          }
        }}
        workspaceSlug={workspaceSlug}
      />
    </div>
  )
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="break-all">{value}</dd>
    </div>
  )
}

function AddMemberDialog({
  actorIsOwner,
  onOpenChange,
  open,
  workspaceSlug,
}: {
  actorIsOwner: boolean
  onOpenChange: (open: boolean) => void
  open: boolean
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [source, setSource] = useState<"existing" | "new">("existing")
  const [userRef, setUserRef] = useState("")
  const [name, setName] = useState("")
  const [displayName, setDisplayName] = useState("")
  const [email, setEmail] = useState("")
  const [role, setRole] = useState<MemberRole>("member")
  const [error, setError] = useState<string | null>(null)
  const mutation = useMutation({
    mutationFn: (input: AddWorkspaceMemberInput) =>
      addWorkspaceMember(workspaceSlug, input),
    onSuccess: async () => {
      await invalidateMembers(queryClient, workspaceSlug)
      handleOpenChange(false)
    },
  })

  function resetForm() {
    setSource("existing")
    setUserRef("")
    setName("")
    setDisplayName("")
    setEmail("")
    setRole("member")
    setError(null)
    mutation.reset()
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetForm()
    }
    onOpenChange(nextOpen)
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    const normalizedRole = roleAllowedForActor(role, actorIsOwner)
      ? role
      : "member"
    if (source === "existing") {
      const user = userRef.trim()
      if (!user) {
        setError(t("members.userRequired"))
        return
      }
      void mutation.mutateAsync({ user, role: normalizedRole }).catch((err) => {
        setError(errorMessage(err instanceof Error ? err : null, t("common.error")))
      })
      return
    }
    const normalizedName = name.trim()
    if (!normalizedName) {
      setError(t("members.nameRequired"))
      return
    }
    void mutation
      .mutateAsync({
        new_user: {
          ...(displayName.trim()
            ? { display_name: displayName.trim() }
            : {}),
          ...(email.trim() ? { email: email.trim() } : {}),
          name: normalizedName,
        },
        role: normalizedRole,
      })
      .catch((err) => {
        setError(errorMessage(err instanceof Error ? err : null, t("common.error")))
      })
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="sm:max-w-md">
        <form className="space-y-4" onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{t("members.add")}</DialogTitle>
            <DialogDescription>{t("members.addDescription")}</DialogDescription>
          </DialogHeader>
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="member-source">{t("members.source")}</Label>
            <Select
              value={source}
              onValueChange={(value) => setSource(value as "existing" | "new")}
            >
              <SelectTrigger aria-label={t("members.source")} id="member-source">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="existing">{t("members.sourceExisting")}</SelectItem>
                <SelectItem value="new">{t("members.sourceNew")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {source === "existing" ? (
            <div className="space-y-2">
              <Label htmlFor="member-user-ref">{t("members.user")}</Label>
              <Input
                id="member-user-ref"
                onChange={(event) => setUserRef(event.target.value)}
                value={userRef}
              />
            </div>
          ) : (
            <>
              <div className="space-y-2">
                <Label htmlFor="member-new-name">{t("members.stableName")}</Label>
                <Input
                  id="member-new-name"
                  onChange={(event) => setName(event.target.value)}
                  value={name}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="member-new-display-name">
                  {t("members.displayName")}
                </Label>
                <Input
                  id="member-new-display-name"
                  onChange={(event) => setDisplayName(event.target.value)}
                  value={displayName}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="member-new-email">{t("members.email")}</Label>
                <Input
                  id="member-new-email"
                  onChange={(event) => setEmail(event.target.value)}
                  type="email"
                  value={email}
                />
              </div>
            </>
          )}
          <RoleSelect
            actorIsOwner={actorIsOwner}
            onValueChange={setRole}
            value={role}
          />
          <DialogFooter>
            <Button
              onClick={() => handleOpenChange(false)}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button disabled={mutation.isPending} type="submit">
              {t("members.addSubmit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function MemberDisplayNameDialog({
  member,
  onOpenChange,
  workspaceSlug,
}: {
  member: WorkspaceMemberRow | null
  onOpenChange: (open: boolean) => void
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const open = member !== null
  const draftSource = member
    ? `${member.user_id}\u0000${member.display_name ?? ""}`
    : ""
  const [draftState, setDraftState] = useState({ source: "", value: "" })
  const value =
    draftState.source === draftSource ? draftState.value : member?.display_name ?? ""
  const mutation = useMutation({
    mutationFn: async (displayName: string) => {
      if (!member) {
        return null
      }
      return modifyWorkspaceMember(workspaceSlug, member.user_id, {
        display_name: displayName,
      })
    },
    onSuccess: async () => {
      await invalidateMembers(queryClient, workspaceSlug)
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
          <DialogTitle>{t("members.editDisplayName")}</DialogTitle>
          <DialogDescription>
            {member
              ? t("members.editDisplayNameDescription", { name: member.name })
              : ""}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="member-display-name">
            {t("members.displayName")}
          </Label>
          <Input
            id="member-display-name"
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

function MemberRoleDialog({
  actorIsOwner,
  member,
  onOpenChange,
  workspaceSlug,
}: {
  actorIsOwner: boolean
  member: WorkspaceMemberRow | null
  onOpenChange: (open: boolean) => void
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [role, setRole] = useState<MemberRole>("member")
  const [confirming, setConfirming] = useState(false)
  const open = member !== null
  const mutation = useMutation({
    mutationFn: async (nextRole: string) => {
      if (!member) {
        return null
      }
      return modifyWorkspaceMember(workspaceSlug, member.user_id, {
        role: nextRole,
      })
    },
    onSuccess: async () => {
      await invalidateMembers(queryClient, workspaceSlug)
      handleOpenChange(false)
    },
  })

  function handleOpenChange(nextOpen: boolean) {
    if (nextOpen && member) {
      setRole(member.role as MemberRole)
    }
    if (!nextOpen) {
      setRole("member")
      setConfirming(false)
      mutation.reset()
    }
    onOpenChange(nextOpen)
  }

  function submitRole() {
    if (!member || role === member.role) {
      handleOpenChange(false)
      return
    }
    if (member.role === "owner" || role === "owner") {
      setConfirming(true)
      return
    }
    void mutation.mutateAsync(role)
  }

  return (
    <>
      <Dialog onOpenChange={handleOpenChange} open={open}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("members.changeRole")}</DialogTitle>
            <DialogDescription>
              {member ? t("members.changeRoleDescription", { name: displayMemberName(member) }) : ""}
            </DialogDescription>
          </DialogHeader>
          <RoleSelect
            actorIsOwner={actorIsOwner}
            onValueChange={(value) => setRole(value)}
            value={role}
          />
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
            <Button disabled={mutation.isPending} onClick={submitRole} type="button">
              {role === "owner" || member?.role === "owner"
                ? t("members.continue")
                : t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AlertDialog open={confirming} onOpenChange={setConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("members.confirmOwnerRoleTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("members.confirmOwnerRoleDescription")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={mutation.isPending}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={mutation.isPending}
              onClick={(event) => {
                event.preventDefault()
                void mutation.mutateAsync(role).then(() => setConfirming(false))
              }}
            >
              {t("members.continue")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function RemoveMemberDialog({
  member,
  onOpenChange,
  workspaceSlug,
}: {
  member: WorkspaceMemberRow | null
  onOpenChange: (open: boolean) => void
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: async () => {
      if (!member) {
        return null
      }
      return removeWorkspaceMember(workspaceSlug, member.user_id)
    },
    onSuccess: async () => {
      await invalidateMembers(queryClient, workspaceSlug)
      handleOpenChange(false)
    },
  })

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      mutation.reset()
    }
    onOpenChange(nextOpen)
  }

  return (
    <AlertDialog onOpenChange={handleOpenChange} open={member !== null}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("members.removeTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {member
              ? t("members.removeDescription", { name: displayMemberName(member) })
              : ""}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {mutation.isError ? (
          <div className="text-sm text-destructive">
            {errorMessage(
              mutation.error instanceof Error ? mutation.error : null,
              t("common.error")
            )}
          </div>
        ) : null}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={mutation.isPending}>
            {t("common.cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={mutation.isPending}
            onClick={(event) => {
              event.preventDefault()
              void mutation.mutateAsync()
            }}
            variant="destructive"
          >
            {t("members.removeSubmit")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function RoleSelect({
  actorIsOwner,
  onValueChange,
  value,
}: {
  actorIsOwner: boolean
  onValueChange: (role: MemberRole) => void
  value: MemberRole
}) {
  const { t } = useTranslation()
  const options = actorIsOwner
    ? ROLE_OPTIONS
    : ROLE_OPTIONS.filter((item) => item !== "owner")
  return (
    <div className="space-y-2">
      <Label htmlFor="member-role">{t("resource.role")}</Label>
      <Select value={value} onValueChange={(next) => onValueChange(next as MemberRole)}>
        <SelectTrigger aria-label={t("resource.role")} id="member-role">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((item) => (
            <SelectItem key={item} value={item}>
              {memberRoleLabel(t, item)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {!actorIsOwner ? (
        <p className="text-xs text-muted-foreground">
          {t("members.ownerOnlyHint")}
        </p>
      ) : null}
    </div>
  )
}

function canManageMember(
  member: WorkspaceMemberRow,
  isOwner: boolean,
  isAdmin: boolean
): boolean {
  if (isOwner) {
    return true
  }
  return isAdmin && member.role !== "owner"
}

function roleAllowedForActor(role: string, actorIsOwner: boolean) {
  return actorIsOwner || role !== "owner"
}

function memberDetailHref(userID: string) {
  return `/members/${encodeURIComponent(userID)}`
}

function openMemberDetail(userID: string) {
  window.history.pushState({}, "", memberDetailHref(userID))
  window.dispatchEvent(new PopStateEvent("popstate"))
}

function memberRoleLabel(t: Translate, role: string) {
  switch (role) {
    case "owner":
      return t("members.roles.owner")
    case "admin":
      return t("members.roles.admin")
    case "member":
      return t("members.roles.member")
    case "viewer":
      return t("members.roles.viewer")
    default:
      return role
  }
}

function memberCounts(members: WorkspaceMemberRow[]) {
  return members.reduce(
    (acc, member) => {
      if (member.role in acc) {
        acc[member.role as MemberRole] += 1
      }
      return acc
    },
    { admin: 0, member: 0, owner: 0, viewer: 0 }
  )
}

async function invalidateMembers(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceSlug: string
) {
  await queryClient.invalidateQueries({
    queryKey: ["workspace", "members", workspaceSlug],
  })
}

function displayMemberName(member: WorkspaceMemberRow): string {
  return member.display_name?.trim() || member.name
}

function displayUserName(user: WorkspaceUserRow): string {
  return user.display_name?.trim() || user.name
}

function userFromMember(
  member: WorkspaceMemberRow | undefined
): WorkspaceUserRow | undefined {
  if (!member) {
    return undefined
  }
  return {
    active: true,
    created_at: member.joined_at,
    display_name: member.display_name,
    email: member.email,
    external_ids: [],
    id: member.user_id,
    modified_at: member.modified_at,
    name: member.name,
  }
}

function filterMemberAudit(
  rows: WorkspaceAuditRow[],
  member: WorkspaceMemberRow | undefined,
  user: WorkspaceUserRow | undefined
) {
  const ids = new Set(
    [member?.user_id, member?.name, user?.id, user?.name].filter(Boolean)
  )
  return rows.filter((row) => row.target_type === "member" && ids.has(row.target_id))
}

function filterMemberTokens(
  rows: WorkspaceTokenRow[],
  member: WorkspaceMemberRow | undefined,
  user: WorkspaceUserRow | undefined
) {
  const ids = new Set(
    [member?.user_id, member?.name, user?.id, user?.name].filter(Boolean)
  )
  let active = 0
  let revoked = 0
  for (const row of rows) {
    if (!ids.has(row.user.id) && !ids.has(row.user.name)) {
      continue
    }
    if (row.revoked_at) {
      revoked += 1
    } else {
      active += 1
    }
  }
  return { active, revoked }
}

function formatTime(unix: number): string {
  return new Date(unix * 1000).toLocaleString()
}

function errorMessage(error: Error | null, fallback: string): string {
  if (error instanceof ApiError) {
    return `${fallback}: ${error.code}`
  }
  return fallback
}
