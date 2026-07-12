/* eslint-disable react-hooks/set-state-in-effect -- 弹窗/面板打开时需在 effect 内重置状态 */
import { useEffect, useState } from "react"

// TaskSeriesStopDialog 是停止循环确认弹窗（spec §15.7）。
// 与普通任务删除、occurrence 跳过使用不同动词和说明。
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
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  // 每次打开时重置选项。
  useEffect(() => {
    if (open) {
      setDeleteOpen(false)
      setSubmitting(false)
    }
  }, [open])

  if (!open || !series) return null

  // 超过 1000 条时禁用 delete-open（spec §11.6）。
  const deleteDisabled = openCount > 1000

  return (
    <div role="dialog" aria-modal="true" aria-label="停止循环" data-testid="task-series-stop-dialog">
      <h3>停止循环</h3>
      <p>停止后不会再生成新任务，历史记录会保留。</p>
      <p>循环任务：{series.title}</p>
      <label>
        <input
          type="checkbox"
          checked={deleteOpen}
          disabled={deleteDisabled}
          onChange={(e) => setDeleteOpen(e.target.checked)}
          data-testid="delete-open-checkbox"
        />
        同时跳过当前 {openCount} 条未完成实例
      </label>
      {deleteDisabled && (
        <p role="note">未完成实例超过 1000 条，请先仅停止系列或等待补齐。</p>
      )}
      <div>
        <button type="button" onClick={onCancel} disabled={submitting}>取消</button>
        <button
          type="button"
          onClick={() => {
            setSubmitting(true)
            onConfirm(deleteOpen)
          }}
          disabled={submitting}
          data-testid="confirm-stop-btn"
        >
          确认停止循环
        </button>
      </div>
    </div>
  )
}
