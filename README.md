# taskg — Taskwarrior 风格的多用户多 Workspace 任务管理系统（Go 版）

`taskg` 是一个用 **纯 Go** 实现的 Taskwarrior 风格任务管理系统：

- 单一二进制：同时承担 **本地 CLI / 远程 CLI 客户端 / HTTP API 服务端 / MCP Server** 四种形态
- 数据库：**SQLite（`modernc.org/sqlite`，零 CGO）**，可跨平台交叉编译
- 多用户、多 workspace、行级隔离
- 兼容 Taskwarrior 的核心命令名、JSON 数据格式与 urgency 公式

## 详细需求

参见 `docs/requirements.md`。该文档梳理了：

- 上游 Taskwarrior 的数据模型 / CLI 语法 / 报表 / Urgency / DOM / Hook / 同步语义
- 多用户多 workspace、MCP、飞书触发等扩展需求
- SQLite 表结构草案、CLI 命令分级、MCP 工具 JSON Schema 草案
- 所有结论附参考来源链接

## 状态

M0 已实现。

## M0 本地 CLI 用法

```bash
go build -o taskg ./cmd/taskg

# 添加任务
./taskg add "Write project spec" project:taskg +planning due:tomorrow
./taskg add "Review PR" priority:H +review

# 查看任务列表
./taskg list

# 查看任务详情
./taskg info 1

# 修改任务
./taskg 1 modify priority:H +next
./taskg 1 modify project:backend

# 完成任务
./taskg 1 done

# 删除任务
./taskg 1 delete

# 导出为 JSON
./taskg export

# 导入 JSON
./taskg import tasks.json

# 查看配置
./taskg show

# 设置配置
./taskg config set date.format rfc3339
./taskg config get date.format
```

默认数据库路径为 `~/.local/share/taskg/taskg.db`，可用 `--db` 或 `TASKG_DB` 环境变量覆盖。

