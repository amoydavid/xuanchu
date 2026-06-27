import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  createProject,
  modifyProject,
  transitionProject,
} from "../api/project-api"

import {
  useCreateProjectMutation,
  useModifyProjectMutation,
  useTransitionProjectMutation,
} from "./use-project-mutations"

vi.mock("../api/project-api", () => ({
  createProject: vi.fn(),
  modifyProject: vi.fn(),
  transitionProject: vi.fn(),
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

function projectResponse() {
  return {
    id: "project-1",
    workspace_id: "workspace-1",
    slug: "console",
    name: "Console",
    status: "active",
    task_count: 0,
    pending_count: 0,
    completed_count: 0,
    created_at: 1,
    modified_at: 1,
  }
}

describe("project mutation hooks", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(createProject).mockResolvedValue(projectResponse())
    vi.mocked(modifyProject).mockResolvedValue(projectResponse())
    vi.mocked(transitionProject).mockResolvedValue(projectResponse())
  })

  it("invalidates the project list after creating a project", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(() => useCreateProjectMutation("acme"), {
      wrapper: makeWrapper(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({ slug: "console", name: "Console" })
    })

    expect(createProject).toHaveBeenCalledWith("acme", {
      slug: "console",
      name: "Console",
    })
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["projects", "acme"],
      })
    })
  })

  it("invalidates project detail and list after modifying a project", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(
      () => useModifyProjectMutation("acme", "console"),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await result.current.mutateAsync({ name: "Console v2" })
    })

    expect(modifyProject).toHaveBeenCalledWith("acme", "console", {
      name: "Console v2",
    })
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "console"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["projects", "acme"],
      })
    })
  })

  it("invalidates detail, list, timeline, and tasks after transitioning a project", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(
      () => useTransitionProjectMutation("acme", "console"),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await result.current.mutateAsync("archived")
    })

    expect(transitionProject).toHaveBeenCalledWith(
      "acme",
      "console",
      "archived"
    )
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "console"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["projects", "acme"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "console", "timeline"],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["project", "acme", "console", "tasks"],
      })
    })
  })
})
