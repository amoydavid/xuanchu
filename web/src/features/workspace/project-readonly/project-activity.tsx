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

  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">{title}</h2>
      <div className="border bg-card p-3">
        <ul className="space-y-2 text-xs text-muted-foreground">
          {entries.slice(0, 5).map((entry, index) => (
            <li key={entry.id || index}>
              {actorName(entry)} {entry.action || entry.event_type || entry.summary}
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}

function actorName(entry: ProjectReadonlyTimelineEntry): string {
  return entry.created_by?.name || entry.actor?.name || "-"
}
