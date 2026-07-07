import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
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

// RailHarness 让 ProjectContextRail 以受控方式使用，便于 collapse/expand 测试。
function RailHarness(
  props: Omit<
    React.ComponentProps<typeof ProjectContextRail>,
    "railOpen" | "onRailOpenChange"
  > & { initialOpen?: boolean }
) {
  const { initialOpen = true, ...rest } = props
  const [open, setOpen] = useState(initialOpen)
  return (
    <ProjectContextRail {...rest} onRailOpenChange={setOpen} railOpen={open} />
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

  it("collapses and expands with icon buttons", async () => {
    const user = userEvent.setup()
    render(
      <RailHarness
        configRows={[]}
        project={project()}
        summary={summary()}
      />,
      { wrapper: Wrapper }
    )
    // RouterProvider 异步加载，先 await 初始内容
    await screen.findByText("项目信息")
    await user.click(screen.getByRole("button", { name: "收起右栏" }))
    // 收起后只保留展开按钮，右栏区块内容隐藏
    expect(screen.queryByText("项目信息")).toBeNull()
    expect(screen.getByRole("button", { name: "展开右栏" })).toBeTruthy()
  })

  it("shows project facts as label/value and hides raw secret values", async () => {
    render(
      <RailHarness
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
      <RailHarness
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
      <RailHarness
        configRows={[]}
        project={project()}
        summary={summary({
          workload: [
            {
              label: "张三",
              open_count: 5,
              overdue_count: 1,
              high_priority_count: 0,
            },
            {
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
    await screen.findByText("负责人负载")
    expect(screen.getByText("张三")).toBeTruthy()
    expect(screen.getByText(/5 待办/)).toBeTruthy()
  })

  it("shows lightweight error when summary request fails", async () => {
    render(
      <RailHarness
        configRows={[]}
        project={project()}
        summaryError={true}
      />,
      { wrapper: Wrapper }
    )
    await screen.findByText("项目摘要暂不可用")
  })
})
