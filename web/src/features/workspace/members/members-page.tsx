import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PencilIcon } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

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
import { PageHeader } from "@/pages/OverviewPage"

import {
  listWorkspaceMembers,
  modifyWorkspaceUserDisplayName,
  type WorkspaceMemberRow,
} from "./members-api"

type MembersPageProps = {
  workspaceSlug: string
}

export function MembersPage({ workspaceSlug }: MembersPageProps) {
  const { t } = useTranslation()
  const [editingMember, setEditingMember] = useState<WorkspaceMemberRow | null>(
    null
  )
  const query = useQuery({
    enabled: workspaceSlug !== "",
    queryKey: ["workspace", "members", workspaceSlug],
    queryFn: () => listWorkspaceMembers(workspaceSlug),
  })

  return (
    <div className="space-y-4">
      <PageHeader title={t("page.members")} />
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
              {(query.data ?? []).length === 0 ? (
                <TableRow>
                  <TableCell
                    className="h-24 text-center text-muted-foreground"
                    colSpan={4}
                  >
                    {t("common.empty")}
                  </TableCell>
                </TableRow>
              ) : (
                (query.data ?? []).map((member) => (
                  <TableRow key={member.user_id}>
                    <TableCell>
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium">
                          {displayMemberName(member)}
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
                      <Badge variant="secondary">{member.role}</Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {formatTime(member.joined_at)}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        aria-label={t("members.editDisplayNameAria", {
                          name: displayMemberName(member),
                        })}
                        onClick={() => setEditingMember(member)}
                        size="icon"
                        type="button"
                        variant="ghost"
                      >
                        <PencilIcon />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
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
    </div>
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
      return modifyWorkspaceUserDisplayName(member.user_id, displayName)
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["workspace", "members", workspaceSlug],
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

function displayMemberName(member: WorkspaceMemberRow): string {
  return member.display_name?.trim() || member.name
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
