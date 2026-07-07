# config_summary 动态过滤 设计

> 状态：已实现（2026-06-16）
> 日期：2026-06-16
> 背景：在 Agent Skill 文档重构过程中发现 `project_get` 的 `config_summary` 用编译期硬编码白名单（4 个 `agent.*` 键）过滤，与"config schema 运行时可定义"的设计相矛盾，会卡住后续所有集成场景。

## 1. 问题

### 1.1 现状

`project_get` 返回 `config_summary`，由 `internal/mcpserver/resources.go` 的 `filterAgentConfig` 生成。当前实现用硬编码白名单：

```go
var allowedAgentKeys = []string{
    "agent.background", "agent.constraints", "agent.default_context", "agent.handoff",
}
func isAllowedAgentKey(key string) bool {
    return slices.Contains(allowedAgentKeys, key)
}
```

### 1.2 为什么是错的

1. **能写不能读**：系统允许 agent 用 `config_schema_set` + `project_config_set` 定义任意配置键（群绑定的 `integrations.feishu.webhook_url`、`im.group_id`，未来任何集成键）。这些键明确是给 agent 用的，但 `config_summary` 用固定白名单把它们挡在外面。agent 只能再发一次 `config_get`，`project_get` 一次性暴露配置的设计意图落空。

2. **每加一个集成都要改 Go 代码**：照现状，CIO 接新集成、定义新 `integrations.xxx` 键，还要回来往 `allowedAgentKeys` 加一行重新编译。这违背 config schema "运行时可定义、零代码扩展"的设计初衷。

3. **安全模型错位**：`filterAgentConfig` 本意是"别把 secret / 非 agent 相关内部键暴露"，但用"白名单"实现把"安全"和"可扩展"对立了。正确做法是黑名单——排除 secret，其余放行。

4. **测试固化了 bug**：`TestProjectResourceOnlyExposesAllowedAgentKeys` 把白名单行为锁死，让人误以为这是设计。

### 1.3 影响的代码点

- `internal/mcpserver/resources.go`：`filterAgentConfig`、`isAllowedAgentKey`、`allowedAgentKeys`
- `internal/mcpserver/resources_test.go`：`TestProjectResourceOnlyExposesAllowedAgentKeys`
- `internal/mcpserver/integration_test.go`：第 1229-1233、1409-1411 行的 config_summary 断言（需确认是否仍通过）
- 文档：Agent Skill 文档里"config_summary 固定 4 键白名单"的表述需回退为动态过滤

## 2. 目标

`config_summary` 改为：**暴露该 project 的全部配置键，排除 schema 标记为 `secret:true` 的键。** 加集成只改 config schema，零 Go 代码改动。

## 3. 设计

### 3.1 过滤规则

| 规则 | 处理 |
|---|---|
| project 配置键，schema 标记 `secret:true` | **排除**（不回显） |
| project 配置键，schema 未标 secret | **暴露** |
| project 配置键，无 schema 定义 | **暴露**（向后兼容；schema 非强制） |

只过滤 secret。前缀（`agent.*` / `integrations.*` / `im.*`）不再做限制——可定义的键就该可见。

### 3.2 改动

**app 层新增方法**（把"列配置 + 排除 secret"封装在 app 层，mcp 层只调用，符合 AGENTS.md 的分层约束）：

`internal/app/project_config.go`：

```go
// ProjectConfigSummary 返回 project 配置中非 secret 的键值对，供 agent 读取。
// schema 标记为 secret 的键被排除；无 schema 定义的键视为非 secret，照常返回。
func (s *Service) ProjectConfigSummary(projectRef string) (map[string]string, error) {
    all, err := s.ProjectConfigList(projectRef)  // 复用现有，含权限校验
    if err != nil {
        return nil, err
    }
    out := make(map[string]string, len(all))
    for k, v := range all {
        if s.configKeyIsSecret(k) {
            continue
        }
        out[k] = v
    }
    return out, nil
}

// configKeyIsSecret 查询某 key 的 schema 是否标记为 secret。无 schema 视为非 secret。
func (s *Service) configKeyIsSecret(key string) bool {
    def, ok, err := s.configDefRepo.Get(s.workspaceID, key)
    if err != nil || !ok {
        return false
    }
    return def.Secret
}
```

**mcp 层改用新方法**：

`internal/mcpserver/resources.go` 的 `filterAgentConfig` 改为直接调用 `svc.ProjectConfigSummary(ref)`，删除 `isAllowedAgentKey` / `allowedAgentKeys`。

### 3.3 测试

- 删除 `TestProjectResourceOnlyExposesAllowedAgentKeys`，替换为 `TestProjectResourceExposesNonSecretKeys`：设一个普通键（应出现）、一个 secret 键（应排除）、一个无 schema 键（应出现）。
- 确认 integration_test.go 的 config_summary 断言仍通过（它们断言 `agent.background` 出现、`context.default` 在旧实现不出现——新实现下 `context.default` 会暴露，需更新这条断言）。

### 3.4 文档回退

xuanchu-manage-access-and-config 和 xuanchu-govern-projects 里"config_summary 固定 4 键白名单"的表述，改回："config_summary 暴露该 project 全部非 secret 配置；secret 键（schema 标 `secret:true`）不回显。" xuanchu-wire-up-automation 的 feishu-bot-setup.md 相应更新（im.group_id / webhook_url 现在可经 config_summary 读）。

## 4. 不改的

- `ProjectConfigList` 本身不变（仍返回全部键，含 secret，供需要完整配置的场景）。
- secret 的写入/引用机制不变（`secret_refs` / `{{secret.*}}`）。
- secret 值在任何"列表/详情"接口的回显策略不变——本 spec 只动 config_summary 一个出口。

## 5. 验收

- [ ] `filterAgentConfig` 不再含硬编码白名单
- [ ] 新测试覆盖：普通键出现、secret 键排除、无 schema 键出现
- [ ] integration_test.go 的 config_summary 断言更新且通过
- [ ] `go test ./...` + `CGO_ENABLED=0 go test ./...` + `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过
- [ ] 文档回退表述
