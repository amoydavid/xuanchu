import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { workspaceApiGet, workspaceApiPost } from "@/features/workspace/session/workspace-api"
import { ApiError } from "@/lib/api"

type WorkspaceRow = {
  id: string
  slug: string
  name: string
  description?: string
  archived: boolean
  archived_at?: number | null
}

export function WorkspaceConsole({ canWrite }: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery<WorkspaceRow[]>({
    queryKey: ["workspaces"],
    queryFn: () => workspaceApiGet<WorkspaceRow[]>("/api/v1/workspaces"),
  })
  const archive = useMutation({
    mutationFn: (slug: string) =>
      workspaceApiPost<void>(`/api/v1/workspaces/${encodeURIComponent(slug)}/archive`, {}),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] })
    },
  })

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold tracking-normal">
          {t("page.workspaces")}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("workspacesConsole.subtitle")}
        </p>
      </div>
      {query.isError ? (
        <div className="border bg-card p-4 text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : t("common.error")}
        </div>
      ) : query.isLoading ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("common.loading")}
        </div>
      ) : (query.data ?? []).length === 0 ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("common.empty")}
        </div>
      ) : (
        <div className="border bg-card">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("resource.slug")}</TableHead>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("common.status")}</TableHead>
                <TableHead className="text-right">{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(query.data ?? []).map((ws) => (
                <TableRow key={ws.id}>
                  <TableCell>
                    <code>{ws.slug}</code>
                  </TableCell>
                  <TableCell>{ws.name}</TableCell>
                  <TableCell>
                    {ws.archived ? (
                      <Badge variant="secondary">
                        {t("admin.workspace.statusArchived")}
                      </Badge>
                    ) : (
                      <Badge variant="outline">
                        {t("admin.workspace.statusActive")}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    {ws.archived ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button disabled size="sm" variant="outline">
                            {t("workspacesConsole.restore")}
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>
                          {t("workspacesConsole.restoreUnavailable")}
                        </TooltipContent>
                      </Tooltip>
                    ) : canWrite ? (
                      <Button
                        onClick={() => {
                          if (
                            window.confirm(t("workspacesConsole.archiveConfirm"))
                          ) {
                            archive.mutate(ws.slug)
                          }
                        }}
                        size="sm"
                        variant="outline"
                      >
                        {t("workspacesConsole.archive")}
                      </Button>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
