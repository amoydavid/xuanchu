/* eslint-disable react-hooks/set-state-in-effect -- 每次打开确认框都需要重置临时选择 */
import { TriangleAlertIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"

export function TaskSeriesStopDialog({
  open,
  series,
  openCount,
  onConfirm,
  onCancel,
}: {
  open: boolean
  series: { title: string } | null
  openCount: number
  onConfirm: (deleteOpen: boolean) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (open) {
      setDeleteOpen(false)
      setSubmitting(false)
    }
  }, [open])

  const deleteDisabled = openCount > 1000

  return (
    <AlertDialog
      open={open && series != null}
      onOpenChange={(next) => !next && !submitting && onCancel()}
    >
      <AlertDialogContent data-testid="task-series-stop-dialog">
        <AlertDialogHeader>
          <AlertDialogMedia className="text-destructive">
            <TriangleAlertIcon />
          </AlertDialogMedia>
          <AlertDialogTitle>{t("taskSeries.stop.title")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("taskSeries.stop.description")}
          </AlertDialogDescription>
        </AlertDialogHeader>

        {series ? (
          <p className="rounded-lg bg-muted px-3 py-2 text-sm font-medium">
            {t("taskSeries.stop.series", { title: series.title })}
          </p>
        ) : null}

        <div className="flex items-start gap-3 rounded-lg border p-3">
          <Checkbox
            checked={deleteOpen}
            data-testid="delete-open-checkbox"
            disabled={deleteDisabled || submitting}
            id="delete-open-occurrences"
            onCheckedChange={(checked) => setDeleteOpen(checked === true)}
          />
          <Label className="leading-5" htmlFor="delete-open-occurrences">
            {t("taskSeries.stop.deleteOpen", { count: openCount })}
          </Label>
        </div>

        {deleteDisabled ? (
          <Alert>
            <TriangleAlertIcon />
            <AlertDescription>
              {t("taskSeries.stop.overLimit")}
            </AlertDescription>
          </Alert>
        ) : null}

        <AlertDialogFooter>
          <AlertDialogCancel disabled={submitting}>
            {t("common.cancel")}
          </AlertDialogCancel>
          <Button
            data-testid="confirm-stop-btn"
            disabled={submitting}
            onClick={() => {
              setSubmitting(true)
              onConfirm(deleteOpen)
            }}
            type="button"
            variant="destructive"
          >
            {submitting
              ? t("taskSeries.stop.confirming")
              : t("taskSeries.stop.confirm")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
