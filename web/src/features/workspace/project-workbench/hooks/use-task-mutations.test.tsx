import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  addTaskAnnotation,
  addTaskLink,
  createTask,
  deleteTask,
  deleteTaskAnnotation,
  deleteTaskLink,
  doneTask,
  importTasks,
  modifyTask,
  startTask,
  stopTask,
} from "../api/task-api"

import {
  useCreateTaskMutation,
  useImportTasksMutation,
  useModifyTaskMutation,
  useTaskActionMutation,
  useTaskAnnotationMutations,
  useTaskLinkMutations,
} from "./use-task-mutations"

vi.mock("../api/task-api", () => ({
  addTaskAnnotation: vi.fn(),
  addTaskLink: vi.fn(),
  createTask: vi.fn(),
  deleteTask: vi.fn(),
  deleteTaskAnnotation: vi.fn(),
  deleteTaskLink: vi.fn(),
  doneTask: vi.fn(),
  importTasks: vi.fn(),
  modifyTask: vi.fn(),
  startTask: vi.fn(),
  stopTask: vi.fn(),
}))

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

function taskResponse() {
  return {
    uuid: "task-1",
    task_slug: "ads-1",
    title: "写投放日报",
    status: "pending",
    project: "adsops",
  }
}

describe("task mutation hooks", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(addTaskAnnotation).mockResolvedValue(taskResponse())
    vi.mocked(addTaskLink).mockResolvedValue({
      id: "link-1",
      type: "doc",
      url: "https://example.com",
    })
    vi.mocked(createTask).mockResolvedValue(taskResponse())
    vi.mocked(deleteTask).mockResolvedValue(taskResponse())
    vi.mocked(deleteTaskAnnotation).mockResolvedValue(taskResponse())
    vi.mocked(deleteTaskLink).mockResolvedValue(taskResponse())
    vi.mocked(doneTask).mockResolvedValue(taskResponse())
    vi.mocked(importTasks).mockResolvedValue({ imported: 2 })
    vi.mocked(modifyTask).mockResolvedValue(taskResponse())
    vi.mocked(startTask).mockResolvedValue(taskResponse())
    vi.mocked(stopTask).mockResolvedValue(taskResponse())
  })

  it("invalidates project tasks and project summary after creating a task", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const filters = new URLSearchParams({ status: "pending" })
    const { result } = renderHook(
      () => useCreateTaskMutation("acme", "adsops", filters),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await result.current.mutateAsync({
        title: "写投放日报",
        project: "adsops",
      })
    })

    expect(createTask).toHaveBeenCalledWith("acme", {
      title: "写投放日报",
      project: "adsops",
    })
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops", "tasks", "status=pending"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["home", "acme"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["my-tasks", "acme"],
      })
    })
  })

  it("invalidates task detail and project tasks after modifying a task", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(
      () => useModifyTaskMutation("acme", "adsops", "ads-1"),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await result.current.mutateAsync({ title: "写周报" })
    })

    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      title: "写周报",
    })
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["task", "acme", "ads-1"],
        refetchType: "none",
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["task", "acme", "ads-1", "audit"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops", "tasks"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["home", "acme"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["my-tasks", "acme"],
      })
    })
  })

  it("invalidates tasks and timeline after finishing a task", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(
      () => useTaskActionMutation("acme", "adsops", "done"),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await result.current.mutateAsync("ads-1")
    })

    expect(doneTask).toHaveBeenCalledWith("acme", "ads-1")
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops", "tasks"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops", "timeline"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["my-tasks", "acme"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["home", "acme"],
      })
    })
  })

  it("invalidates project task surface after importing tasks", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(
      () => useImportTasksMutation("acme", "adsops"),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await result.current.mutateAsync([
        {
          uuid: "task-1",
          title: "导入任务",
          status: "pending",
          entry: "2026-06-28T00:00:00Z",
          modified: "2026-06-28T00:00:00Z",
          project: "adsops",
        },
      ])
    })

    expect(importTasks).toHaveBeenCalledWith("acme", "adsops", [
      expect.objectContaining({ title: "导入任务" }),
    ])
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops", "tasks"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops", "timeline"],
      })
    })
  })

  it("invalidates task detail when annotations change", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(
      () => useTaskAnnotationMutations("acme", "adsops", "ads-1"),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await result.current.add.mutateAsync({ description: "已同步给投放团队" })
    })

    expect(addTaskAnnotation).toHaveBeenCalledWith("acme", "ads-1", {
      description: "已同步给投放团队",
    })
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["task", "acme", "ads-1"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops", "timeline"],
      })
    })
  })

  it("invalidates task detail when links change", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(
      () => useTaskLinkMutations("acme", "adsops", "ads-1"),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await result.current.remove.mutateAsync("link-1")
    })

    expect(deleteTaskLink).toHaveBeenCalledWith("acme", "ads-1", "link-1")
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["task", "acme", "ads-1"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "adsops", "tasks"],
      })
    })
  })
})
