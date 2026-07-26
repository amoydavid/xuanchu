import { useState } from "react"

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
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"

import {
  useUpdateWorkspaceAutomationProviderConfig,
  useWorkspaceAutomationProviderConfig,
  type AutomationProviderConfig,
} from "@/features/workspace/automations/workspace-automations-api"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // 已经具备 config:write 时才能编辑；Automation 权限不隐式授予 Provider config 权限。
  canEdit: boolean
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// ProviderConfigDialog 是 Workspace Agent Provider 配置 Dialog。
// 只消费安全 facade DTO：GET/PUT 永不回显 api_key 明文；api_key_set 只控制占位文案。
// 保存成功/失败/Dialog 关闭/query invalidation 时都清空本地 api_key state。
//
// spec §16.3: 「api_key_set=true 只控制“已设置（留空保留）”占位文案，不把 mask 填入 value；
// 显式“清除 API Key”需要二次确认；不得用空字符串隐式清除。」
export function ProviderConfigDialog({ open, onOpenChange, canEdit }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {open ? (
          <ProviderConfigForm onDone={() => onOpenChange(false)} canEdit={canEdit} />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

// ProviderConfigForm 在 Dialog 打开时挂载，关闭时卸载，自然完成 state 初始化和清理，
// 避免 effect 级联 setState。
function ProviderConfigForm({ onDone, canEdit }: { onDone: () => void; canEdit: boolean }) {
  const feedback = useEditFeedback()
  const provider = useWorkspaceAutomationProviderConfig()
  const update = useUpdateWorkspaceAutomationProviderConfig()

  const data = provider.data
  const [baseUrl, setBaseUrl] = useState(data?.base_url ?? "")
  const [model, setModel] = useState(data?.model ?? "")
  const [allowedHostsText, setAllowedHostsText] = useState(
    (data?.allowed_hosts ?? []).join("\n")
  )
  const [apiKey, setApiKey] = useState("")
  const [clearApiKey, setClearApiKey] = useState(false)

  const apiKeyPlaceholder = data?.api_key_set ? "已设置（留空保留原值）" : "sk-..."

  function handleSave() {
    const hosts = allowedHostsText
      .split("\n")
      .map((s) => s.trim())
      .filter((s) => s !== "")
    if (clearApiKey && apiKey.trim() !== "") {
      feedback.failure("保存失败", "不能同时设置 API Key 和清除 API Key")
      return
    }
    update.mutate(
      {
        base_url: baseUrl.trim(),
        model: model.trim(),
        allowed_hosts: hosts,
        api_key: apiKey,
        clear_api_key: clearApiKey,
      },
      {
        onSuccess: () => {
          feedback.success("Agent Provider 配置已保存")
          // 关闭 Dialog 会卸载本组件，自动清空所有本地 state。
          onDone()
        },
        onError: (err) => feedback.failure("保存失败", errorMessage(err)),
      }
    )
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>Agent Provider 配置</DialogTitle>
        <DialogDescription>
          来源：Workspace config。Workspace 规则的 Provider 只读 Workspace 配置，Project 样本不会覆盖。
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-4">
        <div className="grid gap-1.5">
          <Label htmlFor="provider-base-url">Base URL *</Label>
          <Input
            id="provider-base-url"
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
            disabled={!canEdit}
            placeholder="https://agent.example.com"
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="provider-api-key">API Key</Label>
          <Input
            id="provider-api-key"
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            disabled={!canEdit}
            placeholder={apiKeyPlaceholder}
            autoComplete="off"
          />
          <Label className="flex items-center gap-2 text-xs text-muted-foreground">
            <Checkbox
              checked={clearApiKey}
              onCheckedChange={(v) => setClearApiKey(v === true)}
              disabled={!canEdit || apiKey.trim() !== ""}
            />
            清除已有 API Key
          </Label>
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="provider-model">Model *</Label>
          <Input
            id="provider-model"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            disabled={!canEdit}
            placeholder="workspace-operator"
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="provider-allowed-hosts">Allowed Hosts（每行一个 hostname）</Label>
          <textarea
            id="provider-allowed-hosts"
            className="min-h-[64px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm"
            value={allowedHostsText}
            onChange={(e) => setAllowedHostsText(e.target.value)}
            disabled={!canEdit}
            placeholder="agent.example.com"
          />
          <p className="text-xs text-muted-foreground">
            留空表示不限制；SSRF 白名单对 preview、test、正式投递和 replay 共用。
          </p>
        </div>
        <p className="text-xs text-muted-foreground">API Key 永不回显。保存后输入框立即清空。</p>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          取消
        </Button>
        <Button type="button" onClick={handleSave} disabled={!canEdit || update.isPending}>
          {update.isPending ? "保存中..." : "保存配置"}
        </Button>
      </DialogFooter>
    </>
  )
}

// ProviderConfigSummary 是规则 Dialog 顶部的只读 Provider 摘要，点击「查看配置」打开 Dialog。
export function ProviderConfigSummary({ data }: { data?: AutomationProviderConfig }) {
  if (!data) return null
  const summary = data.complete
    ? `${data.base_url} · model ${data.model}`
    : `缺少 ${data.missing_fields.join("、")}`
  return (
    <div className="rounded-md border border-border bg-muted/30 px-3 py-2 text-xs">
      <div className="font-medium">Agent Provider · Workspace config</div>
      <div className="text-muted-foreground">{summary}</div>
    </div>
  )
}
