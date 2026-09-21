# 贡献指南

感谢关注璇础（Xuanchu）。本项目欢迎 issue、bug 报告、文档改进与功能 PR。

## 开始之前

- 大的功能或边界变化，请先开 issue 讨论，避免与 [ROADMAP.md](./ROADMAP.md) 方向冲突。
- 仓库的完整工程约束（分层边界、测试要求、文档同步规则、SQLite 硬约束等）见 [AGENTS.md](./AGENTS.md)，提交 PR 前请务必阅读。

## 开发环境

- Go 1.25+，且必须保持 `CGO_ENABLED=0` 可构建、可测试（SQLite 使用纯 Go 驱动）
- Node 22+ 与 pnpm 9（仅改动 `web/` 时需要）

## 本地验证

任何提交前至少运行：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

改动 `web/` 时额外运行：

```bash
pnpm --dir web install
pnpm --dir web typecheck && pnpm --dir web lint && pnpm --dir web build && pnpm --dir web test
```

## 提交规范

- 提交信息使用中文，延续 `feat:` / `fix:` / `docs:` / `chore:` 前缀风格。
- 小步提交，每个提交可独立测试、解释与回退。
- 遵守脚本友好约定：stdout 只放结果、stderr 放错误、`--json` 输出保持稳定。

## PR 要求

- 说明改了什么、为什么；附带本地验证结果。
- 用户可见行为变化（命令、默认值、数据库路径等）必须同步更新 `README.md` 与 `docs/`。
- 新增依赖需说明理由；禁止引入需要 CGO 的 SQLite 实现。

## 行为准则

参与本项目即表示同意遵守[行为准则](./CODE_OF_CONDUCT.md)。
