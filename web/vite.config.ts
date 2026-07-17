import path from "node:path"
import { writeFileSync } from "node:fs"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, loadEnv } from "vite"

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "")
  const apiTarget = env.VITE_XUANCHU_API_TARGET || "http://127.0.0.1:9090"
  const consoleBase = env.VITE_XUANCHU_CONSOLE_BASE || "/"
  const normalizedConsoleBase = consoleBase.endsWith("/")
    ? consoleBase
    : `${consoleBase}/`

  return {
    base: normalizedConsoleBase,
    plugins: [react(), tailwindcss(), preserveDistGitkeep()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
      dedupe: ["react", "react-dom"],
    },
    // Vitest 默认以 production 条件解析 react，而 React 19 仅在 development
    // build 导出 act（testing-library 依赖）。在测试模式下强制 dev 条件。
    define: mode === "test"
      ? { "process.env.NODE_ENV": JSON.stringify("development") }
      : undefined,
    server: {
      port: 9095,
      strictPort: true,
      open: normalizedConsoleBase,
      proxy: {
        "/api/v1": {
          target: apiTarget,
          changeOrigin: true,
          timeout: 30000,
        },
        "/sso": {
          target: apiTarget,
          changeOrigin: true,
          timeout: 30000,
        },
        "/auth": {
          target: apiTarget,
          changeOrigin: true,
          timeout: 30000,
        },
        "/mcp": {
          target: apiTarget,
          changeOrigin: true,
          timeout: 30000,
        },
      },
    },
    build: {
      outDir: "../internal/webconsole/dist",
      emptyOutDir: true,
    },
    test: {
      environment: "jsdom",
      globals: true,
      setupFiles: ["./src/test/setup.ts"],
    },
  }
})

function preserveDistGitkeep() {
  return {
    name: "xuanchu-preserve-webconsole-dist-gitkeep",
    closeBundle() {
      // dist 真实产物被 git 忽略；构建后写回占位文件，避免工作区出现删除噪音。
      writeFileSync(
        path.resolve(__dirname, "../internal/webconsole/dist/.gitkeep"),
        ""
      )
    },
  }
}
