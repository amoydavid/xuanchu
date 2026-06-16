import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"

type TokenCreatedResultProps = {
  rawToken: string
  onDone: () => void
}

export function TokenCreatedResult({
  rawToken,
  onDone,
}: TokenCreatedResultProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard?.writeText(rawToken)
      setCopied(true)
    } catch {
      // 剪贴板不可用时静默失败，用户可手动选中复制
    }
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-destructive">{t("token.createdWarning")}</p>
      <div className="flex items-stretch gap-2">
        <code className="flex-1 overflow-x-auto break-all rounded-none border bg-muted p-2 text-sm">
          {rawToken}
        </code>
        <Button onClick={copy} type="button" variant="outline">
          {copied ? t("token.copied") : t("token.copy")}
        </Button>
      </div>
      <div className="flex justify-end">
        <Button onClick={onDone} type="button">
          {t("token.done")}
        </Button>
      </div>
    </div>
  )
}
