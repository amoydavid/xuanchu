import { useEffect, useState } from "react"

import {
  loadContentReference,
  type ContentReferenceResolution,
} from "@/features/workspace/content-references"

export function ReferenceInlineView({
  kind,
  id,
  label,
}: {
  kind: "user" | "task"
  id: string
  label: string
}) {
  const [resolved, setResolved] = useState<ContentReferenceResolution | null>(null)

  useEffect(() => {
    let active = true
    void loadContentReference({ type: kind, id }).then((next) => {
      if (active) setResolved(next)
    })
    return () => { active = false }
  }, [id, kind])

  if (resolved?.status !== "resolved") {
    return <span className="rounded bg-muted px-1 py-0.5 text-xs">{label}</span>
  }
  if (resolved.type === "task") {
    return (
      <a
        className="text-primary underline-offset-2 hover:underline"
        href={resolved.task.url}
      >
        {resolved.task.title}
      </a>
    )
  }
  if (resolved.type === "user") {
    return (
      <span className="rounded bg-muted px-1 py-0.5 text-xs">
        @{resolved.user.display_name || resolved.user.name || label}
      </span>
    )
  }
  return <span className="rounded bg-muted px-1 py-0.5 text-xs">{label}</span>
}
