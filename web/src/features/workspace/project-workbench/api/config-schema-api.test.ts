import { describe, expect, it } from "vitest"

import { configSchemaPath, type ConfigSchemaDefinition } from "./config-schema-api"

describe("config schema api", () => {
  it("builds workspace-scoped config schema list path", () => {
    expect(configSchemaPath()).toBe("/api/v1/config-schema")
  })

  it("exposes ConfigSchemaDefinition shape", () => {
    const def: ConfigSchemaDefinition = {
      key: "agent.background",
      value_type: "string",
      allowed_scopes: ["project"],
      label: "",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: false,
      created_at: 0,
      modified_at: 0,
    }
    expect(def.key).toBe("agent.background")
  })
})
