import type { FormEvent } from "react"
import { useState } from "react"

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

import { isAllowedMarkdownHref } from "./markdown-safety"

type LinkDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** 打开时回填的已有链接，没有则为 undefined */
  initialHref?: string
  /** 提交合法链接时回调，传入校验通过后的 href */
  onSubmit: (href: string) => void
}

export function LinkDialog({
  initialHref,
  onOpenChange,
  onSubmit,
  open,
}: LinkDialogProps) {
  // 每次打开时通过 key 重新挂载，初始值直接用 initialHref，避免 effect 同步 setState
  return (
    <LinkDialogInner
      initialHref={initialHref}
      key={open ? `open-${initialHref ?? ""}` : "closed"}
      onOpenChange={onOpenChange}
      onSubmit={onSubmit}
      open={open}
    />
  )
}

type LinkDialogInnerProps = LinkDialogProps

function LinkDialogInner({
  initialHref,
  onOpenChange,
  onSubmit,
  open,
}: LinkDialogInnerProps) {
  const [href, setHref] = useState(initialHref ?? "https://")
  const [error, setError] = useState<string | null>(null)

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setError(null)
    }
    onOpenChange(nextOpen)
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    const next = href.trim()
    if (!next) {
      setError("请输入链接地址")
      return
    }
    if (!isAllowedMarkdownHref(next)) {
      setError("仅支持 http:// / https:// / mailto: 链接")
      return
    }
    onSubmit(next)
    handleOpenChange(false)
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>链接</DialogTitle>
          <DialogDescription>输入链接地址，仅支持 http / https / mailto。</DialogDescription>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <div className="space-y-1">
            <Label htmlFor="markdown-link-href">链接地址</Label>
            <Input
              aria-invalid={error ? true : undefined}
              autoCapitalize="off"
              autoComplete="off"
              autoCorrect="off"
              id="markdown-link-href"
              onChange={(e) => setHref(e.target.value)}
              placeholder="https://example.com"
              spellCheck={false}
              value={href}
            />
          </div>
          {error ? (
            <p className="text-xs text-destructive">{error}</p>
          ) : null}
          <DialogFooter>
            <Button
              onClick={() => handleOpenChange(false)}
              type="button"
              variant="outline"
            >
              取消
            </Button>
            <Button type="submit">确定</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

