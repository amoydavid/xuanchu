import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { ApiError } from "@/lib/api"

import * as api from "../api/project-template-api"
import { ProjectTemplateCaptureWizard } from "./project-template-capture-wizard"

vi.mock("../api/project-template-api", async (importActual) => {
  const actual =
    await importActual<typeof import("../api/project-template-api")>()
  return {
    ...actual,
    appendProjectTemplateSnapshot: vi.fn(),
    createProjectTemplate: vi.fn(),
    listProjectTemplateAutomationCandidates: vi.fn(),
    listProjectTemplateConfigCandidates: vi.fn(),
    listProjectTemplateSeriesCandidates: vi.fn(),
    listProjectTemplateTaskCandidates: vi.fn(),
    previewProjectTemplateCapture: vi.fn(),
    previewProjectTemplateSnapshotCapture: vi.fn(),
    resolveProjectTemplateCandidateSelection: vi.fn(),
  }
})

const taskA: api.TaskCandidate = {
  ref: "task-a",
  project_id: "project-1",
  project_seq: 1,
  title: "准备上线",
  status: "pending",
  assignees: [],
  warning_count: 0,
}

const taskZ: api.TaskCandidate = {
  ...taskA,
  ref: "task-z",
  project_seq: 1001,
  title: "上线复盘",
}

const emptyPage = { items: [], total: 0, limit: 50, offset: 0 }
const preview: api.CapturePreview = {
  selection: {
    task_refs: ["task-a"],
    series_refs: [],
    config_keys: [],
    automation_rule_ids: [],
  },
  source_hash: "source-hash-1",
  counts: { tasks: 1, series: 0, configs: 0, automations: 0 },
  blocking_issues: [],
  warnings: [],
  snapshot: {
    project: { description: "" },
    configs: [],
    tasks: [],
    series: [],
    automations: [],
  },
}

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined
  let reject: (reason?: unknown) => void = () => undefined
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, reject, resolve }
}

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>{children}</TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function renderCaptureWizard(
  props: Partial<React.ComponentProps<typeof ProjectTemplateCaptureWizard>> = {}
) {
  return render(
    <ProjectTemplateCaptureWizard
      mode="append"
      onOpenChange={() => undefined}
      open
      sourceProject={{ id: "project-1", slug: "launch", name: "上线项目" }}
      templateRef="launch-template"
      workspaceSlug="acme"
      {...props}
    />,
    { wrapper }
  )
}

async function enterSelectionStep() {
  await userEvent.click(screen.getByRole("button", { name: "下一步" }))
  expect(await screen.findByRole("tab", { name: /^任务/ })).toBeTruthy()
}

describe("ProjectTemplateCaptureWizard", () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.mocked(api.listProjectTemplateTaskCandidates).mockImplementation(
      async (_workspace, _project, options) => ({
        items: options.offset === 50 ? [taskZ] : [taskA],
        total: 1001,
        limit: 50,
        offset: options.offset ?? 0,
      })
    )
    vi.mocked(api.listProjectTemplateSeriesCandidates).mockResolvedValue(
      emptyPage
    )
    vi.mocked(api.listProjectTemplateConfigCandidates).mockResolvedValue(
      emptyPage
    )
    vi.mocked(api.listProjectTemplateAutomationCandidates).mockResolvedValue(
      emptyPage
    )
    vi.mocked(api.resolveProjectTemplateCandidateSelection).mockResolvedValue({
      refs: [],
      total: 0,
      source_hash: "resolve-hash",
    })
    vi.mocked(api.previewProjectTemplateSnapshotCapture).mockResolvedValue(
      preview
    )
    vi.mocked(api.appendProjectTemplateSnapshot).mockResolvedValue({} as never)
  })

  it("keeps explicit selections while filters and pages change", async () => {
    renderCaptureWizard()
    await enterSelectionStep()

    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.type(screen.getByLabelText("搜索任务"), "上线")
    await userEvent.click(screen.getByRole("button", { name: "下一页" }))
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "上线复盘" })
    )
    expect(screen.getByRole("button", { name: "已选 2 项" })).toBeTruthy()

    vi.mocked(
      api.resolveProjectTemplateCandidateSelection
    ).mockResolvedValueOnce({
      refs: ["task-z"],
      total: 1,
      source_hash: "filtered-hash",
    })
    await userEvent.click(
      screen.getByRole("button", { name: "清除当前筛选结果的选择" })
    )
    expect(screen.getByRole("button", { name: "已选 1 项" })).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "已选 1 项" }))
    const selectedDialog = screen.getByRole("dialog", { name: "已选内容" })
    expect(within(selectedDialog).getByText("准备上线")).toBeTruthy()
    expect(within(selectedDialog).queryByText("上线复盘")).toBeNull()
  })

  it("keeps each candidate kind filter state while tabs change", async () => {
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.type(screen.getByLabelText("搜索任务"), "上线")
    await userEvent.click(screen.getByRole("tab", { name: /^循环任务/ }))
    await userEvent.type(screen.getByLabelText("搜索循环任务"), "每周")
    await userEvent.click(screen.getByRole("tab", { name: /^任务/ }))

    expect((screen.getByLabelText("搜索任务") as HTMLInputElement).value).toBe(
      "上线"
    )
  })

  it("sends only explicit arrays and preview source hash", async () => {
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenCalledWith(
      "acme",
      "launch-template",
      expect.objectContaining({
        selection: {
          task_refs: ["task-a"],
          series_refs: [],
          config_keys: [],
          automation_rule_ids: [],
        },
      })
    )
    const previewBody = vi.mocked(api.previewProjectTemplateSnapshotCapture)
      .mock.calls[0][2]
    expect(JSON.stringify(previewBody)).not.toMatch(/filter|query/)

    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "保存快照" }))
    const saveBody = vi.mocked(api.appendProjectTemplateSnapshot).mock
      .calls[0][2]
    expect(saveBody.expected_source_hash).toBe("source-hash-1")
    expect(saveBody.selection).toEqual({
      task_refs: ["task-a"],
      series_refs: [],
      config_keys: [],
      automation_rule_ids: [],
    })
    expect(JSON.stringify(saveBody)).not.toMatch(/filter|query/)
  })

  it("keeps existing selection when selecting every match exceeds the limit", async () => {
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    vi.mocked(
      api.resolveProjectTemplateCandidateSelection
    ).mockRejectedValueOnce(
      new ApiError(422, "project_template_candidate_limit_exceeded", "limit")
    )

    await userEvent.click(
      screen.getByRole("button", { name: "选择全部 1001 条匹配结果" })
    )
    expect(
      await screen.findByText("匹配结果超过模板快照上限，请先筛选再选择。")
    ).toBeTruthy()
    expect(screen.getByRole("button", { name: "已选 1 项" })).toBeTruthy()
  })

  it("does not present attachment issues as resolvable content drops", async () => {
    vi.mocked(api.previewProjectTemplateSnapshotCapture).mockResolvedValueOnce({
      ...preview,
      blocking_issues: [
        {
          code: "project_template_attachment_unsupported",
          source_kind: "task",
          source_ref: "task-a",
          message: "任务描述包含不支持的附件引用",
        },
      ],
    })
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    expect(
      await screen.findByText(
        "附件引用不能作为模板内容保留，请先从来源内容移除。"
      )
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "移除内容引用" })).toBeNull()
  })

  it("writes a content drop only for a resolvable missing content task reference", async () => {
    vi.mocked(api.previewProjectTemplateSnapshotCapture)
      .mockResolvedValueOnce({
        ...preview,
        blocking_issues: [
          {
            code: "project_template_dependency_missing",
            source_kind: "task",
            source_ref: "task-a",
            target_ref: "task-b",
            relation: "content",
            message: "内容任务引用未选择",
          },
        ],
      })
      .mockResolvedValueOnce(preview)
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    expect(screen.queryByRole("button", { name: "移除依赖关系" })).toBeNull()
    await userEvent.click(screen.getByRole("button", { name: "移除内容引用" }))
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.objectContaining({
        resolution: expect.objectContaining({
          drop_content_task_refs: [
            {
              source_kind: "task",
              source_ref: "task-a",
              target_task_ref: "task-b",
            },
          ],
        }),
      })
    )
  })

  it("clears the preview and hash when the anchor date changes", async () => {
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    await userEvent.click(screen.getByRole("button", { name: "返回" }))
    await userEvent.click(screen.getByRole("button", { name: "返回" }))
    await userEvent.clear(screen.getByLabelText("日期锚点"))
    await userEvent.type(screen.getByLabelText("日期锚点"), "2026-08-01")
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))

    expect(screen.getByRole("button", { name: "生成预览" })).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.objectContaining({ anchor_date: "2026-08-01" })
    )
  })

  it("deduplicates a resolution and clears it when its missing target is selected", async () => {
    const dependencyIssue: api.CaptureIssue = {
      code: "project_template_dependency_missing",
      source_kind: "task",
      source_ref: "task-a",
      target_ref: "task-b",
      relation: "depends",
      message: "依赖任务未选择",
    }
    vi.mocked(api.previewProjectTemplateSnapshotCapture)
      .mockResolvedValueOnce({
        ...preview,
        blocking_issues: [dependencyIssue],
      })
      .mockResolvedValueOnce({ ...preview, blocking_issues: [dependencyIssue] })
      .mockResolvedValueOnce(preview)
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    await userEvent.click(screen.getByRole("button", { name: "移除依赖关系" }))
    await userEvent.click(screen.getByRole("button", { name: "移除依赖关系" }))
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.objectContaining({
        resolution: expect.objectContaining({
          drop_depends: [
            {
              source_task_ref: "task-a",
              relation: "depends",
              target_task_ref: "task-b",
            },
          ],
        }),
      })
    )

    await userEvent.click(screen.getByRole("button", { name: "补选引用任务" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.not.objectContaining({ resolution: expect.anything() })
    )
  })

  it("clears a parent-drop resolution when the missing parent is selected", async () => {
    const parentIssue: api.CaptureIssue = {
      code: "project_template_dependency_missing",
      source_kind: "task",
      source_ref: "task-a",
      target_ref: "task-b",
      relation: "parent",
      message: "父任务未选择",
    }
    vi.mocked(api.previewProjectTemplateSnapshotCapture)
      .mockResolvedValueOnce({ ...preview, blocking_issues: [parentIssue] })
      .mockResolvedValueOnce({ ...preview, blocking_issues: [parentIssue] })
      .mockResolvedValueOnce(preview)
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(await screen.findByRole("checkbox", { name: "准备上线" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    await userEvent.click(screen.getByRole("button", { name: "移除父任务关系" }))
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    await userEvent.click(screen.getByRole("button", { name: "补选引用任务" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.not.objectContaining({ resolution: expect.anything() })
    )
  })

  it("reruns a dirty preview with Cmd/Ctrl+Enter", async () => {
    vi.mocked(api.previewProjectTemplateSnapshotCapture)
      .mockResolvedValueOnce({
        ...preview,
        warnings: [
          {
            code: "project_template_date_before_anchor",
            source_kind: "task",
            source_ref: "task-a",
            field: "due",
            message: "任务日期早于锚点",
          },
        ],
      })
      .mockResolvedValueOnce(preview)
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(await screen.findByRole("checkbox", { name: "准备上线" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    await userEvent.click(screen.getByRole("button", { name: "清除 due 日期" }))
    await userEvent.keyboard("{Control>}{Enter}{/Control}")
    await waitFor(() =>
      expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenCalledTimes(2)
    )
  })

  it("requires an explicit ended-series schedule instead of inventing a first due", async () => {
    vi.mocked(api.listProjectTemplateSeriesCandidates).mockResolvedValue({
      items: [
        {
          ref: "series-ended",
          project_id: "project-1",
          title: "已结束循环",
          status: "ended",
          recurrence_rule: "weekly",
          first_due: 1_721_548_800,
          assignees: [],
          created_by: { id: "u1", name: "alice" },
          warning_count: 0,
        },
      ],
      total: 1,
      limit: 50,
      offset: 0,
    })
    vi.mocked(api.previewProjectTemplateSnapshotCapture)
      .mockResolvedValueOnce({
        ...preview,
        blocking_issues: [
          {
            code: "project_template_series_schedule_confirmation_required",
            source_kind: "series",
            source_ref: "series-ended",
            field: "schedule",
            message: "已结束循环任务需要确认排期",
          },
        ],
      })
      .mockResolvedValueOnce(preview)
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(screen.getByRole("tab", { name: /^循环任务/ }))
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "已结束循环" })
    )
    await userEvent.click(screen.getByRole("tab", { name: /^任务/ }))
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    expect(
      (screen.getByRole("button", {
        name: "确认循环排期",
      }) as HTMLButtonElement).disabled
    ).toBe(true)
    await userEvent.type(screen.getByLabelText("循环首次到期日偏移"), "2")
    await userEvent.type(screen.getByLabelText("循环首次到期时间"), "10:30")
    expect(
      (screen.getByRole("button", {
        name: "确认循环排期",
      }) as HTMLButtonElement).disabled
    ).toBe(true)
    await userEvent.clear(screen.getByLabelText("循环首次到期时间"))
    await userEvent.type(
      screen.getByLabelText("循环首次到期时间"),
      "24:00:00"
    )
    expect(
      (screen.getByRole("button", {
        name: "确认循环排期",
      }) as HTMLButtonElement).disabled
    ).toBe(true)
    await userEvent.clear(screen.getByLabelText("循环首次到期时间"))
    await userEvent.type(
      screen.getByLabelText("循环首次到期时间"),
      "10:30:00"
    )
    await userEvent.click(
      screen.getByRole("button", { name: "确认循环排期" })
    )
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.objectContaining({
        resolution: expect.objectContaining({
          series_schedule_overrides: [
            {
              source_series_ref: "series-ended",
              first_due: { day_offset: 2, local_time: "10:30:00" },
              until: null,
              clear_until: true,
            },
          ],
        }),
      })
    )
  })

  it("defaults only pending and waiting tasks plus active series", async () => {
    vi.mocked(api.resolveProjectTemplateCandidateSelection).mockImplementation(
      async (_workspace, _project, input) => {
        if (input.kind === "task" && input.task?.status === "pending") {
          return { refs: ["pending-task"], total: 1, source_hash: "pending" }
        }
        if (input.kind === "task" && input.task?.status === "waiting") {
          return { refs: ["waiting-task"], total: 1, source_hash: "waiting" }
        }
        if (input.kind === "series" && input.series?.status === "active") {
          return { refs: ["active-series"], total: 1, source_hash: "active" }
        }
        throw new Error("unexpected default filter")
      }
    )
    renderCaptureWizard({ mode: "create", templateRef: undefined })
    await enterSelectionStep()

    expect(
      await screen.findByRole("button", { name: "已选 3 项" })
    ).toBeTruthy()
    const filters = vi
      .mocked(api.resolveProjectTemplateCandidateSelection)
      .mock.calls.map((call) => JSON.stringify(call[2]))
      .join(" ")
    expect(filters).toContain('"status":"pending"')
    expect(filters).toContain('"status":"waiting"')
    expect(filters).toContain('"status":"active"')
    expect(filters).not.toMatch(/completed|ended|stopped/)
  })

  it("leaves an over-limit default kind unselected instead of truncating it", async () => {
    vi.mocked(api.resolveProjectTemplateCandidateSelection).mockImplementation(
      async (_workspace, _project, input) => {
        if (input.kind === "task" && input.task?.status === "pending") {
          throw new ApiError(
            422,
            "project_template_candidate_limit_exceeded",
            "limit"
          )
        }
        if (input.kind === "task") {
          return { refs: ["waiting-task"], total: 1, source_hash: "waiting" }
        }
        return { refs: ["active-series"], total: 1, source_hash: "active" }
      }
    )
    renderCaptureWizard({ mode: "create", templateRef: undefined })
    await enterSelectionStep()

    expect(
      await screen.findByText(/任务未自动选择，请先筛选再选择/)
    ).toBeTruthy()
    expect(screen.getByRole("tab", { name: /^任务0$/ })).toBeTruthy()
    expect(screen.getByRole("tab", { name: /^循环任务1$/ })).toBeTruthy()
  })

  it("merges asynchronous defaults with manual selection without exceeding a component limit", async () => {
    const pending = deferred<api.ResolvedCandidateSelection>()
    const waiting = deferred<api.ResolvedCandidateSelection>()
    const active = deferred<api.ResolvedCandidateSelection>()
    vi.mocked(api.resolveProjectTemplateCandidateSelection).mockImplementation(
      async (_workspace, _project, input) => {
        if (input.kind === "task" && input.task?.status === "pending") {
          return pending.promise
        }
        if (input.kind === "task" && input.task?.status === "waiting") {
          return waiting.promise
        }
        return active.promise
      }
    )
    renderCaptureWizard({ mode: "create", templateRef: undefined })
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )

    pending.resolve({
      refs: Array.from({ length: 500 }, (_, index) => `pending-${index}`),
      total: 500,
      source_hash: "pending",
    })
    waiting.resolve({
      refs: Array.from({ length: 500 }, (_, index) => `waiting-${index}`),
      total: 500,
      source_hash: "waiting",
    })
    active.resolve({ refs: ["series-a"], total: 1, source_hash: "active" })

    expect(
      await screen.findByText(/任务未自动选择，请先筛选再选择/)
    ).toBeTruthy()
    expect(screen.getByRole("tab", { name: /^任务1$/ })).toBeTruthy()
    expect(screen.getByRole("tab", { name: /^循环任务1$/ })).toBeTruthy()
  })

  it("blocks preview while defaults resolve and invalidates any preview when they change selection", async () => {
    const pending = deferred<api.ResolvedCandidateSelection>()
    const waiting = deferred<api.ResolvedCandidateSelection>()
    const active = deferred<api.ResolvedCandidateSelection>()
    vi.mocked(api.resolveProjectTemplateCandidateSelection).mockImplementation(
      async (_workspace, _project, input) => {
        if (input.kind === "task" && input.task?.status === "pending") {
          return pending.promise
        }
        if (input.kind === "task" && input.task?.status === "waiting") {
          return waiting.promise
        }
        return active.promise
      }
    )
    renderCaptureWizard({ mode: "create", templateRef: undefined })
    await enterSelectionStep()
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    expect(
      (screen.getByRole("button", { name: "生成预览" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)

    pending.resolve({ refs: ["pending-task"], total: 1, source_hash: "pending" })
    waiting.resolve({ refs: [], total: 0, source_hash: "waiting" })
    active.resolve({ refs: [], total: 0, source_hash: "active" })

    await waitFor(() =>
      expect(
        (screen.getByRole("button", { name: "生成预览" }) as HTMLButtonElement)
          .disabled
      ).toBe(false)
    )
    expect(api.previewProjectTemplateCapture).not.toHaveBeenCalled()
  })

  it("offers task date override controls from preview warnings", async () => {
    vi.mocked(api.previewProjectTemplateSnapshotCapture)
      .mockResolvedValueOnce({
        ...preview,
        warnings: [
          {
            code: "project_template_date_before_anchor",
            source_kind: "task",
            source_ref: "task-a",
            field: "due",
            message: "任务日期早于锚点",
          },
        ],
      })
      .mockResolvedValueOnce(preview)
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    await userEvent.click(screen.getByRole("button", { name: "清除 due 日期" }))
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.objectContaining({
        resolution: expect.objectContaining({
          task_date_overrides: [
            { source_task_ref: "task-a", field: "due", value: null },
          ],
        }),
      })
    )
  })

  it("allows an empty explicit selection through preview and save with all four arrays", async () => {
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.objectContaining({
        selection: {
          task_refs: [],
          series_refs: [],
          config_keys: [],
          automation_rule_ids: [],
        },
      })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "保存快照" }))
    expect(
      vi.mocked(api.appendProjectTemplateSnapshot).mock.calls[0][2].selection
    ).toEqual({
      task_refs: [],
      series_refs: [],
      config_keys: [],
      automation_rule_ids: [],
    })
  })

  it("refreshes selected summaries after source drift without dropping their refs", async () => {
    let taskTitle = "准备上线"
    vi.mocked(api.listProjectTemplateTaskCandidates).mockImplementation(
      async (_workspace, _project, options) => ({
        items:
          options.offset === 50 ? [taskZ] : [{ ...taskA, title: taskTitle }],
        total: 1001,
        limit: 50,
        offset: options.offset ?? 0,
      })
    )
    vi.mocked(api.appendProjectTemplateSnapshot).mockImplementationOnce(
      async () => {
        taskTitle = "变更后的任务标题"
        throw new ApiError(409, "project_template_source_changed", "changed")
      }
    )
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "保存快照" }))
    expect(
      await screen.findByText("来源项目已变化，请检查选择后重新预览。")
    ).toBeTruthy()
    expect(
      (await screen.findAllByText("变更后的任务标题")).length
    ).toBeGreaterThan(0)
    await userEvent.click(screen.getByRole("button", { name: "已选 1 项" }))

    const selectedDialog = screen.getByRole("dialog", { name: "已选内容" })
    expect(within(selectedDialog).getByText("变更后的任务标题")).toBeTruthy()
  })

  it("refreshes selected summaries that are outside the visible candidate page after source drift", async () => {
    let taskZTitle = "上线复盘"
    vi.mocked(api.listProjectTemplateTaskCandidates).mockImplementation(
      async (_workspace, _project, options) => {
        if (options.refs?.includes("task-z")) {
          return {
            items: [{ ...taskZ, title: taskZTitle }],
            total: 1,
            limit: 50,
            offset: 0,
          }
        }
        return {
          items: options.offset === 50 ? [{ ...taskZ, title: taskZTitle }] : [taskA],
          total: 1001,
          limit: 50,
          offset: options.offset ?? 0,
        }
      }
    )
    vi.mocked(api.appendProjectTemplateSnapshot).mockImplementationOnce(
      async () => {
        taskZTitle = "变更后的跨页任务"
        throw new ApiError(409, "project_template_source_changed", "changed")
      }
    )
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(screen.getByRole("button", { name: "下一页" }))
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "上线复盘" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "保存快照" }))
    expect(
      await screen.findByText("来源项目已变化，请检查选择后重新预览。")
    ).toBeTruthy()
    await waitFor(() =>
      expect(
        vi.mocked(api.listProjectTemplateTaskCandidates).mock.calls.some(
          (call) => call[2].refs?.includes("task-z")
        )
      ).toBe(true)
    )
    await userEvent.click(screen.getByRole("button", { name: "已选 1 项" }))
    expect(
      within(screen.getByRole("dialog", { name: "已选内容" })).getByText(
        "变更后的跨页任务"
      )
    ).toBeTruthy()
  })

  it("retries a cancelled ref refresh generation after current-page summaries rerender", async () => {
    const firstRefRefresh = deferred<api.Page<api.TaskCandidate>>()
    let drifted = false
    let refRefreshCalls = 0
    vi.mocked(api.listProjectTemplateTaskCandidates).mockImplementation(
      async (_workspace, _project, options) => {
        if (options.refs?.includes("task-z")) {
          refRefreshCalls++
          if (refRefreshCalls === 1) return firstRefRefresh.promise
          return {
            items: [{ ...taskZ, title: "重试后的跨页任务" }],
            total: 1,
            limit: 50,
            offset: 0,
          }
        }
        if (options.offset === 50) {
          return { items: [taskZ], total: 1001, limit: 50, offset: 50 }
        }
        return {
          items: [
            drifted ? { ...taskA, title: "当前页已更新" } : taskA,
          ],
          total: 1001,
          limit: 50,
          offset: 0,
        }
      }
    )
    vi.mocked(api.appendProjectTemplateSnapshot).mockImplementationOnce(
      async () => {
        drifted = true
        throw new ApiError(409, "project_template_source_changed", "changed")
      }
    )
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(await screen.findByRole("checkbox", { name: "准备上线" }))
    await userEvent.click(screen.getByRole("button", { name: "下一页" }))
    await userEvent.click(await screen.findByRole("checkbox", { name: "上线复盘" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "保存快照" }))
    expect(
      await screen.findByText("来源项目已变化，请检查选择后重新预览。")
    ).toBeTruthy()
    await waitFor(() => expect(refRefreshCalls).toBeGreaterThanOrEqual(2))
    await userEvent.click(screen.getByRole("button", { name: "已选 2 项" }))
    expect(
      within(screen.getByRole("dialog", { name: "已选内容" })).getByText(
        "重试后的跨页任务"
      )
    ).toBeTruthy()
  })

  it("supports Cmd/Ctrl+Enter but blocks Escape and repeat submit while pending", async () => {
    const onOpenChange = vi.fn()
    const pendingPreview = deferred<api.CapturePreview>()
    vi.mocked(api.previewProjectTemplateSnapshotCapture).mockReturnValue(
      pendingPreview.promise
    )
    renderCaptureWizard({ onOpenChange })
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.keyboard("{Control>}{Enter}{/Control}")
    await userEvent.keyboard("{Control>}{Enter}{/Control}")
    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenCalledTimes(1)
    await userEvent.keyboard("{Escape}")
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(
      (screen.getByRole("button", { name: "返回" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)

    pendingPreview.resolve(preview)
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "下一步" })).toBeTruthy()
    )
  })

  it("never displays a secret candidate value", async () => {
    vi.mocked(api.listProjectTemplateConfigCandidates).mockResolvedValue({
      items: [
        {
          ref: "OPENAI_API_KEY",
          key: "OPENAI_API_KEY",
          label: "OpenAI API Key",
          mode: "secret",
          value_type: "string",
          warning_count: 0,
        },
      ],
      total: 1,
      limit: 50,
      offset: 0,
    })
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(screen.getByRole("tab", { name: /配置/ }))

    expect(await screen.findByText("OpenAI API Key")).toBeTruthy()
    expect(screen.getByText("机密值不会显示或复制")).toBeTruthy()
    expect(screen.queryByText(/sk-|secret-value/)).toBeNull()
  })

  it("blocks saving until preview issues are resolved and writes structured resolution", async () => {
    vi.mocked(api.previewProjectTemplateSnapshotCapture)
      .mockResolvedValueOnce({
        ...preview,
        blocking_issues: [
          {
            code: "project_template_dependency_missing",
            source_kind: "task",
            source_ref: "task-a",
            target_ref: "task-b",
            relation: "depends",
            message: "依赖任务未选择",
          },
        ],
      })
      .mockResolvedValueOnce(preview)
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    const issue = await screen.findByRole("alert", { name: "依赖任务未选择" })
    expect(document.activeElement).toBe(issue)
    expect(
      (screen.getByRole("button", { name: "保存快照" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    await userEvent.click(screen.getByRole("button", { name: "移除依赖关系" }))
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))

    expect(api.previewProjectTemplateSnapshotCapture).toHaveBeenLastCalledWith(
      "acme",
      "launch-template",
      expect.objectContaining({
        resolution: expect.objectContaining({
          drop_depends: [
            {
              source_task_ref: "task-a",
              relation: "depends",
              target_task_ref: "task-b",
            },
          ],
        }),
      })
    )
  })

  it("returns to selection after source drift and preserves explicit refs", async () => {
    vi.mocked(api.appendProjectTemplateSnapshot).mockRejectedValueOnce(
      new ApiError(409, "project_template_source_changed", "changed")
    )
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步" }))
    await userEvent.click(screen.getByRole("button", { name: "保存快照" }))

    expect(
      await screen.findByText("来源项目已变化，请检查选择后重新预览。")
    ).toBeTruthy()
    expect(screen.getByRole("button", { name: "已选 1 项" })).toBeTruthy()
    expect(api.listProjectTemplateTaskCandidates.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it("renders a full-screen mobile selected sheet backed by the same store", async () => {
    renderCaptureWizard()
    await enterSelectionStep()
    await userEvent.click(
      await screen.findByRole("checkbox", { name: "准备上线" })
    )
    await userEvent.click(screen.getByRole("button", { name: "已选 1 项" }))

    const sheet = screen.getByRole("dialog", { name: "已选内容" })
    expect(sheet.className).toContain("w-screen")
    expect(within(sheet).getByText("准备上线")).toBeTruthy()
  })
})
