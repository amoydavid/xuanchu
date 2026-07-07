import { describe, expect, it } from "vitest"

import {
  configSchemaKeyPath,
  configSchemaPath,
  configSchemaUsagePath,
  deleteConfigSchema,
  getConfigSchema,
  getConfigSchemaUsage,
  listConfigSchema,
  listProjectEffectiveConfig,
  listWorkspaceEffectiveConfig,
  projectConfigEffectivePath,
  setConfigSchema,
  type ConfigSchemaDefinition,
  type ConfigSchemaInput,
  type ConfigSchemaUsage,
  workspaceConfigEffectivePath,
} from "./config-definition-api"

describe("config definition api paths", () => {
  it("builds config schema list path", () => {
    expect(configSchemaPath()).toBe("/api/v1/config-schema")
  })

  it("builds config schema key path without encoding dots", () => {
    expect(configSchemaKeyPath("ads.roi")).toBe("/api/v1/config-schema/ads.roi")
  })

  it("builds config schema usage path", () => {
    expect(configSchemaUsagePath("ads.roi")).toBe(
      "/api/v1/config-schema/ads.roi/usage"
    )
  })

  it("builds workspace effective config path with console_home flag", () => {
    expect(workspaceConfigEffectivePath({ consoleHome: true })).toBe(
      "/api/v1/config/effective?console_home=true"
    )
    expect(workspaceConfigEffectivePath({ consoleHome: false })).toBe(
      "/api/v1/config/effective"
    )
  })

  it("builds project effective config path", () => {
    expect(projectConfigEffectivePath("api")).toBe(
      "/api/v1/projects/api/config/effective"
    )
  })
})

describe("config definition api types", () => {
  it("exposes ConfigSchemaDefinition shape with show_on_console_home", () => {
    const def: ConfigSchemaDefinition = {
      key: "ads.roi",
      value_type: "number",
      allowed_scopes: ["workspace"],
      label: "ROI",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: false,
      created_at: 0,
      modified_at: 0,
    }
    expect(def.show_on_console_home).toBe(false)
  })

  it("exposes ConfigSchemaInput shape with show_on_console_home", () => {
    const input: ConfigSchemaInput = {
      value_type: "number",
      allowed_scopes: ["workspace"],
      label: "ROI",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: true,
    }
    expect(input.show_on_console_home).toBe(true)
  })

  it("exposes ConfigSchemaUsage shape", () => {
    const usage: ConfigSchemaUsage = {
      key: "ads.budget",
      workspace_values: 1,
      project_values: 2,
      total_values: 3,
    }
    expect(usage.total_values).toBe(3)
  })
})

describe("config definition api functions are wired", () => {
  // 这些用例只保证函数被导出且类型正确；网络层由 workspaceApi* 封装，
  // 这里不 mock HTTP，避免和现有 project-api.test.ts 风格偏离。
  it("exports list/get/set/delete/usage/effective functions", () => {
    expect(typeof listConfigSchema).toBe("function")
    expect(typeof getConfigSchema).toBe("function")
    expect(typeof setConfigSchema).toBe("function")
    expect(typeof deleteConfigSchema).toBe("function")
    expect(typeof getConfigSchemaUsage).toBe("function")
    expect(typeof listWorkspaceEffectiveConfig).toBe("function")
    expect(typeof listProjectEffectiveConfig).toBe("function")
  })
})
