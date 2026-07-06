import { useQuery } from "@tanstack/react-query"
import { CheckIcon, CopyIcon, PlugIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api"

import {
  deriveTokenStatus,
  getTokenMcpConfig,
  getTenantTokenMcpConfig,
  type TenantAccessTokenRow,
  type TokenMcpConfig,
  type TokenRow,
  tokenMcpConfigQueryKey,
} from "./token-api"

type TokenMcpConfigDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: TokenRow | TenantAccessTokenRow
  /** mode = "api" 走普通 token endpoint；mode = "tenant" 走租户 token endpoint。 */
  mode: "api" | "tenant"
}

/**
 * TokenMcpConfigDialog 按需 reveal 当前 token 的完整明文与 HTTP MCP 配置片段。
 *
 * 设计要点（见 spec §6.2）：
 * - 仅在弹窗打开时拉取 mcp-config，raw token 不进入列表 cache。
 * - 复制动作只写剪贴板，不持久化到 storage。
 * - agent + impersonate 时额外展示可选 X-Xuanchu-As header，但不默认填用户。
 */
export function TokenMcpConfigDialog({
  mode,
  onOpenChange,
  open,
  token,
}: TokenMcpConfigDialogProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState<string | null>(null)

  const query = useQuery<TokenMcpConfig, ApiError>({
    enabled: open,
    gcTime: 0,
    queryFn: () =>
      mode === "tenant"
        ? getTenantTokenMcpConfig(token.id)
        : getTokenMcpConfig(token.id),
    queryKey: tokenMcpConfigQueryKey(token.type, token.id),
    staleTime: 0,
  })

  const endpoint = useMemo(() => {
    const path = query.data?.endpoint_path ?? "/mcp"
    try {
      return new URL(path, window.location.origin).toString()
    } catch {
      return `${window.location.origin}${path}`
    }
  }, [query.data?.endpoint_path])

  const tokenType = query.data?.token_type ?? token.type
  const scopes = query.data?.scopes ?? token.scopes ?? []
  const canImpersonate = tokenType === "agent" && scopes.includes("impersonate")
  const status = deriveTokenStatus({
    expires_at: query.data?.expires_at ?? token.expires_at,
    revoked_at: query.data?.revoked_at ?? token.revoked_at,
  })

  const configJson = useMemo(() => {
    if (!query.data) return ""
    return JSON.stringify(
      {
        url: endpoint,
        headers: {
          Authorization: `Bearer ${query.data.token}`,
        },
      },
      null,
      2
    )
  }, [endpoint, query.data])

  async function copy(label: string, value: string) {
    try {
      await navigator.clipboard?.writeText(value)
      setCopied(label)
    } catch {
      // 剪贴板不可用时静默失败，用户可手动选中复制
    }
  }

  function close() {
    setCopied(null)
    onOpenChange(false)
  }

  return (
    <Dialog onOpenChange={(next) => (next ? null : close())} open={open}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <PlugIcon className="size-4" />
            {t("token.mcpConfigTitle", { name: token.name })}
          </DialogTitle>
          <DialogDescription className="font-mono text-xs text-muted-foreground">
            {tokenType} · {t("token.field.prefix")} {token.prefix}
          </DialogDescription>
        </DialogHeader>

        {query.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-2/3" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : query.isError ? (
          <p className="text-sm text-destructive">
            {query.error instanceof ApiError
              ? query.error.code === "token_secret_unavailable"
                ? t("token.secretUnavailable")
                : query.error.code === "config_secret_key_missing"
                  ? t("token.secretKeyMissing")
                  : t("token.errors.unknown")
              : t("token.errors.unknown")}
          </p>
        ) : query.data ? (
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <Badge
                variant={status === "active" ? "default" : "destructive"}
              >
                {t(`token.status.${status}`)}
              </Badge>
              {status === "revoked" ? (
                <span className="text-xs text-destructive">
                  {t("token.mcp.revokedWarning")}
                </span>
              ) : null}
              {status === "expired" ? (
                <span className="text-xs text-destructive">
                  {t("token.mcp.expiredWarning")}
                </span>
              ) : null}
            </div>

            <McpField
              copied={copied}
              copy={copy}
              label="endpoint"
              labelText={t("token.mcpEndpoint")}
              value={endpoint}
            />
            <McpField
              copied={copied}
              copy={copy}
              label="token"
              labelText={t("token.bearerToken")}
              value={query.data.token}
            />

            <div className="space-y-2">
              <div className="text-xs font-medium text-muted-foreground">
                {t("token.mcp.configJson")}
              </div>
              <pre className="overflow-x-auto rounded-none border bg-muted p-3 text-xs">
                <code>{configJson}</code>
              </pre>
              <Button
                onClick={() => copy("config", configJson)}
                type="button"
                variant="outline"
              >
                {copied === "config" ? (
                  <CheckIcon className="mr-1 size-4" />
                ) : (
                  <CopyIcon className="mr-1 size-4" />
                )}
                {t("token.copyConfig")}
              </Button>
            </div>

            {canImpersonate ? (
              <div className="space-y-2">
                <div className="text-xs font-medium text-muted-foreground">
                  {t("token.mcp.optionalHeaders")}
                </div>
                <pre className="overflow-x-auto rounded-none border bg-muted p-3 text-xs">
                  <code>{`{\n  "X-Xuanchu-As": "<user-name-or-email>"\n}`}</code>
                </pre>
              </div>
            ) : null}

            <p className="text-xs text-muted-foreground">
              {tokenType === "agent"
                ? t("token.mcp.agentHint")
                : tokenType === "pat"
                  ? t("token.mcp.patHint")
                  : t("token.mcp.tenantHint")}
            </p>
          </div>
        ) : null}

        <div className="flex justify-end pt-2">
          <Button onClick={close} type="button" variant="outline">
            {t("common.close")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

type McpFieldProps = {
  copied: string | null
  copy: (label: string, value: string) => void
  label: string
  labelText: string
  value: string
}

function McpField({ copied, copy, label, labelText, value }: McpFieldProps) {
  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-muted-foreground">
        {labelText}
      </div>
      <div className="flex items-stretch gap-2">
        <code className="flex-1 overflow-x-auto break-all rounded-none border bg-muted p-2 text-xs">
          {value}
        </code>
        <Button
          onClick={() => copy(label, value)}
          type="button"
          variant="outline"
        >
          {copied === label ? (
            <CheckIcon className="size-4" />
          ) : (
            <CopyIcon className="size-4" />
          )}
        </Button>
      </div>
    </div>
  )
}
