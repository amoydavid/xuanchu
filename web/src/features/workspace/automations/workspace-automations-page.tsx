import { useQueryClient } from "@tanstack/react-query"
import { MoreHorizontal } from "lucide-react"
import { useState } from "react"
import { Link } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { DestructiveConfirmDialog } from "@/features/workspace/project-workbench/shared/destructive-confirm-dialog"
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"
import {
  useDeleteWorkspaceAutomationRule,
  useReplayWorkspaceAutomationDelivery,
  useToggleWorkspaceAutomationRule,
  useWorkspaceAutomationDeliveries,
  useWorkspaceAutomationProviderConfig,
  useWorkspaceAutomationRules,
  type WorkspaceAutomationDelivery,
  type WorkspaceAutomationRule,
} from "@/features/workspace/automations/workspace-automations-api"
import { canWriteAutomation } from "@/features/workspace/automations/workspace-automation-permissions"
import { useMe } from "@/features/workspace/session/useMe"
import { cn } from "@/lib/utils"
import { relativeTime } from "@/lib/time"

import {
  deliveryStatusLabel,
  DeliveryStatusDot,
  RuleStatusDot,
} from "@/features/workspace/automations/shared/automation-status"
import { summarizeTrigger, summarizeTriggerFull } from "@/features/workspace/automations/shared/automation-trigger-summary"
import { ProviderConfigDialog } from "@/features/workspace/automations/shared/provider-config-dialog"
import {
  WorkspaceAutomationRuleDialog,
} from "@/features/workspace/automations/shared/workspace-automation-rule-dialog"
import { WorkspaceAutomationDeliveryDetail } from "@/features/workspace/automations/shared/workspace-automation-delivery-detail"

type Tab = "rules" | "deliveries"

// WorkspaceAutomationsConsole 是「管理 → 自动化」入口的控制台。
// 桌面：shadcn <Table> 规则/运行记录 + 新建/编辑 Dialog + Provider Dialog + 运行详情抽屉。
// 移动：card list + 全高 Sheet（通过 md:hidden 响应式切换）。
// spec §16 ASCII 原型：状态点 + 名称 + 触发器摘要 + 最近运行 + 操作 ⋯。
export function WorkspaceAutomationsConsole({
  workspaceSlug,
}: {
  workspaceSlug?: string
}) {
  const me = useMe()
  const canWrite = canWriteAutomation({
    role: me.data?.effective_role,
    actorType: me.data?.actor_type,
    tokenType: me.data?.token.type,
    scopes: me.data?.token.scopes,
  })

  const [tab, setTab] = useState<Tab>("rules")
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<WorkspaceAutomationRule | null>(null)
  const [providerOpen, setProviderOpen] = useState(false)
  const [detailDeliveryId, setDetailDeliveryId] = useState<string | null>(null)

  const provider = useWorkspaceAutomationProviderConfig()
  const providerComplete = provider.data?.complete ?? false

  return (
    <div className="space-y-4">
      <header className="space-y-1">
        <div className="flex items-center justify-between gap-2">
          <h1 className="text-xl font-semibold">工作空间自动化</h1>
          {canWrite ? (
            <Button type="button" size="sm" onClick={() => setCreating(true)}>
              + 新建自动化
            </Button>
          ) : null}
        </div>
        <p className="text-sm text-muted-foreground">
          监听整个工作空间。单项目规则请进入对应项目的「自动化」。
        </p>
      </header>

      {!providerComplete ? (
        <div
          role="status"
          className="flex items-center justify-between gap-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/40 dark:text-amber-100"
        >
          <span>工作空间 Agent Provider 配置不完整，规则不会成功投递。</span>
          {canWrite ? (
            <Button type="button" variant="outline" size="sm" onClick={() => setProviderOpen(true)}>
              查看配置
            </Button>
          ) : null}
        </div>
      ) : null}

      <div className="flex items-center gap-2 border-b">
        <TabButton active={tab === "rules"} onClick={() => setTab("rules")}>
          规则
        </TabButton>
        <TabButton active={tab === "deliveries"} onClick={() => setTab("deliveries")}>
          运行记录
        </TabButton>
      </div>

      {tab === "rules" ? (
        <RulesTab
          canWrite={canWrite}
          workspaceSlug={workspaceSlug}
          onEdit={(rule) => setEditing(rule)}
          onCreating={() => setCreating(true)}
        />
      ) : (
        <DeliveriesTab
          canWrite={canWrite}
          workspaceSlug={workspaceSlug}
          onSelect={(id) => setDetailDeliveryId(id)}
        />
      )}

      <WorkspaceAutomationRuleDialog
        key={editing?.id ?? (creating ? "creating-open" : "creating-closed")}
        open={creating || !!editing}
        onOpenChange={(o) => {
          if (!o) {
            setCreating(false)
            setEditing(null)
          }
        }}
        onSaved={() => {
          /* query invalidated inside hook */
        }}
        initial={editing ?? undefined}
        onOpenProviderConfig={() => setProviderOpen(true)}
        canEdit={canWrite}
      />

      <ProviderConfigDialog open={providerOpen} onOpenChange={setProviderOpen} canEdit={canWrite} />

      <WorkspaceAutomationDeliveryDetail
        open={detailDeliveryId !== null}
        onOpenChange={(o) => !o && setDetailDeliveryId(null)}
        deliveryID={detailDeliveryId}
        workspaceSlug={workspaceSlug}
      />
    </div>
  )
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "-mb-px border-b-2 px-3 py-2 text-sm transition-colors",
        active
          ? "border-primary text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground"
      )}
    >
      {children}
    </button>
  )
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function RulesTab({
  canWrite,
  workspaceSlug,
  onEdit,
  onCreating,
}: {
  canWrite: boolean
  workspaceSlug?: string
  onEdit: (rule: WorkspaceAutomationRule) => void
  onCreating: () => void
}) {
  void workspaceSlug
  void onCreating
  const feedback = useEditFeedback()
  const queryClient = useQueryClient()
  const rules = useWorkspaceAutomationRules(false)
  const toggle = useToggleWorkspaceAutomationRule()
  const deleteMutation = useDeleteWorkspaceAutomationRule()
  const [confirmDelete, setConfirmDelete] = useState<WorkspaceAutomationRule | null>(null)

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["workspace-automations", "rules"] })

  if (rules.isLoading) {
    return <div className="text-sm text-muted-foreground">加载中...</div>
  }
  if (rules.isError) {
    return (
      <div className="text-sm text-destructive">
        加载失败：{errorMessage(rules.error)}
      </div>
    )
  }
  const rows = rules.data ?? []
  if (rows.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
        还没有工作空间自动化。创建规则，在项目创建或定时时让 Agent 处理工作空间任务。
      </div>
    )
  }

  return (
    <>
      {/* 桌面 shadcn Table */}
      <Table containerClassName="hidden md:block">
        <TableHeader>
          <TableRow>
            <TableHead className="w-[60px]">状态</TableHead>
            <TableHead>名称</TableHead>
            <TableHead className="w-[180px]">触发器</TableHead>
            <TableHead>指令摘要</TableHead>
            <TableHead className="w-[120px]">最近运行</TableHead>
            <TableHead className="w-[40px] text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((rule) => (
            <TableRow key={rule.id}>
              <TableCell>
                <div className="flex items-center gap-2">
                  <RuleStatusDot enabled={rule.enabled} />
                  <span className="sr-only">{rule.enabled ? "启用" : "停用"}</span>
                </div>
              </TableCell>
              <TableCell>
                <button
                  type="button"
                  className="truncate font-medium hover:underline"
                  onClick={() => onEdit(rule)}
                >
                  {rule.name}
                </button>
              </TableCell>
              <TableCell className="min-w-0" title={summarizeTriggerFull(rule.trigger_type, rule.trigger_config)}>
                <span className="block truncate">{summarizeTrigger(rule.trigger_type, rule.trigger_config)}</span>
              </TableCell>
              <TableCell className="min-w-0">
                <span
                  className="block truncate text-muted-foreground"
                  title={rule.instruction_template}
                >
                  {rule.instruction_template}
                </span>
              </TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">
                {rule.last_delivery ? relativeTime(rule.last_delivery.created_at) : "从未运行"}
              </TableCell>
              <TableCell className="text-right">
                <RuleActions
                  rule={rule}
                  canWrite={canWrite}
                  togglePending={toggle.isPending && toggle.variables?.ruleId === rule.id}
                  onToggle={() =>
                    toggle.mutate(
                      { ruleId: rule.id, enable: !rule.enabled },
                      {
                        onSuccess: () => invalidate(),
                        onError: (err) => feedback.failure("操作失败", errorMessage(err)),
                      }
                    )
                  }
                  onEdit={() => onEdit(rule)}
                  onDelete={() => setConfirmDelete(rule)}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {/* 移动端 card list */}
      <div className="space-y-2 md:hidden">
        {rows.map((rule) => (
          <div key={rule.id} className="rounded-md border p-3">
            <div className="flex items-start justify-between gap-2">
              <div className="flex items-center gap-2">
                <RuleStatusDot enabled={rule.enabled} />
                <button
                  type="button"
                  className="font-medium hover:underline"
                  onClick={() => onEdit(rule)}
                >
                  {rule.name}
                </button>
              </div>
              <RuleActions
                rule={rule}
                canWrite={canWrite}
                togglePending={toggle.isPending && toggle.variables?.ruleId === rule.id}
                onToggle={() =>
                  toggle.mutate(
                    { ruleId: rule.id, enable: !rule.enabled },
                    {
                      onSuccess: () => invalidate(),
                      onError: (err) => feedback.failure("操作失败", errorMessage(err)),
                    }
                  )
                }
                onEdit={() => onEdit(rule)}
                onDelete={() => setConfirmDelete(rule)}
              />
            </div>
            <div className="mt-1 truncate text-xs text-muted-foreground">
              {summarizeTrigger(rule.trigger_type, rule.trigger_config)}
            </div>
            <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">
              {rule.instruction_template}
            </div>
            <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
              <span>{rule.last_delivery ? relativeTime(rule.last_delivery.created_at) : "从未运行"}</span>
              <span>{rule.enabled ? "启用" : "停用"}</span>
            </div>
          </div>
        ))}
      </div>

      <DestructiveConfirmDialog
        open={!!confirmDelete}
        onOpenChange={(o) => !o && setConfirmDelete(null)}
        title="删除工作空间自动化"
        description={`确认删除规则「${confirmDelete?.name ?? ""}」？此操作不可撤销。`}
        confirmLabel="删除"
        pending={deleteMutation.isPending}
        onConfirm={() => {
          if (!confirmDelete) return
          deleteMutation.mutate(confirmDelete.id, {
            onSuccess: () => {
              setConfirmDelete(null)
              invalidate()
              feedback.success("规则已删除")
            },
            onError: (err) => feedback.failure("删除失败", errorMessage(err)),
          })
        }}
      />
    </>
  )
}

function RuleActions({
  rule,
  canWrite,
  togglePending,
  onToggle,
  onEdit,
  onDelete,
}: {
  rule: WorkspaceAutomationRule
  canWrite: boolean
  togglePending: boolean
  onToggle: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  if (!canWrite) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="h-8 w-8 p-0" aria-label="操作">
          <MoreHorizontal className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={onEdit}>编辑</DropdownMenuItem>
        <DropdownMenuItem onClick={onToggle} disabled={togglePending}>
          {rule.enabled ? "停用" : "启用"}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="text-destructive" onClick={onDelete}>
          删除
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function DeliveriesTab({
  canWrite,
  workspaceSlug,
  onSelect,
}: {
  canWrite: boolean
  workspaceSlug?: string
  onSelect: (deliveryID: string) => void
}) {
  const feedback = useEditFeedback()
  void canWrite
  const queryClient = useQueryClient()
  const [filter, setFilter] = useState("")
  const [statusFilter, setStatusFilter] = useState("")
  const [triggerFilter, setTriggerFilter] = useState("")
  const deliveries = useWorkspaceAutomationDeliveries({
    q: filter || undefined,
    status: (statusFilter as WorkspaceAutomationDelivery["status"] | "") || undefined,
    trigger_type: (triggerFilter as "schedule" | "event" | "") || undefined,
    limit: 50,
  })
  const replay = useReplayWorkspaceAutomationDelivery()
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["workspace-automations", "deliveries"] })

  if (deliveries.isLoading) {
    return <div className="text-sm text-muted-foreground">加载中...</div>
  }
  if (deliveries.isError) {
    return (
      <div className="text-sm text-destructive">
        加载失败：{errorMessage(deliveries.error)}
      </div>
    )
  }
  const rows = deliveries.data ?? []
  if (rows.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
        规则触发后，Agent 调用会显示在这里。
      </div>
    )
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <input
          aria-label="搜索 ID"
          className="h-8 rounded-md border border-input bg-transparent px-2 text-xs"
          placeholder="搜索 delivery/event ID"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <select
          aria-label="状态过滤"
          className="h-8 rounded-md border border-input bg-transparent px-2 text-xs"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
        >
          <option value="">状态：全部</option>
          <option value="succeeded">成功</option>
          <option value="retry_wait">等待重试</option>
          <option value="dead_lettered">失败</option>
          <option value="queued">排队中</option>
          <option value="delivering">投递中</option>
        </select>
        <select
          aria-label="触发过滤"
          className="h-8 rounded-md border border-input bg-transparent px-2 text-xs"
          value={triggerFilter}
          onChange={(e) => setTriggerFilter(e.target.value)}
        >
          <option value="">触发：全部</option>
          <option value="event">event</option>
          <option value="schedule">schedule</option>
        </select>
      </div>

      {/* 桌面 Table */}
      <Table containerClassName="hidden md:block">
        <TableHeader>
          <TableRow>
            <TableHead className="w-[40px]">状态</TableHead>
            <TableHead className="w-[140px]">规则</TableHead>
            <TableHead>Project</TableHead>
            <TableHead className="w-[140px]">触发</TableHead>
            <TableHead className="w-[60px]">HTTP</TableHead>
            <TableHead className="w-[100px]">时间</TableHead>
            <TableHead className="w-[40px] text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((d) => (
            <DeliveryTableRow
              key={d.id}
              delivery={d}
              workspaceSlug={workspaceSlug}
              onSelect={() => onSelect(d.id)}
              onReplay={() =>
                replay.mutate(d.id, {
                  onSuccess: () => {
                    invalidate()
                    feedback.success("已重新投递")
                  },
                  onError: (err) => feedback.failure("重新投递失败", errorMessage(err)),
                })
              }
              replayPending={replay.isPending && replay.variables === d.id}
            />
          ))}
        </TableBody>
      </Table>

      {/* 移动端 card list */}
      <div className="space-y-2 md:hidden">
        {rows.map((d) => (
          <DeliveryCard
            key={d.id}
            delivery={d}
            workspaceSlug={workspaceSlug}
            onSelect={() => onSelect(d.id)}
            onReplay={() =>
              replay.mutate(d.id, {
                onSuccess: () => {
                  invalidate()
                  feedback.success("已重新投递")
                },
                onError: (err) => feedback.failure("重新投递失败", errorMessage(err)),
              })
            }
            replayPending={replay.isPending && replay.variables === d.id}
          />
        ))}
      </div>
    </>
  )
}

function DeliveryTableRow({
  delivery,
  workspaceSlug,
  onSelect,
  onReplay,
  replayPending,
}: {
  delivery: WorkspaceAutomationDelivery
  workspaceSlug?: string
  onSelect: () => void
  onReplay: () => void
  replayPending: boolean
}) {
  return (
    <TableRow>
      <TableCell>
        <div className="flex items-center gap-2">
          <DeliveryStatusDot status={delivery.status} />
          <span className="sr-only">{deliveryStatusLabel(delivery.status)}</span>
        </div>
      </TableCell>
      <TableCell className="font-mono text-xs">{delivery.rule_id.slice(0, 8)}</TableCell>
      <TableCell>
        {delivery.project ? (
          workspaceSlug ? (
            <Link
              to="/workspaces/$workspaceSlug/projects/$projectSlug"
              params={{ workspaceSlug, projectSlug: delivery.project.slug }}
              className="text-primary hover:underline"
            >
              {delivery.project.slug}
            </Link>
          ) : (
            <span>{delivery.project.slug}</span>
          )
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell className="text-xs">
        {delivery.trigger_type === "event" ? delivery.event_type || "event" : "schedule"}
      </TableCell>
      <TableCell className="font-mono text-xs">
        {delivery.response_status_code ?? "—"}
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {relativeTime(delivery.created_at)}
      </TableCell>
      <TableCell className="text-right">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" className="h-8 w-8 p-0" aria-label="操作">
              <MoreHorizontal className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onSelect}>详情</DropdownMenuItem>
            <DropdownMenuItem onClick={onReplay} disabled={replayPending}>
              {replayPending ? "重新投递中..." : "重新投递"}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  )
}

function DeliveryCard({
  delivery,
  workspaceSlug,
  onSelect,
  onReplay,
  replayPending,
}: {
  delivery: WorkspaceAutomationDelivery
  workspaceSlug?: string
  onSelect: () => void
  onReplay: () => void
  replayPending: boolean
}) {
  return (
    <div className="rounded-md border p-3">
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2">
          <DeliveryStatusDot status={delivery.status} />
          <span className="text-sm font-medium">{deliveryStatusLabel(delivery.status)}</span>
        </div>
        <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={onSelect} aria-label="详情">
          <MoreHorizontal className="size-4" />
        </Button>
      </div>
      <div className="mt-1 font-mono text-xs text-muted-foreground">{delivery.id.slice(0, 12)}</div>
      <div className="mt-1 text-xs text-muted-foreground">
        {delivery.trigger_type === "event" ? delivery.event_type || "event" : "schedule"} · HTTP{" "}
        {delivery.response_status_code ?? "—"}
      </div>
      <div className="mt-1 text-xs text-muted-foreground">
        {delivery.project ? (
          workspaceSlug ? (
            <Link
              to="/workspaces/$workspaceSlug/projects/$projectSlug"
              params={{ workspaceSlug, projectSlug: delivery.project.slug }}
              className="text-primary hover:underline"
            >
              {delivery.project.slug}
            </Link>
          ) : (
            <span>{delivery.project.slug}</span>
          )
        ) : (
          <span>—</span>
        )}{" "}
        · {relativeTime(delivery.created_at)}
      </div>
      <div className="mt-2 flex justify-end">
        <Button variant="outline" size="sm" onClick={onReplay} disabled={replayPending}>
          {replayPending ? "重新投递中..." : "重新投递"}
        </Button>
      </div>
    </div>
  )
}
