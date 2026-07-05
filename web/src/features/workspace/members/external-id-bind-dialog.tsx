import type { FormEvent } from "react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
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
import { ApiError } from "@/lib/api"

import { bindExternalID } from "./members-api"

type ExternalIDBindDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onBound: () => void
  userRef: string
}

export function ExternalIDBindDialog({
  onBound,
  onOpenChange,
  open,
  userRef,
}: ExternalIDBindDialogProps) {
  const { t } = useTranslation()
  const [provider, setProvider] = useState("")
  const [externalID, setExternalID] = useState("")
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  function resetForm() {
    setProvider("")
    setExternalID("")
    setSubmitError(null)
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) resetForm()
    onOpenChange(nextOpen)
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setSubmitError(null)
    if (!provider.trim() || !externalID.trim()) {
      setSubmitError(t("members.externalIdRequired"))
      return
    }
    setSubmitting(true)
    try {
      await bindExternalID(userRef, provider.trim(), externalID.trim())
      onBound()
      handleOpenChange(false)
    } catch (err) {
      setSubmitError(
        err instanceof ApiError ? err.message : t("common.error")
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("members.externalIdBindTitle")}</DialogTitle>
          <DialogDescription>
            {t("members.externalIdBindDescription")}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <div className="space-y-1">
            <Label htmlFor="ext-provider">{t("members.externalIdProvider")}</Label>
            <Input
              id="ext-provider"
              onChange={(e) => setProvider(e.target.value)}
              placeholder="feishu / github / google"
              value={provider}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="ext-id">{t("members.externalIdValue")}</Label>
            <Input
              id="ext-id"
              onChange={(e) => setExternalID(e.target.value)}
              placeholder="ou_xxx"
              value={externalID}
            />
          </div>
          {submitError ? (
            <Alert variant="destructive">
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              onClick={() => handleOpenChange(false)}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button disabled={submitting} type="submit">
              {t("members.externalIdBind")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
