import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it } from "vitest"

import { i18n } from "@/i18n"
import type { ConfigEffectiveValue } from "@/features/workspace/config/config-definition-api"
import { renderWithRouter } from "@/test/router-wrapper"

import type {
  ProjectTaskSummary,
  ProjectWorkbenchProject,
} from "../api/project-api"
import { ProjectContextRail } from "./project-context-rail"

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={makeQueryClient()}>
      {renderWithRouter(<>{children}</>)}
    </QueryClientProvider>
  )
}

function project(
  overrides: Partial<ProjectWorkbenchProject> = {}
): ProjectWorkbenchProject {
  return {
    id: "project-1",
    workspace_id: "workspace-1",
    slug: "ops",
    name: "运营项目",
    status: "active",
    task_count: 10,
    pending_count: 6,
    completed_count: 4,
    created_at: 1_800_000_000,
    modified_at: 1_800_001_000,
    ...overrides,
  }
}

function summary(
  overrides: Partial<ProjectTaskSummary> = {}
): ProjectTaskSummary {
  return {
    overdue_count: 1,
    overdue_refs: [],
    high_priority_open_count: 2,
    high_priority_open_refs: [],
    wait_ready_count: 0,
    wait_ready_refs: [],
    unassigned_open_count: 3,
    unassigned_open_refs: [],
    workload: [],
    series_metrics: {
      recurring_series_count: 0,
      active_recurring_series_count: 0,
      open_recurring_occurrence_count: 0,
      overdue_recurring_occurrence_count: 0,
    },
    ...overrides,
  }
}

function configRow(
  overrides: Partial<ConfigEffectiveValue>
): ConfigEffectiveValue {
  return {
    key: "model.provider",
    value: "openai",
    source: "project",
    definition: {
      key: "model.provider",
      value_type: "string",
      allowed_scopes: ["project", "workspace"],
      label: "模型供应商",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: true,
      created_at: 0,
      modified_at: 0,
    },
    show_on_console_home: true,
    missing_required: false,
    ...overrides,
  }
}

describe("ProjectContextRail", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  it("shows project facts as label/value and hides raw secret values", async () => {
    render(
      <ProjectContextRail
        configRows={[
          configRow({ key: "model.provider", value: "openai" }),
          configRow({
            key: "callback.secret",
            value: "••••••",
            definition: {
              key: "callback.secret",
              value_type: "string",
              allowed_scopes: ["project"],
              label: "回调密钥",
              description: "",
              enum_values: [],
              default_value: null,
              required: true,
              secret: true,
              show_on_console_home: true,
              created_at: 0,
              modified_at: 0,
            },
          }),
        ]}
        project={project()}
        summary={summary()}
      />,
      { wrapper: Wrapper }
    )
    await screen.findByText("项目附属信息")
    expect(screen.getByText("模型供应商")).toBeTruthy()
    expect(screen.getByText("openai")).toBeTruthy()
    // secret 只显示「已设置」，不展示原始值
    expect(screen.getByText("回调密钥")).toBeTruthy()
    expect(screen.getByText("已设置")).toBeTruthy()
    expect(screen.queryByText("••••••")).toBeNull()
    // 不使用「运行配置」标题
    expect(screen.queryByText("运行配置")).toBeNull()
  })

  it("renders summary risk counts from project task summary", async () => {
    render(
      <ProjectContextRail
        configRows={[]}
        project={project()}
        summary={summary()}
      />,
      { wrapper: Wrapper }
    )
    await screen.findByText("进度")
    expect(screen.getByText("逾期")).toBeTruthy()
    expect(screen.getByText("高优未完成")).toBeTruthy()
    expect(screen.getByText("等待已到期")).toBeTruthy()
    expect(screen.getByText("未分配任务")).toBeTruthy()
    // 完成进度：4/10 = 40%
    expect(screen.getByText("40%")).toBeTruthy()
  })

  it("shows workload from summary with unassigned bucket", async () => {
    render(
      <ProjectContextRail
        configRows={[]}
        project={project()}
        summary={summary({
          workload: [
            {
              user: { id: "u1", name: "张三" },
              label: "张三",
              open_count: 5,
              overdue_count: 1,
              high_priority_count: 0,
            },
            {
              user: null,
              label: "未分配任务",
              open_count: 3,
              overdue_count: 0,
              high_priority_count: 0,
            },
          ],
        })}
      />,
      { wrapper: Wrapper }
    )
    await screen.findByText("成员待办")
    expect(screen.getByText("张三")).toBeTruthy()
    expect(screen.getByText(/5 待办/)).toBeTruthy()
    // 未分配行 open_count>0 时仍展示（风险计数区也有「未分配任务」，所以 >=2）
    expect(screen.getAllByText("未分配任务").length).toBeGreaterThanOrEqual(2)
  })

  it("shows recurring runtime separately from normal task progress", async () => {
    const summaryWithRecurringMetrics = {
      ...summary(),
      series_metrics: {
        recurring_series_count: 2,
        active_recurring_series_count: 1,
        open_recurring_occurrence_count: 3,
        overdue_recurring_occurrence_count: 1,
      },
    }
    render(
      <ProjectContextRail
        configRows={[]}
        project={project()}
        summary={summaryWithRecurringMetrics}
      />,
      { wrapper: Wrapper }
    )

    await screen.findByText("循环任务运行情况")
    expect(screen.getByText("1 个运行中系列")).toBeTruthy()
    expect(screen.getByText("3 条未完成实例，其中 1 条逾期")).toBeTruthy()
  })

  it("tolerates a legacy summary response without recurring metrics", async () => {
    render(
      <ProjectContextRail
        configRows={[]}
        project={project()}
        summary={summary({ series_metrics: undefined })}
      />,
      { wrapper: Wrapper }
    )

    await screen.findByText("进度")
    expect(screen.queryByText("循环任务运行情况")).toBeNull()
  })

  it("hides unassigned row when its open count is zero", async () => {
    render(
      <ProjectContextRail
        configRows={[]}
        project={project()}
        summary={summary({
          workload: [
            {
              user: { id: "u1", name: "张三" },
              label: "张三",
              open_count: 1,
              overdue_count: 0,
              high_priority_count: 0,
            },
            {
              user: null,
              label: "未分配任务",
              open_count: 0,
              overdue_count: 0,
              high_priority_count: 0,
            },
          ],
        })}
      />,
      { wrapper: Wrapper }
    )
    await screen.findByText("成员待办")
    expect(screen.getByText("张三")).toBeTruthy()
    // 未分配为 0 时负载区不显示该行；只剩风险计数区的「未分配任务」
    expect(screen.getAllByText("未分配任务").length).toBe(1)
  })

  it("shows lightweight error when summary request fails", async () => {
    render(
      <ProjectContextRail
        configRows={[]}
        project={project()}
        summaryError={true}
      />,
      { wrapper: Wrapper }
    )
    await screen.findByText("项目摘要暂不可用")
  })
})
