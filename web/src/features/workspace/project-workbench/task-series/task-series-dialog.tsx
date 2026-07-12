import { useEffect, useMemo, useState } from "react"

import {
  createTaskSeries,
  modifyTaskSeries,
  type TaskSeriesView,
} from "@/features/workspace/project-workbench/api/task-series-api"
import {
  RECURRENCE_OPTIONS,
  formatDateShort,
  isCanonicalRecurrenceRule,
  parseDateToEndOfDay,
  previewRecurrenceDates,
  type CanonicalRecurrenceRule,
} from "./recurrence-preview"

// TaskSeriesDialog 是创建/编辑循环任务的统一弹窗（spec §15.4、§15.16）。
// 创建模式：提交 POST /task-series。
// 编辑模式：first_due 只读；rule 修改需 effective_from（App 层完整实现后接入）。
export function TaskSeriesDialog({
  open,
  workspaceSlug,
  projectSlug,
  mode,
  series,
  onClose,
  onCreated,
}: {
  open: boolean
  workspaceSlug: string
  projectSlug: string
  mode: "create" | "edit"
  series?: TaskSeriesView | null
  onClose: () => void
  onCreated?: (series: TaskSeriesView) => void
}) {
  const [title, setTitle] = useState("")
  const [rule, setRule] = useState<CanonicalRecurrenceRule>("daily")
  const [firstDue, setFirstDue] = useState("")
  const [until, setUntil] = useState("")
  const [priority, setPriority] = useState("")
  const [tags, setTags] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // 打开时填充表单。
  useEffect(() => {
    if (!open) return
    if (mode === "edit" && series) {
      setTitle(series.title)
      if (isCanonicalRecurrenceRule(series.recurrence_rule)) {
        setRule(series.recurrence_rule)
      }
      setFirstDue(formatDateShort(new Date(series.first_due * 1000)))
      setUntil(series.until ? formatDateShort(new Date(series.until * 1000)) : "")
      setPriority(series.priority ?? "")
      setTags((series.tags ?? []).join(", "))
    } else {
      setTitle("")
      setRule("daily")
      setFirstDue("")
      setUntil("")
      setPriority("")
      setTags("")
    }
    setError(null)
    setSubmitting(false)
  }, [open, mode, series])

  // 未来三次预览。
  const previewDates = useMemo(() => {
    if (!firstDue) return []
    const from = parseDateToEndOfDay(firstDue)
    if (!from || !isCanonicalRecurrenceRule(rule)) return []
    const untilDate = until ? parseDateToEndOfDay(until) : null
    return previewRecurrenceDates(from, rule, 3, untilDate)
  }, [firstDue, rule, until])

  // until 早于 first_due 校验。
  const untilError = useMemo(() => {
    if (!firstDue || !until) return null
    const fd = parseDateToEndOfDay(firstDue)
    const ut = parseDateToEndOfDay(until)
    if (!fd || !ut) return null
    if (ut.getTime() < fd.getTime()) return "循环结束必须不早于首次截止"
    return null
  }, [firstDue, until])

  if (!open) return null

  const submit = async () => {
    const trimmedTitle = title.trim()
    if (!trimmedTitle) {
      setError("任务标题不能为空")
      return
    }
    if (!firstDue) {
      setError("首次截止必填")
      return
    }
    const firstDueDate = parseDateToEndOfDay(firstDue)
    if (!firstDueDate) {
      setError("首次截止格式必须为 YYYY-MM-DD")
      return
    }
    if (untilError) {
      setError(untilError)
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      if (mode === "create") {
        const result = await createTaskSeries(workspaceSlug, {
          title: trimmedTitle,
          project: projectSlug,
          recurrence_rule: rule,
          first_due: Math.floor(firstDueDate.getTime() / 1000),
          ...(until ? { until: Math.floor((parseDateToEndOfDay(until)?.getTime() ?? 0) / 1000) } : {}),
          ...(priority ? { priority } : {}),
          ...(tags.trim() ? { tags: tags.split(",").map((t) => t.trim()).filter(Boolean) } : {}),
        })
        onCreated?.(result.series)
      } else if (series) {
        // 编辑：first_due 只读；rule 修改需 effective_from（App 层占位）。
        await modifyTaskSeries(workspaceSlug, series.id, {
          title: trimmedTitle,
          ...(priority ? { priority } : {}),
        })
      }
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败")
      setSubmitting(false)
    }
  }

  const dialogTitle = mode === "create" ? "新建循环任务" : "编辑循环设置"

  return (
    <div role="dialog" aria-modal="true" aria-label={dialogTitle} data-testid="task-series-dialog">
      <h3>{dialogTitle}</h3>
      {mode === "create" && (
        <p>创建规则后，每个日期都是可以独立完成的一次任务。</p>
      )}
      <label>
        标题 *
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          aria-label="任务标题"
        />
      </label>
      <label>
        循环规则 *
        <select
          value={rule}
          onChange={(e) => setRule(e.target.value as CanonicalRecurrenceRule)}
          disabled={mode === "edit"}
          aria-label="循环规则"
        >
          {RECURRENCE_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </label>
      <label>
        首次截止 *
        <input
          type="date"
          value={firstDue}
          onChange={(e) => setFirstDue(e.target.value)}
          disabled={mode === "edit"}
          aria-label="首次截止日期"
        />
      </label>
      <label>
        循环结束
        <input
          type="date"
          value={until}
          onChange={(e) => setUntil(e.target.value)}
          aria-label="循环结束日期"
        />
      </label>
      {untilError && <div role="alert">{untilError}</div>}
      <label>
        优先级
        <select value={priority} onChange={(e) => setPriority(e.target.value)} aria-label="优先级">
          <option value="">无</option>
          <option value="H">H</option>
          <option value="M">M</option>
          <option value="L">L</option>
        </select>
      </label>
      <label>
        标签
        <input
          type="text"
          value={tags}
          onChange={(e) => setTags(e.target.value)}
          placeholder="逗号分隔"
          aria-label="标签"
        />
      </label>
      {previewDates.length > 0 && (
        <p>未来三次：{previewDates.map((d) => formatDateShort(d)).join(" · ")}</p>
      )}
      {error && <div role="alert">{error}</div>}
      <div>
        <button type="button" onClick={onClose} disabled={submitting}>取消</button>
        <button
          type="button"
          onClick={submit}
          disabled={submitting}
          data-testid="series-submit-btn"
        >
          {mode === "create" ? "创建循环任务" : "保存循环设置"}
        </button>
      </div>
    </div>
  )
}
