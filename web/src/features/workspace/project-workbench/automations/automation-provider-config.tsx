import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  listProjectConfig,
  setProjectConfig,
} from "@/features/workspace/project-workbench/api/project-api"
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"

import {
  EMPTY_AUTOMATION_PROVIDER_CONFIG,
  type AutomationProviderConfig,
} from "./project-automations-api"

type Props = {
  projectSlug: string
  workspaceSlug: string
  disabled?: boolean
}

// 从 project config 原始条目中抽取 provider 配置。
export function providerConfigFromEntries(entries: { key: string; value: string }[]): AutomationProviderConfig {
  const get = (key: string) => entries.find((e) => e.key === key)?.value ?? ""
  return {
    base_url: get("agent.provider.base_url"),
    api_key_set: get("agent.provider.api_key") !== "",
    model: get("agent.provider.model"),
    allowed_hosts: get("agent.provider.allowed_hosts"),
  }
}

// isProviderConfigComplete 判断 provider 配置是否齐全，用于决定是否阻塞预览/测试。
// allowed_hosts 是可选项（SSRF 白名单），未配置时跳过 host 校验。
export function isProviderConfigComplete(cfg: AutomationProviderConfig): boolean {
  return (
    cfg.base_url.trim() !== "" &&
    cfg.api_key_set &&
    cfg.model.trim() !== ""
  )
}

// AutomationProviderConfigSection 是 Agent Provider 配置卡片：
// 读取当前 project config，展示 base_url / api_key / model / allowed_hosts，
// 缺失时高亮提示，可在卡片内直接保存到 project config。
//
// 实现上不把 query 数据镜像到 state（避免 effect 级联渲染），
// 而是用 localEdits 记录用户改动，输入框显示值 = localEdits ?? current。
export function AutomationProviderConfigSection({ projectSlug, workspaceSlug, disabled }: Props) {
  const feedback = useEditFeedback()
  const queryClient = useQueryClient()
  const queryKey = ["project", projectSlug, "config"]
  const config = useQuery({
    queryKey,
    queryFn: () => listProjectConfig(workspaceSlug, projectSlug),
  })

  const current = config.data ? providerConfigFromEntries(config.data) : EMPTY_AUTOMATION_PROVIDER_CONFIG
  // localEdits 只记录用户在本会话改过的字段；未改字段回退到 current。
  const [edits, setEdits] = useState<Partial<Record<"base_url" | "api_key" | "model" | "allowed_hosts", string>>>({})

  const fieldValue = (key: "base_url" | "model" | "allowed_hosts") =>
    key in edits ? edits[key]! : current[key]
  const apiKeyInput = edits.api_key ?? ""

  const saveMutation = useMutation({
    mutationFn: async () => {
      const baseUrl = fieldValue("base_url")
      const model = fieldValue("model")
      const allowedHosts = fieldValue("allowed_hosts")
      const apiKey = edits.api_key ?? ""
      const sets: Promise<void>[] = [
        setProjectConfig(workspaceSlug, projectSlug, "agent.provider.base_url", baseUrl),
        setProjectConfig(workspaceSlug, projectSlug, "agent.provider.model", model),
        setProjectConfig(workspaceSlug, projectSlug, "agent.provider.allowed_hosts", allowedHosts),
      ]
      // api_key 为空时不覆盖已设置的值。
      if (apiKey !== "") {
        sets.push(setProjectConfig(workspaceSlug, projectSlug, "agent.provider.api_key", apiKey))
      }
      await Promise.all(sets)
    },
    onSuccess: () => {
      feedback.success("Agent Provider 配置已保存")
      setEdits({})
      queryClient.invalidateQueries({ queryKey })
    },
    onError: (err) => {
      feedback.failure("配置保存失败", err instanceof Error ? err.message : String(err))
    },
  })

  const complete = isProviderConfigComplete(current)
  const missing: string[] = []
  if (current.base_url.trim() === "") missing.push("base_url")
  if (!current.api_key_set) missing.push("api_key")
  if (current.model.trim() === "") missing.push("model")

  return (
    <section className="space-y-3 rounded-md border p-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">Agent Provider 配置</h3>
        <span className={complete ? "text-xs text-emerald-600" : "text-xs text-amber-600"}>
          {complete ? "已配置" : `缺少 ${missing.join("、")}`}
        </span>
      </div>
      {!complete ? (
        <Alert variant="default">
          <AlertTitle>预览和测试需要先配置 Agent Provider</AlertTitle>
          <AlertDescription>
            填写下方 base_url、API Key 和 model 后保存。allowed_hosts 是可选的 SSRF 白名单，未配置时不限制目标 host。
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-3">
        <div className="grid gap-1">
          <label className="text-xs font-medium" htmlFor="provider-base-url">Base URL</label>
          <Input
            id="provider-base-url"
            value={fieldValue("base_url")}
            onChange={(e) => setEdits((prev) => ({ ...prev, base_url: e.target.value }))}
            disabled={disabled}
            placeholder="https://agent.example.com"
          />
        </div>
        <div className="grid gap-1">
          <label className="text-xs font-medium" htmlFor="provider-api-key">API Key</label>
          <Input
            id="provider-api-key"
            type="password"
            value={apiKeyInput}
            onChange={(e) => setEdits((prev) => ({ ...prev, api_key: e.target.value }))}
            disabled={disabled}
            placeholder={current.api_key_set ? "已设置（留空保留原值）" : "sk-..."}
          />
        </div>
        <div className="grid gap-1">
          <label className="text-xs font-medium" htmlFor="provider-model">Model</label>
          <Input
            id="provider-model"
            value={fieldValue("model")}
            onChange={(e) => setEdits((prev) => ({ ...prev, model: e.target.value }))}
            disabled={disabled}
            placeholder="project-operator"
          />
        </div>
        <div className="grid gap-1">
          <label className="text-xs font-medium" htmlFor="provider-allowed-hosts">Allowed Hosts（可选，JSON 数组）</label>
          <Input
            id="provider-allowed-hosts"
            value={fieldValue("allowed_hosts")}
            onChange={(e) => setEdits((prev) => ({ ...prev, allowed_hosts: e.target.value }))}
            disabled={disabled}
            placeholder='["agent.example.com"]'
          />
        </div>
      </div>
      <div className="flex justify-end">
        <Button
          type="button"
          size="sm"
          disabled={disabled || saveMutation.isPending}
          onClick={() => saveMutation.mutate()}
        >
          {saveMutation.isPending ? "保存中..." : "保存配置"}
        </Button>
      </div>
    </section>
  )
}
