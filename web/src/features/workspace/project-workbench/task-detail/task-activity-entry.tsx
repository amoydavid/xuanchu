import { useState } from "react"
import { PencilIcon, Trash2Icon } from "lucide-react"
import { MarkdownView } from "@/components/markdown"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useTranslation } from "react-i18next"

import type {
  TaskActivityEntry as TaskActivityEntryData,
  TaskChangeDisplayValue,
  TaskFieldChange,
} from "../api/task-api"

export function TaskActivityEntryContent({
  canWrite,
  entry,
  onDeleteAnnotation,
  onEditAnnotation,
}: {
  canWrite: boolean
  entry: TaskActivityEntryData
  onDeleteAnnotation: (annotationID: string) => void
  onEditAnnotation: (annotation: { id: string; description: string }) => void
}) {
  const { i18n, t } = useTranslation()
  const actor = activityActorLabel(entry, t)
  const occurredAt = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(entry.occurred_at))

  return (
    <div className="min-w-0 space-y-2 pb-5 text-sm">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 gap-y-0.5 leading-5">
          <span className="font-medium text-foreground">{actor}</span>
          <span className="text-foreground">
            {t(`taskDetail.activityAction.${entry.action}`)}
          </span>
          <time
            className="text-xs text-muted-foreground"
            dateTime={entry.occurred_at}
          >
            · {occurredAt}
          </time>
        </div>
        {canWrite && entry.annotation ? (
          <div className="flex shrink-0 items-center gap-0.5">
            <Button
              aria-label={t("taskDetail.editAnnotation")}
              onClick={() => onEditAnnotation(entry.annotation!)}
              size="icon-sm"
              type="button"
              variant="ghost"
            >
              <PencilIcon />
            </Button>
            <Button
              aria-label={t("taskDetail.deleteAnnotation")}
              onClick={() => onDeleteAnnotation(entry.annotation!.id)}
              size="icon-sm"
              type="button"
              variant="ghost"
            >
              <Trash2Icon />
            </Button>
          </div>
        ) : null}
      </div>
      {entry.changes?.length ? (
        <ActivityChanges changes={entry.changes} />
      ) : null}
      {entry.annotation ? (
        <MarkdownView className="max-w-none text-sm leading-6 text-muted-foreground">
          {entry.annotation.description}
        </MarkdownView>
      ) : null}
      {entry.link ? <ActivityLink entry={entry} /> : null}
    </div>
  )
}

function ActivityChanges({ changes }: { changes: TaskFieldChange[] }) {
  const { i18n, t } = useTranslation()
  return (
    <div className="space-y-1 text-xs text-muted-foreground">
      {changes.map((change, index) =>
        change.kind === "scalar" && change.field === "description" ? (
          <DescriptionChange change={change} key={`${change.field}-${index}`} />
        ) : (
          <div key={`${change.field}-${index}`}>
            <span className="text-foreground">
              {t(`projectWorkbench.taskHistory.field.${change.field}`)}
            </span>
            {change.kind === "scalar" ? (
              <span>
                {" "}
                {activityChangeValue(
                  change.previous,
                  change.field,
                  i18n.language,
                  t
                )}
                {" → "}
                {activityChangeValue(
                  change.current,
                  change.field,
                  i18n.language,
                  t
                )}
              </span>
            ) : change.kind === "set" ? (
              <span>
                {" · "}
                {t("projectWorkbench.taskHistory.added")}:{" "}
                {formatSet(change.added, change.field, t)};{" "}
                {t("projectWorkbench.taskHistory.removed")}:{" "}
                {formatSet(change.removed, change.field, t)}
              </span>
            ) : (
              <ul className="ml-3 list-disc">
                {change.entries.map((item) => (
                  <li key={item.name}>
                    {item.name}:{" "}
                    {item.before?.text ??
                      t("projectWorkbench.taskHistory.unset")}{" "}
                    →{" "}
                    {item.after?.text ??
                      t("projectWorkbench.taskHistory.unset")}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )
      )}
    </div>
  )
}

function DescriptionChange({
  change,
}: {
  change: Extract<TaskFieldChange, { kind: "scalar" }>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const attachmentDelta = descriptionAttachmentDelta(change)
  return (
    <div className="space-y-1">
      <span className="text-foreground">
        {t("projectWorkbench.taskHistory.field.description")}
      </span>
      {attachmentDelta ? (
        <span>
          {" · "}
          {t(
            "projectWorkbench.taskHistory.descriptionAttachmentDelta",
            attachmentDelta
          )}
        </span>
      ) : null}
      <div>
        <Button
          className="h-auto p-0 text-xs"
          onClick={() => setOpen(true)}
          size="sm"
          type="button"
          variant="link"
        >
          {t("projectWorkbench.taskHistory.viewChanges")}
        </Button>
      </div>
      <Dialog onOpenChange={setOpen} open={open}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t("projectWorkbench.taskHistory.field.description")}
            </DialogTitle>
            <DialogDescription>
              {t("projectWorkbench.taskHistory.viewChanges")}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 text-sm">
            <DescriptionValue
              label={t("projectWorkbench.taskHistory.currentValue")}
              value={change.current}
            />
            <DescriptionValue
              label={t("projectWorkbench.taskHistory.previousValue")}
              value={change.previous}
            />
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function DescriptionValue({
  label,
  value,
}: {
  label: string
  value: TaskChangeDisplayValue
}) {
  const { t } = useTranslation()
  return (
    <div>
      <div className="mb-1 text-xs text-muted-foreground uppercase">
        {label}
      </div>
      {typeof value.raw === "string" ? (
        <MarkdownView className="max-w-none text-sm leading-6 text-muted-foreground">
          {value.raw}
        </MarkdownView>
      ) : (
        <span className="text-muted-foreground">
          {value.raw == null
            ? t("projectWorkbench.taskHistory.unset")
            : value.text}
        </span>
      )}
    </div>
  )
}

function activityChangeValue(
  value: TaskChangeDisplayValue,
  field: string,
  locale: string,
  t: ReturnType<typeof useTranslation>["t"]
) {
  if (value.raw == null) return t("projectWorkbench.taskHistory.unset")
  if (
    ["due", "wait", "scheduled", "until"].includes(field) &&
    typeof value.raw === "number"
  ) {
    return new Date(value.raw * 1000).toLocaleDateString(locale)
  }
  if (typeof value.raw === "string" || typeof value.raw === "number")
    return String(value.raw)
  return value.text
}

function formatSet(
  values: TaskChangeDisplayValue[],
  field: string,
  t: ReturnType<typeof useTranslation>["t"]
) {
  if (values.length === 0) return t("projectWorkbench.taskHistory.none")
  return values
    .map((value) => {
      if (field === "assignees" && value.raw && typeof value.raw === "object") {
        const actor = value.raw as {
          display_name?: string
          name?: string
          id?: string
        }
        return actor.display_name || actor.name || actor.id || value.text
      }
      return value.raw == null
        ? t("projectWorkbench.taskHistory.unset")
        : value.text
    })
    .join(", ")
}

function descriptionAttachmentDelta(
  change: Extract<TaskFieldChange, { kind: "scalar" }>
) {
  const previous = attachmentIDs(change.previous)
  const current = attachmentIDs(change.current)
  const added = [...current].filter((id) => !previous.has(id)).length
  const removed = [...previous].filter((id) => !current.has(id)).length
  return added > 0 || removed > 0 ? { added, removed } : null
}

function attachmentIDs(value: TaskChangeDisplayValue): Set<string> {
  if (typeof value.raw !== "string") return new Set()
  return new Set(
    [...value.raw.matchAll(/\]\(ref:\/\/attachment\/([^)]+)\)/g)].map(
      (match) => match[1]
    )
  )
}

function ActivityLink({ entry }: { entry: TaskActivityEntryData }) {
  const link = entry.link
  if (!link) return null
  const label = link.title || link.url || link.id
  return link.url ? (
    <a
      className="inline-flex max-w-full truncate text-xs text-primary underline-offset-4 hover:underline"
      href={link.url}
      rel="noreferrer"
      target="_blank"
    >
      {label}
    </a>
  ) : (
    <span className="text-xs text-muted-foreground">{label}</span>
  )
}

function activityActorLabel(
  entry: TaskActivityEntryData,
  t: ReturnType<typeof useTranslation>["t"]
) {
  if (entry.actor.type === "user" && entry.actor.user) {
    return (
      entry.actor.user.display_name ||
      entry.actor.user.name ||
      entry.actor.user.id
    )
  }
  if (entry.actor.type === "tenant_access_token" && entry.actor.token) {
    return entry.actor.token.name || entry.actor.token.id
  }
  if (entry.actor.type === "system") return t("taskDetail.activityActorSystem")
  return t("taskDetail.activityActorUnknown")
}
