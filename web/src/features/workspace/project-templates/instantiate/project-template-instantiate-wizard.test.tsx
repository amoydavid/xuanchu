import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { EditFeedbackProvider } from "@/features/workspace/project-workbench/shared/edit-feedback"
import * as homeApi from "@/features/workspace/home/home-api"
import { i18n } from "@/i18n"
import { ApiError } from "@/lib/api"

import * as membersApi from "../../members/members-api"
import * as api from "../api/project-template-api"
import { ProjectTemplateInstantiateWizard } from "./project-template-instantiate-wizard"

const navigateMock = vi.hoisted(() => vi.fn())

vi.mock("@tanstack/react-router", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-router")>()
  return { ...actual, useNavigate: () => navigateMock }
})

vi.mock("../api/project-template-api", async (importActual) => {
  const actual =
    await importActual<typeof import("../api/project-template-api")>()
  return {
    ...actual,
    getProjectTemplate: vi.fn(),
    instantiateProjectTemplate: vi.fn(),
    listProjectTemplates: vi.fn(),
    previewProjectTemplateInstantiation: vi.fn(),
  }
})

vi.mock("../../members/members-api", async (importActual) => {
  const actual =
    await importActual<typeof import("../../members/members-api")>()
  return { ...actual, listWorkspaceMembers: vi.fn() }
})

vi.mock("@/features/workspace/home/home-api", async (importActual) => {
  const actual = await importActual<typeof import("@/features/workspace/home/home-api")>()
  return { ...actual, getHome: vi.fn() }
})

const oldUser = {
  id: "user-old",
  name: "departed",
  display_name: "已离开成员",
  email: null,
  external_ids: [],
}

const current = {
  id: "snap-3",
  version: 3,
  hash: "hash-3",
  source_project_id: "source-1",
  counts: { tasks: 12, series: 3, configs: 4, automations: 2 },
  required_secret_keys: ["agent.provider.api_key"],
  created_by: { type: "user", user: oldUser },
  created_at: 1,
}

const historical = { ...current, id: "snap-1", version: 1, hash: "hash-1" }

const template = {
  id: "template-1",
  key: "launch",
  name: "标准上线流程",
  description: "模板说明",
  status: "active" as const,
  current_snapshot: current,
  created_by: current.created_by,
  created_at: 1,
  modified_at: 1,
}

const detail = {
  template,
  snapshot: {
    project: { description: "来自模板的项目说明" },
    configs: [],
    tasks: [],
    series: [],
    automations: [],
  },
  versions: [current, historical],
}

function preview(input: api.InstantiateInput): api.InstantiatePreview {
  const secretSource = input.secret_inputs?.["agent.provider.api_key"]
    ? "input"
    : "missing"
  const replacement = input.assignee_replacements?.[oldUser.id]
  const memberResolved = Object.hasOwn(
    input.assignee_replacements ?? {},
    oldUser.id
  )
  return {
    template,
    snapshot: input.snapshot_id === historical.id ? historical : current,
    project: {
      slug: input.project_slug,
      name: input.project_name,
      description: input.description ?? "",
      start_date: input.start_date,
    },
    counts: current.counts,
    secret_resolutions: [
      { key: "agent.provider.api_key", resolved_from: secretSource },
    ],
    assignee_issues: [
      {
        user: oldUser,
        affected_refs: ["task-1"],
        resolution: memberResolved
          ? replacement === null
            ? "removed"
            : "replaced"
          : "unresolved",
      },
    ],
    issues: [
      ...(secretSource === "missing"
        ? [
            {
              code: "project_template_secret_required",
              severity: "blocking",
              component: "config",
              source_ref: "agent.provider.api_key",
              message: "secret required",
            },
          ]
        : []),
      ...(!memberResolved
        ? [
            {
              code: "project_template_member_unavailable",
              severity: "blocking",
              component: "member",
              source_ref: oldUser.id,
              message: "member unavailable",
            },
          ]
        : []),
    ],
    warnings: [],
  }
}

function renderWizard(
  selection: {
    templateRef: string
    snapshotID: string
    snapshotHash: string
  } | null = {
    templateRef: "launch",
    snapshotID: current.id,
    snapshotHash: current.hash,
  },
  onOpenChange = vi.fn(),
  canInstantiate = true
) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <EditFeedbackProvider>
            <ProjectTemplateInstantiateWizard
              canInstantiate={canInstantiate}
              canManage
              initialSelection={selection ?? undefined}
              onOpenChange={onOpenChange}
              open
              writeScopes={["project:write", "task:write", "config:write", "hook:write"]}
              workspaceSlug="acme"
            />
          </EditFeedbackProvider>
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
  return { onOpenChange, queryClient }
}

function WizardNavigationHarness() {
  const [open, setOpen] = useState(true)
  return (
    <EditFeedbackProvider>
      <ProjectTemplateInstantiateWizard
        canInstantiate
        canManage
        initialSelection={{
          templateRef: "launch",
          snapshotID: current.id,
          snapshotHash: current.hash,
        }}
        onOpenChange={setOpen}
        open={open}
        writeScopes={["project:write", "task:write", "config:write", "hook:write"]}
        workspaceSlug="acme"
      />
      {!open ? <p>项目详情已打开</p> : null}
    </EditFeedbackProvider>
  )
}

function renderWizardNavigationHarness() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <WizardNavigationHarness />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

async function fillProject() {
  await screen.findByRole("dialog", { name: "从模板创建项目" })
  await waitFor(() =>
    expect(
      (screen.getByLabelText("项目说明") as HTMLTextAreaElement).value
    ).toBe("来自模板的项目说明")
  )
  await userEvent.type(screen.getByLabelText("项目 Slug"), "newproj")
  await userEvent.type(screen.getByLabelText("项目名称"), "新项目")
  await userEvent.clear(screen.getByLabelText("开始日期"))
  await userEvent.type(screen.getByLabelText("开始日期"), "2026-08-01")
  await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
}

describe("ProjectTemplateInstantiateWizard", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    navigateMock.mockReset()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(api.listProjectTemplates).mockResolvedValue({
      items: [template],
      total: 1,
      limit: 20,
      offset: 0,
    })
    vi.mocked(homeApi.getHome).mockResolvedValue({
      generated_at: 1,
      today: "2026-07-31",
      actor_type: "user",
      my_work: null,
      project_attention: [],
    })
    vi.mocked(api.getProjectTemplate).mockResolvedValue(detail)
    vi.mocked(api.previewProjectTemplateInstantiation).mockImplementation(
      async (_workspace, _ref, input) => preview(input)
    )
    vi.mocked(api.instantiateProjectTemplate).mockResolvedValue({
      project: {
        id: "project-new",
        slug: "newproj",
        name: "新项目",
        status: "planning",
      },
      counts: current.counts,
    })
    vi.mocked(membersApi.listWorkspaceMembers).mockResolvedValue([
      {
        id: "user-new",
        name: "alice",
        display_name: "Alice",
        role: "member",
        joined_at: 1,
        modified_at: 1,
      },
    ])
  })

  it("renders typed prompt fields in project step and sends secrets only in config_inputs", async () => {
    const configInputs: api.ProjectTemplateConfigInput[] = [
      { key: "launch.region", label: "发布区域", description: "选择发布区域", value_type: "string", enum_values: ["cn", "global"], required: true, secret: false, status: "ready" },
      { key: "launch.gray", label: "启用灰度", description: "可选", value_type: "boolean", enum_values: [], required: false, secret: false, status: "ready" },
      { key: "launch.token", label: "发布令牌", description: "仅用于本次创建", value_type: "string", enum_values: [], required: true, secret: true, status: "ready" },
    ]
    const promptCurrent = { ...current, config_inputs: configInputs }
    vi.mocked(api.getProjectTemplate).mockResolvedValueOnce({
      ...detail,
      template: { ...template, current_snapshot: promptCurrent },
      versions: [promptCurrent],
    })
    vi.mocked(api.previewProjectTemplateInstantiation).mockImplementationOnce(
      async (_workspace, _ref, input) => ({
        ...preview({ ...input, secret_inputs: { "agent.provider.api_key": "legacy" } }),
        template: { ...template, current_snapshot: promptCurrent },
        snapshot: promptCurrent,
        config_inputs: configInputs,
        config_resolutions: configInputs.map((item) => ({
          key: item.key,
          required: item.required,
          secret: item.secret,
          status: input.config_inputs?.[item.key] ? "provided" as const : "omitted" as const,
        })),
        secret_resolutions: [],
        assignee_issues: [],
        issues: [],
      })
    )
    renderWizard()
    await screen.findByRole("dialog", { name: "从模板创建项目" })
    await userEvent.type(screen.getByLabelText("项目 Slug"), "newproj")
    await userEvent.type(screen.getByLabelText("项目名称"), "新项目")
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))
    expect(await screen.findAllByText("此配置为必填项")).toHaveLength(2)
    expect(api.previewProjectTemplateInstantiation).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole("combobox", { name: /发布区域/ }))
    await userEvent.click(await screen.findByText("cn"))
    await userEvent.type(screen.getByLabelText(/发布令牌/), "prompt-secret")
    const token = screen.getByLabelText(/发布令牌/) as HTMLInputElement
    expect(token.type).toBe("password")
    expect(screen.queryByRole("button", { name: /显示机密|reveal/i })).toBeNull()
    expect(screen.getByRole("combobox", { name: /启用灰度/ })).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    await waitFor(() =>
      expect(api.previewProjectTemplateInstantiation).toHaveBeenCalledWith(
        "acme",
        "launch",
        expect.objectContaining({
          config_inputs: {
            "launch.region": "cn",
            "launch.token": "prompt-secret",
          },
        })
      )
    )
    const sent = vi.mocked(api.previewProjectTemplateInstantiation).mock.calls[0][2]
    expect("secret_inputs" in sent).toBe(false)
  })

  it("pins a historical preview snapshot and never renders secret values", async () => {
    renderWizard({
      templateRef: "launch",
      snapshotID: historical.id,
      snapshotHash: historical.hash,
    })
    await fillProject()

    expect(await screen.findByText("当前无有效值")).toBeTruthy()
    expect(screen.queryByDisplayValue("sk-fixture")).toBeNull()
    await userEvent.type(
      screen.getByLabelText("agent.provider.api_key"),
      "sk-input"
    )
    await userEvent.selectOptions(
      screen.getByLabelText("处理已离开成员"),
      "user-new"
    )
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步：确认" }))
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))

    expect(api.instantiateProjectTemplate).toHaveBeenCalledWith(
      "acme",
      "launch",
      expect.objectContaining({
        snapshot_id: "snap-1",
        expected_snapshot_hash: "hash-1",
        secret_inputs: { "agent.provider.api_key": "sk-input" },
        assignee_replacements: { "user-old": "user-new" },
      })
    )
    expect(
      await screen.findByText(
        "已创建 12 个任务、3 个循环任务、4 项配置和 2 条停用自动化"
      )
    ).toBeTruthy()
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: { workspaceSlug: "acme", projectSlug: "newproj" },
    })
  })

  it("keeps the success toast after closing the wizard for navigation", async () => {
    renderWizardNavigationHarness()
    await fillProject()
    await userEvent.type(
      screen.getByLabelText("agent.provider.api_key"),
      "sk-input"
    )
    await userEvent.selectOptions(
      screen.getByLabelText("处理已离开成员"),
      "__remove__"
    )
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步：确认" }))
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))

    expect(await screen.findByText("项目详情已打开")).toBeTruthy()
    expect(screen.queryByRole("dialog", { name: "从模板创建项目" })).toBeNull()
    expect((await screen.findByRole("status")).textContent).toContain(
      "已创建 12 个任务、3 个循环任务、4 项配置和 2 条停用自动化"
    )
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: { workspaceSlug: "acme", projectSlug: "newproj" },
    })
  })

  it("only offers preview-reported members for replacement or removal", async () => {
    renderWizard()
    await fillProject()

    const replacement = await screen.findByLabelText("处理已离开成员")
    expect(screen.queryByLabelText("处理 Alice")).toBeNull()
    await userEvent.selectOptions(replacement, "__remove__")
    await userEvent.type(
      screen.getByLabelText("agent.provider.api_key"),
      "sk-input"
    )
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))

    expect(api.previewProjectTemplateInstantiation).toHaveBeenLastCalledWith(
      "acme",
      "launch",
      expect.objectContaining({
        assignee_replacements: { "user-old": null },
      })
    )
    expect(await screen.findByText("已移除指派")).toBeTruthy()
  })

  it("prevents closing and duplicate requests while instantiate is pending", async () => {
    let resolveInstantiate!: (value: api.InstantiateResult) => void
    vi.mocked(api.instantiateProjectTemplate).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveInstantiate = resolve
        })
    )
    const { onOpenChange } = renderWizard()
    await fillProject()
    await userEvent.type(
      screen.getByLabelText("agent.provider.api_key"),
      "sk-input"
    )
    await userEvent.selectOptions(
      screen.getByLabelText("处理已离开成员"),
      "__remove__"
    )
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步：确认" }))
    const submit = screen.getByRole("button", { name: "创建项目" })
    await userEvent.click(submit)
    await userEvent.click(submit)
    await userEvent.keyboard("{Escape}")

    expect(api.instantiateProjectTemplate).toHaveBeenCalledTimes(1)
    expect((submit as HTMLButtonElement).disabled).toBe(true)
    expect(onOpenChange).not.toHaveBeenCalledWith(false)

    resolveInstantiate({
      project: {
        id: "project-new",
        slug: "newproj",
        name: "新项目",
        status: "planning",
      },
      counts: current.counts,
    })
    await waitFor(() => expect(navigateMock).toHaveBeenCalledTimes(1))
  })

  it("selects only an active current snapshot from the projects entry", async () => {
    renderWizard(null)

    await userEvent.click(
      await screen.findByRole("button", { name: /标准上线流程/ })
    )
    expect(screen.getByText("v3 当前")).toBeTruthy()
    expect(screen.queryByText("v1")).toBeNull()
    await userEvent.click(
      screen.getByRole("button", { name: "下一步：项目与配置" })
    )
    await fillProject()

    expect(api.previewProjectTemplateInstantiation).toHaveBeenCalledWith(
      "acme",
      "launch",
      expect.objectContaining({
        snapshot_id: "snap-3",
        expected_snapshot_hash: "hash-3",
      })
    )
    expect(api.listProjectTemplates).toHaveBeenCalledWith("acme", {
      status: "active",
      q: "",
      limit: 20,
      offset: 0,
    })
  })

  it("does not render the wizard without an explicit instantiation grant", () => {
    renderWizard(null, vi.fn(), false)

    expect(screen.queryByRole("dialog", { name: "从模板创建项目" })).toBeNull()
  })

  it("returns hash drift to template selection without adopting the new version", async () => {
    vi.mocked(api.previewProjectTemplateInstantiation).mockRejectedValueOnce(
      new ApiError(
        409,
        "project_template_snapshot_hash_mismatch",
        "project_template_snapshot_hash_mismatch"
      )
    )
    renderWizard()
    await screen.findByRole("dialog", { name: "从模板创建项目" })
    await userEvent.type(screen.getByLabelText("项目 Slug"), "newproj")
    await userEvent.type(screen.getByLabelText("项目名称"), "新项目")
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    expect(
      await screen.findByText(
        "模板版本已变化。请重新选择版本；系统不会自动切换到新版本。"
      )
    ).toBeTruthy()
    expect(screen.getByText("选择 active 模板的当前版本")).toBeTruthy()
    expect(api.previewProjectTemplateInstantiation).toHaveBeenCalledTimes(1)
  })

  it("rejects a preview response that silently switches the pinned snapshot", async () => {
    vi.mocked(api.previewProjectTemplateInstantiation).mockImplementationOnce(
      async (_workspace, _ref, input) => ({
        ...preview(input),
        snapshot: {
          ...current,
          id: "snap-4",
          version: 4,
          hash: "hash-4",
        },
      })
    )
    renderWizard()
    await screen.findByRole("dialog", { name: "从模板创建项目" })
    await userEvent.type(screen.getByLabelText("项目 Slug"), "newproj")
    await userEvent.type(screen.getByLabelText("项目名称"), "新项目")
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    expect(
      await screen.findByText(
        "模板版本已变化。请重新选择版本；系统不会自动切换到新版本。"
      )
    ).toBeTruthy()
    expect(screen.queryByText(/Preview 已固定 v4/)).toBeNull()
  })

  it("keeps non-secret fields and clears secret input after a server error", async () => {
    vi.mocked(api.instantiateProjectTemplate).mockRejectedValueOnce(
      new ApiError(500, "unknown", "unknown")
    )
    renderWizard()
    await fillProject()
    await userEvent.type(
      screen.getByLabelText("agent.provider.api_key"),
      "sk-input"
    )
    await userEvent.selectOptions(
      screen.getByLabelText("处理已离开成员"),
      "__remove__"
    )
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    await userEvent.click(screen.getByRole("button", { name: "下一步：确认" }))
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))

    expect(
      await screen.findByText(
        "创建失败。项目信息已保留，请重新填写机密值后重试。（unknown）"
      )
    ).toBeTruthy()
    expect(
      (screen.getByLabelText("agent.provider.api_key") as HTMLInputElement)
        .value
    ).toBe("")
    expect(sessionStorage.getItem("sk-input")).toBeNull()
    expect(localStorage.getItem("sk-input")).toBeNull()

    await userEvent.click(screen.getByRole("button", { name: "返回" }))
    expect((screen.getByLabelText("项目 Slug") as HTMLInputElement).value).toBe(
      "newproj"
    )
    expect((screen.getByLabelText("项目名称") as HTMLInputElement).value).toBe(
      "新项目"
    )
    expect(
      (screen.getByLabelText("项目说明") as HTMLTextAreaElement).value
    ).toBe("来自模板的项目说明")
  })

  it("blocks preview until the selected snapshot detail can be loaded", async () => {
    vi.mocked(api.getProjectTemplate).mockRejectedValueOnce(
      new ApiError(500, "unknown", "unknown")
    )
    renderWizard()

    expect(await screen.findByText("读取模板版本失败")).toBeTruthy()
    expect(
      (screen.getByRole("button", { name: "生成预览" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(api.previewProjectTemplateInstantiation).not.toHaveBeenCalled()
  })

  it("uses the server workspace date instead of the browser local date", async () => {
    renderWizard()

    await waitFor(() =>
      expect(
        (screen.getByLabelText("开始日期") as HTMLInputElement).value
      ).toBe("2026-07-31")
    )
    expect(homeApi.getHome).toHaveBeenCalledTimes(1)
  })

  it("freezes form input while preview is in flight", async () => {
    let resolvePreview!: (value: api.InstantiatePreview) => void
    vi.mocked(api.previewProjectTemplateInstantiation).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolvePreview = resolve
        })
    )
    renderWizard()
    await screen.findByRole("dialog", { name: "从模板创建项目" })
    await userEvent.type(screen.getByLabelText("项目 Slug"), "newproj")
    await userEvent.type(screen.getByLabelText("项目名称"), "新项目")
    await userEvent.click(screen.getByRole("button", { name: "生成预览" }))

    const slug = screen.getByLabelText("项目 Slug") as HTMLInputElement
    expect(slug.disabled).toBe(true)
    await userEvent.type(slug, "changed")
    expect(slug.value).toBe("newproj")
    resolvePreview(
      preview({
        snapshot_id: "snap-3",
        expected_snapshot_hash: "hash-3",
        project_slug: "newproj",
        project_name: "新项目",
        start_date: "2026-07-31",
      })
    )

    expect(await screen.findByText("补齐创建条件")).toBeTruthy()
  })

  it("searches and pages active templates from the project entry", async () => {
    vi.mocked(api.listProjectTemplates).mockImplementation(
      async (_workspace, options) => ({
        items:
          options.offset === 20
            ? [{ ...template, id: "template-2", key: "next", name: "下一页模板" }]
            : [template],
        total: 21,
        limit: 20,
        offset: options.offset,
      })
    )
    renderWizard(null)

    await userEvent.type(await screen.findByLabelText("搜索模板"), "上线")
    await userEvent.click(screen.getByRole("button", { name: "搜索" }))
    await waitFor(() =>
      expect(api.listProjectTemplates).toHaveBeenLastCalledWith("acme", {
        status: "active",
        q: "上线",
        limit: 20,
        offset: 0,
      })
    )
    await userEvent.click(screen.getByRole("button", { name: "下一页" }))
    expect(await screen.findByText("下一页模板")).toBeTruthy()
  })

  it("uses Cmd/Ctrl+Enter to advance after selecting a template", async () => {
    renderWizard(null)
    await userEvent.click(
      await screen.findByRole("button", { name: /标准上线流程/ })
    )
    await userEvent.keyboard("{Control>}{Enter}{/Control}")

    expect(await screen.findByLabelText("项目 Slug")).toBeTruthy()
  })

  it("uses Cmd/Ctrl+Enter to generate a preview from project information", async () => {
    renderWizard()
    await screen.findByRole("dialog", { name: "从模板创建项目" })
    await userEvent.type(screen.getByLabelText("项目 Slug"), "newproj")
    await userEvent.type(screen.getByLabelText("项目名称"), "新项目")
    await userEvent.keyboard("{Control>}{Enter}{/Control}")

    await waitFor(() =>
      expect(api.previewProjectTemplateInstantiation).toHaveBeenCalledTimes(1)
    )
  })

  it("uses Cmd/Ctrl+Enter to re-preview dirty resolutions", async () => {
    renderWizard()
    await fillProject()
    await userEvent.type(
      screen.getByLabelText("agent.provider.api_key"),
      "sk-input"
    )
    await userEvent.selectOptions(
      screen.getByLabelText("处理已离开成员"),
      "__remove__"
    )
    await userEvent.keyboard("{Control>}{Enter}{/Control}")

    await waitFor(() =>
      expect(api.previewProjectTemplateInstantiation).toHaveBeenCalledTimes(2)
    )
  })

  it("uses Cmd/Ctrl+Enter to confirm and create a project", async () => {
    renderWizard()
    await fillProject()
    await userEvent.type(
      screen.getByLabelText("agent.provider.api_key"),
      "sk-input"
    )
    await userEvent.selectOptions(
      screen.getByLabelText("处理已离开成员"),
      "__remove__"
    )
    await userEvent.click(screen.getByRole("button", { name: "重新预览" }))
    await userEvent.keyboard("{Control>}{Enter}{/Control}")
    expect(await screen.findByRole("button", { name: "创建项目" })).toBeTruthy()

    await userEvent.keyboard("{Control>}{Enter}{/Control}")
    await waitFor(() =>
      expect(api.instantiateProjectTemplate).toHaveBeenCalledTimes(1)
    )
  })
})
