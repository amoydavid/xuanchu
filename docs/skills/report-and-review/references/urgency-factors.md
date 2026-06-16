# Urgency 因素

`urgency_explain` 返回 `data.factors`，每个因素 `{name, value}` 表示该因素对 urgency 总分的贡献。`data.urgency` 是各因素之和。

## 常见因素

| 因素 | 含义 |
|---|---|
| `priority` | 任务优先级（H/M/L）的贡献 |
| `due` | 临近 due 的贡献（越近越高，逾期更高） |

具体因素和权重取决于系统配置。

## 调整系数

在 workspace 级 config 设 `urgency.*` 键调整各因素权重，例如 `urgency.priority.coeff` 改 priority 的贡献系数。配置操作详见 manage-access-and-config skill。

```json
config_set({
  "workspace": "dajee",
  "scope": "workspace",
  "key": "urgency.priority.coeff",
  "value": "6.0"
})
```

> urgency 系数是 workspace 级业务配置；`agent.*` 配置不在此列。
