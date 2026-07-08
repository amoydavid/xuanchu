import { CopyIcon, TerminalIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

import type { ProjectAutomationPreview } from "./project-automations-api"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  preview: ProjectAutomationPreview | null
}

// AutomationPreviewDialog 展示脱敏后的投递 JSON，支持复制 body 和 curl。
// Authorization 永远显示 Bearer ****，复制 curl 时 token 使用占位符。
export function AutomationPreviewDialog({ open, onOpenChange, preview }: Props) {
  const bodyJSON = preview ? JSON.stringify(preview.body, null, 2) : ""
  const curl = preview
    ? [
        `curl -s -X ${preview.method} ${JSON.stringify(preview.url)}`,
        `  -H "Authorization: Bearer \${AGENT_PROVIDER_API_KEY}"`,
        `  -H "Content-Type: application/json"`,
        `  -d ${JSON.stringify(bodyJSON)}`,
      ].join(" \\\n")
    : ""

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>预览投递 JSON</DialogTitle>
          <DialogDescription className="sr-only">
            查看脱敏后的投递请求方法和 URL、headers 与 body JSON，并可复制。
          </DialogDescription>
        </DialogHeader>
        {preview ? (
          <div className="space-y-3">
            <div className="space-y-1 text-sm">
              <div>
                {preview.method} {preview.url}
              </div>
              {Object.entries(preview.headers).map(([key, value]) => (
                <div key={key}>
                  {key}: {value}
                </div>
              ))}
            </div>
            <pre className="max-h-[420px] overflow-auto rounded-md border bg-muted p-3 text-xs">
              {bodyJSON}
            </pre>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => void navigator.clipboard?.writeText(bodyJSON)}>
                <CopyIcon className="mr-2 h-4 w-4" />
                复制 JSON
              </Button>
              <Button variant="outline" onClick={() => void navigator.clipboard?.writeText(curl)}>
                <TerminalIcon className="mr-2 h-4 w-4" />
                复制 curl
              </Button>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
