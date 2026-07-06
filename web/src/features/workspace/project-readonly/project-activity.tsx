import type { ProjectReadonlyTimelineEntry } from "./project-readonly-api"

export function ProjectActivity({
  entries,
  title,
}: {
  entries?: ProjectReadonlyTimelineEntry[]
  title: string
}) {
  if (!entries || entries.length === 0) {
    return null
  }
  const visibleEntries = [...entries].sort(compareTimelineEntries).slice(0, 5)

  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">{title}</h2>
      <div className="border bg-card px-3 py-2">
        <ol aria-label={title} className="text-xs text-muted-foreground">
          {visibleEntries.map((entry, index) => {
            const time = entryTime(entry)
            const primaryText = [actorName(entry), entryText(entry)]
              .filter(Boolean)
              .join(" ")
            const source = sourceLabel(entry)
            return (
              <li
                className="relative border-l border-border py-2 pl-4 first:pt-1 last:pb-1"
                key={entry.id || entryKey(entry, index)}
              >
                <span className="absolute -left-[5px] top-3 size-2 rounded-full bg-foreground ring-2 ring-card" />
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <span className="font-medium text-foreground">
                    {primaryText}
                  </span>
                  {time ? (
                    <time
                      className="font-mono text-[11px] tabular-nums"
                      dateTime={time.toISOString()}
                    >
                      {formatEntryTime(time)}
                    </time>
                  ) : null}
                </div>
                {source ? <div className="mt-0.5 truncate">{source}</div> : null}
              </li>
            )
          })}
        </ol>
      </div>
    </section>
  )
}

function compareTimelineEntries(
  left: ProjectReadonlyTimelineEntry,
  right: ProjectReadonlyTimelineEntry
): number {
  return entryTimestamp(right) - entryTimestamp(left)
}

function entryTimestamp(entry: ProjectReadonlyTimelineEntry): number {
  const value = entry.entry ?? entry.created_at
  return typeof value === "number" && Number.isFinite(value) ? value : 0
}

function actorName(entry: ProjectReadonlyTimelineEntry): string {
  return (
    entry.created_by?.user?.name ||
    entry.created_by?.token?.name ||
    entry.created_by?.name ||
    entry.actor?.name ||
    ""
  )
}

function entryKey(entry: ProjectReadonlyTimelineEntry, index: number): string {
  return `${entry.source_type ?? "entry"}-${entry.source_id ?? index}-${entry.entry ?? index}`
}

function entryText(entry: ProjectReadonlyTimelineEntry): string {
  return (
    entry.content ||
    entry.action ||
    entry.event_type ||
    entry.summary ||
    entry.source_label ||
    "-"
  )
}

function entryTime(entry: ProjectReadonlyTimelineEntry): Date | null {
  const timestamp = entryTimestamp(entry)
  return timestamp > 0 ? new Date(timestamp * 1000) : null
}

function formatEntryTime(time: Date): string {
  return time.toLocaleString()
}

function sourceLabel(entry: ProjectReadonlyTimelineEntry): string {
  return [entry.source_type, entry.source_label].filter(Boolean).join(" · ")
}
