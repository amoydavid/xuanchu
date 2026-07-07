import { describe, expect, test } from "vitest"
import type { ConfigEnv, UserConfig } from "vite"
import viteConfig from "./vite.config"

function resolveViteConfig(mode = "development"): UserConfig {
  const env: ConfigEnv = {
    command: "serve",
    mode,
    isPreview: false,
    isSsrBuild: false,
  }

  const config = typeof viteConfig === "function" ? viteConfig(env) : viteConfig
  if (config instanceof Promise) {
    throw new Error("vite config test expects a synchronous config")
  }
  return config
}

describe("vite dev proxy", () => {
  test("proxies MCP requests to the Go server instead of serving them from Vite", () => {
    const config = resolveViteConfig()

    expect(config.server?.proxy).toMatchObject({
      "/mcp": {
        target: "http://127.0.0.1:9090",
        changeOrigin: true,
        timeout: 30000,
      },
    })
  })
})
